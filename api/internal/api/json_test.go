package api

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, 201, map[string]string{"foo": "bar"})

	if rec.Code != 201 {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["foo"] != "bar" {
		t.Errorf("body = %v, want {foo: bar}", body)
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, 404, "not found")

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "not found" {
		t.Errorf("body = %v, want {error: \"not found\"}", body)
	}
}

func TestPgConstraintMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"known unique constraint", &pgconn.PgError{Code: "23505", ConstraintName: "accounts_code_unique"}, "account code already in use"},
		{"known unique constraint, currency name", &pgconn.PgError{Code: "23505", ConstraintName: "currencies_name_unique"}, "currency name already in use"},
		{"known unique constraint, currency ISIN", &pgconn.PgError{Code: "23505", ConstraintName: "currencies_isin_unique"}, "ISIN already in use"},
		{"known fkey constraint", &pgconn.PgError{Code: "23503", ConstraintName: "accounts_parent_id_fkey"}, "referenced parent account does not exist"},
		{"unrecognized foreign key violation", &pgconn.PgError{Code: "23503", ConstraintName: "something_else_fkey"}, "referenced record does not exist"},
		{"unrecognized unique violation", &pgconn.PgError{Code: "23505", ConstraintName: "something_else_unique"}, "value already in use"},
		{"unrelated pg error", &pgconn.PgError{Code: "42601"}, ""},
		{"non-pg error", errors.New("boom"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pgConstraintMessage(tt.err); got != tt.want {
				t.Errorf("pgConstraintMessage(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsForeignKeyViolation(t *testing.T) {
	if !isForeignKeyViolation(&pgconn.PgError{Code: "23503"}) {
		t.Error("expected 23503 to be a foreign key violation")
	}
	if isForeignKeyViolation(&pgconn.PgError{Code: "23505"}) {
		t.Error("expected 23505 not to be a foreign key violation")
	}
	if isForeignKeyViolation(errors.New("boom")) {
		t.Error("expected a non-pg error not to be a foreign key violation")
	}
}
