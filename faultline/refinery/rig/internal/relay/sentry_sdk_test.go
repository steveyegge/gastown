package relay_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// buildSentryEnvelope constructs a Sentry envelope with a single event item.
func buildSentryEnvelope(eventID, platform string) []byte {
	header := `{"event_id":"` + eventID + `","dsn":"https://testkey@o1.ingest.sentry.io/1","sent_at":"2024-01-01T00:00:00Z"}` + "\n"
	itemHeader := `{"type":"event","length":0}` + "\n"
	payload := `{"event_id":"` + eventID + `","platform":"` + platform + `","level":"error","message":{"formatted":"test error"}}` + "\n"
	return []byte(header + itemHeader + payload)
}

func gzipBytes(data []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}

// TestBrowserSDK_GzipEnvelope verifies that a browser SDK posting a gzip-compressed
// envelope is stored successfully and the raw payload is preserved for later polling.
func TestBrowserSDK_GzipEnvelope(t *testing.T) {
	h, s := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	envelope := buildSentryEnvelope("aabb0011", "javascript")
	compressed := gzipBytes(envelope)

	// Browser SDK sends gzip with X-Sentry-Auth header.
	req := httptest.NewRequest("POST", "/api/1/envelope/", bytes.NewReader(compressed))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("X-Sentry-Auth", "Sentry sentry_key=testkey, sentry_version=7")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["id"] == nil {
		t.Fatal("response missing id field")
	}

	// Verify the payload is stored (relay stores raw body, including gzip).
	envs, err := s.Poll(0, 100)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 stored envelope, got %d", len(envs))
	}
	if envs[0].ProjectID != 1 {
		t.Errorf("expected project_id=1, got %d", envs[0].ProjectID)
	}
	// Relay stores the raw body as-is (gzip compressed).
	if len(envs[0].Payload) == 0 {
		t.Fatal("stored payload is empty")
	}
}

// TestNodeSDK_UncompressedEnvelope verifies that a Node SDK posting an uncompressed
// envelope is stored successfully.
func TestNodeSDK_UncompressedEnvelope(t *testing.T) {
	h, s := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	envelope := buildSentryEnvelope("ccdd0022", "node")

	// Node SDK sends uncompressed with sentry_key query param.
	req := httptest.NewRequest("POST", "/api/1/envelope/?sentry_key=testkey", bytes.NewReader(envelope))
	req.Header.Set("Content-Type", "application/x-sentry-envelope")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify stored payload matches what was sent.
	envs, err := s.Poll(0, 100)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 stored envelope, got %d", len(envs))
	}
	if !bytes.Equal(envs[0].Payload, envelope) {
		t.Errorf("stored payload doesn't match sent payload\ngot:  %q\nwant: %q", envs[0].Payload, envelope)
	}
}

// TestPollerRetrievesEnvelopes verifies the full ingest → poll → ack cycle:
// SDK posts envelopes, poller retrieves them via GET /relay/poll, then acks.
func TestPollerRetrievesEnvelopes(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Ingest two envelopes (browser gzip + node uncompressed).
	gzBody := gzipBytes(buildSentryEnvelope("eeee0001", "javascript"))
	req := httptest.NewRequest("POST", "/api/1/envelope/", bytes.NewReader(gzBody))
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("X-Sentry-Auth", "Sentry sentry_key=testkey")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ingest 1: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("POST", "/api/2/envelope/?sentry_key=otherkey", bytes.NewReader(buildSentryEnvelope("eeee0002", "node")))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ingest 2: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Poll — should get both envelopes.
	req = httptest.NewRequest("GET", "/relay/poll?since=0&limit=100", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("poll: expected 200, got %d", w.Code)
	}

	var pollResp struct {
		Envelopes []struct {
			ID        int64  `json:"id"`
			ProjectID int64  `json:"project_id"`
			PublicKey string `json:"public_key"`
			Payload   []byte `json:"payload"`
		} `json:"envelopes"`
		Count int `json:"count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&pollResp); err != nil {
		t.Fatalf("decode poll response: %v", err)
	}
	if pollResp.Count != 2 {
		t.Fatalf("expected 2 envelopes, got %d", pollResp.Count)
	}

	// Verify different projects.
	projects := map[int64]bool{}
	for _, e := range pollResp.Envelopes {
		projects[e.ProjectID] = true
	}
	if !projects[1] || !projects[2] {
		t.Errorf("expected envelopes from projects 1 and 2, got %v", projects)
	}

	// Ack both.
	ids := []int64{pollResp.Envelopes[0].ID, pollResp.Envelopes[1].ID}
	ackBody, _ := json.Marshal(map[string]any{"ids": ids})
	req = httptest.NewRequest("POST", "/relay/ack", bytes.NewReader(ackBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ack: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var ackResp struct {
		Acked int64 `json:"acked"`
	}
	_ = json.NewDecoder(w.Body).Decode(&ackResp)
	if ackResp.Acked != 2 {
		t.Fatalf("expected 2 acked, got %d", ackResp.Acked)
	}

	// Poll again — empty.
	req = httptest.NewRequest("GET", "/relay/poll?since=0&limit=100", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var emptyResp struct{ Count int }
	_ = json.NewDecoder(w.Body).Decode(&emptyResp)
	if emptyResp.Count != 0 {
		t.Fatalf("expected 0 after ack, got %d", emptyResp.Count)
	}
}

// TestChunkedTransferEncoding verifies that chunked transfer-encoding is handled.
// Go's HTTP server transparently de-chunks request bodies, so this validates that
// the relay handler works correctly with the standard library's chunked handling.
func TestChunkedTransferEncoding(t *testing.T) {
	h, s := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	envelope := buildSentryEnvelope("ffaa0033", "node")

	// Use a real HTTP test server so chunked encoding is actually applied.
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Create request with chunked body using a pipe (no Content-Length).
	pr, pw := io.Pipe()
	go func() {
		// Write in chunks to simulate chunked transfer.
		chunkSize := len(envelope) / 3
		for i := 0; i < len(envelope); i += chunkSize {
			end := i + chunkSize
			if end > len(envelope) {
				end = len(envelope)
			}
			_, _ = pw.Write(envelope[i:end])
		}
		_ = pw.Close()
	}()

	req, _ := http.NewRequest("POST", srv.URL+"/api/1/envelope/?sentry_key=testkey", pr)
	req.Header.Set("Content-Type", "application/x-sentry-envelope")
	// Transfer-Encoding: chunked is set automatically when Content-Length is absent.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Verify stored.
	envs, _ := s.Poll(0, 100)
	if len(envs) != 1 {
		t.Fatalf("expected 1 stored envelope, got %d", len(envs))
	}
	if !bytes.Equal(envs[0].Payload, envelope) {
		t.Errorf("chunked payload mismatch")
	}
}

// TestCORSPreflight verifies that OPTIONS requests to envelope endpoints return
// correct CORS headers for browser SDK compatibility.
func TestCORSPreflight(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	tests := []struct {
		name string
		path string
	}{
		{"envelope with slash", "/api/1/envelope/"},
		{"envelope without slash", "/api/1/envelope"},
		{"store with slash", "/api/1/store/"},
		{"store without slash", "/api/1/store"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("OPTIONS", tt.path, nil)
			req.Header.Set("Origin", "https://myapp.example.com")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "Content-Type, X-Sentry-Auth")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d", w.Code)
			}

			// Verify required CORS headers.
			checks := map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "POST, OPTIONS",
				"Access-Control-Max-Age":       "86400",
			}
			for header, want := range checks {
				got := w.Header().Get(header)
				if got != want {
					t.Errorf("%s: got %q, want %q", header, got, want)
				}
			}

			// Verify X-Sentry-Auth is in allowed headers.
			allowHeaders := w.Header().Get("Access-Control-Allow-Headers")
			if allowHeaders == "" {
				t.Fatal("missing Access-Control-Allow-Headers")
			}
			for _, required := range []string{"Content-Type", "X-Sentry-Auth", "Authorization"} {
				if !bytes.Contains([]byte(allowHeaders), []byte(required)) {
					t.Errorf("Access-Control-Allow-Headers missing %q: got %q", required, allowHeaders)
				}
			}
		})
	}
}

// TestCORSHeadersOnPOST verifies that actual POST responses also include CORS headers.
func TestCORSHeadersOnPOST(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := buildSentryEnvelope("cors0001", "javascript")
	req := httptest.NewRequest("POST", "/api/1/envelope/?sentry_key=testkey", bytes.NewReader(body))
	req.Header.Set("Origin", "https://myapp.example.com")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want %q", got, "*")
	}
}

// TestXSentryAuthHeader verifies the X-Sentry-Auth header format used by browser SDKs.
func TestXSentryAuthHeader(t *testing.T) {
	h, s := newTestHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := []byte(`{"event_id":"auth0001"}`)

	tests := []struct {
		name   string
		header string
		value  string
	}{
		{
			"browser SDK format",
			"X-Sentry-Auth",
			"Sentry sentry_version=7, sentry_client=sentry.javascript.browser/7.100.0, sentry_key=testkey",
		},
		{
			"minimal X-Sentry-Auth",
			"X-Sentry-Auth",
			"Sentry sentry_key=testkey",
		},
		{
			"lowercase sentry prefix",
			"X-Sentry-Auth",
			"sentry sentry_key=testkey, sentry_version=7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/1/envelope/", bytes.NewReader(body))
			req.Header.Set(tt.header, tt.value)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
		})
	}

	// Verify all three were stored.
	envs, _ := s.Poll(0, 100)
	if len(envs) != 3 {
		t.Errorf("expected 3 stored envelopes, got %d", len(envs))
	}
}

// Ensure slog is used (avoid import cycle).
var _ = slog.New(slog.NewTextHandler(os.Stderr, nil))
