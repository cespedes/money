package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"money/api/internal/models"
	"money/api/internal/store"
)

func TestCurrencyStore_CreateGetListUpdateDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	isin := "US0000000001"
	created, err := s.Currencies.Create(ctx, models.Currency{
		Name:               "US Dollar",
		SymbolPosition:     "before",
		SymbolSpace:        false,
		ThousandsSeparator: ",",
		DecimalSeparator:   ".",
		DecimalPlaces:      2,
		ISIN:               &isin,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("Create: expected a non-zero ID")
	}

	got, err := s.Currencies.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "US Dollar" || got.SymbolPosition != "before" || got.SymbolSpace ||
		got.ThousandsSeparator != "," || got.DecimalSeparator != "." || got.DecimalPlaces != 2 ||
		got.ISIN == nil || *got.ISIN != isin {
		t.Fatalf("Get: got %+v", got)
	}

	second, err := s.Currencies.Create(ctx, models.Currency{
		Name: "Euro", SymbolPosition: "after", SymbolSpace: true, DecimalSeparator: ",", DecimalPlaces: 2,
	})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if second.ISIN != nil {
		t.Fatalf("Create second: ISIN = %v, want nil (not provided)", second.ISIN)
	}

	list, err := s.Currencies.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != created.ID || list[1].ID != second.ID {
		t.Fatalf("List: got %+v, want [%d, %d] in order", list, created.ID, second.ID)
	}

	got.Name = "United States Dollar"
	updated, err := s.Currencies.Update(ctx, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "United States Dollar" {
		t.Fatalf("Update: got name %q", updated.Name)
	}

	if err := s.Currencies.Delete(ctx, second.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Currencies.Get(ctx, second.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestCurrencyStore_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	const missingID = 999999
	if _, err := s.Currencies.Get(ctx, missingID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get: got %v, want ErrNotFound", err)
	}
	if _, err := s.Currencies.Update(ctx, models.Currency{ID: missingID, Name: "X", SymbolPosition: "after"}); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Update: got %v, want ErrNotFound", err)
	}
	if err := s.Currencies.Delete(ctx, missingID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Delete: got %v, want ErrNotFound", err)
	}
}

func TestCurrencyStore_DuplicateNameRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Currencies.Create(ctx, models.Currency{Name: "USD", SymbolPosition: "before"}); err != nil {
		t.Fatalf("create first currency: %v", err)
	}
	_, err := s.Currencies.Create(ctx, models.Currency{Name: "USD", SymbolPosition: "after"})
	if !isPgErrorCode(err, "23505") { // unique_violation
		t.Fatalf("creating a duplicate name: got %v, want unique_violation", err)
	}
}

func TestCurrencyStore_MultipleNilISINsAllowed(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.Currencies.Create(ctx, models.Currency{Name: "USD", SymbolPosition: "before"}); err != nil {
		t.Fatalf("create first currency: %v", err)
	}
	if _, err := s.Currencies.Create(ctx, models.Currency{Name: "EUR", SymbolPosition: "after"}); err != nil {
		t.Fatalf("create second currency with no ISIN: %v", err)
	}
}

func TestCurrencyStore_DeleteReferencedByEntryRestricted(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")

	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Invoice #1",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		},
	})
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	err = s.Currencies.Delete(ctx, usd.ID)
	if !isPgErrorCode(err, "23503") { // foreign_key_violation
		t.Fatalf("deleting a referenced currency: got %v, want foreign_key_violation", err)
	}
}
