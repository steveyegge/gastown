package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/inject"
)

func TestGetInjectQueue(t *testing.T) {
	// Create a temp directory with .runtime structure
	tmpDir, err := os.MkdirTemp("", "inject-cmd-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	runtimeDir := filepath.Join(tmpDir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("creating runtime dir: %v", err)
	}

	// Test with explicit session ID
	injectDrainSession = "test-session-123"
	defer func() { injectDrainSession = "" }()

	// Change to temp directory
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	queue, err := getInjectQueue()
	if err != nil {
		t.Fatalf("getInjectQueue: %v", err)
	}

	// Test enqueue and drain
	if err := queue.Enqueue(inject.TypeMail, "test content"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	entries, err := queue.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Content != "test content" {
		t.Errorf("unexpected content: %s", entries[0].Content)
	}
}

func TestInjectQueueIntegration(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "inject-integration-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionID := "integration-test-session"
	queue := inject.NewQueue(tmpDir, sessionID)

	// Enqueue multiple items of different types
	mailContent := "<system-reminder>\nYou have 2 unread messages.\n</system-reminder>\n"
	decisionContent := "<system-reminder>\nYou have 1 pending decision.\n</system-reminder>\n"

	if err := queue.Enqueue(inject.TypeMail, mailContent); err != nil {
		t.Fatalf("Enqueue mail: %v", err)
	}
	if err := queue.Enqueue(inject.TypeDecision, decisionContent); err != nil {
		t.Fatalf("Enqueue decision: %v", err)
	}

	// Verify count
	count, err := queue.Count()
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Drain and verify order
	entries, err := queue.Drain()
	if err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify order (oldest first)
	if entries[0].Type != inject.TypeMail {
		t.Errorf("expected first entry to be mail, got %s", entries[0].Type)
	}
	if entries[1].Type != inject.TypeDecision {
		t.Errorf("expected second entry to be decision, got %s", entries[1].Type)
	}

	// Verify queue is empty
	count, err = queue.Count()
	if err != nil {
		t.Fatalf("Count after drain: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 after drain, got %d", count)
	}
}
