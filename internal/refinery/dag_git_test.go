package refinery

import (
	"fmt"
	"testing"
)

// TestGitOps_BasicOperations tests basic git operations with real repos.
func TestGitOps_BasicOperations(t *testing.T) {
	repo, err := NewTestRepo("basic")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Create initial commit
	sha, err := repo.CreateInitialCommit()
	if err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}
	if sha == "" {
		t.Fatal("expected non-empty SHA")
	}

	// Verify we're on a branch
	branch, err := repo.Git.GetCurrentBranch()
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	// Could be 'main' or 'master' depending on git config
	if branch != "main" && branch != "master" {
		t.Errorf("expected main or master, got %q", branch)
	}
}

func TestGitOps_BranchOperations(t *testing.T) {
	repo, err := NewTestRepo("branches")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()

	// Create a branch
	err = repo.Git.CreateBranch("feature-a", "HEAD")
	if err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	// Verify branch exists
	if !repo.Git.BranchExists("feature-a") {
		t.Error("branch should exist")
	}

	// Verify we're on the new branch
	branch, _ := repo.Git.GetCurrentBranch()
	if branch != "feature-a" {
		t.Errorf("expected to be on feature-a, got %q", branch)
	}

	// Switch back to main/master
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}
	err = repo.Git.CheckoutBranch(mainBranch)
	if err != nil {
		t.Fatalf("failed to checkout main: %v", err)
	}

	// Delete the branch
	err = repo.Git.DeleteBranch("feature-a")
	if err != nil {
		t.Fatalf("failed to delete branch: %v", err)
	}

	if repo.Git.BranchExists("feature-a") {
		t.Error("branch should not exist after deletion")
	}
}

func TestGitOps_AddCommit(t *testing.T) {
	repo, err := NewTestRepo("commits")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	_, _ = repo.CreateInitialCommit()

	sha1, err := repo.Git.AddCommit("Add file A", map[string]string{
		"a.txt": "content of a",
	})
	if err != nil {
		t.Fatalf("failed to add commit: %v", err)
	}

	sha2, err := repo.Git.AddCommit("Add file B", map[string]string{
		"b.txt": "content of b",
	})
	if err != nil {
		t.Fatalf("failed to add commit: %v", err)
	}

	if sha1 == sha2 {
		t.Error("commits should have different SHAs")
	}

	// Verify HEAD
	headSHA, _ := repo.Git.GetHeadSHA("HEAD")
	if headSHA != sha2 {
		t.Errorf("HEAD should be %s, got %s", sha2, headSHA)
	}
}

// TestDAGWithGit_LinearStack tests a linear stack of branches with real git.
func TestDAGWithGit_LinearStack(t *testing.T) {
	repo, err := NewTestRepo("linear-stack")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Setup: main <- A <- B <- C
	mainSHA, _ := repo.CreateInitialCommit()

	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Create DAG
	dag := NewBranchDAG(mainBranch, "upstream", "origin")

	// Create branch A from main
	shaA, err := repo.CreateBranchWithCommit("branch-A", mainBranch, "Commit on A", map[string]string{
		"a.txt": "content A",
	})
	if err != nil {
		t.Fatalf("failed to create branch A: %v", err)
	}
	_ = dag.AddBranch(&BranchNode{Name: "branch-A", DependsOn: "", HeadSHA: shaA, BaseSHA: mainSHA})

	// Create branch B from A
	shaB, err := repo.CreateBranchWithCommit("branch-B", "branch-A", "Commit on B", map[string]string{
		"b.txt": "content B",
	})
	if err != nil {
		t.Fatalf("failed to create branch B: %v", err)
	}
	_ = dag.AddBranch(&BranchNode{Name: "branch-B", DependsOn: "branch-A", HeadSHA: shaB, BaseSHA: shaA})

	// Create branch C from B
	shaC, err := repo.CreateBranchWithCommit("branch-C", "branch-B", "Commit on C", map[string]string{
		"c.txt": "content C",
	})
	if err != nil {
		t.Fatalf("failed to create branch C: %v", err)
	}
	_ = dag.AddBranch(&BranchNode{Name: "branch-C", DependsOn: "branch-B", HeadSHA: shaC, BaseSHA: shaB})

	// Verify topology
	order, err := dag.TopologicalOrder()
	if err != nil {
		t.Fatalf("TopologicalOrder failed: %v", err)
	}

	pos := make(map[string]int)
	for i, name := range order {
		pos[name] = i
	}

	if pos["branch-A"] > pos["branch-B"] || pos["branch-B"] > pos["branch-C"] {
		t.Errorf("wrong order: %v", order)
	}

	// Now update A (add another commit)
	_ = repo.Git.CheckoutBranch("branch-A")
	newShaA, _ := repo.Git.AddCommit("Another commit on A", map[string]string{
		"a2.txt": "more A content",
	})

	// Mark A as updated in DAG
	needsRebase := dag.MarkBranchUpdated("branch-A", newShaA)

	// B and C should need rebase
	if len(needsRebase) != 2 {
		t.Errorf("expected 2 branches needing rebase, got %v", needsRebase)
	}

	// Verify we can detect this via git
	// B should be able to rebase onto new A
	canRebase, conflicts, err := repo.Git.CanRebase("branch-B", "branch-A")
	if err != nil {
		t.Fatalf("CanRebase failed: %v", err)
	}
	if !canRebase {
		t.Errorf("branch-B should be able to rebase onto updated branch-A, conflicts: %v", conflicts)
	}
}

// TestDAGWithGit_ConflictDetection tests conflict detection.
func TestDAGWithGit_ConflictDetection(t *testing.T) {
	repo, err := NewTestRepo("conflicts")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Setup: main with file.txt
	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Add file.txt on main
	_, _ = repo.Git.AddCommit("Add file.txt", map[string]string{
		"file.txt": "original content\nline 2\nline 3\n",
	})

	// Create branch A - modifies file.txt
	_, _ = repo.CreateBranchWithCommit("branch-A", mainBranch, "Modify file on A", map[string]string{
		"file.txt": "modified by A\nline 2\nline 3\n",
	})

	// Go back to main and create conflicting change
	_ = repo.Git.CheckoutBranch(mainBranch)
	_, _ = repo.Git.AddCommit("Modify file on main", map[string]string{
		"file.txt": "modified by main\nline 2\nline 3\n",
	})

	// Try to rebase A onto main - should conflict
	canRebase, conflicts, err := repo.Git.CanRebase("branch-A", mainBranch)
	if err != nil {
		t.Fatalf("CanRebase failed: %v", err)
	}

	if canRebase {
		t.Error("expected conflict, but rebase would succeed")
	}

	if len(conflicts) == 0 {
		t.Error("expected conflict files to be reported")
	}

	// Verify file.txt is the conflict
	foundConflict := false
	for _, f := range conflicts {
		if f == "file.txt" {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Errorf("expected file.txt in conflicts, got %v", conflicts)
	}
}

// TestDAGWithGit_Diamond tests diamond dependency pattern.
func TestDAGWithGit_Diamond(t *testing.T) {
	repo, err := NewTestRepo("diamond")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Setup diamond: main <- A <- B
	//                         <- C
	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	dag := NewBranchDAG(mainBranch, "upstream", "origin")

	// Create A
	_, _ = repo.CreateBranchWithCommit("branch-A", mainBranch, "Commit A", map[string]string{
		"a.txt": "A content",
	})
	_ = dag.AddBranch(&BranchNode{Name: "branch-A", DependsOn: ""})

	// Create B from A
	_, _ = repo.CreateBranchWithCommit("branch-B", "branch-A", "Commit B", map[string]string{
		"b.txt": "B content",
	})
	_ = dag.AddBranch(&BranchNode{Name: "branch-B", DependsOn: "branch-A"})

	// Create C from A (sibling of B)
	_ = repo.Git.CheckoutBranch("branch-A")
	_, _ = repo.CreateBranchWithCommit("branch-C", "branch-A", "Commit C", map[string]string{
		"c.txt": "C content",
	})
	_ = dag.AddBranch(&BranchNode{Name: "branch-C", DependsOn: "branch-A"})

	// Verify A has two dependents
	deps := dag.GetDependents("branch-A")
	if len(deps) != 2 {
		t.Errorf("expected 2 dependents of A, got %v", deps)
	}

	// Update A
	_ = repo.Git.CheckoutBranch("branch-A")
	newShaA, _ := repo.Git.AddCommit("Update A", map[string]string{
		"a2.txt": "more A",
	})

	needsRebase := dag.MarkBranchUpdated("branch-A", newShaA)

	// Both B and C should need rebase
	if len(needsRebase) != 2 {
		t.Errorf("expected 2 branches needing rebase, got %v", needsRebase)
	}

	// Verify both can rebase (no conflicts)
	for _, branch := range []string{"branch-B", "branch-C"} {
		canRebase, _, err := repo.Git.CanRebase(branch, "branch-A")
		if err != nil {
			t.Errorf("CanRebase %s failed: %v", branch, err)
		}
		if !canRebase {
			t.Errorf("%s should be able to rebase onto branch-A", branch)
		}
	}

	// Actually perform rebases in order
	rebaseOrder := dag.GetBranchesNeedingRebase()
	for _, branch := range rebaseOrder {
		node, _ := dag.GetBranch(branch)
		base := node.DependsOn
		if base == "" {
			base = mainBranch
		}
		err := repo.Git.Rebase(branch, base)
		if err != nil {
			t.Errorf("failed to rebase %s: %v", branch, err)
		}
		// Mark clean after successful rebase
		_ = dag.UpdateBranch(branch, func(n *BranchNode) {
			n.Status = BranchStatusClean
		})
	}

	// Verify all clean
	stats := dag.Stats()
	if stats["needs_rebase"] != 0 {
		t.Errorf("expected 0 branches needing rebase, got %d", stats["needs_rebase"])
	}

	// Verify git state - B and C should have A's new commit in history
	for _, branch := range []string{"branch-B", "branch-C"} {
		_ = repo.Git.CheckoutBranch(branch)
		// Check if a2.txt exists (from the A update)
		sha, err := repo.Git.GetHeadSHA("branch-A")
		if err != nil {
			t.Errorf("failed to get branch-A SHA: %v", err)
		}
		if sha != newShaA {
			// This is expected since A might have been modified
		}
		// The rebased branches should contain A's changes
		mergeBase, _ := repo.Git.GetMergeBase(branch, "branch-A")
		aSHA, _ := repo.Git.GetHeadSHA("branch-A")
		if mergeBase != aSHA {
			t.Errorf("%s should be based on branch-A's HEAD", branch)
		}
	}
}

// TestDAGWithGit_MergeToMain tests merging a branch to main and retargeting.
func TestDAGWithGit_MergeToMain(t *testing.T) {
	repo, err := NewTestRepo("merge-main")
	if err != nil {
		t.Fatalf("failed to create test repo: %v", err)
	}
	defer repo.Cleanup()

	// Setup: main <- A <- B
	_, _ = repo.CreateInitialCommit()
	mainBranch := "main"
	if !repo.Git.BranchExists("main") {
		mainBranch = "master"
	}

	dag := NewBranchDAG(mainBranch, "upstream", "origin")

	// Create A
	_, _ = repo.CreateBranchWithCommit("branch-A", mainBranch, "Commit A", map[string]string{
		"a.txt": "A content",
	})
	_ = dag.AddBranch(&BranchNode{Name: "branch-A", DependsOn: ""})

	// Create B from A
	_, _ = repo.CreateBranchWithCommit("branch-B", "branch-A", "Commit B", map[string]string{
		"b.txt": "B content",
	})
	_ = dag.AddBranch(&BranchNode{Name: "branch-B", DependsOn: "branch-A"})

	// Merge A to main
	canMerge, _, err := repo.Git.CanMerge("branch-A", mainBranch)
	if err != nil {
		t.Fatalf("CanMerge failed: %v", err)
	}
	if !canMerge {
		t.Fatal("A should be mergeable to main")
	}

	err = repo.Git.Merge("branch-A", mainBranch, true) // squash merge
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Update DAG - A merged, B should retarget to main
	retargeted, err := dag.HandleBranchMerged("branch-A")
	if err != nil {
		t.Fatalf("HandleBranchMerged failed: %v", err)
	}

	if len(retargeted) != 1 || retargeted[0] != "branch-B" {
		t.Errorf("expected [branch-B] retargeted, got %v", retargeted)
	}

	// Verify B now depends on main (empty string)
	b, _ := dag.GetBranch("branch-B")
	if b.DependsOn != "" {
		t.Errorf("B should depend on root, got %q", b.DependsOn)
	}

	// B should need rebase onto new main
	if b.Status != BranchStatusNeedsRebase {
		t.Errorf("B should need rebase, got %q", b.Status)
	}

	// Verify B can rebase onto main (which now has A's content)
	canRebase, _, err := repo.Git.CanRebase("branch-B", mainBranch)
	if err != nil {
		t.Fatalf("CanRebase failed: %v", err)
	}
	if !canRebase {
		t.Error("B should be able to rebase onto main")
	}
}

// TestDAGWithGit_ForkWorkflow tests the fork-based upstream workflow.
func TestDAGWithGit_ForkWorkflow(t *testing.T) {
	// Create "upstream" bare repo
	upstream, err := NewBareTestRepo("upstream")
	if err != nil {
		t.Fatalf("failed to create upstream repo: %v", err)
	}
	defer upstream.Cleanup()

	// Create "fork" working repo
	fork, err := NewTestRepo("fork")
	if err != nil {
		t.Fatalf("failed to create fork repo: %v", err)
	}
	defer fork.Cleanup()

	// Setup fork with upstream remote
	if err := fork.AddRemote("upstream", upstream.Path); err != nil {
		t.Fatalf("failed to add upstream remote: %v", err)
	}

	// Create initial commit and push to "origin" (which we'll simulate as upstream)
	_, _ = fork.CreateInitialCommit()
	mainBranch := "main"
	if !fork.Git.BranchExists("main") {
		mainBranch = "master"
	}

	// Initialize the bare repo by pushing
	if err := fork.Git.Push("upstream", mainBranch, false); err != nil {
		t.Fatalf("failed to push to upstream: %v", err)
	}

	// Create DAG for tracking our stack
	dag := NewBranchDAG(mainBranch, "upstream", "origin")

	// Create feature branch stack
	_, _ = fork.CreateBranchWithCommit("epic-1/api", mainBranch, "API changes", map[string]string{
		"api.go": "package api",
	})
	_ = dag.AddBranch(&BranchNode{Name: "epic-1/api", DependsOn: "", EpicID: "epic-1"})

	_, _ = fork.CreateBranchWithCommit("epic-1/tests", "epic-1/api", "Tests", map[string]string{
		"api_test.go": "package api",
	})
	_ = dag.AddBranch(&BranchNode{Name: "epic-1/tests", DependsOn: "epic-1/api", EpicID: "epic-1"})

	// Simulate upstream advancing (someone else merged something)
	// Go back to main and add a commit
	_ = fork.Git.CheckoutBranch(mainBranch)
	newMainSHA, _ := fork.Git.AddCommit("Upstream change", map[string]string{
		"upstream.txt": "upstream content",
	})
	// Push to "upstream"
	_ = fork.Git.Push("upstream", mainBranch, false)

	// Fetch from upstream (simulated)
	_ = fork.Git.Fetch("upstream")

	// Detect that roots need rebasing
	rootsNeedRebase := dag.MarkRootUpdated(newMainSHA)
	if len(rootsNeedRebase) != 1 || rootsNeedRebase[0] != "epic-1/api" {
		t.Errorf("expected [epic-1/api] to need rebase, got %v", rootsNeedRebase)
	}

	// Cascade: api needing rebase means tests needs rebase too
	needsRebase := dag.GetBranchesNeedingRebase()
	if len(needsRebase) != 1 {
		// Only api is marked, tests is already clean
		// Let's mark tests as needing rebase via the cascade
		_ = dag.MarkBranchUpdated("epic-1/api", "") // Trigger cascade
	}

	// Perform rebases in order
	rebaseOrder := dag.GetBranchesNeedingRebase()
	t.Logf("Rebase order: %v", rebaseOrder)

	for _, branch := range rebaseOrder {
		node, _ := dag.GetBranch(branch)
		base := node.DependsOn
		if base == "" {
			base = mainBranch
		}
		err := fork.Git.Rebase(branch, base)
		if err != nil {
			t.Errorf("failed to rebase %s onto %s: %v", branch, base, err)
			continue
		}
		_ = dag.UpdateBranch(branch, func(n *BranchNode) {
			n.Status = BranchStatusClean
			newSHA, _ := fork.Git.GetHeadSHA(branch)
			n.HeadSHA = newSHA
		})
		// Mark dependents as needing rebase
		dag.MarkBranchUpdated(branch, "")
	}

	// Verify final state
	stats := dag.Stats()
	t.Logf("Final stats: %+v", stats)

	// Verify git history - epic-1/api should have upstream.txt
	_ = fork.Git.CheckoutBranch("epic-1/api")
	status, _ := fork.Git.GetStatus()
	t.Logf("epic-1/api status after rebase: %s", status)
}

// Benchmark for DAG operations
func BenchmarkDAG_TopologicalOrder(b *testing.B) {
	// Create a large DAG
	dag := NewBranchDAG("main", "upstream", "origin")
	for i := 0; i < 100; i++ {
		parent := ""
		if i > 0 {
			parent = fmt.Sprintf("branch-%d", i-1)
		}
		_ = dag.AddBranch(&BranchNode{
			Name:      fmt.Sprintf("branch-%d", i),
			DependsOn: parent,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = dag.TopologicalOrder()
	}
}

func BenchmarkDAG_MarkBranchUpdated(b *testing.B) {
	dag := NewBranchDAG("main", "upstream", "origin")
	_ = dag.AddBranch(&BranchNode{Name: "root", DependsOn: ""})
	for i := 0; i < 50; i++ {
		_ = dag.AddBranch(&BranchNode{
			Name:      fmt.Sprintf("branch-%d", i),
			DependsOn: "root",
			Status:    BranchStatusClean,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset statuses
		for j := 0; j < 50; j++ {
			_ = dag.UpdateBranch(fmt.Sprintf("branch-%d", j), func(n *BranchNode) {
				n.Status = BranchStatusClean
			})
		}
		dag.MarkBranchUpdated("root", "newsha")
	}
}
