package quota

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeAPIKey_400_IsUsable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	result := ProbeAPIKey(ts.URL, "test-key")
	if result.Status != ProbeUsable {
		t.Errorf("expected ProbeUsable, got %q (HTTPCode=%d, StatusText=%q)", result.Status, result.HTTPCode, result.StatusText)
	}
	if result.HTTPCode != 400 {
		t.Errorf("expected HTTPCode 400, got %d", result.HTTPCode)
	}
}

func TestProbeAPIKey_429_IsLimited(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	result := ProbeAPIKey(ts.URL, "test-key")
	if result.Status != ProbeLimited {
		t.Errorf("expected ProbeLimited, got %q (HTTPCode=%d, StatusText=%q)", result.Status, result.HTTPCode, result.StatusText)
	}
	if result.HTTPCode != 429 {
		t.Errorf("expected HTTPCode 429, got %d", result.HTTPCode)
	}
}

func TestProbeAPIKey_401_IsInvalid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	result := ProbeAPIKey(ts.URL, "test-key")
	if result.Status != ProbeInvalid {
		t.Errorf("expected ProbeInvalid, got %q (HTTPCode=%d, StatusText=%q)", result.Status, result.HTTPCode, result.StatusText)
	}
	if result.HTTPCode != 401 {
		t.Errorf("expected HTTPCode 401, got %d", result.HTTPCode)
	}
}

func TestProbeAPIKey_500_IsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	result := ProbeAPIKey(ts.URL, "test-key")
	if result.Status != ProbeError {
		t.Errorf("expected ProbeError, got %q (HTTPCode=%d, StatusText=%q)", result.Status, result.HTTPCode, result.StatusText)
	}
	if result.HTTPCode != 500 {
		t.Errorf("expected HTTPCode 500, got %d", result.HTTPCode)
	}
}

func TestProbeAPIKey_SendsMalformedRequest(t *testing.T) {
	var capturedKey string
	var capturedBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedKey = r.Header.Get("x-api-key")
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	ProbeAPIKey(ts.URL, "my-api-key-123")

	if capturedKey != "my-api-key-123" {
		t.Errorf("expected x-api-key header %q, got %q", "my-api-key-123", capturedKey)
	}
	if len(capturedBody) == 0 {
		t.Error("expected non-empty request body")
	}
}

func TestProbeAPIKey_NetworkError(t *testing.T) {
	// Use a port that is not listening (reserved/unlikely port)
	result := ProbeAPIKey("http://127.0.0.1:19999", "test-key")
	if result.Status != ProbeError {
		t.Errorf("expected ProbeError for unreachable port, got %q (StatusText=%q)", result.Status, result.StatusText)
	}
}
