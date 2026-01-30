// Package refinery provides the merge queue processing agent.
package refinery

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// BranchStatus represents the current state of a branch in the DAG.
type BranchStatus string

const (
	// BranchStatusClean means the branch is up-to-date with its base.
	BranchStatusClean BranchStatus = "clean"

	// BranchStatusNeedsRebase means the base branch has changed.
	BranchStatusNeedsRebase BranchStatus = "needs-rebase"

	// BranchStatusConflict means a rebase would cause conflicts.
	BranchStatusConflict BranchStatus = "conflict"

	// BranchStatusMerged means the branch has been merged to its target.
	BranchStatusMerged BranchStatus = "merged"

	// BranchStatusClosed means the branch was abandoned.
	BranchStatusClosed BranchStatus = "closed"
)

// BranchNode represents a branch in the dependency DAG.
type BranchNode struct {
	// Name is the branch name (e.g., "epic-abc/api").
	Name string `json:"name"`

	// DependsOn is the parent branch this was forked from.
	// Empty string means it targets the root (e.g., main).
	DependsOn string `json:"depends_on"`

	// EpicID links this branch to an epic (optional).
	EpicID string `json:"epic_id,omitempty"`

	// SubtaskID links this branch to a subtask (optional).
	SubtaskID string `json:"subtask_id,omitempty"`

	// Worker is the agent assigned to this branch.
	Worker string `json:"worker,omitempty"`

	// Status is the current branch status.
	Status BranchStatus `json:"status"`

	// HeadSHA is the current commit SHA of the branch.
	HeadSHA string `json:"head_sha"`

	// BaseSHA is the SHA of the base branch when this was created/rebased.
	BaseSHA string `json:"base_sha"`

	// LastUpdated is when this node was last modified.
	LastUpdated time.Time `json:"last_updated"`

	// ConflictFiles lists files that would conflict on rebase.
	ConflictFiles []string `json:"conflict_files,omitempty"`

	// PRNumber is the upstream PR number (if submitted).
	PRNumber int `json:"pr_number,omitempty"`

	// PRURL is the upstream PR URL (if submitted).
	PRURL string `json:"pr_url,omitempty"`
}

// BranchDAG tracks branch dependencies for stacked PR workflows.
type BranchDAG struct {
	mu sync.RWMutex

	// Nodes maps branch name to node.
	Nodes map[string]*BranchNode `json:"nodes"`

	// Children maps branch name to its dependents.
	Children map[string][]string `json:"children"`

	// RootBranch is the ultimate target (e.g., "main").
	RootBranch string `json:"root_branch"`

	// UpstreamRemote is the upstream remote name (e.g., "upstream").
	UpstreamRemote string `json:"upstream_remote"`

	// OriginRemote is our fork's remote name (e.g., "origin").
	OriginRemote string `json:"origin_remote"`
}

// NewBranchDAG creates a new branch dependency graph.
func NewBranchDAG(rootBranch, upstreamRemote, originRemote string) *BranchDAG {
	return &BranchDAG{
		Nodes:          make(map[string]*BranchNode),
		Children:       make(map[string][]string),
		RootBranch:     rootBranch,
		UpstreamRemote: upstreamRemote,
		OriginRemote:   originRemote,
	}
}

// AddBranch adds a new branch to the DAG.
func (d *BranchDAG) AddBranch(node *BranchNode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if node.Name == "" {
		return fmt.Errorf("branch name is required")
	}

	if _, exists := d.Nodes[node.Name]; exists {
		return fmt.Errorf("branch %q already exists", node.Name)
	}

	// Validate parent exists (unless it's the root)
	if node.DependsOn != "" && node.DependsOn != d.RootBranch {
		if _, exists := d.Nodes[node.DependsOn]; !exists {
			return fmt.Errorf("parent branch %q does not exist", node.DependsOn)
		}
	}

	if node.Status == "" {
		node.Status = BranchStatusClean
	}
	if node.LastUpdated.IsZero() {
		node.LastUpdated = time.Now()
	}

	d.Nodes[node.Name] = node

	// Track in children map
	parent := node.DependsOn
	if parent == "" {
		parent = d.RootBranch
	}
	d.Children[parent] = append(d.Children[parent], node.Name)

	return nil
}

// RemoveBranch removes a branch from the DAG.
// Returns error if the branch has dependents.
func (d *BranchDAG) RemoveBranch(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	node, exists := d.Nodes[name]
	if !exists {
		return fmt.Errorf("branch %q not found", name)
	}

	// Check for dependents
	if deps := d.Children[name]; len(deps) > 0 {
		return fmt.Errorf("branch %q has dependents: %v", name, deps)
	}

	// Remove from parent's children
	parent := node.DependsOn
	if parent == "" {
		parent = d.RootBranch
	}
	d.Children[parent] = removeFromSlice(d.Children[parent], name)

	delete(d.Nodes, name)
	delete(d.Children, name)

	return nil
}

// GetBranch returns a branch node by name.
func (d *BranchDAG) GetBranch(name string) (*BranchNode, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	node, exists := d.Nodes[name]
	if !exists {
		return nil, false
	}
	// Return a copy to avoid races
	copy := *node
	return &copy, true
}

// UpdateBranch updates a branch's fields.
func (d *BranchDAG) UpdateBranch(name string, update func(*BranchNode)) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	node, exists := d.Nodes[name]
	if !exists {
		return fmt.Errorf("branch %q not found", name)
	}

	update(node)
	node.LastUpdated = time.Now()
	return nil
}

// GetRoots returns branches that depend directly on the root branch.
func (d *BranchDAG) GetRoots() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	roots := make([]string, len(d.Children[d.RootBranch]))
	copy(roots, d.Children[d.RootBranch])
	sort.Strings(roots)
	return roots
}

// GetDependents returns all branches that depend on the given branch.
func (d *BranchDAG) GetDependents(name string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deps := make([]string, len(d.Children[name]))
	copy(deps, d.Children[name])
	sort.Strings(deps)
	return deps
}

// GetAllDependents returns all transitive dependents of a branch.
func (d *BranchDAG) GetAllDependents(name string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []string
	visited := make(map[string]bool)

	var visit func(n string)
	visit = func(n string) {
		for _, child := range d.Children[n] {
			if !visited[child] {
				visited[child] = true
				result = append(result, child)
				visit(child)
			}
		}
	}

	visit(name)
	sort.Strings(result)
	return result
}

// GetAncestors returns all ancestors of a branch up to the root.
func (d *BranchDAG) GetAncestors(name string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []string
	current := name

	for {
		node, exists := d.Nodes[current]
		if !exists {
			break
		}
		if node.DependsOn == "" || node.DependsOn == d.RootBranch {
			break
		}
		result = append(result, node.DependsOn)
		current = node.DependsOn
	}

	return result
}

// TopologicalOrder returns branches in dependency order (dependencies first).
// This is the order in which rebases should be performed.
func (d *BranchDAG) TopologicalOrder() ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []string
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var visit func(name string) error
	visit = func(name string) error {
		if inStack[name] {
			return fmt.Errorf("cycle detected involving %s", name)
		}
		if visited[name] {
			return nil
		}

		inStack[name] = true

		node, exists := d.Nodes[name]
		if exists && node.DependsOn != "" && node.DependsOn != d.RootBranch {
			if err := visit(node.DependsOn); err != nil {
				return err
			}
		}

		inStack[name] = false
		visited[name] = true
		result = append(result, name)
		return nil
	}

	// Visit all nodes
	for name := range d.Nodes {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// MarkBranchUpdated marks a branch as updated and returns all branches
// that now need rebasing (transitive dependents).
func (d *BranchDAG) MarkBranchUpdated(name, newHeadSHA string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Update the branch's HEAD
	if node, exists := d.Nodes[name]; exists {
		node.HeadSHA = newHeadSHA
		node.LastUpdated = time.Now()
	}

	// Mark all dependents as needing rebase
	var needsRebase []string
	var markDependents func(n string)
	markDependents = func(n string) {
		for _, child := range d.Children[n] {
			if node, exists := d.Nodes[child]; exists {
				if node.Status == BranchStatusClean {
					node.Status = BranchStatusNeedsRebase
					node.LastUpdated = time.Now()
					needsRebase = append(needsRebase, child)
				}
				markDependents(child)
			}
		}
	}

	markDependents(name)
	return needsRebase
}

// MarkRootUpdated marks the root branch as updated (e.g., upstream/main advanced).
// Returns all root-level branches that need rebasing.
func (d *BranchDAG) MarkRootUpdated(newRootSHA string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	var needsRebase []string

	// Mark all branches whose BaseSHA no longer matches root
	for _, name := range d.Children[d.RootBranch] {
		if node, exists := d.Nodes[name]; exists {
			if node.BaseSHA != newRootSHA && node.Status == BranchStatusClean {
				node.Status = BranchStatusNeedsRebase
				node.LastUpdated = time.Now()
				needsRebase = append(needsRebase, name)
			}
		}
	}

	return needsRebase
}

// GetBranchesNeedingRebase returns all branches with NeedsRebase status
// in topological order (so rebasing them in order is safe).
func (d *BranchDAG) GetBranchesNeedingRebase() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	order, err := d.topologicalOrderLocked()
	if err != nil {
		return nil
	}

	var result []string
	for _, name := range order {
		if node, exists := d.Nodes[name]; exists {
			if node.Status == BranchStatusNeedsRebase {
				result = append(result, name)
			}
		}
	}

	return result
}

// GetBranchesWithConflict returns all branches with Conflict status.
func (d *BranchDAG) GetBranchesWithConflict() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []string
	for name, node := range d.Nodes {
		if node.Status == BranchStatusConflict {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

// RetargetBranch changes a branch's parent (used when parent merges to root).
func (d *BranchDAG) RetargetBranch(name, newParent string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	node, exists := d.Nodes[name]
	if !exists {
		return fmt.Errorf("branch %q not found", name)
	}

	oldParent := node.DependsOn
	if oldParent == "" {
		oldParent = d.RootBranch
	}

	// Remove from old parent's children
	d.Children[oldParent] = removeFromSlice(d.Children[oldParent], name)

	// Add to new parent's children
	newParentKey := newParent
	if newParent == "" {
		newParentKey = d.RootBranch
	}
	d.Children[newParentKey] = append(d.Children[newParentKey], name)

	node.DependsOn = newParent
	node.Status = BranchStatusNeedsRebase
	node.LastUpdated = time.Now()

	return nil
}

// HandleBranchMerged processes a branch being merged to its target.
// Retargets all dependents to the merged branch's parent.
func (d *BranchDAG) HandleBranchMerged(name string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	node, exists := d.Nodes[name]
	if !exists {
		return nil, fmt.Errorf("branch %q not found", name)
	}

	// Get the branch's parent (what dependents should now target)
	newTarget := node.DependsOn

	// Retarget all direct dependents
	retargeted := make([]string, len(d.Children[name]))
	copy(retargeted, d.Children[name])

	for _, child := range retargeted {
		if childNode, exists := d.Nodes[child]; exists {
			childNode.DependsOn = newTarget
			childNode.Status = BranchStatusNeedsRebase
			childNode.LastUpdated = time.Now()

			// Update children tracking
			d.Children[name] = removeFromSlice(d.Children[name], child)
			newParentKey := newTarget
			if newTarget == "" {
				newParentKey = d.RootBranch
			}
			d.Children[newParentKey] = append(d.Children[newParentKey], child)
		}
	}

	// Mark the merged branch
	node.Status = BranchStatusMerged
	node.LastUpdated = time.Now()

	return retargeted, nil
}

// Validate checks the DAG for inconsistencies.
func (d *BranchDAG) Validate() []error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var errs []error

	// Check for orphaned children references
	for parent, children := range d.Children {
		if parent != d.RootBranch {
			if _, exists := d.Nodes[parent]; !exists {
				errs = append(errs, fmt.Errorf("children reference non-existent parent %q", parent))
			}
		}
		for _, child := range children {
			if _, exists := d.Nodes[child]; !exists {
				errs = append(errs, fmt.Errorf("parent %q references non-existent child %q", parent, child))
			}
		}
	}

	// Check for cycles
	if _, err := d.topologicalOrderLocked(); err != nil {
		errs = append(errs, err)
	}

	// Check parent references match children tracking
	for name, node := range d.Nodes {
		parent := node.DependsOn
		if parent == "" {
			parent = d.RootBranch
		}
		found := false
		for _, child := range d.Children[parent] {
			if child == name {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("branch %q not in parent %q's children", name, parent))
		}
	}

	return errs
}

// Stats returns statistics about the DAG.
func (d *BranchDAG) Stats() map[string]int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := map[string]int{
		"total":        len(d.Nodes),
		"clean":        0,
		"needs_rebase": 0,
		"conflict":     0,
		"merged":       0,
		"closed":       0,
		"roots":        len(d.Children[d.RootBranch]),
	}

	for _, node := range d.Nodes {
		switch node.Status {
		case BranchStatusClean:
			stats["clean"]++
		case BranchStatusNeedsRebase:
			stats["needs_rebase"]++
		case BranchStatusConflict:
			stats["conflict"]++
		case BranchStatusMerged:
			stats["merged"]++
		case BranchStatusClosed:
			stats["closed"]++
		}
	}

	return stats
}

// topologicalOrderLocked returns topological order (must hold lock).
func (d *BranchDAG) topologicalOrderLocked() ([]string, error) {
	var result []string
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var visit func(name string) error
	visit = func(name string) error {
		if inStack[name] {
			return fmt.Errorf("cycle detected involving %s", name)
		}
		if visited[name] {
			return nil
		}

		inStack[name] = true

		node, exists := d.Nodes[name]
		if exists && node.DependsOn != "" && node.DependsOn != d.RootBranch {
			if err := visit(node.DependsOn); err != nil {
				return err
			}
		}

		inStack[name] = false
		visited[name] = true
		result = append(result, name)
		return nil
	}

	for name := range d.Nodes {
		if err := visit(name); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// Helper to remove an element from a slice
func removeFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
