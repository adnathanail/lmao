package cmd

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/adnathanail/lmao/internal/lunchmoney"
)

func id(n int64) *int64 { return &n }

// fakeAPI keeps transactions in memory and records the changes sent.
type fakeAPI struct {
	mu      sync.Mutex
	txs     []lunchmoney.Transaction
	changes []lunchmoney.Change
	fail    error
}

func (f *fakeAPI) UnreviewedTransactions(context.Context) ([]lunchmoney.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []lunchmoney.Transaction
	for _, tx := range f.txs {
		if tx.Status == lunchmoney.StatusUnreviewed {
			out = append(out, tx)
		}
	}
	return out, nil
}

func (f *fakeAPI) Categories(context.Context) ([]lunchmoney.Category, error) {
	return []lunchmoney.Category{
		{ID: 1, Name: "Food", IsGroup: true},
		{ID: 2, Name: "Groceries", GroupID: id(1)},
		{ID: 3, Name: "Eating out", GroupID: id(1)},
		{ID: 4, Name: "Transport"},
		{ID: 5, Name: "Old", Archived: true},
		{ID: 6, Name: "Salary", IsIncome: true},
	}, nil
}

func (f *fakeAPI) ManualAccounts(context.Context) ([]lunchmoney.Account, error) { return nil, nil }

func (f *fakeAPI) PlaidAccounts(context.Context) ([]lunchmoney.Account, error) {
	return []lunchmoney.Account{{ID: 9, Name: "Current account"}}, nil
}

func (f *fakeAPI) Tags(context.Context) ([]lunchmoney.Tag, error) { return nil, nil }

func (f *fakeAPI) UpdateTransaction(_ context.Context, txID int64, c lunchmoney.Change) (lunchmoney.Transaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return lunchmoney.Transaction{}, f.fail
	}
	f.changes = append(f.changes, c)
	for i := range f.txs {
		tx := &f.txs[i]
		if tx.ID != txID {
			continue
		}
		if c.Status != "" {
			tx.Status = c.Status
		}
		if c.SetCategory {
			tx.CategoryID = c.CategoryID
		}
		if c.Notes != nil {
			tx.Notes = c.Notes
		}
		return *tx, nil
	}
	return lunchmoney.Transaction{}, errors.New("not found")
}

func newFake() *fakeAPI {
	return &fakeAPI{txs: []lunchmoney.Transaction{
		{ID: 1, Date: "2026-11-10", Payee: "Corner shop", Amount: "4.5000", Currency: "gbp", Status: "unreviewed", PlaidAccountID: id(9)},
		{ID: 2, Date: "2026-11-12", Payee: "Bus", Amount: "2.8000", Currency: "gbp", Status: "unreviewed", CategoryID: id(4)},
		{ID: 3, Date: "2026-11-11", Payee: "Employer", Amount: "-2100.0000", Currency: "gbp", Status: "unreviewed"},
		{ID: 4, Date: "2026-10-30", Payee: "Garage", Amount: "80.0000", Currency: "gbp", Status: "unreviewed", CategoryID: id(4)},
	}}
}

// run feeds msg to the model and then the messages its commands produce,
// leaving out ticks and anything else that would wait.
func run(t *testing.T, m *model, msg tea.Msg) {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		_, cmd := m.Update(msg)
		queue = append(queue, collect(cmd)...)
	}
}

func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, collect(c)...)
		}
		return out
	case loadedMsg, savedMsg:
		return []tea.Msg{msg}
	}
	return nil
}

func press(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

func typeText(t *testing.T, m *model, s string) {
	t.Helper()
	for _, r := range s {
		run(t, m, press(string(r)))
	}
}

func loadedModel(t *testing.T, f *fakeAPI) *model {
	t.Helper()
	m := newModel(f)
	m.month = monthOf(time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC))
	run(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	for _, msg := range collect(m.Init()) {
		run(t, m, msg)
	}
	return m
}

func payees(m *model) string {
	var p []string
	for _, i := range m.visible {
		p = append(p, m.rows[i].tx.Payee)
	}
	return strings.Join(p, ",")
}

func TestLoadSortsNewestFirst(t *testing.T) {
	m := loadedModel(t, newFake())
	if got := payees(m); got != "Bus,Employer,Corner shop" {
		t.Fatalf("rows = %s", got)
	}
	run(t, m, press("j"))
	run(t, m, press("j"))
	run(t, m, press("j"))
	if m.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (the last row)", m.cursor)
	}
}

func TestSortFlips(t *testing.T) {
	m := loadedModel(t, newFake())
	run(t, m, press("j")) // Employer
	run(t, m, press("s"))
	if got := payees(m); got != "Corner shop,Employer,Bus" {
		t.Fatalf("oldest first: %s", got)
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want the first row", m.cursor)
	}
	if !strings.Contains(ansi.Strip(m.header()), "oldest first") {
		t.Errorf("header = %q", m.header())
	}
	run(t, m, press("s"))
	if got := payees(m); got != "Bus,Employer,Corner shop" {
		t.Fatalf("newest first: %s", got)
	}
}

func TestMonths(t *testing.T) {
	m := loadedModel(t, newFake())
	if !strings.Contains(ansi.Strip(m.header()), "November 2026") || !strings.Contains(ansi.Strip(m.header()), "1 in other months") {
		t.Fatalf("header = %q", ansi.Strip(m.header()))
	}
	run(t, m, press("j"))
	run(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if got := payees(m); got != "Garage" || m.cursor != 0 {
		t.Fatalf("October: %s, cursor %d", got, m.cursor)
	}
	if h := ansi.Strip(m.header()); !strings.Contains(h, "October 2026") || !strings.Contains(h, "3 in other months") {
		t.Fatalf("header = %q", h)
	}

	// Reviewing works on the month shown, and reloading keeps it.
	run(t, m, press("r"))
	if !m.selected().reviewed() {
		t.Fatal("Garage wasn't reviewed")
	}
	run(t, m, press("R"))
	if got := payees(m); got != "" {
		t.Fatalf("October after reload: %q", got)
	}
	if out := ansi.Strip(m.View().Content); !strings.Contains(out, "Nothing to review in October 2026") {
		t.Fatalf("view:\n%s", out)
	}

	run(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	run(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if got := payees(m); got != "" || m.selected() != nil {
		t.Fatalf("December: %q", got)
	}
	run(t, m, press("r")) // nothing selected: does nothing
	run(t, m, press("h"))
	if got := payees(m); got != "Bus,Employer,Corner shop" {
		t.Fatalf("back to November: %s", got)
	}
}

// A reload keeps the order chosen.
func TestReloadKeepsSort(t *testing.T) {
	m := loadedModel(t, newFake())
	run(t, m, press("s"))
	run(t, m, press("R"))
	if got := payees(m); got != "Corner shop,Employer,Bus" {
		t.Fatalf("after reload: %s", got)
	}
}

func TestReviewAndUndo(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	run(t, m, press("r"))
	if !m.rows[0].reviewed() || m.cursor != 1 {
		t.Fatalf("after r: reviewed = %v, cursor = %d", m.rows[0].reviewed(), m.cursor)
	}
	if len(f.changes) != 1 || f.changes[0].Status != lunchmoney.StatusReviewed || f.changes[0].SetCategory {
		t.Fatalf("changes = %+v", f.changes)
	}

	// It stays in the list, green, and r again does nothing.
	run(t, m, press("k"))
	run(t, m, press("r"))
	if len(f.changes) != 1 {
		t.Fatalf("r on a reviewed row sent %+v", f.changes[1:])
	}
	if !strings.Contains(m.message, "u undoes") {
		t.Errorf("message = %q", m.message)
	}

	run(t, m, press("u"))
	if m.rows[0].reviewed() || m.rows[0].before != nil {
		t.Fatalf("after u: %+v", m.rows[0])
	}
	if c := f.changes[1]; c.Status != lunchmoney.StatusUnreviewed || c.SetCategory {
		t.Fatalf("undo sent %+v", c)
	}

	run(t, m, press("u"))
	if len(f.changes) != 2 {
		t.Fatalf("u on an unchanged row sent %+v", f.changes[2:])
	}
}

func TestReviewNeedsCategory(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	run(t, m, press("j")) // Employer, no category
	run(t, m, press("r"))
	if m.rows[1].reviewed() || len(f.changes) != 0 {
		t.Fatalf("reviewed without a category: %+v, sent %+v", m.rows[1].tx, f.changes)
	}
	if m.cursor != 1 || !strings.Contains(m.message, "Set a category first") {
		t.Fatalf("cursor = %d, message = %q", m.cursor, m.message)
	}
}

func TestCategoryReviewsAndUndoRestoresIt(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	// Bus is in Transport.
	run(t, m, press("c"))
	if m.mode != modePicker {
		t.Fatal("c didn't open the picker")
	}
	if it, _ := m.picker.chosen(); it.id != 4 {
		t.Errorf("preselected %+v, want Transport", it)
	}
	for _, it := range m.picker.items {
		if it.id == 1 || it.id == 5 {
			t.Errorf("picker offers %q", it.label)
		}
	}
	typeText(t, m, "food groc")
	if len(m.picker.matches) != 1 {
		t.Fatalf("matches for %q: %d", m.picker.query, len(m.picker.matches))
	}
	run(t, m, press("enter"))
	if m.mode != modeBrowse {
		t.Fatal("enter didn't close the picker")
	}
	r := m.rows[0]
	if !r.reviewed() || r.tx.CategoryID == nil || *r.tx.CategoryID != 2 {
		t.Fatalf("after setting category: %+v", r.tx)
	}
	if c := f.changes[0]; c.Status != lunchmoney.StatusReviewed || !c.SetCategory || *c.CategoryID != 2 {
		t.Fatalf("sent %+v", c)
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.cursor)
	}

	run(t, m, press("k"))
	run(t, m, press("u"))
	r = m.rows[0]
	if r.reviewed() || r.tx.CategoryID == nil || *r.tx.CategoryID != 4 {
		t.Fatalf("after undo: %+v", r.tx)
	}
	if c := f.changes[1]; c.Status != lunchmoney.StatusUnreviewed || !c.SetCategory || *c.CategoryID != 4 {
		t.Fatalf("undo sent %+v", c)
	}
}

func TestUndoClearsCategoryThatWasEmpty(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	run(t, m, press("j")) // Employer, no category
	run(t, m, press("c"))
	typeText(t, m, "salary")
	run(t, m, press("enter"))
	run(t, m, press("k"))
	run(t, m, press("u"))
	if c := f.changes[1]; !c.SetCategory || c.CategoryID != nil {
		t.Fatalf("undo sent %+v, want the category cleared", c)
	}
	if m.rows[1].tx.CategoryID != nil {
		t.Fatalf("category = %v", *m.rows[1].tx.CategoryID)
	}
}

func TestPickerEscCancels(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	run(t, m, press("c"))
	typeText(t, m, "eat")
	run(t, m, press("esc"))
	if m.mode != modeBrowse || len(f.changes) != 0 {
		t.Fatalf("mode = %v, changes = %+v", m.mode, f.changes)
	}
}

func TestNotes(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	run(t, m, press("n"))
	if m.mode != modeNotes {
		t.Fatal("n didn't open the editor")
	}
	typeText(t, m, "lunch with Sam")
	run(t, m, press("enter"))
	if m.mode != modeBrowse {
		t.Fatal("enter didn't close the editor")
	}
	if got := m.rows[0].tx.NotesText(); got != "lunch with Sam" {
		t.Fatalf("notes = %q", got)
	}
	if c := f.changes[0]; c.Status != "" || c.SetCategory {
		t.Fatalf("notes also sent %+v", c)
	}
	if m.rows[0].reviewed() || m.rows[0].before != nil {
		t.Fatal("saving notes changed the review")
	}

	// Unchanged notes aren't sent.
	run(t, m, press("n"))
	run(t, m, press("enter"))
	if len(f.changes) != 1 {
		t.Fatalf("sent %+v", f.changes[1:])
	}
}

func TestSaveErrorLeavesRow(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	f.fail = errors.New("boom")
	run(t, m, press("r"))
	if m.rows[0].reviewed() || m.rows[0].saving {
		t.Fatalf("row = %+v", m.rows[0])
	}
	if !m.messageErr || !strings.Contains(m.message, "boom") {
		t.Fatalf("message = %q", m.message)
	}
}

func TestSavingRowRefusesChanges(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	m.rows[0].saving = true
	_, cmd := m.Update(press("r"))
	if cmd != nil {
		t.Fatal("r on a saving row returned a command")
	}
	if !strings.Contains(m.message, "Still saving") {
		t.Fatalf("message = %q", m.message)
	}
}

func TestReloadDropsReviewed(t *testing.T) {
	f := newFake()
	m := loadedModel(t, f)
	run(t, m, press("r"))
	run(t, m, press("R"))
	if got := payees(m); got != "Employer,Corner shop" {
		t.Fatalf("rows = %s", got)
	}
}

func TestViewRenders(t *testing.T) {
	for _, width := range []int{120, 60} {
		m := loadedModel(t, newFake())
		run(t, m, tea.WindowSizeMsg{Width: width, Height: 20})
		run(t, m, press("r")) // reviews Bus and selects Employer
		run(t, m, press("j")) // Corner shop
		out := ansi.Strip(m.View().Content)
		lines := strings.Split(out, "\n")
		if len(lines) != 20 {
			t.Errorf("width %d: %d lines", width, len(lines))
		}
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > width {
				t.Errorf("width %d: line %d wide: %q", width, w, l)
			}
		}
		for _, want := range []string{"2 to review", "1 reviewed", "✓", "+£2,100.00", "Employer", "Corner shop", "Current account"} {
			if !strings.Contains(out, want) {
				t.Errorf("width %d: view doesn't contain %q:\n%s", width, want, out)
			}
		}
	}
}

func TestFormatAmount(t *testing.T) {
	for _, tt := range []struct {
		amount, currency, want string
		credit                 bool
	}{
		{"4.5000", "gbp", "£4.50", false},
		{"-2100.0000", "gbp", "£2,100.00", true},
		{"1234567.891", "usd", "$1,234,567.89", false},
		{"12.00", "sek", "12.00 SEK", false},
		{"0.0000", "eur", "€0.00", false},
		{"junk", "gbp", "junk", false},
	} {
		got, credit := formatAmount(tt.amount, tt.currency)
		if got != tt.want || credit != tt.credit {
			t.Errorf("formatAmount(%q, %q) = %q, %v; want %q, %v", tt.amount, tt.currency, got, credit, tt.want, tt.credit)
		}
	}
}

// Unselected rows show the date dim, money out in magenta and money in in
// cyan.
func TestListColours(t *testing.T) {
	m := loadedModel(t, newFake())
	lines := m.listLines(100, 10) // Bus (selected), Employer, Corner shop
	if !strings.Contains(lines[1], styleDim.Render("2026-11-11")) {
		t.Errorf("date not dim: %q", lines[1])
	}
	// Amounts are right-aligned, so they start with spaces.
	if !strings.HasSuffix(lines[1], styleIncome.Render("+£2,100.00")) {
		t.Errorf("money in not cyan: %q", lines[1])
	}
	if !strings.HasSuffix(lines[2], styleSpend.Render("     £4.50")) {
		t.Errorf("money out not magenta: %q", lines[2])
	}
}
