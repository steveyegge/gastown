// Package beads provides field parsing utilities for structured issue descriptions.
package beads

import (
	"fmt"
	"strings"
)

// Note: AgentFields, ParseAgentFields, FormatAgentDescription, and CreateAgentBead are in beads.go

// ParseAgentFieldsFromDescription is an alias for ParseAgentFields.
// Used by daemon for compatibility.
func ParseAgentFieldsFromDescription(description string) *AgentFields {
	return ParseAgentFields(description)
}

// AttachmentFields holds the attachment info for pinned beads.
// These fields track which molecule is attached to a handoff/pinned bead.
type AttachmentFields struct {
	AttachedMolecule string // Root issue ID of the attached molecule
	AttachedAt       string // ISO 8601 timestamp when attached
	AttachedArgs     string // Natural language args passed via gt sling --args (no-tmux mode)
	DispatchedBy     string // Agent ID that dispatched this work (for completion notification)
	NoMerge          bool   // If true, gt done skips merge queue (for upstream PRs/human review)
	MergeStrategy    string // Merge strategy: direct (push to main), mr (refinery), local (merge locally)
	ConvoyOwned      bool   // If true, convoy is caller-managed (no witness/refinery)
}

// ParseAttachmentFields extracts attachment fields from an issue's description.
// Fields are expected as "key: value" lines. Returns nil if no attachment fields found.
func ParseAttachmentFields(issue *Issue) *AttachmentFields {
	if issue == nil || issue.Description == "" {
		return nil
	}

	fields := &AttachmentFields{}
	hasFields := false

	for _, line := range strings.Split(issue.Description, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for "key: value" pattern
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		if value == "" {
			continue
		}

		// Map keys to fields (case-insensitive)
		switch strings.ToLower(key) {
		case "attached_molecule", "attached-molecule", "attachedmolecule":
			fields.AttachedMolecule = value
			hasFields = true
		case "attached_at", "attached-at", "attachedat":
			fields.AttachedAt = value
			hasFields = true
		case "attached_args", "attached-args", "attachedargs":
			fields.AttachedArgs = value
			hasFields = true
		case "dispatched_by", "dispatched-by", "dispatchedby":
			fields.DispatchedBy = value
			hasFields = true
		case "no_merge", "no-merge", "nomerge":
			fields.NoMerge = strings.ToLower(value) == "true"
			hasFields = true
		case "merge_strategy", "merge-strategy", "mergestrategy":
			fields.MergeStrategy = value
			hasFields = true
		case "convoy_owned", "convoy-owned", "convoyowned":
			fields.ConvoyOwned = strings.ToLower(value) == "true"
			hasFields = true
		}
	}

	if !hasFields {
		return nil
	}
	return fields
}

// FormatAttachmentFields formats AttachmentFields as a string suitable for an issue description.
// Only non-empty fields are included.
func FormatAttachmentFields(fields *AttachmentFields) string {
	if fields == nil {
		return ""
	}

	var lines []string

	if fields.AttachedMolecule != "" {
		lines = append(lines, "attached_molecule: "+fields.AttachedMolecule)
	}
	if fields.AttachedAt != "" {
		lines = append(lines, "attached_at: "+fields.AttachedAt)
	}
	if fields.AttachedArgs != "" {
		lines = append(lines, "attached_args: "+fields.AttachedArgs)
	}
	if fields.DispatchedBy != "" {
		lines = append(lines, "dispatched_by: "+fields.DispatchedBy)
	}
	if fields.NoMerge {
		lines = append(lines, "no_merge: true")
	}
	if fields.MergeStrategy != "" {
		lines = append(lines, "merge_strategy: "+fields.MergeStrategy)
	}
	if fields.ConvoyOwned {
		lines = append(lines, "convoy_owned: true")
	}

	return strings.Join(lines, "\n")
}

// SetAttachmentFields updates an issue's description with the given attachment fields.
// Existing attachment field lines are replaced; other content is preserved.
// Returns the new description string.
func SetAttachmentFields(issue *Issue, fields *AttachmentFields) string {
	// Known attachment field keys (lowercase)
	attachmentKeys := map[string]bool{
		"attached_molecule": true,
		"attached-molecule": true,
		"attachedmolecule":  true,
		"attached_at":       true,
		"attached-at":       true,
		"attachedat":        true,
		"attached_args":     true,
		"attached-args":     true,
		"attachedargs":      true,
		"dispatched_by":     true,
		"dispatched-by":     true,
		"dispatchedby":      true,
		"no_merge":          true,
		"no-merge":          true,
		"nomerge":           true,
		"merge_strategy":    true,
		"merge-strategy":    true,
		"mergestrategy":     true,
		"convoy_owned":      true,
		"convoy-owned":      true,
		"convoyowned":       true,
	}

	// Collect non-attachment lines from existing description
	var otherLines []string
	if issue != nil && issue.Description != "" {
		for _, line := range strings.Split(issue.Description, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				// Preserve blank lines in content
				otherLines = append(otherLines, line)
				continue
			}

			// Check if this is an attachment field line
			colonIdx := strings.Index(trimmed, ":")
			if colonIdx == -1 {
				otherLines = append(otherLines, line)
				continue
			}

			key := strings.ToLower(strings.TrimSpace(trimmed[:colonIdx]))
			if !attachmentKeys[key] {
				otherLines = append(otherLines, line)
			}
			// Skip attachment field lines - they'll be replaced
		}
	}

	// Build new description: attachment fields first, then other content
	formatted := FormatAttachmentFields(fields)

	// Trim trailing blank lines from other content
	for len(otherLines) > 0 && strings.TrimSpace(otherLines[len(otherLines)-1]) == "" {
		otherLines = otherLines[:len(otherLines)-1]
	}
	// Trim leading blank lines from other content
	for len(otherLines) > 0 && strings.TrimSpace(otherLines[0]) == "" {
		otherLines = otherLines[1:]
	}

	if formatted == "" {
		return strings.Join(otherLines, "\n")
	}
	if len(otherLines) == 0 {
		return formatted
	}

	return formatted + "\n\n" + strings.Join(otherLines, "\n")
}

// MRFields holds the structured fields for a merge-request issue.
// These fields are stored as key: value lines in the issue description.
type MRFields struct {
	Branch      string // Source branch name (e.g., "polecat/Nux/gt-xyz")
	Target      string // Target branch (e.g., "main" or "integration/gt-epic")
	SourceIssue string // The work item being merged (e.g., "gt-xyz")
	Worker      string // Who did the work
	Rig         string // Which rig
	MergeCommit string // SHA of merge commit (set on close)
	CloseReason string // Reason for closing: merged, rejected, conflict, superseded
	AgentBead   string // Agent bead ID that created this MR (for traceability)

	// PR lifecycle tracking (for pr_to_main and pr_to_branch strategies)
	PRUrl    string // GitHub PR URL (e.g., "https://github.com/owner/repo/pull/123")
	PRNumber int    // GitHub PR number (e.g., 123)
	PRState  string // PR state: open, merged, closed

	// Conflict resolution fields (for priority scoring)
	RetryCount      int    // Number of conflict-resolution cycles
	LastConflictSHA string // SHA of main when conflict occurred
	ConflictTaskID  string // Link to conflict-resolution task (if any)

	// Convoy tracking (for priority scoring - convoy starvation prevention)
	ConvoyID        string // Parent convoy ID if part of a convoy
	ConvoyCreatedAt string // Convoy creation time (ISO 8601) for starvation prevention
}

// ParseMRFields extracts structured merge-request fields from an issue's description.
// Fields are expected as "key: value" lines, with optional prose text mixed in.
// Returns nil if no MR fields are found.
func ParseMRFields(issue *Issue) *MRFields {
	if issue == nil || issue.Description == "" {
		return nil
	}

	fields := &MRFields{}
	hasFields := false

	for _, line := range strings.Split(issue.Description, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Look for "key: value" pattern
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		if value == "" {
			continue
		}

		// Map keys to fields (case-insensitive)
		switch strings.ToLower(key) {
		case "branch":
			fields.Branch = value
			hasFields = true
		case "target":
			fields.Target = value
			hasFields = true
		case "source_issue", "source-issue", "sourceissue":
			fields.SourceIssue = value
			hasFields = true
		case "worker":
			fields.Worker = value
			hasFields = true
		case "rig":
			fields.Rig = value
			hasFields = true
		case "merge_commit", "merge-commit", "mergecommit":
			fields.MergeCommit = value
			hasFields = true
		case "close_reason", "close-reason", "closereason":
			fields.CloseReason = value
			hasFields = true
		case "agent_bead", "agent-bead", "agentbead":
			fields.AgentBead = value
			hasFields = true
		case "retry_count", "retry-count", "retrycount":
			if n, err := parseIntField(value); err == nil {
				fields.RetryCount = n
				hasFields = true
			}
		case "last_conflict_sha", "last-conflict-sha", "lastconflictsha":
			fields.LastConflictSHA = value
			hasFields = true
		case "conflict_task_id", "conflict-task-id", "conflicttaskid":
			fields.ConflictTaskID = value
			hasFields = true
		case "convoy_id", "convoy-id", "convoyid", "convoy":
			fields.ConvoyID = value
			hasFields = true
		case "convoy_created_at", "convoy-created-at", "convoycreatedat":
			fields.ConvoyCreatedAt = value
			hasFields = true
		case "pr_url", "pr-url", "prurl":
			fields.PRUrl = value
			hasFields = true
		case "pr_number", "pr-number", "prnumber":
			if n, err := parseIntField(value); err == nil {
				fields.PRNumber = n
				hasFields = true
			}
		case "pr_state", "pr-state", "prstate":
			fields.PRState = value
			hasFields = true
		}
	}

	if !hasFields {
		return nil
	}
	return fields
}

// parseIntField parses an integer from a string, returning 0 on error.
func parseIntField(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// FormatMRFields formats MRFields as a string suitable for an issue description.
// Only non-empty fields are included.
func FormatMRFields(fields *MRFields) string {
	if fields == nil {
		return ""
	}

	var lines []string

	if fields.Branch != "" {
		lines = append(lines, "branch: "+fields.Branch)
	}
	if fields.Target != "" {
		lines = append(lines, "target: "+fields.Target)
	}
	if fields.SourceIssue != "" {
		lines = append(lines, "source_issue: "+fields.SourceIssue)
	}
	if fields.Worker != "" {
		lines = append(lines, "worker: "+fields.Worker)
	}
	if fields.Rig != "" {
		lines = append(lines, "rig: "+fields.Rig)
	}
	if fields.MergeCommit != "" {
		lines = append(lines, "merge_commit: "+fields.MergeCommit)
	}
	if fields.CloseReason != "" {
		lines = append(lines, "close_reason: "+fields.CloseReason)
	}
	if fields.AgentBead != "" {
		lines = append(lines, "agent_bead: "+fields.AgentBead)
	}
	if fields.PRUrl != "" {
		lines = append(lines, "pr_url: "+fields.PRUrl)
	}
	if fields.PRNumber > 0 {
		lines = append(lines, fmt.Sprintf("pr_number: %d", fields.PRNumber))
	}
	if fields.PRState != "" {
		lines = append(lines, "pr_state: "+fields.PRState)
	}
	if fields.RetryCount > 0 {
		lines = append(lines, fmt.Sprintf("retry_count: %d", fields.RetryCount))
	}
	if fields.LastConflictSHA != "" {
		lines = append(lines, "last_conflict_sha: "+fields.LastConflictSHA)
	}
	if fields.ConflictTaskID != "" {
		lines = append(lines, "conflict_task_id: "+fields.ConflictTaskID)
	}
	if fields.ConvoyID != "" {
		lines = append(lines, "convoy_id: "+fields.ConvoyID)
	}
	if fields.ConvoyCreatedAt != "" {
		lines = append(lines, "convoy_created_at: "+fields.ConvoyCreatedAt)
	}

	return strings.Join(lines, "\n")
}

// SetMRFields updates an issue's description with the given MR fields.
// Existing MR field lines are replaced; other content is preserved.
// Returns the new description string.
func SetMRFields(issue *Issue, fields *MRFields) string {
	if issue == nil {
		return FormatMRFields(fields)
	}

	// Known MR field keys (lowercase)
	mrKeys := map[string]bool{
		"branch":             true,
		"target":             true,
		"source_issue":       true,
		"source-issue":       true,
		"sourceissue":        true,
		"worker":             true,
		"rig":                true,
		"merge_commit":       true,
		"merge-commit":       true,
		"mergecommit":        true,
		"close_reason":       true,
		"close-reason":       true,
		"closereason":        true,
		"agent_bead":         true,
		"agent-bead":         true,
		"agentbead":          true,
		"retry_count":        true,
		"retry-count":        true,
		"retrycount":         true,
		"last_conflict_sha":  true,
		"last-conflict-sha":  true,
		"lastconflictsha":    true,
		"conflict_task_id":   true,
		"conflict-task-id":   true,
		"conflicttaskid":     true,
		"convoy_id":          true,
		"convoy-id":          true,
		"convoyid":           true,
		"convoy":             true,
		"convoy_created_at":  true,
		"convoy-created-at":  true,
		"convoycreatedat":    true,
		"pr_url":             true,
		"pr-url":             true,
		"prurl":              true,
		"pr_number":          true,
		"pr-number":          true,
		"prnumber":           true,
		"pr_state":           true,
		"pr-state":           true,
		"prstate":            true,
	}

	// Collect non-MR lines from existing description
	var otherLines []string
	if issue.Description != "" {
		for _, line := range strings.Split(issue.Description, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				// Preserve blank lines in content
				otherLines = append(otherLines, line)
				continue
			}

			// Check if this is an MR field line
			colonIdx := strings.Index(trimmed, ":")
			if colonIdx == -1 {
				otherLines = append(otherLines, line)
				continue
			}

			key := strings.ToLower(strings.TrimSpace(trimmed[:colonIdx]))
			if !mrKeys[key] {
				otherLines = append(otherLines, line)
			}
			// Skip MR field lines - they'll be replaced
		}
	}

	// Build new description: MR fields first, then other content
	formatted := FormatMRFields(fields)

	// Trim trailing blank lines from other content
	for len(otherLines) > 0 && strings.TrimSpace(otherLines[len(otherLines)-1]) == "" {
		otherLines = otherLines[:len(otherLines)-1]
	}
	// Trim leading blank lines from other content
	for len(otherLines) > 0 && strings.TrimSpace(otherLines[0]) == "" {
		otherLines = otherLines[1:]
	}

	if formatted == "" {
		return strings.Join(otherLines, "\n")
	}
	if len(otherLines) == 0 {
		return formatted
	}

	return formatted + "\n\n" + strings.Join(otherLines, "\n")
}

// SynthesisFields holds structured fields for synthesis beads.
// These fields track the synthesis step in a convoy workflow.
type SynthesisFields struct {
	ConvoyID   string `json:"convoy_id"`   // Parent convoy ID
	ReviewID   string `json:"review_id"`   // Review ID for output paths
	OutputPath string `json:"output_path"` // Path to synthesis output file
	Formula    string `json:"formula"`     // Formula name (if from formula)
}

// ParseSynthesisFields extracts synthesis fields from an issue's description.
// Fields are expected as "key: value" lines. Returns nil if no fields found.
func ParseSynthesisFields(issue *Issue) *SynthesisFields {
	if issue == nil || issue.Description == "" {
		return nil
	}

	fields := &SynthesisFields{}
	hasFields := false

	for _, line := range strings.Split(issue.Description, "\n") {
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
		if value == "" {
			continue
		}

		switch strings.ToLower(key) {
		case "convoy", "convoy_id", "convoy-id":
			fields.ConvoyID = value
			hasFields = true
		case "review_id", "review-id", "reviewid":
			fields.ReviewID = value
			hasFields = true
		case "output_path", "output-path", "outputpath":
			fields.OutputPath = value
			hasFields = true
		case "formula":
			fields.Formula = value
			hasFields = true
		}
	}

	if !hasFields {
		return nil
	}
	return fields
}

// FormatSynthesisFields formats SynthesisFields as a string for issue description.
func FormatSynthesisFields(fields *SynthesisFields) string {
	if fields == nil {
		return ""
	}

	var lines []string
	if fields.ConvoyID != "" {
		lines = append(lines, "convoy: "+fields.ConvoyID)
	}
	if fields.ReviewID != "" {
		lines = append(lines, "review_id: "+fields.ReviewID)
	}
	if fields.OutputPath != "" {
		lines = append(lines, "output_path: "+fields.OutputPath)
	}
	if fields.Formula != "" {
		lines = append(lines, "formula: "+fields.Formula)
	}

	return strings.Join(lines, "\n")
}

// RoleConfig holds structured lifecycle configuration for role beads.
// These fields are stored as "key: value" lines in the role bead description.
// This enables agents to self-register their lifecycle configuration,
// replacing hardcoded identity string parsing in the daemon.
type RoleConfig struct {
	// SessionPattern defines how to derive tmux session name.
	// Supports placeholders: {rig}, {name}, {role}
	// Examples: "hq-mayor", "hq-deacon", "gt-{rig}-{role}", "gt-{rig}-{name}"
	SessionPattern string

	// WorkDirPattern defines the working directory relative to town root.
	// Supports placeholders: {town}, {rig}, {name}, {role}
	// Examples: "{town}", "{town}/{rig}", "{town}/{rig}/polecats/{name}"
	WorkDirPattern string

	// NeedsPreSync indicates whether workspace needs git sync before starting.
	// True for agents with persistent clones (refinery, crew, polecat).
	NeedsPreSync bool

	// StartCommand is the command to run after creating the session.
	// Default: "exec claude --dangerously-skip-permissions"
	StartCommand string

	// EnvVars are additional environment variables to set in the session.
	// Stored as "key=value" pairs.
	EnvVars map[string]string

	// Health check thresholds - per ZFC, agents control their own stuck detection.
	// These allow the Deacon's patrol config to be agent-defined rather than hardcoded.

	// PingTimeout is how long to wait for a health check response.
	// Format: duration string (e.g., "30s", "1m"). Default: 30s.
	PingTimeout string

	// ConsecutiveFailures is how many failed health checks before force-kill.
	// Default: 3.
	ConsecutiveFailures int

	// KillCooldown is the minimum time between force-kills of the same agent.
	// Format: duration string (e.g., "5m", "10m"). Default: 5m.
	KillCooldown string

	// StuckThreshold is how long a wisp can be in_progress before considered stuck.
	// Format: duration string (e.g., "1h", "30m"). Default: 1h.
	StuckThreshold string
}

// ParseRoleConfig extracts RoleConfig from a role bead's description.
// Fields are expected as "key: value" lines. Returns nil if no config found.
func ParseRoleConfig(description string) *RoleConfig {
	config := &RoleConfig{
		EnvVars: make(map[string]string),
	}
	hasFields := false

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
		if value == "" || value == "null" {
			continue
		}

		switch strings.ToLower(key) {
		case "session_pattern", "session-pattern", "sessionpattern":
			config.SessionPattern = value
			hasFields = true
		case "work_dir_pattern", "work-dir-pattern", "workdirpattern", "workdir_pattern":
			config.WorkDirPattern = value
			hasFields = true
		case "needs_pre_sync", "needs-pre-sync", "needspresync":
			config.NeedsPreSync = strings.ToLower(value) == "true"
			hasFields = true
		case "start_command", "start-command", "startcommand":
			config.StartCommand = value
			hasFields = true
		case "env_var", "env-var", "envvar":
			// Format: "env_var: KEY=VALUE"
			if eqIdx := strings.Index(value, "="); eqIdx != -1 {
				envKey := strings.TrimSpace(value[:eqIdx])
				envVal := strings.TrimSpace(value[eqIdx+1:])
				config.EnvVars[envKey] = envVal
				hasFields = true
			}
		// Health check threshold fields (ZFC: agent-controlled)
		case "ping_timeout", "ping-timeout", "pingtimeout":
			config.PingTimeout = value
			hasFields = true
		case "consecutive_failures", "consecutive-failures", "consecutivefailures":
			if n, err := parseIntValue(value); err == nil {
				config.ConsecutiveFailures = n
				hasFields = true
			}
		case "kill_cooldown", "kill-cooldown", "killcooldown":
			config.KillCooldown = value
			hasFields = true
		case "stuck_threshold", "stuck-threshold", "stuckthreshold":
			config.StuckThreshold = value
			hasFields = true
		}
	}

	if !hasFields {
		return nil
	}
	return config
}

// parseIntValue parses an integer from a string value.
func parseIntValue(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// FormatRoleConfig formats RoleConfig as a string suitable for a role bead description.
// Only non-empty/non-default fields are included.
func FormatRoleConfig(config *RoleConfig) string {
	if config == nil {
		return ""
	}

	var lines []string

	if config.SessionPattern != "" {
		lines = append(lines, "session_pattern: "+config.SessionPattern)
	}
	if config.WorkDirPattern != "" {
		lines = append(lines, "work_dir_pattern: "+config.WorkDirPattern)
	}
	if config.NeedsPreSync {
		lines = append(lines, "needs_pre_sync: true")
	}
	if config.StartCommand != "" {
		lines = append(lines, "start_command: "+config.StartCommand)
	}
	for k, v := range config.EnvVars {
		lines = append(lines, "env_var: "+k+"="+v)
	}

	return strings.Join(lines, "\n")
}

// ExpandRolePattern expands placeholders in a pattern string.
// Supported placeholders: {town}, {rig}, {name}, {role}
func ExpandRolePattern(pattern, townRoot, rig, name, role string) string {
	result := pattern
	result = strings.ReplaceAll(result, "{town}", townRoot)
	result = strings.ReplaceAll(result, "{rig}", rig)
	result = strings.ReplaceAll(result, "{name}", name)
	result = strings.ReplaceAll(result, "{role}", role)
	return result
}
