package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDoltBinaryCheck_Metadata(t *testing.T) {
	check := NewDoltBinaryCheck()

	if check.Name() != "dolt-binary" {
		t.Errorf("Name() = %q, want %q", check.Name(), "dolt-binary")
	}
	if check.Description() != "Check that dolt is installed and in PATH" {
		t.Errorf("Description() = %q", check.Description())
	}
	if check.Category() != CategoryInfrastructure {
		t.Errorf("Category() = %q, want %q", check.Category(), CategoryInfrastructure)
	}
	if check.CanFix() {
		t.Error("CanFix() should return false (user must install dolt manually)")
	}
}

func TestDoltBinaryCheck_DoltInstalled(t *testing.T) {
	// Skip if dolt is not actually installed in the test environment
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not installed, skipping installed-path test")
	}

	check := NewDoltBinaryCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when dolt is installed, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "dolt version") {
		t.Errorf("expected version string in message, got %q", result.Message)
	}
}

// writeFakeDolt creates a platform-appropriate fake "dolt" executable in dir.
// On Unix, it writes a shell script. On Windows, it writes a .bat file.
// Returns the filename written (e.g. "dolt" or "dolt.bat").
func writeFakeDolt(t *testing.T, dir string, script string, batScript string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "dolt.bat")
		if err := os.WriteFile(path, []byte(batScript), 0755); err != nil {
			t.Fatal(err)
		}
	} else {
		path := filepath.Join(dir, "dolt")
		if err := os.WriteFile(path, []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDoltBinaryCheck_HermeticSuccess(t *testing.T) {
	// Create a fake "dolt" binary that prints a version string
	fakeDir := t.TempDir()
	writeFakeDolt(t, fakeDir,
		"#!/bin/sh\necho 'dolt version 1.0.0'\n",
		"@echo off\r\necho dolt version 1.0.0\r\n",
	)

	t.Setenv("PATH", fakeDir)

	check := NewDoltBinaryCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK with fake dolt, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "dolt version 1.0.0" {
		t.Errorf("expected 'dolt version 1.0.0', got %q", result.Message)
	}
}

func TestDoltBinaryCheck_DoltNotInPath(t *testing.T) {
	// Create an empty directory and set PATH to only that directory,
	// ensuring dolt (and nothing else) is findable.
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	check := NewDoltBinaryCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError when dolt is not in PATH, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "dolt not found in PATH" {
		t.Errorf("unexpected message: %q", result.Message)
	}
	if len(result.Details) != 2 {
		t.Errorf("expected 2 detail lines, got %d", len(result.Details))
	}
	if result.FixHint == "" {
		t.Error("expected a fix hint with install instructions")
	}
	if !strings.Contains(result.FixHint, "dolthub/dolt") {
		t.Errorf("fix hint should reference dolthub/dolt, got %q", result.FixHint)
	}
}

func TestDoltBinaryCheck_DoltVersionFails(t *testing.T) {
	// Create a fake "dolt" binary that exits with an error
	fakeDir := t.TempDir()
	writeFakeDolt(t, fakeDir,
		"#!/bin/sh\nexit 1\n",
		"@echo off\r\nexit /b 1\r\n",
	)

	t.Setenv("PATH", fakeDir)

	check := NewDoltBinaryCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError when dolt version fails, got %v: %s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "dolt version") {
		t.Errorf("expected message to mention 'dolt version', got %q", result.Message)
	}
	if result.FixHint == "" {
		t.Error("expected a fix hint for broken dolt")
	}
}
