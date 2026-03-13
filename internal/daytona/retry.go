package daytona

import (
	"context"
	"errors"
	"math/rand"
	"os/exec"
	"strings"
	"time"
)

// RetryConfig controls exponential backoff for transient Daytona CLI failures.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts including the first call.
	// Set to 1 to disable retries.
	MaxAttempts int

	// InitialDelay is the backoff duration after the first failure.
	// Subsequent retries double this value up to MaxDelay.
	InitialDelay time.Duration

	// MaxDelay caps the exponential backoff so retries don't wait indefinitely.
	MaxDelay time.Duration

	// Jitter is a random fraction of the delay added (centered) to each backoff
	// to decorrelate concurrent retriers. Valid range: 0.0 (no jitter) to 1.0
	// (delay may vary by up to +-50% of its value).
	Jitter float64
}

// DefaultRetryConfig returns sensible defaults for Daytona CLI retries.
// 4 attempts with 2s initial backoff → delays of ~2s, ~4s, ~8s before giving up.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 2 * time.Second,
		MaxDelay:     30 * time.Second,
		Jitter:       0.25,
	}
}

// NoRetryConfig returns a config that disables retries (single attempt).
func NoRetryConfig() RetryConfig {
	return RetryConfig{MaxAttempts: 1}
}

// runWithRetry executes fn with exponential backoff on transient failures.
// When retryOnExitCode is true, non-zero exit codes with transient stderr
// are retried (used for lifecycle methods). When false, only OS-level errors
// are retried (used for Exec, where exit codes come from the inner command).
func (c *Client) runWithRetry(ctx context.Context, retryOnExitCode bool, fn func() (string, string, int, error)) (string, string, int, error) {
	cfg := c.retry
	var stdout, stderr string
	var exitCode int
	var err error

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		stdout, stderr, exitCode, err = fn()

		// Check if this attempt succeeded (from retry perspective).
		// Lifecycle methods: success = no error AND zero exit code.
		// Exec: success = no error (exit code belongs to the inner command).
		succeeded := err == nil && (exitCode == 0 || !retryOnExitCode)
		if succeeded {
			return stdout, stderr, exitCode, err
		}

		// Last attempt — return as-is.
		if attempt == cfg.MaxAttempts {
			break
		}

		// Check if failure is transient (worth retrying).
		if !isTransient(err, exitCode, stderr) {
			break
		}

		// Backoff before next attempt.
		delay := backoffDelay(attempt, cfg)
		select {
		case <-ctx.Done():
			return "", "", -1, ctx.Err()
		case <-time.After(delay):
		}
	}
	return stdout, stderr, exitCode, err
}

// isTransient returns true if the error/exit combination suggests a transient
// failure that may succeed on retry.
func isTransient(err error, exitCode int, stderr string) bool {
	if err != nil {
		// Context cancellation/deadline — don't retry.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false
		}
		// Binary not found — permanent.
		if errors.Is(err, exec.ErrNotFound) {
			return false
		}
		// Other OS-level errors (broken pipe, signal, I/O) — transient.
		return true
	}
	if exitCode == 0 {
		return false
	}
	// Non-zero exit — check stderr for permanent failure patterns.
	lower := strings.ToLower(stderr)
	for _, pattern := range permanentPatterns {
		if strings.Contains(lower, pattern) {
			return false
		}
	}
	return true
}

// permanentPatterns are stderr substrings that indicate non-transient failures.
var permanentPatterns = []string{
	"already exists",
	"unauthorized",
	"forbidden",
	"permission denied",
	"quota exceeded",
}

// backoffDelay calculates the delay for a given attempt number (1-indexed).
// attempt=1 means first retry (after first failure).
func backoffDelay(attempt int, cfg RetryConfig) time.Duration {
	delay := cfg.InitialDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	if cfg.Jitter > 0 {
		jitterRange := float64(delay) * cfg.Jitter
		delta := (rand.Float64() - 0.5) * jitterRange //nolint:gosec // G404: jitter doesn't need crypto rand
		delay += time.Duration(delta)
	}
	return delay
}
