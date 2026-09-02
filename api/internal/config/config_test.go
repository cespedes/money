package config

import "testing"

func TestLoad_Defaults(t *testing.T) {
	cfg := Load()
	if cfg.Addr != ":30730" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":30730")
	}
	if cfg.DatabaseURL != "postgres://money:money@localhost:30731/money?sslmode=disable" {
		t.Errorf("DatabaseURL = %q, want the default connection string", cfg.DatabaseURL)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("ADDR", ":8080")
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/money")

	cfg := Load()
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, ":8080")
	}
	if cfg.DatabaseURL != "postgres://user:pass@db:5432/money" {
		t.Errorf("DatabaseURL = %q, want the overridden value", cfg.DatabaseURL)
	}
}
