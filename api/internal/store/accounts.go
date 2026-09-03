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
	rows, err := s.pool.Query(ctx, `SELECT id, name, code, parent_id, position FROM accounts ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []models.Account
	for rows.Next() {
		var a models.Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Code, &a.ParentID, &a.Position); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}

	if err := s.attachBalances(ctx, accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func (s *AccountStore) Get(ctx context.Context, id int64) (models.Account, error) {
	var a models.Account
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, code, parent_id, position FROM accounts WHERE id = $1`, id,
	).Scan(&a.ID, &a.Name, &a.Code, &a.ParentID, &a.Position)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Account{}, ErrNotFound
	}
	if err != nil {
		return models.Account{}, fmt.Errorf("query account: %w", err)
	}

	accounts := []models.Account{a}
	if err := s.attachBalances(ctx, accounts); err != nil {
		return models.Account{}, err
	}
	return accounts[0], nil
}

// attachBalances fills in each account's Balances: its own transaction
// entries (not any child accounts'), summed per currency. An account with
// no entries in a given currency has no entry for it, rather than a zero
// one — so accounts with no entries at all get a nil slice.
func (s *AccountStore) attachBalances(ctx context.Context, accounts []models.Account) error {
	if len(accounts) == 0 {
		return nil
	}
	ids := make([]int64, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}

	rows, err := s.pool.Query(ctx,
		`SELECT account_id, currency_id, SUM(amount)
		 FROM transaction_entries
		 WHERE account_id = ANY($1)
		 GROUP BY account_id, currency_id
		 ORDER BY account_id, currency_id`, ids)
	if err != nil {
		return fmt.Errorf("query balances: %w", err)
	}
	defer rows.Close()

	balances := make(map[int64][]models.CurrencyAmount)
	for rows.Next() {
		var accountID int64
		var ca models.CurrencyAmount
		if err := rows.Scan(&accountID, &ca.CurrencyID, &ca.Amount); err != nil {
			return fmt.Errorf("scan balance: %w", err)
		}
		balances[accountID] = append(balances[accountID], ca)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate balances: %w", err)
	}

	for i := range accounts {
		accounts[i].Balances = balances[accounts[i].ID]
	}
	return nil
}

// Ledger returns every transaction that has an entry on this account, in
// timestamp order, along with this account's own amount in that
// transaction and currency (its entries summed, in the unlikely case
// there's more than one), and its running balance in that same currency
// through that point. A transaction posting to this account in more than
// one currency contributes one row per currency. It returns ErrNotFound
// if the account doesn't exist.
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
		SELECT t.id, t."timestamp", t.description, te.currency_id, SUM(te.amount),
			SUM(SUM(te.amount)) OVER (PARTITION BY te.currency_id ORDER BY t."timestamp", t.id)
		FROM transaction_entries te
		JOIN transactions t ON t.id = te.transaction_id
		WHERE te.account_id = $1
		GROUP BY t.id, t."timestamp", t.description, te.currency_id
		ORDER BY t."timestamp", t.id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("query ledger: %w", err)
	}
	defer rows.Close()

	var entries []models.LedgerEntry
	for rows.Next() {
		var e models.LedgerEntry
		if err := rows.Scan(&e.TransactionID, &e.Timestamp, &e.Description, &e.CurrencyID, &e.Amount, &e.Balance); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger: %w", err)
	}
	return entries, nil
}

// Create inserts a, assigning it the next position after its
// highest-positioned sibling (or 0 if it has none) — new accounts start
// out last among their siblings; a.Position is ignored.
func (s *AccountStore) Create(ctx context.Context, a models.Account) (models.Account, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO accounts (name, code, parent_id, position)
		 VALUES ($1, $2, $3, COALESCE(
		     (SELECT MAX(position) + 1 FROM accounts WHERE parent_id IS NOT DISTINCT FROM $3), 0))
		 RETURNING id, position`,
		a.Name, a.Code, a.ParentID,
	).Scan(&a.ID, &a.Position)
	if err != nil {
		return models.Account{}, fmt.Errorf("insert account: %w", err)
	}
	return a, nil
}

// Update leaves a.Position untouched in the database regardless of what
// it's set to — position only ever changes via Move, so that reordering
// can't be lost by an unrelated field edit that doesn't happen to know
// the account's current position. Returns ErrCycle instead of writing
// anything if a.ParentID would make a its own ancestor.
func (s *AccountStore) Update(ctx context.Context, a models.Account) (models.Account, error) {
	cyclic, err := s.wouldCreateCycle(ctx, a.ID, a.ParentID)
	if err != nil {
		return models.Account{}, err
	}
	if cyclic {
		return models.Account{}, ErrCycle
	}

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

// wouldCreateCycle reports whether setting id's parent to parentID would
// make id its own ancestor — directly (parentID == id) or through some
// chain of parents (parentID is a descendant of id). It walks up from
// parentID through the existing parent_id chain, which is only possible
// if id turns up among its ancestors, since id can't be its own
// descendant's descendant without already being an ancestor of parentID.
func (s *AccountStore) wouldCreateCycle(ctx context.Context, id int64, parentID *int64) (bool, error) {
	if parentID == nil {
		return false, nil
	}
	if *parentID == id {
		return true, nil
	}

	rows, err := s.pool.Query(ctx, `SELECT id, parent_id FROM accounts`)
	if err != nil {
		return false, fmt.Errorf("query accounts: %w", err)
	}
	parentOf := make(map[int64]*int64)
	for rows.Next() {
		var accountID int64
		var pid *int64
		if err := rows.Scan(&accountID, &pid); err != nil {
			rows.Close()
			return false, fmt.Errorf("scan account: %w", err)
		}
		parentOf[accountID] = pid
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate accounts: %w", err)
	}
	rows.Close()

	current := parentID
	visited := make(map[int64]bool)
	for current != nil {
		if *current == id {
			return true, nil
		}
		if visited[*current] {
			break // a pre-existing cycle elsewhere in the data; don't loop forever
		}
		visited[*current] = true
		current = parentOf[*current]
	}
	return false, nil
}

// MoveUp and MoveDown are the two directions Move accepts.
const (
	MoveUp   = "up"
	MoveDown = "down"
)

// Move swaps id's position with whichever sibling (another account with
// the same parent_id, including other roots when parent_id is NULL) is
// immediately before it (MoveUp) or after it (MoveDown) in position
// order. It's a no-op, not an error, if id is already first/last among
// its siblings. Returns ErrNotFound if id doesn't exist.
func (s *AccountStore) Move(ctx context.Context, id int64, direction string) error {
	var parentID *int64
	err := s.pool.QueryRow(ctx, `SELECT parent_id FROM accounts WHERE id = $1`, id).Scan(&parentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("query account: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, position FROM accounts WHERE parent_id IS NOT DISTINCT FROM $1 ORDER BY position, id`,
		parentID)
	if err != nil {
		return fmt.Errorf("query siblings: %w", err)
	}
	type sibling struct {
		id       int64
		position int64
	}
	var siblings []sibling
	for rows.Next() {
		var sib sibling
		if err := rows.Scan(&sib.id, &sib.position); err != nil {
			rows.Close()
			return fmt.Errorf("scan sibling: %w", err)
		}
		siblings = append(siblings, sib)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate siblings: %w", err)
	}
	rows.Close()

	index := -1
	for i, sib := range siblings {
		if sib.id == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ErrNotFound
	}

	swapWith := index - 1
	if direction == MoveDown {
		swapWith = index + 1
	}
	if swapWith < 0 || swapWith >= len(siblings) {
		return nil // already first/last: no-op
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	a, b := siblings[index], siblings[swapWith]
	if _, err := tx.Exec(ctx, `UPDATE accounts SET position = $1 WHERE id = $2`, b.position, a.id); err != nil {
		return fmt.Errorf("update position: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE accounts SET position = $1 WHERE id = $2`, a.position, b.id); err != nil {
		return fmt.Errorf("update position: %w", err)
	}
	return tx.Commit(ctx)
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
