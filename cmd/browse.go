package cmd

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/adnathanail/lmao/internal/browser"
	"github.com/adnathanail/lmao/internal/lunchmoney"
)

// api is the part of lunchmoney.Client the view uses, so tests can fake it.
type api interface {
	UnreviewedTransactions(ctx context.Context) ([]lunchmoney.Transaction, error)
	Categories(ctx context.Context) ([]lunchmoney.Category, error)
	ManualAccounts(ctx context.Context) ([]lunchmoney.Account, error)
	PlaidAccounts(ctx context.Context) ([]lunchmoney.Account, error)
	Tags(ctx context.Context) ([]lunchmoney.Tag, error)
	UpdateTransaction(ctx context.Context, id int64, change lunchmoney.Change) (lunchmoney.Transaction, error)
}

type mode int

const (
	modeBrowse mode = iota
	modePicker
	modeNotes
)

// snapshot is what undo restores: a transaction's status and category.
type snapshot struct {
	status     string
	categoryID *int64
}

func snapshotOf(tx lunchmoney.Transaction) snapshot {
	return snapshot{tx.Status, tx.CategoryID}
}

// row is one transaction in the list. before is its status and category
// before they were first changed in this session (nil if they haven't been),
// which undo goes back to. While saving, a change is being sent for it, and
// no other change can be made to it until that finishes, so they can't
// arrive out of order.
type row struct {
	tx     lunchmoney.Transaction
	before *snapshot
	saving bool
}

func (r *row) reviewed() bool { return r.tx.Status == lunchmoney.StatusReviewed }

// model is the full-screen view. It only changes in Update: requests run as
// commands and report back with messages.
type model struct {
	api api

	rows         []row     // every unreviewed transaction loaded, in order
	visible      []int     // indexes in rows of those in the month shown
	cursor       int       // index in visible
	offset       int       // first row of visible shown
	month        time.Time // the first of the month shown, in UTC
	detailOffset int       // first line of the details shown

	width, height int

	categories []lunchmoney.Category // in the app's order, groups included
	catByID    map[int64]lunchmoney.Category
	manual     map[int64]lunchmoney.Account
	plaid      map[int64]lunchmoney.Account
	tags       map[int64]string

	loading     bool
	loadedOnce  bool // a load has succeeded
	oldestFirst bool // the list's order; newest first by default
	ticking     bool // the spinner's tick is scheduled
	frame       int

	message    string
	messageErr bool

	mode   mode
	picker picker
	notes  notesEditor

	openURL func(string) error // opens a page in the browser; tests replace it
}

func newModel(a api) *model {
	return &model{api: a, month: monthOf(time.Now()), openURL: browser.Open}
}

// monthOf is the first of t's month, in UTC.
func monthOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

type (
	loadedMsg struct {
		txs        []lunchmoney.Transaction
		categories []lunchmoney.Category
		manual     []lunchmoney.Account
		plaid      []lunchmoney.Account
		tags       []lunchmoney.Tag
		err        error
	}
	savedMsg struct {
		id     int64
		tx     lunchmoney.Transaction
		err    error
		before snapshot // the row's status and category when the change was sent
		undo   bool
	}
	openedMsg    struct{ err error }
	rateLimitMsg struct{ wait time.Duration }
	tickMsg      struct{}
)

func (m *model) Init() tea.Cmd { return m.load() }

// load fetches the transactions and everything needed to describe them, all
// at once.
func (m *model) load() tea.Cmd {
	m.loading = true
	a := m.api
	fetch := func() tea.Msg {
		ctx := context.Background()
		var msg loadedMsg
		var wg sync.WaitGroup
		errs := make([]error, 5)
		wg.Go(func() { msg.txs, errs[0] = a.UnreviewedTransactions(ctx) })
		wg.Go(func() { msg.categories, errs[1] = a.Categories(ctx) })
		wg.Go(func() { msg.manual, errs[2] = a.ManualAccounts(ctx) })
		wg.Go(func() { msg.plaid, errs[3] = a.PlaidAccounts(ctx) })
		wg.Go(func() { msg.tags, errs[4] = a.Tags(ctx) })
		wg.Wait()
		// They usually fail for the same reason (a bad token, no
		// connection), so one is enough to say why.
		for _, err := range errs {
			if err != nil {
				msg.err = err
				break
			}
		}
		return msg
	}
	return tea.Batch(fetch, m.startTick())
}

func (m *model) busy() bool {
	if m.loading {
		return true
	}
	for i := range m.rows {
		if m.rows[i].saving {
			return true
		}
	}
	return false
}

func (m *model) startTick() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking = true
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.mode == modeNotes {
			m.notes.setWidth(m.detailWidth())
		}
	case tickMsg:
		m.ticking = false
		m.frame++
		if m.busy() {
			cmd = m.startTick()
		}
	case loadedMsg:
		m.loaded(msg)
	case savedMsg:
		m.saved(msg)
	case openedMsg:
		if msg.err != nil {
			m.setError(msg.err.Error())
		}
	case rateLimitMsg:
		m.setError("Lunch Money's rate limit was reached: retrying in " + msg.wait.Round(time.Second).String())
	case tea.KeyPressMsg:
		switch m.mode {
		case modePicker:
			cmd = m.pickerKey(msg)
		case modeNotes:
			cmd = m.notesKey(msg)
		default:
			cmd = m.key(msg)
		}
	case tea.MouseClickMsg:
		m.click(msg.Mouse())
	case tea.MouseWheelMsg:
		m.wheel(msg.Mouse())
	default:
		if m.mode == modeNotes {
			cmd = m.notes.update(msg)
		}
	}
	m.scroll()
	return m, cmd
}

// loaded replaces the rows with a fresh load, keeping the same transaction
// selected if it's still there. Rows reviewed in this session aren't in it
// (only unreviewed ones are loaded), so they go.
func (m *model) loaded(msg loadedMsg) {
	m.loading = false
	if msg.err != nil {
		m.setError("Couldn't load transactions: " + msg.err.Error())
		return
	}
	var selected int64 = -1
	if r := m.selected(); r != nil {
		selected = r.tx.ID
	}
	m.rows = make([]row, len(msg.txs))
	for i, tx := range msg.txs {
		m.rows[i] = row{tx: tx}
	}
	m.sortRows(selected)
	m.categories = msg.categories
	m.catByID = map[int64]lunchmoney.Category{}
	for _, c := range msg.categories {
		m.catByID[c.ID] = c
	}
	m.manual = accountMap(msg.manual)
	m.plaid = accountMap(msg.plaid)
	m.tags = map[int64]string{}
	for _, t := range msg.tags {
		m.tags[t.ID] = t.Name
	}
	m.loadedOnce = true
	m.detailOffset = 0
	if m.message != "" && m.messageErr {
		m.message = ""
	}
}

// sortRows sorts the rows by date, newest first (as the Lunch Money app
// lists them) or oldest first, and selects the row with the given ID, or
// the first row of the month if it isn't there.
func (m *model) sortRows(selected int64) {
	slices.SortStableFunc(m.rows, func(a, b row) int {
		c := cmp.Or(strings.Compare(a.tx.Date, b.tx.Date), cmp.Compare(a.tx.ID, b.tx.ID))
		if !m.oldestFirst {
			c = -c
		}
		return c
	})
	m.filter(selected)
}

// filter shows the rows in the month shown, and selects the row with the
// given ID, or the first if it isn't there.
func (m *model) filter(selected int64) {
	key := m.month.Format("2006-01")
	m.visible = m.visible[:0]
	m.cursor = 0
	for i := range m.rows {
		if strings.HasPrefix(m.rows[i].tx.Date, key) {
			if m.rows[i].tx.ID == selected {
				m.cursor = len(m.visible)
			}
			m.visible = append(m.visible, i)
		}
	}
}

// showMonth moves by months, and selects the first row in the new month.
func (m *model) showMonth(by int) {
	m.month = m.month.AddDate(0, by, 0)
	m.filter(-1)
	m.offset, m.detailOffset = 0, 0
}

func accountMap(accounts []lunchmoney.Account) map[int64]lunchmoney.Account {
	out := make(map[int64]lunchmoney.Account, len(accounts))
	for _, a := range accounts {
		out[a.ID] = a
	}
	return out
}

func (m *model) setError(s string) { m.message, m.messageErr = s, true }
func (m *model) note(s string)     { m.message, m.messageErr = s, false }

func (m *model) selected() *row {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return &m.rows[m.visible[m.cursor]]
}

func (m *model) rowByID(id int64) *row {
	for i := range m.rows {
		if m.rows[i].tx.ID == id {
			return &m.rows[i]
		}
	}
	return nil
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	m.message = ""
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-max(1, m.listHeight()-1))
	case "pgdown":
		m.move(max(1, m.listHeight()-1))
	case "home", "g":
		m.move(-len(m.visible))
	case "end", "G":
		m.move(len(m.visible))
	case "left", "h":
		m.showMonth(-1)
	case "right", "l":
		m.showMonth(1)
	case "K", "shift+up":
		m.detailOffset = max(0, m.detailOffset-1)
	case "J", "shift+down":
		m.detailOffset++
	case "s":
		// Start from the top of the new order.
		m.oldestFirst = !m.oldestFirst
		m.sortRows(-1)
		m.offset, m.detailOffset = 0, 0
	case "R":
		if !m.loading {
			return m.load()
		}
	case "r":
		return m.review()
	case "u":
		return m.undo()
	case "c":
		return m.openPicker()
	case "n":
		return m.openNotes()
	case "o":
		return m.openInBrowser()
	}
	return nil
}

func (m *model) move(by int) {
	c := max(0, min(len(m.visible)-1, m.cursor+by))
	if c != m.cursor {
		m.detailOffset = 0
	}
	m.cursor = c
}

// nextUnreviewed selects the next unreviewed row below the selected one
// that isn't being saved, if there is one.
func (m *model) nextUnreviewed() {
	for i := m.cursor + 1; i < len(m.visible); i++ {
		if r := &m.rows[m.visible[i]]; !r.reviewed() && !r.saving {
			m.move(i - m.cursor)
			return
		}
	}
}

// editable returns the selected row if a change can be made to it now,
// otherwise saying why not.
func (m *model) editable() *row {
	r := m.selected()
	if r == nil {
		return nil
	}
	if r.saving {
		m.note("Still saving the last change to this transaction")
		return nil
	}
	return r
}

func (m *model) review() tea.Cmd {
	r := m.editable()
	if r == nil {
		return nil
	}
	if r.reviewed() {
		m.note("Already reviewed: u undoes it")
		return nil
	}
	if r.tx.CategoryID == nil {
		m.note("Set a category first: c sets one and marks it reviewed")
		return nil
	}
	cmd := m.save(r, lunchmoney.Change{Status: lunchmoney.StatusReviewed}, false)
	m.nextUnreviewed()
	return cmd
}

func (m *model) undo() tea.Cmd {
	r := m.editable()
	if r == nil {
		return nil
	}
	if r.before == nil {
		m.note("Nothing to undo: this transaction hasn't been changed here")
		return nil
	}
	change := lunchmoney.Change{Status: r.before.status}
	if !sameID(r.before.categoryID, r.tx.CategoryID) {
		change.SetCategory, change.CategoryID = true, r.before.categoryID
	}
	if change.Status == r.tx.Status && !change.SetCategory {
		r.before = nil
		return nil
	}
	return m.save(r, change, true)
}

// setCategory sets the selected row's category and marks it reviewed.
func (m *model) setCategory(id int64) tea.Cmd {
	r := m.editable()
	if r == nil {
		return nil
	}
	if r.reviewed() && sameID(r.tx.CategoryID, &id) {
		return nil
	}
	cmd := m.save(r, lunchmoney.Change{Status: lunchmoney.StatusReviewed, SetCategory: true, CategoryID: &id}, false)
	m.nextUnreviewed()
	return cmd
}

// openInBrowser opens the selected transaction in the Lunch Money web app.
func (m *model) openInBrowser() tea.Cmd {
	r := m.selected()
	if r == nil {
		return nil
	}
	open, u := m.openURL, browser.TransactionURL(r.tx.ID)
	return func() tea.Msg {
		return openedMsg{err: open(u)}
	}
}

func (m *model) setNotes(notes string) tea.Cmd {
	r := m.editable()
	if r == nil || notes == r.tx.NotesText() {
		return nil
	}
	return m.save(r, lunchmoney.Change{Notes: &notes}, false)
}

// save sends a change for a row in the background. The row only changes once
// Lunch Money confirms it (saved).
func (m *model) save(r *row, change lunchmoney.Change, undo bool) tea.Cmd {
	r.saving = true
	id, before, a := r.tx.ID, snapshotOf(r.tx), m.api
	send := func() tea.Msg {
		tx, err := a.UpdateTransaction(context.Background(), id, change)
		return savedMsg{id: id, tx: tx, err: err, before: before, undo: undo}
	}
	return tea.Batch(send, m.startTick())
}

func (m *model) saved(msg savedMsg) {
	r := m.rowByID(msg.id)
	if r == nil {
		return // reloaded since
	}
	r.saving = false
	if msg.err != nil {
		m.setError("Couldn't save " + r.tx.Payee + ": " + msg.err.Error())
		return
	}
	switch {
	case msg.undo:
		r.before = nil
	case r.before == nil && snapshotOf(msg.tx) != msg.before:
		r.before = &msg.before
	}
	// The update's response leaves out metadata, which the list request
	// asked for.
	if msg.tx.PlaidMetadata == nil {
		msg.tx.PlaidMetadata = r.tx.PlaidMetadata
	}
	if msg.tx.CustomMetadata == nil {
		msg.tx.CustomMetadata = r.tx.CustomMetadata
	}
	r.tx = msg.tx
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// click selects the row clicked in the list.
func (m *model) click(mouse tea.Mouse) {
	if mouse.Button != tea.MouseLeft || m.mode != modeBrowse {
		return
	}
	line := mouse.Y - headerLines
	if line < 0 || line >= m.listHeight() || (m.wide() && mouse.X >= m.listWidth()) {
		return
	}
	if i := m.offset + line; i < len(m.visible) {
		m.move(i - m.cursor)
	}
}

// wheel moves the selection, or scrolls the details when over them.
func (m *model) wheel(mouse tea.Mouse) {
	by := 0
	switch mouse.Button {
	case tea.MouseWheelUp:
		by = -1
	case tea.MouseWheelDown:
		by = 1
	}
	overDetails := m.wide() && mouse.X >= m.listWidth() || !m.wide() && mouse.Y-headerLines > m.listHeight()
	switch {
	case overDetails && m.mode == modeBrowse:
		m.detailOffset = max(0, m.detailOffset+by)
	case m.mode == modeBrowse:
		m.move(by)
	case m.mode == modePicker:
		m.picker.move(by)
	}
}
