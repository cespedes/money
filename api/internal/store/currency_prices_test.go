package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"money/api/internal/models"
	"money/api/internal/store"
)

func TestCurrencyPriceStore_CreateGetListDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")

	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	created, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.16, AsOf: at,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("Create: expected a non-zero ID")
	}

	got, err := s.CurrencyPrices.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BaseCurrencyID != eur.ID || got.QuoteCurrencyID != usd.ID || got.Rate != 1.16 || !got.AsOf.Equal(at) {
		t.Fatalf("Get: got %+v", got)
	}

	list, err := s.CurrencyPrices.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List: got %+v", list)
	}

	if err := s.CurrencyPrices.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.CurrencyPrices.Get(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestCurrencyPriceStore_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if _, err := s.CurrencyPrices.Get(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get: got %v, want ErrNotFound", err)
	}
	if err := s.CurrencyPrices.Delete(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Delete: got %v, want ErrNotFound", err)
	}
}

func TestCurrencyPriceStore_DistinctCurrenciesRequired(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")

	_, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: eur.ID, Rate: 1, AsOf: time.Now(),
	})
	if !isPgErrorCode(err, "23514") { // check_violation
		t.Fatalf("creating a price against itself: got %v, want check_violation", err)
	}
}

func TestCurrencyPriceStore_DuplicateObservationRejected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.16, AsOf: at,
	}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	_, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.17, AsOf: at,
	})
	if !isPgErrorCode(err, "23505") { // unique_violation
		t.Fatalf("creating a duplicate observation: got %v, want unique_violation", err)
	}
}

func TestCurrencyPriceStore_RateAt_SameCurrency(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")

	rate, err := s.CurrencyPrices.RateAt(ctx, eur.ID, eur.ID, time.Now())
	if err != nil {
		t.Fatalf("RateAt: %v", err)
	}
	if rate != 1 {
		t.Fatalf("RateAt(same currency) = %v, want 1", rate)
	}
}

func TestCurrencyPriceStore_RateAt_NoData(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")

	_, err := s.CurrencyPrices.RateAt(ctx, eur.ID, usd.ID, time.Now())
	if !errors.Is(err, store.ErrNoRate) {
		t.Fatalf("RateAt with no data: got %v, want ErrNoRate", err)
	}
}

func TestCurrencyPriceStore_RateAt_ExactMatch(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.16, AsOf: at,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	rate, err := s.CurrencyPrices.RateAt(ctx, eur.ID, usd.ID, at)
	if err != nil {
		t.Fatalf("RateAt: %v", err)
	}
	if rate != 1.16 {
		t.Fatalf("RateAt = %v, want 1.16", rate)
	}
}

func TestCurrencyPriceStore_RateAt_InverseDirection(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Only EUR->USD is recorded; querying USD in terms of EUR should
	// still work, as the inverse.
	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.25, AsOf: at,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	rate, err := s.CurrencyPrices.RateAt(ctx, usd.ID, eur.ID, at)
	if err != nil {
		t.Fatalf("RateAt: %v", err)
	}
	if want := 1.0 / 1.25; rate != want {
		t.Fatalf("RateAt(inverse) = %v, want %v", rate, want)
	}
}

func TestCurrencyPriceStore_RateAt_LinearInterpolation(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")

	day := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }
	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.01, AsOf: day(1),
	}); err != nil {
		t.Fatalf("create day 1: %v", err)
	}
	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.06, AsOf: day(6),
	}); err != nil {
		t.Fatalf("create day 6: %v", err)
	}

	rate, err := s.CurrencyPrices.RateAt(ctx, eur.ID, usd.ID, day(4))
	if err != nil {
		t.Fatalf("RateAt: %v", err)
	}
	if want := 1.04; abs(rate-want) > 1e-9 {
		t.Fatalf("RateAt(day 4) = %v, want %v", rate, want)
	}
}

func TestCurrencyPriceStore_RateAt_OnlyBeforeOrAfter(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")
	day := func(d int) time.Time { return time.Date(2026, 1, d, 0, 0, 0, 0, time.UTC) }

	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.10, AsOf: day(1),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Only a "before" observation exists: it's used as-is for a later
	// query, and again for an earlier one (only an "after" observation).
	rate, err := s.CurrencyPrices.RateAt(ctx, eur.ID, usd.ID, day(10))
	if err != nil {
		t.Fatalf("RateAt after the only observation: %v", err)
	}
	if rate != 1.10 {
		t.Fatalf("RateAt after the only observation = %v, want 1.10", rate)
	}
	rate, err = s.CurrencyPrices.RateAt(ctx, eur.ID, usd.ID, day(0))
	if err != nil {
		t.Fatalf("RateAt before the only observation: %v", err)
	}
	if rate != 1.10 {
		t.Fatalf("RateAt before the only observation = %v, want 1.10", rate)
	}
}

func TestCurrencyPriceStore_RateAt_ChainedThroughIntermediate(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	eur := createTestCurrency(t, s, "EUR")
	usd := createTestCurrency(t, s, "USD")
	gbp := createTestCurrency(t, s, "GBP")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// No direct EUR/GBP observation: only EUR/USD and USD/GBP.
	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.16, AsOf: at,
	}); err != nil {
		t.Fatalf("create EUR/USD: %v", err)
	}
	if _, err := s.CurrencyPrices.Create(ctx, models.CurrencyPrice{
		BaseCurrencyID: usd.ID, QuoteCurrencyID: gbp.ID, Rate: 0.74, AsOf: at,
	}); err != nil {
		t.Fatalf("create USD/GBP: %v", err)
	}

	rate, err := s.CurrencyPrices.RateAt(ctx, eur.ID, gbp.ID, at)
	if err != nil {
		t.Fatalf("RateAt: %v", err)
	}
	if want := 1.16 * 0.74; abs(rate-want) > 1e-9 {
		t.Fatalf("RateAt(EUR, GBP) = %v, want %v", rate, want)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
