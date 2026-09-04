# Deploying without Docker

`make up` (Docker Compose) is the easiest way to run this app, but it costs
real disk space: the Docker daemon itself, plus the `postgres:16-alpine`
and `golang:1.26-alpine` image layers pulled to build the API image. On a
server where disk space is tight, running PostgreSQL and the API natively
avoids all of that — a native PostgreSQL package and a single static Go
binary are both far smaller than the equivalent container images, and
there's no daemon or layer duplication on top.

This assumes a Linux server with `systemd` (true of most current
distributions). The TUI (`tui/`) was never containerized in the first
place — see the very end of this document.

## 1. Install PostgreSQL

Use your distribution's package manager. On Debian/Ubuntu:

```sh
sudo apt install postgresql
```

By default PostgreSQL listens only on `localhost`
(`listen_addresses = 'localhost'` in `postgresql.conf`) — leave it that
way unless you specifically need remote database access, matching this
project's own default of never exposing more than necessary (see below).

## 2. Create the database and apply the schema

```sh
sudo -u postgres psql -c "CREATE ROLE money LOGIN PASSWORD 'CHANGE-ME';"
sudo -u postgres createdb -O money money
psql "postgres://money:CHANGE-ME@localhost/money?sslmode=disable" -f db/schema.sql
```

Pick a real password — `money`/`money` (the Docker Compose default) is
only fine there because that stack is entirely `127.0.0.1`-only and
throwaway. `db/schema.sql` is the *entire* schema, applied once to an
empty database — see "No migrations" below before you ever change it.

## 3. Build the API binary

The API has no C dependencies (`pgx` is pure Go), so it builds as a
single static binary — no runtime dependencies to install on the server
at all, not even `libc`.

**If the server is disk-constrained, build elsewhere and copy the binary
over** rather than installing the Go toolchain on it — the toolchain
itself is much larger than the binary it produces:

```sh
# On your own machine, from a checkout of this repo:
cd api
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o money-api ./cmd/api
scp money-api youruser@server:/tmp/
```

(Use `GOARCH=arm64` for an ARM server.) Otherwise, if Go 1.26+ is
available on the server itself, just run that same `go build` there
directly.

On the server:

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin money
sudo mkdir -p /opt/money/bin
sudo mv /tmp/money-api /opt/money/bin/money-api
sudo chown -R money:money /opt/money
```

## 4. Run it as a systemd service

Create `/etc/systemd/system/money-api.service`:

```ini
[Unit]
Description=Money accounting API
After=network.target postgresql.service
Wants=postgresql.service

[Service]
User=money
Group=money
Environment=DATABASE_URL=postgres://money:CHANGE-ME@localhost/money?sslmode=disable
Environment=ADDR=127.0.0.1:30730
ExecStart=/opt/money/bin/money-api
Restart=on-failure
RestartSec=2
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

`DATABASE_URL`/`ADDR` are the same two environment variables the API
reads under Docker (see `api/internal/config`) — nothing else to
configure. `ADDR=127.0.0.1:30730` matters: under Docker Compose, the
container itself listens on every interface and it's the *port mapping*
(`127.0.0.1:30730:30730`) that keeps it unreachable from other machines;
run bare like this, the binary must bind to `127.0.0.1` itself to get the
same guarantee.

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now money-api
sudo systemctl status money-api
journalctl -u money-api -f   # logs
```

## Updating the API

Rebuild (or re-copy) the binary and restart:

```sh
sudo systemctl stop money-api
sudo mv /tmp/money-api /opt/money/bin/money-api   # after copying a new build over
sudo systemctl start money-api
```

That's enough for an ordinary code update. It is **not** enough if the
update also changed `db/schema.sql` — see next.

## No migrations — schema changes need a fresh database

There's no migration system: `db/schema.sql` is the whole schema, meant
to be applied once to an *empty* database. A schema change means starting
over, not upgrading in place — so **back up first**:

```sh
pg_dump "postgres://money:CHANGE-ME@localhost/money?sslmode=disable" > money-backup.sql
```

Then, to apply a schema change: drop and recreate the database, reapply
the new `db/schema.sql`, and restore whatever data still applies (a
schema change is usually additive, e.g. a new nullable column or table —
in that case `psql money < money-backup.sql` against the freshly-schema'd
database will just work; a genuinely breaking change needs the dump
edited by hand first). Regardless of whether a change is on the horizon,
put `pg_dump` on a cron job — it's your only way back after a mistake,
container-free deploy or not.

## Running the TUI

The TUI (`tui/`) already runs on the host either way — it was never
containerized, since a terminal UI gains nothing from it. Nothing here
changes that: `make tui-run` (or, from `tui/`, `go build -o money-tui
./cmd/tui` for a standalone binary) works exactly as documented in
`README.md`.

If you're running the TUI in an SSH session on the server itself, the
default `MONEY_API_URL=http://localhost:30730` just works. If you want to
run it from a separate client machine instead, and left the API bound to
`127.0.0.1` (recommended above), forward the port over SSH rather than
opening it up:

```sh
ssh -L 30730:localhost:30730 youruser@server
```

...then run the TUI locally against the default `MONEY_API_URL` in that
same terminal. The alternative — binding `ADDR` to a non-loopback address
and opening a firewall port — works too, but trades away the
"unreachable from other machines by default" property this project
otherwise keeps everywhere, so treat it as a deliberate choice, not a
default.
