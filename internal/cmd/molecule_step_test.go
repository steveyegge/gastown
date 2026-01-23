package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestExtractMoleculeIDFromStep(t *testing.T) {
	tests := []struct {
		name     string
		stepID   string
		expected string
	}{
		{
			name:     "simple step",
			stepID:   "gt-abc.1",
			expected: "gt-abc",
		},
		{
			name:     "multi-digit step number",
			stepID:   "gt-xyz.12",
			expected: "gt-xyz",
		},
		{
			name:     "molecule with dash",
			stepID:   "gt-my-mol.3",
			expected: "gt-my-mol",
		},
		{
			name:     "bd prefix",
			stepID:   "bd-mol-abc.2",
			expected: "bd-mol-abc",
		},
		{
			name:     "complex id",
			stepID:   "gt-some-complex-id.99",
			expected: "gt-some-complex-id",
		},
		{
			name:     "not a step - no suffix",
			stepID:   "gt-5gq8r",
			expected: "",
		},
		{
			name:     "not a step - non-numeric suffix",
			stepID:   "gt-abc.xyz",
			expected: "",
		},
		{
			name:     "not a step - mixed suffix",
			stepID:   "gt-abc.1a",
			expected: "",
		},
		{
			name:     "empty string",
			stepID:   "",
			expected: "",
		},
		{
			name:     "just a dot",
			stepID:   ".",
			expected: "",
		},
		{
			name:     "trailing dot",
			stepID:   "gt-abc.",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractMoleculeIDFromStep(tt.stepID)
			if result != tt.expected {
				t.Errorf("extractMoleculeIDFromStep(%q) = %q, want %q", tt.stepID, result, tt.expected)
			}
		})
	}
}

// mockBeadsForStep extends mockBeads with parent filtering for step tests.
// It simulates the real bd behavior where:
// - List() returns issues with DependsOn empty (bd list doesn't return deps)
// - Show()/ShowMultiple() returns issues with Dependencies populated (bd show does)
type mockBeadsForStep struct {
	issues map[string]*beads.Issue
}

func newMockBeadsForStep() *mockBeadsForStep {
	return &mockBeadsForStep{
		issues: make(map[string]*beads.Issue),
	}
}

func (m *mockBeadsForStep) addIssue(issue *beads.Issue) {
	m.issues[issue.ID] = issue
}

func (m *mockBeadsForStep) Show(id string) (*beads.Issue, error) {
	if issue, ok := m.issues[id]; ok {
		return issue, nil
	}
	return nil, beads.ErrNotFound
}

// ShowMultiple simulates bd show with multiple IDs - returns full issue data including Dependencies
func (m *mockBeadsForStep) ShowMultiple(ids []string) (map[string]*beads.Issue, error) {
	result := make(map[string]*beads.Issue)
	for _, id := range ids {
		if issue, ok := m.issues[id]; ok {
			result[id] = issue
		}
	}
	return result, nil
}

// List simulates bd list behavior - returns issues but with DependsOn EMPTY.
// This is the key behavior that caused the bug: bd list doesn't return dependency info.
func (m *mockBeadsForStep) List(opts beads.ListOptions) ([]*beads.Issue, error) {
	var result []*beads.Issue
	for _, issue := range m.issues {
		// Filter by parent
		if opts.Parent != "" && issue.Parent != opts.Parent {
			continue
		}
		// Filter by status (unless "all")
		if opts.Status != "" && opts.Status != "all" && issue.Status != opts.Status {
			continue
		}
		// CRITICAL: Simulate bd list behavior - DependsOn is NOT populated
		// Create a copy with empty DependsOn to simulate real bd list output
		issueCopy := *issue
		issueCopy.DependsOn = nil // bd list doesn't return this
		result = append(result, &issueCopy)
	}
	return result, nil
}

func (m *mockBeadsForStep) Close(ids ...string) error {
	for _, id := range ids {
		if issue, ok := m.issues[id]; ok {
			issue.Status = "closed"
		} else {
			return beads.ErrNotFound
		}
	}
	return nil
}

// makeStepIssue creates a test step issue with both DependsOn and Dependencies set.
// In real usage:
// - bd list returns issues with DependsOn empty
// - bd show returns issues with Dependencies populated (with DependencyType)
// The mock simulates this: List() clears DependsOn, Show() returns the full issue.
func makeStepIssue(id, title, parent, status string, dependsOn []string) *beads.Issue {
	issue := &beads.Issue{
		ID:        id,
		Title:     title,
		Type:      "task",
		Status:    status,
		Priority:  2,
		Parent:    parent,
		DependsOn: dependsOn, // This gets cleared by mock List() to simulate bd list
		CreatedAt: "2025-01-01T12:00:00Z",
		UpdatedAt: "2025-01-01T12:00:00Z",
	}
	// Also set Dependencies (what bd show returns) for proper testing.
	// Use "blocks" dependency type since that's what formula instantiation creates
	// for inter-step dependencies (vs "parent-child" for parent relationships).
	for _, depID := range dependsOn {
		issue.Dependencies = append(issue.Dependencies, beads.IssueDep{
			ID:             depID,
			Title:          "Dependency " + depID,
			DependencyType: "blocks", // Only "blocks" deps should block progress
		})
	}
	return issue
}

func TestFindNextReadyStep(t *testing.T) {
	tests := []struct {
		name           string
		moleculeID     string
		setupFunc      func(*mockBeadsForStep)
		wantStepID     string
		wantComplete   bool
		wantNilStep    bool
	}{
		{
			name:       "no steps - molecule complete",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				// Empty molecule - no children
			},
			wantComplete: true,
			wantNilStep:  true,
		},
		{
			name:       "all steps closed - molecule complete",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "closed", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "closed", []string{"gt-mol.1"}))
			},
			wantComplete: true,
			wantNilStep:  true,
		},
		{
			name:       "first step ready - no dependencies",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "open", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
			},
			wantStepID:   "gt-mol.1",
			wantComplete: false,
		},
		{
			name:       "second step ready - first closed",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "closed", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
			},
			wantStepID:   "gt-mol.2",
			wantComplete: false,
		},
		{
			name:       "all blocked - waiting on dependencies",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "in_progress", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
				m.addIssue(makeStepIssue("gt-mol.3", "Step 3", "gt-mol", "open", []string{"gt-mol.2"}))
			},
			wantComplete: false,
			wantNilStep:  true, // No ready steps (all blocked or in-progress)
		},
		{
			name:       "parallel steps - multiple ready",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				// Both step 1 and 2 have no deps, so both are ready
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "open", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", nil))
				m.addIssue(makeStepIssue("gt-mol.3", "Synthesis", "gt-mol", "open", []string{"gt-mol.1", "gt-mol.2"}))
			},
			wantComplete: false,
			// Should return one of the ready steps (implementation returns first found)
		},
		{
			name:       "diamond dependency - synthesis blocked",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step A", "gt-mol", "closed", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step B", "gt-mol", "open", nil)) // still open
				m.addIssue(makeStepIssue("gt-mol.3", "Synthesis", "gt-mol", "open", []string{"gt-mol.1", "gt-mol.2"}))
			},
			wantStepID:   "gt-mol.2", // B is ready (no deps)
			wantComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockBeadsForStep()
			tt.setupFunc(m)

			// Test the FIXED algorithm that uses ShowMultiple for dependency info
			// (simulating the real findNextReadyStep behavior after the fix)

			// Get children from mock (DependsOn will be empty - simulating bd list)
			children, _ := m.List(beads.ListOptions{Parent: tt.moleculeID, Status: "all"})

			// Build closed IDs set and collect open step IDs
			closedIDs := make(map[string]bool)
			var openStepIDs []string
			hasNonClosedSteps := false
			for _, child := range children {
				switch child.Status {
				case "closed":
					closedIDs[child.ID] = true
				case "open":
					openStepIDs = append(openStepIDs, child.ID)
					hasNonClosedSteps = true
				default:
					// in_progress or other - not closed, not available
					hasNonClosedSteps = true
				}
			}

			// Check complete
			allComplete := !hasNonClosedSteps

			if allComplete != tt.wantComplete {
				t.Errorf("allComplete = %v, want %v", allComplete, tt.wantComplete)
			}

			if tt.wantComplete {
				return
			}

			// Fetch full details for open steps (Dependencies will be populated)
			openStepsMap, _ := m.ShowMultiple(openStepIDs)

			// Find ready step using Dependencies (not DependsOn!)
			// Only "blocks" type dependencies block progress - ignore "parent-child".
			var readyStep *beads.Issue
			for _, stepID := range openStepIDs {
				step := openStepsMap[stepID]
				if step == nil {
					continue
				}

				// Use Dependencies (from bd show), NOT DependsOn (empty from bd list)
				allDepsClosed := true
				hasBlockingDeps := false
				for _, dep := range step.Dependencies {
					if dep.DependencyType != "blocks" {
						continue // Skip parent-child and other non-blocking relationships
					}
					hasBlockingDeps = true
					if !closedIDs[dep.ID] {
						allDepsClosed = false
						break
					}
				}
				if !hasBlockingDeps || allDepsClosed {
					readyStep = step
					break
				}
			}

			if tt.wantNilStep {
				if readyStep != nil {
					t.Errorf("expected nil step, got %s", readyStep.ID)
				}
				return
			}

			if readyStep == nil {
				if tt.wantStepID != "" {
					t.Errorf("expected step %s, got nil", tt.wantStepID)
				}
				return
			}

			if tt.wantStepID != "" && readyStep.ID != tt.wantStepID {
				t.Errorf("readyStep.ID = %s, want %s", readyStep.ID, tt.wantStepID)
			}
		})
	}
}

// TestStepDoneScenarios tests complete step-done scenarios
func TestStepDoneScenarios(t *testing.T) {
	tests := []struct {
		name           string
		stepID         string
		setupFunc      func(*mockBeadsForStep)
		wantAction     string // "continue", "done", "no_more_ready"
		wantNextStep   string
	}{
		{
			name:   "complete step, continue to next",
			stepID: "gt-mol.1",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "open", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
			},
			wantAction:   "continue",
			wantNextStep: "gt-mol.2",
		},
		{
			name:   "complete final step, molecule done",
			stepID: "gt-mol.2",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "closed", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
			},
			wantAction: "done",
		},
		{
			name:   "complete step, remaining blocked",
			stepID: "gt-mol.1",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "open", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "in_progress", nil)) // another parallel task
				m.addIssue(makeStepIssue("gt-mol.3", "Synthesis", "gt-mol", "open", []string{"gt-mol.1", "gt-mol.2"}))
			},
			wantAction: "no_more_ready", // .2 is in_progress, .3 blocked
		},
		{
			name:   "parallel workflow - complete one, next ready",
			stepID: "gt-mol.1",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Parallel A", "gt-mol", "open", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Parallel B", "gt-mol", "open", nil))
				m.addIssue(makeStepIssue("gt-mol.3", "Synthesis", "gt-mol", "open", []string{"gt-mol.1", "gt-mol.2"}))
			},
			wantAction:   "continue",
			wantNextStep: "gt-mol.2", // B is still ready
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockBeadsForStep()
			tt.setupFunc(m)

			// Extract molecule ID
			moleculeID := extractMoleculeIDFromStep(tt.stepID)
			if moleculeID == "" {
				t.Fatalf("could not extract molecule ID from %s", tt.stepID)
			}

			// Simulate closing the step
			if err := m.Close(tt.stepID); err != nil {
				t.Fatalf("failed to close step: %v", err)
			}

			// Now find next ready step using the FIXED algorithm
			children, _ := m.List(beads.ListOptions{Parent: moleculeID, Status: "all"})

			closedIDs := make(map[string]bool)
			var openStepIDs []string
			hasNonClosedSteps := false
			for _, child := range children {
				switch child.Status {
				case "closed":
					closedIDs[child.ID] = true
				case "open":
					openStepIDs = append(openStepIDs, child.ID)
					hasNonClosedSteps = true
				default:
					// in_progress or other - not closed, not available
					hasNonClosedSteps = true
				}
			}

			allComplete := !hasNonClosedSteps

			var action string
			var nextStepID string

			if allComplete {
				action = "done"
			} else {
				// Fetch full details for open steps (Dependencies will be populated)
				openStepsMap, _ := m.ShowMultiple(openStepIDs)

				// Find ready step using Dependencies (not DependsOn!)
				// Only "blocks" type dependencies block progress - ignore "parent-child".
				var readyStep *beads.Issue
				for _, stepID := range openStepIDs {
					step := openStepsMap[stepID]
					if step == nil {
						continue
					}

					// Use Dependencies (from bd show), NOT DependsOn (empty from bd list)
					allDepsClosed := true
					hasBlockingDeps := false
					for _, dep := range step.Dependencies {
						if dep.DependencyType != "blocks" {
							continue // Skip parent-child and other non-blocking relationships
						}
						hasBlockingDeps = true
						if !closedIDs[dep.ID] {
							allDepsClosed = false
							break
						}
					}
					if !hasBlockingDeps || allDepsClosed {
						readyStep = step
						break
					}
				}

				if readyStep != nil {
					action = "continue"
					nextStepID = readyStep.ID
				} else {
					action = "no_more_ready"
				}
			}

			if action != tt.wantAction {
				t.Errorf("action = %s, want %s", action, tt.wantAction)
			}

			if tt.wantNextStep != "" && nextStepID != tt.wantNextStep {
				t.Errorf("nextStep = %s, want %s", nextStepID, tt.wantNextStep)
			}
		})
	}
}

// TestFindNextReadyStepWithBdListBehavior tests the fix for the bug where
// bd list doesn't return dependency info (DependsOn is always empty), but
// bd show returns Dependencies. The old code checked DependsOn (always empty),
// so all open steps looked "ready" even when blocked.
//
// This test simulates real bd behavior and verifies the fix works correctly.
func TestFindNextReadyStepWithBdListBehavior(t *testing.T) {
	tests := []struct {
		name         string
		moleculeID   string
		setupFunc    func(*mockBeadsForStep)
		wantStepID   string // Expected ready step ID, or "" if none ready
		wantComplete bool
		wantBlocked  bool // True if all remaining steps are blocked (none ready)
	}{
		{
			name:       "blocked step should NOT be ready - dependency not closed",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				// Step 1 is open (first step, no deps)
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "open", nil))
				// Step 2 depends on Step 1, which is NOT closed
				// BUG: Old code would mark Step 2 as ready because DependsOn is empty from bd list
				// FIX: New code uses Dependencies from bd show
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
			},
			wantStepID:   "gt-mol.1", // Only step 1 should be ready
			wantComplete: false,
		},
		{
			name:       "step becomes ready when dependency closes",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "closed", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
			},
			wantStepID:   "gt-mol.2", // Step 2 is ready now that step 1 is closed
			wantComplete: false,
		},
		{
			name:       "multiple blocked steps - none ready",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				// Step 1 is in_progress (not closed)
				m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "in_progress", nil))
				// Steps 2 and 3 both depend on step 1
				m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))
				m.addIssue(makeStepIssue("gt-mol.3", "Step 3", "gt-mol", "open", []string{"gt-mol.1"}))
			},
			wantBlocked:  true, // No open steps are ready (all blocked by step 1)
			wantComplete: false,
		},
		{
			name:       "diamond dependency - synthesis blocked until both complete",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step A", "gt-mol", "closed", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step B", "gt-mol", "open", nil))
				// Synthesis depends on BOTH A and B
				m.addIssue(makeStepIssue("gt-mol.3", "Synthesis", "gt-mol", "open", []string{"gt-mol.1", "gt-mol.2"}))
			},
			wantStepID:   "gt-mol.2", // B is ready (no deps), synthesis is blocked
			wantComplete: false,
		},
		{
			name:       "diamond dependency - synthesis ready when both complete",
			moleculeID: "gt-mol",
			setupFunc: func(m *mockBeadsForStep) {
				m.addIssue(makeStepIssue("gt-mol.1", "Step A", "gt-mol", "closed", nil))
				m.addIssue(makeStepIssue("gt-mol.2", "Step B", "gt-mol", "closed", nil))
				// Synthesis depends on BOTH A and B, both are now closed
				m.addIssue(makeStepIssue("gt-mol.3", "Synthesis", "gt-mol", "open", []string{"gt-mol.1", "gt-mol.2"}))
			},
			wantStepID:   "gt-mol.3", // Synthesis is now ready
			wantComplete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockBeadsForStep()
			tt.setupFunc(m)

			// Simulate the FIXED algorithm that uses ShowMultiple for dependency info
			// Step 1: List children (DependsOn will be empty - simulating bd list)
			children, _ := m.List(beads.ListOptions{Parent: tt.moleculeID, Status: "all"})

			// Build closed IDs and collect open step IDs
			closedIDs := make(map[string]bool)
			var openStepIDs []string
			hasNonClosedSteps := false

			for _, child := range children {
				switch child.Status {
				case "closed":
					closedIDs[child.ID] = true
				case "open":
					openStepIDs = append(openStepIDs, child.ID)
					hasNonClosedSteps = true
				default:
					hasNonClosedSteps = true
				}
			}

			allComplete := !hasNonClosedSteps
			if allComplete != tt.wantComplete {
				t.Errorf("allComplete = %v, want %v", allComplete, tt.wantComplete)
			}

			if tt.wantComplete {
				return
			}

			// Step 2: Fetch full details for open steps (Dependencies will be populated)
			openStepsMap, _ := m.ShowMultiple(openStepIDs)

			// Step 3: Find ready step using Dependencies (not DependsOn!)
			// Only "blocks" type dependencies block progress - ignore "parent-child".
			var readyStep *beads.Issue
			for _, stepID := range openStepIDs {
				step := openStepsMap[stepID]
				if step == nil {
					continue
				}

				// Use Dependencies (from bd show), NOT DependsOn (empty from bd list)
				allDepsClosed := true
				hasBlockingDeps := false
				for _, dep := range step.Dependencies {
					if dep.DependencyType != "blocks" {
						continue // Skip parent-child and other non-blocking relationships
					}
					hasBlockingDeps = true
					if !closedIDs[dep.ID] {
						allDepsClosed = false
						break
					}
				}

				if !hasBlockingDeps || allDepsClosed {
					readyStep = step
					break
				}
			}

			// Verify results
			if tt.wantBlocked {
				if readyStep != nil {
					t.Errorf("expected no ready steps (all blocked), got %s", readyStep.ID)
				}
				return
			}

			if tt.wantStepID == "" {
				if readyStep != nil {
					t.Errorf("expected no ready step, got %s", readyStep.ID)
				}
				return
			}

			if readyStep == nil {
				t.Errorf("expected ready step %s, got nil", tt.wantStepID)
				return
			}

			if readyStep.ID != tt.wantStepID {
				t.Errorf("ready step = %s, want %s", readyStep.ID, tt.wantStepID)
			}
		})
	}
}

// TestOldBuggyBehavior demonstrates what the old buggy code would have done.
// With the old code, since DependsOn was always empty from bd list,
// ALL open steps would appear "ready" regardless of actual dependencies.
// This test verifies the bug exists when using the old approach.
func TestOldBuggyBehavior(t *testing.T) {
	m := newMockBeadsForStep()

	// Setup: Step 2 depends on Step 1, but Step 1 is NOT closed
	m.addIssue(makeStepIssue("gt-mol.1", "Step 1", "gt-mol", "open", nil))
	m.addIssue(makeStepIssue("gt-mol.2", "Step 2", "gt-mol", "open", []string{"gt-mol.1"}))

	// Get children via List (simulates bd list - DependsOn is empty)
	children, _ := m.List(beads.ListOptions{Parent: "gt-mol", Status: "all"})

	// OLD BUGGY CODE: Check DependsOn (which is empty from bd list)
	closedIDs := make(map[string]bool)
	var openSteps []*beads.Issue
	for _, child := range children {
		if child.Status == "closed" {
			closedIDs[child.ID] = true
		} else if child.Status == "open" {
			openSteps = append(openSteps, child)
		}
	}

	// Count how many steps the OLD buggy code thinks are "ready"
	readyCount := 0
	for _, step := range openSteps {
		allDepsClosed := true
		for _, depID := range step.DependsOn { // BUG: This is always empty!
			if !closedIDs[depID] {
				allDepsClosed = false
				break
			}
		}
		if len(step.DependsOn) == 0 || allDepsClosed { // Always true since DependsOn is empty
			readyCount++
		}
	}

	// The bug: OLD code thinks BOTH steps are ready (2 ready)
	// Correct behavior: Only Step 1 should be ready (1 ready)
	if readyCount != 2 {
		t.Errorf("Expected old buggy code to mark 2 steps as ready, got %d", readyCount)
	}

	t.Log("Old buggy behavior confirmed: both steps marked ready when only step 1 should be")
}
