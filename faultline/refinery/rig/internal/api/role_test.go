package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHasMinRole(t *testing.T) {
	tests := []struct {
		account string
		min     string
		want    bool
	}{
		{"owner", "owner", true},
		{"owner", "admin", true},
		{"owner", "member", true},
		{"owner", "viewer", true},
		{"admin", "owner", false},
		{"admin", "admin", true},
		{"admin", "member", true},
		{"admin", "viewer", true},
		{"member", "owner", false},
		{"member", "admin", false},
		{"member", "member", true},
		{"member", "viewer", true},
		{"viewer", "owner", false},
		{"viewer", "admin", false},
		{"viewer", "member", false},
		{"viewer", "viewer", true},
		{"", "viewer", false},
		{"invalid", "viewer", false},
	}
	for _, tt := range tests {
		got := hasMinRole(tt.account, tt.min)
		if got != tt.want {
			t.Errorf("hasMinRole(%q, %q) = %v, want %v", tt.account, tt.min, got, tt.want)
		}
	}
}

func TestRequireRole(t *testing.T) {
	called := false
	handler := requireRole("admin", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		role       string
		wantStatus int
		wantCalled bool
	}{
		{"owner passes", "owner", http.StatusOK, true},
		{"admin passes", "admin", http.StatusOK, true},
		{"member blocked", "member", http.StatusForbidden, false},
		{"viewer blocked", "viewer", http.StatusForbidden, false},
		{"empty blocked", "", http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest("POST", "/", nil)
			r.Header.Set("X-Account-Role", tt.role)
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestRequireRoleMember(t *testing.T) {
	called := false
	handler := requireRole("member", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		role       string
		wantStatus int
		wantCalled bool
	}{
		{"owner passes", "owner", http.StatusOK, true},
		{"admin passes", "admin", http.StatusOK, true},
		{"member passes", "member", http.StatusOK, true},
		{"viewer blocked", "viewer", http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest("POST", "/", nil)
			r.Header.Set("X-Account-Role", tt.role)
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}

func TestRequireRoleOwner(t *testing.T) {
	called := false
	handler := requireRole("owner", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name       string
		role       string
		wantStatus int
		wantCalled bool
	}{
		{"owner passes", "owner", http.StatusOK, true},
		{"admin blocked", "admin", http.StatusForbidden, false},
		{"member blocked", "member", http.StatusForbidden, false},
		{"viewer blocked", "viewer", http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = false
			r := httptest.NewRequest("POST", "/", nil)
			r.Header.Set("X-Account-Role", tt.role)
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("handler called = %v, want %v", called, tt.wantCalled)
			}
		})
	}
}
