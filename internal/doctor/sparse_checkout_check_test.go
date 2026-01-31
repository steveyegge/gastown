package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/git"
)

func TestNewSparseCheckoutCheck(t *testing.T) {
	check := NewSparseCheckoutCheck()

	if check.Name() != "sparse-checkout" {
		t.Errorf("expected name 'sparse-checkout', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestSparseCheckoutCheck_NoRigSpecified(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: ""}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError when no rig specified, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "No rig specified") {
		t.Errorf("expected message about no rig, got %q", result.Message)
	}
}

func TestSparseCheckoutCheck_NoGitRepos(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	// No git repos found = StatusOK (nothing to check)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no git repos, got %v", result.Status)
	}
}

// initGitRepo creates a minimal git repo with an initial commit.
func initGitRepo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}

	// git init
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure user for commits
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config email failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config name failed: %v\n%s", err, out)
	}

	// Create initial commit
	readmePath := filepath.Join(path, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("git", "add", "README.md")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

func TestSparseCheckoutCheck_MayorRigMissingSparseCheckout(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create mayor/rig as a git repo without sparse checkout
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing sparse checkout, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "1 repo(s) missing") {
		t.Errorf("expected message about missing config, got %q", result.Message)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "mayor/rig") {
		t.Errorf("expected details to contain mayor/rig, got %v", result.Details)
	}
}

func TestSparseCheckoutCheck_MayorRigConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create mayor/rig as a git repo with sparse checkout configured
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)
	if err := git.ConfigureSparseCheckout(mayorRig); err != nil {
		t.Fatalf("ConfigureSparseCheckout failed: %v", err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when sparse checkout configured, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_CrewMissingSparseCheckout(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create crew/agent1 as a git repo without sparse checkout
	crewAgent := filepath.Join(rigDir, "crew", "agent1")
	initGitRepo(t, crewAgent)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing sparse checkout, got %v", result.Status)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "crew/agent1") {
		t.Errorf("expected details to contain crew/agent1, got %v", result.Details)
	}
}

func TestSparseCheckoutCheck_PolecatMissingSparseCheckout(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create polecats/pc1 as a git repo without sparse checkout
	polecat := filepath.Join(rigDir, "polecats", "pc1")
	initGitRepo(t, polecat)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing sparse checkout, got %v", result.Status)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "polecats/pc1") {
		t.Errorf("expected details to contain polecats/pc1, got %v", result.Details)
	}
}

func TestSparseCheckoutCheck_MultipleReposMissing(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create multiple git repos without sparse checkout
	initGitRepo(t, filepath.Join(rigDir, "mayor", "rig"))
	initGitRepo(t, filepath.Join(rigDir, "crew", "agent1"))
	initGitRepo(t, filepath.Join(rigDir, "polecats", "pc1"))

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing sparse checkout, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "3 repo(s) missing") {
		t.Errorf("expected message about 3 missing repos, got %q", result.Message)
	}
	if len(result.Details) != 3 {
		t.Errorf("expected 3 details, got %d", len(result.Details))
	}
}

func TestSparseCheckoutCheck_MixedConfigured(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create mayor/rig with sparse checkout configured
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)
	if err := git.ConfigureSparseCheckout(mayorRig); err != nil {
		t.Fatalf("ConfigureSparseCheckout failed: %v", err)
	}

	// Create crew/agent1 WITHOUT sparse checkout
	crewAgent := filepath.Join(rigDir, "crew", "agent1")
	initGitRepo(t, crewAgent)

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing sparse checkout, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "1 repo(s) missing") {
		t.Errorf("expected message about 1 missing repo, got %q", result.Message)
	}
	if len(result.Details) != 1 || !strings.Contains(filepath.ToSlash(result.Details[0]), "crew/agent1") {
		t.Errorf("expected details to contain only crew/agent1, got %v", result.Details)
	}
}

func TestSparseCheckoutCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repos without sparse checkout
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)
	crewAgent := filepath.Join(rigDir, "crew", "agent1")
	initGitRepo(t, crewAgent)

	check := NewSparseCheckoutCheck()
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

	// Verify sparse checkout is now configured
	if !git.IsSparseCheckoutConfigured(mayorRig) {
		t.Error("expected sparse checkout to be configured for mayor/rig")
	}
	if !git.IsSparseCheckoutConfigured(crewAgent) {
		t.Error("expected sparse checkout to be configured for crew/agent1")
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_FixNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo with sparse checkout already configured
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)
	if err := git.ConfigureSparseCheckout(mayorRig); err != nil {
		t.Fatalf("ConfigureSparseCheckout failed: %v", err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Run check to populate state
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK, got %v", result.Status)
	}

	// Fix should be a no-op (no affected repos)
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Still OK
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after no-op fix, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_NonGitDirSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create non-git directories (should be skipped)
	if err := os.MkdirAll(filepath.Join(rigDir, "mayor", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rigDir, "crew", "agent1"), 0755); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	// Non-git dirs are skipped, so StatusOK
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no git repos, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_VerifiesAllPatterns(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	// Configure sparse checkout using our function
	if err := git.ConfigureSparseCheckout(mayorRig); err != nil {
		t.Fatalf("ConfigureSparseCheckout failed: %v", err)
	}

	// Read the sparse-checkout file and verify all patterns are present
	sparseFile := filepath.Join(mayorRig, ".git", "info", "sparse-checkout")
	content, err := os.ReadFile(sparseFile)
	if err != nil {
		t.Fatalf("Failed to read sparse-checkout file: %v", err)
	}

	contentStr := string(content)

	// Verify all required patterns are present
	// Note: .mcp.json is NOT excluded so worktrees inherit MCP server config
	requiredPatterns := []string{
		"!/.claude/",        // Settings, rules, agents, commands
		"!/CLAUDE.md",       // Primary context file
		"!/CLAUDE.local.md", // Personal context file
	}

	for _, pattern := range requiredPatterns {
		if !strings.Contains(contentStr, pattern) {
			t.Errorf("sparse-checkout file missing pattern %q, got:\n%s", pattern, contentStr)
		}
	}
}

func TestSparseCheckoutCheck_LegacyPatternNotSufficient(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	// Manually configure sparse checkout with only legacy .claude/ pattern (missing CLAUDE.md)
	cmd := exec.Command("git", "config", "core.sparseCheckout", "true")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config failed: %v\n%s", err, out)
	}

	sparseFile := filepath.Join(mayorRig, ".git", "info", "sparse-checkout")
	if err := os.MkdirAll(filepath.Dir(sparseFile), 0755); err != nil {
		t.Fatal(err)
	}
	// Only include legacy pattern, missing CLAUDE.md
	if err := os.WriteFile(sparseFile, []byte("/*\n!.claude/\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	result := check.Run(ctx)

	// Should fail because CLAUDE.md pattern is missing
	if result.Status != StatusError {
		t.Errorf("expected StatusError for legacy-only pattern, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_FixUpgradesLegacyPatterns(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo with legacy sparse checkout (only .claude/)
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	cmd := exec.Command("git", "config", "core.sparseCheckout", "true")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config failed: %v\n%s", err, out)
	}

	sparseFile := filepath.Join(mayorRig, ".git", "info", "sparse-checkout")
	if err := os.MkdirAll(filepath.Dir(sparseFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sparseFile, []byte("/*\n!.claude/\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
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

	// Verify all patterns are now present
	content, err := os.ReadFile(sparseFile)
	if err != nil {
		t.Fatalf("Failed to read sparse-checkout file: %v", err)
	}

	contentStr := string(content)
	requiredPatterns := []string{"!/.claude/", "!/CLAUDE.md", "!/CLAUDE.local.md"}
	for _, pattern := range requiredPatterns {
		if !strings.Contains(contentStr, pattern) {
			t.Errorf("after fix, sparse-checkout file missing pattern %q", pattern)
		}
	}

	// Verify check now passes
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestSparseCheckoutCheck_FixFailsWithUntrackedCLAUDEMD(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo without sparse checkout
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	// Create untracked CLAUDE.md (not added to git)
	claudeFile := filepath.Join(mayorRig, "CLAUDE.md")
	if err := os.WriteFile(claudeFile, []byte("# Untracked context\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Fix should fail because CLAUDE.md is untracked and won't be removed
	err := check.Fix(ctx)
	if err == nil {
		t.Fatal("expected Fix to return error for untracked CLAUDE.md, but it succeeded")
	}

	// Verify error message is helpful
	if !strings.Contains(err.Error(), "CLAUDE.md") {
		t.Errorf("expected error to mention CLAUDE.md, got: %v", err)
	}
	if !strings.Contains(err.Error(), "untracked or modified") {
		t.Errorf("expected error to explain files are untracked/modified, got: %v", err)
	}
	if !strings.Contains(err.Error(), "manually remove") {
		t.Errorf("expected error to mention manual removal, got: %v", err)
	}
}

func TestSparseCheckoutCheck_FixFailsWithUntrackedClaudeDir(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo without sparse checkout
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	// Create untracked .claude/ directory (not added to git)
	claudeDir := filepath.Join(mayorRig, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Fix should fail because .claude/ is untracked and won't be removed
	err := check.Fix(ctx)
	if err == nil {
		t.Fatal("expected Fix to return error for untracked .claude/, but it succeeded")
	}

	// Verify error message mentions .claude
	if !strings.Contains(err.Error(), ".claude") {
		t.Errorf("expected error to mention .claude, got: %v", err)
	}
}

func TestSparseCheckoutCheck_FixFailsWithModifiedCLAUDEMD(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo without sparse checkout
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	// Add and commit CLAUDE.md to the repo
	claudeFile := filepath.Join(mayorRig, "CLAUDE.md")
	if err := os.WriteFile(claudeFile, []byte("# Original context\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "CLAUDE.md")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "Add CLAUDE.md")
	cmd.Dir = mayorRig
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Now modify CLAUDE.md without committing (making it "dirty")
	if err := os.WriteFile(claudeFile, []byte("# Modified context - local changes\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Fix should fail because CLAUDE.md is modified and git won't remove it
	err := check.Fix(ctx)
	if err == nil {
		t.Fatal("expected Fix to return error for modified CLAUDE.md, but it succeeded")
	}

	// Verify error message is helpful
	if !strings.Contains(err.Error(), "CLAUDE.md") {
		t.Errorf("expected error to mention CLAUDE.md, got: %v", err)
	}
}

func TestSparseCheckoutCheck_FixFailsWithMultipleProblems(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create git repo without sparse checkout
	mayorRig := filepath.Join(rigDir, "mayor", "rig")
	initGitRepo(t, mayorRig)

	// Create multiple untracked context files
	// Note: .mcp.json is NOT excluded (worktrees inherit MCP config), so we test with CLAUDE.md and CLAUDE.local.md
	if err := os.WriteFile(filepath.Join(mayorRig, "CLAUDE.md"), []byte("# Context\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorRig, "CLAUDE.local.md"), []byte("# Local context\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewSparseCheckoutCheck()
	ctx := &CheckContext{TownRoot: tmpDir, RigName: rigName}

	// Verify fix is needed
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Fix should fail and list multiple files
	err := check.Fix(ctx)
	if err == nil {
		t.Fatal("expected Fix to return error for multiple untracked files, but it succeeded")
	}

	// Verify error mentions both files
	errStr := err.Error()
	if !strings.Contains(errStr, "CLAUDE.md") {
		t.Errorf("expected error to mention CLAUDE.md, got: %v", err)
	}
	if !strings.Contains(errStr, "CLAUDE.local.md") {
		t.Errorf("expected error to mention CLAUDE.local.md, got: %v", err)
	}
}
