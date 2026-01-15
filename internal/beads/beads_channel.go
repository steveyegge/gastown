// Package beads provides channel bead management for beads-native messaging.
// Channels are named pub/sub streams where messages are broadcast to subscribers.
package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ChannelFields holds structured fields for channel beads.
// These are stored as "key: value" lines in the description.
type ChannelFields struct {
	Name           string   // Unique channel name (e.g., "alerts", "builds")
	Subscribers    []string // Addresses subscribed to this channel
	Status         string   // active, closed
	RetentionCount int      // Number of recent messages to retain (0 = unlimited)
	RetentionHours int      // Hours to retain messages (0 = forever)
	CreatedBy      string   // Who created the channel
	CreatedAt      string   // ISO 8601 timestamp
}

// Channel status constants
const (
	ChannelStatusActive = "active"
	ChannelStatusClosed = "closed"
)

// FormatChannelDescription creates a description string from channel fields.
func FormatChannelDescription(title string, fields *ChannelFields) string {
	if fields == nil {
		return title
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("name: %s", fields.Name))

	// Subscribers stored as comma-separated list
	if len(fields.Subscribers) > 0 {
		lines = append(lines, fmt.Sprintf("subscribers: %s", strings.Join(fields.Subscribers, ",")))
	} else {
		lines = append(lines, "subscribers: null")
	}

	if fields.Status != "" {
		lines = append(lines, fmt.Sprintf("status: %s", fields.Status))
	} else {
		lines = append(lines, "status: active")
	}

	lines = append(lines, fmt.Sprintf("retention_count: %d", fields.RetentionCount))
	lines = append(lines, fmt.Sprintf("retention_hours: %d", fields.RetentionHours))

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

// ParseChannelFields extracts channel fields from an issue's description.
func ParseChannelFields(description string) *ChannelFields {
	fields := &ChannelFields{
		Status: ChannelStatusActive,
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
		case "subscribers":
			if value != "" {
				// Parse comma-separated subscribers
				for _, s := range strings.Split(value, ",") {
					s = strings.TrimSpace(s)
					if s != "" {
						fields.Subscribers = append(fields.Subscribers, s)
					}
				}
			}
		case "status":
			fields.Status = value
		case "retention_count":
			if v, err := strconv.Atoi(value); err == nil {
				fields.RetentionCount = v
			}
		case "retention_hours":
			if v, err := strconv.Atoi(value); err == nil {
				fields.RetentionHours = v
			}
		case "created_by":
			fields.CreatedBy = value
		case "created_at":
			fields.CreatedAt = value
		}
	}

	return fields
}

// ChannelBeadID returns the bead ID for a channel name.
// Format: hq-channel-<name> (town-level, channels span rigs)
func ChannelBeadID(name string) string {
	return "hq-channel-" + name
}

// CreateChannelBead creates a channel bead for pub/sub messaging.
// The ID format is: hq-channel-<name> (e.g., hq-channel-alerts)
// Channels are town-level entities (hq- prefix) because they span rigs.
// The created_by field is populated from BD_ACTOR env var for provenance tracking.
func (b *Beads) CreateChannelBead(name string, subscribers []string, createdBy string) (*Issue, error) {
	id := ChannelBeadID(name)
	title := fmt.Sprintf("Channel: %s", name)

	fields := &ChannelFields{
		Name:        name,
		Subscribers: subscribers,
		Status:      ChannelStatusActive,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	description := FormatChannelDescription(title, fields)

	args := []string{"create", "--json",
		"--id=" + id,
		"--title=" + title,
		"--description=" + description,
		"--type=task", // Channels use task type with gt:channel label
		"--labels=gt:channel",
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

// GetChannelBead retrieves a channel bead by name.
// Returns nil, nil if not found.
func (b *Beads) GetChannelBead(name string) (*Issue, *ChannelFields, error) {
	id := ChannelBeadID(name)
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !HasLabel(issue, "gt:channel") {
		return nil, nil, fmt.Errorf("bead %s is not a channel bead (missing gt:channel label)", id)
	}

	fields := ParseChannelFields(issue.Description)
	return issue, fields, nil
}

// GetChannelByID retrieves a channel bead by its full ID.
// Returns nil, nil if not found.
func (b *Beads) GetChannelByID(id string) (*Issue, *ChannelFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !HasLabel(issue, "gt:channel") {
		return nil, nil, fmt.Errorf("bead %s is not a channel bead (missing gt:channel label)", id)
	}

	fields := ParseChannelFields(issue.Description)
	return issue, fields, nil
}

// UpdateChannelSubscribers updates the subscribers list for a channel.
func (b *Beads) UpdateChannelSubscribers(name string, subscribers []string) error {
	issue, fields, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("channel %q not found", name)
	}

	fields.Subscribers = subscribers
	description := FormatChannelDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// SubscribeToChannel adds a subscriber to a channel if not already subscribed.
func (b *Beads) SubscribeToChannel(name string, subscriber string) error {
	issue, fields, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("channel %q not found", name)
	}

	// Check if already subscribed
	for _, s := range fields.Subscribers {
		if s == subscriber {
			return nil // Already subscribed
		}
	}

	fields.Subscribers = append(fields.Subscribers, subscriber)
	description := FormatChannelDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// UnsubscribeFromChannel removes a subscriber from a channel.
func (b *Beads) UnsubscribeFromChannel(name string, subscriber string) error {
	issue, fields, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("channel %q not found", name)
	}

	// Filter out the subscriber
	var newSubscribers []string
	for _, s := range fields.Subscribers {
		if s != subscriber {
			newSubscribers = append(newSubscribers, s)
		}
	}

	fields.Subscribers = newSubscribers
	description := FormatChannelDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// UpdateChannelRetention updates the retention policy for a channel.
func (b *Beads) UpdateChannelRetention(name string, retentionCount, retentionHours int) error {
	issue, fields, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("channel %q not found", name)
	}

	fields.RetentionCount = retentionCount
	fields.RetentionHours = retentionHours
	description := FormatChannelDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// UpdateChannelStatus updates the status of a channel bead.
func (b *Beads) UpdateChannelStatus(name, status string) error {
	// Validate status
	if status != ChannelStatusActive && status != ChannelStatusClosed {
		return fmt.Errorf("invalid channel status %q: must be active or closed", status)
	}

	issue, fields, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if issue == nil {
		return fmt.Errorf("channel %q not found", name)
	}

	fields.Status = status
	description := FormatChannelDescription(issue.Title, fields)

	return b.Update(issue.ID, UpdateOptions{Description: &description})
}

// DeleteChannelBead permanently deletes a channel bead.
func (b *Beads) DeleteChannelBead(name string) error {
	id := ChannelBeadID(name)
	_, err := b.run("delete", id, "--hard", "--force")
	return err
}

// ListChannelBeads returns all channel beads.
func (b *Beads) ListChannelBeads() (map[string]*ChannelFields, error) {
	out, err := b.run("list", "--label=gt:channel", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	result := make(map[string]*ChannelFields, len(issues))
	for _, issue := range issues {
		fields := ParseChannelFields(issue.Description)
		if fields.Name != "" {
			result[fields.Name] = fields
		}
	}

	return result, nil
}

// LookupChannelByName finds a channel by its name field (not by ID).
// This is used for address resolution where we may not know the full bead ID.
func (b *Beads) LookupChannelByName(name string) (*Issue, *ChannelFields, error) {
	// First try direct lookup by standard ID format
	issue, fields, err := b.GetChannelBead(name)
	if err != nil {
		return nil, nil, err
	}
	if issue != nil {
		return issue, fields, nil
	}

	// If not found by ID, search all channels by name field
	channels, err := b.ListChannelBeads()
	if err != nil {
		return nil, nil, err
	}

	if fields, ok := channels[name]; ok {
		// Found by name, now get the full issue
		id := ChannelBeadID(name)
		issue, err := b.Show(id)
		if err != nil {
			return nil, nil, err
		}
		return issue, fields, nil
	}

	return nil, nil, nil // Not found
}

// EnforceChannelRetention prunes old messages from a channel to enforce retention.
// Called after posting a new message to the channel (on-write cleanup).
// If channel has >= retainCount messages, deletes oldest until count < retainCount.
func (b *Beads) EnforceChannelRetention(name string) error {
	// Get channel config
	_, fields, err := b.GetChannelBead(name)
	if err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("channel not found: %s", name)
	}

	// Skip if no retention limit
	if fields.RetentionCount <= 0 {
		return nil
	}

	// Query messages in this channel (oldest first)
	out, err := b.run("list",
		"--type=message",
		"--label=channel:"+name,
		"--json",
		"--limit=0",
		"--sort=created",
	)
	if err != nil {
		return fmt.Errorf("listing channel messages: %w", err)
	}

	var messages []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &messages); err != nil {
		return fmt.Errorf("parsing channel messages: %w", err)
	}

	// Calculate how many to delete
	// We're being called after a new message is posted, so we want to end up with retainCount
	toDelete := len(messages) - fields.RetentionCount
	if toDelete <= 0 {
		return nil // No pruning needed
	}

	// Delete oldest messages (best-effort)
	for i := 0; i < toDelete && i < len(messages); i++ {
		// Use close instead of delete for audit trail
		_, _ = b.run("close", messages[i].ID, "--reason=channel retention pruning")
	}

	return nil
}

// PruneAllChannels enforces retention on all channels.
// Called by Deacon patrol as a backup cleanup mechanism.
// Uses a 10% buffer to avoid thrashing (only prunes if count > retainCount * 1.1).
func (b *Beads) PruneAllChannels() (int, error) {
	channels, err := b.ListChannelBeads()
	if err != nil {
		return 0, err
	}

	pruned := 0
	for name, fields := range channels {
		if fields.RetentionCount <= 0 {
			continue
		}

		// Count messages
		out, err := b.run("list",
			"--type=message",
			"--label=channel:"+name,
			"--json",
			"--limit=0",
		)
		if err != nil {
			continue // Skip on error
		}

		var messages []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(out, &messages); err != nil {
			continue
		}

		// 10% buffer - only prune if significantly over limit
		threshold := int(float64(fields.RetentionCount) * 1.1)
		if len(messages) <= threshold {
			continue
		}

		// Prune down to exactly retainCount
		toDelete := len(messages) - fields.RetentionCount
		for i := 0; i < toDelete && i < len(messages); i++ {
			if _, err := b.run("close", messages[i].ID, "--reason=patrol retention pruning"); err == nil {
				pruned++
			}
		}
	}

	return pruned, nil
}
