package cmd

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/adnathanail/lmao/internal/lunchmoney"
)

// pickItem is a category that can be assigned to a transaction.
type pickItem struct {
	id     int64
	label  string // "Group › Category", or the name for one outside a group
	search string // label, lower case
	income bool
}

// picker chooses a category, narrowing the list to those matching what's
// typed.
type picker struct {
	query   string
	items   []pickItem
	matches []int // indexes in items
	cursor  int   // index in matches
	offset  int
}

func newPicker(categories []lunchmoney.Category, byID map[int64]lunchmoney.Category, current *int64) picker {
	var p picker
	for _, c := range categories {
		if c.IsGroup || c.Archived {
			continue
		}
		label := categoryLabel(c, byID)
		p.items = append(p.items, pickItem{id: c.ID, label: label, search: strings.ToLower(label), income: c.IsIncome})
	}
	p.filter()
	if current != nil {
		for i, mi := range p.matches {
			if p.items[mi].id == *current {
				p.cursor = i
			}
		}
	}
	return p
}

// categoryLabel is "Group › Category", or the category's name if it isn't
// in a group.
func categoryLabel(c lunchmoney.Category, byID map[int64]lunchmoney.Category) string {
	if c.GroupID != nil {
		if g, ok := byID[*c.GroupID]; ok {
			return g.Name + " › " + c.Name
		}
	}
	return c.Name
}

// filter keeps the items containing every word typed, in any order.
func (p *picker) filter() {
	words := strings.Fields(strings.ToLower(p.query))
	p.matches = p.matches[:0]
	for i, it := range p.items {
		ok := true
		for _, w := range words {
			if !strings.Contains(it.search, w) {
				ok = false
				break
			}
		}
		if ok {
			p.matches = append(p.matches, i)
		}
	}
	p.cursor, p.offset = 0, 0
}

func (p *picker) move(by int) {
	p.cursor = max(0, min(len(p.matches)-1, p.cursor+by))
}

func (p *picker) chosen() (pickItem, bool) {
	if p.cursor < 0 || p.cursor >= len(p.matches) {
		return pickItem{}, false
	}
	return p.items[p.matches[p.cursor]], true
}

// lines renders the picker in at most height lines of width.
func (p *picker) lines(payee string, width, height int) []string {
	out := []string{
		styleHeading.Render("Category for ") + payee,
		"> " + p.query + "█",
	}
	if len(p.matches) == 0 {
		return append(out, styleDim.Render("No categories match"))
	}
	h := max(1, height-len(out))
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+h {
		p.offset = p.cursor - h + 1
	}
	for i := p.offset; i < len(p.matches) && i < p.offset+h; i++ {
		it := p.items[p.matches[i]]
		text := fit(it.label, width-2)
		switch {
		case i == p.cursor:
			text = styleSelected.Render("▶ " + text)
		case it.income:
			text = "  " + styleIncome.Render(text)
		default:
			text = "  " + text
		}
		out = append(out, text)
	}
	return out
}

func (m *model) openPicker() tea.Cmd {
	r := m.editable()
	if r == nil {
		return nil
	}
	m.picker = newPicker(m.categories, m.catByID, r.tx.CategoryID)
	if len(m.picker.items) == 0 {
		m.note("There are no categories to choose from")
		return nil
	}
	m.mode = modePicker
	return nil
}

func (m *model) pickerKey(msg tea.KeyPressMsg) tea.Cmd {
	p := &m.picker
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		m.mode = modeBrowse
	case "enter":
		it, ok := p.chosen()
		if !ok {
			return nil
		}
		m.mode = modeBrowse
		return m.setCategory(it.id)
	case "up", "ctrl+p":
		p.move(-1)
	case "down", "ctrl+n":
		p.move(1)
	case "pgup":
		p.move(-10)
	case "pgdown":
		p.move(10)
	case "backspace":
		if p.query != "" {
			r := []rune(p.query)
			p.query = string(r[:len(r)-1])
			p.filter()
		}
	case "ctrl+u":
		p.query = ""
		p.filter()
	default:
		if msg.Text != "" {
			p.query += msg.Text
			p.filter()
		}
	}
	return nil
}

// notesEditor edits a transaction's notes.
type notesEditor struct {
	ta textarea.Model
}

func newNotesEditor(value string, width int) notesEditor {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetHeight(4)
	ta.SetWidth(max(10, width))
	// Enter saves, so new lines need another key.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	plain := textarea.StyleState{
		Base:        lipgloss.NewStyle(),
		Text:        lipgloss.NewStyle(),
		CursorLine:  lipgloss.NewStyle(),
		EndOfBuffer: lipgloss.NewStyle(),
		Placeholder: styleDim,
		Prompt:      lipgloss.NewStyle(),
		Selection:   lipgloss.NewStyle().Reverse(true),
	}
	styles := ta.Styles()
	styles.Focused, styles.Blurred = plain, plain
	styles.Cursor.Color = nil
	styles.Cursor.Blink = false
	ta.SetStyles(styles)
	ta.Placeholder = "No notes"
	ta.SetValue(value)
	return notesEditor{ta: ta}
}

func (n *notesEditor) setWidth(w int) {
	n.ta.SetWidth(max(10, w))
}

func (n *notesEditor) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	n.ta, cmd = n.ta.Update(msg)
	return cmd
}

func (m *model) openNotes() tea.Cmd {
	r := m.editable()
	if r == nil {
		return nil
	}
	m.notes = newNotesEditor(r.tx.NotesText(), m.detailWidth())
	m.mode = modeNotes
	return m.notes.ta.Focus()
}

func (m *model) notesKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		m.mode = modeBrowse
		return nil
	case "enter":
		m.mode = modeBrowse
		return m.setNotes(strings.TrimSpace(m.notes.ta.Value()))
	}
	return m.notes.update(msg)
}
