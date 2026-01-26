// Package beads provides decision bead management.
package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DecisionOption represents a single option in a decision request.
type DecisionOption struct {
	Label       string   `json:"label"`                 // Short label (e.g., "JWT tokens")
	Description string   `json:"description,omitempty"` // Explanation of this choice
	Pros        []string `json:"pros,omitempty"`        // Advantages of this option
	Cons        []string `json:"cons,omitempty"`        // Disadvantages of this option
	Recommended bool     `json:"recommended,omitempty"` // Mark as recommended option
}

// DecisionFields holds structured fields for decision beads.
// These are stored as structured data in the description.
type DecisionFields struct {
	Question                string           `json:"question"`                          // The decision to be made
	Context                 string           `json:"context,omitempty"`                 // Brief background context
	Analysis                string           `json:"analysis,omitempty"`                // Detailed analysis of the situation
	Tradeoffs               string           `json:"tradeoffs,omitempty"`               // General tradeoffs discussion
	RecommendationRationale string           `json:"recommendation_rationale,omitempty"` // Why the recommended option is suggested
	Options                 []DecisionOption `json:"options"`                           // Available choices
	ChosenIndex             int              `json:"chosen_index"`                      // Index of selected option (0 = pending, 1-indexed when resolved)
	Rationale               string           `json:"rationale,omitempty"`               // Why this choice was made
	RequestedBy             string           `json:"requested_by"`                      // Agent that requested decision
	RequestedAt             string           `json:"requested_at"`                      // When requested
	ResolvedBy              string           `json:"resolved_by,omitempty"`             // Who made the decision
	ResolvedAt              string           `json:"resolved_at,omitempty"`             // When resolved
	Urgency                 string           `json:"urgency"`                           // high, medium, low
	Blockers                []string         `json:"blockers,omitempty"`                // Work IDs blocked by this decision
}

// DecisionState constants for decision status tracking.
const (
	DecisionPending  = "pending"
	DecisionResolved = "resolved"
)

// Urgency level constants
const (
	UrgencyHigh   = "high"
	UrgencyMedium = "medium"
	UrgencyLow    = "low"
)

// IsValidUrgency checks if an urgency level is valid.
func IsValidUrgency(urgency string) bool {
	switch urgency {
	case UrgencyHigh, UrgencyMedium, UrgencyLow:
		return true
	default:
		return false
	}
}

// FormatDecisionDescription creates a markdown description from decision fields.
func FormatDecisionDescription(fields *DecisionFields) string {
	if fields == nil {
		return ""
	}

	var lines []string

	// Question section
	lines = append(lines, "## Question")
	lines = append(lines, fields.Question)
	lines = append(lines, "")

	// Context section (if provided)
	if fields.Context != "" {
		lines = append(lines, "## Context")
		lines = append(lines, fields.Context)
		lines = append(lines, "")
	}

	// Analysis section (if provided)
	if fields.Analysis != "" {
		lines = append(lines, "## Analysis")
		lines = append(lines, fields.Analysis)
		lines = append(lines, "")
	}

	// Tradeoffs section (if provided)
	if fields.Tradeoffs != "" {
		lines = append(lines, "## Tradeoffs")
		lines = append(lines, fields.Tradeoffs)
		lines = append(lines, "")
	}

	// Options section
	lines = append(lines, "## Options")
	lines = append(lines, "")
	for i, opt := range fields.Options {
		num := i + 1
		chosenMarker := ""
		if fields.ChosenIndex == num {
			chosenMarker = " **[CHOSEN]**"
		}
		recommendedMarker := ""
		if opt.Recommended {
			recommendedMarker = " *(Recommended)*"
		}
		lines = append(lines, fmt.Sprintf("### %d. %s%s%s", num, opt.Label, recommendedMarker, chosenMarker))
		if opt.Description != "" {
			lines = append(lines, opt.Description)
		}
		// Add pros if present
		if len(opt.Pros) > 0 {
			lines = append(lines, "")
			lines = append(lines, "**Pros:**")
			for _, pro := range opt.Pros {
				lines = append(lines, fmt.Sprintf("- %s", pro))
			}
		}
		// Add cons if present
		if len(opt.Cons) > 0 {
			lines = append(lines, "")
			lines = append(lines, "**Cons:**")
			for _, con := range opt.Cons {
				lines = append(lines, fmt.Sprintf("- %s", con))
			}
		}
		lines = append(lines, "")
	}

	// Recommendation rationale (if provided)
	if fields.RecommendationRationale != "" {
		lines = append(lines, "## Recommendation")
		lines = append(lines, fields.RecommendationRationale)
		lines = append(lines, "")
	}

	// Resolution section (if resolved)
	if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
		lines = append(lines, "---")
		lines = append(lines, "## Resolution")
		lines = append(lines, fmt.Sprintf("**Chosen:** %s", fields.Options[fields.ChosenIndex-1].Label))
		if fields.Rationale != "" {
			lines = append(lines, fmt.Sprintf("**Rationale:** %s", fields.Rationale))
		}
		lines = append(lines, fmt.Sprintf("**Resolved by:** %s", fields.ResolvedBy))
		lines = append(lines, fmt.Sprintf("**Resolved at:** %s", fields.ResolvedAt))
		lines = append(lines, "")
	}

	// Metadata footer
	lines = append(lines, "---")
	lines = append(lines, fmt.Sprintf("_Requested by: %s_", fields.RequestedBy))
	lines = append(lines, fmt.Sprintf("_Requested at: %s_", fields.RequestedAt))
	lines = append(lines, fmt.Sprintf("_Urgency: %s_", fields.Urgency))
	if len(fields.Blockers) > 0 {
		lines = append(lines, fmt.Sprintf("_Blocking: %s_", strings.Join(fields.Blockers, ", ")))
	}

	return strings.Join(lines, "\n")
}

// ParseDecisionFields extracts decision fields from an issue's description.
// This parses the markdown format back into structured data.
func ParseDecisionFields(description string) *DecisionFields {
	fields := &DecisionFields{
		ChosenIndex: 0, // 0 = pending
	}

	lines := strings.Split(description, "\n")
	var currentSection string
	var optionIndex int
	var currentOption *DecisionOption
	var currentSubsection string // Track Pros/Cons within options

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect section headers
		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimPrefix(line, "## ")
			currentSubsection = ""
			continue
		}

		// Parse option headers
		if strings.HasPrefix(line, "### ") {
			if currentOption != nil && optionIndex > 0 {
				fields.Options = append(fields.Options, *currentOption)
			}
			optionIndex++
			currentOption = &DecisionOption{}
			currentSubsection = ""

			// Parse option label (remove number prefix and markers)
			optLine := strings.TrimPrefix(line, "### ")
			if idx := strings.Index(optLine, ". "); idx != -1 {
				optLine = optLine[idx+2:]
			}
			// Check for recommended marker
			if strings.Contains(optLine, "*(Recommended)*") {
				currentOption.Recommended = true
				optLine = strings.Replace(optLine, " *(Recommended)*", "", 1)
			}
			// Check for chosen marker
			if strings.Contains(optLine, "**[CHOSEN]**") {
				fields.ChosenIndex = optionIndex
				optLine = strings.Replace(optLine, " **[CHOSEN]**", "", 1)
			}
			currentOption.Label = strings.TrimSpace(optLine)
			continue
		}

		// Detect subsections within options (Pros/Cons)
		if currentSection == "Options" && currentOption != nil {
			if line == "**Pros:**" {
				currentSubsection = "Pros"
				continue
			} else if line == "**Cons:**" {
				currentSubsection = "Cons"
				continue
			}
		}

		// Parse section content
		switch currentSection {
		case "Question":
			if line != "" && !strings.HasPrefix(line, "#") {
				if fields.Question != "" {
					fields.Question += " "
				}
				fields.Question += line
			}

		case "Context":
			if line != "" && !strings.HasPrefix(line, "#") && line != "---" {
				if fields.Context != "" {
					fields.Context += "\n"
				}
				fields.Context += line
			}

		case "Analysis":
			if line != "" && !strings.HasPrefix(line, "#") && line != "---" {
				if fields.Analysis != "" {
					fields.Analysis += "\n"
				}
				fields.Analysis += line
			}

		case "Tradeoffs":
			if line != "" && !strings.HasPrefix(line, "#") && line != "---" {
				if fields.Tradeoffs != "" {
					fields.Tradeoffs += "\n"
				}
				fields.Tradeoffs += line
			}

		case "Recommendation":
			if line != "" && !strings.HasPrefix(line, "#") && line != "---" {
				if fields.RecommendationRationale != "" {
					fields.RecommendationRationale += "\n"
				}
				fields.RecommendationRationale += line
			}

		case "Options":
			if currentOption != nil && line != "" && !strings.HasPrefix(line, "#") && line != "---" {
				// Check if it's a list item for pros/cons
				if strings.HasPrefix(line, "- ") {
					item := strings.TrimPrefix(line, "- ")
					switch currentSubsection {
					case "Pros":
						currentOption.Pros = append(currentOption.Pros, item)
					case "Cons":
						currentOption.Cons = append(currentOption.Cons, item)
					default:
						// Regular description line that happens to start with -
						if currentOption.Description != "" {
							currentOption.Description += " "
						}
						currentOption.Description += line
					}
				} else if currentSubsection == "" {
					// Regular description content
					if currentOption.Description != "" {
						currentOption.Description += " "
					}
					currentOption.Description += line
				}
			}

		case "Resolution":
			if strings.HasPrefix(line, "**Chosen:**") {
				// Already parsed from option markers
			} else if strings.HasPrefix(line, "**Rationale:**") {
				fields.Rationale = strings.TrimPrefix(line, "**Rationale:** ")
			} else if strings.HasPrefix(line, "**Resolved by:**") {
				fields.ResolvedBy = strings.TrimPrefix(line, "**Resolved by:** ")
			} else if strings.HasPrefix(line, "**Resolved at:**") {
				fields.ResolvedAt = strings.TrimPrefix(line, "**Resolved at:** ")
			}
		}

		// Parse metadata footer
		if strings.HasPrefix(line, "_Requested by:") {
			fields.RequestedBy = strings.TrimSuffix(strings.TrimPrefix(line, "_Requested by: "), "_")
		} else if strings.HasPrefix(line, "_Requested at:") {
			fields.RequestedAt = strings.TrimSuffix(strings.TrimPrefix(line, "_Requested at: "), "_")
		} else if strings.HasPrefix(line, "_Urgency:") {
			fields.Urgency = strings.TrimSuffix(strings.TrimPrefix(line, "_Urgency: "), "_")
		} else if strings.HasPrefix(line, "_Blocking:") {
			blockers := strings.TrimSuffix(strings.TrimPrefix(line, "_Blocking: "), "_")
			fields.Blockers = strings.Split(blockers, ", ")
		}
	}

	// Add last option
	if currentOption != nil && optionIndex > 0 {
		fields.Options = append(fields.Options, *currentOption)
	}

	return fields
}

// CreateDecisionBead creates a decision bead for tracking decisions.
func (b *Beads) CreateDecisionBead(title string, fields *DecisionFields) (*Issue, error) {
	description := FormatDecisionDescription(fields)

	args := []string{"create", "--json",
		"--title=" + title,
		"--description=" + description,
		"--type=task",
		"--labels=gt:decision",
		"--labels=decision:pending",
	}

	// Add urgency as a label for easy filtering
	if fields != nil && fields.Urgency != "" {
		args = append(args, fmt.Sprintf("--labels=urgency:%s", fields.Urgency))
	}

	// Default actor from BD_ACTOR env var for provenance tracking
	if actor := b.getActor(); actor != "" {
		args = append(args, "--actor="+actor)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parsing bd create output: %w", err)
	}

	return &issue, nil
}

// ResolveDecision marks a decision as resolved with the chosen option.
func (b *Beads) ResolveDecision(id string, chosenIndex int, rationale, resolvedBy string) error {
	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Verify it's a decision
	if !HasLabel(issue, "gt:decision") {
		return fmt.Errorf("issue %s is not a decision bead (missing gt:decision label)", id)
	}

	// Parse existing fields
	fields := ParseDecisionFields(issue.Description)

	// Validate choice
	if chosenIndex < 1 || chosenIndex > len(fields.Options) {
		return fmt.Errorf("invalid choice %d: must be between 1 and %d", chosenIndex, len(fields.Options))
	}

	// Update fields
	fields.ChosenIndex = chosenIndex
	fields.Rationale = rationale
	fields.ResolvedBy = resolvedBy
	fields.ResolvedAt = time.Now().Format(time.RFC3339)

	// Format new description
	description := FormatDecisionDescription(fields)

	// Update title to show resolution
	newTitle := issue.Title
	if !strings.Contains(newTitle, "[RESOLVED:") {
		newTitle = fmt.Sprintf("%s [RESOLVED: %s]", issue.Title, fields.Options[chosenIndex-1].Label)
	}

	// Update the bead
	if err := b.Update(id, UpdateOptions{
		Title:        &newTitle,
		Description:  &description,
		AddLabels:    []string{"decision:resolved"},
		RemoveLabels: []string{"decision:pending"},
	}); err != nil {
		return err
	}

	// Close the issue
	_, err = b.run("close", id, "--reason=Resolved: "+fields.Options[chosenIndex-1].Label)
	return err
}

// GetDecisionBead retrieves a decision bead by ID.
// Returns nil if not found.
func (b *Beads) GetDecisionBead(id string) (*Issue, *DecisionFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !HasLabel(issue, "gt:decision") {
		return nil, nil, fmt.Errorf("issue %s is not a decision bead (missing gt:decision label)", id)
	}

	fields := ParseDecisionFields(issue.Description)
	return issue, fields, nil
}

// ListDecisions returns all pending decision beads.
func (b *Beads) ListDecisions() ([]*Issue, error) {
	out, err := b.run("list", "--label=gt:decision", "--label=decision:pending", "--status=open", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	return issues, nil
}

// ListAllDecisions returns all decision beads (pending and resolved).
func (b *Beads) ListAllDecisions() ([]*Issue, error) {
	out, err := b.run("list", "--label=gt:decision", "--status=all", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	return issues, nil
}

// ListDecisionsByUrgency returns pending decision beads filtered by urgency.
func (b *Beads) ListDecisionsByUrgency(urgency string) ([]*Issue, error) {
	out, err := b.run("list",
		"--label=gt:decision",
		"--label=decision:pending",
		"--label=urgency:"+urgency,
		"--status=open",
		"--json",
	)
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	return issues, nil
}

// AddDecisionBlocker adds a blocker dependency to a decision.
// The blocked work will depend on this decision being resolved.
func (b *Beads) AddDecisionBlocker(decisionID, blockedWorkID string) error {
	return b.AddDependency(blockedWorkID, decisionID)
}

// RemoveDecisionBlocker removes a blocker dependency from a decision.
func (b *Beads) RemoveDecisionBlocker(decisionID, blockedWorkID string) error {
	return b.RemoveDependency(blockedWorkID, decisionID)
}

// ListStaleDecisions returns pending decisions older than the given threshold.
func (b *Beads) ListStaleDecisions(threshold time.Duration) ([]*Issue, error) {
	// Get all pending decisions
	decisions, err := b.ListDecisions()
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-threshold)
	var stale []*Issue

	for _, issue := range decisions {
		createdAt, err := time.Parse(time.RFC3339, issue.CreatedAt)
		if err != nil {
			continue
		}

		if createdAt.Before(cutoff) {
			stale = append(stale, issue)
		}
	}

	return stale, nil
}

// ListRecentlyResolvedDecisions returns decisions resolved within the given duration.
func (b *Beads) ListRecentlyResolvedDecisions(within time.Duration) ([]*Issue, error) {
	// Get all decisions
	decisions, err := b.ListAllDecisions()
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().Add(-within)
	var recent []*Issue

	for _, issue := range decisions {
		// Only resolved ones
		if !HasLabel(issue, "decision:resolved") {
			continue
		}

		// Check closed_at timestamp
		if issue.ClosedAt == "" {
			continue
		}
		closedAt, err := time.Parse(time.RFC3339, issue.ClosedAt)
		if err != nil {
			continue
		}

		if closedAt.After(cutoff) {
			recent = append(recent, issue)
		}
	}

	return recent, nil
}
