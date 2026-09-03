package api_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"money/api/internal/models"
)

func TestCurrencyPricesCRUD(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")
	usd := createTestCurrency(t, h, "USD")
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	var created models.CurrencyPrice
	rec := do(t, h, http.MethodPost, "/currency-prices", models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.16, AsOf: at,
	}, &created)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if created.ID == 0 || created.Rate != 1.16 {
		t.Fatalf("create: got %+v", created)
	}

	var got models.CurrencyPrice
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/currency-prices/%d", created.ID), nil, &got)
	if rec.Code != http.StatusOK || got.ID != created.ID {
		t.Fatalf("get: status=%d got=%+v", rec.Code, got)
	}

	rec = do(t, h, http.MethodGet, "/currency-prices/999999", nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var list []models.CurrencyPrice
	rec = do(t, h, http.MethodGet, "/currency-prices", nil, &list)
	if rec.Code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: status=%d got=%+v", rec.Code, list)
	}

	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/currency-prices/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/currency-prices/%d", created.ID), nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateCurrencyPrice_Validation(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")
	usd := createTestCurrency(t, h, "USD")
	at := time.Now()

	cases := []struct {
		name  string
		price models.CurrencyPrice
		want  string
	}{
		{"missing currencies", models.CurrencyPrice{Rate: 1, AsOf: at}, "base_currency_id and quote_currency_id are required"},
		{"same currency", models.CurrencyPrice{BaseCurrencyID: eur.ID, QuoteCurrencyID: eur.ID, Rate: 1, AsOf: at}, "base_currency_id and quote_currency_id must differ"},
		{"zero rate", models.CurrencyPrice{BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 0, AsOf: at}, "rate must be positive"},
		{"negative rate", models.CurrencyPrice{BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: -1, AsOf: at}, "rate must be positive"},
		{"missing as_of", models.CurrencyPrice{BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1}, "as_of is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var body map[string]string
			rec := do(t, h, http.MethodPost, "/currency-prices", c.price, &body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if body["error"] != c.want {
				t.Fatalf("error = %q, want %q", body["error"], c.want)
			}
		})
	}
}

func TestCreateCurrencyPrice_DuplicateObservation(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")
	usd := createTestCurrency(t, h, "USD")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	rec := do(t, h, http.MethodPost, "/currency-prices", models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.16, AsOf: at,
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body map[string]string
	rec = do(t, h, http.MethodPost, "/currency-prices", models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.17, AsOf: at,
	}, &body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("duplicate: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if body["error"] != "a rate for this currency pair at this exact instant already exists" {
		t.Fatalf("duplicate: body = %v", body)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func rateURL(base, quote int64, at string) string {
	v := url.Values{"base": {fmt.Sprint(base)}, "quote": {fmt.Sprint(quote)}}
	if at != "" {
		v.Set("at", at)
	}
	return "/currency-prices/rate?" + v.Encode()
}

func TestGetCurrencyRate(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")
	usd := createTestCurrency(t, h, "USD")
	gbp := createTestCurrency(t, h, "GBP")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	do(t, h, http.MethodPost, "/currency-prices", models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.16, AsOf: at,
	}, nil)
	do(t, h, http.MethodPost, "/currency-prices", models.CurrencyPrice{
		BaseCurrencyID: usd.ID, QuoteCurrencyID: gbp.ID, Rate: 0.74, AsOf: at,
	}, nil)

	var body map[string]any
	rec := do(t, h, http.MethodGet, rateURL(eur.ID, usd.ID, at.Format(time.RFC3339)), nil, &body)
	if rec.Code != http.StatusOK {
		t.Fatalf("direct rate: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := body["rate"].(float64); got != 1.16 {
		t.Fatalf("direct rate = %v, want 1.16", got)
	}

	// Chained through USD, since there's no direct EUR/GBP observation.
	rec = do(t, h, http.MethodGet, rateURL(eur.ID, gbp.ID, at.Format(time.RFC3339)), nil, &body)
	if rec.Code != http.StatusOK {
		t.Fatalf("chained rate: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := body["rate"].(float64), 1.16*0.74; abs(got-want) > 1e-9 {
		t.Fatalf("chained rate = %v, want %v", got, want)
	}
}

func TestGetCurrencyRate_SameCurrency(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")

	var body map[string]any
	rec := do(t, h, http.MethodGet, rateURL(eur.ID, eur.ID, ""), nil, &body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := body["rate"].(float64); got != 1 {
		t.Fatalf("rate = %v, want 1", got)
	}
}

func TestGetCurrencyRate_DefaultsAtToNow(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")
	usd := createTestCurrency(t, h, "USD")

	do(t, h, http.MethodPost, "/currency-prices", models.CurrencyPrice{
		BaseCurrencyID: eur.ID, QuoteCurrencyID: usd.ID, Rate: 1.2, AsOf: time.Now().Add(-time.Hour),
	}, nil)

	var body map[string]any
	rec := do(t, h, http.MethodGet, rateURL(eur.ID, usd.ID, ""), nil, &body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := body["rate"].(float64); got != 1.2 {
		t.Fatalf("rate = %v, want 1.2 (the only, past, observation)", got)
	}
}

func TestGetCurrencyRate_NotFound(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")
	usd := createTestCurrency(t, h, "USD")

	var body map[string]string
	rec := do(t, h, http.MethodGet, rateURL(eur.ID, usd.ID, ""), nil, &body)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if body["error"] != "no exchange rate available between these currencies" {
		t.Fatalf("body = %v", body)
	}
}

func TestGetCurrencyRate_InvalidParams(t *testing.T) {
	h := newTestHandler(t)
	eur := createTestCurrency(t, h, "EUR")
	usd := createTestCurrency(t, h, "USD")

	cases := []string{
		"/currency-prices/rate?quote=" + fmt.Sprint(usd.ID),
		"/currency-prices/rate?base=" + fmt.Sprint(eur.ID),
		"/currency-prices/rate?base=abc&quote=" + fmt.Sprint(usd.ID),
		rateURL(eur.ID, usd.ID, "not-a-timestamp"),
	}
	for _, path := range cases {
		rec := do(t, h, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusBadRequest)
		}
	}
}
