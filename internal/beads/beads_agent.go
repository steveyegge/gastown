// Package beads provides agent bead management.
package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runSlotSet runs `bd slot set` from a specific directory.
// This is needed when the agent bead was created via routing to a different
// database than the Beads wrapper's default directory.
// Uses --no-daemon to avoid hanging when daemon isn't running (fix: fhc-e520ae).
// Explicitly sets BEADS_DIR to prevent inherited env vars from causing routing issues.
func runSlotSet(workDir, beadID, slotName, slotValue string) error {
	cmd := exec.Command("bd", "--no-daemon", "slot", "set", beadID, slotName, slotValue)
	cmd.Dir = workDir
	// Resolve beads directory and set explicitly to prevent inherited BEADS_DIR from
	// causing routing issues (fix: hq-aee961.18)
	beadsDir := ResolveBeadsDir(workDir)
	if beadsDir != "" {
		cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// runSlotClear runs `bd slot clear` from a specific directory.
// Uses --no-daemon to avoid hanging when daemon isn't running (fix: fhc-e520ae).
// Explicitly sets BEADS_DIR to prevent inherited env vars from causing routing issues.
func runSlotClear(workDir, beadID, slotName string) error {
	cmd := exec.Command("bd", "--no-daemon", "slot", "clear", beadID, slotName)
	cmd.Dir = workDir
	// Resolve beads directory and set explicitly to prevent inherited BEADS_DIR from
	// causing routing issues (fix: hq-aee961.18)
	beadsDir := ResolveBeadsDir(workDir)
	if beadsDir != "" {
		cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// AgentFields holds structured fields for agent beads.
// These are stored as "key: value" lines in the description.
type AgentFields struct {
	RoleType          string   // polecat, witness, refinery, deacon, mayor
	Rig               string   // Rig name (empty for global agents like mayor/deacon)
	AgentState        string   // spawning, working, done, stuck
	HookBead          string   // Currently pinned work bead ID
	CleanupStatus     string   // ZFC: polecat self-reports git state (clean, has_uncommitted, has_stash, has_unpushed)
	ActiveMR          string   // Currently active merge request bead ID (for traceability)
	NotificationLevel string   // DND mode: verbose, normal, muted (default: normal)
	OwnedFormulas     []string // Formulas this agent owns/maintains (crew workers)
	// Note: RoleBead field removed - role definitions are now config-based.
	// See internal/config/roles/*.toml and config-based-roles.md.
}

// Notification level constants
const (
	NotifyVerbose = "verbose" // All notifications (mail, convoy events, etc.)
	NotifyNormal  = "normal"  // Important events only (default)
	NotifyMuted   = "muted"   // Silent/DND mode - batch for later
)

// FormatAgentDescription creates a description string from agent fields.
func FormatAgentDescription(title string, fields *AgentFields) string {
	if fields == nil {
		return title
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("role_type: %s", fields.RoleType))

	if fields.Rig != "" {
		lines = append(lines, fmt.Sprintf("rig: %s", fields.Rig))
	} else {
		lines = append(lines, "rig: null")
	}

	lines = append(lines, fmt.Sprintf("agent_state: %s", fields.AgentState))

	if fields.HookBead != "" {
		lines = append(lines, fmt.Sprintf("hook_bead: %s", fields.HookBead))
	} else {
		lines = append(lines, "hook_bead: null")
	}

	// Note: role_bead field no longer written - role definitions are config-based

	if fields.CleanupStatus != "" {
		lines = append(lines, fmt.Sprintf("cleanup_status: %s", fields.CleanupStatus))
	} else {
		lines = append(lines, "cleanup_status: null")
	}

	if fields.ActiveMR != "" {
		lines = append(lines, fmt.Sprintf("active_mr: %s", fields.ActiveMR))
	} else {
		lines = append(lines, "active_mr: null")
	}

	if fields.NotificationLevel != "" {
		lines = append(lines, fmt.Sprintf("notification_level: %s", fields.NotificationLevel))
	} else {
		lines = append(lines, "notification_level: null")
	}

	return strings.Join(lines, "\n")
}

// ParseAgentFields extracts agent fields from an issue's description.
func ParseAgentFields(description string) *AgentFields {
	fields := &AgentFields{}

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
		case "role_type":
			fields.RoleType = value
		case "rig":
			fields.Rig = value
		case "agent_state":
			fields.AgentState = value
		case "hook_bead":
			fields.HookBead = value
		case "role_bead":
			// Ignored - role definitions are now config-based (backward compat)
		case "cleanup_status":
			fields.CleanupStatus = value
		case "active_mr":
			fields.ActiveMR = value
		case "notification_level":
			fields.NotificationLevel = value
		}
	}

	return fields
}

// CreateAgentBead creates an agent bead for tracking agent lifecycle.
// The ID format is: <prefix>-<rig>-<role>-<name> (e.g., gt-gastown-polecat-Toast)
// Use AgentBeadID() helper to generate correct IDs.
// The created_by field is populated from BD_ACTOR env var for provenance tracking.
//
// This function automatically ensures custom types are configured in the target
// database before creating the bead. This handles multi-repo routing scenarios
// where the bead may be routed to a different database than the one this wrapper
// is connected to.
func (b *Beads) CreateAgentBead(id, title string, fields *AgentFields) (*Issue, error) {
	// Resolve where this bead will actually be written (handles multi-repo routing)
	targetDir := ResolveRoutingTarget(b.getTownRoot(), id, b.getResolvedBeadsDir())

	// Ensure target database has custom types configured
	// This is cached (sentinel file + in-memory) so repeated calls are fast
	if err := EnsureCustomTypes(targetDir); err != nil {
		return nil, fmt.Errorf("prepare target for agent bead %s: %w", id, err)
	}

	description := FormatAgentDescription(title, fields)

	args := []string{"create", "--json",
		"--id=" + id,
		"--title=" + title,
		"--description=" + description,
		"--type=agent",
		"--labels=gt:agent",
		"--pinned", // Agent beads must be pinned for AttachMolecule (fix: bd-3q6.5-1)
	}
	if NeedsForceForID(id) {
		args = append(args, "--force")
	}

	// Default actor from BD_ACTOR env var for provenance tracking
	// Uses getActor() to respect isolated mode (tests)
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

	// Note: role slot no longer set - role definitions are config-based

	// Set the hook slot if specified (this is the authoritative storage)
	// This fixes the slot inconsistency bug where bead status is 'hooked' but
	// agent's hook slot is empty. See mi-619.
	// Must run from targetDir since that's where the agent bead was created
	if fields != nil && fields.HookBead != "" {
		if err := runSlotSet(targetDir, id, "hook", fields.HookBead); err != nil {
			// Non-fatal: warn but continue - description text has the backup
			fmt.Printf("Warning: could not set hook slot: %v\n", err)
		}
	}

	return &issue, nil
}

// CreateOrReopenAgentBead creates an agent bead or reopens an existing one.
// This handles the case where a polecat is nuked and re-spawned with the same name:
// the old agent bead exists as a closed bead, so we reopen and update it instead of
// failing with a UNIQUE constraint error.
//
// NOTE: This does NOT handle tombstones. If the old bead was hard-deleted (creating
// a tombstone), this function will fail. Use CloseAndClearAgentBead instead of DeleteAgentBead
// when cleaning up agent beads to ensure they can be reopened later.
//
//
// The function:
// 1. Tries to create the agent bead
// 2. If UNIQUE constraint fails, reopens the existing bead and updates its fields
func (b *Beads) CreateOrReopenAgentBead(id, title string, fields *AgentFields) (*Issue, error) {
	// First try to create the bead
	issue, err := b.CreateAgentBead(id, title, fields)
	if err == nil {
		return issue, nil
	}

	// Check if it's a UNIQUE constraint error (SQLite or Dolt/MySQL)
	errStr := err.Error()
	isDuplicateKey := strings.Contains(errStr, "UNIQUE constraint failed") || // SQLite
		strings.Contains(errStr, "duplicate primary key") // Dolt/MySQL Error 1062
	if !isDuplicateKey {
		return nil, err
	}

	// Resolve where this bead lives (for slot operations)
	targetDir := ResolveRoutingTarget(b.getTownRoot(), id, b.getResolvedBeadsDir())

	// The bead already exists (should be closed from previous polecat lifecycle)
	// Reopen it and update its fields
	if _, reopenErr := b.run("reopen", id, "--reason=re-spawning agent"); reopenErr != nil {
		// If reopen fails, the bead might already be open - continue with update
		if !strings.Contains(reopenErr.Error(), "already open") {
			return nil, fmt.Errorf("reopening existing agent bead: %w (original error: %v)", reopenErr, err)
		}
	}

	// Update the bead with new fields and set status to pinned.
	// Agent beads must be pinned for AttachMolecule to work (fix: bd-3q6.5-1).
	// After reopen, status is "open" - we need to explicitly pin it.
	description := FormatAgentDescription(title, fields)
	pinnedStatus := StatusPinned
	updateOpts := UpdateOptions{
		Title:       &title,
		Description: &description,
		Status:      &pinnedStatus,
	}
	if err := b.Update(id, updateOpts); err != nil {
		return nil, fmt.Errorf("updating reopened agent bead: %w", err)
	}

	// Note: role slot no longer set - role definitions are config-based

	// Clear any existing hook slot (handles stale state from previous lifecycle)
	// Must run from targetDir since that's where the agent bead lives
	_ = runSlotClear(targetDir, id, "hook")

	// Set the hook slot if specified
	// Must run from targetDir since that's where the agent bead lives
	if fields != nil && fields.HookBead != "" {
		if err := runSlotSet(targetDir, id, "hook", fields.HookBead); err != nil {
			// Non-fatal: warn but continue - description text has the backup
			fmt.Printf("Warning: could not set hook slot: %v\n", err)
		}
	}

	// Return the updated bead
	return b.Show(id)
}

// UpdateAgentState updates the agent_state field in an agent bead.
// Optionally updates hook_bead if provided.
//
// IMPORTANT: This function uses the proper bd commands to update agent fields:
// - `bd agent state` for agent_state (uses SQLite column directly)
// - `bd slot set/clear` for hook_bead (uses SQLite column directly)
//
// This ensures consistency with `bd slot show` and other beads commands.
// Previously, this function embedded these fields in the description text,
// which caused inconsistencies with bd slot commands (see GH #gt-9v52).
func (b *Beads) UpdateAgentState(id string, state string, hookBead *string) error {
	// Update agent state using bd agent state command
	// This updates the agent_state column directly in SQLite
	_, err := b.run("agent", "state", id, state)
	if err != nil {
		return fmt.Errorf("updating agent state: %w", err)
	}

	// Update hook_bead if provided
	if hookBead != nil {
		if *hookBead != "" {
			// Set the hook using bd slot set
			// This updates the hook_bead column directly in SQLite
			_, err = b.run("slot", "set", id, "hook", *hookBead)
			if err != nil {
				// If slot is already occupied, clear it first then retry
				// This handles re-slinging scenarios where we're updating the hook
				errStr := err.Error()
				if strings.Contains(errStr, "already occupied") {
					_, _ = b.run("slot", "clear", id, "hook")
					_, err = b.run("slot", "set", id, "hook", *hookBead)
				}
				if err != nil {
					return fmt.Errorf("setting hook: %w", err)
				}
			}
		} else {
			// Clear the hook
			_, err = b.run("slot", "clear", id, "hook")
			if err != nil {
				return fmt.Errorf("clearing hook: %w", err)
			}
		}
	}

	return nil
}

// SetHookBead sets the hook_bead slot on an agent bead.
// This is a convenience wrapper that only sets the hook without changing agent_state.
// Per gt-zecmc: agent_state ("running", "dead", "idle") is observable from tmux
// and should not be recorded in beads ("discover, don't track" principle).
func (b *Beads) SetHookBead(agentBeadID, hookBeadID string) error {
	// Set the hook using bd slot set
	// This updates the hook_bead column directly in SQLite
	_, err := b.run("slot", "set", agentBeadID, "hook", hookBeadID)
	if err != nil {
		// If slot is already occupied, clear it first then retry
		errStr := err.Error()
		if strings.Contains(errStr, "already occupied") {
			_, _ = b.run("slot", "clear", agentBeadID, "hook")
			_, err = b.run("slot", "set", agentBeadID, "hook", hookBeadID)
		}
		if err != nil {
			return fmt.Errorf("setting hook: %w", err)
		}
	}
	return nil
}

// ClearHookBead clears the hook_bead slot on an agent bead.
// Used when work is complete or unslung.
func (b *Beads) ClearHookBead(agentBeadID string) error {
	_, err := b.run("slot", "clear", agentBeadID, "hook")
	if err != nil {
		return fmt.Errorf("clearing hook: %w", err)
	}
	return nil
}

// UpdateAgentCleanupStatus updates the cleanup_status field in an agent bead.
// This is called by the polecat to self-report its git state (ZFC compliance).
// Valid statuses: clean, has_uncommitted, has_stash, has_unpushed
func (b *Beads) UpdateAgentCleanupStatus(id string, cleanupStatus string) error {
	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseAgentFields(issue.Description)
	fields.CleanupStatus = cleanupStatus

	// Format new description
	description := FormatAgentDescription(issue.Title, fields)

	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateAgentActiveMR updates the active_mr field in an agent bead.
// This links the agent to their current merge request for traceability.
// Pass empty string to clear the field (e.g., after merge completes).
func (b *Beads) UpdateAgentActiveMR(id string, activeMR string) error {
	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseAgentFields(issue.Description)
	fields.ActiveMR = activeMR

	// Format new description
	description := FormatAgentDescription(issue.Title, fields)

	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateAgentNotificationLevel updates the notification_level field in an agent bead.
// Valid levels: verbose, normal, muted (DND mode).
// Pass empty string to reset to default (normal).
func (b *Beads) UpdateAgentNotificationLevel(id string, level string) error {
	// Validate level
	if level != "" && level != NotifyVerbose && level != NotifyNormal && level != NotifyMuted {
		return fmt.Errorf("invalid notification level %q: must be verbose, normal, or muted", level)
	}

	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseAgentFields(issue.Description)
	fields.NotificationLevel = level

	// Format new description
	description := FormatAgentDescription(issue.Title, fields)

	return b.Update(id, UpdateOptions{Description: &description})
}

// GetAgentNotificationLevel returns the notification level for an agent.
// Returns "normal" if not set (the default).
func (b *Beads) GetAgentNotificationLevel(id string) (string, error) {
	_, fields, err := b.GetAgentBead(id)
	if err != nil {
		return "", err
	}
	if fields == nil {
		return NotifyNormal, nil
	}
	if fields.NotificationLevel == "" {
		return NotifyNormal, nil
	}
	return fields.NotificationLevel, nil
}

// DeleteAgentBead permanently deletes an agent bead.
// Uses --hard --force for immediate permanent deletion (no tombstone).
//
// WARNING: Due to a bd bug, --hard --force still creates tombstones instead of
// truly deleting. This breaks CreateOrReopenAgentBead because tombstones are
// invisible to bd show/reopen but still block bd create via UNIQUE constraint.
//
//
// WORKAROUND: Use CloseAndClearAgentBead instead, which allows CreateOrReopenAgentBead
// to reopen the bead on re-spawn.
func (b *Beads) DeleteAgentBead(id string) error {
	_, err := b.run("delete", id, "--hard", "--force")
	return err
}

// CloseAndClearAgentBead closes an agent bead (soft delete).
// This is the recommended way to clean up agent beads because CreateOrReopenAgentBead
// can reopen closed beads when re-spawning polecats with the same name.
//
// This is a workaround for the bd tombstone bug where DeleteAgentBead creates
// tombstones that cannot be reopened.
//
// To emulate the clean slate of delete --force --hard, this clears all mutable
// fields (hook_bead, active_mr, cleanup_status, agent_state) before closing.
func (b *Beads) CloseAndClearAgentBead(id, reason string) error {
	// Clear mutable fields to emulate delete --force --hard behavior.
	// This ensures reopened agent beads don't have stale state.

	// First get current issue to preserve immutable fields
	issue, err := b.Show(id)
	if err != nil {
		// If we can't read the issue, still attempt to close
		args := []string{"close", id}
		if reason != "" {
			args = append(args, "--reason="+reason)
		}
		_, closeErr := b.run(args...)
		return closeErr
	}

	// Parse existing fields and clear mutable ones
	fields := ParseAgentFields(issue.Description)
	fields.HookBead = ""     // Clear hook_bead
	fields.ActiveMR = ""     // Clear active_mr
	fields.CleanupStatus = "" // Clear cleanup_status
	fields.AgentState = "closed"

	// Update description with cleared fields
	description := FormatAgentDescription(issue.Title, fields)
	if err := b.Update(id, UpdateOptions{Description: &description}); err != nil {
		// Non-fatal: continue with close even if update fails
	}

	// Also clear the hook slot in the database
	if err := b.ClearHookBead(id); err != nil {
		// Non-fatal
	}

	args := []string{"close", id}
	if reason != "" {
		args = append(args, "--reason="+reason)
	}
	_, err = b.run(args...)
	return err
}

// GetAgentBead retrieves an agent bead by ID.
// Returns nil if not found.
func (b *Beads) GetAgentBead(id string) (*Issue, *AgentFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !HasLabel(issue, "gt:agent") {
		return nil, nil, fmt.Errorf("issue %s is not an agent bead (missing gt:agent label)", id)
	}

	fields := ParseAgentFields(issue.Description)
	return issue, fields, nil
}

// ListAgentBeads returns all agent beads in a single query.
// Returns a map of agent bead ID to Issue.
func (b *Beads) ListAgentBeads() (map[string]*Issue, error) {
	out, err := b.run("list", "--label=gt:agent", "--json")
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
