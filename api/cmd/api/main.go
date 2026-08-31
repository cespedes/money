// Command api runs the accounting HTTP API server.
package main

import (
	"context"
	"log"
	"net/http"

	"money/api/internal/api"
	"money/api/internal/config"
	"money/api/internal/db"
	"money/api/internal/store"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	s := store.New(pool)
	handler := api.NewRouter(s)

	log.Printf("listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
