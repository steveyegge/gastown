// Package oauth2 implements an OAuth2 Authorization Code flow server with
// PKCE support (RFC 7636) and token introspection (RFC 7662).
//
// It is a standalone package with no project-specific dependencies,
// designed for reuse across web projects.
package oauth2

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Scope constants for faultline.
const (
	ScopeRelayRead  = "relay:read"
	ScopeRelayWrite = "relay:write"
	ScopeSlackSend  = "slack:send"
	ScopeAccountRead = "account:read"
)

// AllScopes is the set of all defined scopes.
var AllScopes = []string{ScopeRelayRead, ScopeRelayWrite, ScopeSlackSend, ScopeAccountRead}

// DefaultAccessTokenTTL is the default access token lifetime.
const DefaultAccessTokenTTL = 1 * time.Hour

// DefaultRefreshTokenTTL is the default refresh token lifetime.
const DefaultRefreshTokenTTL = 30 * 24 * time.Hour

// DefaultCodeTTL is the lifetime of an authorization code.
const DefaultCodeTTL = 10 * time.Minute

// Client represents a registered OAuth2 client.
type Client struct {
	ID           string   `json:"client_id"`
	Secret       string   `json:"-"` // hashed; empty for public clients
	RedirectURIs []string `json:"redirect_uris"`
	Public       bool     `json:"public"` // public clients require PKCE
	Name         string   `json:"name"`
}

// AuthorizationCode represents a pending authorization code exchange.
type AuthorizationCode struct {
	Code                string
	ClientID            string
	AccountID           string
	RedirectURI         string
	Scopes              []string
	CodeChallenge       string // PKCE
	CodeChallengeMethod string // "S256" or "plain"
	ExpiresAt           time.Time
}

// TokenPair holds an access token and optional refresh token.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

// AccessToken represents a stored access token.
type AccessToken struct {
	Token     string
	ClientID  string
	AccountID string
	Scopes    []string
	ExpiresAt time.Time
}

// RefreshToken represents a stored refresh token.
type RefreshToken struct {
	Token     string
	ClientID  string
	AccountID string
	Scopes    []string
	ExpiresAt time.Time
}

// IntrospectionResponse is the RFC 7662 response.
type IntrospectionResponse struct {
	Active    bool   `json:"active"`
	Scope     string `json:"scope,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	Sub       string `json:"sub,omitempty"`
	Exp       int64  `json:"exp,omitempty"`
	Iat       int64  `json:"iat,omitempty"`
	TokenType string `json:"token_type,omitempty"`
}

// Store is the persistence interface for the OAuth2 server.
// Implement this to back the server with any database.
type Store interface {
	GetClient(id string) (*Client, error)
	SaveAuthorizationCode(code *AuthorizationCode) error
	GetAuthorizationCode(code string) (*AuthorizationCode, error)
	DeleteAuthorizationCode(code string) error
	SaveAccessToken(token *AccessToken) error
	GetAccessToken(token string) (*AccessToken, error)
	RevokeAccessToken(token string) error
	SaveRefreshToken(token *RefreshToken) error
	GetRefreshToken(token string) (*RefreshToken, error)
	RevokeRefreshToken(token string) error
}

// ConsentFunc is called during authorization to verify the user has approved
// the request. It receives the HTTP request, client, and requested scopes.
// It must return the authenticated account ID, or an error to deny.
type ConsentFunc func(r *http.Request, client *Client, scopes []string) (accountID string, err error)

// Server implements OAuth2 Authorization Code flow endpoints.
type Server struct {
	Store           Store
	Consent         ConsentFunc
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CodeTTL         time.Duration
}

// NewServer creates a server with default TTLs.
func NewServer(store Store, consent ConsentFunc) *Server {
	return &Server{
		Store:           store,
		Consent:         consent,
		AccessTokenTTL:  DefaultAccessTokenTTL,
		RefreshTokenTTL: DefaultRefreshTokenTTL,
		CodeTTL:         DefaultCodeTTL,
	}
}

// RegisterRoutes registers OAuth2 endpoints on the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /oauth2/authorize", s.HandleAuthorize)
	mux.HandleFunc("POST /oauth2/authorize", s.HandleAuthorize)
	mux.HandleFunc("POST /oauth2/token", s.HandleToken)
	mux.HandleFunc("POST /oauth2/introspect", s.HandleIntrospect)
}

// HandleAuthorize handles the authorization endpoint.
func (s *Server) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	responseType := r.FormValue("response_type")
	scope := r.FormValue("scope")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")

	if responseType != "code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_response_type", "only 'code' is supported")
		return
	}

	client, err := s.Store.GetClient(clientID)
	if err != nil || client == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "unknown client_id")
		return
	}

	if !isValidRedirectURI(client, redirectURI) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
		return
	}

	// Public clients must use PKCE.
	if client.Public && codeChallenge == "" {
		redirectWithError(w, r, redirectURI, state, "invalid_request", "PKCE required for public clients")
		return
	}

	if codeChallengeMethod == "" && codeChallenge != "" {
		codeChallengeMethod = "S256"
	}
	if codeChallenge != "" && codeChallengeMethod != "S256" && codeChallengeMethod != "plain" {
		redirectWithError(w, r, redirectURI, state, "invalid_request", "unsupported code_challenge_method")
		return
	}

	scopes := parseScopes(scope)
	if !validScopes(scopes) {
		redirectWithError(w, r, redirectURI, state, "invalid_scope", "one or more scopes are invalid")
		return
	}

	accountID, err := s.Consent(r, client, scopes)
	if err != nil {
		redirectWithError(w, r, redirectURI, state, "access_denied", err.Error())
		return
	}

	code, err := generateToken(32)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to generate code")
		return
	}

	ttl := s.CodeTTL
	if ttl == 0 {
		ttl = DefaultCodeTTL
	}

	authCode := &AuthorizationCode{
		Code:                code,
		ClientID:            clientID,
		AccountID:           accountID,
		RedirectURI:         redirectURI,
		Scopes:              scopes,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().UTC().Add(ttl),
	}
	if err := s.Store.SaveAuthorizationCode(authCode); err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to store code")
		return
	}

	q := fmt.Sprintf("code=%s", code)
	if state != "" {
		q += "&state=" + state
	}
	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	http.Redirect(w, r, redirectURI+sep+q, http.StatusFound)
}

// HandleToken handles the token endpoint (code exchange and refresh).
func (s *Server) HandleToken(w http.ResponseWriter, r *http.Request) {
	grantType := r.FormValue("grant_type")

	switch grantType {
	case "authorization_code":
		s.handleCodeExchange(w, r)
	case "refresh_token":
		s.handleRefresh(w, r)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "use authorization_code or refresh_token")
	}
}

func (s *Server) handleCodeExchange(w http.ResponseWriter, r *http.Request) {
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	codeVerifier := r.FormValue("code_verifier")

	authCode, err := s.Store.GetAuthorizationCode(code)
	if err != nil || authCode == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid or expired authorization code")
		return
	}

	// Code is single-use: delete immediately.
	_ = s.Store.DeleteAuthorizationCode(code)

	if time.Now().After(authCode.ExpiresAt) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code expired")
		return
	}

	if authCode.ClientID != clientID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}

	if authCode.RedirectURI != redirectURI {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}

	client, err := s.Store.GetClient(clientID)
	if err != nil || client == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client")
		return
	}

	// Authenticate confidential clients.
	if !client.Public {
		if clientSecret == "" {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client_secret required")
			return
		}
		if !verifyClientSecret(client, clientSecret) {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "invalid client_secret")
			return
		}
	}

	// Verify PKCE.
	if authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier required")
			return
		}
		if !verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
			return
		}
	}

	pair, err := s.issueTokens(client.ID, authCode.AccountID, authCode.Scopes)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to issue tokens")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(pair)
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	refreshToken := r.FormValue("refresh_token")
	clientID := r.FormValue("client_id")

	rt, err := s.Store.GetRefreshToken(refreshToken)
	if err != nil || rt == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "invalid refresh token")
		return
	}

	if time.Now().After(rt.ExpiresAt) {
		_ = s.Store.RevokeRefreshToken(refreshToken)
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "refresh token expired")
		return
	}

	if rt.ClientID != clientID {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")
		return
	}

	// Revoke old refresh token (rotation).
	_ = s.Store.RevokeRefreshToken(refreshToken)

	// Optionally narrow scopes on refresh.
	scopes := rt.Scopes
	if reqScope := r.FormValue("scope"); reqScope != "" {
		requested := parseScopes(reqScope)
		if !isSubset(requested, scopes) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_scope", "cannot expand scope on refresh")
			return
		}
		scopes = requested
	}

	pair, err := s.issueTokens(rt.ClientID, rt.AccountID, scopes)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to issue tokens")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(pair)
}

// HandleIntrospect implements RFC 7662 token introspection.
func (s *Server) HandleIntrospect(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	if token == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(IntrospectionResponse{Active: false})
		return
	}

	at, err := s.Store.GetAccessToken(token)
	if err != nil || at == nil || time.Now().After(at.ExpiresAt) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(IntrospectionResponse{Active: false})
		return
	}

	resp := IntrospectionResponse{
		Active:    true,
		Scope:     strings.Join(at.Scopes, " "),
		ClientID:  at.ClientID,
		Sub:       at.AccountID,
		Exp:       at.ExpiresAt.Unix(),
		Iat:       at.ExpiresAt.Add(-s.accessTTL()).Unix(),
		TokenType: "Bearer",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) issueTokens(clientID, accountID string, scopes []string) (*TokenPair, error) {
	accessRaw, err := generateToken(32)
	if err != nil {
		return nil, err
	}
	refreshRaw, err := generateToken(32)
	if err != nil {
		return nil, err
	}

	atTTL := s.accessTTL()
	rtTTL := s.refreshTTL()

	at := &AccessToken{
		Token:     accessRaw,
		ClientID:  clientID,
		AccountID: accountID,
		Scopes:    scopes,
		ExpiresAt: time.Now().UTC().Add(atTTL),
	}
	if err := s.Store.SaveAccessToken(at); err != nil {
		return nil, err
	}

	rtk := &RefreshToken{
		Token:     refreshRaw,
		ClientID:  clientID,
		AccountID: accountID,
		Scopes:    scopes,
		ExpiresAt: time.Now().UTC().Add(rtTTL),
	}
	if err := s.Store.SaveRefreshToken(rtk); err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessRaw,
		TokenType:    "Bearer",
		ExpiresIn:    int(atTTL.Seconds()),
		RefreshToken: refreshRaw,
		Scope:        strings.Join(scopes, " "),
	}, nil
}

func (s *Server) accessTTL() time.Duration {
	if s.AccessTokenTTL > 0 {
		return s.AccessTokenTTL
	}
	return DefaultAccessTokenTTL
}

func (s *Server) refreshTTL() time.Duration {
	if s.RefreshTokenTTL > 0 {
		return s.RefreshTokenTTL
	}
	return DefaultRefreshTokenTTL
}

// --- PKCE ---

// verifyPKCE checks the code verifier against the stored challenge.
func verifyPKCE(challenge, method, verifier string) bool {
	switch method {
	case "S256":
		h := sha256.Sum256([]byte(verifier))
		computed := base64.RawURLEncoding.EncodeToString(h[:])
		return computed == challenge
	case "plain":
		return verifier == challenge
	default:
		return false
	}
}

// GenerateCodeVerifier creates a random PKCE code verifier.
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// S256Challenge computes the S256 code challenge for a verifier.
func S256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// --- helpers ---

func generateToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func parseScopes(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

var validScopeSet = func() map[string]bool {
	m := make(map[string]bool, len(AllScopes))
	for _, s := range AllScopes {
		m[s] = true
	}
	return m
}()

func validScopes(scopes []string) bool {
	for _, s := range scopes {
		if !validScopeSet[s] {
			return false
		}
	}
	return true
}

func isSubset(requested, allowed []string) bool {
	set := make(map[string]bool, len(allowed))
	for _, s := range allowed {
		set[s] = true
	}
	for _, s := range requested {
		if !set[s] {
			return false
		}
	}
	return true
}

func isValidRedirectURI(client *Client, uri string) bool {
	for _, u := range client.RedirectURIs {
		if u == uri {
			return true
		}
	}
	return false
}

func verifyClientSecret(client *Client, secret string) bool {
	h := sha256.Sum256([]byte(secret))
	return client.Secret == hex.EncodeToString(h[:])
}

// HashClientSecret returns the SHA-256 hex hash for storing a client secret.
func HashClientSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

func writeOAuthError(w http.ResponseWriter, status int, errCode, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": desc,
	})
}

func redirectWithError(w http.ResponseWriter, r *http.Request, uri, state, errCode, desc string) {
	sep := "?"
	if strings.Contains(uri, "?") {
		sep = "&"
	}
	q := fmt.Sprintf("error=%s&error_description=%s", errCode, desc)
	if state != "" {
		q += "&state=" + state
	}
	http.Redirect(w, r, uri+sep+q, http.StatusFound)
}
