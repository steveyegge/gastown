package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) (*Server, *MemStore) {
	t.Helper()
	store := NewMemStore()
	store.RegisterClient(&Client{
		ID:           "test-client",
		Secret:       HashClientSecret("test-secret"),
		RedirectURIs: []string{"http://localhost:3000/callback"},
		Public:       false,
		Name:         "Test App",
	})
	store.RegisterClient(&Client{
		ID:           "public-client",
		RedirectURIs: []string{"http://localhost:3000/callback"},
		Public:       true,
		Name:         "Public App",
	})

	consent := func(r *http.Request, client *Client, scopes []string) (string, error) {
		return "user-123", nil
	}

	srv := NewServer(store, consent)
	return srv, store
}

func TestAuthorizationCodeFlow(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Step 1: Authorize.
	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type": {"code"},
		"client_id":     {"test-client"},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"scope":         {"relay:read account:read"},
		"state":         {"xyz"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("authorize: expected 302, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	code := u.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}
	if u.Query().Get("state") != "xyz" {
		t.Fatalf("state mismatch: %s", u.Query().Get("state"))
	}

	// Step 2: Exchange code for tokens.
	tokenReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"client_id":     {"test-client"},
		"client_secret": {"test-secret"},
	}.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, tokenReq)

	if w.Code != http.StatusOK {
		t.Fatalf("token: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pair TokenPair
	if err := json.NewDecoder(w.Body).Decode(&pair); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if pair.AccessToken == "" {
		t.Fatal("no access_token")
	}
	if pair.RefreshToken == "" {
		t.Fatal("no refresh_token")
	}
	if pair.TokenType != "Bearer" {
		t.Fatalf("token_type: %s", pair.TokenType)
	}
	if pair.ExpiresIn != 3600 {
		t.Fatalf("expires_in: %d", pair.ExpiresIn)
	}

	// Step 3: Introspect.
	introReq := httptest.NewRequest("POST", "/oauth2/introspect", strings.NewReader(url.Values{
		"token": {pair.AccessToken},
	}.Encode()))
	introReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, introReq)

	var intro IntrospectionResponse
	if err := json.NewDecoder(w.Body).Decode(&intro); err != nil {
		t.Fatalf("decode introspect: %v", err)
	}
	if !intro.Active {
		t.Fatal("token should be active")
	}
	if intro.Sub != "user-123" {
		t.Fatalf("sub: %s", intro.Sub)
	}
	if intro.ClientID != "test-client" {
		t.Fatalf("client_id: %s", intro.ClientID)
	}
}

func TestPKCEFlow(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	challenge := S256Challenge(verifier)

	// Authorize with PKCE.
	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {"public-client"},
		"redirect_uri":          {"http://localhost:3000/callback"},
		"scope":                 {"relay:read"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("authorize: expected 302, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	code := u.Query().Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}

	// Exchange with correct verifier.
	tokenReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"client_id":     {"public-client"},
		"code_verifier": {verifier},
	}.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, tokenReq)

	if w.Code != http.StatusOK {
		t.Fatalf("token: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var pair TokenPair
	if err := json.NewDecoder(w.Body).Decode(&pair); err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken == "" {
		t.Fatal("no access_token")
	}
}

func TestPKCERequiredForPublicClients(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Public client without PKCE should be rejected.
	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type": {"code"},
		"client_id":     {"public-client"},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"scope":         {"relay:read"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	if u.Query().Get("error") != "invalid_request" {
		t.Fatalf("expected invalid_request error, got: %s", loc)
	}
}

func TestPKCEWrongVerifier(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	verifier, _ := GenerateCodeVerifier()
	challenge := S256Challenge(verifier)

	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {"public-client"},
		"redirect_uri":          {"http://localhost:3000/callback"},
		"scope":                 {"relay:read"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	code := u.Query().Get("code")

	// Wrong verifier.
	tokenReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"client_id":     {"public-client"},
		"code_verifier": {"wrong-verifier-value"},
	}.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, tokenReq)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRefreshTokenFlow(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Get tokens via code flow.
	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type": {"code"},
		"client_id":     {"test-client"},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"scope":         {"relay:read relay:write"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	code := u.Query().Get("code")

	tokenReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"client_id":     {"test-client"},
		"client_secret": {"test-secret"},
	}.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, tokenReq)

	var pair TokenPair
	_ = json.NewDecoder(w.Body).Decode(&pair)

	// Refresh with scope narrowing.
	refreshReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {pair.RefreshToken},
		"client_id":     {"test-client"},
		"scope":         {"relay:read"},
	}.Encode()))
	refreshReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, refreshReq)

	if w.Code != http.StatusOK {
		t.Fatalf("refresh: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var newPair TokenPair
	_ = json.NewDecoder(w.Body).Decode(&newPair)
	if newPair.AccessToken == "" {
		t.Fatal("no access_token after refresh")
	}
	if newPair.Scope != "relay:read" {
		t.Fatalf("expected narrowed scope, got: %s", newPair.Scope)
	}

	// Old refresh token should be revoked (rotation).
	refreshReq2 := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {pair.RefreshToken},
		"client_id":     {"test-client"},
	}.Encode()))
	refreshReq2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, refreshReq2)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("old refresh token should be rejected, got %d", w.Code)
	}
}

func TestRefreshScopeExpansionDenied(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Get tokens with limited scope.
	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type": {"code"},
		"client_id":     {"test-client"},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"scope":         {"relay:read"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	code := u.Query().Get("code")

	tokenReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"client_id":     {"test-client"},
		"client_secret": {"test-secret"},
	}.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, tokenReq)

	var pair TokenPair
	_ = json.NewDecoder(w.Body).Decode(&pair)

	// Try to expand scope on refresh.
	refreshReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {pair.RefreshToken},
		"client_id":     {"test-client"},
		"scope":         {"relay:read relay:write"},
	}.Encode()))
	refreshReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, refreshReq)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("scope expansion should be rejected, got %d", w.Code)
	}
}

func TestIntrospectInvalidToken(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/oauth2/introspect", strings.NewReader(url.Values{
		"token": {"nonexistent-token"},
	}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var intro IntrospectionResponse
	_ = json.NewDecoder(w.Body).Decode(&intro)
	if intro.Active {
		t.Fatal("nonexistent token should be inactive")
	}
}

func TestCodeReuse(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type": {"code"},
		"client_id":     {"test-client"},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"scope":         {"relay:read"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	code := u.Query().Get("code")

	// First exchange.
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"client_id":     {"test-client"},
		"client_secret": {"test-secret"},
	}
	tokenReq := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(body.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, tokenReq)
	if w.Code != http.StatusOK {
		t.Fatalf("first exchange: %d", w.Code)
	}

	// Second exchange with same code should fail.
	tokenReq2 := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(body.Encode()))
	tokenReq2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, tokenReq2)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code reuse should fail, got %d", w.Code)
	}
}

func TestInvalidScope(t *testing.T) {
	srv, _ := setupTestServer(t)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/oauth2/authorize?"+url.Values{
		"response_type": {"code"},
		"client_id":     {"test-client"},
		"redirect_uri":  {"http://localhost:3000/callback"},
		"scope":         {"nonexistent:scope"},
	}.Encode(), nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	u, _ := url.Parse(loc)
	if u.Query().Get("error") != "invalid_scope" {
		t.Fatalf("expected invalid_scope error, got: %s", loc)
	}
}

func TestPKCEHelpers(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(verifier) == 0 {
		t.Fatal("empty verifier")
	}

	challenge := S256Challenge(verifier)
	if challenge == "" {
		t.Fatal("empty challenge")
	}

	if !verifyPKCE(challenge, "S256", verifier) {
		t.Fatal("S256 verification should succeed")
	}
	if verifyPKCE(challenge, "S256", "wrong") {
		t.Fatal("S256 verification should fail with wrong verifier")
	}
	if !verifyPKCE("plain-value", "plain", "plain-value") {
		t.Fatal("plain verification should succeed")
	}
}
