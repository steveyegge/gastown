package session

import (
	"strings"
	"testing"
	"time"
)

func TestParseTmuxSessionCreated(t *testing.T) {
	input := "2026-01-24 01:02:03"
	expected, err := time.ParseInLocation("2006-01-02 15:04:05", input, time.Local)
	if err != nil {
		t.Fatalf("parse expected: %v", err)
	}

	parsed, err := ParseTmuxSessionCreated(input)
	if err != nil {
		t.Fatalf("ParseTmuxSessionCreated: %v", err)
	}
	if !parsed.Equal(expected) {
		t.Fatalf("parsed time mismatch: got %v want %v", parsed, expected)
	}
}

func TestStaleReasonForTimes(t *testing.T) {
	now := time.Date(2026, 1, 24, 2, 0, 0, 0, time.UTC)
	newer := now.Add(2 * time.Minute)
	older := now.Add(-2 * time.Minute)

	t.Run("message before session", func(t *testing.T) {
		stale, reason := StaleReasonForTimes(older, newer)
		if !stale {
			t.Fatalf("expected stale")
		}
		if !strings.Contains(reason, "message=") || !strings.Contains(reason, "session_started=") {
			t.Fatalf("expected reason details, got %q", reason)
		}
	})

	t.Run("message after session", func(t *testing.T) {
		stale, reason := StaleReasonForTimes(newer, older)
		if stale || reason != "" {
			t.Fatalf("expected not stale, got %v %q", stale, reason)
		}
	})

	t.Run("zero message time", func(t *testing.T) {
		stale, reason := StaleReasonForTimes(time.Time{}, now)
		if stale || reason != "" {
			t.Fatalf("expected not stale for zero message time, got %v %q", stale, reason)
		}
	})

	t.Run("zero session time", func(t *testing.T) {
		stale, reason := StaleReasonForTimes(now, time.Time{})
		if stale || reason != "" {
			t.Fatalf("expected not stale for zero session time, got %v %q", stale, reason)
		}
	})
}
