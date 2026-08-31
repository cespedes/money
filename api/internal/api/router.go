// Package api implements the HTTP layer of the accounting service.
package api

import (
	"log"
	"net/http"
	"time"

	"money/api/internal/store"
)

type Handler struct {
	store *store.Store
}

func NewRouter(s *store.Store) http.Handler {
	h := &Handler{store: s}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /accounts", h.listAccounts)
	mux.HandleFunc("POST /accounts", h.createAccount)
	mux.HandleFunc("GET /accounts/{id}", h.getAccount)
	mux.HandleFunc("PUT /accounts/{id}", h.updateAccount)
	mux.HandleFunc("DELETE /accounts/{id}", h.deleteAccount)

	mux.HandleFunc("GET /transactions", h.listTransactions)
	mux.HandleFunc("POST /transactions", h.createTransaction)
	mux.HandleFunc("GET /transactions/{id}", h.getTransaction)
	mux.HandleFunc("DELETE /transactions/{id}", h.deleteTransaction)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	return logging(mux)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
