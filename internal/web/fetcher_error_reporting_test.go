package web

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGetIssueDetailsBatch_ReturnsStructuredErrorOnCommandFailure(t *testing.T) {
	original := fetcherRunCmd
	t.Cleanup(func() {
		fetcherRunCmd = original
	})

	fetcherRunCmd = func(_ time.Duration, _ string, _ ...string) (*bytes.Buffer, error) {
		return nil, errors.New("boom")
	}

	f := &LiveConvoyFetcher{cmdTimeout: 100 * time.Millisecond}
	_, err := f.getIssueDetailsBatch([]string{"gt-1", "gt-2"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "bd show failed") {
		t.Fatalf("expected structured failure context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "issue_count=2") {
		t.Fatalf("expected issue count in error, got: %v", err)
	}
}

func TestGetIssueDetailsBatch_ReturnsStructuredErrorOnInvalidJSON(t *testing.T) {
	original := fetcherRunCmd
	t.Cleanup(func() {
		fetcherRunCmd = original
	})

	fetcherRunCmd = func(_ time.Duration, _ string, _ ...string) (*bytes.Buffer, error) {
		return bytes.NewBufferString("{invalid"), nil
	}

	f := &LiveConvoyFetcher{cmdTimeout: 100 * time.Millisecond}
	_, err := f.getIssueDetailsBatch([]string{"gt-9"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("expected parse context, got: %v", err)
	}
	if !strings.Contains(err.Error(), "issue_count=1") {
		t.Fatalf("expected issue count in error, got: %v", err)
	}
}

func TestGetIssueDetailsBatch_ParsesIssueDetails(t *testing.T) {
	original := fetcherRunCmd
	t.Cleanup(func() {
		fetcherRunCmd = original
	})

	fetcherRunCmd = func(_ time.Duration, _ string, _ ...string) (*bytes.Buffer, error) {
		return bytes.NewBufferString(`[
			{"id":"gt-1","title":"One","status":"open","assignee":"rig/polecats/a","updated_at":"2026-02-01T12:00:00Z"},
			{"id":"gt-2","title":"Two","status":"closed","assignee":"","updated_at":"2026-02-01T12:01:00Z"}
		]`), nil
	}

	f := &LiveConvoyFetcher{cmdTimeout: 100 * time.Millisecond}
	details, err := f.getIssueDetailsBatch([]string{"gt-1", "gt-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(details) != 2 {
		t.Fatalf("expected 2 details, got %d", len(details))
	}
	if details["gt-1"] == nil || details["gt-1"].Title != "One" {
		t.Fatalf("unexpected parsed details for gt-1: %#v", details["gt-1"])
	}
	if details["gt-2"] == nil || details["gt-2"].Status != "closed" {
		t.Fatalf("unexpected parsed details for gt-2: %#v", details["gt-2"])
	}
}
