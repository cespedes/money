package client_test

import (
	"encoding/json"
	"testing"

	"money/tui/internal/client"
)

func TestCurrency_Format(t *testing.T) {
	usd := client.Currency{
		Name:               "USD",
		SymbolBefore:       true,
		SymbolSpace:        false,
		ThousandsSeparator: ",",
		DecimalSeparator:   ".",
		DecimalPlaces:      2,
	}
	eur := client.Currency{
		Name:               "EUR",
		SymbolBefore:       false,
		SymbolSpace:        true,
		ThousandsSeparator: ".",
		DecimalSeparator:   ",",
		DecimalPlaces:      2,
	}
	jpyLike := client.Currency{ // no decimals, no thousands separator
		Name:          "JPY",
		SymbolBefore:  true,
		SymbolSpace:   true,
		DecimalPlaces: 0,
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

func TestCurrency_ParseAmount(t *testing.T) {
	usd := client.Currency{
		Name:               "USD",
		ThousandsSeparator: ",",
		DecimalSeparator:   ".",
		DecimalPlaces:      2,
	}
	eur := client.Currency{
		Name:               "EUR",
		ThousandsSeparator: ".",
		DecimalSeparator:   ",",
		DecimalPlaces:      2,
	}
	jpyLike := client.Currency{
		Name:          "JPY",
		DecimalPlaces: 0,
	}

	tests := []struct {
		c     client.Currency
		input string
		want  int64
	}{
		{usd, "10", 1000},
		{usd, "10.50", 1050},
		{usd, "-10.50", -1050},
		{usd, "+10.50", 1050},
		{usd, ".5", 50},
		{usd, "1,234.56", 123456},
		{usd, "0", 0},
		{eur, "1.234,56", 123456},
		{eur, "10,5", 1050},
		{jpyLike, "1234", 1234},
		{jpyLike, "-1234", -1234},
	}
	for _, tt := range tests {
		got, err := tt.c.ParseAmount(tt.input)
		if err != nil {
			t.Errorf("%+v.ParseAmount(%q) unexpected error: %v", tt.c, tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%+v.ParseAmount(%q) = %d, want %d", tt.c, tt.input, got, tt.want)
		}
	}
}

func TestCurrency_ParseAmount_Errors(t *testing.T) {
	usd := client.Currency{
		Name:               "USD",
		ThousandsSeparator: ",",
		DecimalSeparator:   ".",
		DecimalPlaces:      2,
	}

	tests := []struct {
		input string
	}{
		{""},
		{"abc"},
		{"10.555"},
		{"10.5.5"},
	}
	for _, tt := range tests {
		if _, err := usd.ParseAmount(tt.input); err == nil {
			t.Errorf("ParseAmount(%q) expected error, got nil", tt.input)
		}
	}
}

func TestCurrency_ToMinorUnits(t *testing.T) {
	usd := client.Currency{DecimalPlaces: 2}
	jpyLike := client.Currency{DecimalPlaces: 0}

	tests := []struct {
		c    client.Currency
		n    json.Number
		want int64
	}{
		{usd, "10", 1000},
		{usd, "10.5", 1050},
		{usd, "-10.5", -1050},
		{usd, "0", 0},
		{jpyLike, "1234", 1234},
	}
	for _, tt := range tests {
		got, err := tt.c.ToMinorUnits(tt.n)
		if err != nil {
			t.Errorf("%+v.ToMinorUnits(%q) unexpected error: %v", tt.c, tt.n, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%+v.ToMinorUnits(%q) = %d, want %d", tt.c, tt.n, got, tt.want)
		}
	}
}

func TestCurrency_FromMinorUnits(t *testing.T) {
	usd := client.Currency{DecimalPlaces: 2}
	jpyLike := client.Currency{DecimalPlaces: 0}

	tests := []struct {
		c     client.Currency
		minor int64
		want  json.Number
	}{
		{usd, 1000, "10"},
		{usd, 1050, "10.5"},
		{usd, -1050, "-10.5"},
		{usd, 0, "0"},
		{usd, 5, "0.05"},
		{jpyLike, 1234, "1234"},
	}
	for _, tt := range tests {
		if got := tt.c.FromMinorUnits(tt.minor); got != tt.want {
			t.Errorf("%+v.FromMinorUnits(%d) = %q, want %q", tt.c, tt.minor, got, tt.want)
		}
	}
}

func TestCurrency_AmountRoundTrip(t *testing.T) {
	for _, decimalPlaces := range []int{0, 1, 2, 3, 4} {
		c := client.Currency{DecimalPlaces: decimalPlaces}
		for _, minor := range []int64{0, 1, 5, 10, 100, 1234, -1234, 999999} {
			n := c.FromMinorUnits(minor)
			got, err := c.ToMinorUnits(n)
			if err != nil {
				t.Errorf("decimalPlaces=%d: ToMinorUnits(FromMinorUnits(%d)=%q) unexpected error: %v", decimalPlaces, minor, n, err)
				continue
			}
			if got != minor {
				t.Errorf("decimalPlaces=%d: ToMinorUnits(FromMinorUnits(%d)=%q) = %d, want %d", decimalPlaces, minor, n, got, minor)
			}
		}
	}
}
