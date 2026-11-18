package cmd

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/adnathanail/lmao/internal/lunchmoney"
)

const (
	headerLines = 1
	wideFrom    = 90 // columns from which the details go beside the list
	panelGap    = " │ "
	labelWidth  = 15
)

func (m *model) wide() bool { return m.width >= wideFrom }

func (m *model) bodyHeight() int { return max(1, m.height-headerLines-m.footerLines()) }

// footerLines is the message line, and however many lines the key hints
// take.
func (m *model) footerLines() int { return 1 + len(m.keyLines()) }

func (m *model) listHeight() int {
	if m.wide() {
		return m.bodyHeight()
	}
	return max(1, m.bodyHeight()/2)
}

func (m *model) listWidth() int {
	if m.wide() {
		return m.width * 58 / 100
	}
	return m.width
}

func (m *model) detailWidth() int {
	if m.wide() {
		return max(10, m.width-m.listWidth()-lipgloss.Width(panelGap))
	}
	return max(10, m.width)
}

func (m *model) detailHeight() int {
	if m.wide() {
		return m.bodyHeight()
	}
	return max(1, m.bodyHeight()-m.listHeight()-1) // the separator
}

// scroll keeps the selected row on screen.
func (m *model) scroll() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(0, min(m.offset, len(m.visible)-h))
}

// fit truncates s to width, then pads it to width.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = ansi.Truncate(s, width, "…")
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

func (m *model) categoryName(id *int64) string {
	if id == nil {
		return ""
	}
	c, ok := m.catByID[*id]
	if !ok {
		return "#" + strconv.FormatInt(*id, 10)
	}
	return categoryLabel(c, m.catByID)
}

// listCategory is the category as the list shows it: just its name, as the
// group would take up most of the column.
func (m *model) listCategory(id *int64) string {
	if id == nil {
		return ""
	}
	if c, ok := m.catByID[*id]; ok {
		return c.Name
	}
	return "#" + strconv.FormatInt(*id, 10)
}

// listLines renders the rows from offset, each width wide.
func (m *model) listLines(width, height int) []string {
	amounts := make([]string, len(m.rows))
	credits := make([]bool, len(m.rows))
	amountW, catW := 0, 0
	for i := range m.rows {
		tx := m.rows[i].tx
		amounts[i], credits[i] = listAmount(tx.Amount, tx.Currency)
		amountW = max(amountW, lipgloss.Width(amounts[i]))
		catW = max(catW, lipgloss.Width(m.listCategory(tx.CategoryID)))
	}
	// Size the category column for any category, so it doesn't change
	// width as categories are set.
	for _, c := range m.categories {
		if !c.IsGroup && !c.Archived {
			catW = max(catW, lipgloss.Width(c.Name))
		}
	}
	// marker, date, payee, category, amount, with a space between each.
	const markerW, dateW = 2, 10
	rest := max(0, width-markerW-dateW-amountW-3)
	catW = min(max(catW, 1), max(8, rest*2/5))
	payeeW := max(0, rest-catW)

	var lines []string
	for v := m.offset; v < len(m.visible) && len(lines) < height; v++ {
		i := m.visible[v]
		r := &m.rows[i]
		marker := " "
		switch {
		case r.saving:
			marker = spinnerFrames[m.frame%len(spinnerFrames)]
		case r.reviewed():
			marker = "✓"
		}
		cat := m.listCategory(r.tx.CategoryID)
		catText := fit(cat, catW)
		if cat == "" {
			catText = fit("—", catW)
		}
		amount := strings.Repeat(" ", amountW-lipgloss.Width(amounts[i])) + amounts[i]
		cells := []string{
			marker + " ",
			fit(r.tx.Date, dateW) + " ",
			fit(r.tx.Payee, payeeW) + " ",
			catText + " ",
			amount,
		}
		line := fit(strings.Join(cells, ""), width)
		switch {
		case v == m.cursor && r.reviewed():
			line = styleSelected.Inherit(styleReviewed).Render(line)
		case v == m.cursor:
			line = styleSelected.Render(line)
		case r.reviewed():
			line = styleReviewed.Render(line)
		default:
			// Style the cells separately: the date and no category are
			// dim, and money in and out have their own colours.
			cells[1] = styleDim.Render(r.tx.Date) + " "
			if cat == "" {
				cells[3] = styleDim.Render(catText) + " "
			}
			if credits[i] {
				cells[4] = styleIncome.Render(amount)
			} else {
				cells[4] = styleSpend.Render(amount)
			}
			if r.saving {
				cells[0] = styleDim.Render(marker) + " "
			}
			line = strings.Join(cells, "")
			line += strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
		}
		lines = append(lines, line)
	}
	if len(m.visible) == 0 && !m.loading && m.loadedOnce {
		text := "Nothing to review"
		if len(m.rows) > 0 {
			text += " in " + m.month.Format("January 2006") + styleDim.Render(": ←/→ for other months")
		}
		lines = append(lines, styleReviewed.Render(text))
	}
	return lines
}

// field is one line of the details: a label and its value.
type field struct {
	label, value string
}

// jsonFields are the fields holding JSON, which wrap at the width rather
// than at spaces (which would leave lone punctuation on lines).
var jsonFields = map[string]bool{"Custom data": true, "Plaid data": true}

func (m *model) fields(r *row) (main, more []field) {
	tx := r.tx
	amount, credit := formatAmount(tx.Amount, tx.Currency)
	if credit {
		amount = styleIncome.Render(amount + " in")
	} else {
		amount = styleSpend.Render(amount + " out")
	}
	category := m.categoryName(tx.CategoryID)
	if category == "" {
		category = styleDim.Render("Uncategorised")
	}
	status := tx.Status
	if r.reviewed() {
		status = styleReviewed.Render(status)
	}
	if tx.IsPending {
		status += " · pending"
	}
	if r.saving {
		status += styleDim.Render(" · saving…")
	}
	notes := tx.NotesText()
	if notes == "" {
		notes = styleDim.Render("—")
	}
	main = []field{
		{"Amount", amount},
		{"Date", tx.Date},
		{"Category", category},
		{"Account", m.accountName(tx)},
		{"Notes", notes},
		{"Status", status},
	}
	if len(tx.TagIDs) > 0 {
		var names []string
		for _, id := range tx.TagIDs {
			if n, ok := m.tags[id]; ok {
				names = append(names, n)
			} else {
				names = append(names, "#"+strconv.FormatInt(id, 10))
			}
		}
		main = append(main, field{"Tags", strings.Join(names, ", ")})
	}

	add := func(label string, value *string) {
		if value != nil && *value != "" {
			more = append(more, field{label, *value})
		}
	}
	addID := func(label string, id *int64) {
		if id != nil {
			more = append(more, field{label, strconv.FormatInt(*id, 10)})
		}
	}
	if tx.OriginalName != nil && *tx.OriginalName != tx.Payee {
		add("Original name", tx.OriginalName)
	}
	add("Source", tx.Source)
	if base := strconv.FormatFloat(tx.ToBase, 'f', 2, 64); !sameAmount(tx.Amount, base) {
		text, _ := formatAmount(base, "")
		more = append(more, field{"In base", text})
	}
	addID("Recurring ID", tx.RecurringID)
	add("External ID", tx.ExternalID)
	addID("Split from", tx.SplitParentID)
	addID("In group", tx.GroupParentID)
	for _, f := range []field{{"Created", tx.CreatedAt}, {"Updated", tx.UpdatedAt}} {
		if f.value != "" {
			more = append(more, field{f.label, formatTime(f.value)})
		}
	}
	more = append(more, field{"ID", strconv.FormatInt(tx.ID, 10)})
	if s := prettyJSON(tx.CustomMetadata); s != "" {
		more = append(more, field{"Custom data", s})
	}
	if s := prettyJSON(tx.PlaidMetadata); s != "" {
		more = append(more, field{"Plaid data", s})
	}
	return main, more
}

func sameAmount(a, b string) bool {
	x, _ := formatAmount(a, "")
	y, _ := formatAmount(b, "")
	return x == y
}

func (m *model) accountName(tx lunchmoney.Transaction) string {
	var a lunchmoney.Account
	var ok bool
	switch {
	case tx.PlaidAccountID != nil:
		a, ok = m.plaid[*tx.PlaidAccountID]
		if !ok {
			return "Plaid account #" + strconv.FormatInt(*tx.PlaidAccountID, 10)
		}
	case tx.ManualAccountID != nil:
		a, ok = m.manual[*tx.ManualAccountID]
		if !ok {
			return "Manual account #" + strconv.FormatInt(*tx.ManualAccountID, 10)
		}
	default:
		return "Cash"
	}
	name := a.Label()
	if a.Mask != nil && *a.Mask != "" {
		name += " ••" + *a.Mask
	}
	if a.InstitutionName != nil && *a.InstitutionName != "" && *a.InstitutionName != name {
		name += styleDim.Render(" · " + *a.InstitutionName)
	}
	return name
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return ""
	}
	var b bytes.Buffer
	if json.Indent(&b, raw, "", "  ") != nil {
		return string(raw)
	}
	return b.String()
}

// details renders the selected transaction's fields, wrapped to width.
func (m *model) details(width int) []string {
	r := m.selected()
	if r == nil {
		return nil
	}
	lines := wrap(styleHeading.Render(r.tx.Payee), width)
	lines = append(lines, "")
	main, more := m.fields(r)
	put := func(fs []field, style lipgloss.Style) {
		valueW := max(1, width-labelWidth)
		for _, f := range fs {
			label := style.Render(fit(f.label, labelWidth-1)) + " "
			indent := strings.Repeat(" ", labelWidth)
			wrapped := wrap(f.value, valueW)
			if jsonFields[f.label] {
				wrapped = hardWrap(f.value, valueW)
			}
			for i, l := range wrapped {
				if i == 0 {
					lines = append(lines, label+l)
				} else {
					lines = append(lines, indent+l)
				}
			}
		}
	}
	put(main, styleDim)
	lines = append(lines, "")
	put(more, styleDim)
	return lines
}

// wrap splits s into lines at most width wide, keeping styles from leaking
// into the next line.
func wrap(s string, width int) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		out = append(out, strings.Split(lipgloss.Wrap(l, width, ""), "\n")...)
	}
	return out
}

// hardWrap splits s into lines exactly width wide.
func hardWrap(s string, width int) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		out = append(out, strings.Split(ansi.Hardwrap(l, width, true), "\n")...)
	}
	return out
}

// rightPanel is what goes beside (or below) the list: the details, or the
// picker or notes editor in their place.
func (m *model) rightPanel(width, height int) []string {
	r := m.selected()
	switch {
	case m.mode == modePicker && r != nil:
		return m.picker.lines(r.tx.Payee, width, height)
	case m.mode == modeNotes && r != nil:
		lines := []string{styleHeading.Render("Notes for ") + r.tx.Payee, ""}
		return append(lines, strings.Split(m.notes.ta.View(), "\n")...)
	}
	lines := m.details(width)
	if len(lines) > 0 {
		m.detailOffset = max(0, min(m.detailOffset, len(lines)-height))
		lines = lines[m.detailOffset:]
	}
	return lines
}

func (m *model) header() string {
	parts := []string{styleHeading.Render("lmao"), styleHeading.Render("‹ " + m.month.Format("January 2006") + " ›")}
	reviewed, elsewhere := 0, 0
	for _, i := range m.visible {
		if m.rows[i].reviewed() {
			reviewed++
		}
	}
	for i := range m.rows {
		if !m.rows[i].reviewed() {
			elsewhere++
		}
	}
	elsewhere -= len(m.visible) - reviewed
	if m.loadedOnce {
		parts = append(parts, plural(len(m.visible)-reviewed, "to review", "to review"))
		if reviewed > 0 {
			parts = append(parts, styleReviewed.Render(plural(reviewed, "reviewed", "reviewed")))
		}
		if elsewhere > 0 {
			parts = append(parts, styleDim.Render(plural(elsewhere, "in other months", "in other months")))
		}
	}
	if m.oldestFirst {
		parts = append(parts, "oldest first")
	}
	if m.loading {
		parts = append(parts, styleDim.Render(spinnerFrames[m.frame%len(spinnerFrames)]+" Loading…"))
	}
	return strings.Join(parts, styleDim.Render(" · "))
}

func (m *model) messageLine() string {
	switch {
	case m.messageErr:
		return styleError.Render(m.message)
	case m.message != "":
		return styleWarn.Render(m.message)
	}
	return ""
}

// hints are the keys the footer lists for the current mode.
func (m *model) hints() []string {
	switch m.mode {
	case modePicker:
		return []string{"type to filter", "↑/↓ select", "enter set and review", "esc cancel"}
	case modeNotes:
		return []string{"enter save", "alt+enter new line", "esc cancel"}
	}
	return []string{"r review", "c category", "n notes", "u undo", "o open", "←/→ month", "s sort", "J/K scroll details", "R reload", "q quit"}
}

// keyLines lays the hints out in as many lines as they need at the
// window's width, breaking only between hints.
func (m *model) keyLines() []string {
	const sep = " · "
	var lines []string
	line := ""
	for _, h := range m.hints() {
		switch {
		case line == "":
			line = h
		case lipgloss.Width(line+sep+h) <= m.width:
			line += sep + h
		default:
			lines = append(lines, line)
			line = h
		}
	}
	lines = append(lines, line)
	for i, l := range lines {
		lines[i] = styleDim.Render(ansi.Truncate(l, m.width, "…"))
	}
	return lines
}

func (m *model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "lmao"
	if m.width == 0 || m.height == 0 {
		return v
	}

	listW := m.listWidth()
	list := m.listLines(listW, m.listHeight())
	for len(list) < m.listHeight() {
		list = append(list, strings.Repeat(" ", listW))
	}
	for i, l := range list {
		list[i] = fit(l, listW)
	}

	var body []string
	if m.wide() {
		right := m.rightPanel(m.detailWidth(), m.detailHeight())
		for i := range m.bodyHeight() {
			r := ""
			if i < len(right) {
				r = right[i]
			}
			body = append(body, list[i]+styleDim.Render(panelGap)+ansi.Truncate(r, m.detailWidth(), "…"))
		}
	} else {
		body = append(body, list...)
		body = append(body, styleDim.Render(strings.Repeat("─", m.width)))
		right := m.rightPanel(m.detailWidth(), m.detailHeight())
		for i := range m.detailHeight() {
			r := ""
			if i < len(right) {
				r = right[i]
			}
			body = append(body, ansi.Truncate(r, m.width, "…"))
		}
	}

	out := []string{ansi.Truncate(m.header(), m.width, "…")}
	out = append(out, body...)
	out = append(out, ansi.Truncate(m.messageLine(), m.width, "…"))
	out = append(out, m.keyLines()...)
	v.SetContent(strings.Join(out[:min(len(out), m.height)], "\n"))
	return v
}
