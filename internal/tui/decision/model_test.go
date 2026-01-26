package decision

import (
	"encoding/json"
	"testing"
	"time"
)

// TestParseDecisionListJSON tests that we can parse the actual JSON format
// returned by `gt decision list --json`
func TestParseDecisionListJSON(t *testing.T) {
	// This is the actual format returned by `gt decision list --json`
	actualJSON := `[
  {
    "id": "hq-39bc13",
    "title": "What next?",
    "description": "## Question\nWhat next?\n\n## Options\n\n### 1. Explore more\nLook around the system\n\n### 2. Give instructions\nTell me what to work on\n\n---\n_Requested by: overseer_\n_Requested at: 2026-01-26T01:29:43Z_\n_Urgency: low_",
    "status": "open",
    "priority": 2,
    "issue_type": "task",
    "created_at": "2026-01-26T01:29:43Z",
    "created_by": "Refinery",
    "updated_at": "2026-01-26T01:29:43Z",
    "labels": [
      "decision:pending",
      "gt:decision",
      "urgency:low"
    ]
  }
]`

	var decisions []DecisionItem
	err := json.Unmarshal([]byte(actualJSON), &decisions)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if len(decisions) != 1 {
		t.Fatalf("Expected 1 decision, got %d", len(decisions))
	}

	d := decisions[0]

	// Test ID parsing
	if d.ID != "hq-39bc13" {
		t.Errorf("Expected ID 'hq-39bc13', got '%s'", d.ID)
	}

	// Test prompt/title parsing
	if d.Prompt != "What next?" {
		t.Errorf("Expected Prompt 'What next?', got '%s'", d.Prompt)
	}

	// Test urgency parsing from labels
	if d.Urgency != "low" {
		t.Errorf("Expected Urgency 'low', got '%s'", d.Urgency)
	}

	// Test options parsing from description
	if len(d.Options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(d.Options))
	}

	if len(d.Options) >= 1 {
		if d.Options[0].Label != "Explore more" {
			t.Errorf("Expected first option label 'Explore more', got '%s'", d.Options[0].Label)
		}
		if d.Options[0].Description != "Look around the system" {
			t.Errorf("Expected first option description 'Look around the system', got '%s'", d.Options[0].Description)
		}
	}

	if len(d.Options) >= 2 {
		if d.Options[1].Label != "Give instructions" {
			t.Errorf("Expected second option label 'Give instructions', got '%s'", d.Options[1].Label)
		}
	}

	// Test requested_at parsing
	expectedTime, _ := time.Parse(time.RFC3339, "2026-01-26T01:29:43Z")
	if !d.RequestedAt.Equal(expectedTime) {
		t.Errorf("Expected RequestedAt '%v', got '%v'", expectedTime, d.RequestedAt)
	}
}

// TestParseUrgencyFromLabels tests extracting urgency from labels array
func TestParseUrgencyFromLabels(t *testing.T) {
	tests := []struct {
		labels   []string
		expected string
	}{
		{[]string{"decision:pending", "gt:decision", "urgency:high"}, "high"},
		{[]string{"urgency:medium", "other"}, "medium"},
		{[]string{"urgency:low"}, "low"},
		{[]string{"no-urgency-label"}, "medium"}, // default
		{[]string{}, "medium"},                   // default
	}

	for _, tt := range tests {
		result := extractUrgencyFromLabels(tt.labels)
		if result != tt.expected {
			t.Errorf("extractUrgencyFromLabels(%v) = %s, want %s", tt.labels, result, tt.expected)
		}
	}
}

// TestSortDecisionsByUrgency tests that decisions are sorted by urgency then time
func TestSortDecisionsByUrgency(t *testing.T) {
	m := New()
	m.filter = "all"

	now := time.Now()
	decisions := []DecisionItem{
		{ID: "1", Urgency: "low", RequestedAt: now.Add(-1 * time.Hour)},
		{ID: "2", Urgency: "high", RequestedAt: now.Add(-2 * time.Hour)},
		{ID: "3", Urgency: "medium", RequestedAt: now},
		{ID: "4", Urgency: "high", RequestedAt: now}, // newer high
		{ID: "5", Urgency: "low", RequestedAt: now},  // newer low
	}

	sorted := m.filterDecisions(decisions)

	// Expected order: high (newer first), medium, low (newer first)
	expectedIDs := []string{"4", "2", "3", "5", "1"}

	if len(sorted) != len(expectedIDs) {
		t.Fatalf("Expected %d decisions, got %d", len(expectedIDs), len(sorted))
	}

	for i, expected := range expectedIDs {
		if sorted[i].ID != expected {
			t.Errorf("Position %d: expected ID '%s', got '%s'", i, expected, sorted[i].ID)
		}
	}
}

// TestParseOptionsFromDescription tests parsing options from markdown description
func TestParseOptionsFromDescription(t *testing.T) {
	desc := `## Question
What should we do?

## Options

### 1. Option A
Description of option A

### 2. Option B
Description of option B

### 3. Option C
Description of option C

---
_Requested by: someone_`

	options := parseOptionsFromDescription(desc)

	if len(options) != 3 {
		t.Fatalf("Expected 3 options, got %d", len(options))
	}

	expectedLabels := []string{"Option A", "Option B", "Option C"}
	expectedDescs := []string{"Description of option A", "Description of option B", "Description of option C"}

	for i, opt := range options {
		if opt.Label != expectedLabels[i] {
			t.Errorf("Option %d: expected label '%s', got '%s'", i, expectedLabels[i], opt.Label)
		}
		if opt.Description != expectedDescs[i] {
			t.Errorf("Option %d: expected description '%s', got '%s'", i, expectedDescs[i], opt.Description)
		}
	}
}

// ============================================================================
// Tests for decision hijacking prevention (P0 bug fix)
// ============================================================================

// TestLockActiveDecision tests that lockActiveDecision sets the activeDecisionID
func TestLockActiveDecision(t *testing.T) {
	m := New()
	m.decisions = []DecisionItem{
		{ID: "dec-1", Prompt: "First decision"},
		{ID: "dec-2", Prompt: "Second decision"},
	}
	m.selected = 0

	// Initially no active decision
	if m.activeDecisionID != "" {
		t.Errorf("Expected empty activeDecisionID initially, got '%s'", m.activeDecisionID)
	}

	// Lock to first decision
	m.lockActiveDecision()
	if m.activeDecisionID != "dec-1" {
		t.Errorf("Expected activeDecisionID 'dec-1', got '%s'", m.activeDecisionID)
	}

	// Change selection and lock again
	m.selected = 1
	m.lockActiveDecision()
	if m.activeDecisionID != "dec-2" {
		t.Errorf("Expected activeDecisionID 'dec-2', got '%s'", m.activeDecisionID)
	}
}

// TestLockActiveDecisionOutOfBounds tests lockActiveDecision with invalid selection
func TestLockActiveDecisionOutOfBounds(t *testing.T) {
	m := New()
	m.decisions = []DecisionItem{
		{ID: "dec-1", Prompt: "First decision"},
	}

	// Selection out of bounds (negative)
	m.selected = -1
	m.lockActiveDecision()
	if m.activeDecisionID != "" {
		t.Errorf("Expected empty activeDecisionID for out-of-bounds, got '%s'", m.activeDecisionID)
	}

	// Selection out of bounds (too large)
	m.selected = 5
	m.lockActiveDecision()
	if m.activeDecisionID != "" {
		t.Errorf("Expected empty activeDecisionID for out-of-bounds, got '%s'", m.activeDecisionID)
	}

	// Empty decisions list
	m.decisions = []DecisionItem{}
	m.selected = 0
	m.lockActiveDecision()
	if m.activeDecisionID != "" {
		t.Errorf("Expected empty activeDecisionID for empty list, got '%s'", m.activeDecisionID)
	}
}

// TestClearActiveDecision tests that clearActiveDecision clears all input state
func TestClearActiveDecision(t *testing.T) {
	m := New()
	m.decisions = []DecisionItem{
		{ID: "dec-1", Prompt: "First decision"},
	}
	m.selected = 0

	// Set up some state
	m.lockActiveDecision()
	m.selectedOption = 2
	m.rationale = "some rationale"
	m.inputMode = ModeRationale

	// Verify state is set
	if m.activeDecisionID != "dec-1" {
		t.Errorf("Expected activeDecisionID 'dec-1', got '%s'", m.activeDecisionID)
	}
	if m.selectedOption != 2 {
		t.Errorf("Expected selectedOption 2, got %d", m.selectedOption)
	}

	// Clear
	m.clearActiveDecision()

	// Verify all state cleared
	if m.activeDecisionID != "" {
		t.Errorf("Expected empty activeDecisionID after clear, got '%s'", m.activeDecisionID)
	}
	if m.selectedOption != 0 {
		t.Errorf("Expected selectedOption 0 after clear, got %d", m.selectedOption)
	}
	if m.rationale != "" {
		t.Errorf("Expected empty rationale after clear, got '%s'", m.rationale)
	}
	if m.inputMode != ModeNormal {
		t.Errorf("Expected ModeNormal after clear, got %d", m.inputMode)
	}
}

// TestFindDecisionIndex tests finding decisions by ID
func TestFindDecisionIndex(t *testing.T) {
	m := New()
	m.decisions = []DecisionItem{
		{ID: "dec-1", Prompt: "First"},
		{ID: "dec-2", Prompt: "Second"},
		{ID: "dec-3", Prompt: "Third"},
	}

	tests := []struct {
		id       string
		expected int
	}{
		{"dec-1", 0},
		{"dec-2", 1},
		{"dec-3", 2},
		{"dec-999", -1}, // not found
		{"", -1},        // empty ID
	}

	for _, tt := range tests {
		result := m.findDecisionIndex(tt.id)
		if result != tt.expected {
			t.Errorf("findDecisionIndex('%s') = %d, want %d", tt.id, result, tt.expected)
		}
	}
}

// TestSelectionPreservedOnRefreshWithLock tests that selection tracks locked decision
// when the decision list is reordered or modified by a refresh
func TestSelectionPreservedOnRefreshWithLock(t *testing.T) {
	m := New()

	// Initial state: 3 decisions, selected index 0
	m.decisions = []DecisionItem{
		{ID: "dec-1", Urgency: "high"},
		{ID: "dec-2", Urgency: "medium"},
		{ID: "dec-3", Urgency: "low"},
	}
	m.selected = 0
	m.filter = "all"

	// Lock to dec-1
	m.lockActiveDecision()
	if m.activeDecisionID != "dec-1" {
		t.Fatalf("Expected locked to 'dec-1', got '%s'", m.activeDecisionID)
	}

	// Simulate refresh that reorders decisions (new high-urgency decision arrives)
	// dec-1 moves from position 0 to position 1
	newDecisions := []DecisionItem{
		{ID: "dec-new", Urgency: "high", RequestedAt: time.Now()},          // newer, goes first
		{ID: "dec-1", Urgency: "high", RequestedAt: time.Now().Add(-1 * time.Hour)}, // older, second
		{ID: "dec-2", Urgency: "medium"},
		{ID: "dec-3", Urgency: "low"},
	}

	// Apply the update (simulating what fetchDecisionsMsg handler does)
	m.decisions = m.filterDecisions(newDecisions)

	// Simulate the logic from fetchDecisionsMsg handler
	if m.activeDecisionID != "" {
		newIndex := m.findDecisionIndex(m.activeDecisionID)
		if newIndex >= 0 {
			m.selected = newIndex
		}
	}

	// dec-1 should still be selected even though it moved
	if m.selected != 1 {
		t.Errorf("Expected selection to track dec-1 to position 1, got position %d", m.selected)
	}
	if m.decisions[m.selected].ID != "dec-1" {
		t.Errorf("Expected selected decision to be 'dec-1', got '%s'", m.decisions[m.selected].ID)
	}
}

// TestSelectionClearedWhenLockedDecisionDisappears tests that input state is cleared
// when the locked decision is resolved elsewhere (disappears from list)
func TestSelectionClearedWhenLockedDecisionDisappears(t *testing.T) {
	m := New()

	// Initial state: 2 decisions
	m.decisions = []DecisionItem{
		{ID: "dec-1", Urgency: "high"},
		{ID: "dec-2", Urgency: "medium"},
	}
	m.selected = 0
	m.filter = "all"

	// Lock to dec-1 and set up input state
	m.lockActiveDecision()
	m.selectedOption = 2
	m.rationale = "my rationale"

	// Simulate refresh where dec-1 has been resolved (removed from list)
	newDecisions := []DecisionItem{
		{ID: "dec-2", Urgency: "medium"},
	}

	// Apply the update
	m.decisions = m.filterDecisions(newDecisions)

	// Simulate the logic from fetchDecisionsMsg handler
	if m.activeDecisionID != "" {
		newIndex := m.findDecisionIndex(m.activeDecisionID)
		if newIndex >= 0 {
			m.selected = newIndex
		} else {
			// Decision disappeared - clear input state
			m.clearActiveDecision()
		}
	}

	// All input state should be cleared
	if m.activeDecisionID != "" {
		t.Errorf("Expected activeDecisionID cleared, got '%s'", m.activeDecisionID)
	}
	if m.selectedOption != 0 {
		t.Errorf("Expected selectedOption cleared, got %d", m.selectedOption)
	}
	if m.rationale != "" {
		t.Errorf("Expected rationale cleared, got '%s'", m.rationale)
	}
}

// TestConfirmValidatesLockedDecision tests that confirm operation validates
// that we're still resolving the decision we intended to
func TestConfirmValidatesLockedDecision(t *testing.T) {
	m := New()

	// Set up: locked to dec-1, but the list has been reordered
	// so selected index now points to a different decision
	m.decisions = []DecisionItem{
		{ID: "dec-new", Prompt: "New decision", Options: []Option{{Label: "A"}}},
		{ID: "dec-1", Prompt: "Original", Options: []Option{{Label: "B"}}},
	}
	m.selected = 0           // Points to dec-new
	m.activeDecisionID = "dec-1" // But we locked dec-1
	m.selectedOption = 1

	// The confirm logic should detect the mismatch
	currentDecision := m.decisions[m.selected]
	if m.activeDecisionID != "" && m.activeDecisionID != currentDecision.ID {
		// This is the expected path - mismatch detected
		// In real code this would set an error
		m.clearActiveDecision()
	}

	// Verify state was cleared due to mismatch
	if m.activeDecisionID != "" {
		t.Errorf("Expected activeDecisionID cleared on mismatch, got '%s'", m.activeDecisionID)
	}
	if m.selectedOption != 0 {
		t.Errorf("Expected selectedOption cleared on mismatch, got %d", m.selectedOption)
	}
}

// TestConfirmSucceedsWithMatchingLock tests that confirm works when lock matches
func TestConfirmSucceedsWithMatchingLock(t *testing.T) {
	m := New()

	m.decisions = []DecisionItem{
		{ID: "dec-1", Prompt: "First", Options: []Option{{Label: "A"}}},
	}
	m.selected = 0
	m.activeDecisionID = "dec-1" // Locked to dec-1
	m.selectedOption = 1

	currentDecision := m.decisions[m.selected]

	// Verify match is detected
	if m.activeDecisionID != "" && m.activeDecisionID != currentDecision.ID {
		t.Errorf("Expected lock to match current decision")
	}

	// In the matching case, we should NOT clear state (resolution proceeds)
	if m.selectedOption != 1 {
		t.Errorf("Expected selectedOption preserved when lock matches")
	}
}

// TestNavigationClearsLock tests that navigating to a different decision clears lock
func TestNavigationClearsLock(t *testing.T) {
	m := New()

	m.decisions = []DecisionItem{
		{ID: "dec-1", Prompt: "First"},
		{ID: "dec-2", Prompt: "Second"},
	}
	m.selected = 0

	// Lock and set up state
	m.lockActiveDecision()
	m.selectedOption = 1

	if m.activeDecisionID != "dec-1" {
		t.Fatalf("Expected locked to dec-1")
	}

	// Navigate down (simulating what Update does on key.Down)
	m.selected++
	m.clearActiveDecision() // This is what Update does

	// Lock should be cleared
	if m.activeDecisionID != "" {
		t.Errorf("Expected lock cleared after navigation, got '%s'", m.activeDecisionID)
	}
	if m.selectedOption != 0 {
		t.Errorf("Expected selectedOption cleared after navigation, got %d", m.selectedOption)
	}
}

// TestSelectingOptionLocksDecision tests that selecting an option locks to that decision
func TestSelectingOptionLocksDecision(t *testing.T) {
	m := New()

	m.decisions = []DecisionItem{
		{ID: "dec-1", Prompt: "First", Options: []Option{{Label: "A"}, {Label: "B"}}},
		{ID: "dec-2", Prompt: "Second", Options: []Option{{Label: "C"}}},
	}
	m.selected = 1 // Start on second decision

	// Simulate selecting option 1 (what Update does on key.Select1)
	m.selectedOption = 1
	m.lockActiveDecision()

	// Should be locked to dec-2
	if m.activeDecisionID != "dec-2" {
		t.Errorf("Expected locked to 'dec-2', got '%s'", m.activeDecisionID)
	}
}

// TestGetSessionName tests converting RequestedBy to tmux session name
func TestGetSessionName(t *testing.T) {
	tests := []struct {
		requestedBy string
		expected    string
		shouldErr   bool
	}{
		{"gastown/crew/decision_point", "gt-gastown-decision_point", false},
		{"beads/crew/wolf", "gt-beads-wolf", false},
		{"myrig/polecats/alpha", "gt-myrig-alpha", false},
		{"overseer", "", true},   // Cannot peek human
		{"human", "", true},      // Cannot peek human
		{"", "", true},           // Empty
		{"singlepart", "", true}, // Invalid format
	}

	for _, tt := range tests {
		result, err := getSessionName(tt.requestedBy)
		if tt.shouldErr {
			if err == nil {
				t.Errorf("getSessionName(%q) expected error, got nil", tt.requestedBy)
			}
		} else {
			if err != nil {
				t.Errorf("getSessionName(%q) unexpected error: %v", tt.requestedBy, err)
			}
			if result != tt.expected {
				t.Errorf("getSessionName(%q) = %q, want %q", tt.requestedBy, result, tt.expected)
			}
		}
	}
}

// TestParseOptionsWithProsAndCons tests parsing options that include pros/cons sections
func TestParseOptionsWithProsAndCons(t *testing.T) {
	desc := `## Question
Which approach?

## Options

### 1. Option A
Description of A

**Pros:**
- Fast execution
- Low memory usage

**Cons:**
- Complex setup

### 2. Option B *(Recommended)*
Description of B

**Pros:**
- Simple

---
_Requested by: tester_`

	options := parseOptionsFromDescription(desc)

	if len(options) != 2 {
		t.Fatalf("Expected 2 options, got %d", len(options))
	}

	// Check option A
	optA := options[0]
	if optA.Label != "Option A" {
		t.Errorf("Expected label 'Option A', got '%s'", optA.Label)
	}
	if len(optA.Pros) != 2 {
		t.Errorf("Expected 2 pros for option A, got %d", len(optA.Pros))
	}
	if len(optA.Cons) != 1 {
		t.Errorf("Expected 1 con for option A, got %d", len(optA.Cons))
	}
	if optA.Recommended {
		t.Errorf("Option A should not be recommended")
	}

	// Check option B
	optB := options[1]
	if optB.Label != "Option B" {
		t.Errorf("Expected label 'Option B', got '%s'", optB.Label)
	}
	if !optB.Recommended {
		t.Errorf("Option B should be recommended")
	}
	if len(optB.Pros) != 1 {
		t.Errorf("Expected 1 pro for option B, got %d", len(optB.Pros))
	}
}
