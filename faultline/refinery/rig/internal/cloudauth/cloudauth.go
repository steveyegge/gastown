// Package cloudauth implements OAuth2 Authorization Code flow with PKCE
// for authenticating the faultline CLI to faultline.live.
package cloudauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultIssuer is the faultline.live OAuth2 authorization server.
	DefaultIssuer = "https://faultline.live"

	// EnvIssuer overrides the default issuer for development/testing.
	EnvIssuer = "FAULTLINE_CLOUD_ISSUER"

	// EnvClientID overrides the default OAuth2 client ID.
	EnvClientID = "FAULTLINE_CLOUD_CLIENT_ID"

	// DefaultClientID is the public OAuth2 client ID for the faultline CLI.
	DefaultClientID = "faultline-cli"

	// tokenFile is the filename for stored tokens within ~/.faultline/.
	tokenFile = "cloud-token.json"

	// refreshBuffer is how early before expiry we refresh the token.
	refreshBuffer = 5 * time.Minute
)

// Token represents the stored OAuth2 token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// Valid reports whether the access token is present and not expired.
func (t *Token) Valid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Before(t.ExpiresAt)
}

// NeedsRefresh reports whether the token should be refreshed.
func (t *Token) NeedsRefresh() bool {
	return t != nil && t.RefreshToken != "" && time.Now().After(t.ExpiresAt.Add(-refreshBuffer))
}

// Config holds OAuth2 client configuration.
type Config struct {
	Issuer   string // Authorization server base URL
	ClientID string // Public client ID (no secret for PKCE)
}

// DefaultConfig returns configuration from environment or defaults.
func DefaultConfig() Config {
	issuer := os.Getenv(EnvIssuer)
	if issuer == "" {
		issuer = DefaultIssuer
	}
	clientID := os.Getenv(EnvClientID)
	if clientID == "" {
		clientID = DefaultClientID
	}
	return Config{
		Issuer:   strings.TrimRight(issuer, "/"),
		ClientID: clientID,
	}
}

func (c Config) authorizeURL() string { return c.Issuer + "/oauth/authorize" }
func (c Config) tokenURL() string     { return c.Issuer + "/oauth/token" }

// pkce holds PKCE challenge parameters.
type pkce struct {
	Verifier  string
	Challenge string
	Method    string
}

// newPKCE generates a PKCE code verifier and S256 challenge.
func newPKCE() (*pkce, error) {
	// 32 bytes → 43 base64url chars (within 43-128 range per RFC 7636).
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(buf)

	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return &pkce{
		Verifier:  verifier,
		Challenge: challenge,
		Method:    "S256",
	}, nil
}

// Login performs the OAuth2 Authorization Code flow with PKCE:
// 1. Starts a local HTTP server for the callback
// 2. Opens the browser to the authorization URL
// 3. Waits for the callback with the authorization code
// 4. Exchanges the code for tokens
// 5. Stores the tokens at ~/.faultline/cloud-token.json
//
// openBrowser is called with the authorization URL and should open it
// in the user's default browser.
func Login(ctx context.Context, cfg Config, openBrowser func(string) error) (*Token, error) {
	p, err := newPKCE()
	if err != nil {
		return nil, err
	}

	state, err := randomState()
	if err != nil {
		return nil, err
	}

	// Start local callback server on a random port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Build authorization URL.
	authURL := cfg.authorizeURL() + "?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"code_challenge":        {p.Challenge},
		"code_challenge_method": {p.Method},
		"scope":                 {"offline_access"},
	}.Encode()

	type callbackResult struct {
		code string
		err  error
	}
	resultCh := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<h2>Authorization failed</h2><p>%s: %s</p>", errMsg, desc)
			resultCh <- callbackResult{err: fmt.Errorf("authorization denied: %s: %s", errMsg, desc)}
			return
		}

		returnedState := r.URL.Query().Get("state")
		if returnedState != state {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<h2>Authorization failed</h2><p>Invalid state parameter.</p>")
			resultCh <- callbackResult{err: fmt.Errorf("state mismatch: expected %q, got %q", state, returnedState)}
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, "<h2>Authorization failed</h2><p>No authorization code received.</p>")
			resultCh <- callbackResult{err: fmt.Errorf("no authorization code in callback")}
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<h2>Authorization successful!</h2><p>You can close this window.</p>")
		resultCh <- callbackResult{code: code}
	})

	srv := &http.Server{Handler: mux}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(listener)
	}()

	// Open browser to authorization URL.
	if err := openBrowser(authURL); err != nil {
		_ = srv.Close()
		wg.Wait()
		return nil, fmt.Errorf("open browser: %w", err)
	}

	fmt.Println("Waiting for authorization in browser...")

	// Wait for callback or context cancellation.
	var result callbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		_ = srv.Close()
		wg.Wait()
		return nil, ctx.Err()
	}

	// Shut down the callback server.
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	wg.Wait()

	if result.err != nil {
		return nil, result.err
	}

	// Exchange authorization code for tokens.
	token, err := exchangeCode(ctx, cfg, result.code, redirectURI, p.Verifier)
	if err != nil {
		return nil, err
	}

	// Store the token.
	if err := StoreToken(token); err != nil {
		return nil, fmt.Errorf("store token: %w", err)
	}

	return token, nil
}

// exchangeCode exchanges an authorization code for an access token.
func exchangeCode(ctx context.Context, cfg Config, code, redirectURI, codeVerifier string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	return doTokenRequest(ctx, cfg.tokenURL(), data)
}

// RefreshToken uses a refresh token to obtain a new access token.
func RefreshToken(ctx context.Context, cfg Config, refreshToken string) (*Token, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {cfg.ClientID},
		"refresh_token": {refreshToken},
	}

	token, err := doTokenRequest(ctx, cfg.tokenURL(), data)
	if err != nil {
		return nil, err
	}

	if err := StoreToken(token); err != nil {
		return nil, fmt.Errorf("store refreshed token: %w", err)
	}

	return token, nil
}

// doTokenRequest performs a POST to the token endpoint and parses the response.
func doTokenRequest(ctx context.Context, tokenURL string, data url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}

	if body.Error != "" {
		return nil, fmt.Errorf("token exchange failed: %s: %s", body.Error, body.ErrorDesc)
	}

	if body.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token (status %d)", resp.StatusCode)
	}

	return &Token{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		TokenType:    body.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
		Scope:        body.Scope,
	}, nil
}

// LoadToken reads the stored token from ~/.faultline/cloud-token.json.
// Returns nil, nil if no token file exists.
func LoadToken() (*Token, error) {
	path, err := tokenPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("parse token file: %w", err)
	}

	return &token, nil
}

// LoadAndRefresh loads the stored token and refreshes it if expired.
// Returns nil, nil if no token is stored.
func LoadAndRefresh(ctx context.Context, cfg Config) (*Token, error) {
	token, err := LoadToken()
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, nil
	}

	if token.Valid() && !token.NeedsRefresh() {
		return token, nil
	}

	if token.RefreshToken == "" {
		return nil, fmt.Errorf("token expired and no refresh token available; run 'faultline login'")
	}

	return RefreshToken(ctx, cfg, token.RefreshToken)
}

// StoreToken writes the token to ~/.faultline/cloud-token.json with 0600 permissions.
func StoreToken(token *Token) error {
	path, err := tokenPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}

	return nil
}

// RemoveToken deletes the stored token file.
func RemoveToken() error {
	path, err := tokenPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); os.IsNotExist(err) {
		return nil // already gone
	} else if err != nil {
		return fmt.Errorf("remove token file: %w", err)
	}

	return nil
}

func tokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".faultline", tokenFile), nil
}

func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
