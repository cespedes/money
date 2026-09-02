package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"money/api/internal/models"
)

type AccountStore struct {
	pool *pgxpool.Pool
}

func NewAccountStore(pool *pgxpool.Pool) *AccountStore {
	return &AccountStore{pool: pool}
}

// accountWithBalanceQuery computes each account's balance as the sum of
// its own transaction entries (not any child accounts'), via a LEFT JOIN
// so that accounts with no entries yet still come back with balance 0.
const accountWithBalanceQuery = `
	SELECT a.id, a.name, a.code, a.parent_id, COALESCE(SUM(te.value), 0)
	FROM accounts a
	LEFT JOIN transaction_entries te ON te.account_id = a.id`

func (s *AccountStore) List(ctx context.Context) ([]models.Account, error) {
	rows, err := s.pool.Query(ctx, accountWithBalanceQuery+` GROUP BY a.id ORDER BY a.id`)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Code, &a.ParentID, &a.Balance); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return accounts, nil
}

func (s *AccountStore) Get(ctx context.Context, id int64) (models.Account, error) {
	var a models.Account
	err := s.pool.QueryRow(ctx,
		accountWithBalanceQuery+` WHERE a.id = $1 GROUP BY a.id`, id,
	).Scan(&a.ID, &a.Name, &a.Code, &a.ParentID, &a.Balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Account{}, ErrNotFound
	}
	if err != nil {
		return models.Account{}, fmt.Errorf("query account: %w", err)
	}
	return a, nil
}

// Ledger returns every transaction that has an entry on this account, in
// timestamp order, along with this account's own value in that
// transaction (its entries summed, in the unlikely case there's more
// than one) and its running balance through that point. It returns
// ErrNotFound if the account doesn't exist.
func (s *AccountStore) Ledger(ctx context.Context, accountID int64) ([]models.LedgerEntry, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)`, accountID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check account exists: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t."timestamp", t.description, SUM(te.value),
			SUM(SUM(te.value)) OVER (ORDER BY t."timestamp", t.id)
		FROM transaction_entries te
		JOIN transactions t ON t.id = te.transaction_id
		WHERE te.account_id = $1
		GROUP BY t.id, t."timestamp", t.description
		ORDER BY t."timestamp", t.id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("query ledger: %w", err)
	}
	defer rows.Close()

	var entries []models.LedgerEntry
	for rows.Next() {
		var e models.LedgerEntry
		if err := rows.Scan(&e.TransactionID, &e.Timestamp, &e.Description, &e.Value, &e.Balance); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger: %w", err)
	}
	return entries, nil
}

func (s *AccountStore) Create(ctx context.Context, a models.Account) (models.Account, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO accounts (name, code, parent_id) VALUES ($1, $2, $3) RETURNING id`,
		a.Name, a.Code, a.ParentID,
	).Scan(&a.ID)
	if err != nil {
		return models.Account{}, fmt.Errorf("insert account: %w", err)
	}
	return a, nil
}

func (s *AccountStore) Update(ctx context.Context, a models.Account) (models.Account, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE accounts SET name = $1, code = $2, parent_id = $3 WHERE id = $4`,
		a.Name, a.Code, a.ParentID, a.ID,
	)
	if err != nil {
		return models.Account{}, fmt.Errorf("update account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.Account{}, ErrNotFound
	}
	return a, nil
}

func (s *AccountStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
