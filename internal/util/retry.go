package util

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"
)

// RetryConfig configures retry behavior with exponential backoff.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (default: 3).
	MaxAttempts int

	// InitialDelay is the delay before the first retry (default: 100ms).
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries (default: 5s).
	MaxDelay time.Duration

	// Multiplier is the backoff multiplier (default: 2.0).
	Multiplier float64

	// Jitter adds randomness to delays to prevent thundering herd (default: true).
	Jitter bool

	// IsRetryable determines if an error should be retried.
	// If nil, uses DefaultIsRetryable.
	IsRetryable func(error) bool
}

// DefaultRetryConfig returns sensible defaults for API operations.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		IsRetryable:  DefaultIsRetryable,
	}
}

// transientErrorPatterns contains substrings that indicate transient errors
// which are worth retrying. These are common temporary failure modes for
// CLI operations and network calls.
var transientErrorPatterns = []string{
	"resource temporarily unavailable",
	"connection refused",
	"connection reset",
	"connection timed out",
	"timeout",
	"temporary failure",
	"try again",
	"EAGAIN",
	"ECONNREFUSED",
	"ECONNRESET",
	"ETIMEDOUT",
	"database is locked",
	"locked by another process",
	"too many open files",
	"broken pipe",
	"EOF",
	"network is unreachable",
	"no route to host",
}

// DefaultIsRetryable returns true for transient errors that might succeed on retry.
// It returns false for permanent errors like "not found" or command not installed.
func DefaultIsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Check for known transient error patterns
	for _, pattern := range transientErrorPatterns {
		if strings.Contains(errStr, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

// Retry executes fn with exponential backoff retry logic.
// It returns the result of fn or the last error if all attempts fail.
func Retry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	// Apply defaults
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 5 * time.Second
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = 2.0
	}
	if cfg.IsRetryable == nil {
		cfg.IsRetryable = DefaultIsRetryable
	}

	var zero T
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Always check for permanent errors (marked by MarkPermanent)
		if IsPermanent(err) {
			return zero, err
		}

		// Don't retry non-retryable errors
		if !cfg.IsRetryable(err) {
			return zero, err
		}

		// Don't sleep after the last attempt
		if attempt == cfg.MaxAttempts {
			break
		}

		// Calculate sleep duration with optional jitter
		sleepDuration := delay
		if cfg.Jitter {
			// Add up to 25% jitter
			jitter := time.Duration(rand.Float64() * 0.25 * float64(delay))
			sleepDuration += jitter
		}

		// Sleep with context awareness
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(sleepDuration):
		}

		// Increase delay for next attempt
		delay = time.Duration(float64(delay) * cfg.Multiplier)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	return zero, lastErr
}

// RetrySimple is a convenience wrapper for Retry with default config.
// Use this for simple retry scenarios where defaults are acceptable.
func RetrySimple[T any](fn func() (T, error)) (T, error) {
	return Retry(context.Background(), DefaultRetryConfig(), fn)
}

// RetryWithContext is a convenience wrapper for Retry with default config and context.
func RetryWithContext[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	return Retry(ctx, DefaultRetryConfig(), fn)
}

// IsTransientError checks if an error is transient (worth retrying).
// This is a public helper for use in custom retry logic.
func IsTransientError(err error) bool {
	return DefaultIsRetryable(err)
}

// PermanentError wraps an error to indicate it should not be retried.
// Use this to short-circuit retry logic for known permanent failures.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string {
	return e.Err.Error()
}

func (e *PermanentError) Unwrap() error {
	return e.Err
}

// IsPermanent checks if an error is marked as permanent.
func IsPermanent(err error) bool {
	var permErr *PermanentError
	return errors.As(err, &permErr)
}

// MarkPermanent wraps an error to indicate it should not be retried.
func MarkPermanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Err: err}
}
