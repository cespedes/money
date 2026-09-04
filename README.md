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
  (a chart of accounts). Each also has a `position`, ordering it for display
  among its siblings (other accounts with the same `parent_id`, including
  other root accounts when it's null) — assigned automatically on creation
  (after its last sibling) and otherwise changed only by explicitly moving
  an account up or down (`POST /accounts/{id}/move`), never by a plain
  update. The API also reports each account's `balances`: one
  `(currency_id, amount)` pair per currency it has entries in — its own
  entries only, not including any child accounts' — with no entry at all for
  a currency it's never been posted in. It also reports
  `last_transaction_at`, the timestamp of the most recent transaction with
  an entry on it (again its own only) — omitted if it has none.
- **Currencies** (or, more generally, commodities) are the units entries are
  posted in. Each has a `name`, formatting configuration (`symbol_before`
  — whether the name goes before the amount, defaulting to `false` —,
  `symbol_space`, `thousands_separator`, `decimal_separator`, and
  `decimal_places`, which governs both how amounts are stored and how many
  decimal digits to render), and an optional `isin`.
- **Transactions** have a `timestamp`, a `description`, and a list of
  **entries**, each an `(account_id, amount, currency_id)` triple. The
  entries of a transaction must always sum to zero *within each currency*
  (double-entry bookkeeping) — amounts in different currencies are never
  summed together, so a single transaction can freely mix currencies as
  long as each one balances on its own. A positive amount is a debit, a
  negative amount is a credit.
- **Currency prices** record a directly-observed exchange rate: one unit
  of `base_currency_id` was worth `rate` units of `quote_currency_id`, as
  of a specific instant (`as_of`). Unlike a transaction's `amount`, `rate`
  is an approximate market price — a plain floating-point number, not an
  integer tied to either currency's `decimal_places` — since it's not a
  ledger quantity. See below for how a rate is looked up for an instant
  that wasn't directly recorded.

Monetary amounts (transaction/ledger entry `amount`, ledger `balance`,
account `balances`) are stored internally as integers in the minor unit of
their currency (e.g. cents, per that currency's `decimal_places`), never
floating point, to avoid rounding errors — but on the API's JSON wire
format they're represented as a decimal JSON number in the currency's own
major unit, e.g. `10.50` (not `1050`) in a currency with 2 decimal places.
The API converts between the two exactly, via string-digit arithmetic
rather than floating point, so this involves no rounding; a number with
more decimal digits than the currency's `decimal_places` is rejected. An
amount always shows exactly `decimal_places` decimal digits on the way
out — `1050` is sent back as `10.50`, not `10.5`, and a whole number like
`1000` as `10.00`, not `10` — matching how money is conventionally
written. This doesn't apply to currency prices (see above), which are
inherently approximate market data rather than ledger quantities.

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

There's no migration system: `db/schema.sql` is the whole schema, applied
once to an empty database (automatically by `make up`/`make db-up` on
their first run, since it's mounted as a Postgres init script). A schema
change means starting from a fresh database — `make db-reset` (or
`docker compose down -v` for the full stack) — not upgrading an existing
one in place.

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
| GET    | `/accounts/{id}/transactions` | List an account's ledger (see below) |
| POST   | `/accounts/{id}/move` | Move an account up/down among its siblings (see below) |
| GET    | `/transactions`     | List transactions (with their entries)    |
| POST   | `/transactions`     | Create a transaction (entries must sum to zero per currency) |
| GET    | `/transactions/{id}`| Get a transaction (with its entries)      |
| DELETE | `/transactions/{id}`| Delete a transaction                      |
| GET    | `/currencies`       | List currencies                           |
| POST   | `/currencies`       | Create a currency                         |
| GET    | `/currencies/{id}`  | Get a currency                            |
| PUT    | `/currencies/{id}`  | Update a currency                         |
| DELETE | `/currencies/{id}`  | Delete a currency                         |
| GET    | `/currency-prices`  | List currency prices                      |
| POST   | `/currency-prices`  | Record an exchange rate observation       |
| GET    | `/currency-prices/{id}` | Get a currency price                  |
| DELETE | `/currency-prices/{id}` | Delete a currency price               |
| GET    | `/currency-prices/rate` | Look up a rate between two currencies at an instant (see below) |
| GET    | `/healthz`          | Health check                              |

Example: create a currency and two accounts, then a balanced transaction
between them.

```sh
curl -s localhost:30730/currencies -d '{
  "name": "US Dollar", "symbol_before": true, "symbol_space": false,
  "thousands_separator": ",", "decimal_separator": ".", "decimal_places": 2
}'
curl -s localhost:30730/accounts -d '{"name": "Cash", "code": "1000"}'
curl -s localhost:30730/accounts -d '{"name": "Revenue", "code": "4000"}'

curl -s localhost:30730/transactions -d '{
  "timestamp": "2026-08-31T10:00:00Z",
  "description": "Invoice #1",
  "entries": [
    {"account_id": 1, "amount": 10, "currency_id": 1},
    {"account_id": 2, "amount": -10, "currency_id": 1}
  ]
}'
```

A single transaction may post in more than one currency, as long as each
currency's own entries sum to zero.

`GET /accounts/{id}/transactions` returns that account's ledger: every
transaction with an entry on it, in timestamp order, with that account's
own amount (and currency) in the transaction and its running balance *in
that same currency* through that point — e.g.
`[{"transaction_id": 1, "timestamp": "...", "description": "Invoice #1", "currency_id": 1, "amount": 10.00, "balance": 10.00}]`.
A transaction posting to the account in more than one currency contributes
one row per currency.

`POST /accounts/{id}/move` takes `{"direction": "up"}` or `{"direction":
"down"}` and swaps that account's `position` with whichever sibling is
immediately before/after it — a no-op, not an error, if it's already
first/last among its siblings.

`POST /currency-prices` records one directly-observed rate, e.g. `{
"base_currency_id": 1, "quote_currency_id": 2, "rate": 1.16, "as_of":
"2026-08-31T10:00:00Z" }` for "1 unit of currency 1 was worth 1.16 units
of currency 2 at that instant". `base_currency_id`/`quote_currency_id`
must differ, and at most one observation can be recorded per currency
pair at the exact same instant.

`GET /currency-prices/rate?base={id}&quote={id}&at={RFC3339 timestamp}`
looks up how many units of `quote` one unit of `base` was worth at `at`
(defaulting to now if omitted), e.g.
`{"base_currency_id": 1, "quote_currency_id": 2, "rate": 1.1583, "at": "..."}`.
It doesn't require an observation recorded at exactly that instant:

- If there's an observation exactly at `at`, that's the rate.
- If there are observations both before and after `at` (for that pair,
  in either direction — an observation always implies its inverse rate
  too), the rate is linearly interpolated between them. E.g. if 1 EUR was
  worth 1.01 USD on day 1 and 1.06 USD on day 6, the rate on day 4 is
  taken to be about 1.04.
- If there's only an observation before `at`, or only one after it,
  that's the rate — same as if it had held constant since/until then.
- If there's no observation at all directly relating the two currencies,
  it looks for a chain of intermediate currencies that does connect them
  (breadth-first, fewest hops), multiplying each leg's own
  (interpolated) rate along the way — the way cross-rates normally
  compose. E.g. if 1 EUR is worth 1.16 USD and 1 USD is worth 0.74 GBP,
  1 EUR is taken to be worth about 0.86 GBP even with no EUR/GBP
  observation on record.
- If the two currencies aren't connected by any chain of observations,
  the request returns `404`.

## TUI

The TUI doesn't have a view for currency prices yet — use the API
directly (see above) to record and query exchange rates.

`Tab`/`Shift+Tab` cycle through the Accounts, Transactions, and Currencies
views. Within a view: `↑`/`↓` navigate, `n` creates a new record, `r`
refreshes, `Enter` opens a transaction's entries or an account's ledger
(Currencies has no such drill-down). Inside an account's ledger, `n`
instead adds a transaction (see below), rather than a new account. `Esc`
cancels a form or backs out of a ledger/detail view, `q` quits. Accounts and Currencies also support `e`
to edit the selected record and `d` to delete it; Transactions only
supports `d` (no edit). Accounts additionally support `K`/`J` to move the
selected account up/down among its siblings (see `POST /accounts/{id}/move`
above) — the selection follows the account as it moves, and moving it past
the first/last sibling is a no-op. These are capital letters, not
`Shift`+arrow: some terminals (e.g. xfce4-terminal) don't report Shift
held with an arrow key as a distinguishable event, but every terminal
reports a capital letter as a plain keystroke.

Editing reuses the same pop-up as creating (see below), pre-filled with
the selected record's current values, and submits a PUT instead of a POST
on `Enter` — the pop-up's title and the footer's hint ("save" instead of
"create") reflect which mode it's in. Editing an account leaves that
account and all of its descendants out of its own Parent dropdown, since
choosing any of them would make the account its own ancestor; the API
rejects such an update too (`400`, "an account cannot be its own
ancestor"), as a safety net against any other writer.

Every displayed amount is formatted per its own currency's rules (name
position/spacing, thousands/decimal separators, decimal places) via
`client.Currency.Format`; an account's balance is shown as one such
amount per currency it has entries in (e.g. `US Dollar10.00, 5,00 Euro`),
and a ledger's running balance is likewise kept separate per currency.

Creating an account opens a pop-up over the accounts list, laid out as a
small table — Parent, Name, and Code as fixed-width columns with their
names above. `Tab`/`Shift+Tab` move between them; `←`/`→` edit within
whichever text field (Name or Code) has focus instead, moving the cursor
the way they would in any text input. While Parent has focus, a dropdown
lists "(none)" and every existing account — all of them at once, or as
many as fit in the window — and `↑`/`↓` pick one. Name is required, code
is optional.

Creating a currency opens a similar pop-up with three fields: Name, Format,
and ISIN. Format is a single example of how the currency renders a fixed
test quantity of 1234 whole units — e.g. `$1,234.00`, `1.234,00 EUR`, or
`1234 PTS` — from which the currency's `symbol_before`, `symbol_space`,
`thousands_separator`, `decimal_separator`, and `decimal_places` are all
derived on submit; Name must appear at the very start or end of it
(optionally set off by a single space) and the rest must match "1234"
exactly, split by an optional thousands separator into "1"+"234" and/or
followed by a decimal separator and one or more decimal digits, or the
submission is rejected with an error. Only Name is required. The
currencies list itself shows Name, that same computed Format (applied to
the currency's actual stored rules), and ISIN — rather than the
individual formatting fields.

When creating a transaction, the TUI walks through description, timestamp,
and then repeatedly asks for an account, a currency (picked from a
dropdown, the same way an account's parent is), and an amount — until you
confirm you're done. Amount is typed as a real number using that
currency's own thousands/decimal separators (e.g. `1.234,56` for a
currency formatted like EUR), not the API's own decimal syntax or the
underlying integer minor units. A transaction can mix currencies freely;
it will only submit once each currency's own entries sum to zero.

Pressing `n` inside an account's ledger is a quicker path to the common
case: a transaction between that account and exactly one other. Unlike
the general wizard above, it's a single table-shaped pop-up with two
rows, both visible at once — Timestamp, Description, Amount, Currency
for this account, then Account, Amount, Currency for the other side —
`Tab`/`Shift+Tab` move between all seven fields and `Enter` submits from
any of them. It pre-fills the current date/time, an empty description
and amount, and whichever currency this account's ledger was last posted
in (or the first available currency, if it has none yet) for both rows —
all editable, including the second row's currency, which needn't match
the first's. The other account is picked from a dropdown (everything but
the account whose ledger is open). The second row's amount is optional
only when both rows share the same currency, where a blank amount is
taken to be whatever balances the transaction against the first; with
different currencies there's no such well-defined default (no exchange
rate is applied here), so an explicit amount is required, and as always
the transaction only actually submits once each currency's own entries
sum to zero.

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
