package refinery

import (
	"fmt"
	"testing"
	"time"
)

func TestBranchDAG_AddBranch(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	// Add a root-level branch
	err := dag.AddBranch(&BranchNode{
		Name:      "feature-a",
		DependsOn: "",
		HeadSHA:   "abc123",
	})
	if err != nil {
		t.Fatalf("failed to add branch: %v", err)
	}

	// Verify it was added
	node, exists := dag.GetBranch("feature-a")
	if !exists {
		t.Fatal("branch not found after adding")
	}
	if node.Status != BranchStatusClean {
		t.Errorf("expected status %q, got %q", BranchStatusClean, node.Status)
	}

	// Verify it's in roots
	roots := dag.GetRoots()
	if len(roots) != 1 || roots[0] != "feature-a" {
		t.Errorf("expected [feature-a] in roots, got %v", roots)
	}
}

func TestBranchDAG_AddBranch_WithParent(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	// Add parent first
	_ = dag.AddBranch(&BranchNode{Name: "parent", DependsOn: ""})

	// Add child
	err := dag.AddBranch(&BranchNode{
		Name:      "child",
		DependsOn: "parent",
	})
	if err != nil {
		t.Fatalf("failed to add child branch: %v", err)
	}

	// Verify dependency
	deps := dag.GetDependents("parent")
	if len(deps) != 1 || deps[0] != "child" {
		t.Errorf("expected [child] as dependent, got %v", deps)
	}
}

func TestBranchDAG_AddBranch_MissingParent(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	err := dag.AddBranch(&BranchNode{
		Name:      "orphan",
		DependsOn: "nonexistent",
	})
	if err == nil {
		t.Error("expected error for missing parent")
	}
}

func TestBranchDAG_AddBranch_Duplicate(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "branch-a"})
	err := dag.AddBranch(&BranchNode{Name: "branch-a"})
	if err == nil {
		t.Error("expected error for duplicate branch")
	}
}

func TestBranchDAG_RemoveBranch(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "to-remove"})

	err := dag.RemoveBranch("to-remove")
	if err != nil {
		t.Fatalf("failed to remove branch: %v", err)
	}

	_, exists := dag.GetBranch("to-remove")
	if exists {
		t.Error("branch still exists after removal")
	}
}

func TestBranchDAG_RemoveBranch_WithDependents(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "parent"})
	_ = dag.AddBranch(&BranchNode{Name: "child", DependsOn: "parent"})

	err := dag.RemoveBranch("parent")
	if err == nil {
		t.Error("expected error when removing branch with dependents")
	}
}

func TestBranchDAG_TopologicalOrder_Linear(t *testing.T) {
	// Linear stack: main <- A <- B <- C
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A"})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "B"})

	order, err := dag.TopologicalOrder()
	if err != nil {
		t.Fatalf("TopologicalOrder failed: %v", err)
	}

	// Build position map
	pos := make(map[string]int)
	for i, name := range order {
		pos[name] = i
	}

	// Verify order: A before B before C
	if pos["A"] > pos["B"] {
		t.Error("A should come before B")
	}
	if pos["B"] > pos["C"] {
		t.Error("B should come before C")
	}
}

func TestBranchDAG_TopologicalOrder_Diamond(t *testing.T) {
	// Diamond: main <- A <- B <- D
	//                   \<- C <-/
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A"})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "A"})
	_ = dag.AddBranch(&BranchNode{Name: "D", DependsOn: "B"}) // D depends on B (could also depend on C)

	order, err := dag.TopologicalOrder()
	if err != nil {
		t.Fatalf("TopologicalOrder failed: %v", err)
	}

	pos := make(map[string]int)
	for i, name := range order {
		pos[name] = i
	}

	// A must come before B, C, and D
	if pos["A"] > pos["B"] || pos["A"] > pos["C"] || pos["A"] > pos["D"] {
		t.Error("A should come before all others")
	}
	// B must come before D
	if pos["B"] > pos["D"] {
		t.Error("B should come before D")
	}
}

func TestBranchDAG_TopologicalOrder_Cycle(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	// Manually create a cycle (bypassing normal validation)
	dag.Nodes["A"] = &BranchNode{Name: "A", DependsOn: "C"}
	dag.Nodes["B"] = &BranchNode{Name: "B", DependsOn: "A"}
	dag.Nodes["C"] = &BranchNode{Name: "C", DependsOn: "B"}
	dag.Children["C"] = []string{"A"}
	dag.Children["A"] = []string{"B"}
	dag.Children["B"] = []string{"C"}

	_, err := dag.TopologicalOrder()
	if err == nil {
		t.Error("expected cycle detection error")
	}
}

func TestBranchDAG_MarkBranchUpdated(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: "", Status: BranchStatusClean})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A", Status: BranchStatusClean})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "B", Status: BranchStatusClean})

	// Update A
	needsRebase := dag.MarkBranchUpdated("A", "newsha123")

	// B and C should need rebase
	if len(needsRebase) != 2 {
		t.Errorf("expected 2 branches needing rebase, got %d: %v", len(needsRebase), needsRebase)
	}

	// Verify statuses
	b, _ := dag.GetBranch("B")
	c, _ := dag.GetBranch("C")
	if b.Status != BranchStatusNeedsRebase {
		t.Errorf("B should need rebase, got %q", b.Status)
	}
	if c.Status != BranchStatusNeedsRebase {
		t.Errorf("C should need rebase, got %q", c.Status)
	}
}

func TestBranchDAG_MarkBranchUpdated_AlreadyNeedsRebase(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A", Status: BranchStatusNeedsRebase})

	// Update A again
	needsRebase := dag.MarkBranchUpdated("A", "newsha")

	// B should not be in the list (already needs rebase)
	if len(needsRebase) != 0 {
		t.Errorf("expected 0 new branches needing rebase, got %v", needsRebase)
	}
}

func TestBranchDAG_MarkRootUpdated(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: "", BaseSHA: "old123", Status: BranchStatusClean})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "", BaseSHA: "old123", Status: BranchStatusClean})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "A", Status: BranchStatusClean}) // Not a root

	// Upstream main advanced
	needsRebase := dag.MarkRootUpdated("new456")

	// A and B are roots and should need rebase
	if len(needsRebase) != 2 {
		t.Errorf("expected 2 root branches needing rebase, got %v", needsRebase)
	}

	a, _ := dag.GetBranch("A")
	b, _ := dag.GetBranch("B")
	if a.Status != BranchStatusNeedsRebase {
		t.Errorf("A should need rebase")
	}
	if b.Status != BranchStatusNeedsRebase {
		t.Errorf("B should need rebase")
	}
}

func TestBranchDAG_GetBranchesNeedingRebase(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: "", Status: BranchStatusClean})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A", Status: BranchStatusNeedsRebase})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "B", Status: BranchStatusNeedsRebase})
	_ = dag.AddBranch(&BranchNode{Name: "D", DependsOn: "", Status: BranchStatusClean})

	needsRebase := dag.GetBranchesNeedingRebase()

	// Should be in topological order: B before C
	if len(needsRebase) != 2 {
		t.Fatalf("expected 2 branches, got %v", needsRebase)
	}

	pos := make(map[string]int)
	for i, name := range needsRebase {
		pos[name] = i
	}

	if pos["B"] > pos["C"] {
		t.Error("B should come before C in rebase order")
	}
}

func TestBranchDAG_HandleBranchMerged(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	// Setup: main <- A <- B
	//             <- C (also depends on A, will be retargeted when A merges)
	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A"})

	// A merges to main
	retargeted, err := dag.HandleBranchMerged("A")
	if err != nil {
		t.Fatalf("HandleBranchMerged failed: %v", err)
	}

	// B should be retargeted to main (A's parent)
	if len(retargeted) != 1 || retargeted[0] != "B" {
		t.Errorf("expected [B] retargeted, got %v", retargeted)
	}

	// Verify B now depends on "" (root)
	b, _ := dag.GetBranch("B")
	if b.DependsOn != "" {
		t.Errorf("B should now depend on root, got %q", b.DependsOn)
	}
	if b.Status != BranchStatusNeedsRebase {
		t.Errorf("B should need rebase after retarget")
	}

	// Verify A is marked merged
	a, _ := dag.GetBranch("A")
	if a.Status != BranchStatusMerged {
		t.Errorf("A should be marked merged, got %q", a.Status)
	}

	// Verify B is now in root's children
	roots := dag.GetRoots()
	foundB := false
	for _, r := range roots {
		if r == "B" {
			foundB = true
		}
	}
	if !foundB {
		t.Error("B should now be a root-level branch")
	}
}

func TestBranchDAG_RetargetBranch(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "A"})

	// Retarget C from A to B
	err := dag.RetargetBranch("C", "B")
	if err != nil {
		t.Fatalf("RetargetBranch failed: %v", err)
	}

	c, _ := dag.GetBranch("C")
	if c.DependsOn != "B" {
		t.Errorf("C should depend on B, got %q", c.DependsOn)
	}
	if c.Status != BranchStatusNeedsRebase {
		t.Errorf("C should need rebase after retarget")
	}

	// Verify C is no longer in A's children
	aDeps := dag.GetDependents("A")
	for _, d := range aDeps {
		if d == "C" {
			t.Error("C should not be in A's dependents anymore")
		}
	}

	// Verify C is in B's children
	bDeps := dag.GetDependents("B")
	foundC := false
	for _, d := range bDeps {
		if d == "C" {
			foundC = true
		}
	}
	if !foundC {
		t.Error("C should be in B's dependents")
	}
}

func TestBranchDAG_GetAllDependents(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	// Tree: A -> B -> D
	//         -> C -> E
	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A"})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "A"})
	_ = dag.AddBranch(&BranchNode{Name: "D", DependsOn: "B"})
	_ = dag.AddBranch(&BranchNode{Name: "E", DependsOn: "C"})

	allDeps := dag.GetAllDependents("A")

	if len(allDeps) != 4 {
		t.Errorf("expected 4 dependents of A, got %v", allDeps)
	}

	expected := map[string]bool{"B": true, "C": true, "D": true, "E": true}
	for _, d := range allDeps {
		if !expected[d] {
			t.Errorf("unexpected dependent: %s", d)
		}
	}
}

func TestBranchDAG_GetAncestors(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A"})
	_ = dag.AddBranch(&BranchNode{Name: "C", DependsOn: "B"})

	ancestors := dag.GetAncestors("C")

	if len(ancestors) != 2 {
		t.Errorf("expected 2 ancestors, got %v", ancestors)
	}
	if ancestors[0] != "B" || ancestors[1] != "A" {
		t.Errorf("expected [B, A], got %v", ancestors)
	}
}

func TestBranchDAG_Validate(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", DependsOn: ""})
	_ = dag.AddBranch(&BranchNode{Name: "B", DependsOn: "A"})

	errs := dag.Validate()
	if len(errs) > 0 {
		t.Errorf("expected no validation errors, got %v", errs)
	}
}

func TestBranchDAG_Validate_Inconsistent(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	// Manually create inconsistency
	dag.Nodes["A"] = &BranchNode{Name: "A", DependsOn: ""}
	// Don't add to Children - creates inconsistency

	errs := dag.Validate()
	if len(errs) == 0 {
		t.Error("expected validation errors for inconsistent DAG")
	}
}

func TestBranchDAG_Stats(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	_ = dag.AddBranch(&BranchNode{Name: "A", Status: BranchStatusClean})
	_ = dag.AddBranch(&BranchNode{Name: "B", Status: BranchStatusNeedsRebase})
	_ = dag.AddBranch(&BranchNode{Name: "C", Status: BranchStatusConflict})
	_ = dag.AddBranch(&BranchNode{Name: "D", Status: BranchStatusMerged})

	stats := dag.Stats()

	if stats["total"] != 4 {
		t.Errorf("expected total=4, got %d", stats["total"])
	}
	if stats["clean"] != 1 {
		t.Errorf("expected clean=1, got %d", stats["clean"])
	}
	if stats["needs_rebase"] != 1 {
		t.Errorf("expected needs_rebase=1, got %d", stats["needs_rebase"])
	}
	if stats["conflict"] != 1 {
		t.Errorf("expected conflict=1, got %d", stats["conflict"])
	}
	if stats["merged"] != 1 {
		t.Errorf("expected merged=1, got %d", stats["merged"])
	}
}

func TestBranchDAG_Concurrency(t *testing.T) {
	dag := NewBranchDAG("main", "upstream", "origin")

	// Add initial branches
	for i := 0; i < 10; i++ {
		_ = dag.AddBranch(&BranchNode{
			Name:      fmt.Sprintf("branch-%d", i),
			DependsOn: "",
		})
	}

	// Concurrent reads and writes
	done := make(chan bool)

	// Reader
	go func() {
		for i := 0; i < 100; i++ {
			_ = dag.GetRoots()
			_, _ = dag.TopologicalOrder()
			_ = dag.Stats()
		}
		done <- true
	}()

	// Writer
	go func() {
		for i := 10; i < 20; i++ {
			_ = dag.AddBranch(&BranchNode{
				Name:      fmt.Sprintf("branch-%d", i),
				DependsOn: "",
			})
			dag.MarkBranchUpdated(fmt.Sprintf("branch-%d", i-10), "newsha")
		}
		done <- true
	}()

	// Wait with timeout
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent operations")
		}
	}
}

