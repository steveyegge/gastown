package templates

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Section 16: Posting selection prompt in priming templates
// Tests 16.1–16.5 from gt-4g0
// ---------------------------------------------------------------------------

// TestPostingSelectionPrompt_16_1_MayorContainsPrompt verifies that the mayor
// priming template contains the posting selection prompt block.
func TestPostingSelectionPrompt_16_1_MayorContainsPrompt(t *testing.T) {
	t.Parallel()
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:     "mayor",
		TownRoot: "/test/town",
		TownName: "town",
		WorkDir:  "/test/town",
	}

	output, err := tmpl.RenderRole("mayor", data)
	if err != nil {
		t.Fatalf("RenderRole(mayor) error = %v", err)
	}

	if !strings.Contains(output, "Posting Selection") {
		t.Error("mayor priming should contain 'Posting Selection' block")
	}
}

// TestPostingSelectionPrompt_16_2_CrewContainsPrompt verifies that the crew
// priming template contains the posting selection prompt block.
func TestPostingSelectionPrompt_16_2_CrewContainsPrompt(t *testing.T) {
	t.Parallel()
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:    "crew",
		RigName: "testrig",
		Polecat: "mel",
		WorkDir: "/test/town/testrig/crew/mel",
	}

	output, err := tmpl.RenderRole("crew", data)
	if err != nil {
		t.Fatalf("RenderRole(crew) error = %v", err)
	}

	if !strings.Contains(output, "Posting Selection") {
		t.Error("crew priming should contain 'Posting Selection' block")
	}
}

// TestPostingSelectionPrompt_16_3_PolecatDoesNotContainPrompt verifies that
// the polecat priming template does NOT contain the posting selection prompt.
// Polecats don't sling work, so they shouldn't see this prompt.
func TestPostingSelectionPrompt_16_3_PolecatDoesNotContainPrompt(t *testing.T) {
	t.Parallel()
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:    "polecat",
		RigName: "testrig",
		Polecat: "alpha",
		WorkDir: "/test/town/testrig/polecats/alpha",
	}

	output, err := tmpl.RenderRole("polecat", data)
	if err != nil {
		t.Fatalf("RenderRole(polecat) error = %v", err)
	}

	if strings.Contains(output, "Posting Selection") {
		t.Error("polecat priming should NOT contain 'Posting Selection' block")
	}
}

// TestPostingSelectionPrompt_16_4_MayorReferencesPostingList verifies that
// the mayor posting selection prompt references `gt posting list`.
func TestPostingSelectionPrompt_16_4_MayorReferencesPostingList(t *testing.T) {
	t.Parallel()
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:     "mayor",
		TownRoot: "/test/town",
		TownName: "town",
		WorkDir:  "/test/town",
	}

	output, err := tmpl.RenderRole("mayor", data)
	if err != nil {
		t.Fatalf("RenderRole(mayor) error = %v", err)
	}

	if !strings.Contains(output, "posting list") {
		t.Error("mayor posting selection prompt should reference 'posting list' command")
	}
}

// TestPostingSelectionPrompt_16_5_MayorContainsPostingFlag verifies that
// the mayor posting selection prompt contains `--posting` flag examples.
func TestPostingSelectionPrompt_16_5_MayorContainsPostingFlag(t *testing.T) {
	t.Parallel()
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	data := RoleData{
		Role:     "mayor",
		TownRoot: "/test/town",
		TownName: "town",
		WorkDir:  "/test/town",
	}

	output, err := tmpl.RenderRole("mayor", data)
	if err != nil {
		t.Fatalf("RenderRole(mayor) error = %v", err)
	}

	if !strings.Contains(output, "--posting") {
		t.Error("mayor posting selection prompt should contain '--posting' flag examples")
	}
}

// ---------------------------------------------------------------------------
// Section 16b: Posting self-assumption guidance in priming templates
// Tests 16.6–16.16 from gt-p2i
// ---------------------------------------------------------------------------

// helper to render a role template
func renderRole(t *testing.T, role string, data RoleData) string {
	t.Helper()
	tmpl, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	output, err := tmpl.RenderRole(role, data)
	if err != nil {
		t.Fatalf("RenderRole(%s) error = %v", role, err)
	}
	return output
}

var crewData = RoleData{
	Role:    "crew",
	RigName: "testrig",
	Polecat: "mel",
	WorkDir: "/test/town/testrig/crew/mel",
}

var polecatData = RoleData{
	Role:    "polecat",
	RigName: "testrig",
	Polecat: "alpha",
	WorkDir: "/test/town/testrig/polecats/alpha",
}

var mayorData = RoleData{
	Role:     "mayor",
	TownRoot: "/test/town",
	TownName: "town",
	WorkDir:  "/test/town",
}

// TestPostingSelfAssumption_16_6_CrewHasGuidance verifies that the crew
// priming template contains posting self-assumption guidance.
func TestPostingSelfAssumption_16_6_CrewHasGuidance(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "crew", crewData)
	if !strings.Contains(output, "Posting Self-Assumption") {
		t.Error("crew priming should contain 'Posting Self-Assumption' section")
	}
}

// TestPostingSelfAssumption_16_7_PolecatHasGuidance verifies that the polecat
// priming template contains posting self-assumption guidance.
func TestPostingSelfAssumption_16_7_PolecatHasGuidance(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "polecat", polecatData)
	if !strings.Contains(output, "Posting Self-Assumption") {
		t.Error("polecat priming should contain 'Posting Self-Assumption' section")
	}
}

// TestPostingSelfAssumption_16_8_MayorDoesNotHaveGuidance verifies that the
// mayor priming template does NOT contain posting self-assumption guidance.
func TestPostingSelfAssumption_16_8_MayorDoesNotHaveGuidance(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "mayor", mayorData)
	if strings.Contains(output, "Posting Self-Assumption") {
		t.Error("mayor priming should NOT contain 'Posting Self-Assumption' section")
	}
}

// TestPostingSelfAssumption_16_9_CrewMentionsAssumeWithReason verifies that
// crew self-assumption guidance mentions `gt posting assume --reason`.
func TestPostingSelfAssumption_16_9_CrewMentionsAssumeWithReason(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "crew", crewData)
	if !strings.Contains(output, "posting assume") || !strings.Contains(output, "--reason") {
		t.Error("crew self-assumption guidance should mention 'posting assume' with '--reason'")
	}
}

// TestPostingSelfAssumption_16_10_CrewMentionsDrop verifies that crew
// self-assumption guidance mentions `gt posting drop`.
func TestPostingSelfAssumption_16_10_CrewMentionsDrop(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "crew", crewData)
	if !strings.Contains(output, "posting drop") {
		t.Error("crew self-assumption guidance should mention 'posting drop'")
	}
}

// TestPostingSelfAssumption_16_11_CrewMentionsCommunicateToMayor verifies that
// crew self-assumption guidance mentions communicating the assumption to mayor.
func TestPostingSelfAssumption_16_11_CrewMentionsCommunicateToMayor(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "crew", crewData)
	if !strings.Contains(output, "mayor") {
		t.Error("crew self-assumption guidance should mention communicating to mayor")
	}
}

// TestPostingSelfAssumption_16_12_PolecatMentionsAssumeWithReason verifies that
// polecat self-assumption guidance mentions `gt posting assume --reason`.
func TestPostingSelfAssumption_16_12_PolecatMentionsAssumeWithReason(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "polecat", polecatData)
	if !strings.Contains(output, "posting assume") || !strings.Contains(output, "--reason") {
		t.Error("polecat self-assumption guidance should mention 'posting assume' with '--reason'")
	}
}

// TestPostingSelfAssumption_16_13_PolecatMentionsPartialBead verifies that
// polecat self-assumption guidance mentions partial-bead scenarios.
func TestPostingSelfAssumption_16_13_PolecatMentionsPartialBead(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "polecat", polecatData)
	if !strings.Contains(output, "artial-bead") {
		t.Error("polecat self-assumption guidance should mention partial-bead scenarios")
	}
}

// TestPostingSelfAssumption_16_14_PolecatNotesHandoffPreservesPosting verifies
// that polecat self-assumption guidance notes handoff preserves posting.
func TestPostingSelfAssumption_16_14_PolecatNotesHandoffPreservesPosting(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "polecat", polecatData)
	if !strings.Contains(output, "andoff preserves posting") {
		t.Error("polecat self-assumption guidance should note that handoff preserves posting")
	}
}

// TestPostingSelfAssumption_16_15_CrewReferencesPostingList verifies that
// crew self-assumption guidance references `gt posting list`.
func TestPostingSelfAssumption_16_15_CrewReferencesPostingList(t *testing.T) {
	t.Parallel()
	output := renderRole(t, "crew", crewData)
	// The self-assumption section should reference posting list for discovery
	if !strings.Contains(output, "posting list") {
		t.Error("crew self-assumption guidance should reference 'posting list' command")
	}
}

// TestPostingSelfAssumption_16_16_BothMentionDefaultNoPosting verifies that
// both crew and polecat self-assumption guidance mention that default (no
// posting) is correct for general-purpose work.
func TestPostingSelfAssumption_16_16_BothMentionDefaultNoPosting(t *testing.T) {
	t.Parallel()

	crewOutput := renderRole(t, "crew", crewData)
	polecatOutput := renderRole(t, "polecat", polecatData)

	for _, tc := range []struct {
		name   string
		output string
	}{
		{"crew", crewOutput},
		{"polecat", polecatOutput},
	} {
		// Check that both mention default/no posting is correct for general work
		if !strings.Contains(tc.output, "no posting") || !strings.Contains(tc.output, "general-purpose") {
			t.Errorf("%s self-assumption guidance should mention default (no posting) is correct for general-purpose work", tc.name)
		}
	}
}
