package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"money/api/internal/models"
	"money/api/internal/store"
)

func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	transactions, err := h.store.Transactions.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toTransactionsJSON(transactions, decimalPlaces))
}

func (h *Handler) getTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transaction id")
		return
	}
	transaction, err := h.store.Transactions.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toTransactionJSON(transaction, decimalPlaces))
}

// parseTransactionJSON decodes and validates a transaction request body
// shared by createTransaction and updateTransaction, converting its
// entries' decimal amounts to minor units via decimalPlaces (see
// fromEntryJSON). On invalid input it returns the message to send back
// as a 400 response instead of a models.Transaction.
func parseTransactionJSON(r *http.Request, decimalPlaces map[int64]int) (models.Transaction, string) {
	var body transactionJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return models.Transaction{}, "invalid request body"
	}
	if strings.TrimSpace(body.Description) == "" {
		return models.Transaction{}, "description is required"
	}
	if len(body.Entries) < 2 {
		return models.Transaction{}, "a transaction needs at least two entries"
	}
	if body.Timestamp.IsZero() {
		return models.Transaction{}, "timestamp is required"
	}

	entries := make([]models.Entry, len(body.Entries))
	for i, e := range body.Entries {
		entry, err := fromEntryJSON(e, decimalPlaces)
		if err != nil {
			return models.Transaction{}, err.Error()
		}
		entries[i] = entry
	}
	return models.Transaction{Description: body.Description, Timestamp: body.Timestamp, Entries: entries}, ""
}

func (h *Handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	t, errMsg := parseTransactionJSON(r, decimalPlaces)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}

	created, err := h.store.Transactions.Create(r.Context(), t)
	if errors.Is(err, store.ErrUnbalanced) {
		writeError(w, http.StatusBadRequest, "entry amounts must sum to zero within each currency")
		return
	}
	if err != nil {
		if isForeignKeyViolation(err) {
			writeError(w, http.StatusBadRequest, "one or more entries reference an account or currency that does not exist")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toTransactionJSON(created, decimalPlaces))
}

func (h *Handler) updateTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transaction id")
		return
	}
	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	t, errMsg := parseTransactionJSON(r, decimalPlaces)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	t.ID = id

	updated, err := h.store.Transactions.Update(r.Context(), t)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	if errors.Is(err, store.ErrUnbalanced) {
		writeError(w, http.StatusBadRequest, "entry amounts must sum to zero within each currency")
		return
	}
	if err != nil {
		if isForeignKeyViolation(err) {
			writeError(w, http.StatusBadRequest, "one or more entries reference an account or currency that does not exist")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toTransactionJSON(updated, decimalPlaces))
}

func (h *Handler) deleteTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid transaction id")
		return
	}
	err = h.store.Transactions.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
