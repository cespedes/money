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
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"}) // enter create mode
	m = updated.(App)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // confirm "(none)" as parent, -> name field
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
	m.accounts.table.SetRows(accountsToRows(m.accounts.rows))

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
	m.accounts.table.SetRows(accountsToRows(m.accounts.rows))

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
	if !strings.Contains(v.Content, "Money") {
		t.Errorf("view content = %q, want it to contain the title", v.Content)
	}
	if !strings.Contains(v.Content, "Accounts") || !strings.Contains(v.Content, "Transactions") {
		t.Errorf("view content = %q, want both tab labels", v.Content)
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
	if !strings.Contains(m.footer(), "confirm field") {
		t.Error("footer should show the editing help while a form is open")
	}
}
