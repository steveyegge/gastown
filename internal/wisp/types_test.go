package wisp

import "testing"

func TestWispDir(t *testing.T) {
	// Test that WispDir constant is defined correctly
	expected := ".beads"
	if WispDir != expected {
		t.Errorf("WispDir = %q, want %q", WispDir, expected)
	}
}

func TestWispDirNotEmpty(t *testing.T) {
	// Test that WispDir is not empty
	if WispDir == "" {
		t.Error("WispDir should not be empty")
	}
}

func TestWispDirStartsWithDot(t *testing.T) {
	// Test that WispDir is a hidden directory (starts with dot)
	if len(WispDir) == 0 || WispDir[0] != '.' {
		t.Errorf("WispDir should start with '.' for hidden directory, got %q", WispDir)
	}
}
