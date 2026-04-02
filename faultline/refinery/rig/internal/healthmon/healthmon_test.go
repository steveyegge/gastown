package healthmon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockPinger implements the Pinger interface for testing.
type mockPinger struct {
	pingErr error
}

func (m *mockPinger) PingContext(ctx context.Context) error {
	return m.pingErr
}

func TestMonitor_DoltPingFailure(t *testing.T) {
	var mu sync.Mutex
	var errors []string

	db := &mockPinger{pingErr: fmt.Errorf("connection refused")}
	cfg := Config{
		Interval:        50 * time.Millisecond,
		DoltPingTimeout: 100 * time.Millisecond,
		RunGTDoctor:     false,
	}

	m := New(db, newTestLogger(), cfg, func(severity, message string) {
		mu.Lock()
		defer mu.Unlock()
		errors = append(errors, fmt.Sprintf("%s: %s", severity, message))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	m.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(errors) == 0 {
		t.Fatal("expected errors from ping failure, got none")
	}
	for _, e := range errors {
		if e != "error: Dolt ping failed: connection refused" {
			t.Errorf("unexpected error: %s", e)
		}
	}
}

func TestMonitor_DoltPingSuccess(t *testing.T) {
	var mu sync.Mutex
	var errors []string

	db := &mockPinger{} // no errors
	cfg := Config{
		Interval:        50 * time.Millisecond,
		DoltPingTimeout: 100 * time.Millisecond,
		RunGTDoctor:     false,
	}

	m := New(db, newTestLogger(), cfg, func(severity, message string) {
		mu.Lock()
		defer mu.Unlock()
		errors = append(errors, message)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	m.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(errors) != 0 {
		t.Errorf("expected no errors when ping succeeds, got %v", errors)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Interval != 60*time.Second {
		t.Errorf("expected 60s interval, got %v", cfg.Interval)
	}
	if cfg.DoltPingTimeout != 5*time.Second {
		t.Errorf("expected 5s ping timeout, got %v", cfg.DoltPingTimeout)
	}
	if !cfg.RunGTDoctor {
		t.Error("expected RunGTDoctor=true by default")
	}
	if cfg.DoctorEveryN != 10 {
		t.Errorf("expected DoctorEveryN=10, got %d", cfg.DoctorEveryN)
	}
}
