package beads

import (
	"strings"
	"testing"
)

func TestParseEpicFields(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expected    *EpicFields
	}{
		{
			name: "full fields",
			description: `Epic Title

epic_state: drafting
contributing_md: CONTRIBUTING.md
upstream_prs: https://github.com/test/repo/pull/1
integration_branch: integration/ep-123
subtask_count: 3
completed_count: 1`,
			expected: &EpicFields{
				EpicState:      EpicStateDrafting,
				ContributingMD: "CONTRIBUTING.md",
				UpstreamPRs:    "https://github.com/test/repo/pull/1",
				IntegrationBr:  "integration/ep-123",
				SubtaskCount:   3,
				CompletedCount: 1,
			},
		},
		{
			name: "minimal fields",
			description: `Epic Title

epic_state: ready`,
			expected: &EpicFields{
				EpicState: EpicStateReady,
			},
		},
		{
			name: "null values",
			description: `Epic Title

epic_state: in_progress
contributing_md: null
upstream_prs: null`,
			expected: &EpicFields{
				EpicState:      EpicStateInProgress,
				ContributingMD: "",
				UpstreamPRs:    "",
			},
		},
		{
			name:        "empty description",
			description: "",
			expected:    &EpicFields{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseEpicFields(tt.description)

			if result.EpicState != tt.expected.EpicState {
				t.Errorf("EpicState: expected '%s', got '%s'", tt.expected.EpicState, result.EpicState)
			}
			if result.ContributingMD != tt.expected.ContributingMD {
				t.Errorf("ContributingMD: expected '%s', got '%s'", tt.expected.ContributingMD, result.ContributingMD)
			}
			if result.UpstreamPRs != tt.expected.UpstreamPRs {
				t.Errorf("UpstreamPRs: expected '%s', got '%s'", tt.expected.UpstreamPRs, result.UpstreamPRs)
			}
			if result.IntegrationBr != tt.expected.IntegrationBr {
				t.Errorf("IntegrationBr: expected '%s', got '%s'", tt.expected.IntegrationBr, result.IntegrationBr)
			}
			if result.SubtaskCount != tt.expected.SubtaskCount {
				t.Errorf("SubtaskCount: expected %d, got %d", tt.expected.SubtaskCount, result.SubtaskCount)
			}
			if result.CompletedCount != tt.expected.CompletedCount {
				t.Errorf("CompletedCount: expected %d, got %d", tt.expected.CompletedCount, result.CompletedCount)
			}
		})
	}
}

func TestFormatEpicDescription(t *testing.T) {
	fields := &EpicFields{
		EpicState:      EpicStateDrafting,
		ContributingMD: "CONTRIBUTING.md",
		UpstreamPRs:    "https://github.com/test/repo/pull/1",
		IntegrationBr:  "integration/ep-123",
	}

	result := FormatEpicDescription("Test Epic", fields, "")

	// Check that all fields are present
	if !strings.Contains(result, "epic_state: drafting") {
		t.Error("missing epic_state field")
	}
	if !strings.Contains(result, "contributing_md: CONTRIBUTING.md") {
		t.Error("missing contributing_md field")
	}
	if !strings.Contains(result, "upstream_prs: https://github.com/test/repo/pull/1") {
		t.Error("missing upstream_prs field")
	}
	if !strings.Contains(result, "integration_branch: integration/ep-123") {
		t.Error("missing integration_branch field")
	}
}

func TestFormatEpicDescription_WithPlan(t *testing.T) {
	fields := &EpicFields{
		EpicState: EpicStateDrafting,
	}

	planContent := `## Step: implement
Do the implementation
Tier: opus

## Step: test
Write tests
Needs: implement`

	result := FormatEpicDescription("Test Epic", fields, planContent)

	// Check that plan content is included
	if !strings.Contains(result, "## Step: implement") {
		t.Error("missing plan content")
	}
	if !strings.Contains(result, "Needs: implement") {
		t.Error("missing dependencies in plan")
	}
}

func TestFormatEpicDescription_NilFields(t *testing.T) {
	result := FormatEpicDescription("Test Epic", nil, "")

	// Should still work with nil fields
	if !strings.Contains(result, "epic_state: drafting") {
		t.Error("should default to drafting state")
	}
}

func TestExtractPlanContent(t *testing.T) {
	description := `Test Epic

epic_state: drafting
contributing_md: CONTRIBUTING.md
upstream_prs: null

## Step: implement
Do the implementation
Tier: opus

## Step: test
Write tests
Needs: implement`

	result := ExtractPlanContent(description)

	// Should contain the plan steps
	if !strings.Contains(result, "## Step: implement") {
		t.Error("missing Step: implement")
	}
	if !strings.Contains(result, "## Step: test") {
		t.Error("missing Step: test")
	}
	if !strings.Contains(result, "Needs: implement") {
		t.Error("missing Needs declaration")
	}

	// Should NOT contain metadata fields
	if strings.Contains(result, "epic_state:") {
		t.Error("should not contain epic_state metadata")
	}
	if strings.Contains(result, "contributing_md:") {
		t.Error("should not contain contributing_md metadata")
	}
}

func TestValidEpicStateTransition(t *testing.T) {
	tests := []struct {
		from  EpicState
		to    EpicState
		valid bool
	}{
		// Valid transitions
		{EpicStateDrafting, EpicStateReady, true},
		{EpicStateDrafting, EpicStateClosed, true},
		{EpicStateReady, EpicStateInProgress, true},
		{EpicStateReady, EpicStateDrafting, true},
		{EpicStateInProgress, EpicStateReview, true},
		{EpicStateReview, EpicStateSubmitted, true},
		{EpicStateSubmitted, EpicStateLanded, true},
		{EpicStateLanded, EpicStateClosed, true},
		{EpicStateClosed, EpicStateDrafting, true}, // Reopen

		// Invalid transitions
		{EpicStateDrafting, EpicStateInProgress, false},
		{EpicStateDrafting, EpicStateSubmitted, false},
		{EpicStateReady, EpicStateSubmitted, false},
		{EpicStateLanded, EpicStateDrafting, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			result := ValidEpicStateTransition(tt.from, tt.to)
			if result != tt.valid {
				t.Errorf("ValidEpicStateTransition(%s, %s) = %v, expected %v",
					tt.from, tt.to, result, tt.valid)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{&testError{"not found"}, true},
		{&testError{"issue does not exist"}, true},
		{&testError{"no such issue"}, true},
		{&testError{"some other error"}, false},
	}

	for _, tt := range tests {
		result := IsNotFound(tt.err)
		if result != tt.expected {
			t.Errorf("IsNotFound(%v) = %v, expected %v", tt.err, result, tt.expected)
		}
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
