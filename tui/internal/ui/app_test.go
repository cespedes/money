package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"money/tui/internal/client"
)

func newTestApp(t *testing.T) App {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)
	return New(client.New(srv.URL))
}

func TestApp_WindowSizeResizesActiveTables(t *testing.T) {
	m := newTestApp(t)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd != nil {
		t.Error("WindowSizeMsg should not produce a command")
	}
	app := updated.(App)
	if app.width != 100 || app.height != 30 {
		t.Fatalf("app size = (%d, %d), want (100, 30)", app.width, app.height)
	}
	if got := app.accounts.table.Width(); got != 100 {
		t.Errorf("accounts table width = %d, want 100", got)
	}
	if got := app.transactions.tbl.Width(); got != 100 {
		t.Errorf("transactions table width = %d, want 100", got)
	}
}

func TestApp_TabSwitchesActiveView(t *testing.T) {
	m := newTestApp(t)
	if m.active != tabAccounts {
		t.Fatalf("initial active tab = %v, want tabAccounts", m.active)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(App)
	if m.active != tabTransactions {
		t.Fatalf("active after tab = %v, want tabTransactions", m.active)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(App)
	if m.active != tabAccounts {
		t.Fatalf("active after shift+tab = %v, want tabAccounts", m.active)
	}
}

func TestApp_TabCyclesThroughAllTabs(t *testing.T) {
	m := newTestApp(t)

	order := []tab{tabTransactions, tabCurrencies, tabAccounts}
	for i, want := range order {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = updated.(App)
		if m.active != want {
			t.Fatalf("after tab #%d: active = %v, want %v", i+1, m.active, want)
		}
	}

	// shift+tab cycles the other way.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(App)
	if m.active != tabCurrencies {
		t.Fatalf("active after shift+tab = %v, want tabCurrencies", m.active)
	}
}

func TestApp_TabIsSwallowedWhileEditing(t *testing.T) {
	m := newTestApp(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}) // enter create mode on Accounts
	m = updated.(App)
	if !m.currentEditing() {
		t.Fatal("expected the accounts tab to be in an editing mode")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(App)
	if m.active != tabAccounts {
		t.Fatalf("active = %v, want tabAccounts (tab should be swallowed by the form)", m.active)
	}
}

// isQuit reports whether cmd, when run, requests the program to quit. A
// plain nil check isn't enough here: routing a key through to a focused
// textinput (e.g. typing "q" into a form field) can itself return a
// non-nil cmd (for cursor blinking) that has nothing to do with quitting.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestApp_QuitKeys(t *testing.T) {
	m := newTestApp(t)

	// "q" quits when the active tab isn't editing.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !isQuit(cmd) {
		t.Fatal("expected \"q\" to return tea.Quit outside of an editing mode")
	}

	// "q" does not quit while editing (e.g. it's part of a typed value).
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}) // enter create mode, focus on Parent
	m = updated.(App)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Name
	m = updated.(App)
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if isQuit(cmd) {
		t.Fatal("\"q\" should not quit while a form is open")
	}
	if m.accounts.inputs[fieldAccountName].Value() != "q" {
		t.Fatalf("expected \"q\" to have been typed into the name field, got %q", m.accounts.inputs[fieldAccountName].Value())
	}

	// ctrl+c always quits.
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !isQuit(cmd) {
		t.Fatal("expected ctrl+c to return tea.Quit even while editing")
	}
}

func TestApp_LedgerQKeyGoesBackInsteadOfQuitting(t *testing.T) {
	m := newTestApp(t)
	m.accounts.rows = []client.Account{{ID: 5, Name: "Cash"}}
	m.accounts.table.SetRows(accountsToRows(m.accounts.rows, nil))

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(App)
	if m.accounts.mode != accountsModeLedger {
		t.Fatalf("mode = %v, want accountsModeLedger", m.accounts.mode)
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if isQuit(cmd) {
		t.Fatal("\"q\" should not quit the app while viewing an account's ledger")
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = updated.(App)
	if m.accounts.mode != accountsModeList {
		t.Fatalf("mode after q = %v, want accountsModeList (q should go back)", m.accounts.mode)
	}
}

func TestApp_LedgerFooter(t *testing.T) {
	m := newTestApp(t)
	m.accounts.rows = []client.Account{{ID: 5, Name: "Cash"}}
	m.accounts.table.SetRows(accountsToRows(m.accounts.rows, nil))

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(App)

	footer := m.footer()
	if !strings.Contains(footer, "esc/q: back") {
		t.Errorf("footer = %q, want it to mention esc/q: back", footer)
	}
	if strings.Contains(footer, "confirm field") {
		t.Errorf("footer = %q, should not show the generic form-editing hint while viewing the ledger", footer)
	}
}

func TestApp_KeysOnlyReachActiveTab(t *testing.T) {
	m := newTestApp(t)
	m.transactions.currencies = []client.Currency{testUSD}    // "n" refuses with none loaded
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // switch to Transactions
	m = updated.(App)

	// "n" should open the create form on the Transactions tab, and leave
	// Accounts untouched.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(App)
	if m.accounts.mode != accountsModeList {
		t.Fatalf("accounts.mode = %v, want accountsModeList (key should not have reached it)", m.accounts.mode)
	}
	if m.transactions.mode != transactionsModeCreate {
		t.Fatalf("transactions.mode = %v, want transactionsModeCreate", m.transactions.mode)
	}
}

// TestApp_PasteOnlyReachesActiveTab mirrors TestApp_KeysOnlyReachActiveTab
// for tea.PasteMsg: it must be routed the same way tea.KeyMsg is, or a
// paste meant for the active tab's focused field could instead land in a
// field on a tab that isn't even visible.
func TestApp_PasteOnlyReachesActiveTab(t *testing.T) {
	m := newTestApp(t)
	// Simulate the Accounts tab having a focused field left over from
	// before the user switched away from it.
	m.accounts.mode = accountsModeCreate
	m.accounts.setCreateFocus(focusName)
	m.active = tabTransactions
	m.transactions.mode = transactionsModeCreate
	m.transactions.descInput.Focus()

	updated, _ := m.Update(tea.PasteMsg{Content: "pasted text"})
	m = updated.(App)

	if got := m.accounts.inputs[fieldAccountName].Value(); got != "" {
		t.Errorf("Accounts Name field = %q, want empty (paste should not reach the inactive tab)", got)
	}
	if got := m.transactions.descInput.Value(); got != "pasted text" {
		t.Errorf("Transactions descInput = %q, want %q (paste should reach the active tab)", got, "pasted text")
	}
}

func TestApp_NonKeyMessagesReachBothTabs(t *testing.T) {
	m := newTestApp(t)
	msg := accountsLoadedMsg{accounts: nil}
	updated, _ := m.Update(msg)
	_ = updated.(App)

	// Sanity: a transactions-specific load message doesn't panic or get
	// dropped when routed to both models (the accounts model must ignore
	// message types it doesn't recognize).
	tmsg := transactionsLoadedMsg{transactions: nil}
	updated, _ = m.Update(tmsg)
	_ = updated.(App)
}

func TestApp_View(t *testing.T) {
	m := newTestApp(t)
	v := m.View()
	if !v.AltScreen {
		t.Error("expected AltScreen to be true")
	}
	if !strings.Contains(v.Content, "Money — Accounts") {
		t.Errorf("view content = %q, want the title to name the active tab (Accounts)", v.Content)
	}
}

// TestApp_TitleTracksActiveTab checks that the title line names whichever
// tab is currently active — the only place the active tab is named at
// all now that there's no separate row of tab labels.
func TestApp_TitleTracksActiveTab(t *testing.T) {
	m := newTestApp(t)

	for _, want := range []string{"Transactions", "Currencies", "Accounts"} {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = updated.(App)

		v := m.View()
		if !strings.Contains(v.Content, "Money — "+want) {
			t.Errorf("active = %v: view content = %q, want the title to say %q", m.active, v.Content, "Money — "+want)
		}
	}
}

// TestApp_TitleShowsBreadcrumbInLedgerView checks that viewing an
// account's ledger replaces the title's usual "Accounts" with that
// account's breadcrumb path (see accountBreadcrumb) — and that the
// ledger view itself no longer repeats it as a separate subtitle line.
func TestApp_TitleShowsBreadcrumbInLedgerView(t *testing.T) {
	m := newTestApp(t)
	assets := client.Account{ID: 1, Name: "Assets"}
	cash := client.Account{ID: 2, Name: "Cash", ParentID: &assets.ID}
	m.accounts.rows = []client.Account{assets, cash}
	m.accounts.table.SetRows(accountsToRows(m.accounts.rows, nil))
	m.accounts.table.SetCursor(1) // Cash

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(App)
	if m.accounts.mode != accountsModeLedger {
		t.Fatalf("accounts.mode = %v, want accountsModeLedger", m.accounts.mode)
	}

	v := m.View()
	want := "Money — Transactions in Assets → Cash"
	if !strings.Contains(v.Content, want) {
		t.Errorf("view content = %q, want the title to say %q", v.Content, want)
	}
	if strings.Contains(v.Content, "Transactions for") {
		t.Errorf("view content = %q, should not repeat a separate subtitle", v.Content)
	}
}

// TestApp_CreatePopupOverlaysBackground checks that opening the
// new-account form composites a pop-up over the accounts list, rather
// than replacing it: both the list and the pop-up must show up in the
// same rendered view.
func TestApp_CreatePopupOverlaysBackground(t *testing.T) {
	m := newTestApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(App)

	m.accounts.rows = []client.Account{{ID: 1, Name: "Assets"}}
	m.accounts.table.SetRows(accountsToRows(m.accounts.rows, nil))

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(App)

	v := m.View()
	if !strings.Contains(v.Content, "New account") {
		t.Errorf("view should contain the pop-up, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Assets") {
		t.Errorf("view should still show the accounts list behind the pop-up, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Money") {
		t.Errorf("view should still show the app title behind the pop-up, got:\n%s", v.Content)
	}
}

// TestApp_CurrenciesPopupOverlaysBackground mirrors
// TestApp_CreatePopupOverlaysBackground for the Currencies tab.
func TestApp_CurrenciesPopupOverlaysBackground(t *testing.T) {
	m := newTestApp(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(App)

	m.currencies.rows = []client.Currency{{ID: 1, Name: "USD"}}
	m.currencies.table.SetRows(currenciesToRows(m.currencies.rows))

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Transactions
	m = updated.(App)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Currencies
	m = updated.(App)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(App)

	v := m.View()
	if !strings.Contains(v.Content, "New currency") {
		t.Errorf("view should contain the pop-up, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "USD") {
		t.Errorf("view should still show the currencies list behind the pop-up, got:\n%s", v.Content)
	}
}

func TestApp_CurrenciesFooter(t *testing.T) {
	m := newTestApp(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Transactions
	m = updated.(App)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // -> Currencies
	m = updated.(App)

	if strings.Contains(m.footer(), "enter:") {
		t.Errorf("footer = %q, currencies list has no enter-drilldown", m.footer())
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(App)
	if !strings.Contains(m.footer(), "switch field") {
		t.Errorf("footer = %q, want it to hint at switching fields while the new-currency form is open", m.footer())
	}
}

func TestApp_Footer(t *testing.T) {
	m := newTestApp(t)
	if strings.Contains(m.footer(), "confirm field") {
		t.Error("footer should show the navigation help outside of an editing mode")
	}
	if !strings.Contains(m.footer(), "enter: view transactions") {
		t.Error("footer should hint at viewing an account's transactions on the Accounts tab")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(App)
	if !strings.Contains(m.footer(), "switch field") {
		t.Error("footer should hint at switching fields while the new-account form is open")
	}
}
