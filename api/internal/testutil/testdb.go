// Package testutil provides a disposable PostgreSQL database for tests
// that exercise the store and API layers against a real database, since
// the double-entry invariant is partly enforced by a database trigger
// (see db/schema.sql) that a mock could not reproduce.
package testutil

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// adminDatabaseURL is the connection string for the PostgreSQL server
// tests run against (started via `make db-up` or `make up`), pointing at
// its default "money" database. Override with TEST_DATABASE_URL, e.g. to
// point at a different host or port.
func adminDatabaseURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://money:money@localhost:30731/money?sslmode=disable"
}

var testDBCounter atomic.Uint64

// nextTestDBName returns a name unique to this test process, so that
// concurrently-running packages (`go test ./...` runs each package's
// tests in its own process, in parallel) each get their own database
// instead of racing over shared tables.
func nextTestDBName() string {
	n := testDBCounter.Add(1)
	return fmt.Sprintf("money_test_%d_%d", time.Now().UnixNano(), n)
}

// NewDatabaseURL creates a fresh, empty, schema-loaded database dedicated
// to the calling test, and returns a connection string for it. It never
// touches the "money" database that manual/local testing uses, and the
// database is dropped again when the test finishes. If no PostgreSQL
// server is reachable, it skips the calling test.
func NewDatabaseURL(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, adminDatabaseURL())
	if err != nil {
		t.Skipf("connect to postgres: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres not reachable at %s (start it with `make db-up`): %v", adminDatabaseURL(), err)
	}

	dbName := nextTestDBName()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		admin.Close()
		t.Fatalf("create %s database: %v", dbName, err)
	}

	dbURL := withDatabase(adminDatabaseURL(), dbName)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to %s database: %v", dbName, err)
	}

	schema, err := os.ReadFile(schemaPath())
	if err != nil {
		t.Fatalf("read db/schema.sql: %v", err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatalf("apply db/schema.sql: %v", err)
	}
	pool.Close()

	t.Cleanup(func() {
		defer admin.Close()
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName); err != nil {
			t.Logf("drop %s database: %v", dbName, err)
		}
	})

	return dbURL
}

// NewPool is like NewDatabaseURL, but returns an open connection pool to
// the database instead of its URL.
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), NewDatabaseURL(t))
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// schemaPath locates db/schema.sql relative to this source file, so it
// resolves correctly regardless of the working directory tests run from.
func schemaPath() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// api/internal/testutil/testdb.go -> repo root/db/schema.sql
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "db", "schema.sql")
}

func withDatabase(databaseURL, dbName string) string {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return databaseURL
	}
	u.Path = "/" + dbName
	return u.String()
}
