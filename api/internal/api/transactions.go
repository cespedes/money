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
	writeJSON(w, http.StatusOK, transactions)
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
	writeJSON(w, http.StatusOK, transaction)
}

func (h *Handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	var t models.Transaction
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(t.Description) == "" {
		writeError(w, http.StatusBadRequest, "description is required")
		return
	}
	if len(t.Entries) < 2 {
		writeError(w, http.StatusBadRequest, "a transaction needs at least two entries")
		return
	}
	if t.Timestamp.IsZero() {
		writeError(w, http.StatusBadRequest, "timestamp is required")
		return
	}
	t.ID = 0

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
	writeJSON(w, http.StatusCreated, created)
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
