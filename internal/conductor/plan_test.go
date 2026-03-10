package conductor

import (
	"testing"
)

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Add user profile page", "add-user-profile-page"},
		{"Fix Bug #123", "fix-bug-123"},
		{"hello--world", "hello-world"},
		{"  spaces  ", "spaces"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "specialchars"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSlugify_LongTitle(t *testing.T) {
	long := "this is a very long title that should be truncated to fifty characters maximum length"
	got := slugify(long)
	if len(got) > 50 {
		t.Errorf("slugify truncated to %d chars, want <= 50", len(got))
	}
}

func TestGeneratePlan_RequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input PlanInput
	}{
		{"missing bead ID", PlanInput{Title: "test", RigName: "rig"}},
		{"missing title", PlanInput{BeadID: "gt-abc", RigName: "rig"}},
		{"missing rig", PlanInput{BeadID: "gt-abc", Title: "test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GeneratePlan(tt.input)
			if err == nil {
				t.Error("expected error for missing required field")
			}
		})
	}
}

func TestGeneratePlan_Basic(t *testing.T) {
	input := PlanInput{
		BeadID:      "gt-abc123",
		Title:       "Add user profile",
		RigName:     "gastown",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	if plan.ParentBeadID != "gt-abc123" {
		t.Errorf("ParentBeadID = %q, want %q", plan.ParentBeadID, "gt-abc123")
	}
	if plan.FeatureName != "add-user-profile" {
		t.Errorf("FeatureName = %q, want %q", plan.FeatureName, "add-user-profile")
	}
	if plan.IntegrationBranch != "integration/add-user-profile" {
		t.Errorf("IntegrationBranch = %q", plan.IntegrationBranch)
	}
	if plan.RigName != "gastown" {
		t.Errorf("RigName = %q", plan.RigName)
	}
}

func TestGeneratePlan_SubBeadCount(t *testing.T) {
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "gastown",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	// Phase 1 (examine): 0 sub-beads (handled via mail)
	// Phase 2 (harden): 1
	// Phase 3 (modernize): 1
	// Phase 4 (specify): 1
	// Phase 5 (implement): 2 (frontend + backend, excludes tests/security/docs)
	// Phase 6 (secure): 1
	// Phase 7 (document): 1
	// Total: 7
	if len(plan.SubBeads) != 7 {
		t.Errorf("SubBead count = %d, want 7", len(plan.SubBeads))
		for _, sb := range plan.SubBeads {
			t.Logf("  phase=%s specialty=%s branch=%s", sb.PhaseName, sb.Specialty, sb.Branch)
		}
	}
}

func TestGeneratePlan_NoExamineSubBead(t *testing.T) {
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "gastown",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	for _, sb := range plan.SubBeads {
		if sb.Phase == PhaseExamine {
			t.Error("should not have a sub-bead for examine phase")
		}
	}
}

func TestGeneratePlan_BranchNaming(t *testing.T) {
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "User Profile",
		RigName:     "gastown",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	expectedBranches := map[string]bool{
		"user-profile/harden":    true,
		"user-profile/modernize": true,
		"user-profile/specify":   true,
		"user-profile/frontend":  true,
		"user-profile/backend":   true,
		"user-profile/security":  true,
		"user-profile/docs":      true,
	}

	for _, sb := range plan.SubBeads {
		if !expectedBranches[sb.Branch] {
			t.Errorf("unexpected branch %q", sb.Branch)
		}
		delete(expectedBranches, sb.Branch)
	}

	for branch := range expectedBranches {
		t.Errorf("missing expected branch %q", branch)
	}
}

func TestGeneratePlan_Dependencies(t *testing.T) {
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "gastown",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	deps := make(map[string][]string)
	for _, sb := range plan.SubBeads {
		deps[sb.Branch] = sb.DependsOn
	}

	// Harden has no deps
	if len(deps["feature/harden"]) != 0 {
		t.Errorf("harden should have no deps, got %v", deps["feature/harden"])
	}

	// Modernize depends on harden
	if len(deps["feature/modernize"]) != 1 || deps["feature/modernize"][0] != "feature/harden" {
		t.Errorf("modernize deps = %v, want [feature/harden]", deps["feature/modernize"])
	}

	// Specify depends on modernize
	if len(deps["feature/specify"]) != 1 || deps["feature/specify"][0] != "feature/modernize" {
		t.Errorf("specify deps = %v, want [feature/modernize]", deps["feature/specify"])
	}

	// Implement specialties depend on specify
	for _, sb := range plan.SubBeads {
		if sb.Phase == PhaseImplement {
			if len(sb.DependsOn) != 1 || sb.DependsOn[0] != "feature/specify" {
				t.Errorf("implement %s deps = %v, want [feature/specify]", sb.Specialty, sb.DependsOn)
			}
		}
	}

	// Secure depends on all implement branches
	if len(deps["feature/security"]) != 2 {
		t.Errorf("secure deps count = %d, want 2 (frontend + backend)", len(deps["feature/security"]))
	}

	// Docs depends on security
	if len(deps["feature/docs"]) != 1 || deps["feature/docs"][0] != "feature/security" {
		t.Errorf("docs deps = %v, want [feature/security]", deps["feature/docs"])
	}
}

func TestGeneratePlan_SpecifyHasUserGateLabel(t *testing.T) {
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "gastown",
		Specialties: []string{"backend", "tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	for _, sb := range plan.SubBeads {
		if sb.Phase == PhaseSpecify {
			hasGateLabel := false
			for _, l := range sb.Labels {
				if l == "user-gate" {
					hasGateLabel = true
				}
			}
			if !hasGateLabel {
				t.Error("specify sub-bead missing user-gate label")
			}
		}
	}
}

func TestGeneratePlan_OnlyBackend(t *testing.T) {
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "gastown",
		Specialties: []string{"backend", "tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	// Should have 6 sub-beads (no frontend implement)
	if len(plan.SubBeads) != 6 {
		t.Errorf("SubBead count = %d, want 6", len(plan.SubBeads))
	}

	// Only one implement sub-bead
	implCount := 0
	for _, sb := range plan.SubBeads {
		if sb.Phase == PhaseImplement {
			implCount++
			if sb.Specialty != "backend" {
				t.Errorf("implement specialty = %q, want backend", sb.Specialty)
			}
		}
	}
	if implCount != 1 {
		t.Errorf("implement sub-bead count = %d, want 1", implCount)
	}
}

func TestGeneratePlan_NoImplSpecialties(t *testing.T) {
	// Only non-implementation specialties
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Feature",
		RigName:     "gastown",
		Specialties: []string{"tests", "security", "docs"},
	}

	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan() error: %v", err)
	}

	// Should still get an implement sub-bead (generic backend fallback)
	implCount := 0
	for _, sb := range plan.SubBeads {
		if sb.Phase == PhaseImplement {
			implCount++
		}
	}
	if implCount != 1 {
		t.Errorf("implement sub-bead count = %d, want 1 (fallback)", implCount)
	}
}

func TestImplementationSpecialties(t *testing.T) {
	all := []string{"frontend", "backend", "tests", "security", "docs"}
	got := implementationSpecialties(all)
	if len(got) != 2 {
		t.Fatalf("implementationSpecialties returned %d, want 2", len(got))
	}
	if got[0] != "frontend" || got[1] != "backend" {
		t.Errorf("got %v, want [frontend, backend]", got)
	}
}

func TestBestModernizeSpecialty(t *testing.T) {
	tests := []struct {
		specs []string
		want  string
	}{
		{[]string{"frontend", "backend"}, "backend"},
		{[]string{"frontend"}, "frontend"},
		{[]string{"tests", "docs"}, "tests"},
		{nil, "backend"},
	}
	for _, tt := range tests {
		got := bestModernizeSpecialty(tt.specs)
		if got != tt.want {
			t.Errorf("bestModernizeSpecialty(%v) = %q, want %q", tt.specs, got, tt.want)
		}
	}
}
