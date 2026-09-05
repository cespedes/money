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

	txn := transactionDTO{
		Timestamp:   time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC),
		Description: "Invoice #1",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: 10, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -10, CurrencyID: usd.ID},
		},
	}

	var created transactionDTO
	rec := do(t, h, http.MethodPost, "/transactions", txn, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if created.ID == 0 || len(created.Entries) != 2 {
		t.Fatalf("create: got %+v", created)
	}
	if created.Entries[0].CurrencyID != usd.ID || created.Entries[0].Amount != 10 {
		t.Fatalf("create: entry 0 = %+v, want amount 10, currency %d", created.Entries[0], usd.ID)
	}
	if created.Entries[1].Amount != -10 {
		t.Fatalf("create: entry 1 = %+v, want amount -10", created.Entries[1])
	}

	var got transactionDTO
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/transactions/%d", created.ID), nil, &got)
	if rec.Code != http.StatusOK || len(got.Entries) != 2 {
		t.Fatalf("get: status=%d got=%+v", rec.Code, got)
	}

	var list []transactionDTO
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

// TestCreateTransaction_CurrencyExchange proves a transaction touching
// exactly two currencies, with a nonzero net of opposite sign in each,
// is accepted as an implicit currency exchange rather than rejected as
// unbalanced (see TestCurrencyPricesReflectsTransactionExchange for how
// it then surfaces as a currency-price observation).
func TestCreateTransaction_CurrencyExchange(t *testing.T) {
	h := newTestHandler(t)
	cash, _ := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD")
	eur := createTestCurrency(t, h, "EUR")

	txn := transactionDTO{
		Timestamp:   time.Now(),
		Description: "Exchange USD for EUR",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: -10, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 8.5, CurrencyID: eur.ID},
		},
	}

	var created transactionDTO
	rec := do(t, h, http.MethodPost, "/transactions", txn, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// TestCreateTransaction_DecimalAmounts confirms an entry's amount is a
// real decimal number in the currency's own units, not an integer count
// of minor units — e.g. 10.5 for a currency with 2 decimal places means
// 1050 minor units internally, and comes back the same way.
func TestCreateTransaction_DecimalAmounts(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD") // 2 decimal places

	var created transactionDTO
	rec := do(t, h, http.MethodPost, "/transactions", transactionDTO{
		Timestamp:   time.Now(),
		Description: "Fractional",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: 10.5, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -10.5, CurrencyID: usd.ID},
		},
	}, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if created.Entries[0].Amount != 10.5 || created.Entries[1].Amount != -10.5 {
		t.Fatalf("entries = %+v, want 10.5 and -10.5", created.Entries)
	}

	var got accountDTO
	do(t, h, http.MethodGet, fmt.Sprintf("/accounts/%d", cash.ID), nil, &got)
	if bal, ok := balanceFor(got.Balances, usd.ID); !ok || bal != 10.5 {
		t.Fatalf("balance = (%v, %v), want 10.5", bal, ok)
	}
}

func TestCreateTransaction_Validation(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD")
	ts := time.Now()

	tests := []struct {
		name string
		txn  transactionDTO
	}{
		{
			name: "missing description",
			txn: transactionDTO{Timestamp: ts, Entries: []entryDTO{
				{AccountID: cash.ID, Amount: 1, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1, CurrencyID: usd.ID},
			}},
		},
		{
			name: "missing timestamp",
			txn: transactionDTO{Description: "x", Entries: []entryDTO{
				{AccountID: cash.ID, Amount: 1, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1, CurrencyID: usd.ID},
			}},
		},
		{
			name: "fewer than two entries",
			txn:  transactionDTO{Timestamp: ts, Description: "x", Entries: []entryDTO{{AccountID: cash.ID, Amount: 0, CurrencyID: usd.ID}}},
		},
		{
			name: "entries do not sum to zero",
			txn: transactionDTO{Timestamp: ts, Description: "x", Entries: []entryDTO{
				{AccountID: cash.ID, Amount: 10, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -9, CurrencyID: usd.ID},
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
	rec := do(t, h, http.MethodPost, "/transactions", transactionDTO{
		Timestamp:   time.Now(),
		Description: "Bad reference",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: 10, CurrencyID: usd.ID},
			{AccountID: 999999, Amount: -10, CurrencyID: usd.ID},
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

	rec := do(t, h, http.MethodPost, "/transactions", transactionDTO{
		Timestamp:   time.Now(),
		Description: "Bad currency",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: 10, CurrencyID: 999999},
			{AccountID: revenue.ID, Amount: -10, CurrencyID: 999999},
		},
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateTransaction_AmountTooManyDecimalDigits(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD") // 2 decimal places

	var body map[string]string
	rec := do(t, h, http.MethodPost, "/transactions", transactionDTO{
		Timestamp:   time.Now(),
		Description: "Too precise",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: 10.555, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -10.555, CurrencyID: usd.ID},
		},
	}, &body)
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

	rec := do(t, h, http.MethodPost, "/transactions", transactionDTO{
		Timestamp:   time.Now(),
		Description: "Balanced in both currencies",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: 10, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -10, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 5, CurrencyID: eur.ID},
			{AccountID: revenue.ID, Amount: -5, CurrencyID: eur.ID},
		},
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("balanced multi-currency: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body map[string]string
	rec = do(t, h, http.MethodPost, "/transactions", transactionDTO{
		Timestamp:   time.Now(),
		Description: "USD short by 1",
		Entries: []entryDTO{
			{AccountID: cash.ID, Amount: 10, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -9, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 5, CurrencyID: eur.ID},
			{AccountID: revenue.ID, Amount: -5, CurrencyID: eur.ID},
		},
	}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unbalanced in one currency: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "entry amounts must sum to zero within each currency" {
		t.Fatalf("body = %v", body)
	}
}
