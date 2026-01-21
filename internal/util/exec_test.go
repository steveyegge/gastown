package util

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestExecWithOutput(t *testing.T) {
	// Test successful command
	var output string
	var err error
	if runtime.GOOS == "windows" {
		output, err = ExecWithOutput(".", "cmd", "/c", "echo hello")
	} else {
		output, err = ExecWithOutput(".", "echo", "hello")
	}
	if err != nil {
		t.Fatalf("ExecWithOutput failed: %v", err)
	}
	if output != "hello" {
		t.Errorf("expected 'hello', got %q", output)
	}

	// Test command that fails
	if runtime.GOOS == "windows" {
		_, err = ExecWithOutput(".", "cmd", "/c", "exit /b 1")
	} else {
		_, err = ExecWithOutput(".", "false")
	}
	if err == nil {
		t.Error("expected error for failing command")
	}
}

func TestExecRun(t *testing.T) {
	// Test successful command
	var err error
	if runtime.GOOS == "windows" {
		err = ExecRun(".", "cmd", "/c", "exit /b 0")
	} else {
		err = ExecRun(".", "true")
	}
	if err != nil {
		t.Fatalf("ExecRun failed: %v", err)
	}

	// Test command that fails
	if runtime.GOOS == "windows" {
		err = ExecRun(".", "cmd", "/c", "exit /b 1")
	} else {
		err = ExecRun(".", "false")
	}
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
	var output string
	if runtime.GOOS == "windows" {
		output, err = ExecWithOutput(tmpDir, "cmd", "/c", "cd")
	} else {
		output, err = ExecWithOutput(tmpDir, "pwd")
	}
	if err != nil {
		t.Fatalf("ExecWithOutput failed: %v", err)
	}
	if !strings.Contains(output, tmpDir) && !strings.Contains(tmpDir, output) {
		t.Errorf("expected output to contain %q, got %q", tmpDir, output)
	}
}

func TestExecWithOutput_StderrInError(t *testing.T) {
	// Test that stderr is captured in error
	var err error
	if runtime.GOOS == "windows" {
		_, err = ExecWithOutput(".", "cmd", "/c", "echo error message 1>&2 & exit /b 1")
	} else {
		_, err = ExecWithOutput(".", "sh", "-c", "echo 'error message' >&2; exit 1")
	}
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "error message") {
		t.Errorf("expected error to contain stderr, got %q", err.Error())
	}
}
