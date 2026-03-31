package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// registerTokenRoutes adds API token management endpoints.
func (h *Handler) registerTokenRoutes(mux *http.ServeMux) {
	auth := h.requireBearerWithAccount

	mux.HandleFunc("POST /api/v1/tokens", auth(h.createToken))
	mux.HandleFunc("POST /api/v1/tokens/", auth(h.createToken))
	mux.HandleFunc("GET /api/v1/tokens", auth(h.listTokens))
	mux.HandleFunc("GET /api/v1/tokens/", auth(h.listTokens))
	mux.HandleFunc("DELETE /api/v1/tokens/{token_id}", auth(h.revokeToken))
	mux.HandleFunc("DELETE /api/v1/tokens/{token_id}/", auth(h.revokeToken))
}

// requireBearerWithAccount validates auth and passes the account ID via header.
func (h *Handler) requireBearerWithAccount(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="faultline"`)
			writeErr(w, http.StatusUnauthorized, "missing or invalid Authorization header")
			return
		}

		// Try session token first.
		account, err := h.DB.GetSession(r.Context(), token)
		if err != nil || account == nil {
			// Try API token.
			account, err = h.DB.ValidateAPIToken(r.Context(), token)
		}
		if err != nil || account == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="faultline"`)
			writeErr(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		r.Header.Set("X-Account-ID", formatInt64(account.ID))
		next(w, r)
	}
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	accountID := headerInt64(r, "X-Account-ID")
	if accountID <= 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		Name      string `json:"name"`
		ProjectID *int64 `json:"project_id,omitempty"`
		ExpiresIn *int   `json:"expires_in_days,omitempty"` // days from now, nil = no expiry
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	var expiresAt *time.Time
	if body.ExpiresIn != nil && *body.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(*body.ExpiresIn) * 24 * time.Hour)
		expiresAt = &t
	}

	plaintext, token, err := h.DB.CreateAPIToken(r.Context(), accountID, body.ProjectID, body.Name, expiresAt)
	if err != nil {
		h.Log.Error("create api token", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":     plaintext, // shown only once
		"token_info": token,
	})
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	accountID := headerInt64(r, "X-Account-ID")
	if accountID <= 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokens, err := h.DB.ListAPITokens(r.Context(), accountID)
	if err != nil {
		h.Log.Error("list api tokens", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"tokens": tokens,
	})
}

func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request) {
	accountID := headerInt64(r, "X-Account-ID")
	if accountID <= 0 {
		writeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tokenID := pathInt64(r, "token_id")
	if tokenID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid token_id")
		return
	}

	if err := h.DB.RevokeAPIToken(r.Context(), tokenID, accountID); err != nil {
		writeErr(w, http.StatusNotFound, "token not found or already revoked")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func formatInt64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func headerInt64(r *http.Request, name string) int64 {
	v, _ := strconv.ParseInt(r.Header.Get(name), 10, 64)
	return v
}
