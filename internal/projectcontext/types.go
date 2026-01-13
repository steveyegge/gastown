// Package projectcontext defines the structured representation of project-specific Claude context.
package projectcontext

import (
	"fmt"
	"strings"
)

// ProjectContext holds the structured, normalized project-specific context
// extracted from CLAUDE.md and the .claude/ directory.
// This object is immutable for the lifetime of a polecat and is stored
// in beads for crash recovery.
type ProjectContext struct {
	Guidelines   []string `json:"guidelines,omitempty"`
	Constraints  []string `json:"constraints,omitempty"`
	Rules        []string `json:"rules,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	Skills       []Skill  `json:"skills,omitempty"`
}

// Skill represents a normalized skill derived from a .claude/commands file.
// Skills are mapped to molecules or molecule fragments.
type Skill struct {
	Name        string   `json:"name"`
	Intent      string   `json:"intent,omitempty"`
	Steps       []string `json:"steps,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

// IsEmpty returns true if the ProjectContext contains no meaningful content.
func (p *ProjectContext) IsEmpty() bool {
	if p == nil {
		return true
	}
	return len(p.Guidelines) == 0 &&
		len(p.Constraints) == 0 &&
		len(p.Rules) == 0 &&
		len(p.Tools) == 0 &&
		len(p.Skills) == 0
}

// FormatForMolecule formats the ProjectContext for injection into molecule context.
// The output is structured markdown suitable for inclusion in polecat context via gt prime.
//
// Per the spec: Project context is interpreted once, converted to structured form,
// and injected into molecules. Polecats never directly read CLAUDE.md.
func (p *ProjectContext) FormatForMolecule() string {
	if p.IsEmpty() {
		return ""
	}

	var sections []string

	// Format guidelines
	if len(p.Guidelines) > 0 {
		var sb strings.Builder
		sb.WriteString("## Project Guidelines\n\n")
		sb.WriteString("The following project-specific guidelines apply to your work:\n\n")
		for _, g := range p.Guidelines {
			sb.WriteString(fmt.Sprintf("- %s\n", g))
		}
		sections = append(sections, sb.String())
	}

	// Format constraints
	if len(p.Constraints) > 0 {
		var sb strings.Builder
		sb.WriteString("## Project Constraints\n\n")
		sb.WriteString("The following constraints MUST be respected:\n\n")
		for _, c := range p.Constraints {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
		sections = append(sections, sb.String())
	}

	// Format rules
	if len(p.Rules) > 0 {
		var sb strings.Builder
		sb.WriteString("## Project Rules\n\n")
		for _, r := range p.Rules {
			sb.WriteString(fmt.Sprintf("- %s\n", r))
		}
		sections = append(sections, sb.String())
	}

	// Format available tools
	if len(p.Tools) > 0 {
		var sb strings.Builder
		sb.WriteString("## Available Project Tools\n\n")
		sb.WriteString("The project has configured the following tools:\n\n")
		for _, t := range p.Tools {
			sb.WriteString(fmt.Sprintf("- %s\n", t))
		}
		sections = append(sections, sb.String())
	}

	// Format skills as executable workflows
	if len(p.Skills) > 0 {
		var sb strings.Builder
		sb.WriteString("## Project Skills\n\n")
		sb.WriteString("The project defines the following executable skills:\n\n")
		for _, skill := range p.Skills {
			sb.WriteString(fmt.Sprintf("### /%s\n\n", skill.Name))
			if skill.Intent != "" {
				sb.WriteString(fmt.Sprintf("**Purpose**: %s\n\n", skill.Intent))
			}
			if len(skill.Steps) > 0 {
				sb.WriteString("**Steps**:\n")
				for i, step := range skill.Steps {
					sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
				}
				sb.WriteString("\n")
			}
			if len(skill.Constraints) > 0 {
				sb.WriteString("**Constraints**:\n")
				for _, c := range skill.Constraints {
					sb.WriteString(fmt.Sprintf("- %s\n", c))
				}
				sb.WriteString("\n")
			}
		}
		sections = append(sections, sb.String())
	}

	if len(sections) == 0 {
		return ""
	}

	// Wrap in a clear project context section
	header := "# Project-Specific Context\n\n"
	header += "The following context has been extracted from the project's Claude configuration.\n"
	header += "This context supplements Gas Town operational instructions.\n\n"

	return header + strings.Join(sections, "\n")
}

// FormatSkillAsMoleculeSteps converts a Skill into molecule-compatible step format.
// This allows project-defined skills to integrate with Gas Town's molecule system.
func (s *Skill) FormatSkillAsMoleculeSteps() string {
	if s == nil || len(s.Steps) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Skill: %s\n\n", s.Name))
	if s.Intent != "" {
		sb.WriteString(fmt.Sprintf("**Intent**: %s\n\n", s.Intent))
	}

	sb.WriteString("**Execution Steps**:\n\n")
	for i, step := range s.Steps {
		sb.WriteString(fmt.Sprintf("### Step %d\n\n", i+1))
		sb.WriteString(fmt.Sprintf("%s\n\n", step))
	}

	if len(s.Constraints) > 0 {
		sb.WriteString("**Skill Constraints**:\n\n")
		for _, c := range s.Constraints {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	return sb.String()
}
