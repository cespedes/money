.PHONY: up down db-up db-down db-reset db-shell api-run api-build tui-run tui-build build test

DATABASE_URL ?= postgres://money:money@localhost:30731/money?sslmode=disable

# Full stack (postgres + containerized api) via docker compose.
up:
	docker compose up -d --build

down:
	docker compose down

db-up:
	docker compose up -d postgres

db-down:
	docker compose stop postgres
	docker compose rm -f postgres

db-reset:
	docker compose down -v
	docker compose up -d postgres

db-shell:
	psql "$(DATABASE_URL)"

# Run the API directly on the host (against a postgres started with db-up),
# instead of the containerized one started by `make up`.
api-run:
	cd api && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/api

api-build:
	cd api && go build -o ../bin/api ./cmd/api

tui-run:
	cd tui && go run ./cmd/tui

tui-build:
	cd tui && go build -o ../bin/tui ./cmd/tui

build: api-build tui-build

test:
	cd api && go test ./...
	cd tui && go test ./...
