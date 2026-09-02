package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

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

func TestAccountsToRows(t *testing.T) {
	code := "1000"
	parent := int64(7)
	rows := accountsToRows([]client.Account{
		{ID: 1, Name: "Cash", Code: &code, ParentID: &parent},
		{ID: 2, Name: "Bank"},
	})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if got, want := rows[0], (table.Row{"1", "1000", "Cash", "7"}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 0 = %v, want %v", got, want)
	}
	if got, want := rows[1], (table.Row{"2", "", "Bank", ""}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 1 = %v, want %v", got, want)
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
	m, _ = m.Update(keyPress("n"))

	if m.mode != accountsModeCreate {
		t.Fatalf("mode = %v, want accountsModeCreate", m.mode)
	}
	if !m.inputs[fieldAccountName].Focused() {
		t.Fatal("expected the name field to be focused")
	}
	if !m.Editing() {
		t.Fatal("Editing() should be true in create mode")
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

func TestAccountsModel_CreateInvalidParentID(t *testing.T) {
	m := newTestAccountsModel(t, nil)
	m, _ = m.Update(keyPress("n"))

	m = typeString(m, "Cash")
	m, _ = m.Update(keyPress("enter")) // -> code field
	m, _ = m.Update(keyPress("enter")) // -> parent field
	m = typeString(m, "abc")
	m, _ = m.Update(keyPress("enter")) // submit

	if m.err != "parent account ID must be a number" {
		t.Fatalf("err = %q, want %q", m.err, "parent account ID must be a number")
	}
}

func TestAccountsModel_CreateSubmitsAndReturnsToList(t *testing.T) {
	var gotName string
	m := newTestAccountsModel(t, func(w http.ResponseWriter, r *http.Request) {
		var a client.Account
		json.NewDecoder(r.Body).Decode(&a)
		gotName = a.Name
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Account{ID: 1, Name: a.Name})
	})
	m, _ = m.Update(keyPress("n"))
	m = typeString(m, "Cash")
	m, _ = m.Update(keyPress("enter")) // -> code
	m, _ = m.Update(keyPress("enter")) // -> parent
	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected a command to submit the create request")
	}

	msg := cmd()
	mutated, ok := msg.(accountMutatedMsg)
	if !ok {
		t.Fatalf("got %T, want accountMutatedMsg", msg)
	}
	if mutated.err != nil {
		t.Fatalf("unexpected error: %v", mutated.err)
	}
	if gotName != "Cash" {
		t.Fatalf("API received name %q, want %q", gotName, "Cash")
	}

	m, cmd = m.Update(mutated)
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

	// Too small a height must not be applied, to keep the table usable.
	m.SetSize(80, 3)
	if got := m.table.Height(); got != got20 {
		t.Errorf("table height after a too-small SetSize = %d, want unchanged %d", got, got20)
	}
}
