package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"money/api/internal/models"
	"money/api/internal/store"
)

func (h *Handler) listCurrencyPrices(w http.ResponseWriter, r *http.Request) {
	prices, err := h.store.CurrencyPrices.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, prices)
}

func (h *Handler) getCurrencyPrice(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid currency price id")
		return
	}
	price, err := h.store.CurrencyPrices.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "currency price not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, price)
}

func (h *Handler) createCurrencyPrice(w http.ResponseWriter, r *http.Request) {
	var p models.CurrencyPrice
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateCurrencyPrice(p); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	p.ID = 0

	created, err := h.store.CurrencyPrices.Create(r.Context(), p)
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

func validateCurrencyPrice(p models.CurrencyPrice) string {
	if p.BaseCurrencyID == 0 || p.QuoteCurrencyID == 0 {
		return "base_currency_id and quote_currency_id are required"
	}
	if p.BaseCurrencyID == p.QuoteCurrencyID {
		return "base_currency_id and quote_currency_id must differ"
	}
	if p.Rate <= 0 {
		return "rate must be positive"
	}
	if p.AsOf.IsZero() {
		return "as_of is required"
	}
	return ""
}

func (h *Handler) deleteCurrencyPrice(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid currency price id")
		return
	}
	err = h.store.CurrencyPrices.Delete(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "currency price not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// getCurrencyRate answers "how many units of quote was one unit of base
// worth at this instant" (defaulting to now), per
// CurrencyPriceStore.RateAt — which may combine several observations
// (interpolating over time, chaining through intermediate currencies) to
// answer a query that doesn't exactly match one that was recorded.
func (h *Handler) getCurrencyRate(w http.ResponseWriter, r *http.Request) {
	baseID, err := strconv.ParseInt(r.URL.Query().Get("base"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or missing base query parameter")
		return
	}
	quoteID, err := strconv.ParseInt(r.URL.Query().Get("quote"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or missing quote query parameter")
		return
	}
	at := time.Now()
	if raw := r.URL.Query().Get("at"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "at must be an RFC3339 timestamp")
			return
		}
		at = parsed
	}

	rate, err := h.store.CurrencyPrices.RateAt(r.Context(), baseID, quoteID, at)
	if errors.Is(err, store.ErrNoRate) {
		writeError(w, http.StatusNotFound, "no exchange rate available between these currencies")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"base_currency_id":  baseID,
		"quote_currency_id": quoteID,
		"rate":              rate,
		"at":                at,
	})
}
