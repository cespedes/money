package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/table"

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
		{ID: 1, Name: "US Dollar", SymbolBefore: true, DecimalPlaces: 2, ISIN: &isin},
		{ID: 2, Name: "Euro", SymbolBefore: false, DecimalPlaces: 2},
	})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if got, want := rows[0], (table.Row{"1", "US Dollar", "before", "2", isin}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 0 = %v, want %v", got, want)
	}
	if got, want := rows[1], (table.Row{"2", "Euro", "after", "2", ""}); !reflect.DeepEqual(got, want) {
		t.Errorf("row 1 = %v, want %v", got, want)
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

func TestCurrenciesModel_NKeyEntersCreateModeWithDefaults(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m, _ = m.Update(keyPress("n"))

	if m.mode != currenciesModeCreate {
		t.Fatalf("mode = %v, want currenciesModeCreate", m.mode)
	}
	if m.createFocus != focusCurrencyName {
		t.Fatalf("createFocus = %v, want focusCurrencyName", m.createFocus)
	}
	if !m.symbolBefore || m.symbolSpace || m.decimalPlaces != 2 {
		t.Fatalf("defaults = before=%v space=%v decimals=%d, want true/false/2", m.symbolBefore, m.symbolSpace, m.decimalPlaces)
	}
	if !m.Editing() {
		t.Fatal("Editing() should be true in create mode")
	}
}

func TestCurrenciesModel_CreateFocusCycles(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m, _ = m.Update(keyPress("n"))

	order := []currencyFocus{
		focusCurrencyName, focusCurrencySymbolBefore, focusCurrencySymbolSpace, focusCurrencyDecimalPlaces,
		focusCurrencyThousandsSep, focusCurrencyDecimalSep, focusCurrencyISIN, focusCurrencyName,
	}
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
	if m.createFocus != focusCurrencyDecimalSep {
		t.Fatalf("after left: createFocus = %v, want focusCurrencyDecimalSep", m.createFocus)
	}
	m, _ = m.Update(keyPress("right"))
	if m.createFocus != focusCurrencyISIN {
		t.Fatalf("after right: createFocus = %v, want focusCurrencyISIN", m.createFocus)
	}
}

func TestCurrenciesModel_ToggleFields(t *testing.T) {
	m := newTestCurrenciesModel(t, nil)
	m, _ = m.Update(keyPress("n")) // before=true, space=false, decimals=2

	m, _ = m.Update(keyPress("tab")) // -> Position
	m, _ = m.Update(keyPress("up"))
	if m.symbolBefore {
		t.Fatal("symbolBefore after toggle should be false")
	}
	m, _ = m.Update(keyPress("down"))
	if !m.symbolBefore {
		t.Fatal("symbolBefore after second toggle should be true")
	}

	m, _ = m.Update(keyPress("tab")) // -> Space
	m, _ = m.Update(keyPress("up"))
	if !m.symbolSpace {
		t.Fatal("symbolSpace should be true after toggling")
	}

	m, _ = m.Update(keyPress("tab")) // -> Decimals
	m, _ = m.Update(keyPress("up"))
	m, _ = m.Update(keyPress("up"))
	if m.decimalPlaces != 4 {
		t.Fatalf("decimalPlaces = %d, want 4", m.decimalPlaces)
	}
	m, _ = m.Update(keyPress("down"))
	if m.decimalPlaces != 3 {
		t.Fatalf("decimalPlaces = %d, want 3", m.decimalPlaces)
	}

	// Decimals is clamped at 0 (can't go negative).
	for range 10 {
		m, _ = m.Update(keyPress("down"))
	}
	if m.decimalPlaces != 0 {
		t.Fatalf("decimalPlaces should clamp at 0, got %d", m.decimalPlaces)
	}

	// ... and at maxCurrencyDecimalPlaces.
	for range maxCurrencyDecimalPlaces + 5 {
		m, _ = m.Update(keyPress("up"))
	}
	if m.decimalPlaces != maxCurrencyDecimalPlaces {
		t.Fatalf("decimalPlaces should clamp at %d, got %d", maxCurrencyDecimalPlaces, m.decimalPlaces)
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

func TestCurrenciesModel_CreateSubmits(t *testing.T) {
	var gotBody client.Currency
	m := newTestCurrenciesModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Currency{ID: 1, Name: gotBody.Name})
	})
	m, _ = m.Update(keyPress("n"))
	m = typeStringC(m, "USD")
	m, _ = m.Update(keyPress("tab")) // -> Position
	m, _ = m.Update(keyPress("up"))  // "after"
	m, _ = m.Update(keyPress("tab")) // -> Space
	m, _ = m.Update(keyPress("up"))  // true
	m, _ = m.Update(keyPress("tab")) // -> Decimals
	m, _ = m.Update(keyPress("up"))  // 3
	m, _ = m.Update(keyPress("tab")) // -> Thousands
	m = typeStringC(m, ",")
	m, _ = m.Update(keyPress("tab")) // -> Decimal sep
	m = typeStringC(m, ".")
	m, _ = m.Update(keyPress("tab")) // -> ISIN
	m = typeStringC(m, "US0000000001")

	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("expected a command to submit the create request")
	}
	msg := cmd()
	mutated, ok := msg.(currencyMutatedMsg)
	if !ok || mutated.err != nil {
		t.Fatalf("got %#v", msg)
	}

	if gotBody.Name != "USD" || gotBody.SymbolBefore || !gotBody.SymbolSpace ||
		gotBody.DecimalPlaces != 3 || gotBody.ThousandsSeparator != "," || gotBody.DecimalSeparator != "." {
		t.Fatalf("API received %+v", gotBody)
	}
	if gotBody.ISIN == nil || *gotBody.ISIN != "US0000000001" {
		t.Fatalf("API received ISIN %v, want US0000000001", gotBody.ISIN)
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
	m = typeStringC(m, "USD")

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

	popup := m.createPopup()
	for _, want := range []string{"New currency", "Name", "Position", "Space", "Decimals", "Thousands", "Decimal Sep", "ISIN", "EUR", "before", "no", "2"} {
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
	if got := formatAmount(1000, testCurrencies, testUSD.ID); got != "USD10.00" {
		t.Errorf("formatAmount with known currency = %q, want %q", got, "USD10.00")
	}
	if got := formatAmount(1000, testCurrencies, 999); got != "1000 (?)" {
		t.Errorf("formatAmount with unknown currency = %q, want the raw-amount fallback", got)
	}
}

func TestYesNo(t *testing.T) {
	if yesNo(true) != "yes" || yesNo(false) != "no" {
		t.Errorf("yesNo(true)=%q yesNo(false)=%q", yesNo(true), yesNo(false))
	}
}
