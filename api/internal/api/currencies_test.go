package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"money/api/internal/models"
)

func TestCurrenciesCRUD(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, http.MethodPost, "/currencies", map[string]string{}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create without name: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	isin := "US0000000001"
	var created models.Currency
	rec = do(t, h, http.MethodPost, "/currencies", models.Currency{
		Name:               "US Dollar",
		SymbolPosition:     "before",
		SymbolSpace:        false,
		ThousandsSeparator: ",",
		DecimalSeparator:   ".",
		DecimalPlaces:      2,
		ISIN:               &isin,
	}, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if created.ID == 0 || created.Name != "US Dollar" || created.ISIN == nil || *created.ISIN != isin {
		t.Fatalf("create: got %+v", created)
	}

	var got models.Currency
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/currencies/%d", created.ID), nil, &got)
	if rec.Code != http.StatusOK || got.ID != created.ID {
		t.Fatalf("get: status=%d got=%+v", rec.Code, got)
	}

	rec = do(t, h, http.MethodGet, "/currencies/999999", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var list []models.Currency
	rec = do(t, h, http.MethodGet, "/currencies", nil, &list)
	if rec.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: status=%d got=%+v", rec.Code, list)
	}

	var updated models.Currency
	created.Name = "United States Dollar"
	rec = do(t, h, http.MethodPut, fmt.Sprintf("/currencies/%d", created.ID), created, &updated)
	if rec.Code != http.StatusOK || updated.Name != "United States Dollar" {
		t.Fatalf("update: status=%d got=%+v", rec.Code, updated)
	}

	rec = do(t, h, http.MethodPut, "/currencies/999999", created, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("update missing: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/currencies/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec = do(t, h, http.MethodGet, fmt.Sprintf("/currencies/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateCurrency_InvalidSymbolPosition(t *testing.T) {
	h := newTestHandler(t)

	var body map[string]string
	rec := do(t, h, http.MethodPost, "/currencies", models.Currency{Name: "USD", SymbolPosition: "middle"}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != `symbol_position must be "before" or "after"` {
		t.Fatalf("body = %v", body)
	}
}

func TestCreateCurrency_NegativeDecimalPlaces(t *testing.T) {
	h := newTestHandler(t)

	rec := do(t, h, http.MethodPost, "/currencies", models.Currency{Name: "USD", SymbolPosition: "before", DecimalPlaces: -1}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateCurrency_DuplicateName(t *testing.T) {
	h := newTestHandler(t)
	createTestCurrency(t, h, "USD")

	var body map[string]string
	rec := do(t, h, http.MethodPost, "/currencies", models.Currency{Name: "USD", SymbolPosition: "before"}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "currency name already in use" {
		t.Fatalf("body = %v", body)
	}
}

func TestDeleteCurrency_ReferencedByEntry(t *testing.T) {
	h := newTestHandler(t)
	cash, revenue := createTwoAccountsHTTP(t, h)
	usd := createTestCurrency(t, h, "USD")

	rec := do(t, h, http.MethodPost, "/transactions", models.Transaction{
		Timestamp:   time.Now(),
		Description: "Invoice #1",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		},
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create transaction: status = %d", rec.Code)
	}

	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/currencies/%d", usd.ID), nil, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete referenced currency: status = %d, want %d", rec.Code, http.StatusConflict)
	}
}
