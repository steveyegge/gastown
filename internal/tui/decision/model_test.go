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

// TestExtractContextFromDescription tests extracting context section from description
func TestExtractContextFromDescription(t *testing.T) {
	tests := []struct {
		name     string
		desc     string
		expected string
	}{
		{
			name: "with context section",
			desc: `## Question
What should we do?

## Context
This is the context section.
It has multiple lines.

## Options

### 1. Option A
Description`,
			expected: "This is the context section.\nIt has multiple lines.",
		},
		{
			name:     "no context section",
			desc:     "## Question\nWhat should we do?\n\n## Options\n### 1. A\nDesc",
			expected: "",
		},
		{
			name:     "empty context section",
			desc:     "## Context\n\n## Options",
			expected: "",
		},
		{
			name:     "context at end of description",
			desc:     "## Question\nQ\n\n## Context\nSome context here",
			expected: "Some context here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractContextFromDescription(tt.desc)
			if result != tt.expected {
				t.Errorf("extractContextFromDescription() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestExtractRequestedByFromDescription tests extracting requester from markdown footer
func TestExtractRequestedByFromDescription(t *testing.T) {
	tests := []struct {
		name     string
		desc     string
		expected string
	}{
		{
			name:     "standard format",
			desc:     "Some content\n---\n_Requested by: gastown/crew/decision_",
			expected: "gastown/crew/decision",
		},
		{
			name:     "with extra spaces",
			desc:     "_Requested by:   alice   _",
			expected: "alice",
		},
		{
			name:     "no requester",
			desc:     "Some content without requester",
			expected: "",
		},
		{
			name:     "malformed - no closing underscore",
			desc:     "_Requested by: bob",
			expected: "",
		},
		{
			name:     "overseer requester",
			desc:     "_Requested by: overseer_",
			expected: "overseer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRequestedByFromDescription(tt.desc)
			if result != tt.expected {
				t.Errorf("extractRequestedByFromDescription() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGetSessionName tests converting RequestedBy to tmux session name
func TestGetSessionName(t *testing.T) {
	tests := []struct {
		name        string
		requestedBy string
		wantSession string
		wantErr     bool
	}{
		{
			name:        "crew path",
			requestedBy: "gastown/crew/decision",
			wantSession: "gt-gastown-crew-decision",
			wantErr:     false,
		},
		{
			name:        "polecat path",
			requestedBy: "gastown/polecats/alpha",
			wantSession: "gt-gastown-polecats-alpha",
			wantErr:     false,
		},
		{
			name:        "overseer",
			requestedBy: "overseer",
			wantSession: "",
			wantErr:     true,
		},
		{
			name:        "human",
			requestedBy: "human",
			wantSession: "",
			wantErr:     true,
		},
		{
			name:        "empty",
			requestedBy: "",
			wantSession: "",
			wantErr:     true,
		},
		{
			name:        "single part",
			requestedBy: "something",
			wantSession: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session, err := getSessionName(tt.requestedBy)
			if (err != nil) != tt.wantErr {
				t.Errorf("getSessionName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if session != tt.wantSession {
				t.Errorf("getSessionName() = %q, want %q", session, tt.wantSession)
			}
		})
	}
}

// TestNewModel tests the model constructor
func TestNewModel(t *testing.T) {
	m := New()

	if m == nil {
		t.Fatal("New() returned nil")
	}

	// Check default values
	if m.filter != "all" {
		t.Errorf("default filter = %q, want %q", m.filter, "all")
	}

	if m.inputMode != ModeNormal {
		t.Errorf("default inputMode = %v, want ModeNormal", m.inputMode)
	}

	if m.selected != 0 {
		t.Errorf("default selected = %d, want 0", m.selected)
	}

	if m.selectedOption != 0 {
		t.Errorf("default selectedOption = %d, want 0", m.selectedOption)
	}

	if m.showHelp {
		t.Error("default showHelp should be false")
	}

	if m.peeking {
		t.Error("default peeking should be false")
	}

	if m.creatingCrew {
		t.Error("default creatingCrew should be false")
	}
}

// TestSetFilter tests the SetFilter method
func TestSetFilter(t *testing.T) {
	m := New()

	tests := []string{"all", "high", "medium", "low"}
	for _, filter := range tests {
		m.SetFilter(filter)
		if m.filter != filter {
			t.Errorf("SetFilter(%q): filter = %q, want %q", filter, m.filter, filter)
		}
	}
}

// TestSetNotify tests the SetNotify method
func TestSetNotify(t *testing.T) {
	m := New()

	if m.notify {
		t.Error("default notify should be false")
	}

	m.SetNotify(true)
	if !m.notify {
		t.Error("SetNotify(true): notify should be true")
	}

	m.SetNotify(false)
	if m.notify {
		t.Error("SetNotify(false): notify should be false")
	}
}

// TestSetWorkspace tests the SetWorkspace method
func TestSetWorkspace(t *testing.T) {
	m := New()

	m.SetWorkspace("/home/user/town", "gastown")

	if m.townRoot != "/home/user/town" {
		t.Errorf("townRoot = %q, want %q", m.townRoot, "/home/user/town")
	}
	if m.currentRig != "gastown" {
		t.Errorf("currentRig = %q, want %q", m.currentRig, "gastown")
	}
}

// TestFilterDecisions tests the filter functionality
func TestFilterDecisions(t *testing.T) {
	m := New()
	now := time.Now()

	decisions := []DecisionItem{
		{ID: "1", Urgency: "high", RequestedAt: now.Add(-1 * time.Hour)},
		{ID: "2", Urgency: "medium", RequestedAt: now},
		{ID: "3", Urgency: "low", RequestedAt: now.Add(-2 * time.Hour)},
	}

	t.Run("filter all", func(t *testing.T) {
		m.SetFilter("all")
		result := m.filterDecisions(decisions)
		if len(result) != 3 {
			t.Errorf("filter 'all': got %d decisions, want 3", len(result))
		}
	})

	t.Run("filter high", func(t *testing.T) {
		m.SetFilter("high")
		result := m.filterDecisions(decisions)
		if len(result) != 1 {
			t.Errorf("filter 'high': got %d decisions, want 1", len(result))
		}
		if result[0].ID != "1" {
			t.Errorf("filter 'high': got ID %s, want '1'", result[0].ID)
		}
	})

	t.Run("filter medium", func(t *testing.T) {
		m.SetFilter("medium")
		result := m.filterDecisions(decisions)
		if len(result) != 1 {
			t.Errorf("filter 'medium': got %d decisions, want 1", len(result))
		}
		if result[0].ID != "2" {
			t.Errorf("filter 'medium': got ID %s, want '2'", result[0].ID)
		}
	})

	t.Run("filter low", func(t *testing.T) {
		m.SetFilter("low")
		result := m.filterDecisions(decisions)
		if len(result) != 1 {
			t.Errorf("filter 'low': got %d decisions, want 1", len(result))
		}
		if result[0].ID != "3" {
			t.Errorf("filter 'low': got ID %s, want '3'", result[0].ID)
		}
	})
}

// TestDecisionItemUnmarshalJSON tests the custom JSON unmarshaling
func TestDecisionItemUnmarshalJSON(t *testing.T) {
	jsonData := `{
		"id": "hq-abc123",
		"title": "Choose an approach",
		"description": "## Question\nWhat approach?\n\n## Context\nSome context here.\n\n## Options\n\n### 1. Fast\nQuick solution\n\n### 2. Thorough\nComplete solution\n\n---\n_Requested by: gastown/crew/test_\n_Requested at: 2026-01-26T12:00:00Z_",
		"status": "open",
		"created_at": "2026-01-26T12:00:00Z",
		"created_by": "Agent",
		"labels": ["gt:decision", "urgency:high", "decision:pending"]
	}`

	var d DecisionItem
	err := json.Unmarshal([]byte(jsonData), &d)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if d.ID != "hq-abc123" {
		t.Errorf("ID = %q, want %q", d.ID, "hq-abc123")
	}

	if d.Prompt != "Choose an approach" {
		t.Errorf("Prompt = %q, want %q", d.Prompt, "Choose an approach")
	}

	if d.Urgency != "high" {
		t.Errorf("Urgency = %q, want %q", d.Urgency, "high")
	}

	if len(d.Options) != 2 {
		t.Fatalf("len(Options) = %d, want 2", len(d.Options))
	}

	if d.Options[0].Label != "Fast" {
		t.Errorf("Options[0].Label = %q, want %q", d.Options[0].Label, "Fast")
	}

	if d.RequestedBy != "gastown/crew/test" {
		t.Errorf("RequestedBy = %q, want %q", d.RequestedBy, "gastown/crew/test")
	}

	if d.Context != "Some context here." {
		t.Errorf("Context = %q, want %q", d.Context, "Some context here.")
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2026-01-26T12:00:00Z")
	if !d.RequestedAt.Equal(expectedTime) {
		t.Errorf("RequestedAt = %v, want %v", d.RequestedAt, expectedTime)
	}
}

// TestInputModeConstants tests that input mode constants are distinct
func TestInputModeConstants(t *testing.T) {
	if ModeNormal == ModeRationale {
		t.Error("ModeNormal should not equal ModeRationale")
	}
	if ModeNormal == ModeText {
		t.Error("ModeNormal should not equal ModeText")
	}
	if ModeRationale == ModeText {
		t.Error("ModeRationale should not equal ModeText")
	}
}
