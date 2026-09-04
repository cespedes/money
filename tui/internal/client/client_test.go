package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"money/tui/internal/client"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return client.New(srv.URL)
}

func TestListAccounts(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/accounts" {
			t.Errorf("got %s %s, want GET /accounts", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]client.Account{{ID: 1, Name: "Cash"}})
	})

	accounts, err := c.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Name != "Cash" {
		t.Fatalf("ListAccounts = %+v", accounts)
	}
}

func TestCreateAccount(t *testing.T) {
	code := "1000"
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts" {
			t.Errorf("got %s %s, want POST /accounts", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got client.Account
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got.Name != "Cash" || got.Code == nil || *got.Code != "1000" {
			t.Errorf("request body = %+v", got)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Account{ID: 1, Name: got.Name, Code: got.Code})
	})

	created, err := c.CreateAccount(context.Background(), client.Account{Name: "Cash", Code: &code})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("CreateAccount = %+v", created)
	}
}

func TestUpdateAccount(t *testing.T) {
	code := "1000"
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/accounts/3" {
			t.Errorf("got %s %s, want PUT /accounts/3", r.Method, r.URL.Path)
		}
		var got client.Account
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got.Name != "Petty Cash" || got.Code == nil || *got.Code != "1000" {
			t.Errorf("request body = %+v", got)
		}
		json.NewEncoder(w).Encode(client.Account{ID: 3, Name: got.Name, Code: got.Code})
	})

	updated, err := c.UpdateAccount(context.Background(), 3, client.Account{Name: "Petty Cash", Code: &code})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if updated.ID != 3 || updated.Name != "Petty Cash" {
		t.Fatalf("UpdateAccount = %+v", updated)
	}
}

func TestMoveAccount(t *testing.T) {
	var gotBody map[string]string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts/5/move" {
			t.Errorf("got %s %s, want POST /accounts/5/move", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.MoveAccount(context.Background(), 5, client.MoveUp); err != nil {
		t.Fatalf("MoveAccount: %v", err)
	}
	if gotBody["direction"] != "up" {
		t.Fatalf("request body = %v", gotBody)
	}
}

func TestDeleteAccount(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/accounts/42" {
			t.Errorf("got %s %s, want DELETE /accounts/42", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteAccount(context.Background(), 42); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
}

func TestGetAccountLedger(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/accounts/7/transactions" {
			t.Errorf("got %s %s, want GET /accounts/7/transactions", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]client.LedgerEntry{
			{TransactionID: 1, Description: "Invoice #1", Amount: "10", CurrencyID: 5, Balance: "10"},
		})
	})

	entries, err := c.GetAccountLedger(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetAccountLedger: %v", err)
	}
	if len(entries) != 1 || entries[0].Balance != "10" {
		t.Fatalf("GetAccountLedger = %+v", entries)
	}
}

func TestListTransactions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]client.Transaction{{ID: 1, Description: "Invoice #1"}})
	})

	transactions, err := c.ListTransactions(context.Background())
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(transactions) != 1 || transactions[0].Description != "Invoice #1" {
		t.Fatalf("ListTransactions = %+v", transactions)
	}
}

func TestGetTransaction(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transactions/7" {
			t.Errorf("path = %q, want /transactions/7", r.URL.Path)
		}
		json.NewEncoder(w).Encode(client.Transaction{ID: 7, Description: "Invoice #1"})
	})

	got, err := c.GetTransaction(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetTransaction: %v", err)
	}
	if got.ID != 7 {
		t.Fatalf("GetTransaction = %+v", got)
	}
}

func TestCreateTransaction(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var got client.Transaction
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if len(got.Entries) != 2 {
			t.Errorf("request entries = %+v", got.Entries)
		}
		w.WriteHeader(http.StatusCreated)
		got.ID = 1
		json.NewEncoder(w).Encode(got)
	})

	created, err := c.CreateTransaction(context.Background(), client.Transaction{
		Description: "Invoice #1",
		Entries: []client.Entry{
			{AccountID: 1, Amount: "10", CurrencyID: 5},
			{AccountID: 2, Amount: "-10", CurrencyID: 5},
		},
	})
	if err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("CreateTransaction = %+v", created)
	}
}

func TestDeleteTransaction(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/transactions/3" {
			t.Errorf("path = %q, want /transactions/3", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteTransaction(context.Background(), 3); err != nil {
		t.Fatalf("DeleteTransaction: %v", err)
	}
}

func TestDo_APIErrorMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name is required"})
	})

	_, err := c.CreateAccount(context.Background(), client.Account{})
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("CreateAccount error = %v, want it to mention %q", err, "name is required")
	}
}

func TestDo_UnexpectedStatusWithoutErrorBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	})

	_, err := c.ListAccounts(context.Background())
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("ListAccounts error = %v, want it to mention the status code", err)
	}
}

func TestListCurrencies(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/currencies" {
			t.Errorf("got %s %s, want GET /currencies", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode([]client.Currency{{ID: 1, Name: "US Dollar"}})
	})

	currencies, err := c.ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("ListCurrencies: %v", err)
	}
	if len(currencies) != 1 || currencies[0].Name != "US Dollar" {
		t.Fatalf("ListCurrencies = %+v", currencies)
	}
}

func TestCreateCurrency(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/currencies" {
			t.Errorf("got %s %s, want POST /currencies", r.Method, r.URL.Path)
		}
		var got client.Currency
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got.Name != "Euro" {
			t.Errorf("request body = %+v", got)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(client.Currency{ID: 1, Name: got.Name})
	})

	created, err := c.CreateCurrency(context.Background(), client.Currency{Name: "Euro"})
	if err != nil {
		t.Fatalf("CreateCurrency: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("CreateCurrency = %+v", created)
	}
}

func TestUpdateCurrency(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/currencies/9" {
			t.Errorf("got %s %s, want PUT /currencies/9", r.Method, r.URL.Path)
		}
		var got client.Currency
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if got.Name != "United States Dollar" {
			t.Errorf("request body = %+v", got)
		}
		json.NewEncoder(w).Encode(client.Currency{ID: 9, Name: got.Name})
	})

	updated, err := c.UpdateCurrency(context.Background(), 9, client.Currency{Name: "United States Dollar"})
	if err != nil {
		t.Fatalf("UpdateCurrency: %v", err)
	}
	if updated.ID != 9 || updated.Name != "United States Dollar" {
		t.Fatalf("UpdateCurrency = %+v", updated)
	}
}

func TestDeleteCurrency(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/currencies/9" {
			t.Errorf("got %s %s, want DELETE /currencies/9", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DeleteCurrency(context.Background(), 9); err != nil {
		t.Fatalf("DeleteCurrency: %v", err)
	}
}

func TestDo_ConnectionError(t *testing.T) {
	c := client.New("http://127.0.0.1:1") // nothing listens on port 1
	if _, err := c.ListAccounts(context.Background()); err == nil {
		t.Fatal("ListAccounts: expected a connection error, got nil")
	}
}
