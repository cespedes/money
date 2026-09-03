package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"money/api/internal/models"
)

func createTwoAccountsHTTP(t *testing.T, h http.Handler) (cash, revenue models.Account) {
	t.Helper()
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Cash"}, &cash)
	do(t, h, http.MethodPost, "/accounts", models.Account{Name: "Revenue"}, &revenue)
	return cash, revenue
}

func TestTransactionsCRUD(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD")

	txn := models.Transaction{
		Timestamp:   time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC),
		Description: "Invoice #1",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		},
	}

	var created models.Transaction
	rec := do(t, h, http.MethodPost, "/transactions", txn, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if created.ID == 0 || len(created.Entries) != 2 {
		t.Fatalf("create: got %+v", created)
	}
	if created.Entries[0].CurrencyID != usd.ID {
		t.Fatalf("create: entry currency = %d, want %d", created.Entries[0].CurrencyID, usd.ID)
	}

	var got models.Transaction
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/transactions/%d", created.ID), nil, &got)
	if rec.Code != http.StatusOK || len(got.Entries) != 2 {
		t.Fatalf("get: status=%d got=%+v", rec.Code, got)
	}

	var list []models.Transaction
	rec = do(t, h, http.MethodGet, "/transactions", nil, &list)
	if rec.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: status=%d got=%+v", rec.Code, list)
	}

	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/transactions/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec = do(t, h, http.MethodGet, fmt.Sprintf("/transactions/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateTransaction_Validation(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD")
	ts := time.Now()

	tests := []struct {
		name string
		txn  models.Transaction
	}{
		{
			name: "missing description",
			txn: models.Transaction{Timestamp: ts, Entries: []models.Entry{
				{AccountID: cash.ID, Amount: 1, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1, CurrencyID: usd.ID},
			}},
		},
		{
			name: "missing timestamp",
			txn: models.Transaction{Description: "x", Entries: []models.Entry{
				{AccountID: cash.ID, Amount: 1, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1, CurrencyID: usd.ID},
			}},
		},
		{
			name: "fewer than two entries",
			txn:  models.Transaction{Timestamp: ts, Description: "x", Entries: []models.Entry{{AccountID: cash.ID, Amount: 0, CurrencyID: usd.ID}}},
		},
		{
			name: "entries do not sum to zero",
			txn: models.Transaction{Timestamp: ts, Description: "x", Entries: []models.Entry{
				{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -900, CurrencyID: usd.ID},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/transactions", tt.txn, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestCreateTransaction_UnknownAccount(t *testing.T) {
	h := newTestHandler(t)
	cash, _ := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD")

	var body map[string]string
	rec := do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Now(),
		Description: "Bad reference",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: 999999, Amount: -1000, CurrencyID: usd.ID},
		},
	}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "one or more entries reference an account or currency that does not exist" {
		t.Fatalf("body = %v", body)
	}
}

func TestCreateTransaction_UnknownCurrency(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)

	rec := do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Now(),
		Description: "Bad currency",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: 999999},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: 999999},
		},
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestCreateTransaction_MixedCurrencies confirms a transaction can post
// in more than one currency at once, as long as each currency's own
// entries sum to zero — and that an imbalance in just one of them is
// still rejected.
func TestCreateTransaction_MixedCurrencies(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD")
	eur := createTestCurrency(t, h, "EUR")

	rec := do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Now(),
		Description: "Balanced in both currencies",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 500, CurrencyID: eur.ID},
			{AccountID: revenue.ID, Amount: -500, CurrencyID: eur.ID},
		},
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("balanced multi-currency: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body map[string]string
	rec = do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Now(),
		Description: "USD short by 100",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -900, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 500, CurrencyID: eur.ID},
			{AccountID: revenue.ID, Amount: -500, CurrencyID: eur.ID},
		},
	}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unbalanced in one currency: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "entry amounts must sum to zero within each currency" {
		t.Fatalf("body = %v", body)
	}
}
