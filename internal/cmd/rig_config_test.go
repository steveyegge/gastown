package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestGetConfigKeys_IncludesRigDefaultBranch(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		t.Fatal(err)
	}

	configJSON := `{
  "type": "rig",
  "version": 1,
  "name": "testrig",
  "git_url": "https://example.com/repo.git",
  "default_branch": "codex/custom-base",
  "created_at": "2026-01-01T00:00:00Z"
}`
	if err := os.WriteFile(filepath.Join(rigPath, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name: "testrig",
		Path: rigPath,
	}

	keys := getConfigKeys(townRoot, r)
	found := false
	for _, key := range keys {
		if key == "default_branch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected default_branch in config keys, got %v", keys)
	}
}
