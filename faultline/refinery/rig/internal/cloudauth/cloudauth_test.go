package cloudauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPKCE(t *testing.T) {
	p, err := newPKCE()
	if err != nil {
		t.Fatal(err)
	}

	if len(p.Verifier) < 43 {
		t.Errorf("verifier too short: %d chars", len(p.Verifier))
	}
	if p.Challenge == "" {
		t.Error("challenge is empty")
	}
	if p.Method != "S256" {
		t.Errorf("method = %q, want S256", p.Method)
	}

	// Verifier and challenge should differ (challenge is SHA-256 of verifier).
	if p.Verifier == p.Challenge {
		t.Error("verifier and challenge should not be equal")
	}
}

func TestPKCEUniqueness(t *testing.T) {
	p1, _ := newPKCE()
	p2, _ := newPKCE()
	if p1.Verifier == p2.Verifier {
		t.Error("two PKCE instances should have different verifiers")
	}
}

func TestTokenValidity(t *testing.T) {
	tests := []struct {
		name         string
		token        *Token
		wantValid    bool
		wantRefresh  bool
	}{
		{
			name:      "nil token",
			token:     nil,
			wantValid: false,
		},
		{
			name: "valid token",
			token: &Token{
				AccessToken: "abc",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
			},
			wantValid:   true,
			wantRefresh: false,
		},
		{
			name: "expired token",
			token: &Token{
				AccessToken: "abc",
				ExpiresAt:   time.Now().Add(-1 * time.Hour),
			},
			wantValid: false,
		},
		{
			name: "near-expiry needs refresh",
			token: &Token{
				AccessToken:  "abc",
				RefreshToken: "ref",
				ExpiresAt:    time.Now().Add(2 * time.Minute), // within 5min buffer
			},
			wantValid:   true,
			wantRefresh: true,
		},
		{
			name: "no refresh token available",
			token: &Token{
				AccessToken: "abc",
				ExpiresAt:   time.Now().Add(2 * time.Minute),
			},
			wantValid:   true,
			wantRefresh: false, // no refresh token
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.Valid(); got != tt.wantValid {
				t.Errorf("Valid() = %v, want %v", got, tt.wantValid)
			}
			if got := tt.token.NeedsRefresh(); got != tt.wantRefresh {
				t.Errorf("NeedsRefresh() = %v, want %v", got, tt.wantRefresh)
			}
		})
	}
}

func TestStoreAndLoadToken(t *testing.T) {
	// Use a temp dir as home.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	token := &Token{
		AccessToken:  "test-access",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour).Truncate(time.Millisecond),
		Scope:        "offline_access",
	}

	if err := StoreToken(token); err != nil {
		t.Fatalf("StoreToken: %v", err)
	}

	// Verify file permissions.
	path := filepath.Join(tmpDir, ".faultline", "cloud-token.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("token file permissions = %o, want 0600", perm)
	}

	// Load it back.
	loaded, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded.AccessToken != token.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, token.AccessToken)
	}
	if loaded.RefreshToken != token.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, token.RefreshToken)
	}
}

func TestLoadTokenNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	token, err := LoadToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != nil {
		t.Error("expected nil token when file doesn't exist")
	}
}

func TestRemoveToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Store then remove.
	token := &Token{AccessToken: "x", ExpiresAt: time.Now().Add(time.Hour)}
	if err := StoreToken(token); err != nil {
		t.Fatal(err)
	}

	if err := RemoveToken(); err != nil {
		t.Fatalf("RemoveToken: %v", err)
	}

	// Should be gone.
	loaded, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Error("token should be nil after removal")
	}

	// Removing again should not error.
	if err := RemoveToken(); err != nil {
		t.Fatalf("double RemoveToken: %v", err)
	}
}

func TestExchangeCode(t *testing.T) {
	// Mock token endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code_verifier") == "" {
			t.Error("missing code_verifier")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "mock-access",
			"refresh_token": "mock-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "offline_access",
		})
	}))
	defer srv.Close()

	cfg := Config{Issuer: srv.URL, ClientID: "test-client"}

	token, err := exchangeCode(context.Background(), cfg, "test-code", "http://localhost/callback", "test-verifier")
	if err != nil {
		t.Fatalf("exchangeCode: %v", err)
	}

	if token.AccessToken != "mock-access" {
		t.Errorf("AccessToken = %q, want mock-access", token.AccessToken)
	}
	if token.RefreshToken != "mock-refresh" {
		t.Errorf("RefreshToken = %q, want mock-refresh", token.RefreshToken)
	}
}

func TestExchangeCodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "code expired",
		})
	}))
	defer srv.Close()

	cfg := Config{Issuer: srv.URL, ClientID: "test-client"}

	_, err := exchangeCode(context.Background(), cfg, "bad-code", "http://localhost/callback", "v")
	if err == nil {
		t.Fatal("expected error for invalid_grant")
	}
	if got := err.Error(); !strings.Contains(got, "invalid_grant") {
		t.Errorf("error = %q, should contain invalid_grant", got)
	}
}

func TestRefreshToken(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q, want refresh_token", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	cfg := Config{Issuer: srv.URL, ClientID: "test-client"}

	token, err := RefreshToken(context.Background(), cfg, "old-refresh")
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if token.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want new-access", token.AccessToken)
	}

	// Should also be stored.
	stored, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken != "new-access" {
		t.Errorf("stored AccessToken = %q", stored.AccessToken)
	}
}

func TestLoadAndRefreshValid(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Store a valid token.
	token := &Token{
		AccessToken: "valid",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	StoreToken(token)

	cfg := Config{Issuer: "http://unused", ClientID: "test"}
	loaded, err := LoadAndRefresh(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "valid" {
		t.Errorf("got %q, want valid", loaded.AccessToken)
	}
}

func TestLoadAndRefreshExpired(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "refreshed",
			"refresh_token": "new-ref",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	// Store an expired token with refresh.
	token := &Token{
		AccessToken:  "old",
		RefreshToken: "ref",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
	}
	StoreToken(token)

	cfg := Config{Issuer: srv.URL, ClientID: "test"}
	loaded, err := LoadAndRefresh(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "refreshed" {
		t.Errorf("got %q, want refreshed", loaded.AccessToken)
	}
}

func TestDefaultConfig(t *testing.T) {
	// With no env vars.
	t.Setenv(EnvIssuer, "")
	t.Setenv(EnvClientID, "")
	cfg := DefaultConfig()
	if cfg.Issuer != DefaultIssuer {
		t.Errorf("Issuer = %q, want %q", cfg.Issuer, DefaultIssuer)
	}
	if cfg.ClientID != DefaultClientID {
		t.Errorf("ClientID = %q, want %q", cfg.ClientID, DefaultClientID)
	}

	// With env overrides.
	t.Setenv(EnvIssuer, "https://custom.example.com/")
	t.Setenv(EnvClientID, "custom-client")
	cfg = DefaultConfig()
	if cfg.Issuer != "https://custom.example.com" { // trailing slash trimmed
		t.Errorf("Issuer = %q", cfg.Issuer)
	}
	if cfg.ClientID != "custom-client" {
		t.Errorf("ClientID = %q", cfg.ClientID)
	}
}

func TestLoginEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Mock authorization server.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("unexpected grant_type: %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "test-auth-code" {
			t.Errorf("unexpected code: %s", r.Form.Get("code"))
		}
		if r.Form.Get("code_verifier") == "" {
			t.Error("missing code_verifier in token exchange")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "e2e-access",
			"refresh_token": "e2e-refresh",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"scope":         "offline_access",
		})
	}))
	defer tokenSrv.Close()

	cfg := Config{Issuer: tokenSrv.URL, ClientID: "test-cli"}

	// Our "open browser" function simulates the user completing auth
	// by hitting the callback URL with the authorization code.
	openBrowser := func(authURL string) error {
		// Parse the auth URL to extract state and redirect_uri.
		parsed, err := parseAuthURL(authURL)
		if err != nil {
			return err
		}

		// Simulate the OAuth server redirecting back with a code.
		callbackURL := parsed.redirectURI + "?code=test-auth-code&state=" + parsed.state
		resp, err := http.Get(callbackURL)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, err := Login(ctx, cfg, openBrowser)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if token.AccessToken != "e2e-access" {
		t.Errorf("AccessToken = %q, want e2e-access", token.AccessToken)
	}
	if token.RefreshToken != "e2e-refresh" {
		t.Errorf("RefreshToken = %q, want e2e-refresh", token.RefreshToken)
	}

	// Verify token was persisted.
	stored, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if stored.AccessToken != "e2e-access" {
		t.Errorf("stored AccessToken = %q", stored.AccessToken)
	}
}

// parseAuthURL extracts state and redirect_uri from an authorization URL.
type authURLParams struct {
	state       string
	redirectURI string
}

func parseAuthURL(rawURL string) (*authURLParams, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return &authURLParams{
		state:       u.Query().Get("state"),
		redirectURI: u.Query().Get("redirect_uri"),
	}, nil
}
