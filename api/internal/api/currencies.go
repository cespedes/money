package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"money/api/internal/models"
	"money/api/internal/store"
)

func (h *Handler) listCurrencies(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.store.Currencies.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, currencies)
}

func (h *Handler) getCurrency(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid currency id")
		return
	}
	currency, err := h.store.Currencies.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "currency not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, currency)
}

func (h *Handler) createCurrency(w http.ResponseWriter, r *http.Request) {
	var c models.Currency
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateCurrency(c); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	c.ID = 0

	created, err := h.store.Currencies.Create(r.Context(), c)
	if err != nil {
		if msg := pgConstraintMessage(err); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handler) updateCurrency(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid currency id")
		return
	}
	var c models.Currency
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateCurrency(c); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	c.ID = id

	updated, err := h.store.Currencies.Update(r.Context(), c)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "currency not found")
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
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteCurrency(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid currency id")
		return
	}
	err = h.store.Currencies.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "currency not found")
		return
	}
	if err != nil {
		if isForeignKeyViolation(err) {
			writeError(w, http.StatusConflict, "currency is referenced by transaction entries or currency prices")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func validateCurrency(c models.Currency) string {
	if strings.TrimSpace(c.Name) == "" {
		return "name is required"
	}
	if c.DecimalPlaces < 0 {
		return "decimal_places cannot be negative"
	}
	return ""
}
