package witness

import (
	"os/exec"
	"testing"
)

func TestIsOJJobActive_NoOjBinary(t *testing.T) {
	// When the oj binary is not in PATH, isOJJobActive should return false
	// (graceful degradation — assume not active, allow nuke).
	if _, err := exec.LookPath("oj"); err == nil {
		t.Skip("oj binary found in PATH, skipping no-binary test")
	}

	got := isOJJobActive("/tmp", "job-123")
	if got {
		t.Errorf("isOJJobActive(\"/tmp\", \"job-123\") = true, want false when oj binary is missing")
	}
}

func TestIsOJJobActive_EmptyJobID(t *testing.T) {
	// An empty job ID should always return false (no job to check).
	got := isOJJobActive("/tmp", "")
	if got {
		t.Errorf("isOJJobActive(\"/tmp\", \"\") = true, want false for empty job ID")
	}
}
