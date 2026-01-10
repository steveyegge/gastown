// Package polecat provides polecat lifecycle management.
package polecat

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/tmux"
)

// PendingSpawn represents a polecat that has been spawned but not yet triggered.
// This is discovered from POLECAT_STARTED messages in the Deacon inbox (ZFC).
type PendingSpawn struct {
	// Rig is the rig name (e.g., "gastown")
	Rig string `json:"rig"`

	// Polecat is the polecat name (e.g., "p-abc123")
	Polecat string `json:"polecat"`

	// Session is the tmux session name
	Session string `json:"session"`

	// Issue is the assigned issue ID
	Issue string `json:"issue"`

	// SpawnedAt is when the spawn was detected (from mail timestamp)
	SpawnedAt time.Time `json:"spawned_at"`

	// MailID is the ID of the POLECAT_STARTED message
	MailID string `json:"mail_id"`

	// mailbox is kept for archiving after trigger (not serialized)
	mailbox *mail.Mailbox `json:"-"`
}

// CheckInboxForSpawns discovers pending spawns from POLECAT_STARTED messages
// in the Deacon's inbox. Uses mail as source of truth (ZFC principle).
func CheckInboxForSpawns(townRoot string) ([]*PendingSpawn, error) {
	// Get Deacon's mailbox
	router := mail.NewRouter(townRoot)
	mailbox, err := router.GetMailbox("deacon/")
	if err != nil {
		return nil, fmt.Errorf("getting deacon mailbox: %w", err)
	}

	// Get all messages (both read and unread - we track by archival status)
	messages, err := mailbox.List()
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}

	var pending []*PendingSpawn

	// Look for POLECAT_STARTED messages
	for _, msg := range messages {
		if !strings.HasPrefix(msg.Subject, "POLECAT_STARTED ") {
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
			mailbox:   mailbox,
		}
		pending = append(pending, ps)
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
// Archives mail after successful trigger (ZFC: mail is source of truth).
func TriggerPendingSpawns(townRoot string, timeout time.Duration) ([]TriggerResult, error) {
	pending, err := CheckInboxForSpawns(townRoot)
	if err != nil {
		return nil, fmt.Errorf("checking inbox: %w", err)
	}

	if len(pending) == 0 {
		return nil, nil
	}

	t := tmux.NewTmux()
	var results []TriggerResult

	for _, ps := range pending {
		result := TriggerResult{Spawn: ps}

		// Check if session still exists (ZFC: query tmux directly)
		running, err := t.HasSession(ps.Session)
		if err != nil {
			result.Error = fmt.Errorf("checking session: %w", err)
			results = append(results, result)
			continue
		}

		if !running {
			// Session gone - archive the mail (spawn is dead)
			result.Error = fmt.Errorf("session no longer exists")
			if ps.mailbox != nil {
				_ = ps.mailbox.Archive(ps.MailID)
			}
			results = append(results, result)
			continue
		}

		// Check if runtime is ready (non-blocking poll)
		rigPath := filepath.Join(townRoot, ps.Rig)
		runtimeConfig := config.LoadRuntimeConfig(rigPath)
		err = t.WaitForRuntimeReady(ps.Session, runtimeConfig, timeout)
		if err != nil {
			// Not ready yet - leave mail in inbox for next poll
			continue
		}

		// Runtime is ready - send trigger
		triggerMsg := "Begin."
		if err := t.NudgeSession(ps.Session, triggerMsg); err != nil {
			result.Error = fmt.Errorf("nudging session: %w", err)
			results = append(results, result)
			continue
		}

		// Successfully triggered - archive the mail
		result.Triggered = true
		if ps.mailbox != nil {
			_ = ps.mailbox.Archive(ps.MailID)
		}
		results = append(results, result)
	}

	return results, nil
}

// PruneStalePending archives POLECAT_STARTED messages older than the given age.
// Old spawns likely had their sessions die without triggering.
func PruneStalePending(townRoot string, maxAge time.Duration) (int, error) {
	pending, err := CheckInboxForSpawns(townRoot)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	pruned := 0

	for _, ps := range pending {
		if ps.SpawnedAt.Before(cutoff) {
			// Archive stale spawn message
			if ps.mailbox != nil {
				_ = ps.mailbox.Archive(ps.MailID)
			}
			pruned++
		}
	}

	return pruned, nil
}
