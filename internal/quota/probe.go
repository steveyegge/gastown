package quota

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ProbeStatus string

const (
	ProbeUsable  ProbeStatus = "usable"
	ProbeLimited ProbeStatus = "limited"
	ProbeInvalid ProbeStatus = "invalid"
	ProbeError   ProbeStatus = "error"
)

type ProbeResult struct {
	Status     ProbeStatus
	HTTPCode   int
	StatusText string
}

const DefaultProbeURL = "https://api.anthropic.com/v1/messages"

// ProbeAPIKey sends a malformed request (empty messages array) to the Anthropic API
// to check if a key/token is usable without consuming any tokens.
// url is the API endpoint (use DefaultProbeURL for production, or a test server URL).
// key is the API key or OAuth token.
func ProbeAPIKey(url, key string) ProbeResult {
	body := `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[]}`

	req, err := http.NewRequest("POST", url, strings.NewReader(body))
	if err != nil {
		return ProbeResult{Status: ProbeError, StatusText: fmt.Sprintf("request creation failed: %v", err)}
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{Status: ProbeError, StatusText: fmt.Sprintf("network error: %v", err)}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusBadRequest:
		return ProbeResult{Status: ProbeUsable, HTTPCode: 400, StatusText: "usable (400)"}
	case resp.StatusCode == http.StatusTooManyRequests:
		return ProbeResult{Status: ProbeLimited, HTTPCode: 429, StatusText: "rate limited (429)"}
	case resp.StatusCode == http.StatusUnauthorized:
		return ProbeResult{Status: ProbeInvalid, HTTPCode: 401, StatusText: "invalid token (401)"}
	case resp.StatusCode == http.StatusForbidden:
		return ProbeResult{Status: ProbeInvalid, HTTPCode: 403, StatusText: "forbidden (403)"}
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return ProbeResult{Status: ProbeUsable, HTTPCode: resp.StatusCode, StatusText: "usable (unexpected 2xx)"}
	default:
		return ProbeResult{Status: ProbeError, HTTPCode: resp.StatusCode, StatusText: fmt.Sprintf("unexpected (%d)", resp.StatusCode)}
	}
}
