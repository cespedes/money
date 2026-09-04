package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"money/api/internal/models"
	"money/api/internal/store"
)

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.store.Accounts.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toAccountsJSON(accounts, decimalPlaces))
}

func (h *Handler) getAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	account, err := h.store.Accounts.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
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
	writeJSON(w, http.StatusOK, toAccountJSON(account, decimalPlaces))
}

func (h *Handler) listAccountLedger(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	entries, err := h.store.Accounts.Ledger(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
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
	writeJSON(w, http.StatusOK, toLedgerEntriesJSON(entries, decimalPlaces))
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var a models.Account
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(a.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	a.ID = 0

	created, err := h.store.Accounts.Create(r.Context(), a)
	if err != nil {
		if msg := pgConstraintMessage(err); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toAccountJSON(created, decimalPlaces))
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	var a models.Account
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(a.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	a.ID = id

	updated, err := h.store.Accounts.Update(r.Context(), a)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if errors.Is(err, store.ErrCycle) {
		writeError(w, http.StatusBadRequest, "an account cannot be its own ancestor")
		return
	}
	if err != nil {
		if msg := pgConstraintMessage(err); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	decimalPlaces, err := h.currencyDecimalPlaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toAccountJSON(updated, decimalPlaces))
}

type moveAccountRequest struct {
	Direction string `json:"direction"`
}

func (h *Handler) moveAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	var req moveAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Direction != store.MoveUp && req.Direction != store.MoveDown {
		writeError(w, http.StatusBadRequest, `direction must be "up" or "down"`)
		return
	}

	if err := h.store.Accounts.Move(r.Context(), id, req.Direction); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "account not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	err = h.store.Accounts.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		if isForeignKeyViolation(err) {
			writeError(w, http.StatusConflict, "account is referenced by other accounts or transactions")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
