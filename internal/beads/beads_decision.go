// Package beads provides decision bead management.
package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// maxTitleLength is the maximum length for issue titles in the database.
// Dolt schema uses VARCHAR(500), so we use 450 to leave room for resolution markers.
const maxTitleLength = 450

// truncateTitle truncates a title to fit within database constraints.
// If truncation is needed, it appends "..." to indicate the title was shortened.
func truncateTitle(title string, maxLen int) string {
	if len(title) <= maxLen {
		return title
	}
	// Leave room for "..."
	return title[:maxLen-3] + "..."
}

// DecisionOption represents a single option in a decision request.
type DecisionOption struct {
	Label       string `json:"label"`                 // Short label (e.g., "JWT tokens")
	Description string `json:"description,omitempty"` // Explanation of this choice
	Recommended bool   `json:"recommended,omitempty"` // Mark as recommended option
}

// DecisionFields holds structured fields for decision beads.
// These are stored as structured data in the description.
type DecisionFields struct {
	Question    string           `json:"question"`              // The decision to be made
	Context     string           `json:"context,omitempty"`     // Background/analysis
	Options     []DecisionOption `json:"options"`               // Available choices
	ChosenIndex int              `json:"chosen_index"`          // Index of selected option (0 = pending, 1-indexed when resolved)
	Rationale   string           `json:"rationale,omitempty"`   // Why this choice was made
	RequestedBy string           `json:"requested_by"`          // Agent that requested decision
	RequestedAt string           `json:"requested_at"`          // When requested
	ResolvedBy  string           `json:"resolved_by,omitempty"` // Who made the decision
	ResolvedAt  string           `json:"resolved_at,omitempty"` // When resolved
	Urgency     string           `json:"urgency"`               // high, medium, low
	Blockers    []string         `json:"blockers,omitempty"`    // Work IDs blocked by this decision
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

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect section headers
		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimPrefix(line, "## ")
			continue
		}

		// Parse option headers
		if strings.HasPrefix(line, "### ") {
			if currentOption != nil && optionIndex > 0 {
				fields.Options = append(fields.Options, *currentOption)
			}
			optionIndex++
			currentOption = &DecisionOption{}

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

		case "Options":
			if currentOption != nil && line != "" && !strings.HasPrefix(line, "#") && line != "---" {
				if currentOption.Description != "" {
					currentOption.Description += " "
				}
				currentOption.Description += line
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

	// Truncate title to fit database constraints (VARCHAR(500) in Dolt)
	// Use maxTitleLength (450) to leave room for resolution markers like [RESOLVED: ...]
	safeTitle := truncateTitle(title, maxTitleLength)

	args := []string{"create", "--json",
		"--title=" + safeTitle,
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
// Handles both gt decisions (with gt:decision label) and bd decisions (from decision_points table).
func (b *Beads) ResolveDecision(id string, chosenIndex int, rationale, resolvedBy string) error {
	// First get current issue to check what type of decision it is
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Check if it's a gt decision (has gt:decision label)
	if HasLabel(issue, "gt:decision") {
		return b.resolveGtDecision(issue, id, chosenIndex, rationale, resolvedBy)
	}

	// Try as a bd decision (from decision_points table)
	bdDecision, err := b.GetBdDecision(id)
	if err != nil || bdDecision == nil {
		return fmt.Errorf("issue %s is not a decision (no gt:decision label and not in decision_points)", id)
	}

	return b.resolveBdDecision(bdDecision, id, chosenIndex, rationale, resolvedBy)
}

// resolveGtDecision resolves a gt decision (with gt:decision label).
func (b *Beads) resolveGtDecision(issue *Issue, id string, chosenIndex int, rationale, resolvedBy string) error {
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
		chosenLabel := fields.Options[chosenIndex-1].Label
		newTitle = fmt.Sprintf("%s [RESOLVED: %s]", issue.Title, chosenLabel)
		// Truncate to fit database constraints (VARCHAR(500) in Dolt)
		// Use 500 here since we're at the final title, not creating new
		newTitle = truncateTitle(newTitle, 500)
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
	_, err := b.run("close", id, "--reason=Resolved: "+fields.Options[chosenIndex-1].Label)
	return err
}

// resolveBdDecision resolves a bd decision (from decision_points table).
func (b *Beads) resolveBdDecision(dp *BdDecisionPoint, id string, chosenIndex int, rationale, resolvedBy string) error {
	// Validate choice
	if chosenIndex < 1 || chosenIndex > len(dp.Options) {
		return fmt.Errorf("invalid choice %d: must be between 1 and %d", chosenIndex, len(dp.Options))
	}

	// Get the option ID (bd uses string IDs like "a", "b" or custom)
	optionID := dp.Options[chosenIndex-1].ID

	// Build respond command
	args := []string{"decision", "respond", id, "--select=" + optionID, "--no-daemon"}
	if rationale != "" {
		args = append(args, "--text="+rationale)
	}
	if resolvedBy != "" {
		args = append(args, "--by="+resolvedBy)
	}

	_, err := b.run(args...)
	return err
}

// GetDecisionBead retrieves a decision bead by ID.
// Returns nil if not found.
// Supports both gt decisions (with gt:decision label) and bd decisions (from decision_points table).
func (b *Beads) GetDecisionBead(id string) (*Issue, *DecisionFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// Check if it's a gt decision (has gt:decision label)
	if HasLabel(issue, "gt:decision") {
		fields := ParseDecisionFields(issue.Description)
		return issue, fields, nil
	}

	// Try to get it as a bd decision (from decision_points table)
	bdDecision, err := b.GetBdDecision(id)
	if err != nil || bdDecision == nil || bdDecision.DecisionPoint == nil {
		// Not found in decision_points either - not a decision
		return nil, nil, fmt.Errorf("issue %s is not a decision bead (missing gt:decision label)", id)
	}

	// Convert bd decision to DecisionFields format
	fields := &DecisionFields{
		Question:    bdDecision.DecisionPoint.Prompt,
		RequestedAt: bdDecision.DecisionPoint.CreatedAt,
		ChosenIndex: 0, // Pending
	}

	// Parse options from bd decision
	for _, opt := range bdDecision.Options {
		fields.Options = append(fields.Options, DecisionOption{
			Label:       opt.Label,
			Description: opt.Description,
		})
	}

	return issue, fields, nil
}

// GetBdDecision retrieves a decision from the decision_points table by issue ID.
func (b *Beads) GetBdDecision(id string) (*BdDecisionPoint, error) {
	out, err := b.run("decision", "show", id, "--json", "--no-daemon")
	if err != nil {
		return nil, err
	}

	var dp BdDecisionPoint
	if err := json.Unmarshal(out, &dp); err != nil {
		return nil, err
	}

	return &dp, nil
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

// BdDecisionOption represents an option in a bd decision
type BdDecisionOption struct {
	ID          string `json:"id"`
	Short       string `json:"short"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// BdDecisionPointData is the nested decision_point data from bd decision show
type BdDecisionPointData struct {
	IssueID   string `json:"issue_id"`
	Prompt    string `json:"prompt"`
	Options   string `json:"options"` // JSON string of options (raw)
	CreatedAt string `json:"created_at"`
}

// BdDecisionShowResponse represents the response from bd decision show
type BdDecisionShowResponse struct {
	ID            string               `json:"id"`
	DecisionPoint *BdDecisionPointData `json:"decision_point"`
	Options       []BdDecisionOption   `json:"options"` // Parsed options at top level
	Issue         *Issue               `json:"issue"`
}

// BdDecisionListItem represents a decision from bd decision list (flat format)
type BdDecisionListItem struct {
	IssueID       string             `json:"issue_id"`
	Prompt        string             `json:"prompt"`
	Options       string             `json:"options"` // JSON string of options (raw)
	OptionsParsed []BdDecisionOption `json:"options_parsed"`
	CreatedAt     string             `json:"created_at"`
	Issue         *Issue             `json:"issue"`
}

// BdDecisionPoint is an alias for BdDecisionShowResponse for backward compatibility
type BdDecisionPoint = BdDecisionShowResponse

// ListBdDecisions returns pending decisions from the decision_points table.
// These are decisions created with `bd decision create` (not `gt decision request`).
func (b *Beads) ListBdDecisions() ([]*Issue, error) {
	// Note: bd decision list shows pending by default (no --pending flag)
	// Use --no-daemon since daemon doesn't support Dolt backend
	out, err := b.run("decision", "list", "--json", "--no-daemon")
	if err != nil {
		// If bd decision list fails, return empty (not all beads DBs have decision_points)
		return nil, nil
	}

	var decisions []BdDecisionListItem
	if err := json.Unmarshal(out, &decisions); err != nil {
		return nil, nil // Return empty on parse error
	}

	// Convert to Issue format
	var issues []*Issue
	for _, dp := range decisions {
		if dp.Issue == nil {
			continue
		}
		// Skip if already closed
		if dp.Issue.Status == "closed" {
			continue
		}
		// Build description from options
		var desc strings.Builder
		desc.WriteString("## Question\n")
		desc.WriteString(dp.Prompt)
		desc.WriteString("\n\n## Options\n\n")
		for i, opt := range dp.OptionsParsed {
			desc.WriteString(fmt.Sprintf("### %d. %s\n", i+1, opt.Label))
			if opt.Description != "" {
				desc.WriteString(opt.Description)
				desc.WriteString("\n")
			}
			desc.WriteString("\n")
		}

		issue := dp.Issue
		issue.Description = desc.String()
		issues = append(issues, issue)
	}

	return issues, nil
}

// ListAllPendingDecisions returns decisions from both gt decision and bd decision.
func (b *Beads) ListAllPendingDecisions() ([]*Issue, error) {
	// Get gt decisions (beads with decision:pending label)
	gtDecisions, err := b.ListDecisions()
	if err != nil {
		gtDecisions = nil
	}

	// Get bd decisions (from decision_points table)
	bdDecisions, err := b.ListBdDecisions()
	if err != nil {
		bdDecisions = nil
	}

	// Merge, avoiding duplicates by ID
	seen := make(map[string]bool)
	var all []*Issue

	for _, d := range gtDecisions {
		if !seen[d.ID] {
			seen[d.ID] = true
			all = append(all, d)
		}
	}

	for _, d := range bdDecisions {
		if !seen[d.ID] {
			seen[d.ID] = true
			all = append(all, d)
		}
	}

	return all, nil
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
