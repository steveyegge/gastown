package cloudauth

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Handler serves the cloud auth HTTP API.
type Handler struct {
	Store         *Store
	Log           *slog.Logger
	loginAttempts map[string][]time.Time // IP → recent attempt timestamps
}

// RegisterRoutes adds cloud auth routes to the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.handleRegister)
	mux.HandleFunc("POST /auth/login", h.handleLogin)
	mux.HandleFunc("GET /auth/profile", h.handleProfile)
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if !validEmail(req.Email) {
		writeErr(w, http.StatusBadRequest, "invalid email address")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	// First account gets owner role automatically.
	role := "member"
	count, err := h.Store.AccountCount(r.Context())
	if err != nil {
		h.Log.Error("account count failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if count == 0 {
		role = "owner"
	}

	account, err := h.Store.CreateAccount(r.Context(), req.Email, req.Name, req.Password, role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeErr(w, http.StatusConflict, "email already registered")
			return
		}
		h.Log.Error("create account failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	token, err := h.Store.CreateSession(r.Context(), account.ID)
	if err != nil {
		h.Log.Error("create session failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.Log.Info("account registered", "email", account.Email, "role", account.Role)
	writeJSON(w, http.StatusCreated, map[string]any{
		"session_token": token,
		"account":       account,
	})
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if h.checkLoginRate(ip) {
		w.Header().Set("Retry-After", "900")
		writeErr(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	h.recordLoginAttempt(ip)

	account, err := h.Store.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.Store.CreateSession(r.Context(), account.ID)
	if err != nil {
		h.Log.Error("create session failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	h.Log.Info("account logged in", "email", account.Email)
	writeJSON(w, http.StatusOK, map[string]any{
		"session_token": token,
		"account":       account,
	})
}

func (h *Handler) handleProfile(w http.ResponseWriter, r *http.Request) {
	token := extractBearer(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing authorization header")
		return
	}

	account, err := h.Store.GetSession(r.Context(), token)
	if err != nil {
		h.Log.Error("get session failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if account == nil {
		writeErr(w, http.StatusUnauthorized, "invalid or expired session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"account": account,
	})
}

// checkLoginRate returns true if the IP has exceeded 5 attempts in the last 15 minutes.
func (h *Handler) checkLoginRate(ip string) bool {
	if h.loginAttempts == nil {
		h.loginAttempts = make(map[string][]time.Time)
	}
	cutoff := time.Now().Add(-15 * time.Minute)
	attempts := h.loginAttempts[ip]
	var recent []time.Time
	for _, t := range attempts {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	h.loginAttempts[ip] = recent
	return len(recent) >= 5
}

func (h *Handler) recordLoginAttempt(ip string) {
	if h.loginAttempts == nil {
		h.loginAttempts = make(map[string][]time.Time)
	}
	h.loginAttempts[ip] = append(h.loginAttempts[ip], time.Now())
}

func extractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

var emailRegexp = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

func validEmail(email string) bool {
	return emailRegexp.MatchString(email)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
