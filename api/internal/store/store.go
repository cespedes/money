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

type Store struct {
	Accounts     *AccountStore
	Transactions *TransactionStore
	Currencies   *CurrencyStore
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		Accounts:     NewAccountStore(pool),
		Transactions: NewTransactionStore(pool),
		Currencies:   NewCurrencyStore(pool),
	}
}
