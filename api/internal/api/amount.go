package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"money/api/internal/models"
)

// decimalToMinor converts n — a JSON number in a currency's own units,
// e.g. "10.5" — into an integer number of that currency's minor units
// (e.g. 1050 for 2 decimal places). n is decoded as a json.Number rather
// than a float64 (see entryJSON etc.), so this works on its exact
// decimal digits and never goes through floating point.
func decimalToMinor(n json.Number, decimalPlaces int) (int64, error) {
	s := string(n)
	if s == "" {
		return 0, fmt.Errorf("amount is required")
	}

	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg = true
		s = s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}

	whole, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		whole, frac = s[:i], s[i+1:]
	}
	if whole == "" {
		whole = "0"
	}
	if !isDigits(whole) || !isDigits(frac) {
		return 0, fmt.Errorf("%q is not a valid amount", string(n))
	}
	if len(frac) > decimalPlaces {
		return 0, fmt.Errorf("amount has more decimal digits than the currency supports (max %d)", decimalPlaces)
	}
	frac += strings.Repeat("0", decimalPlaces-len(frac))

	wholeVal, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid amount", string(n))
	}
	scale := int64(1)
	for range decimalPlaces {
		scale *= 10
	}
	var fracVal int64
	if frac != "" {
		fracVal, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%q is not a valid amount", string(n))
		}
	}

	amount := wholeVal*scale + fracVal
	if neg {
		amount = -amount
	}
	return amount, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// minorToDecimal is decimalToMinor's inverse: minor (an integer number of
// a currency's minor units) as a json.Number in that currency's own
// units, e.g. 1050 with 2 decimal places -> json.Number("10.50") — built
// from minor's digits directly (no floating point). It always shows
// exactly decimalPlaces decimal digits (e.g. "10.00", not "10"), matching
// how money is conventionally written (10.50 EUR, not 10.5 EUR), rather
// than trimming trailing zeros.
func minorToDecimal(minor int64, decimalPlaces int) json.Number {
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	scale := int64(1)
	for range decimalPlaces {
		scale *= 10
	}
	whole := strconv.FormatInt(minor/scale, 10)
	if decimalPlaces == 0 {
		return json.Number(sign + whole)
	}
	frac := strconv.FormatInt(minor%scale, 10)
	frac = strings.Repeat("0", decimalPlaces-len(frac)) + frac
	return json.Number(sign + whole + "." + frac)
}

// currencyDecimalPlaces fetches every currency's DecimalPlaces, keyed by
// ID, for converting amounts to/from their wire (decimal) representation
// — see decimalToMinor/minorToDecimal.
func (h *Handler) currencyDecimalPlaces(ctx context.Context) (map[int64]int, error) {
	currencies, err := h.store.Currencies.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("load currencies: %w", err)
	}
	m := make(map[int64]int, len(currencies))
	for _, c := range currencies {
		m[c.ID] = c.DecimalPlaces
	}
	return m, nil
}

// entryJSON is the wire representation of models.Entry: Amount is a
// decimal JSON number in the currency's own units (e.g. 10.5), not an
// integer count of minor units.
type entryJSON struct {
	AccountID  int64       `json:"account_id"`
	Amount     json.Number `json:"amount"`
	CurrencyID int64       `json:"currency_id"`
}

func toEntryJSON(e models.Entry, decimalPlaces map[int64]int) entryJSON {
	return entryJSON{
		AccountID:  e.AccountID,
		Amount:     minorToDecimal(e.Amount, decimalPlaces[e.CurrencyID]),
		CurrencyID: e.CurrencyID,
	}
}

func toEntriesJSON(entries []models.Entry, decimalPlaces map[int64]int) []entryJSON {
	if entries == nil {
		return nil
	}
	out := make([]entryJSON, len(entries))
	for i, e := range entries {
		out[i] = toEntryJSON(e, decimalPlaces)
	}
	return out
}

// fromEntryJSON converts e to a models.Entry, looking up its decimal
// amount's minor-unit value via decimalPlaces (see currencyDecimalPlaces)
// — decimalPlaces must contain e.CurrencyID, or this reports it as an
// unknown currency rather than trying to guess a scale for it.
func fromEntryJSON(e entryJSON, decimalPlaces map[int64]int) (models.Entry, error) {
	dp, ok := decimalPlaces[e.CurrencyID]
	if !ok {
		return models.Entry{}, fmt.Errorf("entry for account %d: currency %d does not exist", e.AccountID, e.CurrencyID)
	}
	amount, err := decimalToMinor(e.Amount, dp)
	if err != nil {
		return models.Entry{}, fmt.Errorf("entry for account %d: %w", e.AccountID, err)
	}
	return models.Entry{AccountID: e.AccountID, Amount: amount, CurrencyID: e.CurrencyID}, nil
}

// transactionJSON is the wire representation of models.Transaction, with
// entryJSON entries instead of models.Entry.
type transactionJSON struct {
	ID          int64       `json:"id"`
	Timestamp   time.Time   `json:"timestamp"`
	Description string      `json:"description"`
	Entries     []entryJSON `json:"entries"`
}

func toTransactionJSON(t models.Transaction, decimalPlaces map[int64]int) transactionJSON {
	return transactionJSON{
		ID:          t.ID,
		Timestamp:   t.Timestamp,
		Description: t.Description,
		Entries:     toEntriesJSON(t.Entries, decimalPlaces),
	}
}

func toTransactionsJSON(transactions []models.Transaction, decimalPlaces map[int64]int) []transactionJSON {
	if transactions == nil {
		return nil
	}
	out := make([]transactionJSON, len(transactions))
	for i, t := range transactions {
		out[i] = toTransactionJSON(t, decimalPlaces)
	}
	return out
}

// currencyAmountJSON is the wire representation of models.CurrencyAmount
// (an account balance in one currency): Amount is a decimal JSON number,
// not an integer count of minor units.
type currencyAmountJSON struct {
	CurrencyID int64       `json:"currency_id"`
	Amount     json.Number `json:"amount"`
}

func toCurrencyAmountsJSON(balances []models.CurrencyAmount, decimalPlaces map[int64]int) []currencyAmountJSON {
	if balances == nil {
		return nil
	}
	out := make([]currencyAmountJSON, len(balances))
	for i, b := range balances {
		out[i] = currencyAmountJSON{CurrencyID: b.CurrencyID, Amount: minorToDecimal(b.Amount, decimalPlaces[b.CurrencyID])}
	}
	return out
}

// accountJSON is the wire representation of models.Account, with
// currencyAmountJSON balances instead of models.CurrencyAmount.
type accountJSON struct {
	ID                int64                `json:"id"`
	Name              string               `json:"name"`
	Code              *string              `json:"code,omitempty"`
	ParentID          *int64               `json:"parent_id,omitempty"`
	Position          int64                `json:"position"`
	Balances          []currencyAmountJSON `json:"balances"`
	LastTransactionAt *time.Time           `json:"last_transaction_at,omitempty"`
}

func toAccountJSON(a models.Account, decimalPlaces map[int64]int) accountJSON {
	return accountJSON{
		ID:                a.ID,
		Name:              a.Name,
		Code:              a.Code,
		ParentID:          a.ParentID,
		Position:          a.Position,
		Balances:          toCurrencyAmountsJSON(a.Balances, decimalPlaces),
		LastTransactionAt: a.LastTransactionAt,
	}
}

func toAccountsJSON(accounts []models.Account, decimalPlaces map[int64]int) []accountJSON {
	if accounts == nil {
		return nil
	}
	out := make([]accountJSON, len(accounts))
	for i, a := range accounts {
		out[i] = toAccountJSON(a, decimalPlaces)
	}
	return out
}

// ledgerEntryJSON is the wire representation of models.LedgerEntry: both
// Amount and Balance are decimal JSON numbers, not integer counts of
// minor units.
type ledgerEntryJSON struct {
	TransactionID int64       `json:"transaction_id"`
	Timestamp     time.Time   `json:"timestamp"`
	Description   string      `json:"description"`
	CurrencyID    int64       `json:"currency_id"`
	Amount        json.Number `json:"amount"`
	Balance       json.Number `json:"balance"`
}

func toLedgerEntriesJSON(entries []models.LedgerEntry, decimalPlaces map[int64]int) []ledgerEntryJSON {
	if entries == nil {
		return nil
	}
	out := make([]ledgerEntryJSON, len(entries))
	for i, e := range entries {
		dp := decimalPlaces[e.CurrencyID]
		out[i] = ledgerEntryJSON{
			TransactionID: e.TransactionID,
			Timestamp:     e.Timestamp,
			Description:   e.Description,
			CurrencyID:    e.CurrencyID,
			Amount:        minorToDecimal(e.Amount, dp),
			Balance:       minorToDecimal(e.Balance, dp),
		}
	}
	return out
}
