// Package models defines the domain types shared by the store and API
// layers.
package models

import "time"

// Account is a node in the (optionally hierarchical) chart of accounts.
// Value is expressed in the minor currency unit of whatever currency the
// deployment uses (e.g. cents), as a signed integer, to avoid floating
// point rounding errors.
type Account struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Code     *string `json:"code,omitempty"`
	ParentID *int64  `json:"parent_id,omitempty"`
	// Balance is the sum of this account's own transaction entries (not
	// including any child accounts'), as returned by List and Get.
	Balance int64 `json:"balance"`
}

// Entry is one leg of a transaction: a signed amount posted to an account.
// A positive value is a debit, a negative value is a credit.
type Entry struct {
	AccountID int64 `json:"account_id"`
	Value     int64 `json:"value"`
}

// Transaction is a timestamped, described group of entries whose values
// must sum to zero.
type Transaction struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	Entries     []Entry   `json:"entries"`
}

// LedgerEntry is one transaction's effect on a specific account: Value is
// that account's own entry (or entries, summed) within the transaction,
// and Balance is the account's running balance through that point, in
// timestamp order.
type LedgerEntry struct {
	TransactionID int64     `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
	Description   string    `json:"description"`
	Value         int64     `json:"value"`
	Balance       int64     `json:"balance"`
}
