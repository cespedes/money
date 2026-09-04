package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"money/tui/internal/client"
)

func newTestCurrenciesModel(t *testing.T, handler http.HandlerFunc) currenciesModel {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newCurrenciesModel(client.New(srv.URL))
}

func typeStringC(m currenciesModel, s string) currenciesModel {
	for _, r := range s {
		m, _ = m.Update(keyPress(string(r)))
	}
	return m
}

func TestCurrenciesToRows(t *testing.T) {
	isin := "US0000000001"
	rows := currenciesToRows([]client.Currency{
		{ID: 1, Name: "$", SymbolBefore: true, ThousandsSeparator: ",", DecimalSeparator: ".", DecimalPlaces: 2, ISIN: &isin},
		{ID: 2, Name: "EUR", SymbolBefore: false, SymbolSpace: true, ThousandsSeparator: ".", DecimalSeparator: ",", DecimalPlaces: 2},
		{ID: 3, Name: "PTS", SymbolBefore: false, SymbolSpace: true, DecimalPlaces: 0},
	})
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if got, want := rows[0], (table.Row{"$", "$1,234.00", isin}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 0 = %v, want %v", got, want)
	}
	if got, want := rows[1], (table.Row{"EUR", "1.234,00 EUR", ""}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 1 = %v, want %v", got, want)
	}
	if got, want := rows[2], (table.Row{"PTS", "1234 PTS", ""}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 2 = %v, want %v", got, want)
	}
}

func TestParseCurrencyFormat(t *testing.T) {
	cases := []struct {
		name, format               string
		wantBefore, wantSpace      bool
		wantThousands, wantDecimal string
		wantDecimalPlaces          int
	}{
		{name: "$", format: "$1,234.00", wantBefore: true, wantSpace: false, wantThousands: ",", wantDecimal: ".", wantDecimalPlaces: 2},
		{name: "EUR", format: "1.234,00 EUR", wantBefore: false, wantSpace: true, wantThousands: ".", wantDecimal: ",", wantDecimalPlaces: 2},
		{name: "PTS", format: "1234 PTS", wantBefore: false, wantSpace: true, wantThousands: "", wantDecimal: "", wantDecimalPlaces: 0},
		{name: "USD", format: "USD1234", wantBefore: true, wantSpace: false, wantThousands: "", wantDecimal: "", wantDecimalPlaces: 0},
		{name: "USD", format: "1234.000 USD", wantBefore: false, wantSpace: true, wantThousands: "", wantDecimal: ".", wantDecimalPlaces: 3},
		{name: "USD", format: "USD 1,234", wantBefore: true, wantSpace: true, wantThousands: ",", wantDecimal: "", wantDecimalPlaces: 0},
	}
	for _, c := range cases {
		before, space, thousands, decimal, decimalPlaces, ok := parseCurrencyFormat(c.name, c.format)
		if !ok {
			t.Errorf("parseCurrencyFormat(%q, %q): got ok=false, want true", c.name, c.format)
			continue
		}
		if before != c.wantBefore || space != c.wantSpace || thousands != c.wantThousands ||
			decimal != c.wantDecimal || decimalPlaces != c.wantDecimalPlaces {
			t.Errorf("parseCurrencyFormat(%q, %q) = (before=%v space=%v thousands=%q decimal=%q places=%d), want (before=%v space=%v thousands=%q decimal=%q places=%d)",
				c.name, c.format, before, space, thousands, decimal, decimalPlaces,
				c.wantBefore, c.wantSpace, c.wantThousands, c.wantDecimal, c.wantDecimalPlaces)
		}
	}
}

func TestParseCurrencyFormat_Invalid(t *testing.T) {
	cases := []struct{ name, format string }{
		{"$", "1,234.00"},      // name missing entirely
		{"$", "$1234 "},        // dangling space, not part of any recognized pattern
		{"$", "$  1234"},       // double space after name
		{"$", "$123"},          // wrong quantity
		{"$", "$12345"},        // wrong quantity
		{"EUR", "1.23,00 EUR"}, // thousands group isn't "234"
		{"EUR", "12,34 EUR"},   // wrong thousands grouping (2+2, not 1+3)
		{"", "$1234"},          // empty name
	}
	for _, c := range cases {
		if _, _, _, _, _, ok := parseCurrencyFormat(c.name, c.format); ok {
			t.Errorf("parseCurrencyFormat(%q, %q): got ok=true, want false", c.name, c.format)
		}
	}
}

func TestCurrenciesModel_LoadCurrencies(t *testing.T) {
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.Currency{{ID: 1, Name: "US Dollar"}})
	})

	msg := m.loadCurrencies()
	loaded, ok := msg.(currenciesLoadedMsg)
	if !ok {
		t.Fatalf("loadCurrencies(): got %T, want currenciesLoadedMsg", msg)
	}
	m2, _ := m.Update(loaded)
	if len(m2.rows) != 1 || m2.rows[0].Name != "US Dollar" {
		t.Fatalf("rows = %+v", m2.rows)
	}
}

func TestCurrenciesModel_NKeyEntersCreateMode(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m, _ = m.Update(keyPress("n"))

	if m.mode != currenciesModeCreate {
		t.Fatalf("mode = %v, want currenciesModeCreate", m.mode)
	}
	if m.createFocus != focusCurrencyName {
		t.Fatalf("createFocus = %v, want focusCurrencyName", m.createFocus)
	}
	if !m.Editing() {
		t.Fatal("Editing() should be true in create mode")
	}
}

// TestCurrenciesModel_CreatePastesIntoFocusedField is a regression test:
// a tea.PasteMsg (sent for bracketed-paste terminal input, distinct from
// tea.KeyMsg) must reach whichever field currently has focus, the same
// as typing would.
func TestCurrenciesModel_CreatePastesIntoFocusedField(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m, _ = m.Update(keyPress("n")) // Name already has focus

	m, _ = m.Update(tea.PasteMsg{Content: "Pasted Name"})
	if got := m.inputs[fieldCurrencyName].Value(); got != "Pasted Name" {
		t.Fatalf("Name value after paste = %q, want %q", got, "Pasted Name")
	}
}

func TestCurrenciesModel_CreateFocusCycles(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m, _ = m.Update(keyPress("n"))

	order := []currencyFocus{focusCurrencyName, focusCurrencyFormat, focusCurrencyISIN, focusCurrencyName}
	for i, want := range order[1:] {
		m, _ = m.Update(keyPress("tab"))
		if m.createFocus != want {
			t.Fatalf("after tab #%d: createFocus = %v, want %v", i+1, m.createFocus, want)
		}
	}

	m, _ = m.Update(keyPress("shift+tab"))
	if m.createFocus != focusCurrencyISIN {
		t.Fatalf("after shift+tab: createFocus = %v, want focusCurrencyISIN", m.createFocus)
	}
	m, _ = m.Update(keyPress("left"))
	if m.createFocus != focusCurrencyFormat {
		t.Fatalf("after left: createFocus = %v, want focusCurrencyFormat", m.createFocus)
	}
	m, _ = m.Update(keyPress("right"))
	if m.createFocus != focusCurrencyISIN {
		t.Fatalf("after right: createFocus = %v, want focusCurrencyISIN", m.createFocus)
	}
}

func TestCurrenciesModel_CreateValidation(t *testing.T) {
	called := false
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	m, _ = m.Update(keyPress("n"))

	m, cmd := m.Update(keyPress("enter")) // empty name
	if m.err != "name is required" {
		t.Fatalf("err = %q, want %q", m.err, "name is required")
	}
	if cmd != nil {
		cmd()
	}
	if called {
		t.Fatal("the API should not have been called for an invalid submission")
	}
	if m.createFocus != focusCurrencyName {
		t.Fatalf("createFocus = %v, want focusCurrencyName", m.createFocus)
	}
}

func TestCurrenciesModel_CreateValidation_InvalidFormat(t *testing.T) {
	called := false
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	m, _ = m.Update(keyPress("n"))
	m = typeStringC(m, "USD")
	m, _ = m.Update(keyPress("tab")) // -> Format
	m = typeStringC(m, "nonsense")

	m, cmd := m.Update(keyPress("enter"))
	if m.err == "" {
		t.Fatal("expected an error for an unparseable format")
	}
	if cmd != nil {
		cmd()
	}
	if called {
		t.Fatal("the API should not have been called for an invalid submission")
	}
	if m.createFocus != focusCurrencyFormat {
		t.Fatalf("createFocus = %v, want focusCurrencyFormat", m.createFocus)
	}
}

func TestCurrenciesModel_CreateSubmits(t *testing.T) {
	var gotBody client.Currency
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Currency{ID: 1, Name: gotBody.Name})
	})
	m, _ = m.Update(keyPress("n"))
	m = typeStringC(m, "EUR")
	m, _ = m.Update(keyPress("tab")) // -> Format
	m = typeStringC(m, "1.234,00 EUR")
	m, _ = m.Update(keyPress("tab")) // -> ISIN
	m = typeStringC(m, "EU0000000001")

	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected a command to submit the create request")
	}
	msg := cmd()
	mutated, ok := msg.(currencyMutatedMsg)
	if !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}

	if gotBody.Name != "EUR" || gotBody.SymbolBefore || !gotBody.SymbolSpace ||
		gotBody.DecimalPlaces != 2 || gotBody.ThousandsSeparator != "." || gotBody.DecimalSeparator != "," {
		t.Fatalf("API received %+v", gotBody)
	}
	if gotBody.ISIN == nil || *gotBody.ISIN != "EU0000000001" {
		t.Fatalf("API received ISIN %v, want EU0000000001", gotBody.ISIN)
	}

	m, cmd = m.Update(mutated)
	if m.mode != currenciesModeList {
		t.Fatalf("mode = %v, want currenciesModeList", m.mode)
	}
	if cmd == nil {
		t.Fatal("expected a reload command after a successful create")
	}
}

func TestCurrenciesModel_CreateOmitsBlankISIN(t *testing.T) {
	var gotBody client.Currency
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Currency{ID: 1, Name: gotBody.Name})
	})
	m, _ = m.Update(keyPress("n"))
	m = typeStringC(m, "PTS")
	m, _ = m.Update(keyPress("tab")) // -> Format
	m = typeStringC(m, "1234 PTS")

	_, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected a command to submit the create request")
	}
	cmd()
	if gotBody.ISIN != nil {
		t.Fatalf("ISIN = %v, want nil when left blank", gotBody.ISIN)
	}
}

func TestCurrenciesModel_EscCancelsCreate(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m, _ = m.Update(keyPress("n"))
	m, _ = m.Update(keyPress("esc"))

	if m.mode != currenciesModeList {
		t.Fatalf("mode = %v, want currenciesModeList", m.mode)
	}
	if m.Editing() {
		t.Fatal("Editing() should be false back in list mode")
	}
}

func TestCurrenciesModel_DKeyRequiresRows(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)

	m, _ = m.Update(keyPress("d"))
	if m.mode != currenciesModeList {
		t.Fatalf("mode with no rows = %v, want currenciesModeList (d should be a no-op)", m.mode)
	}

	m.rows = []client.Currency{{ID: 1, Name: "USD"}}
	m, _ = m.Update(keyPress("d"))
	if m.mode != currenciesModeConfirmDelete {
		t.Fatalf("mode with rows = %v, want currenciesModeConfirmDelete", m.mode)
	}
}

func TestCurrenciesModel_EKeyRequiresRows(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)

	m, _ = m.Update(keyPress("e"))
	if m.mode != currenciesModeList {
		t.Fatalf("mode with no rows = %v, want currenciesModeList (e should be a no-op)", m.mode)
	}

	m.rows = []client.Currency{{ID: 1, Name: "USD"}}
	m, _ = m.Update(keyPress("e"))
	if m.mode != currenciesModeCreate {
		t.Fatalf("mode with rows = %v, want currenciesModeCreate", m.mode)
	}
}

// TestCurrenciesModel_EKeyPrefillsForm checks that "e" opens the same
// pop-up as "n", pre-filled with the selected currency's current values —
// Format reconstructed from its stored rules, the inverse of what
// submitForm parses back out of it (see parseCurrencyFormat).
func TestCurrenciesModel_EKeyPrefillsForm(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	isin := "EU0000000001"
	m.rows = []client.Currency{
		{ID: 1, Name: "USD"},
		{ID: 2, Name: "EUR", SymbolBefore: false, SymbolSpace: true, ThousandsSeparator: ".", DecimalSeparator: ",", DecimalPlaces: 2, ISIN: &isin},
	}
	m.table.SetRows(currenciesToRows(m.rows))

	m, _ = m.Update(keyPress("down")) // cursor -> row 1 (EUR)
	m, _ = m.Update(keyPress("e"))

	if m.mode != currenciesModeCreate {
		t.Fatalf("mode = %v, want currenciesModeCreate", m.mode)
	}
	if m.editingID == nil || *m.editingID != 2 {
		t.Fatalf("editingID = %v, want 2", m.editingID)
	}
	if got := m.inputs[fieldCurrencyName].Value(); got != "EUR" {
		t.Errorf("Name = %q, want %q", got, "EUR")
	}
	if got := m.inputs[fieldCurrencyFormat].Value(); got != "1.234,00 EUR" {
		t.Errorf("Format = %q, want %q", got, "1.234,00 EUR")
	}
	if got := m.inputs[fieldCurrencyISIN].Value(); got != isin {
		t.Errorf("ISIN = %q, want %q", got, isin)
	}

	popup := m.createPopup()
	if !strings.Contains(popup, "Edit currency") {
		t.Errorf("popup should be titled %q, got:\n%s", "Edit currency", popup)
	}
}

func TestCurrenciesModel_EditSubmitsUpdate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody client.Currency
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(client.Currency{ID: 1, Name: gotBody.Name})
	})
	m.rows = []client.Currency{{ID: 1, Name: "USD", SymbolBefore: true, DecimalPlaces: 2}}
	m.table.SetRows(currenciesToRows(m.rows))

	m, _ = m.Update(keyPress("e"))
	m, _ = m.Update(keyPress("tab")) // -> Format
	for range m.inputs[fieldCurrencyFormat].Value() {
		m, _ = m.Update(keyPress("backspace"))
	}
	m = typeStringC(m, "1234 USD")

	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected a command to submit the update request")
	}
	msg := cmd()
	mutated, ok := msg.(currencyMutatedMsg)
	if !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if gotMethod != http.MethodPut || gotPath != "/currencies/1" {
		t.Fatalf("got %s %s, want PUT /currencies/1", gotMethod, gotPath)
	}
	if gotBody.SymbolBefore || gotBody.DecimalPlaces != 0 {
		t.Fatalf("request body = %+v", gotBody)
	}

	m, cmd = m.Update(mutated)
	if m.mode != currenciesModeList {
		t.Fatalf("mode = %v, want currenciesModeList", m.mode)
	}
	if m.editingID != nil {
		t.Fatalf("editingID = %v, want nil after a successful edit", m.editingID)
	}
	if cmd == nil {
		t.Fatal("expected a reload command after a successful edit")
	}
}

func TestCurrenciesModel_ConfirmDeleteYesAndNo(t *testing.T) {
	deleted := false
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
		}
		w.WriteHeader(http.StatusNoContent)
	})
	m.rows = []client.Currency{{ID: 5, Name: "USD"}}
	m.table.SetRows(currenciesToRows(m.rows))

	m, _ = m.Update(keyPress("d"))
	m, _ = m.Update(keyPress("n"))
	if m.mode != currenciesModeList || deleted {
		t.Fatalf("cancel: mode=%v deleted=%v", m.mode, deleted)
	}

	m, _ = m.Update(keyPress("d"))
	m, cmd := m.Update(keyPress("y"))
	if cmd == nil {
		t.Fatal("expected a delete command")
	}
	msg := cmd()
	if mutated, ok := msg.(currencyMutatedMsg); !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}
	if !deleted {
		t.Fatal("expected the API to have been called with DELETE")
	}
}

func TestCurrenciesModel_CreatePopup(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m.SetSize(100, 20)
	m.rows = []client.Currency{{ID: 1, Name: "USD"}}
	m.table.SetRows(currenciesToRows(m.rows))
	m, _ = m.Update(keyPress("n"))
	m = typeStringC(m, "EUR")
	m, _ = m.Update(keyPress("tab")) // -> Format
	m = typeStringC(m, "1.234,00 EUR")

	popup := m.createPopup()
	for _, want := range []string{"New currency", "Name", "Format", "ISIN", "EUR", "1.234,00 EUR"} {
		if !strings.Contains(popup, want) {
			t.Errorf("popup should contain %q, got:\n%s", want, popup)
		}
	}

	if got := m.View(); !strings.Contains(got, "USD") {
		t.Errorf("View() should still render the currencies list under the pop-up, got:\n%s", got)
	}
}

func TestIndexCurrencies(t *testing.T) {
	idx := indexCurrencies([]client.Currency{{ID: 1, Name: "USD"}, {ID: 2, Name: "EUR"}})
	if len(idx) != 2 || idx[1].Name != "USD" || idx[2].Name != "EUR" {
		t.Fatalf("indexCurrencies = %+v", idx)
	}
}

func TestFormatAmount(t *testing.T) {
	if got := formatAmount("10", testCurrencies, testUSD.ID); got != "USD10.00" {
		t.Errorf("formatAmount with known currency = %q, want %q", got, "USD10.00")
	}
	if got := formatAmount("1000", testCurrencies, 999); got != "1000 (?)" {
		t.Errorf("formatAmount with unknown currency = %q, want the raw-amount fallback", got)
	}
}

func TestExampleAmount(t *testing.T) {
	if got := exampleAmount(0); got != 1234 {
		t.Errorf("exampleAmount(0) = %d, want 1234", got)
	}
	if got := exampleAmount(2); got != 123400 {
		t.Errorf("exampleAmount(2) = %d, want 123400", got)
	}
}
