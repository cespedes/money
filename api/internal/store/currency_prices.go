package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"money/api/internal/models"
)

type CurrencyPriceStore struct {
	pool *pgxpool.Pool
}

func NewCurrencyPriceStore(pool *pgxpool.Pool) *CurrencyPriceStore {
	return &CurrencyPriceStore{pool: pool}
}

const currencyPriceColumns = `id, base_currency_id, quote_currency_id, rate, as_of`

func scanCurrencyPrice(row pgx.Row) (models.CurrencyPrice, error) {
	var p models.CurrencyPrice
	err := row.Scan(&p.ID, &p.BaseCurrencyID, &p.QuoteCurrencyID, &p.Rate, &p.AsOf)
	return p, err
}

func (s *CurrencyPriceStore) List(ctx context.Context) ([]models.CurrencyPrice, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+currencyPriceColumns+` FROM currency_prices ORDER BY as_of`)
	if err != nil {
		return nil, fmt.Errorf("query currency prices: %w", err)
	}
	defer rows.Close()

	var prices []models.CurrencyPrice
	for rows.Next() {
		p, err := scanCurrencyPrice(rows)
		if err != nil {
			return nil, fmt.Errorf("scan currency price: %w", err)
		}
		prices = append(prices, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate currency prices: %w", err)
	}
	return prices, nil
}

func (s *CurrencyPriceStore) Get(ctx context.Context, id int64) (models.CurrencyPrice, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+currencyPriceColumns+` FROM currency_prices WHERE id = $1`, id)
	p, err := scanCurrencyPrice(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.CurrencyPrice{}, ErrNotFound
	}
	if err != nil {
		return models.CurrencyPrice{}, fmt.Errorf("query currency price: %w", err)
	}
	return p, nil
}

func (s *CurrencyPriceStore) Create(ctx context.Context, p models.CurrencyPrice) (models.CurrencyPrice, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO currency_prices (base_currency_id, quote_currency_id, rate, as_of)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		p.BaseCurrencyID, p.QuoteCurrencyID, p.Rate, p.AsOf,
	).Scan(&p.ID)
	if err != nil {
		return models.CurrencyPrice{}, fmt.Errorf("insert currency price: %w", err)
	}
	return p, nil
}

func (s *CurrencyPriceStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM currency_prices WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete currency price: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RateAt reports how many units of quoteID one unit of baseID was worth
// at the given instant, or ErrNoRate if that can't be determined from
// the currency_prices recorded so far. baseID == quoteID always
// trivially returns 1.
//
// It's a breadth-first search over the graph of currencies connected by
// any currency_prices observation: from baseID, it tries every currency
// with at least one observation against it, taking whichever one first
// reaches quoteID (fewest hops), multiplying each hop's own
// time-interpolated rate (see interpolateRate) along the way. This is
// exactly how cross-rates compose (e.g. EUR/USD * USD/GBP = EUR/GBP), so
// a currency with no direct observations against quoteID can still be
// priced through any chain of intermediate currencies that does connect
// them.
func (s *CurrencyPriceStore) RateAt(ctx context.Context, baseID, quoteID int64, at time.Time) (float64, error) {
	if baseID == quoteID {
		return 1, nil
	}

	type frontierEntry struct {
		currencyID int64
		cumulative float64
	}
	visited := map[int64]bool{baseID: true}
	queue := []frontierEntry{{currencyID: baseID, cumulative: 1}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		prices, err := s.pricesForCurrency(ctx, cur.currencyID)
		if err != nil {
			return 0, err
		}
		series := pairRateSeries(cur.currencyID, prices)

		// Sorted for determinism: which currency BFS reaches quoteID
		// through shouldn't depend on Go's random map iteration order.
		neighborIDs := make([]int64, 0, len(series))
		for id := range series {
			neighborIDs = append(neighborIDs, id)
		}
		sort.Slice(neighborIDs, func(i, j int) bool { return neighborIDs[i] < neighborIDs[j] })

		for _, n := range neighborIDs {
			if visited[n] {
				continue
			}
			rate, ok := interpolateRate(series[n], at)
			if !ok {
				continue // unreachable: series[n] is never empty by construction
			}
			cumulative := cur.cumulative * rate
			if n == quoteID {
				return cumulative, nil
			}
			visited[n] = true
			queue = append(queue, frontierEntry{currencyID: n, cumulative: cumulative})
		}
	}
	return 0, ErrNoRate
}

// pricesForCurrency returns every currency_prices row that has
// currencyID on either side (as base or quote), in as_of order.
func (s *CurrencyPriceStore) pricesForCurrency(ctx context.Context, currencyID int64) ([]models.CurrencyPrice, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+currencyPriceColumns+` FROM currency_prices
		 WHERE base_currency_id = $1 OR quote_currency_id = $1
		 ORDER BY as_of`, currencyID)
	if err != nil {
		return nil, fmt.Errorf("query currency prices: %w", err)
	}
	defer rows.Close()

	var prices []models.CurrencyPrice
	for rows.Next() {
		p, err := scanCurrencyPrice(rows)
		if err != nil {
			return nil, fmt.Errorf("scan currency price: %w", err)
		}
		prices = append(prices, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate currency prices: %w", err)
	}
	return prices, nil
}

// ratePoint is one observation of how many units of some other currency
// one unit of a given currency was worth, at a specific instant.
type ratePoint struct {
	at   time.Time
	rate float64
}

// pairRateSeries groups currencyID's own price rows (see
// pricesForCurrency) by the other currency in each one, converting every
// row into "1 unit of currencyID was worth this many units of the other
// currency" — inverting a row's rate (1/rate) whenever currencyID is the
// row's quote side, since a row only ever records one direction — and
// sorts each currency's series by as_of, ready for interpolateRate.
func pairRateSeries(currencyID int64, prices []models.CurrencyPrice) map[int64][]ratePoint {
	series := make(map[int64][]ratePoint)
	for _, p := range prices {
		switch currencyID {
		case p.BaseCurrencyID:
			series[p.QuoteCurrencyID] = append(series[p.QuoteCurrencyID], ratePoint{at: p.AsOf, rate: p.Rate})
		case p.QuoteCurrencyID:
			series[p.BaseCurrencyID] = append(series[p.BaseCurrencyID], ratePoint{at: p.AsOf, rate: 1 / p.Rate})
		}
	}
	for _, pts := range series {
		sort.Slice(pts, func(i, j int) bool { return pts[i].at.Before(pts[j].at) })
	}
	return series
}

// interpolateRate finds the rate at the given instant from pts: the
// exact observation if one exists at that instant, linear interpolation
// between the nearest observations before and after it if both exist, or
// whichever single one exists if only one side does. ok is false only if
// pts is empty.
func interpolateRate(pts []ratePoint, at time.Time) (rate float64, ok bool) {
	var before, after *ratePoint
	for i := range pts {
		p := &pts[i]
		switch {
		case p.at.Equal(at):
			return p.rate, true
		case p.at.Before(at):
			if before == nil || p.at.After(before.at) {
				before = p
			}
		default:
			if after == nil || p.at.Before(after.at) {
				after = p
			}
		}
	}
	switch {
	case before != nil && after != nil:
		frac := at.Sub(before.at).Seconds() / after.at.Sub(before.at).Seconds()
		return before.rate + (after.rate-before.rate)*frac, true
	case before != nil:
		return before.rate, true
	case after != nil:
		return after.rate, true
	default:
		return 0, false
	}
}
