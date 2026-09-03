// Package store implements persistence for accounts and transactions on
// top of a PostgreSQL connection pool.
package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// ErrUnbalanced is returned when a transaction's entries do not sum to
// zero.
var ErrUnbalanced = errors.New("transaction entries do not sum to zero")

// ErrCycle is returned when an account update's parent_id would make the
// account its own ancestor (directly, or through some chain of parents).
var ErrCycle = errors.New("account cannot be its own ancestor")

// ErrNoRate is returned by CurrencyPriceStore.RateAt when no exchange
// rate can be determined between two currencies — directly, by
// interpolating over time, or by chaining through any intermediate
// currency — from the currency_prices recorded so far.
var ErrNoRate = errors.New("no exchange rate available")

type Store struct {
	Accounts       *AccountStore
	Transactions   *TransactionStore
	Currencies     *CurrencyStore
	CurrencyPrices *CurrencyPriceStore
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		Accounts:       NewAccountStore(pool),
		Transactions:   NewTransactionStore(pool),
		Currencies:     NewCurrencyStore(pool),
		CurrencyPrices: NewCurrencyPriceStore(pool),
	}
}
