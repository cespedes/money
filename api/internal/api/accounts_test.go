package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"money/api/internal/models"
)

func TestAccountsCRUD(t *testing.T) {
	h := newTestHandler(t)

	// Creating without a name is rejected.
	rec := do(t, h, http.MethodPost, "/accounts", map[string]string{}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create without name: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var created models.Account
	rec = do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash"}, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if created.ID == 0 || created.Name != "Cash" {
		t.Fatalf("create: got %+v", created)
	}

	var got models.Account
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d", created.ID), nil, &got)
	if rec.Code != http.StatusOK || got.ID != created.ID {
		t.Fatalf("get: status=%d got=%+v", rec.Code, got)
	}

	rec = do(t, h, http.MethodGet, "/accounts/999999", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var list []models.Account
	rec = do(t, h, http.MethodGet, "/accounts", nil, &list)
	if rec.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: status=%d got=%+v", rec.Code, list)
	}

	var updated models.Account
	rec = do(t, h, http.MethodPut, fmt.Sprintf("/accounts/%d", created.ID), models.Account{Name: "Petty Cash"}, &updated)
	if rec.Code != http.StatusOK || updated.Name != "Petty Cash" {
		t.Fatalf("update: status=%d got=%+v", rec.Code, updated)
	}

	rec = do(t, h, http.MethodPut, "/accounts/999999", models.Account{Name: "X"}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update missing: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/accounts/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec = do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateAccount_InvalidBody(t *testing.T) {
	h := newTestHandler(t)

	req := httptestRequest(t, http.MethodPost, "/accounts", "{not json")
	rec := serve(h, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateAccount_InvalidID(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, http.MethodGet, "/accounts/not-a-number", nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateAccount_DuplicateCode(t *testing.T) {
	h := newTestHandler(t)
	code := "1000"

	rec := do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash", Code: &code}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body map[string]string
	rec = do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash 2", Code: &code}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate code: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "account code already in use" {
		t.Fatalf("duplicate code: body = %v", body)
	}
}

func balanceFor(balances []models.CurrencyAmount, currencyID int64) (int64, bool) {
	for _, b := range balances {
		if b.CurrencyID == currencyID {
			return b.Amount, true
		}
	}
	return 0, false
}

func TestAccountBalance(t *testing.T) {
	h := newTestHandler(t)
	usd := createTestCurrency(t, h, "USD")

	var cash, revenue models.Account
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash"}, &cash)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Revenue"}, &revenue)
	if len(cash.Balances) != 0 {
		t.Fatalf("balances of a brand new account = %+v, want none", cash.Balances)
	}

	do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Now(),
		Description: "Invoice #1",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		},
	}, nil)

	var got models.Account
	do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d", cash.ID), nil, &got)
	if bal, ok := balanceFor(got.Balances, usd.ID); !ok || bal != 1000 {
		t.Fatalf("balance via GET /accounts/{id} = (%d, %v), want 1000", bal, ok)
	}

	var list []models.Account
	do(t, h, http.MethodGet, "/accounts", nil, &list)
	for _, a := range list {
		if a.ID == cash.ID {
			if bal, ok := balanceFor(a.Balances, usd.ID); !ok || bal != 1000 {
				t.Fatalf("balance via GET /accounts = (%d, %v), want 1000", bal, ok)
			}
		}
	}
}

func TestAccountLedger(t *testing.T) {
	h := newTestHandler(t)
	usd := createTestCurrency(t, h, "USD")

	var cash, revenue models.Account
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash"}, &cash)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Revenue"}, &revenue)

	rec := do(t, h, http.MethodGet, "/accounts/999999/transactions", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ledger of a missing account: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Description: "Invoice #1",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		},
	}, nil)
	do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC),
		Description: "Invoice #2",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 500, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -500, CurrencyID: usd.ID},
		},
	}, nil)

	var ledger []models.LedgerEntry
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d/transactions", cash.ID), nil, &ledger)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if len(ledger) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(ledger), ledger)
	}
	if ledger[0].Description != "Invoice #1" || ledger[0].Amount != 1000 || ledger[0].Balance != 1000 || ledger[0].CurrencyID != usd.ID {
		t.Errorf("entry 0 = %+v", ledger[0])
	}
	if ledger[1].Description != "Invoice #2" || ledger[1].Amount != 500 || ledger[1].Balance != 1500 || ledger[1].CurrencyID != usd.ID {
		t.Errorf("entry 1 = %+v", ledger[1])
	}
}

func TestDeleteAccount_ReferencedByChild(t *testing.T) {
	h := newTestHandler(t)

	var parent models.Account
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Assets"}, &parent)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash", ParentID: &parent.ID}, nil)

	rec := do(t, h, http.MethodDelete, fmt.Sprintf("/accounts/%d", parent.ID), nil, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete referenced account: status = %d, want %d", rec.Code, http.StatusConflict)
	}
}
