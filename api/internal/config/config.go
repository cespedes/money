// Package config reads process configuration from the environment.
package config

import "os"

type Config struct {
	// DatabaseURL is a PostgreSQL connection string, e.g.
	// postgres://user:password@localhost:30731/money?sslmode=disable
	DatabaseURL string
	// Addr is the address the HTTP server listens on, e.g. ":30730".
	Addr string
}

func Load() Config {
	return Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://money:money@localhost:30731/money?sslmode=disable"),
		Addr:        getEnv("ADDR", ":30730"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
