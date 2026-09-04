package api

import (
	"encoding/json"
	"testing"
)

func TestDecimalToMinor(t *testing.T) {
	cases := []struct {
		name          string
		n             json.Number
		decimalPlaces int
		want          int64
	}{
		{"whole number, 2 decimal places", "10", 2, 1000},
		{"exact decimal", "10.5", 2, 1050},
		{"full precision", "10.55", 2, 1055},
		{"negative", "-10.5", 2, -1050},
		{"explicit plus sign", "+10.5", 2, 1050},
		{"zero decimal places", "1234", 0, 1234},
		{"fewer decimal digits than allowed", "10.5", 3, 10500},
		{"zero", "0", 2, 0},
		{"leading-dot shorthand", ".5", 2, 50},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decimalToMinor(c.n, c.decimalPlaces)
			if err != nil {
				t.Fatalf("decimalToMinor(%q, %d): %v", c.n, c.decimalPlaces, err)
			}
			if got != c.want {
				t.Errorf("decimalToMinor(%q, %d) = %d, want %d", c.n, c.decimalPlaces, got, c.want)
			}
		})
	}
}

func TestDecimalToMinor_Errors(t *testing.T) {
	cases := []struct {
		name          string
		n             json.Number
		decimalPlaces int
	}{
		{"empty", "", 2},
		{"too many decimal digits", "10.555", 2},
		{"not a number", "abc", 2},
		{"double dot", "10..5", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := decimalToMinor(c.n, c.decimalPlaces); err == nil {
				t.Errorf("decimalToMinor(%q, %d): got no error", c.n, c.decimalPlaces)
			}
		})
	}
}

func TestMinorToDecimal(t *testing.T) {
	cases := []struct {
		name          string
		minor         int64
		decimalPlaces int
		want          json.Number
	}{
		{"whole number: trailing zeros trimmed", 1000, 2, "10"},
		{"exact half", 1050, 2, "10.5"},
		{"full precision", 1055, 2, "10.55"},
		{"negative", -1050, 2, "-10.5"},
		{"zero decimal places", 1234, 0, "1234"},
		{"zero", 0, 2, "0"},
		{"fractional only", 50, 2, "0.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := minorToDecimal(c.minor, c.decimalPlaces)
			if got != c.want {
				t.Errorf("minorToDecimal(%d, %d) = %q, want %q", c.minor, c.decimalPlaces, got, c.want)
			}
		})
	}
}

// TestAmountRoundTrip checks minorToDecimal/decimalToMinor are exact
// inverses across a range of values and decimal_places, with no
// floating point involved anywhere in the round trip.
func TestAmountRoundTrip(t *testing.T) {
	for _, decimalPlaces := range []int{0, 1, 2, 3, 4} {
		for _, minor := range []int64{0, 1, 5, 10, 99, 100, 1234567, -1234567} {
			decimal := minorToDecimal(minor, decimalPlaces)
			got, err := decimalToMinor(decimal, decimalPlaces)
			if err != nil {
				t.Fatalf("decimalPlaces=%d minor=%d: minorToDecimal -> %q, decimalToMinor: %v", decimalPlaces, minor, decimal, err)
			}
			if got != minor {
				t.Errorf("decimalPlaces=%d minor=%d: round trip via %q got %d", decimalPlaces, minor, decimal, got)
			}
		}
	}
}
