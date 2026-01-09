// ABOUTME: Tests for shell integration install/remove functionality.
// ABOUTME: Verifies RC file manipulation and hook script creation.

package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		shellEnv string
		want     string
	}{
		{"/bin/zsh", "zsh"},
		{"/usr/bin/zsh", "zsh"},
		{"/bin/bash", "bash"},
		{"/usr/bin/bash", "bash"},
		{"", "zsh"},
	}

	for _, tt := range tests {
		t.Run(tt.shellEnv, func(t *testing.T) {
			orig := os.Getenv("SHELL")
			defer os.Setenv("SHELL", orig)

			os.Setenv("SHELL", tt.shellEnv)
			got := DetectShell()
			if got != tt.want {
				t.Errorf("DetectShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRCFilePath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		shell string
		want  string
	}{
		{"zsh", filepath.Join(home, ".zshrc")},
		{"bash", filepath.Join(home, ".bashrc")},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			got := RCFilePath(tt.shell)
			if got != tt.want {
				t.Errorf("RCFilePath(%q) = %q, want %q", tt.shell, got, tt.want)
			}
		})
	}
}

func TestAddRemoveFromRCFile(t *testing.T) {
	tmpDir := t.TempDir()
	rcPath := filepath.Join(tmpDir, ".zshrc")

	originalContent := "# existing content\nalias foo=bar\n"
	if err := os.WriteFile(rcPath, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := addToRCFile(rcPath); err != nil {
		t.Fatalf("addToRCFile() error = %v", err)
	}

	data, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, markerStart) {
		t.Error("RC file should contain start marker")
	}
	if !strings.Contains(content, markerEnd) {
		t.Error("RC file should contain end marker")
	}
	if !strings.Contains(content, "shell-hook.sh") {
		t.Error("RC file should source shell-hook.sh")
	}
	if !strings.Contains(content, "# existing content") {
		t.Error("RC file should preserve original content")
	}

	if err := removeFromRCFile(rcPath); err != nil {
		t.Fatalf("removeFromRCFile() error = %v", err)
	}

	data, err = os.ReadFile(rcPath)
	if err != nil {
		t.Fatal(err)
	}
	content = string(data)

	if strings.Contains(content, markerStart) {
		t.Error("RC file should not contain start marker after removal")
	}
	if strings.Contains(content, markerEnd) {
		t.Error("RC file should not contain end marker after removal")
	}
	if !strings.Contains(content, "# existing content") {
		t.Error("RC file should preserve original content after removal")
	}
}

func TestUpdateRCFile(t *testing.T) {
	tmpDir := t.TempDir()
	rcPath := filepath.Join(tmpDir, ".zshrc")

	if err := addToRCFile(rcPath); err != nil {
		t.Fatalf("initial addToRCFile() error = %v", err)
	}

	if err := addToRCFile(rcPath); err != nil {
		t.Fatalf("second addToRCFile() error = %v", err)
	}

	data, _ := os.ReadFile(rcPath)
	content := string(data)

	startCount := strings.Count(content, markerStart)
	if startCount != 1 {
		t.Errorf("RC file has %d start markers, want 1", startCount)
	}
}
