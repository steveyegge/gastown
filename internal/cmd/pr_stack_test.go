package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/refinery"
)

// TestDAGOrchestrator_RegisterBranch tests registering branches in the stack.
func TestDAGOrchestrator_RegisterBranch(t *testing.T) {
	// Create test repo with initial commit
	repo, err := refinery.NewTestRepo("stack-register")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Create initial commit
	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create a feature branch
	_, err = repo.CreateBranchWithCommit("feature-a", "main", "Add feature A", map[string]string{
		"feature_a.txt": "Feature A content",
	})
	if err != nil {
		t.Fatalf("failed to create feature branch: %v", err)
	}

	// Return to main
	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	// Create DAG orchestrator
	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	// Register the branch
	if err := orch.RegisterBranch("feature-a", "", "", "", "test-worker"); err != nil {
		t.Fatalf("failed to register branch: %v", err)
	}

	// Verify the branch is registered
	node, ok := orch.GetBranchStatus("feature-a")
	if !ok {
		t.Fatal("branch not found after registering")
	}
	if node.Status != refinery.BranchStatusClean {
		t.Errorf("expected status %q, got %q", refinery.BranchStatusClean, node.Status)
	}
	if node.Worker != "test-worker" {
		t.Errorf("expected worker %q, got %q", "test-worker", node.Worker)
	}

	// Verify DAG was saved
	dagFile := filepath.Join(repo.Path, ".gastown", "branch-dag.json")
	if _, err := os.Stat(dagFile); os.IsNotExist(err) {
		t.Error("DAG file was not saved")
	}
}

// TestDAGOrchestrator_RegisterStackedBranches tests registering a stack of branches.
func TestDAGOrchestrator_RegisterStackedBranches(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-stacked")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Create initial commit
	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create a feature branch
	if _, err := repo.CreateBranchWithCommit("feature-a", "main", "Add feature A", map[string]string{
		"feature_a.txt": "Feature A content",
	}); err != nil {
		t.Fatalf("failed to create feature-a: %v", err)
	}

	// Create a stacked branch on top of feature-a
	if _, err := repo.CreateBranchWithCommit("feature-b", "feature-a", "Add feature B", map[string]string{
		"feature_b.txt": "Feature B content",
	}); err != nil {
		t.Fatalf("failed to create feature-b: %v", err)
	}

	// Return to main
	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	// Create DAG orchestrator
	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	// Register feature-a first (depends on main/root)
	if err := orch.RegisterBranch("feature-a", "", "", "", "worker-a"); err != nil {
		t.Fatalf("failed to register feature-a: %v", err)
	}

	// Register feature-b (depends on feature-a)
	if err := orch.RegisterBranch("feature-b", "feature-a", "", "", "worker-b"); err != nil {
		t.Fatalf("failed to register feature-b: %v", err)
	}

	// Verify the stack
	nodeA, ok := orch.GetBranchStatus("feature-a")
	if !ok {
		t.Fatal("feature-a not found")
	}
	if nodeA.DependsOn != "" {
		t.Errorf("feature-a should have empty DependsOn (root), got %q", nodeA.DependsOn)
	}

	nodeB, ok := orch.GetBranchStatus("feature-b")
	if !ok {
		t.Fatal("feature-b not found")
	}
	if nodeB.DependsOn != "feature-a" {
		t.Errorf("feature-b should depend on feature-a, got %q", nodeB.DependsOn)
	}

	// Verify topological order
	order := orch.GetRebaseOrder()
	// Since nothing needs rebasing, order should be empty
	if len(order) != 0 {
		t.Errorf("expected empty rebase order for clean branches, got %v", order)
	}

	// Verify stats
	stats := orch.GetDAGStats()
	if stats["total"] != 2 {
		t.Errorf("expected 2 total branches, got %d", stats["total"])
	}
}

// TestDAGOrchestrator_UnregisterBranch tests removing a branch from the stack.
func TestDAGOrchestrator_UnregisterBranch(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-unregister")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("feature-a", "main", "Add feature A", map[string]string{
		"feature_a.txt": "Feature A content",
	}); err != nil {
		t.Fatalf("failed to create feature-a: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	// Register then unregister
	if err := orch.RegisterBranch("feature-a", "", "", "", ""); err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	if err := orch.UnregisterBranch("feature-a"); err != nil {
		t.Fatalf("failed to unregister: %v", err)
	}

	// Verify it's gone
	if _, ok := orch.GetBranchStatus("feature-a"); ok {
		t.Error("branch should not exist after unregister")
	}
}

// TestDAGOrchestrator_UnregisterWithDependents tests that we can't remove a branch with dependents.
func TestDAGOrchestrator_UnregisterWithDependents(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-unregister-deps")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("feature-a", "main", "Add feature A", map[string]string{
		"feature_a.txt": "Feature A content",
	}); err != nil {
		t.Fatalf("failed to create feature-a: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("feature-b", "feature-a", "Add feature B", map[string]string{
		"feature_b.txt": "Feature B content",
	}); err != nil {
		t.Fatalf("failed to create feature-b: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	if err := orch.RegisterBranch("feature-a", "", "", "", ""); err != nil {
		t.Fatalf("failed to register feature-a: %v", err)
	}
	if err := orch.RegisterBranch("feature-b", "feature-a", "", "", ""); err != nil {
		t.Fatalf("failed to register feature-b: %v", err)
	}

	// Try to unregister feature-a (has dependents)
	err = orch.UnregisterBranch("feature-a")
	if err == nil {
		t.Error("expected error when unregistering branch with dependents")
	}
}

// TestDAGOrchestrator_LoadAndSave tests persistence of DAG state.
func TestDAGOrchestrator_LoadAndSave(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-persist")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("feature-a", "main", "Add feature A", map[string]string{
		"feature_a.txt": "Feature A content",
	}); err != nil {
		t.Fatalf("failed to create feature-a: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	// Create and save
	orch1 := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)
	if err := orch1.RegisterBranch("feature-a", "", "", "", "worker-1"); err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	// Create new orchestrator and load
	orch2 := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)
	if err := orch2.LoadDAG(); err != nil {
		t.Fatalf("failed to load DAG: %v", err)
	}

	// Verify state was preserved
	node, ok := orch2.GetBranchStatus("feature-a")
	if !ok {
		t.Fatal("branch not found after reload")
	}
	if node.Worker != "worker-1" {
		t.Errorf("worker not preserved, got %q", node.Worker)
	}
}

// TestDAGOrchestrator_GetRebaseOrder tests topological ordering for rebases.
func TestDAGOrchestrator_GetRebaseOrder(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-order")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create stack: main <- A <- B <- C
	if _, err := repo.CreateBranchWithCommit("A", "main", "Add A", map[string]string{"a.txt": "A"}); err != nil {
		t.Fatalf("failed to create A: %v", err)
	}
	if _, err := repo.CreateBranchWithCommit("B", "A", "Add B", map[string]string{"b.txt": "B"}); err != nil {
		t.Fatalf("failed to create B: %v", err)
	}
	if _, err := repo.CreateBranchWithCommit("C", "B", "Add C", map[string]string{"c.txt": "C"}); err != nil {
		t.Fatalf("failed to create C: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	// Register branches
	if err := orch.RegisterBranch("A", "", "", "", ""); err != nil {
		t.Fatalf("failed to register A: %v", err)
	}
	if err := orch.RegisterBranch("B", "A", "", "", ""); err != nil {
		t.Fatalf("failed to register B: %v", err)
	}
	if err := orch.RegisterBranch("C", "B", "", "", ""); err != nil {
		t.Fatalf("failed to register C: %v", err)
	}

	// No rebases needed yet
	order := orch.GetRebaseOrder()
	if len(order) != 0 {
		t.Errorf("expected empty rebase order, got %v", order)
	}

	// Verify stats
	stats := orch.GetDAGStats()
	if stats["total"] != 3 {
		t.Errorf("expected 3 branches, got %d", stats["total"])
	}
	if stats["clean"] != 3 {
		t.Errorf("expected 3 clean branches, got %d", stats["clean"])
	}
}

// TestDAGOrchestrator_HandleBranchMerged tests the merge flow.
func TestDAGOrchestrator_HandleBranchMerged(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-merge")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create stack: main <- A <- B
	if _, err := repo.CreateBranchWithCommit("A", "main", "Add A", map[string]string{"a.txt": "A"}); err != nil {
		t.Fatalf("failed to create A: %v", err)
	}
	if _, err := repo.CreateBranchWithCommit("B", "A", "Add B", map[string]string{"b.txt": "B"}); err != nil {
		t.Fatalf("failed to create B: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	signaler := &refinery.MockSignaler{}
	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, signaler)

	if err := orch.RegisterBranch("A", "", "", "", "worker-a"); err != nil {
		t.Fatalf("failed to register A: %v", err)
	}
	if err := orch.RegisterBranch("B", "A", "", "", "worker-b"); err != nil {
		t.Fatalf("failed to register B: %v", err)
	}

	// A gets merged
	if err := orch.HandleBranchMerged("A"); err != nil {
		t.Fatalf("HandleBranchMerged failed: %v", err)
	}

	// Verify A is marked merged
	nodeA, _ := orch.GetBranchStatus("A")
	if nodeA.Status != refinery.BranchStatusMerged {
		t.Errorf("A should be marked merged, got %q", nodeA.Status)
	}

	// Verify B was retargeted to root (empty DependsOn)
	nodeB, _ := orch.GetBranchStatus("B")
	if nodeB.DependsOn != "" {
		t.Errorf("B should now depend on root, got %q", nodeB.DependsOn)
	}
	if nodeB.Status != refinery.BranchStatusNeedsRebase {
		t.Errorf("B should need rebase, got %q", nodeB.Status)
	}

	// Verify signals were sent
	if len(signaler.MergedCalls) != 1 {
		t.Errorf("expected 1 merged signal, got %d", len(signaler.MergedCalls))
	}
	if signaler.MergedCalls[0].Worker != "worker-a" {
		t.Errorf("expected worker-a merged signal, got %q", signaler.MergedCalls[0].Worker)
	}

	if len(signaler.RetargetedCalls) != 1 {
		t.Errorf("expected 1 retargeted signal, got %d", len(signaler.RetargetedCalls))
	}
	if signaler.RetargetedCalls[0].Worker != "worker-b" {
		t.Errorf("expected worker-b retargeted signal, got %q", signaler.RetargetedCalls[0].Worker)
	}
}

// TestDAGOrchestrator_ValidateDAG tests DAG validation.
func TestDAGOrchestrator_ValidateDAG(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-validate")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("A", "main", "Add A", map[string]string{"a.txt": "A"}); err != nil {
		t.Fatalf("failed to create A: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	if err := orch.RegisterBranch("A", "", "", "", ""); err != nil {
		t.Fatalf("failed to register A: %v", err)
	}

	// Validate should pass
	errs := orch.ValidateDAG()
	if len(errs) > 0 {
		t.Errorf("expected no validation errors, got %v", errs)
	}
}

// TestDAGOrchestrator_GetStats tests statistics reporting.
func TestDAGOrchestrator_GetStats(t *testing.T) {
	repo, err := refinery.NewTestRepo("stack-stats")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("A", "main", "Add A", map[string]string{"a.txt": "A"}); err != nil {
		t.Fatalf("failed to create A: %v", err)
	}
	if _, err := repo.CreateBranchWithCommit("B", "A", "Add B", map[string]string{"b.txt": "B"}); err != nil {
		t.Fatalf("failed to create B: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	if err := orch.RegisterBranch("A", "", "", "", ""); err != nil {
		t.Fatalf("failed to register A: %v", err)
	}
	if err := orch.RegisterBranch("B", "A", "", "", ""); err != nil {
		t.Fatalf("failed to register B: %v", err)
	}

	stats := orch.GetDAGStats()

	if stats["total"] != 2 {
		t.Errorf("expected total=2, got %d", stats["total"])
	}
	if stats["clean"] != 2 {
		t.Errorf("expected clean=2, got %d", stats["clean"])
	}
	if stats["needs_rebase"] != 0 {
		t.Errorf("expected needs_rebase=0, got %d", stats["needs_rebase"])
	}
}

// TestGetCurrentBranchName tests getting current git branch.
func TestGetCurrentBranchName(t *testing.T) {
	repo, err := refinery.NewTestRepo("current-branch")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Change to repo dir for the test
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	defer os.Chdir(oldDir)

	if err := os.Chdir(repo.Path); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}

	// Test on main branch
	branch, err := getCurrentBranchName()
	if err != nil {
		t.Fatalf("getCurrentBranchName failed: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected 'main', got %q", branch)
	}

	// Create and checkout a feature branch
	if err := repo.Git.CreateBranch("feature-test", "main"); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	branch, err = getCurrentBranchName()
	if err != nil {
		t.Fatalf("getCurrentBranchName failed: %v", err)
	}
	if branch != "feature-test" {
		t.Errorf("expected 'feature-test', got %q", branch)
	}
}

// TestForcePushBranch tests the force push helper (with a local remote).
func TestForcePushBranch(t *testing.T) {
	// Create two repos: a "remote" (bare) and a "local"
	remote, err := refinery.NewBareTestRepo("push-remote")
	if err != nil {
		t.Fatalf("failed to create remote repo: %v", err)
	}
	defer remote.Cleanup()

	local, err := refinery.NewTestRepo("push-local")
	if err != nil {
		t.Fatalf("failed to create local repo: %v", err)
	}
	defer local.Cleanup()

	// Add remote to local
	if err := local.AddRemote("origin", remote.Path); err != nil {
		t.Fatalf("failed to add remote: %v", err)
	}

	// Create initial commit and push
	if _, err := local.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	if err := local.Git.Push("origin", "main", false); err != nil {
		t.Fatalf("failed to push main: %v", err)
	}

	// Create a feature branch
	if _, err := local.CreateBranchWithCommit("feature", "main", "Feature commit", map[string]string{
		"feature.txt": "feature content",
	}); err != nil {
		t.Fatalf("failed to create feature branch: %v", err)
	}

	// Push feature branch
	if err := local.Git.Push("origin", "feature", false); err != nil {
		t.Fatalf("failed to push feature: %v", err)
	}

	// Add another commit
	if _, err := local.Git.AddCommit("Another commit", map[string]string{
		"another.txt": "another content",
	}); err != nil {
		t.Fatalf("failed to add commit: %v", err)
	}

	// Force push should work
	err = forcePushBranch(local.Path, "origin", "feature")
	if err != nil {
		t.Fatalf("forcePushBranch failed: %v", err)
	}
}

// TestDAG_DependencyFiltering tests that branches with unmerged dependencies are blocked.
// This simulates the filtering logic in Engineer.ListReadyMRs().
func TestDAG_DependencyFiltering(t *testing.T) {
	repo, err := refinery.NewTestRepo("dag-filtering")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create stack: main <- feature-a <- feature-b
	if _, err := repo.CreateBranchWithCommit("feature-a", "main", "Feature A", map[string]string{
		"a.txt": "A",
	}); err != nil {
		t.Fatalf("failed to create feature-a: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("feature-b", "feature-a", "Feature B", map[string]string{
		"b.txt": "B",
	}); err != nil {
		t.Fatalf("failed to create feature-b: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	// Register both branches
	if err := orch.RegisterBranch("feature-a", "", "", "", ""); err != nil {
		t.Fatalf("failed to register feature-a: %v", err)
	}
	if err := orch.RegisterBranch("feature-b", "feature-a", "", "", ""); err != nil {
		t.Fatalf("failed to register feature-b: %v", err)
	}

	// Simulate dependency check (as done in ListReadyMRs):
	// feature-b depends on feature-a, which is not merged

	// Check feature-a status - should be clean (no dependency)
	nodeA, ok := orch.GetBranchStatus("feature-a")
	if !ok {
		t.Fatal("feature-a not found")
	}
	if nodeA.Status != refinery.BranchStatusClean {
		t.Errorf("feature-a should be clean, got %q", nodeA.Status)
	}

	// feature-a has no dependency blocking it
	isABlocked := nodeA.DependsOn != "" && nodeA.Status != refinery.BranchStatusMerged
	if isABlocked {
		t.Error("feature-a should NOT be blocked")
	}

	// Check feature-b - depends on feature-a which is not merged
	nodeB, ok := orch.GetBranchStatus("feature-b")
	if !ok {
		t.Fatal("feature-b not found")
	}

	// Simulate the filtering logic from ListReadyMRs
	dependsOn := nodeB.DependsOn
	isBBlocked := false
	if dependsOn != "" && dependsOn != "main" {
		if depNode, exists := orch.GetBranchStatus(dependsOn); exists {
			if depNode.Status != refinery.BranchStatusMerged {
				isBBlocked = true
			}
		}
	}

	if !isBBlocked {
		t.Error("feature-b SHOULD be blocked (feature-a not merged)")
	}

	// Now mark feature-a as merged
	if err := orch.HandleBranchMerged("feature-a"); err != nil {
		t.Fatalf("HandleBranchMerged failed: %v", err)
	}

	// Re-check: feature-b should no longer be blocked
	nodeA, _ = orch.GetBranchStatus("feature-a")
	if nodeA.Status != refinery.BranchStatusMerged {
		t.Errorf("feature-a should be merged, got %q", nodeA.Status)
	}

	// feature-b was retargeted to root (empty DependsOn)
	nodeB, _ = orch.GetBranchStatus("feature-b")
	if nodeB.DependsOn != "" {
		t.Errorf("feature-b should be retargeted to root, got %q", nodeB.DependsOn)
	}

	// Now feature-b is no longer blocked
	isBBlocked = false
	dependsOn = nodeB.DependsOn
	if dependsOn != "" && dependsOn != "main" {
		if depNode, exists := orch.GetBranchStatus(dependsOn); exists {
			if depNode.Status != refinery.BranchStatusMerged {
				isBBlocked = true
			}
		}
	}

	if isBBlocked {
		t.Error("feature-b should NOT be blocked after feature-a merged")
	}
}

// TestDAG_MergeNotificationCascade tests that merging a branch notifies dependents.
func TestDAG_MergeNotificationCascade(t *testing.T) {
	repo, err := refinery.NewTestRepo("dag-cascade")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create deep stack: main <- A <- B <- C <- D
	branches := []string{"A", "B", "C", "D"}
	prevBranch := "main"
	for _, br := range branches {
		if _, err := repo.CreateBranchWithCommit(br, prevBranch, "Add "+br, map[string]string{
			br + ".txt": br,
		}); err != nil {
			t.Fatalf("failed to create %s: %v", br, err)
		}
		prevBranch = br
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	signaler := &refinery.MockSignaler{}
	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, signaler)

	// Register all branches
	prevBranch = ""
	for _, br := range branches {
		if err := orch.RegisterBranch(br, prevBranch, "", "", "worker-"+br); err != nil {
			t.Fatalf("failed to register %s: %v", br, err)
		}
		prevBranch = br
	}

	// Merge A
	if err := orch.HandleBranchMerged("A"); err != nil {
		t.Fatalf("HandleBranchMerged(A) failed: %v", err)
	}

	// B should be retargeted to root and need rebase
	nodeB, _ := orch.GetBranchStatus("B")
	if nodeB.DependsOn != "" {
		t.Errorf("B should be retargeted to root, got %q", nodeB.DependsOn)
	}
	if nodeB.Status != refinery.BranchStatusNeedsRebase {
		t.Errorf("B should need rebase, got %q", nodeB.Status)
	}

	// C and D should still depend on B and C respectively (unchanged)
	nodeC, _ := orch.GetBranchStatus("C")
	if nodeC.DependsOn != "B" {
		t.Errorf("C should still depend on B, got %q", nodeC.DependsOn)
	}

	nodeD, _ := orch.GetBranchStatus("D")
	if nodeD.DependsOn != "C" {
		t.Errorf("D should still depend on C, got %q", nodeD.DependsOn)
	}

	// Verify signals were sent
	if len(signaler.MergedCalls) != 1 {
		t.Errorf("expected 1 merged signal, got %d", len(signaler.MergedCalls))
	}
	if len(signaler.RetargetedCalls) != 1 {
		t.Errorf("expected 1 retargeted signal, got %d", len(signaler.RetargetedCalls))
	}
	if len(signaler.RebaseNeededCalls) != 1 {
		t.Errorf("expected 1 rebase-needed signal, got %d", len(signaler.RebaseNeededCalls))
	}
}

// TestDAG_AutoRegistrationLogic tests the branch registration logic used in mq_submit.
func TestDAG_AutoRegistrationLogic(t *testing.T) {
	repo, err := refinery.NewTestRepo("dag-autoregister")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	if _, err := repo.CreateInitialCommit(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	// Create branches
	if _, err := repo.CreateBranchWithCommit("feature-main", "main", "Feature targeting main", map[string]string{
		"main.txt": "main",
	}); err != nil {
		t.Fatalf("failed to create feature-main: %v", err)
	}

	// Create an integration branch
	if _, err := repo.CreateBranchWithCommit("integration/epic-1", "main", "Epic integration", map[string]string{
		"epic.txt": "epic",
	}); err != nil {
		t.Fatalf("failed to create integration branch: %v", err)
	}

	if _, err := repo.CreateBranchWithCommit("feature-epic", "integration/epic-1", "Feature for epic", map[string]string{
		"epic-feature.txt": "feature",
	}); err != nil {
		t.Fatalf("failed to create feature-epic: %v", err)
	}

	if err := repo.Git.CheckoutBranch("main"); err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	orch := refinery.NewDAGOrchestrator(repo.Path, "main", "upstream", "origin", repo.Git, nil)

	// Simulate mq_submit auto-registration logic:
	// If target == defaultBranch, dependsOn = ""
	// If target != defaultBranch, dependsOn = target

	defaultBranch := "main"

	// Case 1: Branch targeting main - no dependency
	target1 := "main"
	dependsOn1 := ""
	if target1 != defaultBranch {
		dependsOn1 = target1
	}
	if err := orch.RegisterBranch("feature-main", dependsOn1, "", "issue-1", "worker-1"); err != nil {
		t.Fatalf("failed to register feature-main: %v", err)
	}

	node1, _ := orch.GetBranchStatus("feature-main")
	if node1.DependsOn != "" {
		t.Errorf("feature-main should have no dependency, got %q", node1.DependsOn)
	}

	// Case 2: Branch targeting integration branch - depends on integration branch
	target2 := "integration/epic-1"
	dependsOn2 := ""
	if target2 != defaultBranch {
		dependsOn2 = target2
	}

	// First register the integration branch
	if err := orch.RegisterBranch("integration/epic-1", "", "", "epic-1", ""); err != nil {
		t.Fatalf("failed to register integration branch: %v", err)
	}

	if err := orch.RegisterBranch("feature-epic", dependsOn2, "", "issue-2", "worker-2"); err != nil {
		t.Fatalf("failed to register feature-epic: %v", err)
	}

	node2, _ := orch.GetBranchStatus("feature-epic")
	if node2.DependsOn != "integration/epic-1" {
		t.Errorf("feature-epic should depend on integration/epic-1, got %q", node2.DependsOn)
	}

	// Verify the DAG stats
	stats := orch.GetDAGStats()
	if stats["total"] != 3 {
		t.Errorf("expected 3 branches in DAG, got %d", stats["total"])
	}
}

// TestDAG_IntegrationWithMRFields tests that DependsOn is correctly read from MR fields.
func TestDAG_IntegrationWithMRFields(t *testing.T) {
	// This tests the MRFields parsing that ListReadyMRs uses
	tests := []struct {
		name        string
		description string
		wantDep     string
	}{
		{
			name: "no dependency",
			description: `branch: feature-x
target: main
source_issue: gt-123`,
			wantDep: "",
		},
		{
			name: "depends on another branch",
			description: `branch: feature-y
target: main
depends_on: feature-x
source_issue: gt-124`,
			wantDep: "feature-x",
		},
		{
			name: "depends on integration branch",
			description: `branch: feature-z
target: integration/epic-1
depends_on: integration/epic-1
source_issue: gt-125`,
			wantDep: "integration/epic-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate parsing MR fields like ListReadyMRs does
			issue := &beads.Issue{
				ID:          "mr-" + tt.name,
				Description: tt.description,
			}

			fields := beads.ParseMRFields(issue)
			if fields == nil {
				t.Fatal("ParseMRFields returned nil")
			}

			if fields.DependsOn != tt.wantDep {
				t.Errorf("DependsOn = %q, want %q", fields.DependsOn, tt.wantDep)
			}
		})
	}
}
