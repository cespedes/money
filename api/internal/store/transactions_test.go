package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"money/api/internal/models"
	"money/api/internal/store"
	"money/api/internal/testutil"
)

func createTwoAccounts(t *testing.T, s *store.Store) (cash, revenue models.Account) {
	t.Helper()
	ctx := context.Background()

	cash, err := s.Accounts.Create(ctx, models.Account{Name: "Cash"})
	if err != nil {
		t.Fatalf("create cash account: %v", err)
	}
	revenue, err = s.Accounts.Create(ctx, models.Account{Name: "Revenue"})
	if err != nil {
		t.Fatalf("create revenue account: %v", err)
	}
	return cash, revenue
}

func TestTransactionStore_CreateGetListDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)

	ts := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	created, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   ts,
		Description: "Invoice #1",
		Entries: []models.Entry{
			{AccountID: cash.ID, Value: 1000},
			{AccountID: revenue.ID, Value: -1000},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("Create: expected a non-zero ID")
	}
	if len(created.Entries) != 2 {
		t.Fatalf("Create: got %d entries back, want 2", len(created.Entries))
	}

	got, err := s.Transactions.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Description != "Invoice #1" || !got.Timestamp.Equal(ts) {
		t.Fatalf("Get: got %+v", got)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("Get: got %d entries, want 2", len(got.Entries))
	}

	list, err := s.Transactions.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || len(list[0].Entries) != 2 {
		t.Fatalf("List: got %+v", list)
	}

	if err := s.Transactions.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Transactions.Get(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
	if list, err := s.Transactions.List(ctx); err != nil || len(list) != 0 {
		t.Fatalf("List after Delete: got (%+v, %v), want empty list", list, err)
	}
}

func TestTransactionStore_CreateRejectsUnbalanced(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)

	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Unbalanced",
		Entries: []models.Entry{
			{AccountID: cash.ID, Value: 1000},
			{AccountID: revenue.ID, Value: -900},
		},
	})
	if !errors.Is(err, store.ErrUnbalanced) {
		t.Fatalf("Create: got %v, want ErrUnbalanced", err)
	}

	list, err := s.Transactions.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List: got %d transactions, want 0 (unbalanced create must not write anything)", len(list))
	}
}

func TestTransactionStore_CreateRejectsUnknownAccount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, _ := createTwoAccounts(t, s)

	const missingAccountID = 999999
	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Bad reference",
		Entries: []models.Entry{
			{AccountID: cash.ID, Value: 1000},
			{AccountID: missingAccountID, Value: -1000},
		},
	})
	if !isPgErrorCode(err, "23503") { // foreign_key_violation
		t.Fatalf("Create with unknown account: got %v, want foreign_key_violation", err)
	}
}

// TestDatabaseTriggerRejectsUnbalancedEntries writes directly to
// transaction_entries, bypassing TransactionStore.Create's own balance
// check, to confirm the deferred constraint trigger in db/schema.sql
// independently enforces the double-entry invariant (see the "defense in
// depth" comment there).
func TestDatabaseTriggerRejectsUnbalancedEntries(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewPool(t)
	s := store.New(pool)
	cash, revenue := createTwoAccounts(t, s)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	var transactionID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO transactions ("timestamp", description) VALUES (now(), 'Bad') RETURNING id`,
	).Scan(&transactionID)
	if err != nil {
		t.Fatalf("insert transaction: %v", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO transaction_entries (transaction_id, account_id, value) VALUES ($1, $2, 1000), ($1, $3, -900)`,
		transactionID, cash.ID, revenue.ID)
	if err != nil {
		t.Fatalf("insert entries: %v", err)
	}

	err = tx.Commit(ctx)
	if !isPgErrorCode(err, "P0001") { // raise_exception, from check_transaction_balance()
		t.Fatalf("commit of unbalanced entries: got %v, want the balance trigger to reject it", err)
	}
}
