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

func (s *AccountStore) List(ctx context.Context) ([]models.Account, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, code, parent_id FROM accounts ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Code, &a.ParentID); err != nil {
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
		`SELECT id, name, code, parent_id FROM accounts WHERE id = $1`, id,
	).Scan(&a.ID, &a.Name, &a.Code, &a.ParentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Account{}, ErrNotFound
	}
	if err != nil {
		return models.Account{}, fmt.Errorf("query account: %w", err)
	}
	return a, nil
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
