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
