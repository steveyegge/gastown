package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenEndpointsRequireAuth(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	tests := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/tokens"},
		{"GET", "/api/v1/tokens"},
		{"DELETE", "/api/v1/tokens/1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			var body *strings.Reader
			if tt.method == "POST" {
				body = strings.NewReader(`{"name":"test"}`)
			} else {
				body = strings.NewReader("")
			}
			r := httptest.NewRequest(tt.method, tt.path, body)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", w.Code)
			}
		})
	}
}

func TestCreateTokenNoAuth(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	// POST without auth header should get 401.
	r := httptest.NewRequest("POST", "/api/v1/tokens", strings.NewReader(`{"name":"test"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestFormatInt64(t *testing.T) {
	if got := formatInt64(42); got != "42" {
		t.Errorf("formatInt64(42) = %q, want \"42\"", got)
	}
	if got := formatInt64(0); got != "0" {
		t.Errorf("formatInt64(0) = %q, want \"0\"", got)
	}
}

func TestHeaderInt64(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Account-ID", "123")
	if got := headerInt64(r, "X-Account-ID"); got != 123 {
		t.Errorf("headerInt64 = %d, want 123", got)
	}

	r2 := httptest.NewRequest("GET", "/", nil)
	if got := headerInt64(r2, "X-Account-ID"); got != 0 {
		t.Errorf("headerInt64 missing = %d, want 0", got)
	}
}
