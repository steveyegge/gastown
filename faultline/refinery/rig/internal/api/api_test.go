package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPathInt64(t *testing.T) {
	// pathInt64 returns 0 for missing or invalid values.
	r := httptest.NewRequest("GET", "/", nil)
	if v := pathInt64(r, "missing"); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestQueryInt(t *testing.T) {
	r := httptest.NewRequest("GET", "/?limit=10&bad=xyz&neg=-5", nil)
	if v := queryInt(r, "limit", 25); v != 10 {
		t.Errorf("expected 10, got %d", v)
	}
	if v := queryInt(r, "bad", 25); v != 25 {
		t.Errorf("expected fallback 25 for non-numeric, got %d", v)
	}
	if v := queryInt(r, "neg", 25); v != 25 {
		t.Errorf("expected fallback 25 for negative, got %d", v)
	}
	if v := queryInt(r, "missing", 25); v != 25 {
		t.Errorf("expected fallback 25 for missing, got %d", v)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"hello": "world"})
	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["hello"] != "world" {
		t.Errorf("expected world, got %s", body["hello"])
	}
}

func TestWriteErr(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, http.StatusBadRequest, "bad input")
	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "bad input" {
		t.Errorf("expected 'bad input', got %s", body["error"])
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid", "Bearer abc123", "abc123"},
		{"case insensitive", "bearer abc123", "abc123"},
		{"empty", "", ""},
		{"no prefix", "abc123", ""},
		{"just bearer", "Bearer ", ""},
		{"with spaces", "Bearer  abc123 ", "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := extractBearerToken(r)
			if got != tt.want {
				t.Errorf("extractBearerToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestListIssuesInvalidProject(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("GET", "/api/0/issues/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Without Bearer token, should get 401.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUpdateIssueInvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("PUT", "/api/1/issues/abc123/", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Without Bearer token, should get 401.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUpdateIssuePatchMethod(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("PATCH", "/api/1/issues/abc123/", strings.NewReader(`{"status":"ignored"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Without Bearer token, should get 401 (proves the PATCH route is registered).
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestIssueContextInvalidParams(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("GET", "/api/0/issues/abc123/context", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Without Bearer token, should get 401.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDoltLogInvalidParams(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("GET", "/api/0/issues/abc123/dolt-log", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Without Bearer token, should get 401.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAssignBeadInvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("POST", "/api/1/issues/abc123/assign-bead", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	// Without Bearer token, should get 401.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestResolveHookInvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	// Resolve hook does NOT require Bearer auth — should get 400, not 401.
	r := httptest.NewRequest("POST", "/api/hooks/resolve", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestResolveHookMissingFields(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("POST", "/api/hooks/resolve", strings.NewReader(`{"project_id":0}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestProjectsRouteRequiresAuth(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterRoutes(mux)

	r := httptest.NewRequest("GET", "/api/v1/projects/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == "" {
		t.Error("expected error message in response")
	}

	// Check WWW-Authenticate header.
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}
