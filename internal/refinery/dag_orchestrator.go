// Package refinery provides the merge queue processing agent.
package refinery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AgentSignaler defines the interface for signaling agents about branch updates.
// This can be mocked for testing.
type AgentSignaler interface {
	// SignalRebaseNeeded notifies an agent their branch needs rebasing.
	SignalRebaseNeeded(worker, branch, reason string) error

	// SignalConflict notifies an agent their branch has conflicts.
	SignalConflict(worker, branch string, conflictFiles []string) error

	// SignalMerged notifies an agent their branch was merged.
	SignalMerged(worker, branch string) error

	// SignalRetargeted notifies an agent their branch was retargeted.
	SignalRetargeted(worker, branch, oldTarget, newTarget string) error
}

// DAGOrchestrator manages the branch DAG and coordinates updates.
type DAGOrchestrator struct {
	dag      *BranchDAG
	git      GitOperations
	signaler AgentSignaler
	workDir  string
	dagFile  string // Path to persist DAG state
}

// NewDAGOrchestrator creates a new orchestrator.
func NewDAGOrchestrator(workDir, rootBranch, upstreamRemote, originRemote string, git GitOperations, signaler AgentSignaler) *DAGOrchestrator {
	return &DAGOrchestrator{
		dag:      NewBranchDAG(rootBranch, upstreamRemote, originRemote),
		git:      git,
		signaler: signaler,
		workDir:  workDir,
		dagFile:  filepath.Join(workDir, ".gastown", "branch-dag.json"),
	}
}

// LoadDAG loads the DAG state from disk.
func (o *DAGOrchestrator) LoadDAG() error {
	data, err := os.ReadFile(o.dagFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Fresh start
		}
		return fmt.Errorf("reading DAG file: %w", err)
	}

	if err := json.Unmarshal(data, o.dag); err != nil {
		return fmt.Errorf("parsing DAG file: %w", err)
	}

	return nil
}

// SaveDAG persists the DAG state to disk.
func (o *DAGOrchestrator) SaveDAG() error {
	dir := filepath.Dir(o.dagFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating DAG directory: %w", err)
	}

	data, err := json.MarshalIndent(o.dag, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling DAG: %w", err)
	}

	if err := os.WriteFile(o.dagFile, data, 0644); err != nil {
		return fmt.Errorf("writing DAG file: %w", err)
	}

	return nil
}

// RegisterBranch adds a branch to the DAG.
func (o *DAGOrchestrator) RegisterBranch(name, dependsOn, epicID, subtaskID, worker string) error {
	headSHA, err := o.git.GetHeadSHA(name)
	if err != nil {
		return fmt.Errorf("getting HEAD SHA for %s: %w", name, err)
	}

	baseSHA := ""
	if dependsOn != "" {
		baseSHA, err = o.git.GetHeadSHA(dependsOn)
		if err != nil {
			return fmt.Errorf("getting base SHA for %s: %w", dependsOn, err)
		}
	} else {
		baseSHA, err = o.git.GetHeadSHA(o.dag.RootBranch)
		if err != nil {
			return fmt.Errorf("getting root SHA: %w", err)
		}
	}

	node := &BranchNode{
		Name:        name,
		DependsOn:   dependsOn,
		EpicID:      epicID,
		SubtaskID:   subtaskID,
		Worker:      worker,
		Status:      BranchStatusClean,
		HeadSHA:     headSHA,
		BaseSHA:     baseSHA,
		LastUpdated: time.Now(),
	}

	if err := o.dag.AddBranch(node); err != nil {
		return fmt.Errorf("adding branch to DAG: %w", err)
	}

	return o.SaveDAG()
}

// UnregisterBranch removes a branch from the DAG.
func (o *DAGOrchestrator) UnregisterBranch(name string) error {
	if err := o.dag.RemoveBranch(name); err != nil {
		return err
	}
	return o.SaveDAG()
}

// CheckResult contains the result of checking a branch for updates.
type CheckResult struct {
	Branch        string
	NeedsRebase   bool
	HasConflict   bool
	ConflictFiles []string
	BaseBranch    string
}

// CheckUpstreamUpdates checks if upstream has advanced and marks affected branches.
func (o *DAGOrchestrator) CheckUpstreamUpdates() ([]CheckResult, error) {
	// Fetch from upstream
	if err := o.git.Fetch(o.dag.UpstreamRemote); err != nil {
		return nil, fmt.Errorf("fetching upstream: %w", err)
	}

	// Get current upstream main SHA
	upstreamRef := fmt.Sprintf("%s/%s", o.dag.UpstreamRemote, o.dag.RootBranch)
	newRootSHA, err := o.git.GetHeadSHA(upstreamRef)
	if err != nil {
		return nil, fmt.Errorf("getting upstream SHA: %w", err)
	}

	// Mark roots as needing rebase
	affectedRoots := o.dag.MarkRootUpdated(newRootSHA)

	var results []CheckResult
	for _, branch := range affectedRoots {
		result := CheckResult{
			Branch:      branch,
			NeedsRebase: true,
			BaseBranch:  o.dag.RootBranch,
		}

		// Check for conflicts
		canRebase, conflicts, _ := o.git.CanRebase(branch, upstreamRef)
		if !canRebase {
			result.HasConflict = true
			result.ConflictFiles = conflicts

			// Update DAG
			_ = o.dag.UpdateBranch(branch, func(n *BranchNode) {
				n.Status = BranchStatusConflict
				n.ConflictFiles = conflicts
			})

			// Signal worker
			if node, ok := o.dag.GetBranch(branch); ok && node.Worker != "" && o.signaler != nil {
				_ = o.signaler.SignalConflict(node.Worker, branch, conflicts)
			}
		} else if o.signaler != nil {
			if node, ok := o.dag.GetBranch(branch); ok && node.Worker != "" {
				_ = o.signaler.SignalRebaseNeeded(node.Worker, branch, "upstream updated")
			}
		}

		results = append(results, result)
	}

	_ = o.SaveDAG()
	return results, nil
}

// CheckBranchUpdates checks all branches marked as needing rebase.
func (o *DAGOrchestrator) CheckBranchUpdates() ([]CheckResult, error) {
	needsRebase := o.dag.GetBranchesNeedingRebase()

	var results []CheckResult
	for _, branch := range needsRebase {
		node, ok := o.dag.GetBranch(branch)
		if !ok {
			continue
		}

		baseBranch := node.DependsOn
		if baseBranch == "" {
			baseBranch = o.dag.RootBranch
		}

		result := CheckResult{
			Branch:      branch,
			NeedsRebase: true,
			BaseBranch:  baseBranch,
		}

		// Check for conflicts
		canRebase, conflicts, _ := o.git.CanRebase(branch, baseBranch)
		if !canRebase {
			result.HasConflict = true
			result.ConflictFiles = conflicts

			_ = o.dag.UpdateBranch(branch, func(n *BranchNode) {
				n.Status = BranchStatusConflict
				n.ConflictFiles = conflicts
			})
		}

		results = append(results, result)
	}

	_ = o.SaveDAG()
	return results, nil
}

// PerformRebase rebases a branch onto its base and updates the DAG.
func (o *DAGOrchestrator) PerformRebase(branch string) error {
	node, ok := o.dag.GetBranch(branch)
	if !ok {
		return fmt.Errorf("branch %q not found", branch)
	}

	baseBranch := node.DependsOn
	if baseBranch == "" {
		baseBranch = o.dag.RootBranch
	}

	// Perform the rebase
	if err := o.git.Rebase(branch, baseBranch); err != nil {
		return fmt.Errorf("rebase failed: %w", err)
	}

	// Update DAG
	newSHA, _ := o.git.GetHeadSHA(branch)
	baseSHA, _ := o.git.GetHeadSHA(baseBranch)

	_ = o.dag.UpdateBranch(branch, func(n *BranchNode) {
		n.Status = BranchStatusClean
		n.HeadSHA = newSHA
		n.BaseSHA = baseSHA
		n.ConflictFiles = nil
	})

	// Cascade: mark dependents as needing rebase
	affected := o.dag.MarkBranchUpdated(branch, newSHA)

	// Signal affected workers
	if o.signaler != nil {
		for _, dep := range affected {
			if depNode, ok := o.dag.GetBranch(dep); ok && depNode.Worker != "" {
				_ = o.signaler.SignalRebaseNeeded(depNode.Worker, dep, fmt.Sprintf("%s was updated", branch))
			}
		}
	}

	return o.SaveDAG()
}

// HandleBranchMerged processes a branch being merged and retargets dependents.
func (o *DAGOrchestrator) HandleBranchMerged(branch string) error {
	node, ok := o.dag.GetBranch(branch)
	if !ok {
		return fmt.Errorf("branch %q not found", branch)
	}

	oldTarget := node.DependsOn
	if oldTarget == "" {
		oldTarget = o.dag.RootBranch
	}

	retargeted, err := o.dag.HandleBranchMerged(branch)
	if err != nil {
		return err
	}

	// Signal affected workers
	if o.signaler != nil {
		// Notify merged branch's worker
		if node.Worker != "" {
			_ = o.signaler.SignalMerged(node.Worker, branch)
		}

		// Notify retargeted branches
		for _, dep := range retargeted {
			if depNode, ok := o.dag.GetBranch(dep); ok && depNode.Worker != "" {
				_ = o.signaler.SignalRetargeted(depNode.Worker, dep, branch, oldTarget)
				_ = o.signaler.SignalRebaseNeeded(depNode.Worker, dep, fmt.Sprintf("%s was merged", branch))
			}
		}
	}

	return o.SaveDAG()
}

// GetDAGStats returns statistics about the DAG.
func (o *DAGOrchestrator) GetDAGStats() map[string]int {
	return o.dag.Stats()
}

// GetBranchStatus returns the status of a specific branch.
func (o *DAGOrchestrator) GetBranchStatus(branch string) (*BranchNode, bool) {
	return o.dag.GetBranch(branch)
}

// GetRebaseOrder returns branches that need rebasing in correct order.
func (o *DAGOrchestrator) GetRebaseOrder() []string {
	return o.dag.GetBranchesNeedingRebase()
}

// GetConflicts returns all branches with conflicts.
func (o *DAGOrchestrator) GetConflicts() []string {
	return o.dag.GetBranchesWithConflict()
}

// ValidateDAG checks the DAG for inconsistencies.
func (o *DAGOrchestrator) ValidateDAG() []error {
	return o.dag.Validate()
}

// MockSignaler is a test implementation of AgentSignaler that records calls.
type MockSignaler struct {
	RebaseNeededCalls  []SignalCall
	ConflictCalls      []SignalCall
	MergedCalls        []SignalCall
	RetargetedCalls    []SignalCall
}

// SignalCall records a signal call for verification in tests.
type SignalCall struct {
	Worker        string
	Branch        string
	Reason        string
	ConflictFiles []string
	OldTarget     string
	NewTarget     string
}

func (m *MockSignaler) SignalRebaseNeeded(worker, branch, reason string) error {
	m.RebaseNeededCalls = append(m.RebaseNeededCalls, SignalCall{
		Worker: worker,
		Branch: branch,
		Reason: reason,
	})
	return nil
}

func (m *MockSignaler) SignalConflict(worker, branch string, conflictFiles []string) error {
	m.ConflictCalls = append(m.ConflictCalls, SignalCall{
		Worker:        worker,
		Branch:        branch,
		ConflictFiles: conflictFiles,
	})
	return nil
}

func (m *MockSignaler) SignalMerged(worker, branch string) error {
	m.MergedCalls = append(m.MergedCalls, SignalCall{
		Worker: worker,
		Branch: branch,
	})
	return nil
}

func (m *MockSignaler) SignalRetargeted(worker, branch, oldTarget, newTarget string) error {
	m.RetargetedCalls = append(m.RetargetedCalls, SignalCall{
		Worker:    worker,
		Branch:    branch,
		OldTarget: oldTarget,
		NewTarget: newTarget,
	})
	return nil
}

// Reset clears all recorded calls.
func (m *MockSignaler) Reset() {
	m.RebaseNeededCalls = nil
	m.ConflictCalls = nil
	m.MergedCalls = nil
	m.RetargetedCalls = nil
}
