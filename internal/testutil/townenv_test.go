package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRequireTownEnv_ReturnsRoot(t *testing.T) {
	root := RequireTownEnv(t)

	// If we got here (didn't skip), root must be non-empty.
	if root == "" {
		t.Fatal("RequireTownEnv returned empty root")
	}

	// The returned root must contain mayor/rigs.json (the check we just added).
	rigsPath := filepath.Join(root, "mayor", "rigs.json")
	if _, err := os.Stat(rigsPath); err != nil {
		t.Errorf("mayor/rigs.json not found at %s: %v", rigsPath, err)
	}
}
