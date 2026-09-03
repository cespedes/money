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

func TestCreateAccount_AssignsSequentialPosition(t *testing.T) {
	h := newTestHandler(t)

	var a, b, c models.Account
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Assets"}, &a)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Liabilities"}, &b)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Equity"}, &c)

	if a.Position != 0 || b.Position != 1 || c.Position != 2 {
		t.Fatalf("positions = %d, %d, %d, want 0, 1, 2", a.Position, b.Position, c.Position)
	}

	// A child account's position is independent of its parent's siblings:
	// it starts its own sibling group back at 0.
	assetsID := a.ID
	var child models.Account
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash", ParentID: &assetsID}, &child)
	if child.Position != 0 {
		t.Fatalf("first child's position = %d, want 0", child.Position)
	}
}

func TestMoveAccount(t *testing.T) {
	h := newTestHandler(t)

	var a, b, c models.Account
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Assets"}, &a)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Liabilities"}, &b)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Equity"}, &c)

	// Move Liabilities (position 1) up: swaps with Assets (position 0).
	rec := do(t, h, http.MethodPost, fmt.Sprintf("/accounts/%d/move", b.ID), map[string]string{"direction": "up"}, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("move up: status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	var got models.Account
	do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d", a.ID), nil, &got)
	if got.Position != 1 {
		t.Fatalf("Assets.Position after swap = %d, want 1", got.Position)
	}
	do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d", b.ID), nil, &got)
	if got.Position != 0 {
		t.Fatalf("Liabilities.Position after swap = %d, want 0", got.Position)
	}

	// Liabilities is now first; moving it up again is a no-op, not an error.
	rec = do(t, h, http.MethodPost, fmt.Sprintf("/accounts/%d/move", b.ID), map[string]string{"direction": "up"}, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("move up at boundary: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d", b.ID), nil, &got)
	if got.Position != 0 {
		t.Fatalf("Liabilities.Position after no-op move = %d, want unchanged 0", got.Position)
	}

	// Equity is last; moving it down is likewise a no-op.
	rec = do(t, h, http.MethodPost, fmt.Sprintf("/accounts/%d/move", c.ID), map[string]string{"direction": "down"}, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("move down at boundary: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestMoveAccount_InvalidDirection(t *testing.T) {
	h := newTestHandler(t)
	var a models.Account
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Assets"}, &a)

	var body map[string]string
	rec := do(t, h, http.MethodPost, fmt.Sprintf("/accounts/%d/move", a.ID), map[string]string{"direction": "sideways"}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != `direction must be "up" or "down"` {
		t.Fatalf("body = %v", body)
	}
}

func TestMoveAccount_NotFound(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, http.MethodPost, "/accounts/999999/move", map[string]string{"direction": "up"}, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
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
