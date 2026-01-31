package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewBeadsRedirectCheck(t *testing.T) {
	check := NewBeadsRedirectCheck()

	if check.Name() != "beads-redirect" {
		t.Errorf("expected name 'beads-redirect', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestBeadsRedirectCheck_NoRigSpecified(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: ""}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rig specified, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "skipping") {
		t.Errorf("expected message about skipping, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_NoBeadsAtAll(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError when no beads exist (fixable), got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_LocalBeadsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create local beads at rig root (no mayor/rig/.beads)
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for local beads (no redirect needed), got %v", result.Status)
	}
	if !strings.Contains(result.Message, "local beads") {
		t.Errorf("expected message about local beads, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_TrackedBeadsMissingRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing redirect, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "Missing") {
		t.Errorf("expected message about missing redirect, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_TrackedBeadsCorrectRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads with correct redirect
	rigBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	redirectPath := filepath.Join(rigBeads, "redirect")
	if err := os.WriteFile(redirectPath, []byte("mayor/rig/.beads\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for correct redirect, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "correctly configured") {
		t.Errorf("expected message about correct config, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_TrackedBeadsWrongRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads with wrong redirect
	rigBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	redirectPath := filepath.Join(rigBeads, "redirect")
	if err := os.WriteFile(redirectPath, []byte("wrong/path\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong redirect (fixable), got %v", result.Status)
	}
	if !strings.Contains(result.Message, "wrong/path") {
		t.Errorf("expected message to contain wrong path, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_FixWrongRedirect(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig-level .beads with wrong redirect
	rigBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	redirectPath := filepath.Join(rigBeads, "redirect")
	if err := os.WriteFile(redirectPath, []byte("wrong/path\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify redirect was corrected
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("redirect file not found: %v", err)
	}
	if string(content) != "mayor/rig/.beads\n" {
		t.Errorf("redirect content = %q, want 'mayor/rig/.beads\\n'", string(content))
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify redirect file was created
	redirectPath := filepath.Join(rigDir, ".beads", "redirect")
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("redirect file not created: %v", err)
	}

	expected := "mayor/rig/.beads\n"
	if string(content) != expected {
		t.Errorf("redirect content = %q, want %q", string(content), expected)
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_FixNoOp_LocalBeads(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create only local beads (no tracked beads)
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Fix should be a no-op
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify no redirect was created
	redirectPath := filepath.Join(rigDir, ".beads", "redirect")
	if _, err := os.Stat(redirectPath); !os.IsNotExist(err) {
		t.Error("redirect file should not be created for local beads")
	}
}

func TestBeadsRedirectCheck_FixInitBeads(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create rig directory (no beads at all)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create mayor/rigs.json with prefix for the rig
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{
		"version": 1,
		"rigs": {
			"testrig": {
				"git_url": "https://example.com/test.git",
				"beads": {
					"prefix": "tr"
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - this will run 'bd init' if available, otherwise create config.yaml
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify .beads directory was created
	beadsDir := filepath.Join(rigDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		t.Fatal(".beads directory not created")
	}

	// Verify beads was initialized (either by bd init or fallback)
	// bd init creates config.yaml, fallback creates config.yaml with prefix
	configPath := filepath.Join(beadsDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config.yaml not created")
	}

	// Verify check now passes (local beads exist)
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestBeadsRedirectCheck_ConflictingLocalBeads(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}
	// Add some content to tracked beads
	if err := os.WriteFile(filepath.Join(trackedBeads, "issues.jsonl"), []byte(`{"id":"tr-1"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create conflicting local beads with actual data
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}
	// Add data to local beads (this is the conflict)
	if err := os.WriteFile(filepath.Join(localBeads, "issues.jsonl"), []byte(`{"id":"local-1"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localBeads, "config.yaml"), []byte("prefix: local\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Check should detect conflicting beads
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected StatusError for conflicting beads, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "Conflicting") {
		t.Errorf("expected message about conflicting beads, got %q", result.Message)
	}
}

func TestBeadsRedirectCheck_FixConflictingLocalBeads(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create tracked beads at mayor/rig/.beads
	trackedBeads := filepath.Join(rigDir, "mayor", "rig", ".beads")
	if err := os.MkdirAll(trackedBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trackedBeads, "issues.jsonl"), []byte(`{"id":"tr-1"}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create conflicting local beads with actual data
	localBeads := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(localBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localBeads, "issues.jsonl"), []byte(`{"id":"local-1"}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBeadsRedirectCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should remove conflicting local beads and create redirect
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify local issues.jsonl was removed
	if _, err := os.Stat(filepath.Join(localBeads, "issues.jsonl")); !os.IsNotExist(err) {
		t.Error("local issues.jsonl should have been removed")
	}

	// Verify redirect was created
	redirectPath := filepath.Join(localBeads, "redirect")
	content, err := os.ReadFile(redirectPath)
	if err != nil {
		t.Fatalf("redirect file not created: %v", err)
	}
	if string(content) != "mayor/rig/.beads\n" {
		t.Errorf("redirect content = %q, want 'mayor/rig/.beads\\n'", string(content))
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

// Tests for BareRepoIntegrityCheck

func TestNewBareRepoIntegrityCheck(t *testing.T) {
	check := NewBareRepoIntegrityCheck()

	if check.Name() != "bare-repo-integrity" {
		t.Errorf("expected name 'bare-repo-integrity', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestBareRepoIntegrityCheck_NoRigSpecified(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: ""}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no rig specified, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "skipping") {
		t.Errorf("expected message about skipping, got %q", result.Message)
	}
}

func TestBareRepoIntegrityCheck_NoBareRepo(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no bare repo, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "No shared bare repo") {
		t.Errorf("expected message about no bare repo, got %q", result.Message)
	}
}

func TestBareRepoIntegrityCheck_ValidBareRepo(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	bareRepoDir := filepath.Join(rigDir, ".repo.git")

	// Create a valid bare repo structure
	dirs := []string{"objects", "refs", "info", "hooks", "branches"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(bareRepoDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create required files
	if err := os.WriteFile(filepath.Join(bareRepoDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareRepoDir, "config"), []byte("[core]\nbare = true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid bare repo, got %v: %s", result.Status, result.Message)
	}
}

func TestBareRepoIntegrityCheck_MissingHEAD(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	bareRepoDir := filepath.Join(rigDir, ".repo.git")

	// Create bare repo structure without HEAD
	dirs := []string{"objects", "refs", "info", "hooks", "branches"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(bareRepoDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Only create config, not HEAD
	if err := os.WriteFile(filepath.Join(bareRepoDir, "config"), []byte("[core]\nbare = true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing HEAD, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "missing") {
		t.Errorf("expected message about missing files, got %q", result.Message)
	}
}

func TestBareRepoIntegrityCheck_MissingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	bareRepoDir := filepath.Join(rigDir, ".repo.git")

	// Create bare repo structure without config
	dirs := []string{"objects", "refs", "info", "hooks", "branches"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(bareRepoDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Only create HEAD, not config
	if err := os.WriteFile(filepath.Join(bareRepoDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing config, got %v", result.Status)
	}
}

func TestBareRepoIntegrityCheck_MissingDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	bareRepoDir := filepath.Join(rigDir, ".repo.git")

	// Create bare repo with files but no directories
	if err := os.MkdirAll(bareRepoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareRepoDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bareRepoDir, "config"), []byte("[core]\nbare = true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing directories, got %v", result.Status)
	}
	if !strings.Contains(strings.Join(result.Details, " "), "objects") {
		t.Errorf("expected details to mention objects directory, got %v", result.Details)
	}
}

func TestBareRepoIntegrityCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	bareRepoDir := filepath.Join(rigDir, ".repo.git")

	// Create empty bare repo directory (missing everything)
	if err := os.MkdirAll(bareRepoDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify HEAD was created
	headContent, err := os.ReadFile(filepath.Join(bareRepoDir, "HEAD"))
	if err != nil {
		t.Fatalf("HEAD not created: %v", err)
	}
	if !strings.Contains(string(headContent), "refs/heads/main") {
		t.Errorf("HEAD content = %q, expected ref to main", string(headContent))
	}

	// Verify config was created
	configContent, err := os.ReadFile(filepath.Join(bareRepoDir, "config"))
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if !strings.Contains(string(configContent), "bare = true") {
		t.Errorf("config content = %q, expected bare = true", string(configContent))
	}

	// Verify directories were created
	for _, dir := range []string{"objects", "refs", "info", "hooks", "branches"} {
		info, err := os.Stat(filepath.Join(bareRepoDir, dir))
		if os.IsNotExist(err) {
			t.Errorf("directory %s not created", dir)
		} else if !info.IsDir() {
			t.Errorf("%s exists but is not a directory", dir)
		}
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}
}

func TestBareRepoIntegrityCheck_FixWithRigConfig(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	bareRepoDir := filepath.Join(rigDir, ".repo.git")

	// Create rig config with default branch and git URL
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigConfig := `{
		"type": "rig",
		"version": 1,
		"name": "testrig",
		"git_url": "https://github.com/example/test.git",
		"default_branch": "develop"
	}`
	if err := os.WriteFile(filepath.Join(rigDir, "config.json"), []byte(rigConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Create empty bare repo directory
	if err := os.MkdirAll(bareRepoDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewBareRepoIntegrityCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Run check to populate rigConfig
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify HEAD uses the configured default branch
	headContent, err := os.ReadFile(filepath.Join(bareRepoDir, "HEAD"))
	if err != nil {
		t.Fatalf("HEAD not created: %v", err)
	}
	if !strings.Contains(string(headContent), "refs/heads/develop") {
		t.Errorf("HEAD content = %q, expected ref to develop", string(headContent))
	}

	// Verify config includes remote URL
	configContent, err := os.ReadFile(filepath.Join(bareRepoDir, "config"))
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}
	if !strings.Contains(string(configContent), "https://github.com/example/test.git") {
		t.Errorf("config content = %q, expected to contain remote URL", string(configContent))
	}
}
