package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"money/tui/internal/client"
)

func newTestTransactionsModel(t *testing.T, handler http.HandlerFunc) transactionsModel {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newTransactionsModel(client.New(srv.URL))
}

// newTestTransactionsModelWithCurrency is like newTestTransactionsModel,
// but pre-populates a single currency (testUSD), since "n" refuses to
// open the create wizard with none loaded.
func newTestTransactionsModelWithCurrency(t *testing.T, handler http.HandlerFunc) transactionsModel {
	t.Helper()
	m := newTestTransactionsModel(t, handler)
	m.currencies = []client.Currency{testUSD}
	m.currencyIndex = indexCurrencies(m.currencies)
	return m
}

func typeStringT(m transactionsModel, s string) transactionsModel {
	for _, r := range s {
		m, _ = m.Update(keyPress(string(r)))
	}
	return m
}

func TestTransactionsToRows(t *testing.T) {
	ts := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	rows := transactionsToRows([]client.Transaction{
		{ID: 1, Timestamp: ts, Description: "Invoice #1", Entries: []client.Entry{{}, {}}},
	})
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	row := rows[0]
	if row[0] != "1" || row[2] != "Invoice #1" || row[3] != "2" {
		t.Errorf("row = %v", row)
	}
}

func TestTransactionsModel_LoadTransactions(t *testing.T) {
	m := newTestTransactionsModel(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.Transaction{{ID: 1, Description: "Invoice #1"}})
	})

	msg := m.loadTransactions()
	loaded, ok := msg.(transactionsLoadedMsg)
	if !ok {
		t.Fatalf("loadTransactions(): got %T, want transactionsLoadedMsg", msg)
	}
	m2, _ := m.Update(loaded)
	if len(m2.rows) != 1 || m2.rows[0].Description != "Invoice #1" {
		t.Fatalf("rows = %+v", m2.rows)
	}
}

func TestTransactionsModel_NKeyRequiresCurrency(t *testing.T) {
	m := newTestTransactionsModel(t, nil) // no currencies pre-populated
	m, _ = m.Update(keyPress("n"))

	if m.mode != transactionsModeList {
		t.Fatalf("mode = %v, want transactionsModeList (n should refuse with no currencies)", m.mode)
	}
	if m.err == "" {
		t.Fatal("expected an error explaining no currency exists")
	}
}

func TestTransactionsModel_NKeyEntersCreateMode(t *testing.T) {
	m := newTestTransactionsModelWithCurrency(t, nil)
	m, _ = m.Update(keyPress("n"))

	if m.mode != transactionsModeCreate {
		t.Fatalf("mode = %v, want transactionsModeCreate", m.mode)
	}
	if m.step != stepDescription {
		t.Fatalf("step = %v, want stepDescription", m.step)
	}
	if !m.Editing() {
		t.Fatal("Editing() should be true in create mode")
	}
}

func TestTransactionsModel_CreateValidation_EmptyDescription(t *testing.T) {
	m := newTestTransactionsModelWithCurrency(t, nil)
	m, _ = m.Update(keyPress("n"))
	m, _ = m.Update(keyPress("enter")) // submit an empty description

	if m.err != "description is required" {
		t.Fatalf("err = %q, want %q", m.err, "description is required")
	}
	if m.step != stepDescription {
		t.Fatalf("step = %v, want to stay at stepDescription", m.step)
	}
}

func TestTransactionsModel_CreateValidation_BadAccountID(t *testing.T) {
	m := newTestTransactionsModelWithCurrency(t, nil)
	m, _ = m.Update(keyPress("n"))
	m = typeStringT(m, "Rent")
	m, _ = m.Update(keyPress("enter")) // -> stepTimestamp
	m, _ = m.Update(keyPress("enter")) // blank timestamp -> stepEntryAccount
	m = typeStringT(m, "abc")
	m, _ = m.Update(keyPress("enter")) // submit account id

	if m.err != "account ID must be a number" {
		t.Fatalf("err = %q, want %q", m.err, "account ID must be a number")
	}
	if m.step != stepEntryAccount {
		t.Fatalf("step = %v, want stepEntryAccount", m.step)
	}
}

// enterAccountAndCurrency advances past stepEntryAccount (typing acctID)
// and stepEntryCurrency (confirming whatever's under the picker's cursor,
// by default the first/only currency), landing on stepEntryValue.
func enterAccountAndCurrency(m transactionsModel, acctID string) transactionsModel {
	m = typeStringT(m, acctID)
	m, _ = m.Update(keyPress("enter")) // -> stepEntryCurrency
	m, _ = m.Update(keyPress("enter")) // confirm currency -> stepEntryValue
	return m
}

func TestTransactionsModel_CreateValidation_BadValue(t *testing.T) {
	m := newTestTransactionsModelWithCurrency(t, nil)
	m, _ = m.Update(keyPress("n"))
	m = typeStringT(m, "Rent")
	m, _ = m.Update(keyPress("enter"))
	m, _ = m.Update(keyPress("enter"))
	m = enterAccountAndCurrency(m, "1")
	m = typeStringT(m, "abc")
	m, _ = m.Update(keyPress("enter"))

	if m.err != "amount must be an integer" {
		t.Fatalf("err = %q, want %q", m.err, "amount must be an integer")
	}
	if m.step != stepEntryValue {
		t.Fatalf("step = %v, want stepEntryValue", m.step)
	}
}

func TestTransactionsModel_CreateValidation_BadCurrencySelection(t *testing.T) {
	// A currencyPicker with zero rows (e.g. currencies somehow empty by
	// the time this step is reached) must refuse to advance rather than
	// pick a nonexistent currency.
	m := newTestTransactionsModelWithCurrency(t, nil)
	m, _ = m.Update(keyPress("n"))
	m = typeStringT(m, "Rent")
	m, _ = m.Update(keyPress("enter"))
	m, _ = m.Update(keyPress("enter"))
	m = typeStringT(m, "1")
	m, _ = m.Update(keyPress("enter")) // -> stepEntryCurrency
	m.currencyPicker.SetRows(nil)      // simulate no rows under the cursor
	m.currencies = nil

	m, _ = m.Update(keyPress("enter"))
	if m.err != "pick a currency" {
		t.Fatalf("err = %q, want %q", m.err, "pick a currency")
	}
	if m.step != stepEntryCurrency {
		t.Fatalf("step = %v, want stepEntryCurrency", m.step)
	}
}

func TestTransactionsModel_CreateValidation_BadTimestamp(t *testing.T) {
	m := newTestTransactionsModelWithCurrency(t, nil)
	m, _ = m.Update(keyPress("n"))
	m = typeStringT(m, "Rent")
	m, _ = m.Update(keyPress("enter")) // -> stepTimestamp
	m = typeStringT(m, "not-a-date")
	m, _ = m.Update(keyPress("enter")) // -> stepEntryAccount (timestamp isn't parsed until submit)

	// Two balanced entries, so submitCreate gets past the entry checks and
	// actually reaches timestamp parsing.
	m = enterAccountAndCurrency(m, "1")
	m = typeStringT(m, "1000")
	m, _ = m.Update(keyPress("enter")) // -> stepConfirmMore
	m, _ = m.Update(keyPress("y"))     // add another entry -> stepEntryAccount
	m = enterAccountAndCurrency(m, "2")
	m = typeStringT(m, "-1000")
	m, _ = m.Update(keyPress("enter")) // -> stepConfirmMore
	m, _ = m.Update(keyPress("n"))     // submit

	want := "timestamp must look like " + timestampLayout
	if m.err != want {
		t.Fatalf("err = %q, want %q", m.err, want)
	}
	if m.step != stepTimestamp {
		t.Fatalf("step = %v, want stepTimestamp", m.step)
	}
}

func TestTransactionsModel_CreateFullFlow(t *testing.T) {
	var gotBody client.Transaction
	m := newTestTransactionsModelWithCurrency(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		gotBody.ID = 1
		json.NewEncoder(w).Encode(gotBody)
	})

	m, _ = m.Update(keyPress("n"))
	m = typeStringT(m, "Invoice #1")
	m, _ = m.Update(keyPress("enter")) // -> stepTimestamp
	m, _ = m.Update(keyPress("enter")) // blank -> now, -> stepEntryAccount

	m = enterAccountAndCurrency(m, "1")
	m = typeStringT(m, "1000")
	m, _ = m.Update(keyPress("enter")) // -> stepConfirmMore

	if m.step != stepConfirmMore {
		t.Fatalf("step = %v, want stepConfirmMore", m.step)
	}
	if len(m.pendingEntries) != 1 || m.pendingEntries[0].Amount != 1000 || m.pendingEntries[0].CurrencyID != testUSD.ID {
		t.Fatalf("pendingEntries = %+v", m.pendingEntries)
	}

	m, _ = m.Update(keyPress("y")) // add another entry -> stepEntryAccount
	m = enterAccountAndCurrency(m, "2")
	m = typeStringT(m, "-1000")
	m, _ = m.Update(keyPress("enter")) // -> stepConfirmMore

	m, cmd := m.Update(keyPress("n")) // n at stepConfirmMore submits
	if cmd == nil {
		t.Fatal("expected a submit command")
	}
	msg := cmd()
	mutated, ok := msg.(transactionMutatedMsg)
	if !ok {
		t.Fatalf("got %T, want transactionMutatedMsg", msg)
	}
	if mutated.err != nil {
		t.Fatalf("unexpected error: %v", mutated.err)
	}
	if gotBody.Description != "Invoice #1" || len(gotBody.Entries) != 2 {
		t.Fatalf("request body = %+v", gotBody)
	}
	if gotBody.Entries[0].CurrencyID != testUSD.ID || gotBody.Entries[1].CurrencyID != testUSD.ID {
		t.Fatalf("request entries currency = %+v, want both %d", gotBody.Entries, testUSD.ID)
	}

	m, cmd = m.Update(mutated)
	if m.mode != transactionsModeList {
		t.Fatalf("mode = %v, want transactionsModeList", m.mode)
	}
	if cmd == nil {
		t.Fatal("expected a reload command after a successful create")
	}
}

// TestTransactionsModel_CreateRejectsUnbalancedPerCurrency mirrors the
// backend's own invariant: entries must sum to zero within each
// currency, not just in total.
func TestTransactionsModel_CreateRejectsUnbalancedPerCurrency(t *testing.T) {
	eur := client.Currency{ID: 11, Name: "EUR", SymbolPosition: "after", DecimalSeparator: ",", DecimalPlaces: 2}
	m := newTestTransactionsModel(t, nil)
	m.currencies = []client.Currency{testUSD, eur}
	m.currencyIndex = indexCurrencies(m.currencies)

	m, _ = m.Update(keyPress("n"))
	m = typeStringT(m, "Mixed")
	m, _ = m.Update(keyPress("enter")) // -> stepTimestamp
	m, _ = m.Update(keyPress("enter")) // -> stepEntryAccount

	// USD leg: balanced (1000, -1000).
	m = enterAccountAndCurrency(m, "1") // picks cursor 0 = testUSD
	m = typeStringT(m, "1000")
	m, _ = m.Update(keyPress("enter")) // -> stepConfirmMore
	m, _ = m.Update(keyPress("y"))
	m = enterAccountAndCurrency(m, "2") // cursor 0 = testUSD again
	m = typeStringT(m, "-1000")
	m, _ = m.Update(keyPress("enter")) // -> stepConfirmMore
	m, _ = m.Update(keyPress("y"))

	// EUR leg: deliberately unbalanced (500, only one entry).
	m = typeStringT(m, "1")
	m, _ = m.Update(keyPress("enter")) // -> stepEntryCurrency
	m, _ = m.Update(keyPress("down"))  // move to EUR (cursor 1)
	m, _ = m.Update(keyPress("enter")) // confirm EUR -> stepEntryValue
	m = typeStringT(m, "500")
	m, _ = m.Update(keyPress("enter")) // -> stepConfirmMore

	m, _ = m.Update(keyPress("n")) // submit: USD balances, EUR doesn't
	if m.err == "" {
		t.Fatal("expected a validation error for the unbalanced EUR leg")
	}
	if !strings.Contains(m.err, "EUR") {
		t.Errorf("err = %q, want it to name the unbalanced currency (EUR)", m.err)
	}
}

func TestTransactionsModel_DetailView(t *testing.T) {
	m := newTestTransactionsModel(t, nil)
	m.currencyIndex = testCurrencies
	m.rows = []client.Transaction{{ID: 1, Description: "Invoice #1"}}
	m.tbl.SetRows(transactionsToRows(m.rows))

	m, _ = m.Update(keyPress("enter"))
	if m.mode != transactionsModeDetail {
		t.Fatalf("mode = %v, want transactionsModeDetail", m.mode)
	}
	if m.detail.Description != "Invoice #1" {
		t.Fatalf("detail = %+v", m.detail)
	}

	// Any key from the detail view goes back to the list.
	m, _ = m.Update(keyPress("x"))
	if m.mode != transactionsModeList {
		t.Fatalf("mode after leaving detail = %v, want transactionsModeList", m.mode)
	}
}

func TestTransactionsModel_DetailViewShowsFormattedAmounts(t *testing.T) {
	m := newTestTransactionsModel(t, nil)
	m.currencyIndex = testCurrencies
	m.detail = client.Transaction{
		ID:          1,
		Description: "Invoice #1",
		Entries: []client.Entry{
			{AccountID: 1, Amount: 1000, CurrencyID: testUSD.ID},
			{AccountID: 2, Amount: -1000, CurrencyID: testUSD.ID},
		},
	}
	m.mode = transactionsModeDetail

	view := m.View()
	if !strings.Contains(view, "USD10.00") || !strings.Contains(view, "USD-10.00") {
		t.Errorf("detail view should show currency-formatted amounts, got:\n%s", view)
	}
}

func TestTransactionsModel_ConfirmDeleteYesAndNo(t *testing.T) {
	deleted := false
	m := newTestTransactionsModel(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
		}
		w.WriteHeader(http.StatusNoContent)
	})
	m.rows = []client.Transaction{{ID: 9, Description: "Invoice #1"}}
	m.tbl.SetRows(transactionsToRows(m.rows))

	m, _ = m.Update(keyPress("d"))
	m, _ = m.Update(keyPress("n"))
	if m.mode != transactionsModeList || deleted {
		t.Fatalf("cancel: mode=%v deleted=%v", m.mode, deleted)
	}

	m, _ = m.Update(keyPress("d"))
	m, cmd := m.Update(keyPress("y"))
	if cmd == nil {
		t.Fatal("expected a delete command")
	}
	cmd()
	if !deleted {
		t.Fatal("expected the API to have been called with DELETE")
	}
}
