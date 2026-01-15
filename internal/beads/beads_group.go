// Package beads provides group bead management for beads-native messaging.
// Groups are named collections of addresses used for mail distribution.
package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// GroupFields holds structured fields for group beads.
// These are stored as "key: value" lines in the description.
type GroupFields struct {
	Name      string   // Unique group name (e.g., "ops-team", "all-witnesses")
	Members   []string // Addresses, patterns, or group names (can nest)
	CreatedBy string   // Who created the group
	CreatedAt string   // ISO 8601 timestamp
}

// FormatGroupDescription creates a description string from group fields.
func FormatGroupDescription(title string, fields *GroupFields) string {
	if fields == nil {
		return title
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("name: %s", fields.Name))

	// Members stored as comma-separated list
	if len(fields.Members) > 0 {
		lines = append(lines, fmt.Sprintf("members: %s", strings.Join(fields.Members, ",")))
	} else {
		lines = append(lines, "members: null")
	}

	if fields.CreatedBy != "" {
		lines = append(lines, fmt.Sprintf("created_by: %s", fields.CreatedBy))
	} else {
		lines = append(lines, "created_by: null")
	}

	if fields.CreatedAt != "" {
		lines = append(lines, fmt.Sprintf("created_at: %s", fields.CreatedAt))
	} else {
		lines = append(lines, "created_at: null")
	}

	return strings.Join(lines, "\n")
}

// ParseGroupFields extracts group fields from an issue's description.
func ParseGroupFields(description string) *GroupFields {
	fields := &GroupFields{}

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
		case "members":
			if value != "" {
				// Parse comma-separated members
				for _, m := range strings.Split(value, ",") {
					m = strings.TrimSpace(m)
					if m != "" {
						fields.Members = append(fields.Members, m)
					}
				}
			}
		case "created_by":
			fields.CreatedBy = value
		case "created_at":
			fields.CreatedAt = value
		}
	}

	return fields
}

// GroupBeadID returns the bead ID for a group name.
// Format: hq-group-<name> (town-level, groups span rigs)
func GroupBeadID(name string) string {
	return "hq-group-" + name
}

// CreateGroupBead creates a group bead for mail distribution.
// The ID format is: hq-group-<name> (e.g., hq-group-ops-team)
// Groups are town-level entities (hq- prefix) because they span rigs.
// The created_by field is populated from BD_ACTOR env var for provenance tracking.
func (b *Beads) CreateGroupBead(name string, members []string, createdBy string) (*Issue, error) {
	id := GroupBeadID(name)
	title := fmt.Sprintf("Group: %s", name)

	fields := &GroupFields{
		Name:      name,
		Members:   members,
		CreatedBy: createdBy,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	description := FormatGroupDescription(title, fields)

	args := []string{"create", "--json",
		"--id=" + id,
		"--title=" + title,
		"--description=" + description,
		"--type=task", // Groups use task type with gt:group label
		"--labels=gt:group",
		"--force", // Override prefix check (town beads may have mixed prefixes)
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

// GetGroupBead retrieves a group bead by name.
// Returns nil, nil if not found.
func (b *Beads) GetGroupBead(name string) (*Issue, *GroupFields, error) {
	id := GroupBeadID(name)
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !HasLabel(issue, "gt:group") {
		return nil, nil, fmt.Errorf("bead %s is not a group bead (missing gt:group label)", id)
	}

	fields := ParseGroupFields(issue.Description)
	return issue, fields, nil
}

// GetGroupByID retrieves a group bead by its full ID.
// Returns nil, nil if not found.
func (b *Beads) GetGroupByID(id string) (*Issue, *GroupFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !HasLabel(issue, "gt:group") {
		return nil, nil, fmt.Errorf("bead %s is not a group bead (missing gt:group label)", id)
	}

	fields := ParseGroupFields(issue.Description)
	return issue, fields, nil
}

// UpdateGroupMembers updates the members list for a group.
func (b *Beads) UpdateGroupMembers(name string, members []string) error {
	issue, fields, err := b.GetGroupBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("group %q not found", name)
	}

	fields.Members = members
	description := FormatGroupDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// AddGroupMember adds a member to a group if not already present.
func (b *Beads) AddGroupMember(name string, member string) error {
	issue, fields, err := b.GetGroupBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("group %q not found", name)
	}

	// Check if already a member
	for _, m := range fields.Members {
		if m == member {
			return nil // Already a member
		}
	}

	fields.Members = append(fields.Members, member)
	description := FormatGroupDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// RemoveGroupMember removes a member from a group.
func (b *Beads) RemoveGroupMember(name string, member string) error {
	issue, fields, err := b.GetGroupBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("group %q not found", name)
	}

	// Filter out the member
	var newMembers []string
	for _, m := range fields.Members {
		if m != member {
			newMembers = append(newMembers, m)
		}
	}

	fields.Members = newMembers
	description := FormatGroupDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// DeleteGroupBead permanently deletes a group bead.
func (b *Beads) DeleteGroupBead(name string) error {
	id := GroupBeadID(name)
	_, err := b.run("delete", id, "--hard", "--force")
	return err
}

// ListGroupBeads returns all group beads.
func (b *Beads) ListGroupBeads() (map[string]*GroupFields, error) {
	out, err := b.run("list", "--label=gt:group", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	result := make(map[string]*GroupFields, len(issues))
	for _, issue := range issues {
		fields := ParseGroupFields(issue.Description)
		if fields.Name != "" {
			result[fields.Name] = fields
		}
	}

	return result, nil
}

// LookupGroupByName finds a group by its name field (not by ID).
// This is used for address resolution where we may not know the full bead ID.
func (b *Beads) LookupGroupByName(name string) (*Issue, *GroupFields, error) {
	// First try direct lookup by standard ID format
	issue, fields, err := b.GetGroupBead(name)
	if err != nil {
		return nil, nil, err
	}
	if issue != nil {
		return issue, fields, nil
	}

	// If not found by ID, search all groups by name field
	groups, err := b.ListGroupBeads()
	if err != nil {
		return nil, nil, err
	}

	if fields, ok := groups[name]; ok {
		// Found by name, now get the full issue
		id := GroupBeadID(name)
		issue, err := b.Show(id)
		if err != nil {
			return nil, nil, err
		}
		return issue, fields, nil
	}

	return nil, nil, nil // Not found
}
