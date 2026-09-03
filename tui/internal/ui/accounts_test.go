package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"money/tui/internal/client"
)

func newTestAccountsModel(t *testing.T, handler http.HandlerFunc) accountsModel {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newAccountsModel(client.New(srv.URL))
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

func typeString(m accountsModel, s string) accountsModel {
	for _, r := range s {
		m, _ = m.Update(keyPress(string(r)))
	}
	return m
}

func TestPadOrTruncate(t *testing.T) {
	if got := padOrTruncate("Cash", 10); got != "Cash      " {
		t.Errorf("padOrTruncate(%q, 10) = %q", "Cash", got)
	}
	if got := padOrTruncate("", 5); got != "     " {
		t.Errorf("padOrTruncate(\"\", 5) = %q", got)
	}
	if got, want := padOrTruncate("A very long account name", 10), "A very lon"; got != want {
		t.Errorf("padOrTruncate of an over-width string = %q, want %q (truncated)", got, want)
	}
	if got := len(padOrTruncate("x", 8)); got != 8 {
		t.Errorf("padOrTruncate result length = %d, want 8", got)
	}
}

func TestRightAlign(t *testing.T) {
	if got := rightAlign("10.00", 12); got != strings.Repeat(" ", 7)+"10.00" {
		t.Errorf("rightAlign(%q, 12) = %q", "10.00", got)
	}
	if got := rightAlign("", 12); got != strings.Repeat(" ", 12) {
		t.Errorf("rightAlign(\"\", 12) = %q", got)
	}
	if len(rightAlign("10.00", 12)) != 12 {
		t.Errorf("rightAlign result length = %d, want 12", len(rightAlign("10.00", 12)))
	}
	// Content longer than width is left as is, not truncated (the table's
	// own rendering already handles overflow safely).
	if got := rightAlign("12345678901234", 12); got != "12345678901234" {
		t.Errorf("rightAlign of an over-width string should be unchanged, got %q", got)
	}
}

var testUSD = client.Currency{ID: 9, Name: "USD", SymbolBefore: true, DecimalSeparator: ".", DecimalPlaces: 2}
var testCurrencies = currencyByID{testUSD.ID: testUSD}

func TestAccountsToRows(t *testing.T) {
	code := "1000"
	parent := int64(2)
	rows := accountsToRows([]client.Account{
		{ID: 1, Name: "Cash", Code: &code, ParentID: &parent, Balances: []client.CurrencyAmount{{CurrencyID: testUSD.ID, Amount: -500}}},
		{ID: 2, Name: "Assets", Balances: []client.CurrencyAmount{{CurrencyID: testUSD.ID, Amount: 1000}}},
	}, testCurrencies)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// Assets (the root) comes first, followed immediately by its child
	// Cash, indented. The balance column replaces the parent column, and
	// shows each balance formatted per its own currency.
	if got, want := rows[0], (table.Row{"2", "", "Assets", "USD10.00"}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 0 = %v, want %v", got, want)
	}
	if got, want := rows[1], (table.Row{"1", "1000", "  Cash", "USD-5.00"}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 1 = %v, want %v", got, want)
	}
}

func TestFormatBalances(t *testing.T) {
	if got := formatBalances(nil, testCurrencies); got != "" {
		t.Errorf("formatBalances(nil) = %q, want empty", got)
	}
	eur := client.Currency{ID: 7, Name: "EUR", SymbolBefore: false, SymbolSpace: true, DecimalSeparator: ",", DecimalPlaces: 2}
	currencies := currencyByID{testUSD.ID: testUSD, eur.ID: eur}
	got := formatBalances([]client.CurrencyAmount{{CurrencyID: testUSD.ID, Amount: 1000}, {CurrencyID: eur.ID, Amount: 500}}, currencies)
	if want := "USD10.00, 5,00 EUR"; got != want {
		t.Errorf("formatBalances = %q, want %q", got, want)
	}
}

func TestOrderAccountsAsTree(t *testing.T) {
	assets := client.Account{ID: 1, Name: "Assets"}
	liabilities := client.Account{ID: 2, Name: "Liabilities"}
	cash := client.Account{ID: 3, Name: "Cash", ParentID: &assets.ID}
	bank := client.Account{ID: 4, Name: "Bank", ParentID: &assets.ID}
	pettyCash := client.Account{ID: 5, Name: "Petty Cash", ParentID: &cash.ID}

	// Deliberately out of order and with siblings interleaved, to prove
	// the tree walk (not input order) determines the result.
	nodes := orderAccountsAsTree([]client.Account{bank, pettyCash, liabilities, cash, assets})

	type want struct {
		name  string
		depth int
	}
	wants := []want{
		{"Assets", 0},
		{"Cash", 1},
		{"Petty Cash", 2},
		{"Bank", 1},
		{"Liabilities", 0},
	}
	if len(nodes) != len(wants) {
		t.Fatalf("got %d nodes, want %d: %+v", len(nodes), len(wants), nodes)
	}
	for i, w := range wants {
		if nodes[i].account.Name != w.name || nodes[i].depth != w.depth {
			t.Errorf("node %d = (%q, depth %d), want (%q, depth %d)",
				i, nodes[i].account.Name, nodes[i].depth, w.name, w.depth)
		}
	}
}

func TestOrderAccountsAsTree_SiblingsSortedByID(t *testing.T) {
	// Root accounts and a set of children both given out of ID order,
	// all sharing the same (zero) Position: ID is the tiebreak.
	root := client.Account{ID: 10, Name: "Assets"}
	c3 := client.Account{ID: 3, Name: "C", ParentID: &root.ID}
	c1 := client.Account{ID: 1, Name: "A", ParentID: &root.ID}
	c2 := client.Account{ID: 2, Name: "B", ParentID: &root.ID}
	otherRoot := client.Account{ID: 5, Name: "Liabilities"}

	nodes := orderAccountsAsTree([]client.Account{c3, otherRoot, root, c1, c2})

	var gotIDs []int64
	for _, n := range nodes {
		gotIDs = append(gotIDs, n.account.ID)
	}
	want := []int64{5, 10, 1, 2, 3} // roots by ID (5, 10), then 10's children by ID
	if len(gotIDs) != len(want) {
		t.Fatalf("got %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("got %v, want %v", gotIDs, want)
		}
	}
}

// TestOrderAccountsAsTree_SiblingsSortedByPosition is a regression test:
// Position, not ID, is the primary sort key among siblings — as it would
// be after a move (see AccountStore.Move) leaves a lower-ID account with
// a higher Position than its sibling.
func TestOrderAccountsAsTree_SiblingsSortedByPosition(t *testing.T) {
	root := client.Account{ID: 10, Name: "Assets"}
	// c1 has the lower ID but the higher Position, as if it had been
	// moved down past c2.
	c1 := client.Account{ID: 1, Name: "A", ParentID: &root.ID, Position: 1}
	c2 := client.Account{ID: 2, Name: "B", ParentID: &root.ID, Position: 0}

	nodes := orderAccountsAsTree([]client.Account{c1, root, c2})

	var gotIDs []int64
	for _, n := range nodes {
		gotIDs = append(gotIDs, n.account.ID)
	}
	want := []int64{10, 2, 1} // root, then children by Position (c2 before c1)
	if len(gotIDs) != len(want) {
		t.Fatalf("got %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("got %v, want %v", gotIDs, want)
		}
	}
}

func TestOrderAccountsAsTree_CycleIsNotDropped(t *testing.T) {
	// a and b each claim the other as parent: neither is a root, so
	// neither is reachable by the normal tree walk. Both must still show
	// up (as a defensive fallback) instead of vanishing from the list.
	aID, bID := int64(1), int64(2)
	a := client.Account{ID: aID, Name: "A", ParentID: &bID}
	b := client.Account{ID: bID, Name: "B", ParentID: &aID}

	nodes := orderAccountsAsTree([]client.Account{a, b})
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (no account should be dropped): %+v", len(nodes), nodes)
	}
}

func TestAccountsModel_LoadAccounts(t *testing.T) {
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.Account{{ID: 1, Name: "Cash"}})
	})

	msg := m.loadAccounts()
	loaded, ok := msg.(accountsLoadedMsg)
	if !ok {
		t.Fatalf("loadAccounts(): got %T, want accountsLoadedMsg", msg)
	}
	if loaded.err != nil {
		t.Fatalf("Init(): unexpected error %v", loaded.err)
	}

	m2, cmd := m.Update(loaded)
	if cmd != nil {
		t.Errorf("Update(accountsLoadedMsg): unexpected cmd")
	}
	if len(m2.rows) != 1 || m2.rows[0].Name != "Cash" {
		t.Fatalf("rows = %+v", m2.rows)
	}
	if m2.table.Rows() == nil || len(m2.table.Rows()) != 1 {
		t.Fatalf("table rows = %+v", m2.table.Rows())
	}
}

func TestAccountsModel_LoadAccountsError(t *testing.T) {
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	msg := m.loadAccounts()
	loaded := msg.(accountsLoadedMsg)
	if loaded.err == nil {
		t.Fatal("expected an error")
	}
	m2, _ := m.Update(loaded)
	if m2.err == "" {
		t.Fatal("expected m.err to be set")
	}
}

func TestAccountsModel_NKeyEntersCreateMode(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m.rows = []client.Account{{ID: 1, Name: "Assets"}}
	m, _ = m.Update(keyPress("n"))

	if m.mode != accountsModeCreate {
		t.Fatalf("mode = %v, want accountsModeCreate", m.mode)
	}
	if m.createFocus != focusParent {
		t.Fatalf("createFocus = %v, want focusParent", m.createFocus)
	}
	if m.parentPicker.Cursor() != 0 {
		t.Fatalf("parentPicker cursor = %d, want 0 (\"(none)\")", m.parentPicker.Cursor())
	}
	if got, want := m.parentPicker.Rows(), parentDropdownRows(m.rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("parentPicker rows = %v, want %v", got, want)
	}
	if !m.Editing() {
		t.Fatal("Editing() should be true in create mode")
	}
}

func TestAccountsModel_CreateFocusCyclesWithTab(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m, _ = m.Update(keyPress("n"))

	order := []createFocus{focusParent, focusName, focusCode, focusParent}
	for i, want := range order[1:] {
		m, _ = m.Update(keyPress("tab"))
		if m.createFocus != want {
			t.Fatalf("after tab #%d: createFocus = %v, want %v", i+1, m.createFocus, want)
		}
	}

	m, _ = m.Update(keyPress("shift+tab"))
	if m.createFocus != focusCode {
		t.Fatalf("after shift+tab: createFocus = %v, want focusCode", m.createFocus)
	}
}

// TestAccountsModel_LeftRightEditWithinFieldNotFocus is a regression
// test: left/right must move the cursor within the focused text field
// (Name/Code), not change which field has focus — only tab/shift+tab do
// that (see TestAccountsModel_CreateFocusCyclesWithTab).
func TestAccountsModel_LeftRightEditWithinFieldNotFocus(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m, _ = m.Update(keyPress("n"))
	m, _ = m.Update(keyPress("tab")) // -> Name
	m = typeString(m, "Cash")

	m, _ = m.Update(keyPress("left"))
	if m.createFocus != focusName {
		t.Fatalf("createFocus after left = %v, want focusName (unchanged)", m.createFocus)
	}
	if got := m.inputs[fieldAccountName].Value(); got != "Cash" {
		t.Fatalf("Name value after left = %q, want unchanged %q", got, "Cash")
	}
	// The cursor moved left within the field: typing now inserts before
	// the last character instead of appending after it.
	m = typeString(m, "!")
	if got := m.inputs[fieldAccountName].Value(); got != "Cas!h" {
		t.Fatalf("Name value after left+type = %q, want %q", got, "Cas!h")
	}

	m, _ = m.Update(keyPress("right"))
	if m.createFocus != focusName {
		t.Fatalf("createFocus after right = %v, want focusName (unchanged)", m.createFocus)
	}
}

func TestAccountsModel_CreateParentFieldUpDown(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m.rows = []client.Account{{ID: 1, Name: "Assets"}, {ID: 2, Name: "Liabilities"}}
	m, _ = m.Update(keyPress("n")) // focus starts on Parent

	m, _ = m.Update(keyPress("down"))
	if got := m.parentPicker.Cursor(); got != 1 {
		t.Fatalf("cursor after down = %d, want 1", got)
	}
	m, _ = m.Update(keyPress("down"))
	if got := m.parentPicker.Cursor(); got != 2 {
		t.Fatalf("cursor after down,down = %d, want 2", got)
	}
	m, _ = m.Update(keyPress("down")) // clamped: no fourth option to move to
	if got := m.parentPicker.Cursor(); got != 2 {
		t.Fatalf("cursor should stay clamped at the last option, got %d", got)
	}
	m, _ = m.Update(keyPress("up"))
	m, _ = m.Update(keyPress("up"))
	m, _ = m.Update(keyPress("up")) // clamped at 0, not negative
	if got := m.parentPicker.Cursor(); got != 0 {
		t.Fatalf("cursor should stay clamped at 0, got %d", got)
	}

	// up/down only affect the Parent field.
	m, _ = m.Update(keyPress("down"))
	m, _ = m.Update(keyPress("tab")) // -> Name
	m, _ = m.Update(keyPress("down"))
	if got := m.parentPicker.Cursor(); got != 1 {
		t.Fatalf("parentPicker cursor changed while Name was focused: %d", got)
	}
}

// TestAccountsModel_CreatePopupShowsAllParentOptions verifies the
// dropdown lists every account at once when they all fit in the window
// (see TestSyncParentPickerHeight for the "doesn't all fit" case).
func TestAccountsModel_CreatePopupShowsAllParentOptions(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m.SetSize(100, 30)
	m.rows = []client.Account{
		{ID: 1, Name: "Assets"}, {ID: 2, Name: "Liabilities"}, {ID: 3, Name: "Equity"},
	}
	m, _ = m.Update(keyPress("n")) // focus starts on Parent

	popup := m.createPopup()
	for _, want := range []string{"(none)", "Assets", "Liabilities", "Equity"} {
		if !strings.Contains(popup, want) {
			t.Errorf("popup should list %q among the parent options, got:\n%s", want, popup)
		}
	}
}

func TestSyncParentPickerHeight(t *testing.T) {
	m := newTestAccountsModel(t, nil)

	// Plenty of room: every option (+1 for "(none)") fits. table.Model
	// reserves one row of Height() for its own header, so the data-row
	// viewport is len(m.rows)+1 (the "+1" being "(none)").
	m.rows = make([]client.Account, 5)
	m.SetSize(100, 30)
	roomyHeight := m.parentPicker.Height()
	if want := len(m.rows) + 1; roomyHeight != want {
		t.Errorf("height with room to spare = %d, want %d (all options)", roomyHeight, want)
	}

	// A short window: capped to whatever fits, never so small the
	// dropdown becomes useless, and clearly smaller than when there was
	// room for everything.
	m.SetSize(100, 11)
	if got := m.parentPicker.Height(); got < 2 || got >= roomyHeight {
		t.Errorf("height in a short window = %d, want it capped below %d but still usable", got, roomyHeight)
	}
}

func TestAccountsModel_CreatePopup(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m.SetSize(100, 20) // the table needs a known width to render row content
	m.rows = []client.Account{{ID: 1, Name: "Assets"}, {ID: 2, Name: "Liabilities"}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))
	m, _ = m.Update(keyPress("n"))

	popup := m.createPopup()
	for _, want := range []string{"New account", "Parent", "Name", "Code", "(none)", "#2  Liabilities"} {
		if !strings.Contains(popup, want) {
			t.Errorf("popup should contain %q, got:\n%s", want, popup)
		}
	}
	if strings.Contains(popup, "Parent account") {
		t.Errorf("popup should not repeat the Parent label as the dropdown's own column header, got:\n%s", popup)
	}

	// Pick Liabilities as the parent, then move on to Name.
	m, _ = m.Update(keyPress("down"))
	m, _ = m.Update(keyPress("down"))
	m, _ = m.Update(keyPress("tab")) // -> Name
	m = typeString(m, "Cash")

	popup = m.createPopup()
	if !strings.Contains(popup, "Cash") {
		t.Errorf("popup should contain %q, got:\n%s", "Cash", popup)
	}
	// The dropdown itself (and its other option, Assets) is hidden once
	// focus leaves Parent, but the chosen parent must still show as
	// plain text — otherwise the selection becomes invisible while
	// filling in the rest of the form.
	if strings.Contains(popup, "Assets") {
		t.Errorf("the parent dropdown should be hidden once focus leaves Parent, got:\n%s", popup)
	}
	if !strings.Contains(popup, "#2  Liabilities") {
		t.Errorf("the chosen parent should still show as text once focus leaves Parent, got:\n%s", popup)
	}

	// The list view underneath must stay the accounts table, not the
	// form, since the form is a separate pop-up composited on top of it
	// (see overlayCentered).
	if got := m.View(); !strings.Contains(got, "Assets") {
		t.Errorf("View() should still render the accounts list under the pop-up, got:\n%s", got)
	}

	// Validation errors show inside the pop-up, not below the list.
	m.inputs[fieldAccountName].SetValue("")
	m, _ = m.Update(keyPress("enter")) // submit with an empty name
	if m.err != "name is required" {
		t.Fatalf("err = %q, want %q", m.err, "name is required")
	}
	if !strings.Contains(m.createPopup(), "name is required") {
		t.Error("popup should include the validation error")
	}
	if strings.Contains(m.View(), "name is required") {
		t.Error("the error should not also be duplicated below the background list")
	}
}

// TestAccountsModel_CreatePopup_DropdownDirectlyBelowFields is a
// regression test: the parent dropdown's own column header rendered as a
// blank line (see newAccountsModel's parentPicker), leaving an extra
// blank line between the field row and the dropdown's first option
// instead of the dropdown appearing directly beneath it.
func TestAccountsModel_CreatePopup_DropdownDirectlyBelowFields(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m.SetSize(100, 20)
	m.rows = []client.Account{{ID: 1, Name: "Assets"}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))
	m, _ = m.Update(keyPress("n"))

	lines := strings.Split(m.createPopup(), "\n")
	fieldsLine := -1
	for i, l := range lines {
		if strings.Contains(l, "Parent") && strings.Contains(l, "Name") && strings.Contains(l, "Code") {
			fieldsLine = i
			break
		}
	}
	if fieldsLine < 0 || fieldsLine+2 >= len(lines) {
		t.Fatalf("couldn't locate the header/value/dropdown lines in:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[fieldsLine+2], "(none)") {
		t.Errorf("expected the dropdown's first row directly below the field row, got %q instead:\n%s",
			lines[fieldsLine+2], strings.Join(lines, "\n"))
	}
}

func TestOverlayCentered(t *testing.T) {
	bg := "background content\nline two"
	if got := overlayCentered(bg, "popup", 0, 0); got != bg {
		t.Errorf("with no known size, overlayCentered should return background unchanged, got %q", got)
	}

	got := overlayCentered(bg, "POPUP", 40, 10)
	if !strings.Contains(got, "POPUP") {
		t.Errorf("composited output should contain the popup content, got:\n%s", got)
	}
	if !strings.Contains(got, "background content") {
		t.Errorf("composited output should still contain the background, got:\n%s", got)
	}
}

func TestSelectedParentID(t *testing.T) {
	accounts := []client.Account{{ID: 10, Name: "Assets"}, {ID: 20, Name: "Liabilities"}}

	if got := selectedParentID(0, accounts); got != nil {
		t.Errorf("cursor 0 (\"(none)\") = %v, want nil", got)
	}
	if got := selectedParentID(1, accounts); got == nil || *got != 10 {
		t.Errorf("cursor 1 = %v, want 10", got)
	}
	if got := selectedParentID(2, accounts); got == nil || *got != 20 {
		t.Errorf("cursor 2 = %v, want 20", got)
	}
	if got := selectedParentID(3, accounts); got != nil {
		t.Errorf("out-of-range cursor = %v, want nil", got)
	}
}

func TestAccountsModel_DKeyRequiresRows(t *testing.T) {
	m := newTestAccountsModel(t, nil)

	m, _ = m.Update(keyPress("d"))
	if m.mode != accountsModeList {
		t.Fatalf("mode with no rows = %v, want accountsModeList (d should be a no-op)", m.mode)
	}

	m.rows = []client.Account{{ID: 1, Name: "Cash"}}
	m, _ = m.Update(keyPress("d"))
	if m.mode != accountsModeConfirmDelete {
		t.Fatalf("mode with rows = %v, want accountsModeConfirmDelete", m.mode)
	}
}

func TestAccountsModel_EKeyRequiresRows(t *testing.T) {
	m := newTestAccountsModel(t, nil)

	m, _ = m.Update(keyPress("e"))
	if m.mode != accountsModeList {
		t.Fatalf("mode with no rows = %v, want accountsModeList (e should be a no-op)", m.mode)
	}

	m.rows = []client.Account{{ID: 1, Name: "Cash"}}
	m, _ = m.Update(keyPress("e"))
	if m.mode != accountsModeCreate {
		t.Fatalf("mode with rows = %v, want accountsModeCreate", m.mode)
	}
}

// TestAccountsModel_EKeyPrefillsForm checks that "e" opens the same
// pop-up as "n", pre-filled with the selected account's current values —
// including its parent, preselected in the dropdown — and that the
// account itself is left out of its own Parent options (selecting it
// would form a single-node cycle).
func TestAccountsModel_EKeyPrefillsForm(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m.SetSize(100, 30)
	assetsID := int64(1)
	code := "1000"
	m.rows = []client.Account{
		{ID: 1, Name: "Assets"},
		{ID: 2, Name: "Cash", Code: &code, ParentID: &assetsID},
	}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))

	m, _ = m.Update(keyPress("down")) // cursor -> row 1 (Cash)
	m, _ = m.Update(keyPress("e"))

	if m.mode != accountsModeCreate {
		t.Fatalf("mode = %v, want accountsModeCreate", m.mode)
	}
	if m.editingID == nil || *m.editingID != 2 {
		t.Fatalf("editingID = %v, want 2", m.editingID)
	}
	if got := m.inputs[fieldAccountName].Value(); got != "Cash" {
		t.Errorf("Name = %q, want %q", got, "Cash")
	}
	if got := m.inputs[fieldAccountCode].Value(); got != "1000" {
		t.Errorf("Code = %q, want %q", got, "1000")
	}
	// Assets is Cash's parent and comes first in parentOptions (Cash
	// itself is excluded), so cursor 1 is Assets.
	if got := m.parentPicker.Cursor(); got != 1 {
		t.Fatalf("parentPicker cursor = %d, want 1 (Assets)", got)
	}

	popup := m.createPopup()
	if !strings.Contains(popup, "Edit account") {
		t.Errorf("popup should be titled %q, got:\n%s", "Edit account", popup)
	}
	if strings.Contains(popup, "#2  Cash") {
		t.Errorf("popup's Parent dropdown should not offer the account being edited as its own parent, got:\n%s", popup)
	}
	if !strings.Contains(popup, "#1  Assets") {
		t.Errorf("popup's Parent dropdown should still offer other accounts, got:\n%s", popup)
	}
}

func TestAccountsModel_EditSubmitsUpdate(t *testing.T) {
	var gotMethod, gotPath string
	var gotAccount client.Account
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotAccount)
		json.NewEncoder(w).Encode(client.Account{ID: 2, Name: gotAccount.Name})
	})
	m.SetSize(100, 30)
	m.rows = []client.Account{{ID: 1, Name: "Assets"}, {ID: 2, Name: "Cash"}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))

	m, _ = m.Update(keyPress("down"))
	m, _ = m.Update(keyPress("e"))
	m, _ = m.Update(keyPress("tab")) // -> Name
	for range "Cash" {
		m, _ = m.Update(keyPress("backspace"))
	}
	m = typeString(m, "Petty Cash")

	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected a command to submit the update request")
	}
	msg := cmd()
	mutated, ok := msg.(accountMutatedMsg)
	if !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if gotMethod != http.MethodPut || gotPath != "/accounts/2" {
		t.Fatalf("got %s %s, want PUT /accounts/2", gotMethod, gotPath)
	}
	if gotAccount.Name != "Petty Cash" {
		t.Fatalf("request body = %+v", gotAccount)
	}

	m, cmd = m.Update(mutated)
	if m.mode != accountsModeList {
		t.Fatalf("mode = %v, want accountsModeList", m.mode)
	}
	if m.editingID != nil {
		t.Fatalf("editingID = %v, want nil after a successful edit", m.editingID)
	}
	if cmd == nil {
		t.Fatal("expected a reload command after a successful edit")
	}
}

func TestAccountsModel_MoveUpDown(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			gotMethod, gotPath = r.Method, r.URL.Path
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// The reload after a successful move: Liabilities (ID 2) now has
		// the lower Position, matching what a real move would produce.
		json.NewEncoder(w).Encode([]client.Account{
			{ID: 1, Name: "Assets", Position: 1},
			{ID: 2, Name: "Liabilities", Position: 0},
		})
	})
	m.rows = []client.Account{{ID: 1, Name: "Assets", Position: 0}, {ID: 2, Name: "Liabilities", Position: 1}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))

	m, _ = m.Update(keyPress("down")) // cursor -> row 1 (Liabilities)
	m, cmd := m.Update(keyPress("K"))
	if cmd == nil {
		t.Fatal("expected a command to submit the move request")
	}
	msg := cmd()
	mutated, ok := msg.(accountMutatedMsg)
	if !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if gotMethod != http.MethodPost || gotPath != "/accounts/2/move" {
		t.Fatalf("got %s %s, want POST /accounts/2/move", gotMethod, gotPath)
	}
	if gotBody["direction"] != "up" {
		t.Fatalf("request body = %v", gotBody)
	}

	m, cmd = m.Update(mutated)
	if cmd == nil {
		t.Fatal("expected a reload command after a successful move")
	}
	loaded := cmd().(accountsLoadedMsg)
	m, _ = m.Update(loaded)

	// The moved account (Liabilities) should now be first, and the
	// cursor should have followed it there rather than staying at row 1.
	if m.rows[0].ID != 2 {
		t.Fatalf("m.rows[0] = %+v, want Liabilities (ID 2) first after the move", m.rows[0])
	}
	if got := m.table.Cursor(); got != 0 {
		t.Fatalf("cursor after move = %d, want 0 (following Liabilities)", got)
	}
}

func TestAccountsModel_MoveDown(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	})
	m.rows = []client.Account{{ID: 1, Name: "Assets"}, {ID: 2, Name: "Liabilities"}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))

	m, cmd := m.Update(keyPress("J")) // cursor starts on row 0 (Assets)
	if cmd == nil {
		t.Fatal("expected a command to submit the move request")
	}
	cmd()
	if gotPath != "/accounts/1/move" || gotBody["direction"] != "down" {
		t.Fatalf("got path=%q body=%v, want /accounts/1/move with direction=down", gotPath, gotBody)
	}
}

func TestAccountsModel_MoveNoRowsIsNoop(t *testing.T) {
	m := newTestAccountsModel(t, nil)

	m, cmd := m.Update(keyPress("K"))
	if cmd != nil {
		t.Fatal("expected no command with no rows selected")
	}
	m, cmd = m.Update(keyPress("J"))
	if cmd != nil {
		t.Fatal("expected no command with no rows selected")
	}
}

func TestLedgerToRows(t *testing.T) {
	ts := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	rows := ledgerToRows([]client.LedgerEntry{
		{Description: "Invoice #1", CurrencyID: testUSD.ID, Amount: 1000, Balance: 1000, Timestamp: ts},
		{Description: "Invoice #2", CurrencyID: testUSD.ID, Amount: -300, Balance: 700, Timestamp: ts},
	}, testCurrencies)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if got, want := rows[0], (table.Row{
		ts.Local().Format(timestampLayout), "Invoice #1",
		rightAlign("USD10.00", moneyColumnWidth), rightAlign("USD10.00", moneyColumnWidth),
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 0 = %v, want %v", got, want)
	}
	if got, want := rows[1], (table.Row{
		ts.Local().Format(timestampLayout), "Invoice #2",
		rightAlign("USD-3.00", moneyColumnWidth), rightAlign("USD7.00", moneyColumnWidth),
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 1 = %v, want %v", got, want)
	}
}

func TestAccountsModel_EnterRequiresRows(t *testing.T) {
	m := newTestAccountsModel(t, nil)

	m, _ = m.Update(keyPress("enter"))
	if m.mode != accountsModeList {
		t.Fatalf("mode with no rows = %v, want accountsModeList (enter should be a no-op)", m.mode)
	}
}

func TestAccountsModel_EnterOpensLedger(t *testing.T) {
	var gotPath string
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]client.LedgerEntry{
			{Description: "Invoice #1", CurrencyID: testUSD.ID, Amount: 1000, Balance: 1000},
		})
	})
	m.rows = []client.Account{{ID: 5, Name: "Cash"}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))

	m, cmd := m.Update(keyPress("enter"))
	if m.mode != accountsModeLedger {
		t.Fatalf("mode = %v, want accountsModeLedger", m.mode)
	}
	if m.ledgerAccount.ID != 5 {
		t.Fatalf("ledgerAccount = %+v, want ID 5", m.ledgerAccount)
	}
	if !m.Editing() {
		t.Fatal("Editing() should be true while viewing the ledger")
	}
	if cmd == nil {
		t.Fatal("expected a command to load the ledger")
	}

	msg := cmd()
	loaded, ok := msg.(ledgerLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if gotPath != "/accounts/5/transactions" {
		t.Fatalf("requested path = %q, want /accounts/5/transactions", gotPath)
	}

	m, _ = m.Update(loaded)
	if len(m.ledgerTable.Rows()) != 1 {
		t.Fatalf("ledgerTable rows = %+v", m.ledgerTable.Rows())
	}

	// esc returns to the list.
	m, _ = m.Update(keyPress("esc"))
	if m.mode != accountsModeList {
		t.Fatalf("mode after esc = %v, want accountsModeList", m.mode)
	}
}

func TestAccountsModel_QKeyGoesBackFromLedger(t *testing.T) {
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.LedgerEntry{})
	})
	m.rows = []client.Account{{ID: 5, Name: "Cash"}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))

	m, _ = m.Update(keyPress("enter"))
	if m.mode != accountsModeLedger {
		t.Fatalf("mode = %v, want accountsModeLedger", m.mode)
	}

	m, _ = m.Update(keyPress("q"))
	if m.mode != accountsModeList {
		t.Fatalf("mode after q = %v, want accountsModeList (q should go back, not quit)", m.mode)
	}
}

// TestAccountsModel_EnterUsesTreeOrder is a regression test: the table
// displays accounts in tree order (a child right after its parent), which
// can differ from the API's ID order whenever a child's ID doesn't
// immediately follow its parent's — as here, where Juan (ID 4) is Cash's
// child but Revenue (ID 2) has a lower ID. m.rows must be kept in the same
// order as the table, or the cursor position resolves to the wrong
// account (see also TestAccountsModel_DeleteUsesTreeOrder).
func TestAccountsModel_EnterUsesTreeOrder(t *testing.T) {
	var gotPath string
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]client.LedgerEntry{})
	})

	cashID := int64(1)
	accounts := []client.Account{ // raw API order: Cash, Revenue, Juan
		{ID: 1, Name: "Cash"},
		{ID: 2, Name: "Revenue"},
		{ID: 4, Name: "Juan", ParentID: &cashID},
	}
	m, _ = m.Update(accountsLoadedMsg{accounts: accounts})

	// Tree order is Cash, Juan, Revenue: row 1 is Juan, not Revenue.
	if m.rows[1].ID != 4 {
		t.Fatalf("m.rows[1] = %+v, want Juan (ID 4)", m.rows[1])
	}

	m, _ = m.Update(keyPress("down")) // cursor -> row 1 (Juan)
	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected a command to load the ledger")
	}
	cmd()

	if m.ledgerAccount.ID != 4 {
		t.Fatalf("opened ledger for account %+v, want Juan (ID 4)", m.ledgerAccount)
	}
	if gotPath != "/accounts/4/transactions" {
		t.Fatalf("requested path = %q, want /accounts/4/transactions", gotPath)
	}
}

func TestAccountsModel_DeleteUsesTreeOrder(t *testing.T) {
	var deletedID string
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletedID = r.URL.Path
		}
		w.WriteHeader(http.StatusNoContent)
	})

	cashID := int64(1)
	accounts := []client.Account{ // raw API order: Cash, Revenue, Juan
		{ID: 1, Name: "Cash"},
		{ID: 2, Name: "Revenue"},
		{ID: 4, Name: "Juan", ParentID: &cashID},
	}
	m, _ = m.Update(accountsLoadedMsg{accounts: accounts})

	m, _ = m.Update(keyPress("down")) // cursor -> row 1 (Juan, in tree order)
	m, _ = m.Update(keyPress("d"))
	m, cmd := m.Update(keyPress("y"))
	if cmd == nil {
		t.Fatal("expected a delete command")
	}
	cmd()

	if deletedID != "/accounts/4" {
		t.Fatalf("deleted %q, want /accounts/4 (Juan)", deletedID)
	}
}

func TestAccountsModel_CreateValidation(t *testing.T) {
	called := false
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	m, _ = m.Update(keyPress("n"))

	// Submitting with an empty name should not hit the API.
	m, cmd := m.Update(keyPress("enter"))
	if m.err != "name is required" {
		t.Fatalf("err = %q, want %q", m.err, "name is required")
	}
	if cmd != nil {
		cmd() // run it, in case it were the mutate command, to detect the API being hit
	}
	if called {
		t.Fatal("the API should not have been called for an invalid submission")
	}
	if m.mode != accountsModeCreate {
		t.Fatalf("mode = %v, want accountsModeCreate (form should stay open)", m.mode)
	}
	if m.createFocus != focusName {
		t.Fatalf("createFocus = %v, want focusName (so the user can fix it)", m.createFocus)
	}
}

func TestAccountsModel_CreateWithParentSelection(t *testing.T) {
	var gotAccount client.Account
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotAccount)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Account{ID: 3, Name: gotAccount.Name})
	})
	m.rows = []client.Account{{ID: 1, Name: "Assets"}, {ID: 2, Name: "Liabilities"}}
	m, _ = m.Update(keyPress("n")) // focus starts on Parent

	// Choice 0 is "(none)"; move down twice to land on account ID 2.
	m, _ = m.Update(keyPress("down"))
	m, _ = m.Update(keyPress("down"))
	if got := m.parentPicker.Cursor(); got != 2 {
		t.Fatalf("parentPicker cursor = %d, want 2", got)
	}

	m, _ = m.Update(keyPress("tab")) // -> Name
	m = typeString(m, "Cash")
	m, _ = m.Update(keyPress("tab")) // -> Code
	m = typeString(m, "1000")

	m, cmd := m.Update(keyPress("enter")) // submit, from any field
	if cmd == nil {
		t.Fatal("expected a command to submit the create request")
	}

	msg := cmd()
	mutated, ok := msg.(accountMutatedMsg)
	if !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if gotAccount.Name != "Cash" || gotAccount.ParentID == nil || *gotAccount.ParentID != 2 {
		t.Fatalf("API received %+v, want name=Cash parent=2", gotAccount)
	}
	if gotAccount.Code == nil || *gotAccount.Code != "1000" {
		t.Fatalf("API received code %v, want \"1000\"", gotAccount.Code)
	}
}

func TestAccountsModel_CreateDefaultsToNoParent(t *testing.T) {
	var gotAccount client.Account
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotAccount)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Account{ID: 1, Name: gotAccount.Name})
	})
	m.rows = []client.Account{{ID: 1, Name: "Assets"}}
	m, _ = m.Update(keyPress("n")) // focus starts on Parent, defaulting to "(none)"
	m, _ = m.Update(keyPress("tab"))
	m = typeString(m, "Cash")
	m, cmd := m.Update(keyPress("enter")) // submit, without ever touching Code
	if cmd == nil {
		t.Fatal("expected a command to submit the create request")
	}

	msg := cmd()
	if mutated, ok := msg.(accountMutatedMsg); !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if gotAccount.ParentID != nil {
		t.Fatalf("ParentID = %v, want nil", gotAccount.ParentID)
	}

	m, cmd = m.Update(accountMutatedMsg{})
	if m.mode != accountsModeList {
		t.Fatalf("mode = %v, want accountsModeList", m.mode)
	}
	if cmd == nil {
		t.Fatal("expected a reload command after a successful create")
	}
}

func TestAccountsModel_EscCancelsCreate(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m, _ = m.Update(keyPress("n"))
	m, _ = m.Update(keyPress("esc"))

	if m.mode != accountsModeList {
		t.Fatalf("mode = %v, want accountsModeList", m.mode)
	}
	if m.Editing() {
		t.Fatal("Editing() should be false back in list mode")
	}
}

func TestAccountsModel_ConfirmDeleteYesAndNo(t *testing.T) {
	deleted := false
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
		}
		w.WriteHeader(http.StatusNoContent)
	})
	m.rows = []client.Account{{ID: 5, Name: "Cash"}}
	m.table.SetRows(accountsToRows(m.rows, m.currencies))

	// A key other than "y" cancels.
	m, _ = m.Update(keyPress("d"))
	m, _ = m.Update(keyPress("n"))
	if m.mode != accountsModeList {
		t.Fatalf("mode after cancel = %v, want accountsModeList", m.mode)
	}
	if deleted {
		t.Fatal("delete should not have happened on cancel")
	}

	m, _ = m.Update(keyPress("d"))
	m, cmd := m.Update(keyPress("y"))
	if cmd == nil {
		t.Fatal("expected a delete command")
	}
	msg := cmd()
	if mutated, ok := msg.(accountMutatedMsg); !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if !deleted {
		t.Fatal("expected the API to have been called with DELETE")
	}
}

func TestAccountsModel_SetSize(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m.SetSize(80, 20)
	if got := m.table.Width(); got != 80 {
		t.Errorf("table width = %d, want 80", got)
	}
	got20 := m.table.Height()

	// The ledger view has its own two-line header above the table (unlike
	// the plain list), so it needs two fewer rows to keep the footer
	// visible on screen.
	if got := m.ledgerTable.Height(); got != got20-2 {
		t.Errorf("ledgerTable height = %d, want %d (table height - 2)", got, got20-2)
	}

	// Too small a height must not be applied, to keep the table usable.
	m.SetSize(80, 3)
	if got := m.table.Height(); got != got20 {
		t.Errorf("table height after a too-small SetSize = %d, want unchanged %d", got, got20)
	}
}
