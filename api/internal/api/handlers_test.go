package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"money/api/internal/api"
	"money/api/internal/models"
	"money/api/internal/store"
	"money/api/internal/testutil"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	s := store.New(testutil.NewPool(t))
	return api.NewRouter(s)
}

// do sends a request with an optional JSON body and decodes the JSON
// response body into out (if out is non-nil).
func do(t *testing.T, h http.Handler, method, path string, body any, out any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
		}
	}
	return rec
}

// httptestRequest builds a request with a raw (possibly malformed) body,
// for tests that need to send something json.Marshal would never produce.
func httptestRequest(t *testing.T, method, path, rawBody string) *http.Request {
	t.Helper()
	return httptest.NewRequest(method, path, strings.NewReader(rawBody))
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// createTestCurrency creates a currency via the HTTP API, for tests that
// need a valid currency_id to post transaction entries against.
func createTestCurrency(t *testing.T, h http.Handler, name string) models.Currency {
	t.Helper()
	var c models.Currency
	rec := do(t, h, http.MethodPost, "/currencies", models.Currency{
		Name:             name,
		SymbolBefore:     true,
		DecimalSeparator: ".",
		DecimalPlaces:    2,
	}, &c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create currency %q: status = %d", name, rec.Code)
	}
	return c
}

func TestHealthz(t *testing.T) {
	h := newTestHandler(t)

	var body map[string]string
	rec := do(t, h, http.MethodGet, "/healthz", nil, &body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v, want {status: ok}", body)
	}
}
