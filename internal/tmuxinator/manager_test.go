package tmuxinator

import (
	"testing"
)

func TestIsAvailable(t *testing.T) {
	// This test checks that IsAvailable doesn't panic.
	// The result depends on whether tmuxinator is installed on the system.
	available := IsAvailable()
	t.Logf("tmuxinator available: %v", available)
}

func TestVersion(t *testing.T) {
	if !IsAvailable() {
		t.Skip("tmuxinator not installed")
	}

	version, err := Version()
	if err != nil {
		t.Fatalf("Version() error: %v", err)
	}
	if version == "" {
		t.Error("Version() returned empty string")
	}
	t.Logf("tmuxinator version: %s", version)
}

func TestStart_InvalidPath(t *testing.T) {
	if !IsAvailable() {
		t.Skip("tmuxinator not installed")
	}

	err := Start("/nonexistent/path/config.yml")
	if err == nil {
		t.Error("Start() with invalid path should return error")
	}
}
