package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Auth maps public keys to project IDs for the relay.
type Auth struct {
	keyToProject map[string]int64
}

// NewAuth creates relay auth from "project_id:public_key" pairs.
func NewAuth(pairs []string) (*Auth, error) {
	keys := make(map[string]int64, len(pairs))
	for _, p := range pairs {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid relay project pair: %q (want project_id:public_key)", p)
		}
		var id int64
		if _, err := fmt.Sscanf(parts[0], "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid project id in %q: %w", p, err)
		}
		keys[parts[1]] = id
	}
	return &Auth{keyToProject: keys}, nil
}

// Handler serves the relay HTTP API.
type Handler struct {
	Store    *Store
	Auth     *Auth
	Log      *slog.Logger
	PollToken string // shared secret required for poll/ack endpoints
}

// RegisterRoutes adds relay routes to the mux.
// Accepts all Sentry SDK endpoint patterns to be fully future-proof.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Standard Sentry SDK endpoints — all stored as raw payloads.
	ingest := h.cors(h.HandleIngest)
	mux.HandleFunc("POST /api/{project_id}/envelope/", ingest)
	mux.HandleFunc("POST /api/{project_id}/envelope", ingest)
	mux.HandleFunc("POST /api/{project_id}/store/", ingest)
	mux.HandleFunc("POST /api/{project_id}/store", ingest)
	mux.HandleFunc("POST /api/{project_id}/security/", ingest)
	mux.HandleFunc("POST /api/{project_id}/security", ingest)
	mux.HandleFunc("POST /api/{project_id}/minidump/", ingest)
	mux.HandleFunc("POST /api/{project_id}/minidump", ingest)
	mux.HandleFunc("POST /api/{project_id}/unreal/", ingest)
	mux.HandleFunc("POST /api/{project_id}/unreal", ingest)

	// CORS preflight for browser SDKs.
	mux.HandleFunc("OPTIONS /api/{project_id}/envelope/", h.handlePreflight)
	mux.HandleFunc("OPTIONS /api/{project_id}/envelope", h.handlePreflight)
	mux.HandleFunc("OPTIONS /api/{project_id}/store/", h.handlePreflight)
	mux.HandleFunc("OPTIONS /api/{project_id}/store", h.handlePreflight)

	// CI webhooks — stored raw for local faultline to process.
	mux.HandleFunc("POST /api/hooks/ci/github", h.HandleCIWebhook)
	mux.HandleFunc("POST /api/hooks/ci/github/", h.HandleCIWebhook)

	// Relay management API (internal, token-authenticated).
	mux.HandleFunc("GET /relay/poll", h.requirePollToken(h.HandlePoll))
	mux.HandleFunc("POST /relay/ack", h.requirePollToken(h.HandleAck))
	mux.HandleFunc("GET /health", h.HandleHealth)
}

// requirePollToken wraps a handler with bearer token authentication.
// If PollToken is empty, the check is skipped (open access).
func (h *Handler) requirePollToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.PollToken != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+h.PollToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// cors wraps a handler with CORS headers for browser SDK compatibility.
func (h *Handler) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Sentry-Auth, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		next(w, r)
	}
}

// handlePreflight responds to CORS preflight requests.
func (h *Handler) handlePreflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Sentry-Auth, Authorization")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

// HandleIngest accepts a Sentry envelope/store request and stores the raw body.
func (h *Handler) HandleIngest(w http.ResponseWriter, r *http.Request) {
	// Authenticate via sentry key.
	key := extractKey(r)
	if key == "" {
		http.Error(w, "missing sentry authentication", http.StatusUnauthorized)
		return
	}

	projectID, ok := h.Auth.keyToProject[key]
	if !ok {
		http.Error(w, "invalid sentry key", http.StatusUnauthorized)
		return
	}

	// Verify project_id in URL matches auth (if specified).
	if pidStr := r.PathValue("project_id"); pidStr != "" {
		urlPID, err := strconv.ParseInt(pidStr, 10, 64)
		if err == nil && urlPID != projectID {
			http.Error(w, "project id mismatch", http.StatusForbidden)
			return
		}
	}

	// Read body (limit to 10MB — minidumps and attachments can be large).
	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	id, err := h.Store.Insert(projectID, key, body)
	if err != nil {
		h.Log.Error("store insert failed", "err", err)
		w.Header().Set("Retry-After", "30")
		http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
		return
	}

	h.Log.Info("envelope stored", "id", id, "project", projectID, "size", len(body))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
}

// HandlePoll returns unpulled envelopes. Query params: since (ID), limit.
func (h *Handler) HandlePoll(w http.ResponseWriter, r *http.Request) {
	sinceID, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	envelopes, err := h.Store.Poll(sinceID, limit)
	if err != nil {
		h.Log.Error("poll failed", "err", err)
		http.Error(w, "poll error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"envelopes": envelopes,
		"count":     len(envelopes),
	})
}

// HandleAck marks envelopes as pulled. Body: {"ids": [1, 2, 3]}
func (h *Handler) HandleAck(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	acked, err := h.Store.Ack(req.IDs)
	if err != nil {
		h.Log.Error("ack failed", "err", err)
		http.Error(w, "ack error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"acked": acked})
}

// HandleHealth returns relay health status.
// Returns 503 with Retry-After when SQLite is unavailable.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	total, unpulled, err := h.Store.Stats()
	status := "ok"
	code := http.StatusOK
	if err != nil {
		status = "degraded"
		code = http.StatusServiceUnavailable
		w.Header().Set("Retry-After", "30")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":   status,
		"total":    total,
		"unpulled": unpulled,
	})
}

// HandleCIWebhook accepts GitHub webhook payloads and stores them for the local
// faultline to poll and process. Uses project_id=0 as a special marker for CI events.
func (h *Handler) HandleCIWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if len(body) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// Store with project_id=0 and the GitHub event type as the key.
	// The local faultline CI handler will process these on poll.
	eventType := r.Header.Get("X-GitHub-Event")
	signature := r.Header.Get("X-Hub-Signature-256")

	// Wrap the payload with headers so the local handler can verify and route.
	wrapped := map[string]interface{}{
		"type":      "ci_webhook",
		"source":    "github",
		"event":     eventType,
		"signature": signature,
		"payload":   json.RawMessage(body),
	}
	wrappedJSON, _ := json.Marshal(wrapped)

	id, err := h.Store.Insert(0, "ci-webhook", wrappedJSON)
	if err != nil {
		h.Log.Error("store ci webhook failed", "err", err)
		w.Header().Set("Retry-After", "30")
		http.Error(w, "storage unavailable", http.StatusServiceUnavailable)
		return
	}

	h.Log.Info("ci webhook stored", "id", id, "event", eventType, "size", len(body))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
}

// extractKey pulls the sentry_key from the request (same logic as main faultline).
func extractKey(r *http.Request) string {
	for _, hdr := range []string{"X-Sentry-Auth", "Authorization"} {
		if v := r.Header.Get(hdr); v != "" {
			v = strings.TrimPrefix(v, "Sentry ")
			v = strings.TrimPrefix(v, "sentry ")
			for _, part := range strings.Split(v, ",") {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "sentry_key=") {
					return strings.TrimPrefix(part, "sentry_key=")
				}
			}
		}
	}
	return r.URL.Query().Get("sentry_key")
}
