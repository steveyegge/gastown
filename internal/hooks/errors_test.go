package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeHash(t *testing.T) {
	// Same inputs should produce same hash
	h1 := computeHash("SessionStart", "gt prime", "gastown/crew/joe")
	h2 := computeHash("SessionStart", "gt prime", "gastown/crew/joe")
	if h1 != h2 {
		t.Errorf("Same inputs produced different hashes: %s vs %s", h1, h2)
	}

	// Different inputs should produce different hashes
	h3 := computeHash("SessionStart", "gt prime", "gastown/crew/bob")
	if h1 == h3 {
		t.Errorf("Different inputs produced same hash: %s", h1)
	}

	// Hash should be 16 characters (truncated)
	if len(h1) != 16 {
		t.Errorf("Hash length should be 16, got %d", len(h1))
	}
}

func TestErrorLog_ReportAndGet(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log := NewErrorLog(tmpDir)

	// Report an error
	logged, err := log.ReportError("SessionStart", "gt prime --hook", 1, "connection failed", "gastown/crew/test")
	if err != nil {
		t.Fatalf("ReportError failed: %v", err)
	}
	if !logged {
		t.Error("First error should be logged, not deduplicated")
	}

	// Get recent errors
	errors, err := log.GetRecentErrors(10)
	if err != nil {
		t.Fatalf("GetRecentErrors failed: %v", err)
	}
	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}

	// Verify error contents
	e := errors[0]
	if e.HookType != "SessionStart" {
		t.Errorf("Expected hook type 'SessionStart', got '%s'", e.HookType)
	}
	if e.Command != "gt prime --hook" {
		t.Errorf("Expected command 'gt prime --hook', got '%s'", e.Command)
	}
	if e.ExitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", e.ExitCode)
	}
	if e.Stderr != "connection failed" {
		t.Errorf("Expected stderr 'connection failed', got '%s'", e.Stderr)
	}
	if e.Role != "gastown/crew/test" {
		t.Errorf("Expected role 'gastown/crew/test', got '%s'", e.Role)
	}
	if e.Count != 1 {
		t.Errorf("Expected count 1, got %d", e.Count)
	}
}

func TestErrorLog_Deduplication(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log := NewErrorLog(tmpDir)
	// Use a short dedup window for testing
	log.dedupWindow = 5 * time.Second

	// Report the same error twice
	logged1, _ := log.ReportError("SessionStart", "gt prime", 1, "", "role1")
	logged2, _ := log.ReportError("SessionStart", "gt prime", 1, "", "role1")

	if !logged1 {
		t.Error("First error should be logged")
	}
	if logged2 {
		t.Error("Second error should be deduplicated")
	}

	// Check that count was incremented
	errors, _ := log.GetRecentErrors(10)
	if len(errors) != 1 {
		t.Errorf("Expected 1 error entry, got %d", len(errors))
	}
	if errors[0].Count != 2 {
		t.Errorf("Expected count 2, got %d", errors[0].Count)
	}
}

func TestErrorLog_DifferentErrorsNotDeduplicated(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log := NewErrorLog(tmpDir)

	// Report different errors
	log.ReportError("SessionStart", "cmd1", 1, "", "role1")
	log.ReportError("SessionStart", "cmd2", 1, "", "role1") // Different command
	log.ReportError("UserPromptSubmit", "cmd1", 1, "", "role1") // Different hook type
	log.ReportError("SessionStart", "cmd1", 1, "", "role2") // Different role

	errors, _ := log.GetRecentErrors(10)
	if len(errors) != 4 {
		t.Errorf("Expected 4 different errors, got %d", len(errors))
	}
}

func TestErrorLog_StderrTruncation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log := NewErrorLog(tmpDir)

	// Create a very long stderr
	longStderr := ""
	for i := 0; i < 1000; i++ {
		longStderr += "x"
	}

	log.ReportError("SessionStart", "cmd", 1, longStderr, "role")

	errors, _ := log.GetRecentErrors(10)
	if len(errors[0].Stderr) > 510 {
		t.Errorf("Stderr should be truncated, got length %d", len(errors[0].Stderr))
	}
	if !contains(errors[0].Stderr, "...") {
		t.Error("Truncated stderr should end with ...")
	}
}

func TestErrorLog_ClearErrors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log := NewErrorLog(tmpDir)

	// Add some errors
	log.ReportError("SessionStart", "cmd1", 1, "", "role1")
	log.ReportError("SessionStart", "cmd2", 1, "", "role1")

	// Clear
	err = log.ClearErrors()
	if err != nil {
		t.Fatalf("ClearErrors failed: %v", err)
	}

	// Should be empty now
	errors, _ := log.GetRecentErrors(10)
	if len(errors) != 0 {
		t.Errorf("Expected 0 errors after clear, got %d", len(errors))
	}
}

func TestErrorLog_MaxErrorsLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log := NewErrorLog(tmpDir)

	// Add more than MaxErrorsToKeep errors
	for i := 0; i < MaxErrorsToKeep+50; i++ {
		// Use different commands to avoid deduplication
		log.ReportError("SessionStart", "cmd"+string(rune(i)), 1, "", "role")
	}

	// File reading might have issues with invalid runes, let's check we have at most MaxErrorsToKeep
	errors, _ := log.GetRecentErrors(0) // 0 = no limit
	if len(errors) > MaxErrorsToKeep {
		t.Errorf("Expected at most %d errors, got %d", MaxErrorsToKeep, len(errors))
	}
}

func TestErrorLog_GetErrorsSince(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	log := NewErrorLog(tmpDir)

	// Add an error
	log.ReportError("SessionStart", "cmd1", 1, "", "role1")

	// Get errors since now (should be empty)
	errors, _ := log.GetErrorsSince(time.Now().Add(1 * time.Second))
	if len(errors) != 0 {
		t.Errorf("Expected 0 errors since future, got %d", len(errors))
	}

	// Get errors since the past (should include our error)
	errors, _ = log.GetErrorsSince(time.Now().Add(-1 * time.Hour))
	if len(errors) != 1 {
		t.Errorf("Expected 1 error since past, got %d", len(errors))
	}
}

func TestErrorLog_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create first log instance and add error
	log1 := NewErrorLog(tmpDir)
	log1.ReportError("SessionStart", "cmd1", 1, "error msg", "role1")

	// Create second log instance and verify error persisted
	log2 := NewErrorLog(tmpDir)
	errors, _ := log2.GetRecentErrors(10)
	if len(errors) != 1 {
		t.Errorf("Expected 1 error after reload, got %d", len(errors))
	}
	if errors[0].Command != "cmd1" {
		t.Errorf("Expected command 'cmd1', got '%s'", errors[0].Command)
	}
}

func TestErrorLog_DirectoryCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Use a path that doesn't exist yet
	nonExistentPath := filepath.Join(tmpDir, "subdir", "nested")
	log := NewErrorLog(nonExistentPath)

	// Should create directory when reporting
	_, err = log.ReportError("SessionStart", "cmd", 1, "", "role")
	if err != nil {
		t.Fatalf("ReportError should create directory: %v", err)
	}

	// Verify directory was created
	runtimeDir := filepath.Join(nonExistentPath, ".runtime")
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		t.Error("Runtime directory should be created")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}
