package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"money/api/internal/models"
)

type TransactionStore struct {
	pool *pgxpool.Pool
}

func NewTransactionStore(pool *pgxpool.Pool) *TransactionStore {
	return &TransactionStore{pool: pool}
}

func (s *TransactionStore) List(ctx context.Context) ([]models.Transaction, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, "timestamp", description FROM transactions ORDER BY "timestamp" DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query transactions: %w", err)
	}
	defer rows.Close()

	var transactions []models.Transaction
	for rows.Next() {
		var t models.Transaction
		if err := rows.Scan(&t.ID, &t.Timestamp, &t.Description); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		transactions = append(transactions, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}

	for i := range transactions {
		entries, err := s.entriesFor(ctx, s.pool, transactions[i].ID)
		if err != nil {
			return nil, err
		}
		transactions[i].Entries = entries
	}
	return transactions, nil
}

func (s *TransactionStore) Get(ctx context.Context, id int64) (models.Transaction, error) {
	var t models.Transaction
	err := s.pool.QueryRow(ctx,
		`SELECT id, "timestamp", description FROM transactions WHERE id = $1`, id,
	).Scan(&t.ID, &t.Timestamp, &t.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Transaction{}, ErrNotFound
	}
	if err != nil {
		return models.Transaction{}, fmt.Errorf("query transaction: %w", err)
	}

	entries, err := s.entriesFor(ctx, s.pool, t.ID)
	if err != nil {
		return models.Transaction{}, err
	}
	t.Entries = entries
	return t, nil
}

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *TransactionStore) entriesFor(ctx context.Context, q querier, transactionID int64) ([]models.Entry, error) {
	rows, err := q.Query(ctx,
		`SELECT account_id, amount, currency_id FROM transaction_entries WHERE transaction_id = $1 ORDER BY id`,
		transactionID)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()

	var entries []models.Entry
	for rows.Next() {
		var e models.Entry
		if err := rows.Scan(&e.AccountID, &e.Amount, &e.CurrencyID); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entries: %w", err)
	}
	return entries, nil
}

// Create inserts a transaction and its entries atomically. It rejects
// transactions whose entries do not sum to zero within each currency
// before touching the database (amounts in different currencies are
// never summed together); the database also enforces the same invariant
// as a safety net (see db/schema.sql).
func (s *TransactionStore) Create(ctx context.Context, t models.Transaction) (models.Transaction, error) {
	sums := make(map[int64]int64, len(t.Entries))
	for _, e := range t.Entries {
		sums[e.CurrencyID] += e.Amount
	}
	for _, sum := range sums {
		if sum != 0 {
			return models.Transaction{}, ErrUnbalanced
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Transaction{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`INSERT INTO transactions ("timestamp", description) VALUES ($1, $2) RETURNING id`,
		t.Timestamp, t.Description,
	).Scan(&t.ID)
	if err != nil {
		return models.Transaction{}, fmt.Errorf("insert transaction: %w", err)
	}

	batch := &pgx.Batch{}
	for _, e := range t.Entries {
		batch.Queue(
			`INSERT INTO transaction_entries (transaction_id, account_id, amount, currency_id) VALUES ($1, $2, $3, $4)`,
			t.ID, e.AccountID, e.Amount, e.CurrencyID)
	}
	br := tx.SendBatch(ctx, batch)
	for range t.Entries {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return models.Transaction{}, fmt.Errorf("insert entry: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return models.Transaction{}, fmt.Errorf("close batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Transaction{}, fmt.Errorf("commit tx: %w", err)
	}
	return t, nil
}

func (s *TransactionStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM transactions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete transaction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
