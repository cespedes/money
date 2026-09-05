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

func createTestCurrency(t *testing.T, s *store.Store, name string) models.Currency {
	t.Helper()
	c, err := s.Currencies.Create(context.Background(), models.Currency{
		Name:             name,
		SymbolBefore:     true,
		SymbolSpace:      false,
		DecimalSeparator: ".",
		DecimalPlaces:    2,
	})
	if err != nil {
		t.Fatalf("create currency %q: %v", name, err)
	}
	return c
}

// balanceFor looks up an account's balance in a specific currency from
// its Balances slice, or (0, false) if it has no entries in it.
func balanceFor(balances []models.CurrencyAmount, currencyID int64) (int64, bool) {
	for _, b := range balances {
		if b.CurrencyID == currencyID {
			return b.Amount, true
		}
	}
	return 0, false
}

func TestAccountStore_Balance(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")

	// No entries yet: no balance for this currency at all.
	got, err := s.Accounts.Get(ctx, cash.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Balances) != 0 {
		t.Fatalf("balances with no entries = %+v, want none", got.Balances)
	}

	child, err := s.Accounts.Create(ctx, models.Account{Name: "Petty Cash", ParentID: &cash.ID})
	if err != nil {
		t.Fatalf("create child account: %v", err)
	}

	for _, txn := range []models.Transaction{
		{Timestamp: time.Now(), Description: "Invoice #1", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		}},
		{Timestamp: time.Now(), Description: "Invoice #2", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 500, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -500, CurrencyID: usd.ID},
		}},
		{Timestamp: time.Now(), Description: "Move to petty cash", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: -200, CurrencyID: usd.ID}, {AccountID: child.ID, Amount: 200, CurrencyID: usd.ID},
		}},
	} {
		if _, err := s.Transactions.Create(ctx, txn); err != nil {
			t.Fatalf("create %q: %v", txn.Description, err)
		}
	}

	got, err = s.Accounts.Get(ctx, cash.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if bal, ok := balanceFor(got.Balances, usd.ID); !ok || bal != 1300 { // 1000 + 500 - 200
		t.Fatalf("cash USD balance = (%d, %v), want 1300", bal, ok)
	}

	// A parent's balance is only its own entries, not its children's.
	list, err := s.Accounts.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	balances := make(map[int64][]models.CurrencyAmount, len(list))
	for _, a := range list {
		balances[a.ID] = a.Balances
	}
	if bal, ok := balanceFor(balances[cash.ID], usd.ID); !ok || bal != 1300 {
		t.Fatalf("List: cash balance = (%d, %v), want 1300", bal, ok)
	}
	if bal, ok := balanceFor(balances[child.ID], usd.ID); !ok || bal != 200 {
		t.Fatalf("List: petty cash balance = (%d, %v), want 200 (not rolled up into cash)", bal, ok)
	}
	if bal, ok := balanceFor(balances[revenue.ID], usd.ID); !ok || bal != -1500 {
		t.Fatalf("List: revenue balance = (%d, %v), want -1500", bal, ok)
	}
}

func TestAccountStore_BalancePerCurrency(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")

	for _, txn := range []models.Transaction{
		{Timestamp: time.Now(), Description: "USD invoice", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		}},
		{Timestamp: time.Now(), Description: "EUR invoice", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 300, CurrencyID: eur.ID}, {AccountID: revenue.ID, Amount: -300, CurrencyID: eur.ID},
		}},
	} {
		if _, err := s.Transactions.Create(ctx, txn); err != nil {
			t.Fatalf("create %q: %v", txn.Description, err)
		}
	}

	got, err := s.Accounts.Get(ctx, cash.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Balances) != 2 {
		t.Fatalf("balances = %+v, want one entry per currency", got.Balances)
	}
	if bal, ok := balanceFor(got.Balances, usd.ID); !ok || bal != 1000 {
		t.Fatalf("USD balance = (%d, %v), want 1000", bal, ok)
	}
	if bal, ok := balanceFor(got.Balances, eur.ID); !ok || bal != 300 {
		t.Fatalf("EUR balance = (%d, %v), want 300 (not mixed with USD)", bal, ok)
	}
}

func TestAccountStore_Ledger(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")

	if _, err := s.Accounts.Ledger(ctx, 999999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Ledger of a missing account: got %v, want ErrNotFound", err)
	}

	entries, err := s.Accounts.Ledger(ctx, cash.ID)
	if err != nil {
		t.Fatalf("Ledger with no transactions: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Ledger with no transactions = %+v, want empty", entries)
	}

	t1 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 3, 10, 0, 0, 0, time.UTC)

	// Deliberately created out of timestamp order, to prove the ledger is
	// sorted by timestamp rather than creation/insert order.
	for _, txn := range []models.Transaction{
		{Timestamp: t2, Description: "Second", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 500, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -500, CurrencyID: usd.ID},
		}},
		{Timestamp: t1, Description: "First", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		}},
		{Timestamp: t3, Description: "Third", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: -300, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: 300, CurrencyID: usd.ID},
		}},
	} {
		if _, err := s.Transactions.Create(ctx, txn); err != nil {
			t.Fatalf("create %q: %v", txn.Description, err)
		}
	}

	entries, err = s.Accounts.Ledger(ctx, cash.ID)
	if err != nil {
		t.Fatalf("Ledger: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(entries), entries)
	}

	wantDesc := []string{"First", "Second", "Third"}
	wantAmount := []int64{1000, 500, -300}
	wantBalance := []int64{1000, 1500, 1200}
	for i, e := range entries {
		if e.Description != wantDesc[i] || e.Amount != wantAmount[i] || e.Balance != wantBalance[i] || e.CurrencyID != usd.ID {
			t.Errorf("entry %d = %+v, want description=%s amount=%d balance=%d currency=%d",
				i, e, wantDesc[i], wantAmount[i], wantBalance[i], usd.ID)
		}
	}
}

func TestAccountStore_LedgerPerCurrency(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")

	for _, txn := range []models.Transaction{
		{Timestamp: time.Now(), Description: "USD 1", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
		}},
		{Timestamp: time.Now(), Description: "EUR 1", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 300, CurrencyID: eur.ID}, {AccountID: revenue.ID, Amount: -300, CurrencyID: eur.ID},
		}},
		{Timestamp: time.Now(), Description: "USD 2", Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 500, CurrencyID: usd.ID}, {AccountID: revenue.ID, Amount: -500, CurrencyID: usd.ID},
		}},
	} {
		if _, err := s.Transactions.Create(ctx, txn); err != nil {
			t.Fatalf("create %q: %v", txn.Description, err)
		}
	}

	entries, err := s.Accounts.Ledger(ctx, cash.ID)
	if err != nil {
		t.Fatalf("Ledger: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(entries), entries)
	}

	var usdBalances, eurBalances []int64
	for _, e := range entries {
		switch e.CurrencyID {
		case usd.ID:
			usdBalances = append(usdBalances, e.Balance)
		case eur.ID:
			eurBalances = append(eurBalances, e.Balance)
		default:
			t.Errorf("unexpected currency %d in entry %+v", e.CurrencyID, e)
		}
	}
	// The USD running balance must not be perturbed by the EUR entry in
	// between: 1000, then 1500 (not 1000+300=1300).
	if want := []int64{1000, 1500}; len(usdBalances) != 2 || usdBalances[0] != want[0] || usdBalances[1] != want[1] {
		t.Errorf("USD running balances = %v, want %v", usdBalances, want)
	}
	if want := []int64{300}; len(eurBalances) != 1 || eurBalances[0] != want[0] {
		t.Errorf("EUR running balances = %v, want %v", eurBalances, want)
	}
}

func TestTransactionStore_CreateGetListDelete(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")

	ts := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	created, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   ts,
		Description: "Invoice #1",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
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
	if got.Entries[0].CurrencyID != usd.ID {
		t.Fatalf("Get: entry currency = %d, want %d", got.Entries[0].CurrencyID, usd.ID)
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
	usd := createTestCurrency(t, s, "USD")

	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Unbalanced",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -900, CurrencyID: usd.ID},
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

// TestTransactionStore_CreateRejectsUnbalancedInOneCurrency proves the
// balance check is per currency, not a single sum across all of a
// transaction's entries: even though the four entries below sum to zero
// overall (1000 - 900 - 100 + 0... ), each currency must independently
// balance, and here USD is short by 100.
func TestTransactionStore_CreateRejectsUnbalancedInOneCurrency(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")

	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Mixed currencies, USD unbalanced",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -900, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 100, CurrencyID: eur.ID},
			{AccountID: revenue.ID, Amount: -100, CurrencyID: eur.ID},
		},
	})
	if !errors.Is(err, store.ErrUnbalanced) {
		t.Fatalf("Create: got %v, want ErrUnbalanced (USD leg is short by 100)", err)
	}
}

// TestTransactionStore_CreateAllowsCurrencyExchange proves the balance
// exception for an implicit currency exchange: exactly two currencies,
// each with a nonzero net of opposite sign, is accepted even though
// neither currency sums to zero on its own.
func TestTransactionStore_CreateAllowsCurrencyExchange(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, _ := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")

	created, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Exchange USD for EUR",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: -1000, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 850, CurrencyID: eur.ID},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.Entries) != 2 {
		t.Fatalf("Create: got %d entries, want 2", len(created.Entries))
	}
}

// TestTransactionStore_CreateRejectsSameSignTwoCurrencies proves the
// exchange exception only applies to opposite-signed nets: two
// currencies with nonzero nets of the *same* sign don't represent an
// exchange (money would appear from nowhere in both), and are still
// rejected.
func TestTransactionStore_CreateRejectsSameSignTwoCurrencies(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, _ := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")

	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Not an exchange",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 850, CurrencyID: eur.ID},
		},
	})
	if !errors.Is(err, store.ErrUnbalanced) {
		t.Fatalf("Create: got %v, want ErrUnbalanced", err)
	}
}

// TestTransactionStore_CreateRejectsThreeCurrencyExchange proves the
// exchange exception is specifically for exactly two currencies: three
// currencies with nonzero nets, even with mixed signs, are still
// rejected.
func TestTransactionStore_CreateRejectsThreeCurrencyExchange(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, _ := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")
	gbp := createTestCurrency(t, s, "GBP")

	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Not a two-currency exchange",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: -1000, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 500, CurrencyID: eur.ID},
			{AccountID: cash.ID, Amount: 400, CurrencyID: gbp.ID},
		},
	})
	if !errors.Is(err, store.ErrUnbalanced) {
		t.Fatalf("Create: got %v, want ErrUnbalanced", err)
	}
}

// TestDatabaseTriggerAllowsCurrencyExchange writes directly to
// transaction_entries, bypassing TransactionStore.Create's own balance
// check, to confirm the database trigger independently accepts the same
// two-currency-opposite-sign exception (see
// TestDatabaseTriggerRejectsUnbalancedEntries for the negative case).
func TestDatabaseTriggerAllowsCurrencyExchange(t *testing.T) {
	ctx := context.Background()
	pool := testutil.NewPool(t)
	s := store.New(pool)
	cash, _ := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	var transactionID int64
	err = tx.QueryRow(ctx,
		`INSERT INTO transactions ("timestamp", description) VALUES (now(), 'Exchange') RETURNING id`,
	).Scan(&transactionID)
	if err != nil {
		t.Fatalf("insert transaction: %v", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO transaction_entries (transaction_id, account_id, amount, currency_id)
		 VALUES ($1, $2, -1000, $3), ($1, $2, 850, $4)`,
		transactionID, cash.ID, usd.ID, eur.ID)
	if err != nil {
		t.Fatalf("insert entries: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit of a two-currency exchange: got %v, want it accepted", err)
	}
}

// TestTransactionStore_CreateAllowsBalancedMultiCurrency is the positive
// counterpart: a transaction with two independently-balanced currencies
// in the same transaction is accepted.
func TestTransactionStore_CreateAllowsBalancedMultiCurrency(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")
	eur := createTestCurrency(t, s, "EUR")

	created, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Mixed currencies, both balanced",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: usd.ID},
			{AccountID: cash.ID, Amount: 100, CurrencyID: eur.ID},
			{AccountID: revenue.ID, Amount: -100, CurrencyID: eur.ID},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.Entries) != 4 {
		t.Fatalf("Create: got %d entries, want 4", len(created.Entries))
	}
}

func TestTransactionStore_CreateRejectsUnknownAccount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, _ := createTwoAccounts(t, s)
	usd := createTestCurrency(t, s, "USD")

	const missingAccountID = 999999
	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Bad reference",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: usd.ID},
			{AccountID: missingAccountID, Amount: -1000, CurrencyID: usd.ID},
		},
	})
	if !isPgErrorCode(err, "23503") { // foreign_key_violation
		t.Fatalf("Create with unknown account: got %v, want foreign_key_violation", err)
	}
}

func TestTransactionStore_CreateRejectsUnknownCurrency(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	cash, revenue := createTwoAccounts(t, s)

	const missingCurrencyID = 999999
	_, err := s.Transactions.Create(ctx, models.Transaction{
		Timestamp:   time.Now(),
		Description: "Bad currency reference",
		Entries: []models.Entry{
			{AccountID: cash.ID, Amount: 1000, CurrencyID: missingCurrencyID},
			{AccountID: revenue.ID, Amount: -1000, CurrencyID: missingCurrencyID},
		},
	})
	if !isPgErrorCode(err, "23503") { // foreign_key_violation
		t.Fatalf("Create with unknown currency: got %v, want foreign_key_violation", err)
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
	usd := createTestCurrency(t, s, "USD")

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
		`INSERT INTO transaction_entries (transaction_id, account_id, amount, currency_id) VALUES ($1, $2, 1000, $4), ($1, $3, -900, $4)`,
		transactionID, cash.ID, revenue.ID, usd.ID)
	if err != nil {
		t.Fatalf("insert entries: %v", err)
	}

	err = tx.Commit(ctx)
	if !isPgErrorCode(err, "P0001") { // raise_exception, from check_transaction_balance()
		t.Fatalf("commit of unbalanced entries: got %v, want the balance trigger to reject it", err)
	}
}
