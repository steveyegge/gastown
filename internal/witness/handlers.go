package witness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/convoy"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/workspace"
)

// HandlerResult tracks the result of handling a protocol message.
type HandlerResult struct {
	MessageID    string
	ProtocolType ProtocolType
	Handled      bool
	Action       string
	WispCreated  string // ID of created wisp (if any)
	MailSent     string // ID of sent mail (if any)
	Error        error
}

// HandlePolecatDone processes a POLECAT_DONE message from a polecat.
// For ESCALATED/DEFERRED exits (no pending MR), auto-nukes if clean.
// For PHASE_COMPLETE exits, recycles the polecat (session ends, worktree kept).
// For COMPLETED exits with MR and clean state, auto-nukes immediately (ephemeral model).
// For exits with pending MR but dirty state, creates cleanup wisp for manual intervention.
//
// Ephemeral Polecat Model:
// Polecats are truly ephemeral - done at MR submission, recyclable immediately.
// Once the branch is pushed (cleanup_status=clean), the polecat can be nuked.
// The MR lifecycle continues independently in the Refinery.
// If conflicts arise, Refinery creates a NEW conflict-resolution task for a NEW polecat.
func HandlePolecatDone(workDir, rigName string, msg *mail.Message) *HandlerResult {
	result := &HandlerResult{
		MessageID:    msg.ID,
		ProtocolType: ProtoPolecatDone,
	}

	// Parse the message
	payload, err := ParsePolecatDone(msg.Subject, msg.Body)
	if err != nil {
		result.Error = fmt.Errorf("parsing POLECAT_DONE: %w", err)
		return result
	}

	if stale, reason := isStalePolecatDone(rigName, payload.PolecatName, msg); stale {
		result.Handled = true
		result.Action = fmt.Sprintf("ignored stale POLECAT_DONE for %s (%s)", payload.PolecatName, reason)
		return result
	}

	// Handle PHASE_COMPLETE: recycle polecat (session ends but worktree stays)
	// The polecat is registered as a waiter on the gate and will be re-dispatched
	// when the gate closes via gt gate wake.
	if payload.Exit == "PHASE_COMPLETE" {
		result.Handled = true
		result.Action = fmt.Sprintf("phase-complete for %s (gate=%s) - session recycled, awaiting gate", payload.PolecatName, payload.Gate)
		// Note: The polecat has already registered itself as a gate waiter via bd
		// The gate wake mechanism (gt gate wake) will send mail when gate closes
		// A new polecat will be dispatched to continue the molecule from the next step
		return result
	}

	// Check if this polecat has a pending MR
	// ESCALATED/DEFERRED exits typically have no MR pending
	hasPendingMR := payload.MRID != "" || payload.Exit == "COMPLETED"

	// Local-only branches model: if there's a pending MR, DON'T nuke.
	// The polecat's local branch is needed for conflict resolution if merge fails.
	// Once the MR merges (MERGED signal), HandleMerged will nuke the polecat.
	if hasPendingMR {
		// Create cleanup wisp to track this polecat is waiting for merge
		wispID, err := createCleanupWisp(workDir, payload.PolecatName, payload.IssueID, payload.Branch)
		if err != nil {
			result.Error = fmt.Errorf("creating cleanup wisp: %w", err)
			return result
		}

		// Update wisp state to indicate it's waiting for merge
		if err := UpdateCleanupWispState(workDir, wispID, "merge-requested"); err != nil {
			// Non-fatal - wisp was created, just couldn't update state
			result.Error = fmt.Errorf("updating wisp state: %w", err)
		}

		result.Handled = true
		result.WispCreated = wispID
		result.Action = fmt.Sprintf("deferred cleanup for %s (pending MR=%s, local branch preserved for conflict resolution)", payload.PolecatName, payload.MRID)
		return result
	}

	// No pending MR - try to auto-nuke immediately
	nukeResult := AutoNukeIfClean(workDir, rigName, payload.PolecatName)
	if nukeResult.Nuked {
		result.Handled = true
		result.Action = fmt.Sprintf("auto-nuked %s (exit=%s, no MR): %s", payload.PolecatName, payload.Exit, nukeResult.Reason)
		return result
	}
	if nukeResult.Error != nil {
		// Nuke failed - fall through to create wisp for manual cleanup
		result.Error = nukeResult.Error
	}

	// Couldn't auto-nuke (dirty state or verification failed) - create wisp for manual intervention
	wispID, err := createCleanupWisp(workDir, payload.PolecatName, payload.IssueID, payload.Branch)
	if err != nil {
		result.Error = fmt.Errorf("creating cleanup wisp: %w", err)
		return result
	}

	result.Handled = true
	result.WispCreated = wispID
	result.Action = fmt.Sprintf("created cleanup wisp %s for %s (needs manual cleanup: %s)", wispID, payload.PolecatName, nukeResult.Reason)

	return result
}

func isStalePolecatDone(rigName, polecatName string, msg *mail.Message) (bool, string) {
	if msg == nil {
		return false, ""
	}

	sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)
	createdAt, err := session.SessionCreatedAt(sessionName)
	if err != nil {
		// Session not found or tmux not running - can't determine staleness, allow message
		return false, ""
	}

	return session.StaleReasonForTimes(msg.Timestamp, createdAt)
}

// HandleLifecycleShutdown processes a LIFECYCLE:Shutdown message.
// Similar to POLECAT_DONE but triggered by daemon rather than polecat.
// Auto-nukes if clean since shutdown means no pending work.
func HandleLifecycleShutdown(workDir, rigName string, msg *mail.Message) *HandlerResult {
	result := &HandlerResult{
		MessageID:    msg.ID,
		ProtocolType: ProtoLifecycleShutdown,
	}

	// Extract polecat name from subject
	matches := PatternLifecycleShutdown.FindStringSubmatch(msg.Subject)
	if len(matches) < 2 {
		result.Error = fmt.Errorf("invalid LIFECYCLE:Shutdown subject: %s", msg.Subject)
		return result
	}
	polecatName := matches[1]

	// Shutdown means no pending work - try to auto-nuke immediately
	nukeResult := AutoNukeIfClean(workDir, rigName, polecatName)
	if nukeResult.Nuked {
		result.Handled = true
		result.Action = fmt.Sprintf("auto-nuked %s (shutdown): %s", polecatName, nukeResult.Reason)
		return result
	}
	if nukeResult.Error != nil {
		// Nuke failed - fall through to create wisp
		result.Error = nukeResult.Error
	}

	// Couldn't auto-nuke - create a cleanup wisp for manual intervention
	wispID, err := createCleanupWisp(workDir, polecatName, "", "")
	if err != nil {
		result.Error = fmt.Errorf("creating cleanup wisp: %w", err)
		return result
	}

	result.Handled = true
	result.WispCreated = wispID
	result.Action = fmt.Sprintf("created cleanup wisp %s for shutdown %s (needs manual cleanup)", wispID, polecatName)

	return result
}

// HandleHelp processes a HELP message from a polecat requesting intervention.
// Assesses the request and either helps directly or escalates to Mayor.
func HandleHelp(workDir, rigName string, msg *mail.Message, router *mail.Router) *HandlerResult {
	result := &HandlerResult{
		MessageID:    msg.ID,
		ProtocolType: ProtoHelp,
	}

	// Parse the message
	payload, err := ParseHelp(msg.Subject, msg.Body)
	if err != nil {
		result.Error = fmt.Errorf("parsing HELP: %w", err)
		return result
	}

	// Assess the help request
	assessment := AssessHelpRequest(payload)

	if assessment.CanHelp {
		// Log that we can help - actual help is done by the Claude agent
		result.Handled = true
		result.Action = fmt.Sprintf("can help with '%s': %s", payload.Topic, assessment.HelpAction)
		return result
	}

	// Need to escalate to Mayor
	if assessment.NeedsEscalation {
		mailID, err := escalateToMayor(router, rigName, payload, assessment.EscalationReason)
		if err != nil {
			result.Error = fmt.Errorf("escalating to mayor: %w", err)
			return result
		}

		result.Handled = true
		result.MailSent = mailID
		result.Action = fmt.Sprintf("escalated '%s' to mayor: %s", payload.Topic, assessment.EscalationReason)
	}

	return result
}

// HandleMerged processes a MERGED message from the Refinery.
// Verifies cleanup_status before allowing nuke, escalates if work is at risk.
func HandleMerged(workDir, rigName string, msg *mail.Message) *HandlerResult {
	result := &HandlerResult{
		MessageID:    msg.ID,
		ProtocolType: ProtoMerged,
	}

	// Parse the message
	payload, err := ParseMerged(msg.Subject, msg.Body)
	if err != nil {
		result.Error = fmt.Errorf("parsing MERGED: %w", err)
		return result
	}

	// Find the cleanup wisp for this polecat
	wispID, err := findCleanupWisp(workDir, payload.PolecatName)
	if err != nil {
		result.Error = fmt.Errorf("finding cleanup wisp: %w", err)
		return result
	}

	if wispID == "" {
		// No wisp found - polecat may have been cleaned up already
		result.Handled = true
		result.Action = fmt.Sprintf("no cleanup wisp found for %s (may be already cleaned)", payload.PolecatName)
		return result
	}

	// Verify the polecat's commit is actually on main before allowing nuke.
	// This prevents work loss when MERGED signal is for a stale MR or the merge failed.
	onMain, err := verifyCommitOnMain(workDir, rigName, payload.PolecatName)
	if err != nil {
		// Couldn't verify - log warning but continue with other checks
		// The polecat may not exist anymore (already nuked) which is fine
		result.Action = fmt.Sprintf("warning: couldn't verify commit on main for %s: %v", payload.PolecatName, err)
	} else if !onMain {
		// Commit is NOT on main - don't nuke!
		result.Handled = true
		result.WispCreated = wispID
		result.Error = fmt.Errorf("polecat %s commit is NOT on main - MERGED signal may be stale, DO NOT NUKE", payload.PolecatName)
		result.Action = fmt.Sprintf("BLOCKED: %s commit not verified on main, merge may have failed", payload.PolecatName)
		return result
	}

	// Redundant convoy observer: check if completed issue is tracked by a convoy.
	// Run this after verifyCommitOnMain succeeds, regardless of cleanup status.
	// The work is confirmed merged at this point, so convoys tracking this issue
	// can potentially close even if polecat cleanup is blocked.
	if onMain && payload.IssueID != "" {
		townRoot, _ := workspace.Find(workDir)
		if townRoot != "" {
			convoy.CheckConvoysForIssue(townRoot, payload.IssueID, "witness", nil)
		}
	}

	// ZFC #10: Check cleanup_status before allowing nuke
	// This prevents work loss when MERGED signal arrives for stale MRs or
	// when polecat has new unpushed work since the MR was created.
	cleanupStatus := getCleanupStatus(workDir, rigName, payload.PolecatName)

	switch cleanupStatus {
	case "clean":
		// Safe to nuke - polecat has confirmed clean state
		// Execute the nuke immediately
		if err := NukePolecat(workDir, rigName, payload.PolecatName); err != nil {
			result.Handled = true
			result.WispCreated = wispID
			result.Error = fmt.Errorf("nuke failed for %s: %w", payload.PolecatName, err)
			result.Action = fmt.Sprintf("cleanup wisp %s for %s: nuke FAILED", wispID, payload.PolecatName)
		} else {
			result.Handled = true
			result.WispCreated = wispID
			result.Action = fmt.Sprintf("auto-nuked %s (cleanup_status=clean, wisp=%s)", payload.PolecatName, wispID)
		}

	case "has_uncommitted":
		// Has uncommitted changes - might be WIP, escalate to Mayor
		result.Handled = true
		result.WispCreated = wispID
		result.Error = fmt.Errorf("polecat %s has uncommitted changes - escalate to Mayor before nuke", payload.PolecatName)
		result.Action = fmt.Sprintf("BLOCKED: %s has uncommitted work, needs escalation", payload.PolecatName)

	case "has_stash":
		// Has stashed work - definitely needs review
		result.Handled = true
		result.WispCreated = wispID
		result.Error = fmt.Errorf("polecat %s has stashed work - escalate to Mayor before nuke", payload.PolecatName)
		result.Action = fmt.Sprintf("BLOCKED: %s has stashed work, needs escalation", payload.PolecatName)

	case "has_unpushed":
		// Critical: has unpushed commits that could be lost
		result.Handled = true
		result.WispCreated = wispID
		result.Error = fmt.Errorf("polecat %s has unpushed commits - DO NOT NUKE, escalate to Mayor", payload.PolecatName)
		result.Action = fmt.Sprintf("BLOCKED: %s has unpushed commits, DO NOT NUKE", payload.PolecatName)

	default:
		// Unknown or no status - we already verified commit is on main above
		// Safe to nuke since verification passed
		if err := NukePolecat(workDir, rigName, payload.PolecatName); err != nil {
			result.Handled = true
			result.WispCreated = wispID
			result.Error = fmt.Errorf("nuke failed for %s: %w", payload.PolecatName, err)
			result.Action = fmt.Sprintf("cleanup wisp %s for %s: nuke FAILED", wispID, payload.PolecatName)
		} else {
			result.Handled = true
			result.WispCreated = wispID
			result.Action = fmt.Sprintf("auto-nuked %s (commit on main, cleanup_status=%s, wisp=%s)", payload.PolecatName, cleanupStatus, wispID)
		}
	}

	return result
}

// HandleMergeFailed processes a MERGE_FAILED message from the Refinery.
// Notifies the polecat that their merge was rejected and rework is needed.
func HandleMergeFailed(workDir, rigName string, msg *mail.Message, router *mail.Router) *HandlerResult {
	result := &HandlerResult{
		MessageID:    msg.ID,
		ProtocolType: ProtoMergeFailed,
	}

	// Parse the message
	payload, err := ParseMergeFailed(msg.Subject, msg.Body)
	if err != nil {
		result.Error = fmt.Errorf("parsing MERGE_FAILED: %w", err)
		return result
	}

	// Notify the polecat about the failure
	polecatAddr := fmt.Sprintf("%s/polecats/%s", rigName, payload.PolecatName)
	notification := &mail.Message{
		From:     fmt.Sprintf("%s/witness", rigName),
		To:       polecatAddr,
		Subject:  fmt.Sprintf("Merge failed: %s", payload.FailureType),
		Priority: mail.PriorityHigh,
		Type:     mail.TypeTask,
		Body: fmt.Sprintf(`Your merge request was rejected.

Branch: %s
Issue: %s
Failure: %s
Error: %s

Please fix the issue and resubmit with 'gt done'.`,
			payload.Branch,
			payload.IssueID,
			payload.FailureType,
			payload.Error,
		),
	}

	if err := router.Send(notification); err != nil {
		result.Error = fmt.Errorf("sending failure notification: %w", err)
		return result
	}

	result.Handled = true
	result.MailSent = notification.ID
	result.Action = fmt.Sprintf("notified %s of merge failure: %s - %s", payload.PolecatName, payload.FailureType, payload.Error)

	return result
}

// HandleSwarmStart processes a SWARM_START message from the Mayor.
// Creates a swarm tracking wisp to monitor batch polecat work.
func HandleSwarmStart(workDir string, msg *mail.Message) *HandlerResult {
	result := &HandlerResult{
		MessageID:    msg.ID,
		ProtocolType: ProtoSwarmStart,
	}

	// Parse the message
	payload, err := ParseSwarmStart(msg.Body)
	if err != nil {
		result.Error = fmt.Errorf("parsing SWARM_START: %w", err)
		return result
	}

	// Create a swarm tracking wisp
	wispID, err := createSwarmWisp(workDir, payload)
	if err != nil {
		result.Error = fmt.Errorf("creating swarm wisp: %w", err)
		return result
	}

	result.Handled = true
	result.WispCreated = wispID
	result.Action = fmt.Sprintf("created swarm tracking wisp %s for %s", wispID, payload.SwarmID)

	return result
}

// createCleanupWisp creates a wisp to track polecat cleanup.
func createCleanupWisp(workDir, polecatName, issueID, branch string) (string, error) {
	title := fmt.Sprintf("cleanup:%s", polecatName)
	description := fmt.Sprintf("Verify and cleanup polecat %s", polecatName)
	if issueID != "" {
		description += fmt.Sprintf("\nIssue: %s", issueID)
	}
	if branch != "" {
		description += fmt.Sprintf("\nBranch: %s", branch)
	}

	labels := strings.Join(CleanupWispLabels(polecatName, "pending"), ",")

	output, err := util.ExecWithOutput(workDir, "bd", "create",
		"--ephemeral",
		"--title", title,
		"--description", description,
		"--labels", labels,
	)
	if err != nil {
		return "", err
	}

	// Extract wisp ID from output (bd create outputs "Created: <id>")
	if strings.HasPrefix(output, "Created:") {
		return strings.TrimSpace(strings.TrimPrefix(output, "Created:")), nil
	}

	// Try to extract ID from output
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// Look for bead ID pattern (e.g., "gt-abc123")
		if strings.Contains(line, "-") && len(line) < 20 {
			return line, nil
		}
	}

	return output, nil
}

// createSwarmWisp creates a wisp to track swarm (batch) work.
func createSwarmWisp(workDir string, payload *SwarmStartPayload) (string, error) {
	title := fmt.Sprintf("swarm:%s", payload.SwarmID)
	description := fmt.Sprintf("Tracking batch: %s\nTotal: %d polecats", payload.SwarmID, payload.Total)

	labels := strings.Join(SwarmWispLabels(payload.SwarmID, payload.Total, 0, payload.StartedAt), ",")

	output, err := util.ExecWithOutput(workDir, "bd", "create",
		"--ephemeral",
		"--title", title,
		"--description", description,
		"--labels", labels,
	)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(output, "Created:") {
		return strings.TrimSpace(strings.TrimPrefix(output, "Created:")), nil
	}

	return output, nil
}

// findCleanupWisp finds an existing cleanup wisp for a polecat.
func findCleanupWisp(workDir, polecatName string) (string, error) {
	output, err := util.ExecWithOutput(workDir, "bd", "list",
		"--ephemeral",
		"--labels", fmt.Sprintf("polecat:%s,state:merge-requested", polecatName),
		"--status", "open",
		"--json",
	)
	if err != nil {
		// Empty result is fine
		if strings.Contains(err.Error(), "no issues found") {
			return "", nil
		}
		return "", err
	}

	// Parse JSON to get the wisp ID
	if output == "" || output == "[]" || output == "null" {
		return "", nil
	}

	// Simple extraction - look for "id" field
	// Full JSON parsing would add dependency on encoding/json
	if idx := strings.Index(output, `"id":`); idx >= 0 {
		rest := output[idx+5:]
		rest = strings.TrimLeft(rest, ` "`)
		if endIdx := strings.IndexAny(rest, `",}`); endIdx > 0 {
			return rest[:endIdx], nil
		}
	}

	return "", nil
}

// agentBeadResponse is used to parse the bd show --json response for agent beads.
type agentBeadResponse struct {
	Description string `json:"description"`
}

// getCleanupStatus retrieves the cleanup_status from a polecat's agent bead.
// Returns the status string: "clean", "has_uncommitted", "has_stash", "has_unpushed"
// Returns empty string if agent bead doesn't exist or has no cleanup_status.
//
// ZFC #10: This enables the Witness to verify it's safe to nuke before proceeding.
// The polecat self-reports its git state when running `gt done`, and we trust that report.
func getCleanupStatus(workDir, rigName, polecatName string) string {
	// Construct agent bead ID using hq- prefix for town-level storage (fix for gt-myc).
	// All polecat agent beads are stored in town beads with hq- prefix to match manager.go.
	agentBeadID := beads.PolecatBeadIDTown(rigName, polecatName)

	output, err := util.ExecWithOutput(workDir, "bd", "show", agentBeadID, "--json")
	if err != nil {
		// Agent bead doesn't exist or bd failed - return empty (unknown status)
		return ""
	}

	if output == "" {
		return ""
	}

	// Parse the JSON response
	var resp agentBeadResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		return ""
	}

	// Parse cleanup_status from description
	// Description format has "cleanup_status: <value>" line
	for _, line := range strings.Split(resp.Description, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "cleanup_status:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "cleanup_status:"))
			value = strings.TrimSpace(strings.TrimPrefix(value, "Cleanup_status:"))
			if value != "" && value != "null" {
				return value
			}
		}
	}

	return ""
}

// escalateToMayor sends an escalation mail to the Mayor.
func escalateToMayor(router *mail.Router, rigName string, payload *HelpPayload, reason string) (string, error) {
	msg := &mail.Message{
		From:     fmt.Sprintf("%s/witness", rigName),
		To:       "mayor/",
		Subject:  fmt.Sprintf("Escalation: %s needs help", payload.Agent),
		Priority: mail.PriorityHigh,
		Body: fmt.Sprintf(`Agent: %s
Issue: %s
Topic: %s
Problem: %s
Tried: %s
Escalation reason: %s
Requested at: %s`,
			payload.Agent,
			payload.IssueID,
			payload.Topic,
			payload.Problem,
			payload.Tried,
			reason,
			payload.RequestedAt.Format(time.RFC3339),
		),
	}

	if err := router.Send(msg); err != nil {
		return "", err
	}

	return msg.ID, nil
}

// RecoveryPayload contains data for RECOVERY_NEEDED escalation.
type RecoveryPayload struct {
	PolecatName   string
	Rig           string
	CleanupStatus string
	Branch        string
	IssueID       string
	DetectedAt    time.Time
}

// EscalateRecoveryNeeded sends a RECOVERY_NEEDED escalation to the Mayor.
// This is used when a dormant polecat has unpushed work that needs recovery
// before cleanup. The Mayor should coordinate recovery (e.g., push the branch,
// save the work) before authorizing cleanup.
func EscalateRecoveryNeeded(router *mail.Router, rigName string, payload *RecoveryPayload) (string, error) {
	msg := &mail.Message{
		From:     fmt.Sprintf("%s/witness", rigName),
		To:       "mayor/",
		Subject:  fmt.Sprintf("RECOVERY_NEEDED %s/%s", rigName, payload.PolecatName),
		Priority: mail.PriorityUrgent,
		Body: fmt.Sprintf(`Polecat: %s/%s
Cleanup Status: %s
Branch: %s
Issue: %s
Detected: %s

This polecat has unpushed/uncommitted work that will be lost if nuked.
Please coordinate recovery before authorizing cleanup:
1. Check if branch can be pushed to origin
2. Review uncommitted changes for value
3. Either recover the work or authorize force-nuke

DO NOT nuke without --force after recovery.`,
			rigName,
			payload.PolecatName,
			payload.CleanupStatus,
			payload.Branch,
			payload.IssueID,
			payload.DetectedAt.Format(time.RFC3339),
		),
	}

	if err := router.Send(msg); err != nil {
		return "", err
	}

	return msg.ID, nil
}

// UpdateCleanupWispState updates a cleanup wisp's state label.
func UpdateCleanupWispState(workDir, wispID, newState string) error {
	// Get current labels to preserve other labels
	output, err := util.ExecWithOutput(workDir, "bd", "show", wispID, "--json")
	if err != nil {
		return fmt.Errorf("getting wisp: %w", err)
	}

	// Extract polecat name from existing labels for the update
	var polecatName string
	if idx := strings.Index(output, `polecat:`); idx >= 0 {
		rest := output[idx+8:]
		if endIdx := strings.IndexAny(rest, `",]}`); endIdx > 0 {
			polecatName = rest[:endIdx]
		}
	}

	if polecatName == "" {
		polecatName = "unknown"
	}

	// Update with new state
	newLabels := strings.Join(CleanupWispLabels(polecatName, newState), ",")

	return util.ExecRun(workDir, "bd", "update", wispID, "--labels", newLabels)
}

// NukePolecat executes the actual nuke operation for a polecat.
// This kills the tmux session, removes the worktree, and cleans up beads.
// Should only be called after all safety checks pass.
func NukePolecat(workDir, rigName, polecatName string) error {
	// CRITICAL: Kill the tmux session FIRST and unconditionally.
	// The session name follows the pattern gt-<rig>-<polecat>.
	// We do this explicitly here because gt polecat nuke may fail to kill the
	// session due to rig loading issues or race conditions with IsRunning checks.
	// See: gt-g9ft5 - sessions were piling up because nuke wasn't killing them.
	sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)
	t := tmux.NewTmux()

	// Check if session exists and kill it
	if running, _ := t.HasSession(sessionName); running {
		// Try graceful shutdown first (Ctrl-C), then force kill
		_ = t.SendKeysRaw(sessionName, "C-c")
		// Brief delay for graceful handling
		time.Sleep(100 * time.Millisecond)
		// Force kill the session
		if err := t.KillSessionWithProcesses(sessionName); err != nil {
			// Log but continue - session might already be dead
			// The important thing is we tried
		}
	}

	// Now run gt polecat nuke to clean up worktree, branch, and beads
	address := fmt.Sprintf("%s/%s", rigName, polecatName)

	if err := util.ExecRun(workDir, "gt", "polecat", "nuke", address); err != nil {
		return fmt.Errorf("nuke failed: %w", err)
	}

	return nil
}

// NukePolecatResult contains the result of an auto-nuke attempt.
type NukePolecatResult struct {
	Nuked   bool
	Skipped bool
	Reason  string
	Error   error
}

// AutoNukeIfClean checks if a polecat is safe to nuke and nukes it if so.
// This is used for orphaned polecats (no hooked work, no pending MR).
// With the self-cleaning model, polecats should self-nuke on completion.
// An orphan is likely from a crash before gt done completed.
// Returns whether the nuke was performed and any error.
func AutoNukeIfClean(workDir, rigName, polecatName string) *NukePolecatResult {
	result := &NukePolecatResult{}

	// Check cleanup_status from agent bead
	cleanupStatus := getCleanupStatus(workDir, rigName, polecatName)

	switch cleanupStatus {
	case "clean":
		// Safe to nuke
		if err := NukePolecat(workDir, rigName, polecatName); err != nil {
			result.Error = err
			result.Reason = fmt.Sprintf("nuke failed: %v", err)
		} else {
			result.Nuked = true
			result.Reason = "auto-nuked (cleanup_status=clean, no MR)"
		}

	case "has_uncommitted", "has_stash", "has_unpushed":
		// Not safe - has work that could be lost
		result.Skipped = true
		result.Reason = fmt.Sprintf("skipped: has %s", strings.TrimPrefix(cleanupStatus, "has_"))

	default:
		// Unknown status - check git state directly as fallback
		onMain, err := verifyCommitOnMain(workDir, rigName, polecatName)
		if err != nil {
			// Can't verify - skip (polecat may not exist)
			result.Skipped = true
			result.Reason = fmt.Sprintf("skipped: couldn't verify git state: %v", err)
		} else if onMain {
			// Commit is on main, likely safe
			if err := NukePolecat(workDir, rigName, polecatName); err != nil {
				result.Error = err
				result.Reason = fmt.Sprintf("nuke failed: %v", err)
			} else {
				result.Nuked = true
				result.Reason = "auto-nuked (commit on main, no cleanup_status)"
			}
		} else {
			// Not on main - skip, might have unpushed work
			result.Skipped = true
			result.Reason = "skipped: commit not on main, may have unpushed work"
		}
	}

	return result
}

// verifyCommitOnMain checks if the polecat's current commit is on the default branch.
// This prevents nuking a polecat whose work wasn't actually merged.
//
// In multi-remote setups, the code may live on a remote other than "origin"
// (e.g., "gastown" for gastown.git). This function checks ALL remotes to find
// the one containing the default branch with the merged commit.
//
// Returns:
//   - true, nil: commit is verified on default branch
//   - false, nil: commit is NOT on default branch (don't nuke!)
//   - false, error: couldn't verify (treat as unsafe)
func verifyCommitOnMain(workDir, rigName, polecatName string) (bool, error) {
	// Find town root from workDir
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		return false, fmt.Errorf("finding town root: %v", err)
	}

	// Get configured default branch for this rig
	defaultBranch := "main" // fallback
	if rigCfg, err := rig.LoadRigConfig(filepath.Join(townRoot, rigName)); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	// Construct polecat path, handling both new and old structures
	// New structure: polecats/<name>/<rigname>/
	// Old structure: polecats/<name>/
	polecatPath := filepath.Join(townRoot, rigName, "polecats", polecatName, rigName)
	if _, err := os.Stat(polecatPath); os.IsNotExist(err) {
		// Fall back to old structure
		polecatPath = filepath.Join(townRoot, rigName, "polecats", polecatName)
	}

	// Get git for the polecat worktree
	g := git.NewGit(polecatPath)

	// Get the current HEAD commit SHA
	commitSHA, err := g.Rev("HEAD")
	if err != nil {
		return false, fmt.Errorf("getting polecat HEAD: %w", err)
	}

	// Get all configured remotes and check each one for the commit
	// This handles multi-remote setups where code may be on a remote other than "origin"
	remotes, err := g.Remotes()
	if err != nil {
		// If we can't list remotes, fall back to checking just the local branch
		isOnDefaultBranch, err := g.IsAncestor(commitSHA, defaultBranch)
		if err != nil {
			return false, fmt.Errorf("checking if commit is on %s: %w", defaultBranch, err)
		}
		return isOnDefaultBranch, nil
	}

	// Try each remote/<defaultBranch> until we find one where commit is an ancestor
	for _, remote := range remotes {
		remoteBranch := remote + "/" + defaultBranch
		isOnRemote, err := g.IsAncestor(commitSHA, remoteBranch)
		if err == nil && isOnRemote {
			return true, nil
		}
	}

	// Also try the local default branch (in case we're not tracking a remote)
	isOnDefaultBranch, err := g.IsAncestor(commitSHA, defaultBranch)
	if err == nil && isOnDefaultBranch {
		return true, nil
	}

	// Commit is not on any remote's default branch
	return false, nil
}

// PolecatsWithHookedWork contains info about polecats that have hooked work but no active session.
type PolecatsWithHookedWork struct {
	PolecatName string
	HookBead    string
	RigName     string
}

// FindPolecatsWithHookedWork finds polecats that have hooked work but no active tmux session.
// These are polecats that completed work (gt done) but then had new work slung to them via gt sling.
// They need to be respawned to process the hooked work.
// Returns a list of such polecats that should be respawned.
//
// FIX (hq-50u3h): This function addresses the bug where done polecats don't process hooked work.
// The fix checks each polecat's agent bead for hook_bead and verifies no active tmux session.
//
// FIX (hq-dtwfqa): Validate hook_bead exists before including in respawn list.
// If hook_bead points to a deleted/non-existent bead, clear it and skip this polecat.
// This prevents respawn failures when orphaned polecats have stale hook_bead references.
func FindPolecatsWithHookedWork(workDir, rigName string) ([]*PolecatsWithHookedWork, error) {
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		return nil, fmt.Errorf("finding town root: %w", err)
	}

	// Get beads directory
	rigPath := filepath.Join(townRoot, rigName)
	bd := beads.New(rigPath)
	t := tmux.NewTmux()

	var result []*PolecatsWithHookedWork

	// Get all polecat agent beads (returns map of bead ID to Issue)
	agentBeads, err := bd.ListAgentBeads()
	if err != nil {
		return nil, fmt.Errorf("listing agent beads: %w", err)
	}

	// Filter for polecat agents in this rig
	// Format: <prefix>-<rig>-polecat-<name>
	polecatSuffix := fmt.Sprintf("%s-polecat-", rigName)
	for agentID := range agentBeads {
		// Check if this is a polecat agent bead for this rig
		if !strings.Contains(agentID, polecatSuffix) {
			continue
		}

		// Extract polecat name from agent ID
		parts := strings.Split(agentID, "-polecat-")
		if len(parts) < 2 {
			continue
		}
		polecatName := parts[len(parts)-1]

		// Get agent bead fields to check hook_bead
		_, fields, err := bd.GetAgentBead(agentID)
		if err != nil || fields == nil {
			// Skip if we can't get agent bead info
			continue
		}

		// Check if hook_bead is set (indicating work is waiting)
		if fields.HookBead == "" {
			continue
		}

		// FIX (hq-dtwfqa): Validate that hook_bead actually exists before adding to respawn list.
		// If the bead was deleted or doesn't exist, clear the stale reference.
		_, err = bd.Show(fields.HookBead)
		if err != nil {
			// Hook bead doesn't exist - clear the stale reference
			// This is non-fatal; we log and continue
			if clearErr := bd.ClearHookBead(agentID); clearErr != nil {
				// Couldn't clear - log but continue
				fmt.Printf("Warning: could not clear stale hook_bead %s from %s: %v\n", fields.HookBead, agentID, clearErr)
			} else {
				fmt.Printf("Cleared stale hook_bead %s from %s (bead no longer exists)\n", fields.HookBead, agentID)
			}
			continue
		}

		// Check if polecat has an active tmux session
		sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)
		hasSession, _ := t.HasSession(sessionName)

		if hasSession {
			// Session is active - polecat is already working
			continue
		}

		// Polecat has hooked work but no active session - needs respawn
		result = append(result, &PolecatsWithHookedWork{
			PolecatName: polecatName,
			HookBead:    fields.HookBead,
			RigName:     rigName,
		})
	}

	return result, nil
}

// RespawnPolecatWithHookedWork respawns a polecat to process its hooked work.
// This is used when a polecat has completed work (gt done) but new work was slung to it.
// The polecat's session is restarted to process the hooked work.
//
// FIX (hq-50u3h): This function respawns polecats with hooked work but no active session.
// FIX (hq-dtwfqa): Validate hook_bead exists before attempting respawn.
func RespawnPolecatWithHookedWork(workDir, rigName, polecatName string) error {
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		return fmt.Errorf("finding town root: %w", err)
	}

	rigPath := filepath.Join(townRoot, rigName)
	// Create rig struct directly (no LoadRig function exists)
	r := &rig.Rig{
		Name: rigName,
		Path: rigPath,
	}

	// Get the polecat manager
	g := git.NewGit(rigPath)
	t := tmux.NewTmux()
	mgr := polecat.NewManager(r, g, t)

	// Get the hook_bead from the agent bead.
	// All polecat agent beads use hq- prefix and are stored in town beads (fix for gt-myc).
	agentBeadID := beads.PolecatBeadIDTown(rigName, polecatName)
	bd := beads.New(rigPath)
	_, fields, err := bd.GetAgentBead(agentBeadID)
	if err != nil || fields == nil {
		return fmt.Errorf("getting agent bead: %w", err)
	}

	if fields.HookBead == "" {
		return fmt.Errorf("no hooked work found for polecat %s", polecatName)
	}

	// FIX (hq-dtwfqa): Validate hook_bead exists before attempting respawn.
	// If the bead was deleted between finding and respawning, clear it and return error.
	_, err = bd.Show(fields.HookBead)
	if err != nil {
		// Hook bead doesn't exist - clear the stale reference
		if clearErr := bd.ClearHookBead(agentBeadID); clearErr != nil {
			return fmt.Errorf("hook_bead %s no longer exists and failed to clear: %w", fields.HookBead, clearErr)
		}
		return fmt.Errorf("hook_bead %s no longer exists (cleared stale reference)", fields.HookBead)
	}

	// Check if polecat directory exists (using Get to check if polecat exists)
	_, err = mgr.Get(polecatName)
	if err != nil {
		// Polecat doesn't exist - need to create it fresh
		_, err := mgr.AddWithOptions(polecatName, polecat.AddOptions{
			HookBead: fields.HookBead,
		})
		if err != nil {
			return fmt.Errorf("creating polecat: %w", err)
		}
	} else {
		// Polecat exists - repair it to get a fresh worktree
		_, err := mgr.RepairWorktreeWithOptions(polecatName, true, polecat.AddOptions{
			HookBead: fields.HookBead,
		})
		if err != nil {
			return fmt.Errorf("repairing polecat worktree: %w", err)
		}
	}

	// Start a new tmux session for the polecat
	sessionMgr := polecat.NewSessionManager(t, r)

	err = sessionMgr.Start(polecatName, polecat.SessionStartOptions{
		Issue: fields.HookBead,
	})
	if err != nil {
		return fmt.Errorf("starting polecat session: %w", err)
	}

	return nil
}
