package cloudauth

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestHandler(t *testing.T) *Handler {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// Suppress log output in tests.
	return &Handler{
		Store: store,
		Log:   discardLogger(),
	}
}

func TestRegisterFirstAccountGetsOwner(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"email":"alice@example.com","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		SessionToken string   `json:"session_token"`
		Account      Account  `json:"account"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Account.Role != "owner" {
		t.Errorf("first account should be owner, got %q", resp.Account.Role)
	}
	if resp.SessionToken == "" {
		t.Error("expected session token")
	}
	if resp.Account.Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %q", resp.Account.Email)
	}
}

func TestRegisterSecondAccountGetsMember(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// First account.
	body := `{"email":"alice@example.com","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first register failed: %d", rec.Code)
	}

	// Second account.
	body = `{"email":"bob@example.com","name":"Bob","password":"password456"}`
	req = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second register failed: %d", rec.Code)
	}

	var resp struct {
		Account Account `json:"account"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Account.Role != "member" {
		t.Errorf("second account should be member, got %q", resp.Account.Role)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"email":"alice@example.com","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first register failed: %d", rec.Code)
	}

	// Same email.
	req = httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rec.Code)
	}
}

func TestRegisterValidation(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	tests := []struct {
		name string
		body string
		want int
		msg  string
	}{
		{"bad email", `{"email":"notanemail","name":"A","password":"12345678"}`, 400, "invalid email"},
		{"empty name", `{"email":"a@b.com","name":"","password":"12345678"}`, 400, "name is required"},
		{"short password", `{"email":"a@b.com","name":"A","password":"short"}`, 400, "password must be"},
		{"bad json", `{bad`, 400, "invalid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestLoginSuccess(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Register.
	body := `{"email":"alice@example.com","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d", rec.Code)
	}

	// Login.
	body = `{"email":"alice@example.com","password":"password123"}`
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		SessionToken string  `json:"session_token"`
		Account      Account `json:"account"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.SessionToken == "" {
		t.Error("expected session token")
	}
	if resp.Account.Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %q", resp.Account.Email)
	}
}

func TestLoginBadPassword(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Register.
	body := `{"email":"alice@example.com","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Login with wrong password.
	body = `{"email":"alice@example.com","password":"wrongpass"}`
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestLoginRateLimiting(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Register.
	body := `{"email":"alice@example.com","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// 5 failed attempts.
	for i := 0; i < 5; i++ {
		body := `{"email":"alice@example.com","password":"wrongpass"}`
		req := httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(body))
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
	}

	// 6th attempt should be rate limited.
	body = `{"email":"alice@example.com","password":"password123"}`
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(body))
	req.RemoteAddr = "10.0.0.1:12345"
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}

func TestProfile(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Register and get token.
	body := `{"email":"alice@example.com","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var regResp struct {
		SessionToken string `json:"session_token"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&regResp)

	// Get profile.
	req = httptest.NewRequest("GET", "/auth/profile", nil)
	req.Header.Set("Authorization", "Bearer "+regResp.SessionToken)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile failed: %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Account Account `json:"account"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Account.Email != "alice@example.com" {
		t.Errorf("expected alice@example.com, got %q", resp.Account.Email)
	}
}

func TestProfileNoAuth(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/auth/profile", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestProfileBadToken(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/auth/profile", nil)
	req.Header.Set("Authorization", "Bearer invalidtoken")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestEmailNormalization(t *testing.T) {
	h := setupTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Register with uppercase.
	body := `{"email":"Alice@Example.COM","name":"Alice","password":"password123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d", rec.Code)
	}

	// Login with lowercase.
	body = `{"email":"alice@example.com","password":"password123"}`
	req = httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("login with normalized email failed: %d", rec.Code)
	}
}

// discardLogger returns a logger that discards output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError + 4}))
}
