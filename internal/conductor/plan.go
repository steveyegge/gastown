package conductor

import (
	"fmt"
	"strings"
	"time"
)

// Plan represents the conductor's execution plan for a feature bead.
// It contains the branch strategy, sub-beads to create, and phase ordering.
type Plan struct {
	// ParentBeadID is the original high-level bead that triggered planning.
	ParentBeadID string `json:"parent_bead_id"`

	// FeatureName is derived from the bead title, used for branch naming.
	FeatureName string `json:"feature_name"`

	// IntegrationBranch is the parent branch all phase work merges into.
	IntegrationBranch string `json:"integration_branch"`

	// SubBeads are the planned sub-beads, one per phase/specialty combination.
	SubBeads []SubBead `json:"sub_beads"`

	// CreatedAt is when the plan was generated.
	CreatedAt time.Time `json:"created_at"`

	// RigName is the rig this plan targets.
	RigName string `json:"rig_name"`
}

// SubBead represents a planned sub-bead to be created and routed.
type SubBead struct {
	// Phase is which development phase this sub-bead belongs to.
	Phase Phase `json:"phase"`

	// PhaseName is the human-readable phase name.
	PhaseName string `json:"phase_name"`

	// Title is the sub-bead title.
	Title string `json:"title"`

	// Specialty is which artisan specialty should handle this.
	Specialty string `json:"specialty"`

	// Branch is the git branch name for this work.
	Branch string `json:"branch"`

	// DependsOn lists branch names this sub-bead depends on.
	DependsOn []string `json:"depends_on,omitempty"`

	// Labels are bead labels to apply.
	Labels []string `json:"labels"`

	// BeadID is populated after the sub-bead is created.
	BeadID string `json:"bead_id,omitempty"`
}

// PlanInput contains the information needed to generate a plan.
type PlanInput struct {
	// BeadID is the parent bead being planned.
	BeadID string

	// Title is the bead title.
	Title string

	// Description is the full bead description.
	Description string

	// RigName is the target rig.
	RigName string

	// Specialties are the available artisan specialties for routing.
	Specialties []string

	// ArchitectReport is the Phase 1 examination output (optional, may be empty
	// if planning happens before or concurrently with examination).
	ArchitectReport string
}

// GeneratePlan creates an execution plan from a bead and available specialties.
// The plan follows the 7-phase methodology with appropriate branch naming.
func GeneratePlan(input PlanInput) (*Plan, error) {
	if input.BeadID == "" {
		return nil, fmt.Errorf("bead ID is required")
	}
	if input.Title == "" {
		return nil, fmt.Errorf("bead title is required")
	}
	if input.RigName == "" {
		return nil, fmt.Errorf("rig name is required")
	}

	featureName := slugify(input.Title)
	integrationBranch := fmt.Sprintf("integration/%s", featureName)

	plan := &Plan{
		ParentBeadID:      input.BeadID,
		FeatureName:       featureName,
		IntegrationBranch: integrationBranch,
		CreatedAt:         time.Now(),
		RigName:           input.RigName,
	}

	phases := AllPhases()
	for _, phase := range phases {
		subBeads := planPhase(phase, featureName, integrationBranch, input.Specialties)
		plan.SubBeads = append(plan.SubBeads, subBeads...)
	}

	return plan, nil
}

// planPhase generates sub-beads for a single phase.
func planPhase(info PhaseInfo, featureName, integrationBranch string, specialties []string) []SubBead {
	switch info.Phase {
	case PhaseExamine:
		// Architect examines — no sub-bead needed (handled via mail consultation)
		return nil

	case PhaseHarden:
		return []SubBead{{
			Phase:     info.Phase,
			PhaseName: info.Name,
			Title:     fmt.Sprintf("[harden] Add test coverage for %s", featureName),
			Specialty: "tests",
			Branch:    fmt.Sprintf("%s/harden", featureName),
			Labels:    []string{"phase:harden", "tests"},
		}}

	case PhaseModernize:
		return []SubBead{{
			Phase:     info.Phase,
			PhaseName: info.Name,
			Title:     fmt.Sprintf("[modernize] Apply best practices to %s target area", featureName),
			Specialty: bestModernizeSpecialty(specialties),
			Branch:    fmt.Sprintf("%s/modernize", featureName),
			DependsOn: []string{fmt.Sprintf("%s/harden", featureName)},
			Labels:    []string{"phase:modernize"},
		}}

	case PhaseSpecify:
		return []SubBead{{
			Phase:     info.Phase,
			PhaseName: info.Name,
			Title:     fmt.Sprintf("[specify] Write failing specs for %s", featureName),
			Specialty: "tests",
			Branch:    fmt.Sprintf("%s/specify", featureName),
			DependsOn: []string{fmt.Sprintf("%s/modernize", featureName)},
			Labels:    []string{"phase:specify", "tests", "user-gate"},
		}}

	case PhaseImplement:
		// Create one sub-bead per implementation specialty
		return planImplementPhase(featureName, specialties)

	case PhaseSecure:
		return []SubBead{{
			Phase:     info.Phase,
			PhaseName: info.Name,
			Title:     fmt.Sprintf("[secure] Security review for %s", featureName),
			Specialty: "security",
			Branch:    fmt.Sprintf("%s/security", featureName),
			DependsOn: implementBranches(featureName, specialties),
			Labels:    []string{"phase:secure", "security"},
		}}

	case PhaseDocument:
		return []SubBead{{
			Phase:     info.Phase,
			PhaseName: info.Name,
			Title:     fmt.Sprintf("[docs] Document %s", featureName),
			Specialty: "docs",
			Branch:    fmt.Sprintf("%s/docs", featureName),
			DependsOn: []string{fmt.Sprintf("%s/security", featureName)},
			Labels:    []string{"phase:document", "docs"},
		}}
	}

	return nil
}

// planImplementPhase creates sub-beads for parallel specialty implementation.
func planImplementPhase(featureName string, specialties []string) []SubBead {
	specifyBranch := fmt.Sprintf("%s/specify", featureName)
	implSpecialties := implementationSpecialties(specialties)

	var beads []SubBead
	for _, spec := range implSpecialties {
		beads = append(beads, SubBead{
			Phase:     PhaseImplement,
			PhaseName: "implement",
			Title:     fmt.Sprintf("[implement] %s work for %s", spec, featureName),
			Specialty: spec,
			Branch:    fmt.Sprintf("%s/%s", featureName, spec),
			DependsOn: []string{specifyBranch},
			Labels:    []string{"phase:implement", spec},
		})
	}

	// If no implementation specialties, create a generic one
	if len(beads) == 0 {
		beads = append(beads, SubBead{
			Phase:     PhaseImplement,
			PhaseName: "implement",
			Title:     fmt.Sprintf("[implement] %s", featureName),
			Specialty: "backend",
			Branch:    fmt.Sprintf("%s/implement", featureName),
			DependsOn: []string{specifyBranch},
			Labels:    []string{"phase:implement"},
		})
	}

	return beads
}

// implementationSpecialties returns specialties that do implementation work
// (excludes tests, security, docs which have their own phases).
func implementationSpecialties(specialties []string) []string {
	excluded := map[string]bool{"tests": true, "security": true, "docs": true}
	var result []string
	for _, s := range specialties {
		if !excluded[s] {
			result = append(result, s)
		}
	}
	return result
}

// implementBranches returns branch names for all implementation sub-beads.
func implementBranches(featureName string, specialties []string) []string {
	implSpecs := implementationSpecialties(specialties)
	if len(implSpecs) == 0 {
		return []string{fmt.Sprintf("%s/implement", featureName)}
	}
	branches := make([]string, len(implSpecs))
	for i, spec := range implSpecs {
		branches[i] = fmt.Sprintf("%s/%s", featureName, spec)
	}
	return branches
}

// bestModernizeSpecialty picks the most appropriate specialty for modernization.
// Prefers backend for Go projects, frontend for JS/TS projects.
func bestModernizeSpecialty(specialties []string) string {
	for _, s := range specialties {
		if s == "backend" {
			return s
		}
	}
	for _, s := range specialties {
		if s == "frontend" {
			return s
		}
	}
	if len(specialties) > 0 {
		return specialties[0]
	}
	return "backend"
}

// slugify converts a title to a branch-safe slug.
func slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)

	// Collapse multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")

	// Limit length
	if len(s) > 50 {
		s = s[:50]
		s = strings.TrimRight(s, "-")
	}

	return s
}
