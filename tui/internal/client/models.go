// Package client is a REST client for the accounting API.
package client

import (
	"strconv"
	"strings"
	"time"
)

// Account mirrors the API's account representation. Code and ParentID are
// optional, matching the API's chart-of-accounts hierarchy. Balances is
// this account's own transaction entries (not any child accounts'),
// summed per currency; an account with no entries in a given currency has
// no entry for it here.
type Account struct {
	ID       int64            `json:"id"`
	Name     string           `json:"name"`
	Code     *string          `json:"code,omitempty"`
	ParentID *int64           `json:"parent_id,omitempty"`
	Balances []CurrencyAmount `json:"balances"`
}

// CurrencyAmount is an amount posted in a specific currency.
type CurrencyAmount struct {
	CurrencyID int64 `json:"currency_id"`
	Amount     int64 `json:"amount"`
}

// Currency (or, more generally, commodity) is a unit that transaction
// entries can be posted in, along with how amounts in it should be
// formatted for display (see Format). Amounts are stored as an integer
// number of the currency's minor unit (e.g. cents for a currency with
// DecimalPlaces of 2), to avoid floating point rounding errors.
type Currency struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	SymbolPosition     string  `json:"symbol_position"` // "before" or "after"
	SymbolSpace        bool    `json:"symbol_space"`
	ThousandsSeparator string  `json:"thousands_separator"`
	DecimalSeparator   string  `json:"decimal_separator"`
	DecimalPlaces      int     `json:"decimal_places"`
	ISIN               *string `json:"isin,omitempty"`
}

// Format renders amount (an integer number of this currency's minor
// unit) as a decimal string with this currency's name attached, per its
// own display configuration.
func (c Currency) Format(amount int64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}

	scale := int64(1)
	for range c.DecimalPlaces {
		scale *= 10
	}
	whole := strconv.FormatInt(amount/scale, 10)
	if c.ThousandsSeparator != "" {
		whole = groupThousands(whole, c.ThousandsSeparator)
	}

	number := sign + whole
	if c.DecimalPlaces > 0 {
		frac := strconv.FormatInt(amount%scale, 10)
		frac = strings.Repeat("0", c.DecimalPlaces-len(frac)) + frac
		number += c.DecimalSeparator + frac
	}

	space := ""
	if c.SymbolSpace {
		space = " "
	}
	if c.SymbolPosition == "before" {
		return c.Name + space + number
	}
	return number + space + c.Name
}

// groupThousands inserts sep every three digits from the right of a
// (non-negative, digits-only) number string, e.g. "1234567" -> "1,234,567".
func groupThousands(digits, sep string) string {
	n := len(digits)
	if n <= 3 {
		return digits
	}
	var b strings.Builder
	lead := n % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
		b.WriteString(sep)
	}
	for i := lead; i < n; i += 3 {
		if i > lead {
			b.WriteString(sep)
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// Entry is one leg of a transaction: a signed amount, in a specific
// currency, posted to an account.
type Entry struct {
	AccountID  int64 `json:"account_id"`
	Amount     int64 `json:"amount"`
	CurrencyID int64 `json:"currency_id"`
}

// Transaction mirrors the API's transaction representation. Entries must
// sum to zero within each currency.
type Transaction struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	Entries     []Entry   `json:"entries"`
}

// LedgerEntry is one transaction's effect on a specific account, in a
// specific currency: Amount is that account's own amount in the
// transaction, and Balance is the account's running balance in that same
// currency through that point.
type LedgerEntry struct {
	TransactionID int64     `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
	Description   string    `json:"description"`
	CurrencyID    int64     `json:"currency_id"`
	Amount        int64     `json:"amount"`
	Balance       int64     `json:"balance"`
}
