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

func TestAccountsToRows(t *testing.T) {
	code := "1000"
	parent := int64(2)
	rows := accountsToRows([]client.Account{
		{ID: 1, Name: "Cash", Code: &code, ParentID: &parent, Balance: -500},
		{ID: 2, Name: "Assets", Balance: 1000},
	})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// Assets (the root) comes first, followed immediately by its child
	// Cash, indented. The balance column replaces the parent column, and
	// its value is right-aligned within moneyColumnWidth.
	if got, want := rows[0], (table.Row{"2", "", "Assets", rightAlign("10.00", moneyColumnWidth)}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 0 = %v, want %v", got, want)
	}
	if got, want := rows[1], (table.Row{"1", "1000", "  Cash", rightAlign("-5.00", moneyColumnWidth)}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 1 = %v, want %v", got, want)
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
	// Root accounts and a set of children both given out of ID order.
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

	msg := m.Init()()
	loaded, ok := msg.(accountsLoadedMsg)
	if !ok {
		t.Fatalf("Init(): got %T, want accountsLoadedMsg", msg)
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

	msg := m.Init()()
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
	if m.createStep != stepPickParent {
		t.Fatalf("createStep = %v, want stepPickParent (parent is asked first)", m.createStep)
	}
	if got, want := m.parentPicker.Rows(), (parentPickerRows(m.rows)); !reflect.DeepEqual(got, want) {
		t.Fatalf("parentPicker rows = %v, want %v", got, want)
	}
	if m.parentPicker.Cursor() != 0 {
		t.Fatalf("parentPicker cursor = %d, want 0 (\"(none)\")", m.parentPicker.Cursor())
	}
	if !m.Editing() {
		t.Fatal("Editing() should be true in create mode")
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

func TestLedgerToRows(t *testing.T) {
	ts := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	rows := ledgerToRows([]client.LedgerEntry{
		{Description: "Invoice #1", Value: 1000, Balance: 1000, Timestamp: ts},
		{Description: "Invoice #2", Value: -300, Balance: 700, Timestamp: ts},
	})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if got, want := rows[0], (table.Row{
		ts.Local().Format(timestampLayout), "Invoice #1",
		rightAlign("10.00", moneyColumnWidth), rightAlign("10.00", moneyColumnWidth),
	}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 0 = %v, want %v", got, want)
	}
	if got, want := rows[1], (table.Row{
		ts.Local().Format(timestampLayout), "Invoice #2",
		rightAlign("-3.00", moneyColumnWidth), rightAlign("7.00", moneyColumnWidth),
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
			{Description: "Invoice #1", Value: 1000, Balance: 1000},
		})
	})
	m.rows = []client.Account{{ID: 5, Name: "Cash"}}
	m.table.SetRows(accountsToRows(m.rows))

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
	m.table.SetRows(accountsToRows(m.rows))

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
	m, _ = m.Update(keyPress("enter"))
	m, _ = m.Update(keyPress("enter"))
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
}

func TestAccountsModel_CreateWithParentSelection(t *testing.T) {
	var gotAccount client.Account
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotAccount)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Account{ID: 3, Name: gotAccount.Name})
	})
	m.rows = []client.Account{{ID: 1, Name: "Assets"}, {ID: 2, Name: "Liabilities"}}
	m, _ = m.Update(keyPress("n"))

	// Row 0 is "(none)"; move down twice to land on account ID 2.
	m, _ = m.Update(keyPress("down"))
	m, _ = m.Update(keyPress("down"))
	m, _ = m.Update(keyPress("enter"))

	if m.createStep != stepAccountName {
		t.Fatalf("createStep = %v, want stepAccountName", m.createStep)
	}
	if m.pendingParentID == nil || *m.pendingParentID != 2 {
		t.Fatalf("pendingParentID = %v, want 2", m.pendingParentID)
	}

	m = typeString(m, "Cash")
	m, cmd := m.Update(keyPress("enter")) // -> code
	if m.createStep != stepAccountCode {
		t.Fatalf("createStep = %v, want stepAccountCode", m.createStep)
	}
	m = typeString(m, "1000")
	m, cmd = m.Update(keyPress("enter")) // submit
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
	m, _ = m.Update(keyPress("n"))
	m, _ = m.Update(keyPress("enter")) // confirm "(none)" as parent
	m = typeString(m, "Cash")
	m, _ = m.Update(keyPress("enter")) // -> code
	m, cmd := m.Update(keyPress("enter"))
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
	m.table.SetRows(accountsToRows(m.rows))

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
