package selfmon

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestHandler_ForwardsToInner(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	h := NewHandler(inner, Config{})

	log := slog.New(h)
	log.Info("test message")

	if buf.Len() == 0 {
		t.Fatal("inner handler did not receive message")
	}
}

func TestHandler_ReportsErrors(t *testing.T) {
	var mu sync.Mutex
	var received []sentryEvent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev sentryEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		received = append(received, ev)
		mu.Unlock()
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer srv.Close()

	inner := slog.NewJSONHandler(io.Discard, nil)
	h := NewHandler(inner, Config{
		Endpoint:  srv.URL + "/api/0/store/",
		SentryKey: "test_key",
		MinLevel:  slog.LevelError,
	})

	log := slog.New(h)
	log.Error("database connection failed", "err", "connection refused", "host", "localhost:3307")

	// Wait for async report.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	ev := received[0]
	if ev.Level != "error" {
		t.Errorf("level = %q, want %q", ev.Level, "error")
	}
	if ev.Message != "database connection failed" {
		t.Errorf("message = %q, want %q", ev.Message, "database connection failed")
	}
	if ev.Platform != "go" {
		t.Errorf("platform = %q, want %q", ev.Platform, "go")
	}
	if ev.Tags["source"] != "selfmon" {
		t.Errorf("tags[source] = %q, want %q", ev.Tags["source"], "selfmon")
	}
	if ev.Exception == nil || len(ev.Exception.Values) == 0 {
		t.Fatal("expected exception in event")
	}
	if ev.Exception.Values[0].Type != "faultline.internal" {
		t.Errorf("exception type = %q, want %q", ev.Exception.Values[0].Type, "faultline.internal")
	}
}

func TestHandler_SkipsInfoLevel(t *testing.T) {
	var mu sync.Mutex
	var received int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received++
		mu.Unlock()
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer srv.Close()

	inner := slog.NewJSONHandler(io.Discard, nil)
	h := NewHandler(inner, Config{
		Endpoint:  srv.URL + "/api/0/store/",
		SentryKey: "test_key",
		MinLevel:  slog.LevelError,
	})

	log := slog.New(h)
	log.Info("normal operation")
	log.Warn("minor issue")

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if received != 0 {
		t.Errorf("expected 0 events for info/warn, got %d", received)
	}
}

func TestHandler_SetsAuthHeader(t *testing.T) {
	var mu sync.Mutex
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeader = r.Header.Get("X-Sentry-Auth")
		mu.Unlock()
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer srv.Close()

	inner := slog.NewJSONHandler(io.Discard, nil)
	h := NewHandler(inner, Config{
		Endpoint:  srv.URL + "/api/0/store/",
		SentryKey: "my_secret_key",
		MinLevel:  slog.LevelError,
	})

	log := slog.New(h)
	log.Error("test auth")

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	got := authHeader
	mu.Unlock()
	if got != "Sentry sentry_key=my_secret_key" {
		t.Errorf("auth header = %q, want %q", got, "Sentry sentry_key=my_secret_key")
	}
}

func TestHandler_NoRecursion(t *testing.T) {
	// If the endpoint fails, we shouldn't get stuck in a loop.
	var mu sync.Mutex
	var count int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		w.WriteHeader(500) // simulate failure
	}))
	defer srv.Close()

	inner := slog.NewJSONHandler(io.Discard, nil)
	h := NewHandler(inner, Config{
		Endpoint:  srv.URL + "/api/0/store/",
		SentryKey: "test_key",
		MinLevel:  slog.LevelError,
	})

	log := slog.New(h)
	// Fire multiple errors rapidly.
	for i := 0; i < 10; i++ {
		log.Error("rapid fire error", "i", i)
	}

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// The guard flag means concurrent reports are dropped. We should see
	// far fewer than 10 requests (likely 1-3 due to the atomic guard).
	if count > 5 {
		t.Errorf("expected <=5 requests due to recursion guard, got %d", count)
	}
}

func TestCollectFrames(t *testing.T) {
	frames := collectFrames(1)
	if len(frames) == 0 {
		t.Fatal("expected at least one frame")
	}
	// Outermost frame should be from the test runner (or runtime).
	// Innermost (last) should be this test function.
	last := frames[len(frames)-1]
	if last.Function == "" {
		t.Error("expected function name in frame")
	}
}

func TestHandler_DifferentMessagesGetDifferentFingerprints(t *testing.T) {
	var mu sync.Mutex
	var received []sentryEvent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var ev sentryEvent
		if json.Unmarshal(body, &ev) == nil {
			mu.Lock()
			received = append(received, ev)
			mu.Unlock()
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"test"}`))
	}))
	defer srv.Close()

	inner := slog.NewJSONHandler(io.Discard, nil)
	h := NewHandler(inner, Config{
		Endpoint:  srv.URL + "/api/0/store/",
		SentryKey: "test_key",
		MinLevel:  slog.LevelError,
	})

	log := slog.New(h)
	log.Error("retention: purge events: delete error")
	time.Sleep(200 * time.Millisecond)
	// Reset sending guard so second error can fire.
	h.sending.Store(false)
	log.Error("relay ingest failed: parse envelope")
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) < 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	// Both should have type "faultline.internal" but different fingerprints.
	ev1 := received[0]
	ev2 := received[1]
	if ev1.Exception.Values[0].Type != "faultline.internal" {
		t.Errorf("ev1 type = %q", ev1.Exception.Values[0].Type)
	}
	if ev2.Exception.Values[0].Type != "faultline.internal" {
		t.Errorf("ev2 type = %q", ev2.Exception.Values[0].Type)
	}
	// Fingerprints must differ despite same exception type.
	if len(ev1.Fingerprint) == 0 || len(ev2.Fingerprint) == 0 {
		t.Fatal("events should have explicit fingerprints")
	}
	fp1 := ev1.Fingerprint[0] + ev1.Fingerprint[1]
	fp2 := ev2.Fingerprint[0] + ev2.Fingerprint[1]
	if fp1 == fp2 {
		t.Fatalf("different error messages should produce different fingerprints: %v vs %v", ev1.Fingerprint, ev2.Fingerprint)
	}
}
