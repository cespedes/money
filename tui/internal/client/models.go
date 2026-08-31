// Package client is a REST client for the accounting API.
package client

import "time"

// Account mirrors the API's account representation. Code and ParentID are
// optional, matching the API's chart-of-accounts hierarchy.
type Account struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Code     *string `json:"code,omitempty"`
	ParentID *int64  `json:"parent_id,omitempty"`
}

// Entry is one leg of a transaction: a signed amount, in minor currency
// units, posted to an account.
type Entry struct {
	AccountID int64 `json:"account_id"`
	Value     int64 `json:"value"`
}

// Transaction mirrors the API's transaction representation. Entries must
// sum to zero.
type Transaction struct {
	ID          int64     `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Description string    `json:"description"`
	Entries     []Entry   `json:"entries"`
}
