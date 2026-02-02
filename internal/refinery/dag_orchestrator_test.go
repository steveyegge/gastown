package refinery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDAGOrchestrator_RegisterBranch(t *testing.T) {
	repo, err := NewTestRepo("orchestrator-register")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Create a feature branch
	_, _ = repo.CreateBranchWithCommit("feature-a", mainBranch, "Feature A", map[string]string{
		"a.txt": "content",
	})

	// Create orchestrator
	signaler := &MockSignaler{}
	orch := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, signaler)

	// Register the branch
	err = orch.RegisterBranch("feature-a", "", "epic-1", "task-1", "polecat-alpha")
	if err != nil {
		t.Fatalf("failed to register branch: %v", err)
	}

	// Verify it was added
	node, ok := orch.GetBranchStatus("feature-a")
	if !ok {
		t.Fatal("branch not found after registration")
	}
	if node.EpicID != "epic-1" {
		t.Errorf("expected epicID 'epic-1', got %q", node.EpicID)
	}
	if node.Worker != "polecat-alpha" {
		t.Errorf("expected worker 'polecat-alpha', got %q", node.Worker)
	}
}

func TestDAGOrchestrator_SaveLoadDAG(t *testing.T) {
	repo, err := NewTestRepo("orchestrator-save")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	_, _ = repo.CreateBranchWithCommit("feature-a", mainBranch, "Feature A", map[string]string{
		"a.txt": "content",
	})

	// Create and save
	orch1 := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, nil)
	_ = orch1.RegisterBranch("feature-a", "", "epic-1", "task-1", "worker-1")

	// Create new orchestrator and load
	orch2 := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, nil)
	err = orch2.LoadDAG()
	if err != nil {
		t.Fatalf("failed to load DAG: %v", err)
	}

	// Verify data persisted
	node, ok := orch2.GetBranchStatus("feature-a")
	if !ok {
		t.Fatal("branch not found after reload")
	}
	if node.EpicID != "epic-1" {
		t.Errorf("expected epicID 'epic-1' after reload, got %q", node.EpicID)
	}
}

func TestDAGOrchestrator_CheckBranchUpdates(t *testing.T) {
	repo, err := NewTestRepo("orchestrator-check")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Create stack: main <- A <- B
	_, _ = repo.CreateBranchWithCommit("branch-A", mainBranch, "Feature A", map[string]string{
		"a.txt": "content A",
	})
	_, _ = repo.CreateBranchWithCommit("branch-B", "branch-A", "Feature B", map[string]string{
		"b.txt": "content B",
	})

	signaler := &MockSignaler{}
	orch := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, signaler)

	_ = orch.RegisterBranch("branch-A", "", "", "", "worker-a")
	_ = orch.RegisterBranch("branch-B", "branch-A", "", "", "worker-b")

	// Update A
	_ = repo.Git.CheckoutBranch("branch-A")
	newSHA, _ := repo.Git.AddCommit("Update A", map[string]string{
		"a2.txt": "more content",
	})

	// Mark A as updated in DAG (simulate detecting the update)
	orch.dag.MarkBranchUpdated("branch-A", newSHA)

	// Check updates
	results, err := orch.CheckBranchUpdates()
	if err != nil {
		t.Fatalf("CheckBranchUpdates failed: %v", err)
	}

	// B should need rebase
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Branch != "branch-B" {
		t.Errorf("expected branch-B, got %s", results[0].Branch)
	}
	if !results[0].NeedsRebase {
		t.Error("expected NeedsRebase to be true")
	}
}

func TestDAGOrchestrator_PerformRebase(t *testing.T) {
	repo, err := NewTestRepo("orchestrator-rebase")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Create stack: main <- A <- B
	_, _ = repo.CreateBranchWithCommit("branch-A", mainBranch, "Feature A", map[string]string{
		"a.txt": "content A",
	})
	_, _ = repo.CreateBranchWithCommit("branch-B", "branch-A", "Feature B", map[string]string{
		"b.txt": "content B",
	})

	signaler := &MockSignaler{}
	orch := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, signaler)

	_ = orch.RegisterBranch("branch-A", "", "", "", "worker-a")
	_ = orch.RegisterBranch("branch-B", "branch-A", "", "", "worker-b")

	// Update A
	_ = repo.Git.CheckoutBranch("branch-A")
	_, _ = repo.Git.AddCommit("Update A", map[string]string{
		"a2.txt": "more content",
	})

	// Mark B as needing rebase
	_ = orch.dag.UpdateBranch("branch-B", func(n *BranchNode) {
		n.Status = BranchStatusNeedsRebase
	})

	// Perform rebase
	err = orch.PerformRebase("branch-B")
	if err != nil {
		t.Fatalf("PerformRebase failed: %v", err)
	}

	// Verify B is now clean
	node, _ := orch.GetBranchStatus("branch-B")
	if node.Status != BranchStatusClean {
		t.Errorf("expected status clean, got %q", node.Status)
	}

	// Verify git state - B should have A's new file
	_ = repo.Git.CheckoutBranch("branch-B")
	if _, err := os.Stat(filepath.Join(repo.Path, "a2.txt")); os.IsNotExist(err) {
		t.Error("branch-B should have a2.txt after rebase")
	}
}

func TestDAGOrchestrator_HandleBranchMerged(t *testing.T) {
	repo, err := NewTestRepo("orchestrator-merged")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Create stack: main <- A <- B
	_, _ = repo.CreateBranchWithCommit("branch-A", mainBranch, "Feature A", map[string]string{
		"a.txt": "content A",
	})
	_, _ = repo.CreateBranchWithCommit("branch-B", "branch-A", "Feature B", map[string]string{
		"b.txt": "content B",
	})

	signaler := &MockSignaler{}
	orch := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, signaler)

	_ = orch.RegisterBranch("branch-A", "", "", "", "worker-a")
	_ = orch.RegisterBranch("branch-B", "branch-A", "", "", "worker-b")

	// Merge A to main
	_ = repo.Git.Merge("branch-A", mainBranch, true)

	// Handle merge in orchestrator
	err = orch.HandleBranchMerged("branch-A")
	if err != nil {
		t.Fatalf("HandleBranchMerged failed: %v", err)
	}

	// Verify A is marked merged
	nodeA, _ := orch.GetBranchStatus("branch-A")
	if nodeA.Status != BranchStatusMerged {
		t.Errorf("expected A status merged, got %q", nodeA.Status)
	}

	// Verify B was retargeted
	nodeB, _ := orch.GetBranchStatus("branch-B")
	if nodeB.DependsOn != "" {
		t.Errorf("expected B to depend on root, got %q", nodeB.DependsOn)
	}
	if nodeB.Status != BranchStatusNeedsRebase {
		t.Errorf("expected B to need rebase, got %q", nodeB.Status)
	}

	// Verify signals
	if len(signaler.MergedCalls) != 1 {
		t.Errorf("expected 1 merged signal, got %d", len(signaler.MergedCalls))
	}
	if len(signaler.RetargetedCalls) != 1 {
		t.Errorf("expected 1 retargeted signal, got %d", len(signaler.RetargetedCalls))
	}
	if len(signaler.RebaseNeededCalls) != 1 {
		t.Errorf("expected 1 rebase needed signal, got %d", len(signaler.RebaseNeededCalls))
	}
}

func TestDAGOrchestrator_ConflictDetection(t *testing.T) {
	repo, err := NewTestRepo("orchestrator-conflict")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Add file.txt on main
	_, _ = repo.Git.AddCommit("Add file.txt", map[string]string{
		"file.txt": "original\nline 2\n",
	})

	// Create branch A - modifies file.txt
	_, _ = repo.CreateBranchWithCommit("branch-A", mainBranch, "Modify file", map[string]string{
		"file.txt": "modified by A\nline 2\n",
	})

	signaler := &MockSignaler{}
	orch := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, signaler)
	_ = orch.RegisterBranch("branch-A", "", "", "", "worker-a")

	// Make conflicting change on main
	_ = repo.Git.CheckoutBranch(mainBranch)
	_, _ = repo.Git.AddCommit("Conflicting change", map[string]string{
		"file.txt": "modified by main\nline 2\n",
	})

	// Mark A as needing rebase
	_ = orch.dag.UpdateBranch("branch-A", func(n *BranchNode) {
		n.Status = BranchStatusNeedsRebase
	})

	// Check updates - should detect conflict
	results, err := orch.CheckBranchUpdates()
	if err != nil {
		t.Fatalf("CheckBranchUpdates failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !results[0].HasConflict {
		t.Error("expected conflict to be detected")
	}

	if len(results[0].ConflictFiles) == 0 {
		t.Error("expected conflict files to be listed")
	}

	// Verify DAG status updated
	node, _ := orch.GetBranchStatus("branch-A")
	if node.Status != BranchStatusConflict {
		t.Errorf("expected status conflict, got %q", node.Status)
	}
}

func TestDAGOrchestrator_FullWorkflow(t *testing.T) {
	// This test simulates a complete epic workflow:
	// 1. Create stack of branches (epic subtasks)
	// 2. Upstream advances
	// 3. Cascade rebases through the stack
	// 4. Merge branches one by one

	repo, err := NewTestRepo("full-workflow")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Create upstream bare repo
	upstream, err := NewBareTestRepo("upstream")
	if err != nil {
		t.Fatalf("failed to create upstream: %v", err)
	}
	defer upstream.Cleanup()

	// Setup
	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}
	_ = repo.AddRemote("upstream", upstream.Path)
	_ = repo.Git.Push("upstream", mainBranch, false)

	signaler := &MockSignaler{}
	orch := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, signaler)

	// Step 1: Create epic stack
	// main <- api <- tests <- docs
	t.Log("Step 1: Creating epic stack")

	_, _ = repo.CreateBranchWithCommit("epic-1/api", mainBranch, "API implementation", map[string]string{
		"api.go": "package api\n\nfunc New() {}\n",
	})
	_ = orch.RegisterBranch("epic-1/api", "", "epic-1", "subtask-api", "polecat-a")

	_, _ = repo.CreateBranchWithCommit("epic-1/tests", "epic-1/api", "Tests", map[string]string{
		"api_test.go": "package api\n\nfunc TestNew(t *testing.T) {}\n",
	})
	_ = orch.RegisterBranch("epic-1/tests", "epic-1/api", "epic-1", "subtask-tests", "polecat-b")

	_, _ = repo.CreateBranchWithCommit("epic-1/docs", "epic-1/tests", "Docs", map[string]string{
		"README.md": "# API\n\nDocumentation\n",
	})
	_ = orch.RegisterBranch("epic-1/docs", "epic-1/tests", "epic-1", "subtask-docs", "polecat-c")

	stats := orch.GetDAGStats()
	if stats["total"] != 3 {
		t.Errorf("expected 3 branches, got %d", stats["total"])
	}

	// Step 2: Simulate upstream advancing
	t.Log("Step 2: Simulating upstream advance")

	_ = repo.Git.CheckoutBranch(mainBranch)
	_, _ = repo.Git.AddCommit("Upstream hotfix", map[string]string{
		"hotfix.go": "package hotfix\n",
	})
	_ = repo.Git.Push("upstream", mainBranch, false)

	// Check for upstream updates
	results, err := orch.CheckUpstreamUpdates()
	if err != nil {
		t.Fatalf("CheckUpstreamUpdates failed: %v", err)
	}

	t.Logf("Upstream check results: %d branches affected", len(results))
	if len(results) == 0 {
		t.Log("No branches directly affected (expected for non-root branches)")
	}

	// Manually mark root branch as needing update (since we don't have real upstream fetch)
	_ = orch.dag.UpdateBranch("epic-1/api", func(n *BranchNode) {
		n.Status = BranchStatusNeedsRebase
	})

	// Step 3: Cascade rebases
	t.Log("Step 3: Performing cascade rebases")

	// Rebase iteratively until no more branches need rebasing
	// This handles the cascade: api -> tests -> docs
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		rebaseOrder := orch.GetRebaseOrder()
		if len(rebaseOrder) == 0 {
			break
		}
		t.Logf("Rebase iteration %d, order: %v", i+1, rebaseOrder)

		for _, branch := range rebaseOrder {
			err := orch.PerformRebase(branch)
			if err != nil {
				t.Errorf("Failed to rebase %s: %v", branch, err)
			} else {
				t.Logf("Rebased %s successfully", branch)
			}
		}
	}

	// Verify all clean
	stats = orch.GetDAGStats()
	if stats["needs_rebase"] != 0 {
		t.Errorf("expected 0 branches needing rebase, got %d", stats["needs_rebase"])
	}

	// Step 4: Merge branches one by one
	t.Log("Step 4: Merging branches")

	// Merge api to main
	_ = repo.Git.Merge("epic-1/api", mainBranch, true)
	err = orch.HandleBranchMerged("epic-1/api")
	if err != nil {
		t.Fatalf("HandleBranchMerged api failed: %v", err)
	}

	// Tests should now target main
	testsNode, _ := orch.GetBranchStatus("epic-1/tests")
	if testsNode.DependsOn != "" {
		t.Errorf("tests should target root after api merged, got %q", testsNode.DependsOn)
	}

	// Rebase tests onto new main
	if testsNode.Status == BranchStatusNeedsRebase {
		err = orch.PerformRebase("epic-1/tests")
		if err != nil {
			t.Errorf("Failed to rebase tests: %v", err)
		}
	}

	// Merge tests to main
	_ = repo.Git.Merge("epic-1/tests", mainBranch, true)
	err = orch.HandleBranchMerged("epic-1/tests")
	if err != nil {
		t.Fatalf("HandleBranchMerged tests failed: %v", err)
	}

	// Docs should now target main
	docsNode, _ := orch.GetBranchStatus("epic-1/docs")
	if docsNode.DependsOn != "" {
		t.Errorf("docs should target root after tests merged, got %q", docsNode.DependsOn)
	}

	// Rebase docs onto new main
	if docsNode.Status == BranchStatusNeedsRebase {
		err = orch.PerformRebase("epic-1/docs")
		if err != nil {
			t.Errorf("Failed to rebase docs: %v", err)
		}
	}

	// Merge docs to main
	_ = repo.Git.Merge("epic-1/docs", mainBranch, true)
	err = orch.HandleBranchMerged("epic-1/docs")
	if err != nil {
		t.Fatalf("HandleBranchMerged docs failed: %v", err)
	}

	// Final state
	stats = orch.GetDAGStats()
	t.Logf("Final stats: %+v", stats)

	if stats["merged"] != 3 {
		t.Errorf("expected 3 merged branches, got %d", stats["merged"])
	}

	// Verify signal counts
	t.Logf("Signal counts: merged=%d, retargeted=%d, rebase_needed=%d",
		len(signaler.MergedCalls),
		len(signaler.RetargetedCalls),
		len(signaler.RebaseNeededCalls))

	if len(signaler.MergedCalls) != 3 {
		t.Errorf("expected 3 merged signals, got %d", len(signaler.MergedCalls))
	}
}

func TestDAGOrchestrator_ParallelBranches(t *testing.T) {
	// Test parallel branches (diamond pattern):
	// main <- api <- frontend
	//           <- backend

	repo, err := NewTestRepo("parallel")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	signaler := &MockSignaler{}
	orch := NewDAGOrchestrator(repo.Path, mainBranch, "upstream", "origin", repo.Git, signaler)

	// Create api (root of the diamond)
	_, _ = repo.CreateBranchWithCommit("api", mainBranch, "API", map[string]string{
		"api.go": "package api",
	})
	_ = orch.RegisterBranch("api", "", "", "", "worker-api")

	// Create frontend and backend (both depend on api)
	_, _ = repo.CreateBranchWithCommit("frontend", "api", "Frontend", map[string]string{
		"frontend.js": "// frontend",
	})
	_ = orch.RegisterBranch("frontend", "api", "", "", "worker-fe")

	_ = repo.Git.CheckoutBranch("api")
	_, _ = repo.CreateBranchWithCommit("backend", "api", "Backend", map[string]string{
		"backend.go": "package backend",
	})
	_ = orch.RegisterBranch("backend", "api", "", "", "worker-be")

	// Update api
	_ = repo.Git.CheckoutBranch("api")
	newSHA, _ := repo.Git.AddCommit("API update", map[string]string{
		"api_v2.go": "package api // v2",
	})

	// Mark api as updated
	affected := orch.dag.MarkBranchUpdated("api", newSHA)

	// Both frontend and backend should need rebase
	if len(affected) != 2 {
		t.Errorf("expected 2 affected branches, got %d: %v", len(affected), affected)
	}

	// Get rebase order - both can be rebased (no dependency between them)
	rebaseOrder := orch.GetRebaseOrder()
	t.Logf("Rebase order: %v", rebaseOrder)

	// Perform rebases
	for _, branch := range rebaseOrder {
		err := orch.PerformRebase(branch)
		if err != nil {
			t.Errorf("Failed to rebase %s: %v", branch, err)
		}
	}

	// Verify both have the new file
	for _, branch := range []string{"frontend", "backend"} {
		_ = repo.Git.CheckoutBranch(branch)
		if _, err := os.Stat(filepath.Join(repo.Path, "api_v2.go")); os.IsNotExist(err) {
			t.Errorf("%s should have api_v2.go after rebase", branch)
		}
	}
}
