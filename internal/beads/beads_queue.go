// Package beads provides queue bead management.
package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// QueueFields holds structured fields for queue beads.
// These are stored as "key: value" lines in the description.
type QueueFields struct {
	Name            string // Queue name (human-readable identifier)
	ClaimPattern    string // Pattern for who can claim from queue (e.g., "gastown/polecats/*")
	Status          string // active, paused, closed
	MaxConcurrency  int    // Maximum number of concurrent workers (0 = unlimited)
	ProcessingOrder string // fifo, priority (default: fifo)
	AvailableCount  int    // Number of items ready to process
	ProcessingCount int    // Number of items currently being processed
	CompletedCount  int    // Number of items completed
	FailedCount     int    // Number of items that failed
	CreatedBy       string // Who created this queue
	CreatedAt       string // ISO 8601 timestamp of creation
}

// Queue status constants
const (
	QueueStatusActive = "active"
	QueueStatusPaused = "paused"
	QueueStatusClosed = "closed"
)

// Queue processing order constants
const (
	QueueOrderFIFO     = "fifo"
	QueueOrderPriority = "priority"
)

// FormatQueueDescription creates a description string from queue fields.
func FormatQueueDescription(title string, fields *QueueFields) string {
	if fields == nil {
		return title
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")

	if fields.Name != "" {
		lines = append(lines, fmt.Sprintf("name: %s", fields.Name))
	} else {
		lines = append(lines, "name: null")
	}

	if fields.ClaimPattern != "" {
		lines = append(lines, fmt.Sprintf("claim_pattern: %s", fields.ClaimPattern))
	} else {
		lines = append(lines, "claim_pattern: *") // Default: anyone can claim
	}

	if fields.Status != "" {
		lines = append(lines, fmt.Sprintf("status: %s", fields.Status))
	} else {
		lines = append(lines, "status: active")
	}

	lines = append(lines, fmt.Sprintf("max_concurrency: %d", fields.MaxConcurrency))

	if fields.ProcessingOrder != "" {
		lines = append(lines, fmt.Sprintf("processing_order: %s", fields.ProcessingOrder))
	} else {
		lines = append(lines, "processing_order: fifo")
	}

	lines = append(lines, fmt.Sprintf("available_count: %d", fields.AvailableCount))
	lines = append(lines, fmt.Sprintf("processing_count: %d", fields.ProcessingCount))
	lines = append(lines, fmt.Sprintf("completed_count: %d", fields.CompletedCount))
	lines = append(lines, fmt.Sprintf("failed_count: %d", fields.FailedCount))

	if fields.CreatedBy != "" {
		lines = append(lines, fmt.Sprintf("created_by: %s", fields.CreatedBy))
	}
	if fields.CreatedAt != "" {
		lines = append(lines, fmt.Sprintf("created_at: %s", fields.CreatedAt))
	}

	return strings.Join(lines, "\n")
}

// ParseQueueFields extracts queue fields from an issue's description.
func ParseQueueFields(description string) *QueueFields {
	fields := &QueueFields{
		Status:          QueueStatusActive,
		ProcessingOrder: QueueOrderFIFO,
		ClaimPattern:    "*", // Default: anyone can claim
	}

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
		case "name":
			fields.Name = value
		case "claim_pattern":
			if value != "" {
				fields.ClaimPattern = value
			}
		case "status":
			fields.Status = value
		case "max_concurrency":
			if v, err := strconv.Atoi(value); err == nil {
				fields.MaxConcurrency = v
			}
		case "processing_order":
			fields.ProcessingOrder = value
		case "available_count":
			if v, err := strconv.Atoi(value); err == nil {
				fields.AvailableCount = v
			}
		case "processing_count":
			if v, err := strconv.Atoi(value); err == nil {
				fields.ProcessingCount = v
			}
		case "completed_count":
			if v, err := strconv.Atoi(value); err == nil {
				fields.CompletedCount = v
			}
		case "failed_count":
			if v, err := strconv.Atoi(value); err == nil {
				fields.FailedCount = v
			}
		case "created_by":
			fields.CreatedBy = value
		case "created_at":
			fields.CreatedAt = value
		}
	}

	return fields
}

// QueueBeadID returns the queue bead ID for a given queue name.
// Format: hq-q-<name> for town-level queues, gt-q-<name> for rig-level queues.
func QueueBeadID(name string, isTownLevel bool) string {
	if isTownLevel {
		return "hq-q-" + name
	}
	return "gt-q-" + name
}

// CreateQueueBead creates a queue bead for tracking work queues.
// The ID format is: <prefix>-q-<name> (e.g., gt-q-merge, hq-q-dispatch)
// The created_by field is populated from BD_ACTOR env var for provenance tracking.
func (b *Beads) CreateQueueBead(id, title string, fields *QueueFields) (*Issue, error) {
	description := FormatQueueDescription(title, fields)

	args := []string{"create", "--json",
		"--id=" + id,
		"--title=" + title,
		"--description=" + description,
		"--type=queue",
		"--labels=gt:queue",
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

// GetQueueBead retrieves a queue bead by ID.
// Returns nil if not found.
func (b *Beads) GetQueueBead(id string) (*Issue, *QueueFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !HasLabel(issue, "gt:queue") {
		return nil, nil, fmt.Errorf("issue %s is not a queue bead (missing gt:queue label)", id)
	}

	fields := ParseQueueFields(issue.Description)
	return issue, fields, nil
}

// UpdateQueueFields updates the fields of a queue bead.
func (b *Beads) UpdateQueueFields(id string, fields *QueueFields) error {
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	description := FormatQueueDescription(issue.Title, fields)
	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateQueueCounts updates the count fields of a queue bead.
// This is a convenience method for incrementing/decrementing counts.
func (b *Beads) UpdateQueueCounts(id string, available, processing, completed, failed int) error {
	issue, currentFields, err := b.GetQueueBead(id)
	if err != nil {
		return err
	}
	if issue == nil {
		return ErrNotFound
	}

	currentFields.AvailableCount = available
	currentFields.ProcessingCount = processing
	currentFields.CompletedCount = completed
	currentFields.FailedCount = failed

	return b.UpdateQueueFields(id, currentFields)
}

// UpdateQueueStatus updates the status of a queue bead.
func (b *Beads) UpdateQueueStatus(id, status string) error {
	// Validate status
	if status != QueueStatusActive && status != QueueStatusPaused && status != QueueStatusClosed {
		return fmt.Errorf("invalid queue status %q: must be active, paused, or closed", status)
	}

	issue, currentFields, err := b.GetQueueBead(id)
	if err != nil {
		return err
	}
	if issue == nil {
		return ErrNotFound
	}

	currentFields.Status = status
	return b.UpdateQueueFields(id, currentFields)
}

// ListQueueBeads returns all queue beads.
func (b *Beads) ListQueueBeads() (map[string]*Issue, error) {
	out, err := b.run("list", "--label=gt:queue", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	result := make(map[string]*Issue, len(issues))
	for _, issue := range issues {
		result[issue.ID] = issue
	}

	return result, nil
}

// DeleteQueueBead permanently deletes a queue bead.
// Uses --hard --force for immediate permanent deletion (no tombstone).
func (b *Beads) DeleteQueueBead(id string) error {
	_, err := b.run("delete", id, "--hard", "--force")
	return err
}

// MatchClaimPattern checks if an identity matches a claim pattern.
// Patterns support:
//   - "*" matches anyone
//   - "gastown/polecats/*" matches any polecat in gastown rig
//   - "*/witness" matches any witness role across rigs
//   - Exact match for specific identities
func MatchClaimPattern(pattern, identity string) bool {
	// Wildcard matches anyone
	if pattern == "*" {
		return true
	}

	// Exact match
	if pattern == identity {
		return true
	}

	// Wildcard pattern matching
	if strings.Contains(pattern, "*") {
		// Convert to simple glob matching
		// "gastown/polecats/*" should match "gastown/polecats/capable"
		// "*/witness" should match "gastown/witness"
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			if strings.HasPrefix(identity, prefix) && strings.HasSuffix(identity, suffix) {
				// Check that the middle part doesn't contain path separators
				// unless the pattern allows it (e.g., "*/" at start)
				middle := identity[len(prefix) : len(identity)-len(suffix)]
				// Only allow single segment match (no extra slashes)
				if !strings.Contains(middle, "/") {
					return true
				}
			}
		}
	}

	return false
}

// FindEligibleQueues returns all queue beads that the given identity can claim from.
func (b *Beads) FindEligibleQueues(identity string) ([]*Issue, []*QueueFields, error) {
	queues, err := b.ListQueueBeads()
	if err != nil {
		return nil, nil, err
	}

	var eligibleIssues []*Issue
	var eligibleFields []*QueueFields

	for _, issue := range queues {
		fields := ParseQueueFields(issue.Description)

		// Skip inactive queues
		if fields.Status != QueueStatusActive {
			continue
		}

		// Check if identity matches claim pattern
		if MatchClaimPattern(fields.ClaimPattern, identity) {
			eligibleIssues = append(eligibleIssues, issue)
			eligibleFields = append(eligibleFields, fields)
		}
	}

	return eligibleIssues, eligibleFields, nil
}
