package ingest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func TestRateLimiter_Allow(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rl := NewRateLimiter(10, log) // 10 ev/s, burst 10

	handler := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First 10 requests should succeed (burst).
	for i := range 10 {
		req := httptest.NewRequest("POST", "/api/1/envelope/", nil)
		req.SetPathValue("project_id", "1")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, rec.Code)
		}
	}

	// 11th request should be rate limited.
	req := httptest.NewRequest("POST", "/api/1/envelope/", nil)
	req.SetPathValue("project_id", "1")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}

	// Check required headers.
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header")
	}
	if rec.Header().Get("X-Sentry-Rate-Limits") == "" {
		t.Fatal("missing X-Sentry-Rate-Limits header")
	}

	// Check response body.
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["detail"] == "" {
		t.Fatal("missing detail in response body")
	}
}

func TestRateLimiter_PerProject(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rl := NewRateLimiter(5, log) // 5 ev/s, burst 5

	handler := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exhaust project 1's burst.
	for range 5 {
		req := httptest.NewRequest("POST", "/api/1/envelope/", nil)
		req.SetPathValue("project_id", "1")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("project 1: got %d, want 200", rec.Code)
		}
	}

	// Project 1 should be limited.
	req := httptest.NewRequest("POST", "/api/1/envelope/", nil)
	req.SetPathValue("project_id", "1")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("project 1: got %d, want 429", rec.Code)
	}

	// Project 2 should still work.
	req = httptest.NewRequest("POST", "/api/2/envelope/", nil)
	req.SetPathValue("project_id", "2")
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("project 2: got %d, want 200", rec.Code)
	}
}

func TestRateLimiter_NoProjectID(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rl := NewRateLimiter(1, log) // 1 ev/s

	handler := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request without project_id should pass through without rate limiting.
	for range 10 {
		req := httptest.NewRequest("POST", "/health", nil)
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("got %d, want 200", rec.Code)
		}
	}
}

func TestRateLimitHeader_Format(t *testing.T) {
	header := formatRateLimitHeader(60)
	if header != "60::project" {
		t.Fatalf("got %q, want %q", header, "60::project")
	}
}

func TestRetryAfter_MinimumOne(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rl := NewRateLimiter(10, log)

	// Exhaust burst for project 1.
	for range 10 {
		req := httptest.NewRequest("POST", "/api/1/envelope/", nil)
		req.SetPathValue("project_id", "1")
		rec := httptest.NewRecorder()
		rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})(rec, req)
	}

	// Next request should be limited with Retry-After >= 1.
	req := httptest.NewRequest("POST", "/api/1/envelope/", nil)
	req.SetPathValue("project_id", "1")
	rec := httptest.NewRecorder()
	rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})(rec, req)

	ra, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	if err != nil {
		t.Fatalf("parse Retry-After: %v", err)
	}
	if ra < 1 {
		t.Fatalf("Retry-After %d < 1", ra)
	}
}
