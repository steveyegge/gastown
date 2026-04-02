package gtfaultline

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// collector is a test HTTP server that captures Sentry envelopes.
type collector struct {
	mu    sync.Mutex
	items []json.RawMessage
	srv   *httptest.Server
}

func newCollector() *collector {
	c := &collector{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			defer func() { _ = gz.Close() }()
			reader = gz
		}
		body, _ := io.ReadAll(reader)
		// Sentry envelopes are newline-delimited JSON. The first line is the
		// envelope header, subsequent pairs are item header + item body.
		lines := strings.Split(strings.TrimSpace(string(body)), "\n")
		c.mu.Lock()
		for _, line := range lines {
			if line == "" {
				continue
			}
			c.items = append(c.items, json.RawMessage(line))
		}
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	return c
}

func (c *collector) dsn() string {
	// Build a Sentry-compatible DSN from the test server address.
	// DSN format: http://<key>@<host>/<project_id>
	addr := strings.TrimPrefix(c.srv.URL, "http://")
	return "http://testkey@" + addr + "/1"
}

func (c *collector) close() {
	c.srv.Close()
}

func (c *collector) rawItems() []json.RawMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]json.RawMessage, len(c.items))
	copy(cp, c.items)
	return cp
}

// bodyContains checks whether any captured envelope body contains substr.
func (c *collector) bodyContains(substr string) bool {
	for _, raw := range c.rawItems() {
		if strings.Contains(string(raw), substr) {
			return true
		}
	}
	return false
}

func TestInitDefaultDSN(t *testing.T) {
	if DefaultDSN == "" {
		t.Fatal("DefaultDSN should not be empty")
	}
	if !strings.Contains(DefaultDSN, "localhost:8080") {
		t.Fatalf("DefaultDSN should reference localhost:8080, got %s", DefaultDSN)
	}
}

func TestInitWithCollector(t *testing.T) {
	c := newCollector()
	defer c.close()

	err := Init(Config{DSN: c.dsn()})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestCaptureError(t *testing.T) {
	c := newCollector()
	defer c.close()

	err := Init(Config{DSN: c.dsn()})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	CaptureError(errors.New("test error from CaptureError"))
	Flush(2 * time.Second)

	if !c.bodyContains("test error from CaptureError") {
		t.Error("expected envelope to contain 'test error from CaptureError'")
	}
}

func TestCaptureMessage(t *testing.T) {
	c := newCollector()
	defer c.close()

	err := Init(Config{DSN: c.dsn()})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	CaptureMessage("hello from test")
	Flush(2 * time.Second)

	if !c.bodyContains("hello from test") {
		t.Error("expected envelope to contain 'hello from test'")
	}
}

func TestRecoverAndReport(t *testing.T) {
	c := newCollector()
	defer c.close()

	err := Init(Config{DSN: c.dsn()})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Error("expected re-panic after RecoverAndReport")
			}
		}()
		defer RecoverAndReport()
		panic("boom")
	}()

	Flush(2 * time.Second)

	if !c.bodyContains("boom") {
		t.Error("expected envelope to contain panic message 'boom'")
	}
}

func TestSlogHandler(t *testing.T) {
	c := newCollector()
	defer c.close()

	err := Init(Config{DSN: c.dsn()})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	base := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	handler := SlogHandler(base, slog.LevelError)
	logger := slog.New(handler)

	// This should NOT trigger a Sentry event (below threshold).
	logger.Info("info message should be ignored")
	Flush(2 * time.Second)

	if c.bodyContains("info message should be ignored") {
		t.Error("info-level message should not be sent to faultline")
	}

	// This SHOULD trigger a Sentry event.
	logger.Error("slog error event")
	Flush(2 * time.Second)

	if !c.bodyContains("slog error event") {
		t.Error("expected envelope to contain 'slog error event'")
	}
}
