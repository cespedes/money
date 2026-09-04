package api_test

import "time"

// These mirror the API's wire-format DTOs (entryJSON etc., unexported in
// package api) for use from this external test package: Amount (and
// Balance) are decimal numbers in the currency's own units, not integer
// counts of minor units the way models.Entry etc. represent them
// internally. float64 is precise enough for the small, simple amounts
// used in these tests; production code instead works on exact decimal
// digit strings (see decimalToMinor/minorToDecimal) to avoid any
// floating-point risk on arbitrary user input.
type entryDTO struct {
	AccountID  int64   `json:"account_id"`
	Amount     float64 `json:"amount"`
	CurrencyID int64   `json:"currency_id"`
}

type transactionDTO struct {
	ID          int64      `json:"id"`
	Timestamp   time.Time  `json:"timestamp"`
	Description string     `json:"description"`
	Entries     []entryDTO `json:"entries"`
}

type currencyAmountDTO struct {
	CurrencyID int64   `json:"currency_id"`
	Amount     float64 `json:"amount"`
}

type accountDTO struct {
	ID                int64               `json:"id"`
	Name              string              `json:"name"`
	Code              *string             `json:"code,omitempty"`
	ParentID          *int64              `json:"parent_id,omitempty"`
	Position          int64               `json:"position"`
	Balances          []currencyAmountDTO `json:"balances"`
	LastTransactionAt *time.Time          `json:"last_transaction_at,omitempty"`
}

type ledgerEntryDTO struct {
	TransactionID int64     `json:"transaction_id"`
	Timestamp     time.Time `json:"timestamp"`
	Description   string    `json:"description"`
	CurrencyID    int64     `json:"currency_id"`
	Amount        float64   `json:"amount"`
	Balance       float64   `json:"balance"`
}

// balanceFor looks up an account's balance in a specific currency from
// its Balances slice, or (0, false) if it has no entries in it.
func balanceFor(balances []currencyAmountDTO, currencyID int64) (float64, bool) {
	for _, b := range balances {
		if b.CurrencyID == currencyID {
			return b.Amount, true
		}
	}
	return 0, false
}
