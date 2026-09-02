package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"money/api/internal/db"
	"money/api/internal/testutil"
)

func TestConnect_InvalidURL(t *testing.T) {
	_, err := db.Connect(context.Background(), "not a valid postgres url")
	if err == nil {
		t.Fatal("expected an error for a malformed connection string")
	}
	if !strings.Contains(err.Error(), "create pool") {
		t.Errorf("err = %v, want it to mention pool creation", err)
	}
}

func TestConnect_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Nothing listens on port 1; the connection should be refused quickly.
	_, err := db.Connect(ctx, "postgres://money:money@127.0.0.1:1/money?sslmode=disable")
	if err == nil {
		t.Fatal("expected an error connecting to an unreachable server")
	}
	if !strings.Contains(err.Error(), "ping database") {
		t.Errorf("err = %v, want it to mention the ping failure", err)
	}
}

func TestConnect_Success(t *testing.T) {
	dbURL := testutil.NewDatabaseURL(t)

	pool, err := db.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pool.Close()
}
