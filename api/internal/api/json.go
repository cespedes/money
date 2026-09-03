package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// pgConstraintMessage maps common PostgreSQL constraint violations to a
// human-readable message, or returns "" if err is not one of them.
func pgConstraintMessage(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	switch pgErr.ConstraintName {
	case "accounts_code_unique":
		return "account code already in use"
	case "accounts_parent_id_fkey":
		return "referenced parent account does not exist"
	case "currencies_name_unique":
		return "currency name already in use"
	case "currencies_isin_unique":
		return "ISIN already in use"
	case "currency_prices_unique_observation":
		return "a rate for this currency pair at this exact instant already exists"
	case "currency_prices_base_currency_id_fkey", "currency_prices_quote_currency_id_fkey":
		return "referenced currency does not exist"
	}
	switch pgErr.Code {
	case "23503": // foreign_key_violation
		return "referenced record does not exist"
	case "23505": // unique_violation
		return "value already in use"
	default:
		return ""
	}
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
