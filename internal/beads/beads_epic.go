// Package beads provides epic bead management.
package beads

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EpicState represents the current state of an epic.
type EpicState string

// Epic state constants.
const (
	EpicStateDrafting   EpicState = "drafting"    // Initial planning phase
	EpicStateReady      EpicState = "ready"       // Plan finalized, ready to execute
	EpicStateInProgress EpicState = "in_progress" // Subtasks being worked
	EpicStateReview     EpicState = "review"      // All MRs merged, reviewing
	EpicStateSubmitted  EpicState = "submitted"   // Upstream PRs created
	EpicStateLanded     EpicState = "landed"      // All PRs merged upstream
	EpicStateClosed     EpicState = "closed"      // Abandoned or completed
)

// EpicFields holds structured fields for epic beads.
// These are stored as "key: value" lines in the description.
type EpicFields struct {
	EpicState      EpicState // drafting, ready, in_progress, review, submitted, landed, closed
	ContributingMD string    // Path to CONTRIBUTING.md (relative to rig)
	UpstreamPRs    string    // Comma-separated list of upstream PR URLs
	IntegrationBr  string    // Integration branch name
	SubtaskCount   int       // Number of subtasks
	CompletedCount int       // Number of completed subtasks
}

// FormatEpicDescription creates a description string from epic fields and plan content.
func FormatEpicDescription(title string, fields *EpicFields, planContent string) string {
	if fields == nil {
		fields = &EpicFields{EpicState: EpicStateDrafting}
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	// Epic state
	if fields.EpicState != "" {
		lines = append(lines, fmt.Sprintf("epic_state: %s", fields.EpicState))
	} else {
		lines = append(lines, "epic_state: drafting")
	}

	// Contributing MD path
	if fields.ContributingMD != "" {
		lines = append(lines, fmt.Sprintf("contributing_md: %s", fields.ContributingMD))
	} else {
		lines = append(lines, "contributing_md: null")
	}

	// Upstream PRs
	if fields.UpstreamPRs != "" {
		lines = append(lines, fmt.Sprintf("upstream_prs: %s", fields.UpstreamPRs))
	} else {
		lines = append(lines, "upstream_prs: null")
	}

	// Integration branch
	if fields.IntegrationBr != "" {
		lines = append(lines, fmt.Sprintf("integration_branch: %s", fields.IntegrationBr))
	} else {
		lines = append(lines, "integration_branch: null")
	}

	// Subtask counts (only if non-zero)
	if fields.SubtaskCount > 0 {
		lines = append(lines, fmt.Sprintf("subtask_count: %d", fields.SubtaskCount))
		lines = append(lines, fmt.Sprintf("completed_count: %d", fields.CompletedCount))
	}

	// Add plan content if provided
	if planContent != "" {
		lines = append(lines, "")
		lines = append(lines, planContent)
	}

	return strings.Join(lines, "\n")
}

// ParseEpicFields extracts epic fields from an issue's description.
func ParseEpicFields(description string) *EpicFields {
	fields := &EpicFields{}

	for _, line := range strings.Split(description, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		if value == "null" || value == "" {
			value = ""
		}

		switch strings.ToLower(key) {
		case "epic_state":
			fields.EpicState = EpicState(value)
		case "contributing_md":
			fields.ContributingMD = value
		case "upstream_prs":
			fields.UpstreamPRs = value
		case "integration_branch":
			fields.IntegrationBr = value
		case "subtask_count":
			if count, err := strconv.Atoi(value); err == nil {
				fields.SubtaskCount = count
			}
		case "completed_count":
			if count, err := strconv.Atoi(value); err == nil {
				fields.CompletedCount = count
			}
		}
	}

	return fields
}

// ExtractPlanContent extracts the plan content from epic description.
// The plan content is everything after the metadata fields.
func ExtractPlanContent(description string) string {
	lines := strings.Split(description, "\n")
	var planLines []string
	inMetadata := true
	foundBlankAfterMeta := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip the title (first non-empty line)
		if !foundBlankAfterMeta && trimmed != "" && !strings.Contains(trimmed, ":") {
			continue
		}

		// Check if this is a metadata line
		if inMetadata {
			if trimmed == "" {
				foundBlankAfterMeta = true
				continue
			}
			colonIdx := strings.Index(trimmed, ":")
			if colonIdx > 0 {
				key := strings.ToLower(strings.TrimSpace(trimmed[:colonIdx]))
				if key == "epic_state" || key == "contributing_md" || key == "upstream_prs" ||
					key == "integration_branch" || key == "subtask_count" || key == "completed_count" {
					continue
				}
			}
			// Not a metadata line - we've reached plan content
			inMetadata = false
		}

		if !inMetadata {
			planLines = append(planLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(planLines, "\n"))
}

// CreateEpicBead creates an epic bead.
// The ID format is: <prefix>-epic-<shortid>
func (b *Beads) CreateEpicBead(id, title string, fields *EpicFields) (*Issue, error) {
	description := FormatEpicDescription(title, fields, "")

	args := []string{"create", "--json",
		"--id=" + id,
		"--title=" + title,
		"--description=" + description,
		"--type=epic",
		"--labels=gt:epic",
	}

	// Default actor from BD_ACTOR env var for provenance tracking
	if actor := os.Getenv("BD_ACTOR"); actor != "" {
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

// UpdateEpicState updates the epic_state field in an epic bead.
func (b *Beads) UpdateEpicState(id string, state EpicState) error {
	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseEpicFields(issue.Description)
	fields.EpicState = state

	// Preserve plan content
	planContent := ExtractPlanContent(issue.Description)

	// Format new description
	description := FormatEpicDescription(issue.Title, fields, planContent)

	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateEpicFields updates all fields in an epic bead.
func (b *Beads) UpdateEpicFields(id string, fields *EpicFields) error {
	// First get current issue to preserve plan content
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Preserve plan content
	planContent := ExtractPlanContent(issue.Description)

	// Format new description
	description := FormatEpicDescription(issue.Title, fields, planContent)

	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateEpicPlan updates the plan content in an epic bead.
func (b *Beads) UpdateEpicPlan(id, planContent string) error {
	// First get current issue to preserve fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseEpicFields(issue.Description)

	// Format new description with updated plan
	description := FormatEpicDescription(issue.Title, fields, planContent)

	return b.Update(id, UpdateOptions{Description: &description})
}

// GetEpicBead retrieves an epic bead by ID.
// Returns nil if not found.
func (b *Beads) GetEpicBead(id string) (*Issue, *EpicFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if issue.Type != "epic" && !HasLabel(issue, "gt:epic") {
		return nil, nil, fmt.Errorf("issue %s is not an epic bead", id)
	}

	fields := ParseEpicFields(issue.Description)
	return issue, fields, nil
}

// ListEpicBeads returns all epic beads.
func (b *Beads) ListEpicBeads() ([]*Issue, error) {
	out, err := b.run("list", "--type=epic", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	return issues, nil
}

// ListEpicsByState returns epics filtered by state.
func (b *Beads) ListEpicsByState(state EpicState) ([]*Issue, error) {
	allEpics, err := b.ListEpicBeads()
	if err != nil {
		return nil, err
	}

	var filtered []*Issue
	for _, epic := range allEpics {
		fields := ParseEpicFields(epic.Description)
		if fields.EpicState == state {
			filtered = append(filtered, epic)
		}
	}

	return filtered, nil
}

// GetEpicSubtasks returns all subtasks (children) of an epic.
func (b *Beads) GetEpicSubtasks(epicID string) ([]*Issue, error) {
	out, err := b.run("list", "--parent="+epicID, "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	return issues, nil
}

// AddUpstreamPR adds a PR URL to the epic's upstream_prs field.
func (b *Beads) AddUpstreamPR(epicID, prURL string) error {
	issue, fields, err := b.GetEpicBead(epicID)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("epic %s not found", epicID)
	}

	// Parse existing PRs
	var prs []string
	if fields.UpstreamPRs != "" {
		prs = strings.Split(fields.UpstreamPRs, ",")
	}

	// Check if already exists
	for _, pr := range prs {
		if strings.TrimSpace(pr) == prURL {
			return nil // Already exists
		}
	}

	// Add new PR
	prs = append(prs, prURL)
	fields.UpstreamPRs = strings.Join(prs, ",")

	return b.UpdateEpicFields(epicID, fields)
}

// ValidEpicStateTransition checks if a state transition is valid.
func ValidEpicStateTransition(from, to EpicState) bool {
	transitions := map[EpicState][]EpicState{
		EpicStateDrafting:   {EpicStateReady, EpicStateClosed},
		EpicStateReady:      {EpicStateInProgress, EpicStateDrafting, EpicStateClosed},
		EpicStateInProgress: {EpicStateReview, EpicStateDrafting, EpicStateClosed},
		EpicStateReview:     {EpicStateSubmitted, EpicStateInProgress, EpicStateClosed},
		EpicStateSubmitted:  {EpicStateLanded, EpicStateReview, EpicStateClosed},
		EpicStateLanded:     {EpicStateClosed},
		EpicStateClosed:     {EpicStateDrafting}, // Can reopen to drafting
	}

	allowed, ok := transitions[from]
	if !ok {
		return false
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// IsNotFound checks if an error is a "not found" error.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "no such") ||
		strings.Contains(errStr, "does not exist")
}
