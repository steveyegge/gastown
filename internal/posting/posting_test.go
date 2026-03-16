package posting

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/constants"
)

func TestReadEmpty(t *testing.T) {
	dir := t.TempDir()
	got := Read(dir)
	if got != "" {
		t.Errorf("Read(empty dir) = %q, want empty", got)
	}
}

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "security-reviewer"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	got := Read(dir)
	if got != "security-reviewer" {
		t.Errorf("Read() = %q, want %q", got, "security-reviewer")
	}
}

func TestWriteTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "  docs-writer  "); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	got := Read(dir)
	if got != "docs-writer" {
		t.Errorf("Read() = %q, want %q", got, "docs-writer")
	}
}

func TestWriteEmptyClears(t *testing.T) {
	dir := t.TempDir()
	// Write a posting first
	if err := Write(dir, "tester"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	// Writing empty should clear
	if err := Write(dir, ""); err != nil {
		t.Fatalf("Write('') error: %v", err)
	}
	got := Read(dir)
	if got != "" {
		t.Errorf("Read() after Write('') = %q, want empty", got)
	}
	// File should not exist
	path := filepath.Join(dir, constants.DirRuntime, FilePosting)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("posting file should not exist after clearing")
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	// Write then clear
	if err := Write(dir, "reviewer"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if err := Clear(dir); err != nil {
		t.Fatalf("Clear() error: %v", err)
	}
	got := Read(dir)
	if got != "" {
		t.Errorf("Read() after Clear() = %q, want empty", got)
	}
}

func TestClearNoop(t *testing.T) {
	dir := t.TempDir()
	// Clear on non-existent should not error
	if err := Clear(dir); err != nil {
		t.Fatalf("Clear() on empty dir error: %v", err)
	}
}

func TestWriteCreatesRuntimeDir(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "auditor"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	runtimeDir := filepath.Join(dir, constants.DirRuntime)
	info, err := os.Stat(runtimeDir)
	if err != nil {
		t.Fatalf(".runtime dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".runtime is not a directory")
	}
}

func TestOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "first"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if err := Write(dir, "second"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	got := Read(dir)
	if got != "second" {
		t.Errorf("Read() = %q, want %q", got, "second")
	}
}
