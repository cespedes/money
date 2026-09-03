package client_test

import (
	"testing"

	"money/tui/internal/client"
)

func TestCurrency_Format(t *testing.T) {
	usd := client.Currency{
		Name:               "USD",
		SymbolPosition:     "before",
		SymbolSpace:        false,
		ThousandsSeparator: ",",
		DecimalSeparator:   ".",
		DecimalPlaces:      2,
	}
	eur := client.Currency{
		Name:               "EUR",
		SymbolPosition:     "after",
		SymbolSpace:        true,
		ThousandsSeparator: ".",
		DecimalSeparator:   ",",
		DecimalPlaces:      2,
	}
	jpyLike := client.Currency{ // no decimals, no thousands separator
		Name:           "JPY",
		SymbolPosition: "before",
		SymbolSpace:    true,
		DecimalPlaces:  0,
	}

	tests := []struct {
		c      client.Currency
		amount int64
		want   string
	}{
		{usd, 1000, "USD10.00"},
		{usd, -1000, "USD-10.00"},
		{usd, 0, "USD0.00"},
		{usd, 5, "USD0.05"},
		{usd, 123456789, "USD1,234,567.89"},
		{eur, 1000, "10,00 EUR"},
		{eur, 1234567, "12.345,67 EUR"},
		{jpyLike, 1234, "JPY 1234"},
		{jpyLike, -1234, "JPY -1234"},
	}
	for _, tt := range tests {
		if got := tt.c.Format(tt.amount); got != tt.want {
			t.Errorf("%+v.Format(%d) = %q, want %q", tt.c, tt.amount, got, tt.want)
		}
	}
}
