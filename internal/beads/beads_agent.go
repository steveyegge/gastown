// Package beads provides agent bead management.
package beads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"

	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/telemetry"
)

// lockAgentBead acquires an exclusive file lock for a specific agent bead ID.
// This prevents concurrent read-modify-write races in methods like
// CreateOrReopenAgentBead, ResetAgentBeadForReuse, and UpdateAgentDescriptionFields.
// Caller must defer fl.Unlock().
func (b *Beads) lockAgentBead(id string) (*flock.Flock, error) {
	lockDir := filepath.Join(b.getResolvedBeadsDir(), ".locks")
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("creating bead lock dir: %w", err)
	}
	lockPath := filepath.Join(lockDir, fmt.Sprintf("agent-%s.lock", id))
	fl := flock.New(lockPath)
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("acquiring agent bead lock for %s: %w", id, err)
	}
	return fl, nil
}

// AgentFields holds structured fields for agent beads.
// These are stored as "key: value" lines in the description.
type AgentFields struct {
	RoleType          string // polecat, witness, refinery, deacon, mayor
	Rig               string // Rig name (empty for global agents like mayor/deacon)
	AgentState        string // spawning, working, done, stuck
	HookBead          string // Currently pinned work bead ID
	CleanupStatus     string // ZFC: polecat self-reports git state (clean, has_uncommitted, has_stash, has_unpushed)
	ActiveMR          string // Currently active merge request bead ID (for traceability)
	NotificationLevel string // DND mode: verbose, normal, muted (default: normal)
	Mode              string // Execution mode: "" (normal) or "ralph" (Ralph Wiggum loop)
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

	if fields.Mode != "" {
		lines = append(lines, fmt.Sprintf("mode: %s", fields.Mode))
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
		case "mode":
			fields.Mode = value
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
	// Guard against flag-like titles (gt-e0kx5: --help garbage beads)
	if IsFlagLikeTitle(title) {
		return nil, fmt.Errorf("refusing to create agent bead: %w (got %q)", ErrFlagTitle, title)
	}

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
		"--ephemeral",
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
	// Use a target Beads instance with proper BEADS_DIR routing (gt-wrnwq).
	if fields != nil && fields.HookBead != "" {
		target := b
		if targetDir != b.getResolvedBeadsDir() {
			target = NewWithBeadsDir(filepath.Dir(targetDir), targetDir)
		}
		if err := target.SetHookBead(id, fields.HookBead); err != nil {
			// Non-fatal: warn but continue - description text has the backup
			style.PrintWarning("could not set hook slot: %v", err)
		}
	}

	return &issue, nil
}

// CreateOrReopenAgentBead creates an agent bead or reopens an existing one.
// This handles the case where a polecat is nuked and re-spawned with the same name:
// the old agent bead exists (open or closed), so we update it instead of
// failing with a UNIQUE constraint error.
//
// The function:
// 1. Tries to create the agent bead
// 2. If create fails, checks if bead exists (via bd show)
// 3. If bead exists and is closed, reopens it
// 4. Updates the bead with new fields regardless of prior state
//
// This is robust against Dolt backend issues where bd close/reopen may fail:
// - If nuke used ResetAgentBeadForReuse, bead is open → update directly
// - If bead is closed (legacy state), reopen then update
// - If bead is in unknown state, falls back to show+update
func (b *Beads) CreateOrReopenAgentBead(id, title string, fields *AgentFields) (*Issue, error) {
	// First try to create the bead (no lock needed - create is atomic)
	issue, err := b.CreateAgentBead(id, title, fields)
	if err == nil {
		return issue, nil
	}

	// Create failed - need to do Show→Reopen→Update which requires locking
	// to prevent concurrent modifications (e.g., nuke clearing fields while
	// spawn is updating them). See gt-joazs.
	fl, lockErr := b.lockAgentBead(id)
	if lockErr != nil {
		return nil, fmt.Errorf("locking agent bead %s: %w", id, lockErr)
	}
	defer func() { _ = fl.Unlock() }()

	// Create failed - check if bead already exists (handles both open and closed states)
	createErr := err

	// Resolve where this bead lives. For cross-rig beads (e.g., bd-beads-polecat-obsidian
	// created from gastown), the target database differs from b's local database.
	// We need a Beads instance pointed at the target to run show/update/reopen,
	// because bd show/update don't route cross-rig when BEADS_DIR is set (gt-mh3tb).
	targetDir := ResolveRoutingTarget(b.getTownRoot(), id, b.getResolvedBeadsDir())
	target := b
	if targetDir != b.getResolvedBeadsDir() {
		target = NewWithBeadsDir(filepath.Dir(targetDir), targetDir)
	}

	existing, showErr := target.Show(id)
	if showErr != nil {
		// Bead doesn't exist (or can't be read) - return original create error
		return nil, createErr
	}

	// If bead is closed, reopen it first
	if existing.Status == "closed" {
		if _, reopenErr := target.run("reopen", id, "--reason=re-spawning agent"); reopenErr != nil {
			// Reopen failed - try setting status to open via update as fallback
			// This handles Dolt backends where bd reopen may not work
			openStatus := "open"
			if updateErr := target.Update(id, UpdateOptions{Status: &openStatus}); updateErr != nil {
				return nil, fmt.Errorf("could not reopen agent bead %s (reopen: %v, update: %v, original: %v)",
					id, reopenErr, updateErr, createErr)
			}
		}
	}

	// Update the bead with new fields and ensure type=agent (gt-dr02sy:
	// old beads may have type=task, which breaks bd slot set).
	description := FormatAgentDescription(title, fields)
	updateOpts := UpdateOptions{
		Title:       &title,
		Description: &description,
		SetLabels:   []string{"gt:agent"},
	}
	if err := target.Update(id, updateOpts); err != nil {
		return nil, fmt.Errorf("updating agent bead: %w", err)
	}
	// Fix type separately — UpdateOptions doesn't support type changes
	if _, err := target.run("update", id, "--type=agent"); err != nil {
		return nil, fmt.Errorf("fixing agent bead type: %w", err)
	}
	// Ensure agent bead is ephemeral (wisp) — agent operational state has
	// zero git history consumers (gt-bewatn.9)
	if _, err := target.run("update", id, "--ephemeral"); err != nil {
		// Non-fatal: the bead is functional without ephemeral flag
		style.PrintWarning("could not mark agent bead as ephemeral: %v", err)
	}

	// Note: role slot no longer set - role definitions are config-based

	// Clear any existing hook slot (handles stale state from previous lifecycle)
	// Use target Beads instance with proper BEADS_DIR routing (gt-wrnwq).
	_ = target.ClearHookBead(id)

	// Set the hook slot if specified
	if fields != nil && fields.HookBead != "" {
		if err := target.SetHookBead(id, fields.HookBead); err != nil {
			// Non-fatal: warn but continue - description text has the backup
			style.PrintWarning("could not set hook slot: %v", err)
		}
	}

	// Return the updated bead
	return target.Show(id)
}

// ResetAgentBeadForReuse clears all mutable fields on an agent bead without closing it.
// This is the preferred cleanup method during polecat nuke because it avoids the
// close/reopen cycle that fails on Dolt backends (tombstone operations not supported,
// bd reopen failures). By keeping the bead open with agent_state="nuked",
// CreateOrReopenAgentBead can simply update it on re-spawn without needing reopen.
//
// This is the standard nuke path (gt-14b8o).
func (b *Beads) ResetAgentBeadForReuse(id, reason string) error {
	// Lock the agent bead to prevent concurrent read-modify-write races.
	// Without this, a concurrent CreateOrReopenAgentBead could overwrite
	// the nuked state we're about to set. See gt-joazs.
	fl, lockErr := b.lockAgentBead(id)
	if lockErr != nil {
		return fmt.Errorf("locking agent bead %s: %w", id, lockErr)
	}
	defer func() { _ = fl.Unlock() }()

	// Resolve where this bead lives (handles cross-rig routing).
	// Without this, cross-rig agent beads (e.g., bd-beads-polecat-obsidian
	// from gastown) would be looked up in the local rig's database and fail.
	targetDir := ResolveRoutingTarget(b.getTownRoot(), id, b.getResolvedBeadsDir())
	target := b
	if targetDir != b.getResolvedBeadsDir() {
		target = NewWithBeadsDir(filepath.Dir(targetDir), targetDir)
	}

	// Get current issue to preserve immutable fields (title, role_type, rig)
	issue, err := target.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields and clear mutable ones
	fields := ParseAgentFields(issue.Description)
	fields.HookBead = ""      // Clear hook_bead
	fields.ActiveMR = ""      // Clear active_mr
	fields.CleanupStatus = "" // Clear cleanup_status
	fields.AgentState = "nuked"

	// Update description with cleared fields
	description := FormatAgentDescription(issue.Title, fields)
	if err := target.Update(id, UpdateOptions{Description: &description}); err != nil {
		return fmt.Errorf("resetting agent bead fields: %w", err)
	}

	// Also clear the hook slot in the database
	_ = target.ClearHookBead(id)

	return nil
}

// UpdateAgentState updates the agent_state field in an agent bead.
// Optionally updates hook_bead if provided.
//
// IMPORTANT: This function uses the proper bd commands to update agent fields:
// - `bd agent state` for agent_state (uses the database column directly)
// - `bd slot set/clear` for hook_bead (uses the database column directly)
//
// This ensures consistency with `bd slot show` and other beads commands.
// Previously, this function embedded these fields in the description text,
// which caused inconsistencies with bd slot commands (see GH #gt-9v52).
func (b *Beads) UpdateAgentState(id string, state string, hookBead *string) (retErr error) {
	defer func() { telemetry.RecordAgentStateChange(context.Background(), id, state, hookBead, retErr) }()
	// Update agent state using bd agent state command
	// Use runWithRouting so bd can resolve cross-prefix agent beads (e.g., wa-*
	// agent beads from hq context) via routes.jsonl instead of BEADS_DIR.
	_, err := b.runWithRouting("agent", "state", id, state)
	if err != nil {
		return fmt.Errorf("updating agent state: %w", err)
	}

	// Update hook_bead if provided
	// Use runWithRouting for slot ops so bd can resolve cross-prefix beads
	// (e.g., hq-* hook beads on gt-* agent beads) via routes.jsonl.
	if hookBead != nil {
		if *hookBead != "" {
			// Set the hook using bd slot set
			_, err = b.runWithRouting("slot", "set", id, "hook", *hookBead)
			if err != nil {
				// If slot is already occupied, clear it first then retry
				// This handles re-slinging scenarios where we're updating the hook
				errStr := err.Error()
				if strings.Contains(errStr, "already occupied") {
					_, _ = b.runWithRouting("slot", "clear", id, "hook")
					_, err = b.runWithRouting("slot", "set", id, "hook", *hookBead)
				}
				if err != nil {
					return fmt.Errorf("setting hook: %w", err)
				}
			}
		} else {
			// Clear the hook
			_, err = b.runWithRouting("slot", "clear", id, "hook")
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
	// Use runWithRouting so bd can resolve cross-prefix beads (e.g., hq-* hook
	// beads on gt-* agent beads) via routes.jsonl instead of BEADS_DIR.
	_, err := b.runWithRouting("slot", "set", agentBeadID, "hook", hookBeadID)
	if err != nil {
		// If slot is already occupied, clear it first then retry
		errStr := err.Error()
		if strings.Contains(errStr, "already occupied") {
			_, _ = b.runWithRouting("slot", "clear", agentBeadID, "hook")
			_, err = b.runWithRouting("slot", "set", agentBeadID, "hook", hookBeadID)
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
	// Use runWithRouting so bd can resolve cross-prefix beads via routes.jsonl.
	_, err := b.runWithRouting("slot", "clear", agentBeadID, "hook")
	if err != nil {
		return fmt.Errorf("clearing hook: %w", err)
	}
	return nil
}

// AgentFieldUpdates specifies which agent description fields to update.
// Only non-nil fields are modified; nil fields are left unchanged.
// This allows multiple fields to be updated in a single read-modify-write
// cycle, avoiding races where concurrent callers overwrite each other's changes.
type AgentFieldUpdates struct {
	CleanupStatus     *string
	ActiveMR          *string
	NotificationLevel *string
	Mode              *string
}

// UpdateAgentDescriptionFields atomically updates one or more agent description
// fields in a single Show-Parse-Modify-Update cycle. This prevents the race
// condition where concurrent callers updating different fields overwrite each
// other because the entire description is replaced.
func (b *Beads) UpdateAgentDescriptionFields(id string, updates AgentFieldUpdates) error {
	// Validate notification level if provided
	if updates.NotificationLevel != nil {
		level := *updates.NotificationLevel
		if level != "" && level != NotifyVerbose && level != NotifyNormal && level != NotifyMuted {
			return fmt.Errorf("invalid notification level %q: must be verbose, normal, or muted", level)
		}
	}

	// Lock the agent bead to prevent concurrent read-modify-write races.
	// Without this, concurrent callers updating different fields could overwrite
	// each other's changes. See gt-joazs.
	fl, lockErr := b.lockAgentBead(id)
	if lockErr != nil {
		return fmt.Errorf("locking agent bead %s: %w", id, lockErr)
	}
	defer func() { _ = fl.Unlock() }()

	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	fields := ParseAgentFields(issue.Description)

	if updates.CleanupStatus != nil {
		fields.CleanupStatus = *updates.CleanupStatus
	}
	if updates.ActiveMR != nil {
		fields.ActiveMR = *updates.ActiveMR
	}
	if updates.NotificationLevel != nil {
		fields.NotificationLevel = *updates.NotificationLevel
	}
	if updates.Mode != nil {
		fields.Mode = *updates.Mode
	}

	description := FormatAgentDescription(issue.Title, fields)
	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateAgentCleanupStatus updates the cleanup_status field in an agent bead.
// This is called by the polecat to self-report its git state (ZFC compliance).
// Valid statuses: clean, has_uncommitted, has_stash, has_unpushed
func (b *Beads) UpdateAgentCleanupStatus(id string, cleanupStatus string) error {
	return b.UpdateAgentDescriptionFields(id, AgentFieldUpdates{CleanupStatus: &cleanupStatus})
}

// UpdateAgentActiveMR updates the active_mr field in an agent bead.
// This links the agent to their current merge request for traceability.
// Pass empty string to clear the field (e.g., after merge completes).
func (b *Beads) UpdateAgentActiveMR(id string, activeMR string) error {
	return b.UpdateAgentDescriptionFields(id, AgentFieldUpdates{ActiveMR: &activeMR})
}

// UpdateAgentNotificationLevel updates the notification_level field in an agent bead.
// Valid levels: verbose, normal, muted (DND mode).
// Pass empty string to reset to default (normal).
func (b *Beads) UpdateAgentNotificationLevel(id string, level string) error {
	return b.UpdateAgentDescriptionFields(id, AgentFieldUpdates{NotificationLevel: &level})
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

	if !IsAgentBead(issue) {
		return nil, nil, fmt.Errorf("issue %s is not an agent bead (type=%s)", id, issue.Type)
	}

	fields := ParseAgentFields(issue.Description)
	return issue, fields, nil
}

// ListAgentBeads returns all agent beads in a single query.
// Returns a map of agent bead ID to Issue.
//
// Queries both the wisps table (primary, for migrated agent beads) and
// the issues table (backward compat during migration). Wisps take
// precedence for duplicate IDs.
func (b *Beads) ListAgentBeads() (map[string]*Issue, error) {
	result := make(map[string]*Issue)

	// Query wisps table first (primary source after agent bead migration).
	// Gracefully ignore errors — wisps table may not exist yet.
	if wispBeads, _ := b.ListAgentBeadsFromWisps(); len(wispBeads) > 0 {
		for id, issue := range wispBeads {
			result[id] = issue
		}
	}

	// Also query issues table (backward compat during migration).
	out, err := b.run("list", "--label=gt:agent", "--json")
	if err != nil && len(result) == 0 {
		return nil, err
	}
	if err == nil {
		var issues []*Issue
		if jsonErr := json.Unmarshal(out, &issues); jsonErr == nil {
			for _, issue := range issues {
				if _, exists := result[issue.ID]; !exists {
					result[issue.ID] = issue
				}
			}
		}
	}

	return result, nil
}

// ListAgentBeadsFromWisps queries the wisps table for agent beads.
// Returns nil, nil if the wisps table doesn't exist yet or has no agent beads.
func (b *Beads) ListAgentBeadsFromWisps() (map[string]*Issue, error) {
	out, err := b.run("mol", "wisp", "list", "--json")
	if err != nil {
		return nil, nil // Wisps table may not exist yet
	}

	// bd mol wisp list --json returns {"wisps": [...], "count": N, ...}
	var wrapper struct {
		Wisps []*Issue `json:"wisps"`
	}
	if err := json.Unmarshal(out, &wrapper); err != nil {
		return nil, nil
	}

	result := make(map[string]*Issue)
	for _, w := range wrapper.Wisps {
		// Check by type/label first (works when fields are present)
		if IsAgentBead(w) {
			result[w.ID] = w
			continue
		}
		// Fallback: wisps JSON may omit issue_type/labels fields.
		// Detect agent beads by ID pattern (prefix-rig-role format).
		if isAgentBeadByID(w.ID) {
			result[w.ID] = w
		}
	}

	return result, nil
}

// isAgentBeadByID detects agent beads by their ID naming convention.
// Agent bead IDs follow the pattern: prefix-rig-role[-name]
// where role is one of: witness, refinery, crew, polecat, deacon, mayor.
func isAgentBeadByID(id string) bool {
	parts := strings.Split(id, "-")
	if len(parts) < 3 {
		return false
	}
	// Check if any part matches an agent role keyword
	for _, part := range parts[2:] {
		switch part {
		case "witness", "refinery", "crew", "polecat", "deacon", "mayor":
			return true
		}
	}
	return false
}

// ListWispIDs returns a set of all wisp IDs in the wisps table.
// This is useful for existence checks where wisp metadata (type, labels)
// may not be available in the list output.
func (b *Beads) ListWispIDs() (map[string]bool, error) {
	out, err := b.run("mol", "wisp", "list", "--json")
	if err != nil {
		return nil, nil
	}

	var wrapper struct {
		Wisps []struct {
			ID string `json:"id"`
		} `json:"wisps"`
	}
	if err := json.Unmarshal(out, &wrapper); err != nil {
		return nil, nil
	}

	result := make(map[string]bool, len(wrapper.Wisps))
	for _, w := range wrapper.Wisps {
		result[w.ID] = true
	}
	return result, nil
}
