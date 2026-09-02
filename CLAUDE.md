# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

A small double-entry accounting application with three parts: `db/` (PostgreSQL schema), `api/` (Go REST API, containerized), and `tui/` (Go terminal UI using Bubble Tea, run on the host). `api` and `tui` are separate Go modules tied together at the root by `go.work`.

## Commands

```sh
make up          # start postgres + containerized api via docker compose (builds api image, loads db/schema.sql)
make down        # stop everything
make tui-run     # run the TUI on the host (talks to http://localhost:30730 by default; MONEY_API_URL overrides)

make db-up       # start only postgres (for iterating on the api locally)
make api-run     # run the api with `go run` against DATABASE_URL / ADDR env vars, instead of the container
make db-shell    # psql into the database (DATABASE_URL defaults to localhost:30731)
make db-reset    # docker compose down -v + db-up (wipes postgres data volume)

make build       # go build both api and tui into ./bin
make test        # go test ./... in both api and tui modules
```

To run a single test: `cd api && go test ./internal/store/... -run TestName` (same pattern under `tui/`).

The `api` module's tests (`internal/store`, `internal/api`, `internal/db`) hit a real PostgreSQL server — `make db-up` is enough — because part of the double-entry invariant lives in a database trigger a mock can't reproduce. Each test creates and drops its own disposable `money_test_*` database via `internal/testutil`, so they never touch the dev database; they skip automatically if no server is reachable. The `tui` module's tests need no database or terminal: `internal/client` is tested against an `httptest` server, and `internal/ui`'s Bubble Tea models are driven directly with synthetic `tea.Msg`/`tea.KeyPressMsg` values (see `keyPress`/`typeString` helpers in `internal/ui/accounts_test.go`) against the same kind of fake server.

Both `api` and `postgres` containers publish ports to `127.0.0.1` only (api on 30730, postgres on 30731) — not reachable from other machines by default. Keep this binding when editing `docker-compose.yml`.

## Architecture

**Money values** are always integers in the minor currency unit (cents), never floats, to avoid rounding errors.

**The double-entry invariant** — a transaction's entries (`account_id`, `value`) must sum to zero — is enforced in two independent places, and both must be kept in sync if this logic ever changes:
1. `api/internal/store/transactions.go` checks the sum before writing and returns `store.ErrUnbalanced` (mapped to HTTP 400) if it doesn't hold.
2. `db/schema.sql` has a deferred constraint trigger (`check_transaction_balance`) that re-validates the same invariant at the database level after any write to `transaction_entries`, as a safety net against any other writer.

**API layering** (`api/`): `cmd/api/main.go` wires config → db pool → store → router → http.Server. `internal/config` reads env vars (`DATABASE_URL`, `ADDR`). `internal/db` sets up the pgx connection pool. `internal/store` is the only package that talks to Postgres (`AccountStore`, `TransactionStore` under one `Store`); it defines `ErrNotFound` / `ErrUnbalanced` which the `internal/api` handlers translate into HTTP status codes. `internal/api/router.go` wires `net/http` 1.22+ method+path patterns (e.g. `"GET /accounts/{id}"`) directly to handler methods — no external router library.

**TUI** (`tui/`): a single Bubble Tea program (`internal/ui/app.go`) with `Tab`-switchable Accounts/Transactions views, backed by `internal/client` — a plain REST client for the API (no shared code with `api/`; it's a separate consumer of the public HTTP contract).

**Accounts** support a self-referencing `parent_id` for a chart-of-accounts hierarchy; deletes are `ON DELETE RESTRICT` (children/entries block deleting a parent/account).
