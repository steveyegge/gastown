package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestRigContractInitScaffoldsRepoFiles(t *testing.T) {
	townRoot, rigName := setupTestRigForSettings(t)
	repoRoot := filepath.Join(townRoot, rigName, "mayor", "rig")
	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n\ngo 1.23.0\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	resetRigContractFlags()
	rigContractGitHubActions = true
	if err := runRigContractInit(rigContractInitCmd, []string{rigName}); err != nil {
		t.Fatalf("runRigContractInit: %v", err)
	}

	settings, err := config.LoadRepoSettings(repoRoot)
	if err != nil {
		t.Fatalf("LoadRepoSettings: %v", err)
	}
	if settings == nil || settings.MergeQueue == nil {
		t.Fatal("expected merge_queue settings")
	}
	if settings.MergeQueue.GetVerificationMode() != config.VerificationModeStrict {
		t.Fatalf("verification_mode = %q, want %q", settings.MergeQueue.GetVerificationMode(), config.VerificationModeStrict)
	}
	if settings.RepoContract == nil {
		t.Fatal("expected repo_contract settings")
	}
	if settings.RepoContract.RepoType != "backend-api" {
		t.Fatalf("repo_type = %q, want backend-api", settings.RepoContract.RepoType)
	}
	if settings.RepoContract.VerifyCommand != "./scripts/ci/verify.sh" {
		t.Fatalf("verify_command = %q, want ./scripts/ci/verify.sh", settings.RepoContract.VerifyCommand)
	}

	verifyPath := filepath.Join(repoRoot, "scripts", "ci", "verify.sh")
	verifyContent, err := os.ReadFile(verifyPath)
	if err != nil {
		t.Fatalf("read verify.sh: %v", err)
	}
	verifyText := string(verifyContent)
	if !strings.Contains(verifyText, "go test ./...") {
		t.Fatalf("verify.sh missing go verifier steps:\n%s", verifyText)
	}

	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	workflowContent, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	if !strings.Contains(string(workflowContent), "./scripts/ci/verify.sh") {
		t.Fatalf("workflow does not call repo verifier:\n%s", string(workflowContent))
	}
}

func TestRigContractInitPreservesExistingFilesWithoutForce(t *testing.T) {
	townRoot, rigName := setupTestRigForSettings(t)
	repoRoot := filepath.Join(townRoot, rigName, "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(repoRoot, "scripts", "ci"), 0755); err != nil {
		t.Fatalf("mkdir scripts/ci: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".github", "workflows"), 0755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	customVerify := "#!/usr/bin/env bash\necho custom verify\n"
	customWorkflow := "name: Custom CI\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "scripts", "ci", "verify.sh"), []byte(customVerify), 0755); err != nil {
		t.Fatalf("write existing verify.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"), []byte(customWorkflow), 0644); err != nil {
		t.Fatalf("write existing ci.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "package.json"), []byte("{\"name\":\"test\",\"version\":\"1.0.0\"}\n"), 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	resetRigContractFlags()
	rigContractGitHubActions = true
	if err := runRigContractInit(rigContractInitCmd, []string{rigName}); err != nil {
		t.Fatalf("runRigContractInit: %v", err)
	}

	verifyContent, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "ci", "verify.sh"))
	if err != nil {
		t.Fatalf("read verify.sh: %v", err)
	}
	if string(verifyContent) != customVerify {
		t.Fatalf("verify.sh was overwritten without force:\n%s", string(verifyContent))
	}

	workflowContent, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read ci.yml: %v", err)
	}
	if string(workflowContent) != customWorkflow {
		t.Fatalf("ci.yml was overwritten without force:\n%s", string(workflowContent))
	}

	settings, err := config.LoadRepoSettings(repoRoot)
	if err != nil {
		t.Fatalf("LoadRepoSettings: %v", err)
	}
	if settings == nil || settings.RepoContract == nil {
		t.Fatal("expected repo settings to be scaffolded")
	}
}

func TestDetectRepoContractScaffoldPlan_Frontend(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "package.json"), []byte("{\"name\":\"frontend\",\"version\":\"1.0.0\"}\n"), 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "next.config.js"), []byte("module.exports = {}\n"), 0644); err != nil {
		t.Fatalf("write next.config.js: %v", err)
	}

	plan, err := detectRepoContractScaffoldPlan(repoRoot, "", "")
	if err != nil {
		t.Fatalf("detectRepoContractScaffoldPlan: %v", err)
	}
	if plan.RepoType != "frontend-app" {
		t.Fatalf("repo type = %q, want frontend-app", plan.RepoType)
	}
	if plan.EnforcementTier != config.RepoContractTierStrong {
		t.Fatalf("tier = %q, want %q", plan.EnforcementTier, config.RepoContractTierStrong)
	}
}

func resetRigContractFlags() {
	rigContractForce = false
	rigContractRepoType = ""
	rigContractTier = config.RepoContractTierStrong
	rigContractGitHubActions = true
}
