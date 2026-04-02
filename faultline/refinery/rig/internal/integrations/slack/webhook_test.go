package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestVerifySlackSignature(t *testing.T) {
	secret := "test-signing-secret-abc123"
	handler := &WebhookHandler{SigningSecret: secret}

	body := []byte(`{"type":"block_actions","user":{"id":"U12345"}}`)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	// Compute valid signature.
	sigBase := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sigBase))
	validSig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		timestamp string
		signature string
		want      bool
	}{
		{"valid signature", timestamp, validSig, true},
		{"wrong signature", timestamp, "v0=deadbeef", false},
		{"missing signature", timestamp, "", false},
		{"missing timestamp", "", validSig, false},
		{"old timestamp (replay)", fmt.Sprintf("%d", time.Now().Unix()-600), validSig, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			headers.Set("X-Slack-Request-Timestamp", tt.timestamp)
			headers.Set("X-Slack-Signature", tt.signature)

			got := handler.verifySlackSignature(body, headers)
			if got != tt.want {
				t.Errorf("verifySlackSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifySlackSignature_RejectsFuture(t *testing.T) {
	secret := "test-secret"
	handler := &WebhookHandler{SigningSecret: secret}

	body := []byte(`{}`)
	// Timestamp 10 minutes in the future.
	timestamp := fmt.Sprintf("%d", time.Now().Unix()+600)

	sigBase := fmt.Sprintf("v0:%s:%s", timestamp, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sigBase))
	sig := "v0=" + hex.EncodeToString(mac.Sum(nil))

	headers := http.Header{}
	headers.Set("X-Slack-Request-Timestamp", timestamp)
	headers.Set("X-Slack-Signature", sig)

	if handler.verifySlackSignature(body, headers) {
		t.Error("should reject future timestamps")
	}
}
