package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/outdoorsea/faultline/internal/db"
)

// WebhookHandler handles incoming Slack interaction payloads.
type WebhookHandler struct {
	SigningSecret string // Slack app signing secret for request verification
	DB           *db.DB
	Log          *slog.Logger
}

// RegisterRoutes adds the Slack webhook endpoint to the mux.
func (h *WebhookHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/hooks/slack", h.HandleInteraction)
	mux.HandleFunc("POST /api/hooks/slack/", h.HandleInteraction)
}

// HandleInteraction processes a Slack interaction webhook payload.
func (h *WebhookHandler) HandleInteraction(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	// Verify Slack request signature.
	if h.SigningSecret != "" {
		if !h.verifySlackSignature(body, r.Header) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Slack sends interaction payloads as application/x-www-form-urlencoded
	// with a "payload" field containing JSON.
	if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		if err := r.ParseForm(); err == nil {
			if p := r.FormValue("payload"); p != "" {
				body = []byte(p)
			}
		}
	}

	var payload interactionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.Log.Warn("slack webhook: invalid payload", "err", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	h.Log.Info("slack interaction received",
		"type", payload.Type,
		"user", payload.User.ID,
		"callback_id", payload.CallbackID,
	)

	// Acknowledge the interaction.
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// verifySlackSignature validates the X-Slack-Signature header using HMAC-SHA256.
// See https://api.slack.com/authentication/verifying-requests-from-slack
func (h *WebhookHandler) verifySlackSignature(body []byte, headers http.Header) bool {
	timestamp := headers.Get("X-Slack-Request-Timestamp")
	signature := headers.Get("X-Slack-Signature")

	if timestamp == "" || signature == "" {
		return false
	}

	// Reject requests older than 5 minutes to prevent replay attacks.
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if abs(time.Now().Unix()-ts) > 300 {
		return false
	}

	// Compute expected signature: v0=HMAC-SHA256(signing_secret, "v0:{timestamp}:{body}")
	sigBaseString := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(h.SigningSecret))
	mac.Write([]byte(sigBaseString))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// interactionPayload represents a Slack interaction callback.
type interactionPayload struct {
	Type       string `json:"type"`
	CallbackID string `json:"callback_id"`
	TriggerID  string `json:"trigger_id"`
	User       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"user"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
}
