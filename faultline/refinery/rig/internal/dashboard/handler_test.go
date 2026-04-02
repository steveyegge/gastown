package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionToken(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if token := sessionToken(r); token != "" {
		t.Errorf("expected empty token, got %q", token)
	}

	r.AddCookie(&http.Cookie{Name: "session", Value: "abc123"})
	if token := sessionToken(r); token != "abc123" {
		t.Errorf("expected abc123, got %q", token)
	}
}

func TestPathInt64(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if v := pathInt64(r, "missing"); v != 0 {
		t.Errorf("expected 0, got %d", v)
	}
}

func TestQueryInt(t *testing.T) {
	r := httptest.NewRequest("GET", "/?page=3&bad=xyz", nil)
	if v := queryInt(r, "page"); v != 3 {
		t.Errorf("expected 3, got %d", v)
	}
	if v := queryInt(r, "bad"); v != 1 {
		t.Errorf("expected fallback 1, got %d", v)
	}
	if v := queryInt(r, "missing"); v != 1 {
		t.Errorf("expected fallback 1, got %d", v)
	}
}

func TestRequireAuthRedirects(t *testing.T) {
	h := &Handler{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dashboard/", h.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No session cookie → redirect to login.
	r := httptest.NewRequest("GET", "/dashboard/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/dashboard/login" {
		t.Errorf("expected redirect to /dashboard/login, got %s", loc)
	}
}

func TestViewHelpers(t *testing.T) {
	tests := []struct {
		level string
		icon  string
		class string
	}{
		{"fatal", "🔴", "severity-rupture"},
		{"error", "⚡", "severity-quake"},
		{"warning", "〰️", "severity-tremor"},
		{"info", "·", ""},
	}
	for _, tt := range tests {
		if got := severityIcon(tt.level); got != tt.icon {
			t.Errorf("severityIcon(%s) = %q, want %q", tt.level, got, tt.icon)
		}
		if got := severityClass(tt.level); got != tt.class {
			t.Errorf("severityClass(%s) = %q, want %q", tt.level, got, tt.class)
		}
	}

	if got := magnitude(1500); got != "Great" {
		t.Errorf("magnitude(1500) = %q, want Great", got)
	}
	if got := magnitude(5); got != "Minor" {
		t.Errorf("magnitude(5) = %q, want Minor", got)
	}
	if got := statusLabel("resolved"); got != "Stabilized" {
		t.Errorf("statusLabel(resolved) = %q, want Stabilized", got)
	}
}
