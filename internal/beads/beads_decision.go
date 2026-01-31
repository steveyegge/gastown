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
	Question        string           `json:"question"`                    // The decision to be made
	Context         string           `json:"context,omitempty"`           // Background/analysis
	Options         []DecisionOption `json:"options"`                     // Available choices
	ChosenIndex     int              `json:"chosen_index"`                // Index of selected option (0 = pending, 1-indexed when resolved)
	Rationale       string           `json:"rationale,omitempty"`         // Why this choice was made
	RequestedBy     string           `json:"requested_by"`                // Agent that requested decision
	RequestedAt     string           `json:"requested_at"`                // When requested
	ResolvedBy      string           `json:"resolved_by,omitempty"`       // Who made the decision
	ResolvedAt      string           `json:"resolved_at,omitempty"`       // When resolved
	Urgency         string           `json:"urgency"`                     // high, medium, low
	Blockers        []string         `json:"blockers,omitempty"`          // Work IDs blocked by this decision
	PredecessorID   string           `json:"predecessor_id,omitempty"`    // Predecessor decision ID for chaining
	ParentBeadID    string           `json:"parent_bead_id,omitempty"`    // Parent bead ID (e.g., epic) for hierarchy
	ParentBeadTitle string           `json:"parent_bead_title,omitempty"` // Parent bead title for channel derivation
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

// BdDecisionCreateOption represents an option in bd decision create format.
type BdDecisionCreateOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

// CreateBdDecision creates a decision using bd decision create (canonical storage).
// This stores the decision in the decision_points table per the canonical design.
func (b *Beads) CreateBdDecision(fields *DecisionFields) (*Issue, error) {
	// Convert options to bd's JSON format with IDs
	var bdOptions []BdDecisionCreateOption
	for i, opt := range fields.Options {
		bdOpt := BdDecisionCreateOption{
			ID:          fmt.Sprintf("%d", i+1), // Use "1", "2", "3", etc. as IDs
			Label:       opt.Label,
			Description: opt.Description,
			Recommended: opt.Recommended,
		}
		bdOptions = append(bdOptions, bdOpt)
	}

	optionsJSON, err := json.Marshal(bdOptions)
	if err != nil {
		return nil, fmt.Errorf("marshaling options: %w", err)
	}

	// Build bd decision create command
	args := []string{"decision", "create", "--json",
		"--prompt=" + fields.Question,
		"--options=" + string(optionsJSON),
		"--no-daemon", // Use direct mode to avoid daemon issues
	}

	// Add urgency if specified (gt-7eew9)
	if fields.Urgency != "" {
		args = append(args, "--urgency="+fields.Urgency)
	}

	// Add context if provided
	if fields.Context != "" {
		args = append(args, "--context="+fields.Context)
	}

	// Add predecessor for decision chaining
	if fields.PredecessorID != "" {
		args = append(args, "--predecessor="+fields.PredecessorID)
	}

	// Add requested-by for wake notifications
	if fields.RequestedBy != "" {
		args = append(args, "--requested-by="+fields.RequestedBy)
	}

	// Add blocks dependency
	if len(fields.Blockers) > 0 {
		args = append(args, "--blocks="+fields.Blockers[0])
	}

	// Add parent for hierarchy
	if fields.ParentBeadID != "" {
		args = append(args, "--parent="+fields.ParentBeadID)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, fmt.Errorf("bd decision create: %w", err)
	}

	// Parse the JSON response
	var result struct {
		ID      string `json:"id"`
		Prompt  string `json:"prompt"`
		Urgency string `json:"urgency,omitempty"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parsing bd decision create output: %w", err)
	}

	return &Issue{ID: result.ID}, nil
}

// CreateDecisionBead creates a decision bead for tracking decisions.
// DEPRECATED: Use CreateBdDecision for new code (canonical storage).
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
// Priority: bd decisions (decision_points table) take precedence over gt decisions (markdown).
func (b *Beads) ResolveDecision(id string, chosenIndex int, rationale, resolvedBy string) error {
	// First check if it's a bd decision (decision_points table is canonical)
	// This takes priority because bd-created decisions now have both the gt:decision
	// label AND a decision_points record (hq-946577.39)
	bdDecision, bdErr := b.GetBdDecision(id)
	if bdErr == nil && bdDecision != nil && bdDecision.DecisionPoint != nil {
		return b.resolveBdDecision(bdDecision, id, chosenIndex, rationale, resolvedBy)
	}

	// Fall back to gt decision (markdown-based, legacy)
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Check if it's a gt decision (has gt:decision label)
	if HasLabel(issue, "gt:decision") {
		return b.resolveGtDecision(issue, id, chosenIndex, rationale, resolvedBy)
	}

	return fmt.Errorf("issue %s is not a decision (not in decision_points table and no gt:decision label)", id)
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

// ResolveDecisionWithCustomText resolves a decision with custom text response (no predefined option).
// This is used for the "Other" option where users provide their own response.
// It uses --accept-guidance to close the decision directly with the custom text.
func (b *Beads) ResolveDecisionWithCustomText(id, customText, resolvedBy string) error {
	if customText == "" {
		return fmt.Errorf("custom text is required for 'Other' responses")
	}

	// Build respond command with --accept-guidance to accept custom text as directive
	args := []string{"decision", "respond", id, "--text=" + customText, "--accept-guidance", "--no-daemon"}
	if resolvedBy != "" {
		args = append(args, "--by="+resolvedBy)
	}

	if _, err := b.run(args...); err != nil {
		return err
	}

	// Add implicit:custom_text label to mark this as a custom text resolution
	issue, err := b.Show(id)
	if err != nil {
		return nil // Decision is resolved, label failure is non-fatal
	}

	newLabels := append(issue.Labels, "implicit:custom_text")
	return b.Update(id, UpdateOptions{SetLabels: newLabels})
}

// GetDecisionBead retrieves a decision bead by ID.
// Returns nil if not found.
// Supports both gt decisions (with gt:decision label) and bd decisions (from decision_points table).
// Priority: bd decisions (decision_points table) take precedence (hq-946577.39).
func (b *Beads) GetDecisionBead(id string) (*Issue, *DecisionFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// First check decision_points table (canonical storage for bd-created decisions)
	bdDecision, bdErr := b.GetBdDecision(id)
	if bdErr == nil && bdDecision != nil && bdDecision.DecisionPoint != nil {
		// Convert bd decision to DecisionFields format
		fields := &DecisionFields{
			Question:      bdDecision.DecisionPoint.Prompt,
			Context:       bdDecision.DecisionPoint.Context,
			RequestedAt:   bdDecision.DecisionPoint.CreatedAt,
			RequestedBy:   bdDecision.DecisionPoint.RequestedBy,
			Urgency:       bdDecision.DecisionPoint.Urgency,
			PredecessorID: bdDecision.DecisionPoint.PriorID,
			ParentBeadID:  bdDecision.DecisionPoint.ParentBeadID,
			ChosenIndex:   0, // Default pending, will be updated if resolved
		}

		// Look up parent bead title for channel routing
		if fields.ParentBeadID != "" {
			parentIssue, parentErr := b.Show(fields.ParentBeadID)
			if parentErr == nil && parentIssue != nil {
				fields.ParentBeadTitle = parentIssue.Title
			}
		}

		// Parse options from bd decision
		// First try the top-level Options array (parsed by bd decision show)
		for _, opt := range bdDecision.Options {
			fields.Options = append(fields.Options, DecisionOption{
				Label:       opt.Label,
				Description: opt.Description,
			})
		}

		// Fallback: if Options is empty but raw options string exists, parse it
		// This handles cases where bd decision show doesn't populate the top-level Options
		if len(fields.Options) == 0 && bdDecision.DecisionPoint.Options != "" {
			var rawOptions []BdDecisionOption
			if err := json.Unmarshal([]byte(bdDecision.DecisionPoint.Options), &rawOptions); err == nil {
				for _, opt := range rawOptions {
					fields.Options = append(fields.Options, DecisionOption{
						Label:       opt.Label,
						Description: opt.Description,
					})
				}
			}
		}

		// Populate resolution fields if the decision has been resolved
		if bdDecision.DecisionPoint.SelectedOption != "" {
			// Find the chosen index by matching the option ID
			for i, opt := range bdDecision.Options {
				if opt.ID == bdDecision.DecisionPoint.SelectedOption {
					fields.ChosenIndex = i + 1 // 1-indexed
					break
				}
			}
			fields.ResolvedBy = bdDecision.DecisionPoint.RespondedBy
			fields.ResolvedAt = bdDecision.DecisionPoint.RespondedAt
			fields.Rationale = bdDecision.DecisionPoint.ResponseText
		}

		return issue, fields, nil
	}

	// Fall back to gt decision (markdown-based, legacy)
	if HasLabel(issue, "gt:decision") {
		fields := ParseDecisionFields(issue.Description)
		return issue, fields, nil
	}

	return nil, nil, fmt.Errorf("issue %s is not a decision bead (not in decision_points table and no gt:decision label)", id)
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
// This queries both gt-style decisions (using labels) and bd-style decisions
// (from the decision_points table), merging and deduplicating the results.
func (b *Beads) ListDecisions() ([]*Issue, error) {
	// Track seen IDs to deduplicate
	seen := make(map[string]bool)
	var allIssues []*Issue

	// First, query gt-style decisions using labels
	out, err := b.run("list", "--label=gt:decision", "--label=decision:pending", "--status=open", "--json")
	if err == nil {
		var gtIssues []*Issue
		if err := json.Unmarshal(out, &gtIssues); err == nil {
			for _, issue := range gtIssues {
				if !seen[issue.ID] {
					seen[issue.ID] = true
					allIssues = append(allIssues, issue)
				}
			}
		}
	}

	// Also query bd-style decisions from decision_points table
	bdIssues, _ := b.ListBdDecisions()
	for _, issue := range bdIssues {
		if !seen[issue.ID] {
			seen[issue.ID] = true
			allIssues = append(allIssues, issue)
		}
	}

	return allIssues, nil
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
	IssueID        string `json:"issue_id"`
	Prompt         string `json:"prompt"`
	Context        string `json:"context,omitempty"`         // Decision context (JSON)
	Options        string `json:"options"`                   // JSON string of options (raw)
	CreatedAt      string `json:"created_at"`
	SelectedOption string `json:"selected_option,omitempty"` // Option ID if resolved
	RespondedBy    string `json:"responded_by,omitempty"`    // Who resolved the decision
	RespondedAt    string `json:"responded_at,omitempty"`    // When resolved
	ResponseText   string `json:"response_text,omitempty"`   // Rationale/comment
	RequestedBy    string `json:"requested_by,omitempty"`    // Who requested the decision
	Urgency        string `json:"urgency,omitempty"`         // Urgency level
	PriorID        string `json:"prior_id,omitempty"`        // Predecessor decision ID
	ParentBeadID   string `json:"parent_bead_id,omitempty"`  // Parent bead ID (e.g., epic) for hierarchy
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
	RequestedBy   string             `json:"requested_by,omitempty"` // Who requested the decision (hq-orlqi0)
	Urgency       string             `json:"urgency,omitempty"`      // Urgency level (hq-orlqi0)
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

		// Add metadata footer for ParseDecisionFields to extract (hq-orlqi0)
		desc.WriteString("---\n")
		if dp.RequestedBy != "" {
			desc.WriteString(fmt.Sprintf("_Requested by: %s_\n", dp.RequestedBy))
		}
		if dp.Urgency != "" {
			desc.WriteString(fmt.Sprintf("_Urgency: %s_\n", dp.Urgency))
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

// ListPendingDecisionsForRequester returns pending decisions requested by a specific agent.
// Used to enforce single-decision-per-agent rule.
func (b *Beads) ListPendingDecisionsForRequester(requesterID string) ([]*Issue, error) {
	decisions, err := b.ListDecisions()
	if err != nil {
		return nil, err
	}

	var matching []*Issue
	for _, issue := range decisions {
		fields := ParseDecisionFields(issue.Description)
		if fields.RequestedBy == requesterID {
			matching = append(matching, issue)
		}
	}

	return matching, nil
}

// CloseDecisionAsSuperseded closes a pending decision because a new one was requested.
// Records the superseding decision ID in the close reason.
func (b *Beads) CloseDecisionAsSuperseded(decisionID, supersedingID string) error {
	reason := fmt.Sprintf("Superseded by %s", supersedingID)
	if err := b.CloseWithReason(reason, decisionID); err != nil {
		return err
	}

	// Update labels: remove decision:pending, add decision:superseded
	issue, err := b.Show(decisionID)
	if err != nil {
		return nil // Non-fatal, decision is already closed
	}

	newLabels := []string{}
	for _, label := range issue.Labels {
		if label != "decision:pending" {
			newLabels = append(newLabels, label)
		}
	}
	newLabels = append(newLabels, "decision:superseded")

	return b.Update(decisionID, UpdateOptions{SetLabels: newLabels})
}
