package util

import (
	"os"
	"strings"
	"testing"
)

func TestExecWithOutput(t *testing.T) {
	// Test successful command
	output, err := ExecWithOutput(".", "echo", "hello")
	if err != nil {
		t.Fatalf("ExecWithOutput failed: %v", err)
	}
	if output != "hello" {
		t.Errorf("expected 'hello', got %q", output)
	}

	// Test command that fails
	_, err = ExecWithOutput(".", "false")
	if err == nil {
		t.Error("expected error for failing command")
	}
}

func TestExecRun(t *testing.T) {
	// Test successful command
	err := ExecRun(".", "true")
	if err != nil {
		t.Fatalf("ExecRun failed: %v", err)
	}

	// Test command that fails
	err = ExecRun(".", "false")
	if err == nil {
		t.Error("expected error for failing command")
	}
}

func TestExecWithOutput_WorkDir(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "exec-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test that workDir is respected
	output, err := ExecWithOutput(tmpDir, "pwd")
	if err != nil {
		t.Fatalf("ExecWithOutput failed: %v", err)
	}
	if !strings.Contains(output, tmpDir) && !strings.Contains(tmpDir, output) {
		t.Errorf("expected output to contain %q, got %q", tmpDir, output)
	}
}

func TestExecWithOutput_StderrInError(t *testing.T) {
	// Test that stderr is captured in error
	_, err := ExecWithOutput(".", "sh", "-c", "echo 'error message' >&2; exit 1")
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "error message") {
		t.Errorf("expected error to contain stderr, got %q", err.Error())
	}
}
