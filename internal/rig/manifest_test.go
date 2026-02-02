package rig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestAndCrewSpecs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestDir := filepath.Join(root, ".gt")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatalf("mkdir .gt: %v", err)
	}

	content := `version = 1

[rig]
name = "gastown"
prefix = "gt"
default_branch = "main"

[git]
upstream = "https://github.com/steveyegge/gastown.git"
fork_policy = "prompt"

[setup]
command = "go install ./cmd/gt"
workdir = "."

[[crew]]
name = "max"
agent = "codex"
model = "gpt-5"
account = "work"
branch = true
args = ["--debug"]
env = { GT_FOO = "bar" }

[[crew]]
name = "alex"
branch = "feature/alex"
`

	if err := os.WriteFile(filepath.Join(manifestDir, "rig.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if manifest == nil {
		t.Fatal("expected manifest, got nil")
	}

	specs, err := manifest.CrewSpecs()
	if err != nil {
		t.Fatalf("CrewSpecs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("spec count = %d, want 2", len(specs))
	}

	if !specs[0].CreateBranch {
		t.Errorf("specs[0].CreateBranch = false, want true")
	}
	if specs[0].BranchName != "" {
		t.Errorf("specs[0].BranchName = %q, want empty", specs[0].BranchName)
	}
	if specs[0].Agent != "codex" {
		t.Errorf("specs[0].Agent = %q, want codex", specs[0].Agent)
	}
	if specs[0].Model != "gpt-5" {
		t.Errorf("specs[0].Model = %q, want gpt-5", specs[0].Model)
	}
	if specs[0].Account != "work" {
		t.Errorf("specs[0].Account = %q, want work", specs[0].Account)
	}
	if len(specs[0].Args) != 1 || specs[0].Args[0] != "--debug" {
		t.Errorf("specs[0].Args = %v, want [--debug]", specs[0].Args)
	}
	if specs[0].Env == nil || specs[0].Env["GT_FOO"] != "bar" {
		t.Errorf("specs[0].Env = %v, want GT_FOO=bar", specs[0].Env)
	}
	if specs[1].BranchName != "feature/alex" {
		t.Errorf("specs[1].BranchName = %q, want feature/alex", specs[1].BranchName)
	}
}

func TestLoadManifestMissing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifest, err := LoadManifest(root)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if manifest != nil {
		t.Fatalf("expected nil manifest, got %+v", manifest)
	}
}

func TestLoadManifestInvalidVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manifestDir := filepath.Join(root, ".gt")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatalf("mkdir .gt: %v", err)
	}

	content := `version = 2`
	if err := os.WriteFile(filepath.Join(manifestDir, "rig.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := LoadManifest(root); err == nil {
		t.Fatal("expected error for unsupported version")
	}
}
