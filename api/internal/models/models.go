// Package models defines the domain types shared by the store and API
// layers.
package models

import "time"

// Account is a node in the (optionally hierarchical) chart of accounts.
type Account struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Code     *string `json:"code,omitempty"`
	ParentID *int64  `json:"parent_id,omitempty"`
	// Balances is this account's own transaction entries (not including
	// any child accounts'), summed per currency, as returned by List and
	// Get. An account with no entries in a given currency has no entry
	// for it here, rather than a zero one.
	Balances []CurrencyAmount `json:"balances"`
}

// CurrencyAmount is an amount posted in a specific currency.
type CurrencyAmount struct {
	CurrencyID int64 `json:"currency_id"`
	Amount     int64 `json:"amount"`
}

// Currency (or, more generally, commodity) is a unit that transaction
// entries can be posted in, along with how amounts in it should be
// formatted for display. Amounts are stored as an integer number of the
// currency's minor unit (e.g. cents for a currency with DecimalPlaces of
// 2), to avoid floating point rounding errors; DecimalPlaces governs both
// how they're stored and how many decimal digits to render.
type Currency struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// SymbolPosition is "before" or "after" the amount.
	SymbolPosition string `json:"symbol_position"`
	// SymbolSpace is whether to put a space between the name and the
	// amount.
	SymbolSpace bool `json:"symbol_space"`
	// ThousandsSeparator separates groups of thousands, or "" for none.
	ThousandsSeparator string `json:"thousands_separator"`
	DecimalSeparator   string `json:"decimal_separator"`
	DecimalPlaces      int    `json:"decimal_places"`
	// ISIN is this currency's International Securities Identification
	// Number, if it has one.
	ISIN *string `json:"isin,omitempty"`
}

// Entry is one leg of a transaction: a signed amount, in a specific
// currency, posted to an account. A positive amount is a debit, a
// negative amount is a credit.
type Entry struct {
	AccountID  int64 `json:"account_id"`
	Amount     int64 `json:"amount"`
	CurrencyID int64 `json:"currency_id"`
}

// Transaction is a timestamped, described group of entries whose amounts
// must sum to zero within each currency.
type Transaction struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	Entries     []Entry   `json:"entries"`
}

// LedgerEntry is one transaction's effect on a specific account, in a
// specific currency: Amount is that account's own amount in that
// currency within the transaction (its entries summed, in the unlikely
// case there's more than one), and Balance is the account's running
// balance in that same currency through that point, in timestamp order.
type LedgerEntry struct {
	TransactionID int64     `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
	Description   string    `json:"description"`
	CurrencyID    int64     `json:"currency_id"`
	Amount        int64     `json:"amount"`
	Balance       int64     `json:"balance"`
}
