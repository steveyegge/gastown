package util

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_Success(t *testing.T) {
	callCount := 0
	result, err := Retry(context.Background(), DefaultRetryConfig(), func() (string, error) {
		callCount++
		return "success", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %q", result)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestRetry_EventualSuccess(t *testing.T) {
	callCount := 0
	transientErr := errors.New("connection refused")

	result, err := Retry(context.Background(), DefaultRetryConfig(), func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", transientErr
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %q", result)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetry_MaxAttemptsExceeded(t *testing.T) {
	callCount := 0
	transientErr := errors.New("connection refused")

	cfg := RetryConfig{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		IsRetryable:  DefaultIsRetryable,
	}

	_, err := Retry(context.Background(), cfg, func() (string, error) {
		callCount++
		return "", transientErr
	})

	if err == nil {
		t.Error("expected error after max attempts")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	callCount := 0
	permanentErr := errors.New("file not found") // Not in transient patterns

	_, err := Retry(context.Background(), DefaultRetryConfig(), func() (string, error) {
		callCount++
		return "", permanentErr
	})

	if err == nil {
		t.Error("expected error for non-retryable error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry), got %d", callCount)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	callCount := 0
	_, err := Retry(ctx, DefaultRetryConfig(), func() (string, error) {
		callCount++
		return "success", nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if callCount != 0 {
		t.Errorf("expected 0 calls (cancelled before first attempt), got %d", callCount)
	}
}

func TestRetry_PermanentError(t *testing.T) {
	callCount := 0
	permanentErr := MarkPermanent(errors.New("database connection refused"))

	_, err := Retry(context.Background(), DefaultRetryConfig(), func() (string, error) {
		callCount++
		return "", permanentErr
	})

	if err == nil {
		t.Error("expected error for permanent error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry for permanent), got %d", callCount)
	}
}

func TestDefaultIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"connection refused", errors.New("dial: connection refused"), true},
		{"timeout", errors.New("read timeout"), true},
		{"EAGAIN", errors.New("resource temporarily unavailable (EAGAIN)"), true},
		{"database locked", errors.New("database is locked"), true},
		{"permanent error", errors.New("file not found"), false},
		{"case insensitive", errors.New("CONNECTION REFUSED"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultIsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("DefaultIsRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestRetrySimple(t *testing.T) {
	result, err := RetrySimple(func() (int, error) {
		return 42, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestMarkPermanent(t *testing.T) {
	original := errors.New("original error")
	permanent := MarkPermanent(original)

	if !IsPermanent(permanent) {
		t.Error("expected IsPermanent to return true")
	}

	if !errors.Is(permanent, original) {
		t.Error("expected permanent error to wrap original")
	}

	if permanent.Error() != "original error" {
		t.Errorf("expected error message to be preserved, got %q", permanent.Error())
	}

	// Test nil input
	if MarkPermanent(nil) != nil {
		t.Error("expected MarkPermanent(nil) to return nil")
	}
}
