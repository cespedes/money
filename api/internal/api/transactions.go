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

func (h *Handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	var body transactionJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Description) == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return
	}
	if len(body.Entries) < 2 {
		writeError(w, http.StatusBadRequest, "a transaction needs at least two entries")
		return
	}
	if body.Timestamp.IsZero() {
		writeError(w, http.StatusBadRequest, "timestamp is required")
		return
	}

	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	entries := make([]models.Entry, len(body.Entries))
	for i, e := range body.Entries {
		entry, err := fromEntryJSON(e, decimalPlaces)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		entries[i] = entry
	}
	t := models.Transaction{Description: body.Description, Timestamp: body.Timestamp, Entries: entries}

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
