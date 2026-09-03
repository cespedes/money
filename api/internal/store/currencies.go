package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"money/api/internal/models"
)

type CurrencyStore struct {
	pool *pgxpool.Pool
}

func NewCurrencyStore(pool *pgxpool.Pool) *CurrencyStore {
	return &CurrencyStore{pool: pool}
}

const currencyColumns = `id, name, symbol_before, symbol_space, thousands_separator, decimal_separator, decimal_places, isin`

func scanCurrency(row pgx.Row) (models.Currency, error) {
	var c models.Currency
	err := row.Scan(&c.ID, &c.Name, &c.SymbolBefore, &c.SymbolSpace,
		&c.ThousandsSeparator, &c.DecimalSeparator, &c.DecimalPlaces, &c.ISIN)
	return c, err
}

func (s *CurrencyStore) List(ctx context.Context) ([]models.Currency, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+currencyColumns+` FROM currencies ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query currencies: %w", err)
	}
	defer rows.Close()

	var currencies []models.Currency
	for rows.Next() {
		c, err := scanCurrency(rows)
		if err != nil {
			return nil, fmt.Errorf("scan currency: %w", err)
		}
		currencies = append(currencies, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate currencies: %w", err)
	}
	return currencies, nil
}

func (s *CurrencyStore) Get(ctx context.Context, id int64) (models.Currency, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+currencyColumns+` FROM currencies WHERE id = $1`, id)
	c, err := scanCurrency(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Currency{}, ErrNotFound
	}
	if err != nil {
		return models.Currency{}, fmt.Errorf("query currency: %w", err)
	}
	return c, nil
}

func (s *CurrencyStore) Create(ctx context.Context, c models.Currency) (models.Currency, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO currencies (name, symbol_before, symbol_space, thousands_separator, decimal_separator, decimal_places, isin)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		c.Name, c.SymbolBefore, c.SymbolSpace, c.ThousandsSeparator, c.DecimalSeparator, c.DecimalPlaces, c.ISIN,
	).Scan(&c.ID)
	if err != nil {
		return models.Currency{}, fmt.Errorf("insert currency: %w", err)
	}
	return c, nil
}

func (s *CurrencyStore) Update(ctx context.Context, c models.Currency) (models.Currency, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE currencies
		 SET name = $1, symbol_before = $2, symbol_space = $3, thousands_separator = $4,
		     decimal_separator = $5, decimal_places = $6, isin = $7
		 WHERE id = $8`,
		c.Name, c.SymbolBefore, c.SymbolSpace, c.ThousandsSeparator, c.DecimalSeparator, c.DecimalPlaces, c.ISIN, c.ID,
	)
	if err != nil {
		return models.Currency{}, fmt.Errorf("update currency: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return models.Currency{}, ErrNotFound
	}
	return c, nil
}

func (s *CurrencyStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM currencies WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete currency: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
