package daytona

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestIsTransient_OSErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error exit 0", nil, false}, // tested via exitCode check below
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"exec not found", exec.ErrNotFound, false},
		{"wrapped exec not found", fmt.Errorf("run: %w", exec.ErrNotFound), false},
		{"generic OS error", fmt.Errorf("broken pipe"), true},
		{"signal killed", fmt.Errorf("signal: killed"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode := -1
			if tt.err == nil {
				exitCode = 0
			}
			got := isTransient(tt.err, exitCode, "")
			if got != tt.want {
				t.Errorf("isTransient(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsTransient_ExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		stderr   string
		want     bool
	}{
		{"success", 0, "", false},
		{"transient exit", 1, "connection reset by peer", true},
		{"transient timeout", 1, "request timed out", true},
		{"permanent quota", 1, "Error: quota exceeded", false},
		{"permanent already exists", 1, "workspace already exists", false},
		{"permanent unauthorized", 1, "Unauthorized: bad token", false},
		{"permanent forbidden", 1, "Forbidden: insufficient permissions", false},
		{"permanent permission denied", 1, "permission denied", false},
		{"case insensitive", 1, "QUOTA EXCEEDED", false},
		{"empty stderr transient", 1, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransient(nil, tt.exitCode, tt.stderr)
			if got != tt.want {
				t.Errorf("isTransient(nil, %d, %q) = %v, want %v", tt.exitCode, tt.stderr, got, tt.want)
			}
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	cfg := RetryConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Jitter:       0, // no jitter for deterministic tests
	}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 100 * time.Millisecond},  // first retry
		{2, 200 * time.Millisecond},  // doubled
		{3, 400 * time.Millisecond},  // doubled again
		{4, 800 * time.Millisecond},  // doubled again
		{5, 1 * time.Second},         // capped at MaxDelay
		{6, 1 * time.Second},         // still capped
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			got := backoffDelay(tt.attempt, cfg)
			if got != tt.want {
				t.Errorf("backoffDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}

func TestBackoffDelay_WithJitter(t *testing.T) {
	cfg := RetryConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Jitter:       0.5,
	}

	// With 50% jitter on 100ms, delta is in [-25ms, +25ms], so delay in [75ms, 125ms].
	for i := 0; i < 100; i++ {
		delay := backoffDelay(1, cfg)
		if delay < 75*time.Millisecond || delay > 125*time.Millisecond {
			t.Errorf("backoffDelay with jitter = %v, want in [75ms, 125ms]", delay)
		}
	}
}

// retryMockRunner tracks calls per attempt and returns different responses.
type retryMockRunner struct {
	callCount int
	responses []mockResponse
}

func (m *retryMockRunner) Run(_ context.Context, name string, args ...string) (string, string, int, error) {
	idx := m.callCount
	m.callCount++
	if idx < len(m.responses) {
		r := m.responses[idx]
		return r.stdout, r.stderr, r.exitCode, r.err
	}
	// Default: success
	return "", "", 0, nil
}

func TestRetry_CreateSucceedsAfterTransientFailure(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "connection reset", exitCode: 1},           // attempt 1: transient
			{stderr: "connection reset", exitCode: 1},           // attempt 2: transient
			{exitCode: 0},                                       // attempt 3: success
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Jitter:       0,
	})

	err := c.Create(context.Background(), "ws", "url", "main", CreateOptions{})
	if err != nil {
		t.Fatalf("Create() should succeed after retries, got: %v", err)
	}
	if mock.callCount != 3 {
		t.Errorf("expected 3 attempts, got %d", mock.callCount)
	}
}

func TestRetry_StartSucceedsAfterTransientFailure(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "timeout", exitCode: 1},  // attempt 1: transient
			{exitCode: 0},                     // attempt 2: success
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Jitter:       0,
	})

	err := c.Start(context.Background(), "ws")
	if err != nil {
		t.Fatalf("Start() should succeed after retry, got: %v", err)
	}
	if mock.callCount != 2 {
		t.Errorf("expected 2 attempts, got %d", mock.callCount)
	}
}

func TestRetry_NoPermanentRetry(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "quota exceeded", exitCode: 1}, // permanent
			{exitCode: 0},                           // should never reach
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Jitter:       0,
	})

	err := c.Create(context.Background(), "ws", "url", "main", CreateOptions{})
	if err == nil {
		t.Fatal("expected error for permanent failure")
	}
	if mock.callCount != 1 {
		t.Errorf("expected 1 attempt (no retry on permanent), got %d", mock.callCount)
	}
}

func TestRetry_MaxAttemptsExhausted(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "connection reset", exitCode: 1},
			{stderr: "connection reset", exitCode: 1},
			{stderr: "connection reset", exitCode: 1},
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Jitter:       0,
	})

	err := c.Create(context.Background(), "ws", "url", "main", CreateOptions{})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if mock.callCount != 3 {
		t.Errorf("expected 3 attempts, got %d", mock.callCount)
	}
}

func TestRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "timeout", exitCode: 1}, // transient, would retry
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 500 * time.Millisecond, // long enough to cancel during wait
		MaxDelay:     1 * time.Second,
		Jitter:       0,
	})

	// Cancel context shortly after the first attempt.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := c.Create(ctx, "ws", "url", "main", CreateOptions{})
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
	if err.Error() != "daytona create: context canceled" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRetry_ExecNoRetryOnExitCode(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "command not found", exitCode: 127},
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Jitter:       0,
	})

	_, stderr, exitCode, err := c.Exec(context.Background(), "ws", nil, "badcmd")
	if err != nil {
		t.Fatalf("Exec() should not return error for non-zero exit: %v", err)
	}
	if exitCode != 127 {
		t.Errorf("exitCode = %d, want 127", exitCode)
	}
	if !strings.Contains(stderr, "command not found") {
		t.Errorf("stderr = %q, want 'command not found'", stderr)
	}
	if mock.callCount != 1 {
		t.Errorf("expected 1 attempt (no retry on exit code for Exec), got %d", mock.callCount)
	}
}

func TestRetry_ExecRetriesOnOSError(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{err: fmt.Errorf("broken pipe")},       // transient OS error
			{stdout: "ok", exitCode: 0},             // success
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Jitter:       0,
	})

	stdout, _, exitCode, err := c.Exec(context.Background(), "ws", nil, "echo", "hi")
	if err != nil {
		t.Fatalf("Exec() should succeed after retry, got: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if stdout != "ok" {
		t.Errorf("stdout = %q, want %q", stdout, "ok")
	}
	if mock.callCount != 2 {
		t.Errorf("expected 2 attempts, got %d", mock.callCount)
	}
}

func TestRetry_ListOwnedRetriesOnTransient(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "connection refused", exitCode: 1},
			{stdout: `[{"id":"ws1","name":"gt-test-rig--p","state":"running"}]`, exitCode: 0},
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	c.SetRetry(RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Jitter:       0,
	})

	workspaces, err := c.ListOwned(context.Background())
	if err != nil {
		t.Fatalf("ListOwned() should succeed after retry, got: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(workspaces))
	}
	if mock.callCount != 2 {
		t.Errorf("expected 2 attempts, got %d", mock.callCount)
	}
}

func TestRetry_StopAndDelete(t *testing.T) {
	for _, op := range []string{"stop", "delete"} {
		t.Run(op, func(t *testing.T) {
			mock := &retryMockRunner{
				responses: []mockResponse{
					{stderr: "connection reset", exitCode: 1},
					{exitCode: 0},
				},
			}
			c := NewClientWithRunner("gt-test", mock)
			c.SetRetry(RetryConfig{
				MaxAttempts:  3,
				InitialDelay: 1 * time.Millisecond,
				MaxDelay:     5 * time.Millisecond,
				Jitter:       0,
			})

			var err error
			if op == "stop" {
				err = c.Stop(context.Background(), "ws")
			} else {
				err = c.Delete(context.Background(), "ws")
			}
			if err != nil {
				t.Fatalf("%s() should succeed after retry, got: %v", op, err)
			}
			if mock.callCount != 2 {
				t.Errorf("expected 2 attempts, got %d", mock.callCount)
			}
		})
	}
}

func TestRetry_NoRetryConfig(t *testing.T) {
	mock := &retryMockRunner{
		responses: []mockResponse{
			{stderr: "connection reset", exitCode: 1},
			{exitCode: 0}, // should never reach
		},
	}
	c := NewClientWithRunner("gt-test", mock)
	// NoRetryConfig is already the default for NewClientWithRunner

	err := c.Create(context.Background(), "ws", "url", "main", CreateOptions{})
	if err == nil {
		t.Fatal("expected error with no retries")
	}
	if mock.callCount != 1 {
		t.Errorf("expected 1 attempt, got %d", mock.callCount)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxAttempts != 4 {
		t.Errorf("MaxAttempts = %d, want 4", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 2*time.Second {
		t.Errorf("InitialDelay = %v, want 2s", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
	if cfg.Jitter != 0.25 {
		t.Errorf("Jitter = %v, want 0.25", cfg.Jitter)
	}
}

func TestNoRetryConfig(t *testing.T) {
	cfg := NoRetryConfig()
	if cfg.MaxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want 1", cfg.MaxAttempts)
	}
}
