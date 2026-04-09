package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBdError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *bdError
		want string
	}{
		{
			name: "stderr present",
			err: &bdError{
				Err:    errors.New("some error"),
				Stderr: "stderr output",
			},
			want: "stderr output",
		},
		{
			name: "no stderr, has error",
			err: &bdError{
				Err:    errors.New("some error"),
				Stderr: "",
			},
			want: "some error",
		},
		{
			name: "no stderr, no error",
			err: &bdError{
				Err:    nil,
				Stderr: "",
			},
			want: "unknown bd error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("bdError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBdError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	bdErr := &bdError{
		Err:    originalErr,
		Stderr: "stderr output",
	}

	unwrapped := bdErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("bdError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}
}

func TestBdError_UnwrapNil(t *testing.T) {
	bdErr := &bdError{
		Err:    nil,
		Stderr: "",
	}

	unwrapped := bdErr.Unwrap()
	if unwrapped != nil {
		t.Errorf("bdError.Unwrap() with nil Err should return nil, got %v", unwrapped)
	}
}

func TestBdError_ContainsError(t *testing.T) {
	tests := []struct {
		name     string
		err      *bdError
		substr   string
		contains bool
	}{
		{
			name: "substring present",
			err: &bdError{
				Stderr: "error: bead not found",
			},
			substr:   "bead not found",
			contains: true,
		},
		{
			name: "substring not present",
			err: &bdError{
				Stderr: "error: bead not found",
			},
			substr:   "permission denied",
			contains: false,
		},
		{
			name: "empty stderr",
			err: &bdError{
				Stderr: "",
			},
			substr:   "anything",
			contains: false,
		},
		{
			name: "case sensitive",
			err: &bdError{
				Stderr: "Error: Bead Not Found",
			},
			substr:   "bead not found",
			contains: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.ContainsError(tt.substr)
			if got != tt.contains {
				t.Errorf("bdError.ContainsError(%q) = %v, want %v", tt.substr, got, tt.contains)
			}
		})
	}
}

func TestBdError_ContainsErrorPartialMatch(t *testing.T) {
	err := &bdError{
		Stderr: "fatal: invalid bead ID format: expected prefix-#id",
	}

	// Test partial matches
	if !err.ContainsError("invalid bead ID") {
		t.Error("Should contain partial substring")
	}
	if !err.ContainsError("fatal:") {
		t.Error("Should contain prefix")
	}
	if !err.ContainsError("expected prefix") {
		t.Error("Should contain suffix")
	}
}

func TestBdError_ContainsErrorSpecialChars(t *testing.T) {
	err := &bdError{
		Stderr: "error: bead 'gt-123' not found (exit 1)",
	}

	if !err.ContainsError("'gt-123'") {
		t.Error("Should handle quotes in substring")
	}
	if !err.ContainsError("(exit 1)") {
		t.Error("Should handle parentheses in substring")
	}
}

func TestBdError_ImplementsErrorInterface(t *testing.T) {
	// Verify bdError implements error interface
	var err error = &bdError{
		Err:    errors.New("test"),
		Stderr: "test stderr",
	}

	_ = err.Error() // Should compile and not panic
}

func TestBdError_WithAllFields(t *testing.T) {
	originalErr := errors.New("original error")
	bdErr := &bdError{
		Err:    originalErr,
		Stderr: "command failed: bead not found",
	}

	// Test Error() returns stderr
	got := bdErr.Error()
	want := "command failed: bead not found"
	if got != want {
		t.Errorf("bdError.Error() = %q, want %q", got, want)
	}

	// Test Unwrap() returns original error
	unwrapped := bdErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("bdError.Unwrap() = %v, want %v", unwrapped, originalErr)
	}

	// Test ContainsError works
	if !bdErr.ContainsError("bead not found") {
		t.Error("ContainsError should find substring in stderr")
	}
	if bdErr.ContainsError("not present") {
		t.Error("ContainsError should return false for non-existent substring")
	}
}

func TestRunBdCommand_UsesBeadsDirParentAsWorkingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a bash bd stub")
	}

	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	beadsDir := filepath.Join(tmpDir, "town", ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("mkdir beads dir: %v", err)
	}

	stubPath := filepath.Join(binDir, "bd")
	script := `#!/usr/bin/env bash
set -euo pipefail
printf 'cwd=%s\n' "$PWD"
printf 'beads=%s\n' "$BEADS_DIR"
printf 'db=%s\n' "${BEADS_DOLT_SERVER_DATABASE-}"
printf 'args=%s\n' "$*"
`
	if err := os.WriteFile(stubPath, []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stdout, err := runBdCommand(context.Background(), []string{"show", "gs-wisp-ext", "--json"}, filepath.Join(tmpDir, "repo"), beadsDir)
	if err != nil {
		t.Fatalf("runBdCommand() error = %v, want nil", err)
	}

	out := string(stdout)
	if !strings.Contains(out, "cwd="+filepath.Dir(beadsDir)) {
		t.Fatalf("runBdCommand() output %q missing cwd=%s", out, filepath.Dir(beadsDir))
	}
	if !strings.Contains(out, "beads="+beadsDir) {
		t.Fatalf("runBdCommand() output %q missing beads=%s", out, beadsDir)
	}
	if !strings.Contains(out, "db=\n") {
		t.Fatalf("runBdCommand() output %q should leave BEADS_DOLT_SERVER_DATABASE unset", out)
	}
}

func TestIsBdNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matches not found",
			err:  &bdError{Stderr: "error: bead not found"},
			want: true,
		},
		{
			name: "matches no issue found",
			err:  &bdError{Stderr: "error: no issue found matching \"gt-123\""},
			want: true,
		},
		{
			name: "ignores other bd errors",
			err:  &bdError{Stderr: "permission denied"},
			want: false,
		},
		{
			name: "ignores non bd errors",
			err:  errors.New("no issue found matching \"gt-123\""),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBdNotFoundError(tt.err); got != tt.want {
				t.Errorf("isBdNotFoundError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsMessageNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matches bd no issue found",
			err:  &bdError{Stderr: "error: no issue found matching \"gt-123\""},
			want: true,
		},
		{
			name: "matches store no issue found",
			err:  errors.New("store: no issue found matching \"gt-123\""),
			want: true,
		},
		{
			name: "ignores unrelated errors",
			err:  errors.New("permission denied"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMessageNotFoundError(tt.err); got != tt.want {
				t.Errorf("isMessageNotFoundError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
