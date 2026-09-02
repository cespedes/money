# Money

A small double-entry accounting application, made of three parts:

- **`db/`** — a PostgreSQL schema.
- **`api/`** — a REST API, written in Go, on top of PostgreSQL. Runs in a
  container; see `api/Dockerfile`.
- **`tui/`** — a terminal UI, written in Go, that talks to the API. Runs on
  the host (a TUI doesn't gain anything from being containerized).

## Data model

- **Accounts** have a `name`, an optional `code`, and an optional `parent_id`
  pointing to another account, so accounts can be organized into a hierarchy
  (a chart of accounts).
- **Transactions** have a `timestamp`, a `description`, and a list of
  **entries**, each an `(account_id, value)` pair. The values of a
  transaction's entries must always sum to zero (double-entry bookkeeping):
  a positive value is a debit, a negative value is a credit.

Monetary values are stored and transmitted as integers in the minor
currency unit (e.g. cents) rather than floating point numbers, to avoid
rounding errors. `1050` means `10.50` in whatever currency the deployment
uses.

The zero-sum invariant is enforced twice: the API rejects unbalanced
transactions before writing anything, and the database itself has a
deferred constraint trigger (`db/schema.sql`) that re-checks it, as a
safety net against any other writer.

## Prerequisites

- Go 1.26+ (only needed to build/run the TUI; the API builds inside Docker)
- Docker, with Compose
- `psql`, if you want to inspect the database by hand

## Running it

Start PostgreSQL and the API with Docker Compose (this also builds the API
image and loads `db/schema.sql` into Postgres as an init script on first
start):

```sh
make up
```

Both containers publish their ports to `127.0.0.1` only — Postgres on
`30731` and the API on `30730` — so nothing is reachable from other
machines by default.

Run the TUI on the host, in another terminal (talks to
`http://localhost:30730` by default; set `MONEY_API_URL` to override):

```sh
make tui-run
```

To stop everything:

```sh
make down
```

### Running the API on the host instead

If you'd rather iterate on the API without rebuilding a container each
time, start only Postgres and run the API with `go run`:

```sh
make db-up
make api-run
```

`api-run` listens on `:30730` by default; set `ADDR` / `DATABASE_URL` to
override.

To apply the schema against a database you're managing yourself instead of
`make db-up`:

```sh
psql "$DATABASE_URL" -f db/schema.sql
```

### Running the tests

The API's tests exercise the store and HTTP layers against a real
PostgreSQL server, since part of the double-entry invariant is enforced by
a database trigger (see `db/schema.sql`) that a mock can't reproduce. They
need a running server (`make db-up` is enough) and create/drop their own
disposable `money_test_*` databases on it, so they never touch the
database used for local development. Point them elsewhere with
`TEST_DATABASE_URL`; they skip automatically if no server is reachable.

```sh
make db-up
make test
```

## API

All endpoints accept and return JSON.

| Method | Path                | Description                              |
|--------|---------------------|-------------------------------------------|
| GET    | `/accounts`         | List accounts                             |
| POST   | `/accounts`         | Create an account                         |
| GET    | `/accounts/{id}`    | Get an account                            |
| PUT    | `/accounts/{id}`    | Update an account                         |
| DELETE | `/accounts/{id}`    | Delete an account                         |
| GET    | `/transactions`     | List transactions (with their entries)    |
| POST   | `/transactions`     | Create a transaction (entries must sum to zero) |
| GET    | `/transactions/{id}`| Get a transaction (with its entries)      |
| DELETE | `/transactions/{id}`| Delete a transaction                      |
| GET    | `/healthz`          | Health check                              |

Example: create two accounts, then a balanced transaction between them.

```sh
curl -s localhost:30730/accounts -d '{"name": "Cash", "code": "1000"}'
curl -s localhost:30730/accounts -d '{"name": "Revenue", "code": "4000"}'

curl -s localhost:30730/transactions -d '{
  "timestamp": "2026-08-31T10:00:00Z",
  "description": "Invoice #1",
  "entries": [
    {"account_id": 1, "value": 1000},
    {"account_id": 2, "value": -1000}
  ]
}'
```

## TUI

`Tab` switches between the Accounts and Transactions views. Within a view:
`↑`/`↓` navigate, `n` creates a new record, `d` deletes the selected one,
`r` refreshes, and (in the Transactions view) `Enter` opens a transaction's
entries. `Esc` cancels a form, `q` quits.

When creating an account, the TUI first asks you to pick a parent from a
list of the existing accounts (or "(none)", the default), then a name
(required), then a code (optional).

When creating a transaction, the TUI walks through description, timestamp,
and then repeatedly asks for `(account_id, value)` entries until you
confirm you're done; it will only submit once at least two entries are
present and they sum to zero.

## Project layout

```
db/                   PostgreSQL schema
api/
  Dockerfile          container image for the API
  cmd/api/            main package, wires everything together
  internal/config/    environment-based configuration
  internal/db/        connection pool setup
  internal/models/    domain types
  internal/store/     PostgreSQL persistence (pgx)
  internal/api/        HTTP handlers and routing
tui/
  cmd/tui/            main package
  internal/client/    REST client for the API
  internal/ui/        Bubble Tea model/view code
```

`go.work` at the repository root ties the two independent Go modules
(`api`, `tui`) together for local development.
