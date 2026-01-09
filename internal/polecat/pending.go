// Package polecat provides polecat lifecycle management.
package polecat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/tmux"
)

// PendingSpawn represents a polecat that has been spawned but not yet triggered.
type PendingSpawn struct {
	// Rig is the rig name (e.g., "gastown")
	Rig string `json:"rig"`

	// Polecat is the polecat name (e.g., "p-abc123")
	Polecat string `json:"polecat"`

	// Session is the tmux session name
	Session string `json:"session"`

	// Issue is the assigned issue ID
	Issue string `json:"issue"`

	// SpawnedAt is when the spawn was detected
	SpawnedAt time.Time `json:"spawned_at"`

	// MailID is the ID of the POLECAT_STARTED message
	MailID string `json:"mail_id"`
}

// PendingFile returns the path to the pending spawns file.
func PendingFile(townRoot string) string {
	return filepath.Join(townRoot, "spawn", "pending.json")
}

// LoadPending loads the pending spawns from disk.
func LoadPending(townRoot string) ([]*PendingSpawn, error) {
	path := PendingFile(townRoot)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var pending []*PendingSpawn
	if err := json.Unmarshal(data, &pending); err != nil {
		return nil, err
	}
	return pending, nil
}

// SavePending saves the pending spawns to disk.
func SavePending(townRoot string, pending []*PendingSpawn) error {
	path := PendingFile(townRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// CheckInboxForSpawns reads the Deacon's inbox for POLECAT_STARTED messages
// and adds them to the pending list.
func CheckInboxForSpawns(townRoot string) ([]*PendingSpawn, error) {
	// Get Deacon's mailbox
	router := mail.NewRouter(townRoot)
	mailbox, err := router.GetMailbox("deacon/")
	if err != nil {
		return nil, fmt.Errorf("getting deacon mailbox: %w", err)
	}

	// Get unread messages
	messages, err := mailbox.ListUnread()
	if err != nil {
		return nil, fmt.Errorf("listing unread: %w", err)
	}

	// Load existing pending
	pending, err := LoadPending(townRoot)
	if err != nil {
		return nil, fmt.Errorf("loading pending: %w", err)
	}

	// Track existing by mail ID to avoid duplicates
	existing := make(map[string]bool)
	for _, p := range pending {
		existing[p.MailID] = true
	}

	// Look for POLECAT_STARTED messages
	for _, msg := range messages {
		if !strings.HasPrefix(msg.Subject, "POLECAT_STARTED ") {
			continue
		}

		// Skip if already tracked
		if existing[msg.ID] {
			continue
		}

		// Parse subject: "POLECAT_STARTED rig/polecat"
		parts := strings.SplitN(strings.TrimPrefix(msg.Subject, "POLECAT_STARTED "), "/", 2)
		if len(parts) != 2 {
			continue
		}

		rig := parts[0]
		polecat := parts[1]

		// Parse body for session and issue
		var session, issue string
		for _, line := range strings.Split(msg.Body, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Session: ") {
				session = strings.TrimPrefix(line, "Session: ")
			} else if strings.HasPrefix(line, "Issue: ") {
				issue = strings.TrimPrefix(line, "Issue: ")
			}
		}

		ps := &PendingSpawn{
			Rig:       rig,
			Polecat:   polecat,
			Session:   session,
			Issue:     issue,
			SpawnedAt: msg.Timestamp,
			MailID:    msg.ID,
		}
		pending = append(pending, ps)
		existing[msg.ID] = true

		// Mark message as read (non-fatal: message tracking)
		_ = mailbox.MarkRead(msg.ID)
	}

	// Save updated pending list
	if err := SavePending(townRoot, pending); err != nil {
		return nil, fmt.Errorf("saving pending: %w", err)
	}

	return pending, nil
}

// TriggerResult holds the result of attempting to trigger a pending spawn.
type TriggerResult struct {
	Spawn     *PendingSpawn
	Triggered bool
	Error     error
}

// TriggerPendingSpawns polls each pending spawn and triggers when ready.
// Returns the spawns that were successfully triggered.
func TriggerPendingSpawns(townRoot string, timeout time.Duration) ([]TriggerResult, error) {
	pending, err := LoadPending(townRoot)
	if err != nil {
		return nil, fmt.Errorf("loading pending: %w", err)
	}

	if len(pending) == 0 {
		return nil, nil
	}

	t := tmux.NewTmux()
	var results []TriggerResult
	var remaining []*PendingSpawn

	for _, ps := range pending {
		result := TriggerResult{Spawn: ps}

		// Check if session still exists
		running, err := t.HasSession(ps.Session)
		if err != nil {
			result.Error = fmt.Errorf("checking session: %w", err)
			results = append(results, result)
			remaining = append(remaining, ps)
			continue
		}

		if !running {
			// Session gone - remove from pending
			result.Error = fmt.Errorf("session no longer exists")
			results = append(results, result)
			continue
		}

		// Check if runtime is ready (non-blocking poll)
		rigPath := filepath.Join(townRoot, ps.Rig)
		runtimeConfig := config.LoadRuntimeConfig(rigPath)
		err = t.WaitForRuntimeReady(ps.Session, runtimeConfig, timeout)
		if err != nil {
			// Not ready yet - keep in pending
			remaining = append(remaining, ps)
			continue
		}

		// Runtime is ready - send trigger
		triggerMsg := "Begin."
		if err := t.NudgeSession(ps.Session, triggerMsg); err != nil {
			result.Error = fmt.Errorf("nudging session: %w", err)
			results = append(results, result)
			remaining = append(remaining, ps)
			continue
		}

		// Successfully triggered
		result.Triggered = true
		results = append(results, result)
	}

	// Save remaining (untriggered) spawns
	if err := SavePending(townRoot, remaining); err != nil {
		return results, fmt.Errorf("saving remaining: %w", err)
	}

	return results, nil
}

// PruneStalePending removes pending spawns older than the given age.
// Spawns that are too old likely had their sessions die.
func PruneStalePending(townRoot string, maxAge time.Duration) (int, error) {
	pending, err := LoadPending(townRoot)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	var remaining []*PendingSpawn
	pruned := 0

	for _, ps := range pending {
		if ps.SpawnedAt.Before(cutoff) {
			pruned++
		} else {
			remaining = append(remaining, ps)
		}
	}

	if pruned > 0 {
		if err := SavePending(townRoot, remaining); err != nil {
			return pruned, err
		}
	}

	return pruned, nil
}
