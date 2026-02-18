package witness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/convoy"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/workspace"
)

// initRegistryFromWorkDir initializes the session prefix registry from a work
// directory. This ensures session.PrefixFor(rigName) returns the correct rig
// prefix (e.g., "tr" for testrig) instead of the default "gt".
func initRegistryFromWorkDir(workDir string) {
	if townRoot, err := workspace.Find(workDir); err == nil && townRoot != "" {
		_ = session.InitRegistry(townRoot)
	}
}

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
// When a pending MR exists, sends MERGE_READY to the Refinery to trigger
// immediate merge queue processing. This ensures work flows through the system
// without waiting for the daemon's heartbeat cycle.
//
// Ephemeral Polecat Model:
// Polecats are truly ephemeral - done at MR submission, recyclable immediately.
// Once the branch is pushed (cleanup_status=clean), the polecat can be nuked.
// The MR lifecycle continues independently in the Refinery.
// If conflicts arise, Refinery creates a NEW conflict-resolution task for a NEW polecat.
func HandlePolecatDone(workDir, rigName string, msg *mail.Message, router *mail.Router) *HandlerResult {
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

	if stale, reason := isStalePolecatDone(workDir, rigName, payload.PolecatName, msg); stale {
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

		// Send MERGE_READY to Refinery to trigger immediate processing.
		// This is the canonical signal that keeps work flowing through the system
		// without waiting for the daemon's heartbeat cycle.
		if router != nil {
			mailID, err := sendMergeReady(router, rigName, payload)
			if err != nil {
				// Non-fatal - Refinery will still pick up work on next patrol cycle
				if result.Error != nil {
					result.Error = fmt.Errorf("sending MERGE_READY: %w (also: %v)", err, result.Error)
				} else {
					result.Error = fmt.Errorf("sending MERGE_READY: %w (non-fatal)", err)
				}
			} else {
				result.MailSent = mailID

				// Nudge the refinery to check its inbox immediately.
				townRoot, _ := workspace.Find(workDir)
				if nudgeErr := nudgeRefinery(townRoot, rigName); nudgeErr != nil {
					// Non-fatal - refinery will still pick up on next cycle
					if result.Error == nil {
						result.Error = fmt.Errorf("nudging refinery: %w (non-fatal)", nudgeErr)
					}
				}
			}
		}

		result.Handled = true
		result.WispCreated = wispID
		result.Action = fmt.Sprintf("deferred cleanup for %s (pending MR=%s, MERGE_READY sent to refinery)", payload.PolecatName, payload.MRID)
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

func isStalePolecatDone(workDir, rigName, polecatName string, msg *mail.Message) (bool, string) {
	if msg == nil {
		return false, ""
	}

	initRegistryFromWorkDir(workDir)
	sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
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

	// Need to escalate to Deacon (first line of escalation for routine ops)
	if assessment.NeedsEscalation {
		mailID, err := escalateToDeacon(router, rigName, payload, assessment.EscalationReason)
		if err != nil {
			result.Error = fmt.Errorf("escalating to deacon: %w", err)
			return result
		}

		result.Handled = true
		result.MailSent = mailID
		result.Action = fmt.Sprintf("escalated '%s' to deacon: %s", payload.Topic, assessment.EscalationReason)
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
		"--json",
		"--title", title,
		"--description", description,
		"--labels", labels,
	)
	if err != nil {
		return "", err
	}

	// Parse JSON output from bd create --json
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(output), &created); err == nil && created.ID != "" {
		return created.ID, nil
	}

	// Fallback: extract from "Created: <id>" format
	if strings.HasPrefix(output, "Created:") {
		return strings.TrimSpace(strings.TrimPrefix(output, "Created:")), nil
	}

	return "", fmt.Errorf("could not parse bead ID from bd create output: %q", output)
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
		"--label", fmt.Sprintf("polecat:%s,state:merge-requested", polecatName),
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

	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(output), &items); err != nil {
		return "", fmt.Errorf("parsing cleanup wisp response: %w", err)
	}
	if len(items) > 0 {
		return items[0].ID, nil
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
	// Construct agent bead ID using the rig's configured prefix
	// This supports non-gt prefixes like "bd-" for the beads rig
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		// Fall back to default prefix
		townRoot = workDir
	}
	prefix := beads.GetPrefixForRig(townRoot, rigName)
	agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)

	output, err := util.ExecWithOutput(workDir, "bd", "show", agentBeadID, "--json")
	if err != nil {
		// Agent bead doesn't exist or bd failed - return empty (unknown status)
		return ""
	}

	if output == "" {
		return ""
	}

	// Parse the JSON response — bd show --json returns an array
	var issues []agentBeadResponse
	if err := json.Unmarshal([]byte(output), &issues); err != nil || len(issues) == 0 {
		return ""
	}

	// Parse cleanup_status from description
	// Description format has "cleanup_status: <value>" line
	for _, line := range strings.Split(issues[0].Description, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "cleanup_status:") {
			// Use the lowercased version to ensure consistent prefix removal
			value := strings.TrimSpace(strings.TrimPrefix(lower, "cleanup_status:"))
			if value != "" && value != "null" {
				return value
			}
		}
	}

	return ""
}

// sendMergeReady sends a MERGE_READY notification to the Refinery.
// This signals that a polecat's work is ready for merge queue processing.
func sendMergeReady(router *mail.Router, rigName string, payload *PolecatDonePayload) (string, error) {
	msg := mail.NewMessage(
		fmt.Sprintf("%s/witness", rigName),
		fmt.Sprintf("%s/refinery", rigName),
		fmt.Sprintf("MERGE_READY %s", payload.PolecatName),
		fmt.Sprintf(`Branch: %s
Issue: %s
MR: %s
Polecat: %s
Verified: clean git state`,
			payload.Branch,
			payload.IssueID,
			payload.MRID,
			payload.PolecatName,
		),
	)
	msg.Priority = mail.PriorityHigh
	msg.Type = mail.TypeTask

	if err := router.Send(msg); err != nil {
		return "", err
	}

	return msg.ID, nil
}

// nudgeRefinery queues a nudge for the refinery session to check its inbox.
// Uses the nudge queue for cooperative delivery so we don't interrupt in-flight
// tool calls. The refinery will pick this up at its next turn boundary.
func nudgeRefinery(townRoot, rigName string) error {
	_ = session.InitRegistry(townRoot)
	sessionName := session.RefinerySessionName(session.PrefixFor(rigName))

	// Check if refinery is running
	t := tmux.NewTmux()
	running, err := t.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking refinery session: %w", err)
	}

	if !running {
		// Refinery not running - daemon will start it on next heartbeat.
		// The MERGE_READY mail will be waiting in its inbox.
		return nil
	}

	// Queue the nudge for cooperative delivery at next turn boundary
	nudgeMsg := "MERGE_READY received - check inbox for pending work"
	if townRoot != "" {
		return nudge.Enqueue(townRoot, sessionName, nudge.QueuedNudge{
			Sender:  rigName + "/witness",
			Message: nudgeMsg,
		})
	}
	// Fallback to direct nudge if town root unavailable
	return t.NudgeSession(sessionName, nudgeMsg)
}

// escalateToDeacon sends an escalation mail to the Deacon for routine operational issues.
// The Deacon is the first line of escalation for witness operations. Only truly strategic
// issues (deacon down, cross-rig coordination) should go directly to Mayor.
func escalateToDeacon(router *mail.Router, rigName string, payload *HelpPayload, reason string) (string, error) {
	msg := &mail.Message{
		From:     fmt.Sprintf("%s/witness", rigName),
		To:       "deacon/",
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

// EscalateRecoveryNeeded sends a RECOVERY_NEEDED escalation to the Deacon.
// This is used when a dormant polecat has unpushed work that needs recovery
// before cleanup. The Deacon should coordinate recovery (e.g., push the branch,
// save the work) before authorizing cleanup. Only escalates to Mayor if Deacon
// cannot resolve.
func EscalateRecoveryNeeded(router *mail.Router, rigName string, payload *RecoveryPayload) (string, error) {
	msg := &mail.Message{
		From:     fmt.Sprintf("%s/witness", rigName),
		To:       "deacon/",
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

	// Extract polecat name from existing labels via JSON parsing
	polecatName := extractPolecatFromJSON(output)

	if polecatName == "" {
		polecatName = "unknown"
	}

	// Update with new state — pass one --set-labels=<label> per label,
	// matching the pattern used in agent_state.go and molecule_await_signal.go.
	labels := CleanupWispLabels(polecatName, newState)
	args := []string{"update", wispID}
	for _, l := range labels {
		args = append(args, "--set-labels="+l)
	}
	return util.ExecRun(workDir, "bd", args...)
}

// extractPolecatFromJSON extracts the polecat name from bd show --json output.
// Returns empty string if the output is malformed or no polecat label is found.
func extractPolecatFromJSON(output string) string {
	var items []struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(output), &items); err != nil || len(items) == 0 {
		return ""
	}
	for _, label := range items[0].Labels {
		if name, ok := strings.CutPrefix(label, "polecat:"); ok {
			return name
		}
	}
	return ""
}

// NukePolecat executes the actual nuke operation for a polecat.
// This kills the tmux session, removes the worktree, and cleans up beads.
// Should only be called after all safety checks pass.
func NukePolecat(workDir, rigName, polecatName string) error {
	// CRITICAL: Kill the tmux session FIRST and unconditionally.
	// We do this explicitly here because gt polecat nuke may fail to kill the
	// session due to rig loading issues or race conditions with IsRunning checks.
	// See: gt-g9ft5 - sessions were piling up because nuke wasn't killing them.
	initRegistryFromWorkDir(workDir)
	sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
	t := tmux.NewTmux()

	// Check if session exists and kill it
	if running, _ := t.HasSession(sessionName); running {
		// Try graceful shutdown first (Ctrl-C), then force kill
		_ = t.SendKeysRaw(sessionName, "C-c")
		// Brief delay for graceful handling
		time.Sleep(100 * time.Millisecond)
		// Force kill the session
		if err := t.KillSession(sessionName); err != nil {
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

// ZombieResult describes a detected zombie polecat and the action taken.
type ZombieResult struct {
	PolecatName   string
	AgentState    string
	HookBead      string
	Action        string // "auto-nuked", "escalated", "cleanup-wisp-created"
	BeadRecovered bool   // true if hooked bead was reset to open for re-dispatch
	Error         error
}

// DetectZombiePolecatsResult contains the results of a zombie detection sweep.
type DetectZombiePolecatsResult struct {
	Checked int
	Zombies []ZombieResult
	Errors  []error // Transient errors that prevented checking some polecats
}

// DetectZombiePolecats cross-references polecat agent state with tmux session
// existence and agent process liveness to find zombie polecats. Two zombie classes:
//   - Session-dead: tmux session is dead but agent bead still shows agent_state=
//     "working", "running", or "spawning", or has a hook_bead assigned.
//   - Agent-dead: tmux session exists but the agent process (Claude/node) inside
//     it has died. Detected via IsAgentAlive. See gt-kj6r6.
//
// Zombies cannot send POLECAT_DONE or other signals, so they sit undetected
// by the reactive signal-based patrol. This function provides proactive detection.
//
// Race safety: Records the detection timestamp before checking session liveness.
// Before taking any destructive action (nuke), re-verifies that the session
// hasn't been recreated since detection. This prevents killing newly-spawned
// sessions that reuse the same name.
//
// Dedup: Checks for existing cleanup wisps before escalating, preventing
// infinite escalation loops on subsequent patrol cycles.
//
// For each zombie found:
//   - If git state is clean (no unpushed work): auto-nuke
//   - If git state is dirty (unpushed/uncommitted work): escalate to Mayor via
//     EscalateRecoveryNeeded, create cleanup wisp
func DetectZombiePolecats(workDir, rigName string, router *mail.Router) *DetectZombiePolecatsResult {
	result := &DetectZombiePolecatsResult{}

	// Find town root for beads prefix resolution and session naming
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}
	_ = session.InitRegistry(townRoot)

	// List all polecat directories
	polecatsDir := filepath.Join(townRoot, rigName, "polecats")
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		return result // No polecats directory
	}

	t := tmux.NewTmux()

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecatName := entry.Name()
		sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
		result.Checked++

		// Record timestamp BEFORE checking session liveness.
		// Used later to guard against TOCTOU race where a new session
		// could be spawned between our check and the nuke action.
		detectedAt := time.Now()

		// Check if tmux session exists
		sessionAlive, err := t.HasSession(sessionName)
		if err != nil {
			// Transient tmux error — record it so the caller knows detection
			// was incomplete, rather than silently skipping this polecat.
			result.Errors = append(result.Errors,
				fmt.Errorf("checking session %s: %w", sessionName, err))
			continue
		}
		// Read agent bead labels for done-intent detection.
		// Done early because we need it for both live and dead session paths.
		prefix := beads.GetPrefixForRig(townRoot, rigName)
		agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)
		labels := getAgentBeadLabels(workDir, agentBeadID)
		doneIntent := extractDoneIntent(labels)

		if sessionAlive {
			// Live session — normally not a zombie. But check for done-intent
			// that's been stuck too long (polecat hung in gt done).
			if doneIntent != nil && time.Since(doneIntent.Timestamp) > 60*time.Second {
				// Polecat has been stuck in gt done for >60s — kill session.
				// Read hook bead before nuke (nuke may clean up agent bead)
				_, stuckHookBead := getAgentBeadState(workDir, agentBeadID)
				zombie := ZombieResult{
					PolecatName: polecatName,
					AgentState:  "stuck-in-done",
					HookBead:    stuckHookBead,
					Action:      fmt.Sprintf("killed-stuck-session (done-intent age=%v)", time.Since(doneIntent.Timestamp).Round(time.Second)),
				}
				if err := NukePolecat(workDir, rigName, polecatName); err != nil {
					zombie.Error = err
					zombie.Action = fmt.Sprintf("kill-stuck-session-failed: %v", err)
				}
				// Notify convoys if hooked bead was closed (gt-nsteq7)
				if stuckHookBead != "" && getBeadStatus(workDir, stuckHookBead) == "closed" {
					convoy.CheckConvoysForIssue(townRoot, stuckHookBead, "witness-zombie", nil)
				}
				// Reset abandoned bead for re-dispatch (gt-c3lgp)
				zombie.BeadRecovered = resetAbandonedBead(workDir, rigName, stuckHookBead, polecatName, router)
				result.Zombies = append(result.Zombies, zombie)
				continue
			}

			// Tmux session exists but agent process may have died inside it.
			// This catches the "tmux-alive-but-agent-dead" zombie class that
			// status.go detects but DetectZombiePolecats previously missed.
			// See: gt-kj6r6
			if !t.IsAgentAlive(sessionName) {
				// Read hook bead before nuke (nuke may clean up agent bead)
				_, deadAgentHookBead := getAgentBeadState(workDir, agentBeadID)
				zombie := ZombieResult{
					PolecatName: polecatName,
					AgentState:  "agent-dead-in-session",
					HookBead:    deadAgentHookBead,
					Action:      "killed-agent-dead-session",
				}
				if err := NukePolecat(workDir, rigName, polecatName); err != nil {
					zombie.Error = err
					zombie.Action = fmt.Sprintf("kill-agent-dead-session-failed: %v", err)
				}
				// Notify convoys if hooked bead was closed (gt-nsteq7)
				if deadAgentHookBead != "" && getBeadStatus(workDir, deadAgentHookBead) == "closed" {
					convoy.CheckConvoysForIssue(townRoot, deadAgentHookBead, "witness-zombie", nil)
				}
				// Reset abandoned bead for re-dispatch (gt-c3lgp)
				zombie.BeadRecovered = resetAbandonedBead(workDir, rigName, deadAgentHookBead, polecatName, router)
				result.Zombies = append(result.Zombies, zombie)
			} else {
				// Agent is alive. Check if the hooked bead has been closed.
				// A polecat that closed its bead but didn't run gt done is
				// occupying a slot without doing work. See: gt-h1l6i
				_, hookBead := getAgentBeadState(workDir, agentBeadID)
				if hookBead != "" && getBeadStatus(workDir, hookBead) == "closed" {
					zombie := ZombieResult{
						PolecatName: polecatName,
						AgentState:  "bead-closed-still-running",
						HookBead:    hookBead,
						Action:      "nuke-bead-closed-polecat",
					}
					if err := NukePolecat(workDir, rigName, polecatName); err != nil {
						zombie.Error = err
						zombie.Action = fmt.Sprintf("nuke-bead-closed-failed: %v", err)
					}
					// Notify convoys about the closed bead (gt-nsteq7)
					convoy.CheckConvoysForIssue(townRoot, hookBead, "witness-zombie", nil)
					result.Zombies = append(result.Zombies, zombie)
				}
			}
			continue // Either handled or not a zombie
		}

		// Session is dead. Check for done-intent first (faster path).
		if doneIntent != nil {
			age := time.Since(doneIntent.Timestamp)
			if age < 30*time.Second {
				// Recent done-intent — polecat is still working through gt done.
				// Skip, don't interfere.
				continue
			}
			// Old done-intent + dead session = polecat tried to exit but session
			// died mid-gt-done. Auto-nuke without further checks.
			// Read hook bead before nuke (nuke may clean up agent bead)
			_, diHookBead := getAgentBeadState(workDir, agentBeadID)
			zombie := ZombieResult{
				PolecatName: polecatName,
				AgentState:  "done-intent-dead",
				HookBead:    diHookBead,
				Action:      fmt.Sprintf("auto-nuked (done-intent age=%v, type=%s)", age.Round(time.Second), doneIntent.ExitType),
			}
			if err := NukePolecat(workDir, rigName, polecatName); err != nil {
				zombie.Error = err
				zombie.Action = fmt.Sprintf("nuke-failed (done-intent): %v", err)
			}
			// Notify convoys if hooked bead was closed (gt-nsteq7)
			if diHookBead != "" && getBeadStatus(workDir, diHookBead) == "closed" {
				convoy.CheckConvoysForIssue(townRoot, diHookBead, "witness-zombie", nil)
			}
			// Reset abandoned bead for re-dispatch (gt-c3lgp)
			zombie.BeadRecovered = resetAbandonedBead(workDir, rigName, diHookBead, polecatName, router)
			result.Zombies = append(result.Zombies, zombie)
			continue
		}

		// No done-intent. Fall back to standard zombie detection.
		agentState, hookBead := getAgentBeadState(workDir, agentBeadID)

		// A zombie has a dead session but agent_state suggests it should be alive,
		// or it still has work hooked. Include "spawning" so polecats that crash
		// during spawn are detected rather than invisible to zombie detection.
		isZombie := false
		if hookBead != "" {
			isZombie = true
		}
		if agentState == "working" || agentState == "running" || agentState == "spawning" {
			isZombie = true
		}

		if !isZombie {
			continue
		}

		// TOCTOU guard: Before taking any destructive action, re-verify that
		// the session hasn't been recreated since we checked. A new polecat
		// manager may have spawned a fresh session with the same name.
		if sessionRecreated(t, sessionName, detectedAt) {
			continue // New session exists — not a zombie, skip
		}

		// Zombie detected! Determine cleanup action based on git state.
		zombie := ZombieResult{
			PolecatName: polecatName,
			AgentState:  agentState,
			HookBead:    hookBead,
		}

		cleanupStatus := getCleanupStatus(workDir, rigName, polecatName)

		switch cleanupStatus {
		case "clean":
			// Polecat ran gt done and confirmed clean state — safe to auto-nuke.
			nukeResult := AutoNukeIfClean(workDir, rigName, polecatName)
			if nukeResult.Nuked {
				zombie.Action = "auto-nuked"
			} else if nukeResult.Skipped {
				wispID, wispErr := createCleanupWisp(workDir, polecatName, hookBead, "")
				if wispErr != nil {
					zombie.Error = wispErr
				}
				zombie.Action = fmt.Sprintf("cleanup-wisp-created:%s (skip reason: %s)", wispID, nukeResult.Reason)
			} else if nukeResult.Error != nil {
				zombie.Error = nukeResult.Error
				zombie.Action = "nuke-failed"
			}

		case "":
			// Empty cleanup_status means the agent bead has no cleanup info —
			// the polecat likely crashed before running gt done. AutoNukeIfClean
			// handles this via verifyCommitOnMain fallback: only nukes if the
			// polecat's commit is already on main, otherwise skips.
			nukeResult := AutoNukeIfClean(workDir, rigName, polecatName)
			if nukeResult.Nuked {
				zombie.Action = "auto-nuked"
			} else if nukeResult.Skipped {
				// Couldn't nuke cleanly — create cleanup wisp
				wispID, wispErr := createCleanupWisp(workDir, polecatName, hookBead, "")
				if wispErr != nil {
					zombie.Error = wispErr
				}
				zombie.Action = fmt.Sprintf("cleanup-wisp-created:%s (skip reason: %s)", wispID, nukeResult.Reason)
			} else if nukeResult.Error != nil {
				zombie.Error = nukeResult.Error
				zombie.Action = "nuke-failed"
			}

		case "has_uncommitted", "has_stash", "has_unpushed":
			// Dirty state — escalate to Mayor for recovery, but only if we
			// haven't already created a cleanup wisp for this polecat (dedup).
			existingWisp := findAnyCleanupWisp(workDir, polecatName)
			if existingWisp != "" {
				// Already tracked — skip escalation to prevent infinite loops.
				zombie.Action = fmt.Sprintf("already-tracked (cleanup_status=%s, existing-wisp=%s)", cleanupStatus, existingWisp)
			} else {
				if router != nil {
					_, escErr := EscalateRecoveryNeeded(router, rigName, &RecoveryPayload{
						PolecatName:   polecatName,
						Rig:           rigName,
						CleanupStatus: cleanupStatus,
						IssueID:       hookBead,
						DetectedAt:    time.Now(),
					})
					if escErr != nil {
						zombie.Error = escErr
					}
				}
				// Create cleanup wisp for tracking
				wispID, wispErr := createCleanupWisp(workDir, polecatName, hookBead, "")
				if wispErr != nil && zombie.Error == nil {
					zombie.Error = wispErr
				}
				zombie.Action = fmt.Sprintf("escalated (cleanup_status=%s, wisp=%s)", cleanupStatus, wispID)
			}
		}

		// Notify convoys if hooked bead was closed (gt-nsteq7).
		// This covers all standard zombie paths: clean, empty status, and dirty.
		// The bead may have been closed even if the polecat never ran gt done.
		if hookBead != "" && getBeadStatus(workDir, hookBead) == "closed" {
			convoy.CheckConvoysForIssue(townRoot, hookBead, "witness-zombie", nil)
		}

		// Reset abandoned bead for re-dispatch (gt-c3lgp)
		zombie.BeadRecovered = resetAbandonedBead(workDir, rigName, hookBead, polecatName, router)

		result.Zombies = append(result.Zombies, zombie)
	}

	return result
}

// StalledResult represents a single stalled polecat detection.
type StalledResult struct {
	PolecatName string // e.g., "alpha"
	StallType   string // "bypass-permissions", "unknown-prompt"
	Action      string // "auto-dismissed", "escalated"
	Error       error
}

// DetectStalledPolecatsResult holds aggregate results.
type DetectStalledPolecatsResult struct {
	Checked int             // Number of live polecats inspected
	Stalled []StalledResult // Stalled polecats found and processed
	Errors  []error         // Transient errors
}

// DetectStalledPolecats checks live polecat sessions for agents stuck on
// interactive prompts (e.g., the "Bypass Permissions mode" startup warning).
// Unlike zombie detection which looks for dead sessions/agents, this targets
// alive-but-stuck agents that will never make progress without intervention.
//
// For each qualifying polecat (live session + alive agent):
//   - Captures pane content (last 30 lines)
//   - Checks for known stall patterns
//   - Auto-dismisses known prompts (bypass-permissions) or escalates
//
// This is idempotent: calling AcceptBypassPermissionsWarning on a non-stalled
// session is harmless, so no dedup or TOCTOU guards are needed.
func DetectStalledPolecats(workDir, rigName string) *DetectStalledPolecatsResult {
	result := &DetectStalledPolecatsResult{}

	// Find town root for path resolution and session naming
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}
	_ = session.InitRegistry(townRoot)

	// List all polecat directories
	polecatsDir := filepath.Join(townRoot, rigName, "polecats")
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		return result // No polecats directory
	}

	t := tmux.NewTmux()

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecatName := entry.Name()
		sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
		result.Checked++

		// Only check live sessions with alive agents (the opposite of zombie detection)
		sessionAlive, err := t.HasSession(sessionName)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("checking session %s: %w", sessionName, err))
			continue
		}
		if !sessionAlive {
			continue // Dead session — zombie detection handles this
		}
		if !t.IsAgentAlive(sessionName) {
			continue // Dead agent — zombie detection handles this
		}

		// Agent is alive. Capture pane to check for known stall patterns.
		content, err := t.CapturePane(sessionName, 30)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("capturing pane for %s: %w", sessionName, err))
			continue
		}

		// Check for bypass-permissions prompt
		if strings.Contains(content, "Bypass Permissions mode") {
			stalled := StalledResult{
				PolecatName: polecatName,
				StallType:   "bypass-permissions",
			}
			if err := t.AcceptBypassPermissionsWarning(sessionName); err != nil {
				stalled.Action = "escalated"
				stalled.Error = fmt.Errorf("auto-dismiss failed: %w", err)
			} else {
				stalled.Action = "auto-dismissed"
			}
			result.Stalled = append(result.Stalled, stalled)
		}
	}

	return result
}

// getAgentBeadState reads agent_state and hook_bead from an agent bead.
// Returns the agent_state string and hook_bead ID.
func getAgentBeadState(workDir, agentBeadID string) (agentState, hookBead string) {
	output, err := util.ExecWithOutput(workDir, "bd", "show", agentBeadID, "--json")
	if err != nil || output == "" {
		return "", ""
	}

	// Parse JSON response — bd show --json returns an array
	var issues []struct {
		AgentState string `json:"agent_state"`
		HookBead   string `json:"hook_bead"`
	}
	if err := json.Unmarshal([]byte(output), &issues); err != nil || len(issues) == 0 {
		return "", ""
	}

	return issues[0].AgentState, issues[0].HookBead
}

// getBeadStatus returns the status of a bead (e.g., "open", "closed", "hooked").
// Returns empty string if the bead doesn't exist or can't be queried.
func getBeadStatus(workDir, beadID string) string {
	if beadID == "" {
		return ""
	}
	output, err := util.ExecWithOutput(workDir, "bd", "show", beadID, "--json")
	if err != nil || output == "" {
		return ""
	}
	var issues []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(output), &issues); err != nil || len(issues) == 0 {
		return ""
	}
	return issues[0].Status
}

// resetAbandonedBead resets a dead polecat's hooked bead so it can be re-dispatched.
// If the bead is in "hooked" or "in_progress" status, it:
// 1. Resets status to open
// 2. Clears assignee
// 3. Sends mail to deacon for re-dispatch
// Returns true if the bead was recovered.
func resetAbandonedBead(workDir, rigName, hookBead, polecatName string, router *mail.Router) bool {
	if hookBead == "" {
		return false
	}
	status := getBeadStatus(workDir, hookBead)
	if status != "hooked" && status != "in_progress" {
		return false
	}

	// Reset bead status to open and clear assignee
	if err := util.ExecRun(workDir, "bd", "update", hookBead, "--status=open", "--assignee="); err != nil {
		return false
	}

	// Send mail to deacon for re-dispatch
	if router != nil {
		msg := &mail.Message{
			From:     fmt.Sprintf("%s/witness", rigName),
			To:       "deacon/",
			Subject:  fmt.Sprintf("RECOVERED_BEAD %s", hookBead),
			Priority: mail.PriorityHigh,
			Body: fmt.Sprintf(`Recovered abandoned bead from dead polecat.

Bead: %s
Polecat: %s/%s
Previous Status: %s

The bead has been reset to open with no assignee.
Please re-dispatch to an available polecat.`,
				hookBead, rigName, polecatName, status),
		}
		_ = router.Send(msg) // Best-effort
	}

	return true
}

// OrphanedBeadResult contains a single detected orphaned bead.
type OrphanedBeadResult struct {
	BeadID        string
	Assignee      string // Original assignee (e.g. "gastown/polecats/alpha")
	PolecatName   string // Extracted polecat name
	BeadRecovered bool
}

// DetectOrphanedBeadsResult contains the results of an orphaned bead scan.
type DetectOrphanedBeadsResult struct {
	Checked int
	Orphans []OrphanedBeadResult
	Errors  []error
}

// DetectOrphanedBeads finds in_progress or hooked beads assigned to non-existent polecats.
//
// This complements DetectZombiePolecats which scans FROM polecat directories.
// If a polecat was nuked and its directory removed, DetectZombiePolecats won't
// see it, but the bead remains in_progress/hooked. This function scans FROM
// beads to catch that case.
func DetectOrphanedBeads(workDir, rigName string, router *mail.Router) *DetectOrphanedBeadsResult {
	result := &DetectOrphanedBeadsResult{}

	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}
	_ = session.InitRegistry(townRoot)

	// Scan both in_progress and hooked beads — resetAbandonedBead handles both
	// states, and orphaned beads can be stuck in either.
	var beadList []struct {
		ID       string `json:"id"`
		Assignee string `json:"assignee"`
	}
	for _, status := range []string{"in_progress", "hooked"} {
		output, err := util.ExecWithOutput(workDir, "bd", "list", "--status="+status, "--json", "--limit=0")
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("listing %s beads: %w", status, err))
			continue
		}
		if output == "" {
			continue
		}
		var batch []struct {
			ID       string `json:"id"`
			Assignee string `json:"assignee"`
		}
		if err := json.Unmarshal([]byte(output), &batch); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("parsing %s beads: %w", status, err))
			continue
		}
		beadList = append(beadList, batch...)
	}

	t := tmux.NewTmux()

	for _, bead := range beadList {
		if bead.Assignee == "" {
			continue // No assignee — not a dead-polecat orphan
		}

		// Parse assignee: "rigname/polecats/polecatname"
		parts := strings.Split(bead.Assignee, "/")
		if len(parts) != 3 || parts[1] != "polecats" {
			continue // Not a polecat assignee (crew, refinery, etc.)
		}
		assigneeRig := parts[0]
		polecatName := parts[2]

		// Only check beads assigned to polecats in this rig
		if assigneeRig != rigName {
			continue
		}
		result.Checked++

		// Check if the polecat's tmux session exists
		sessionName := session.PolecatSessionName(session.PrefixFor(assigneeRig), polecatName)
		sessionAlive, err := t.HasSession(sessionName)
		if err != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("checking session %s for bead %s: %w", sessionName, bead.ID, err))
			continue
		}
		if sessionAlive {
			continue // Polecat is alive — not an orphan
		}

		// Session is dead. Also check if polecat directory still exists
		// (if dir exists, DetectZombiePolecats will handle it)
		polecatsDir := filepath.Join(townRoot, assigneeRig, "polecats", polecatName)
		if _, statErr := os.Stat(polecatsDir); statErr == nil {
			continue // Directory exists — DetectZombiePolecats handles this case
		} else if !os.IsNotExist(statErr) {
			// Transient error (permission denied, I/O error) — skip to avoid false recovery
			result.Errors = append(result.Errors,
				fmt.Errorf("checking polecat dir %s for bead %s: %w", polecatsDir, bead.ID, statErr))
			continue
		}

		// Re-check directory and session immediately before reset to narrow the
		// TOCTOU window — a polecat could have been recreated between the first
		// checks and now.
		if _, statErr := os.Stat(polecatsDir); statErr == nil {
			continue // Directory reappeared — skip, not an orphan anymore
		} else if !os.IsNotExist(statErr) {
			result.Errors = append(result.Errors,
				fmt.Errorf("re-checking polecat dir %s for bead %s: %w", polecatsDir, bead.ID, statErr))
			continue
		}
		if alive, _ := t.HasSession(sessionName); alive {
			continue // Session reappeared — polecat was respawned, not an orphan
		}

		// Polecat is truly gone (no session, no directory). Reset the bead.
		orphan := OrphanedBeadResult{
			BeadID:      bead.ID,
			Assignee:    bead.Assignee,
			PolecatName: polecatName,
		}
		orphan.BeadRecovered = resetAbandonedBead(workDir, assigneeRig, bead.ID, polecatName, router)
		result.Orphans = append(result.Orphans, orphan)
	}

	return result
}

// OrphanedMoleculeResult represents a single orphaned molecule detection.
type OrphanedMoleculeResult struct {
	BeadID        string // The base work bead with the orphaned molecule
	MoleculeID    string // The attached molecule (wisp) ID
	Assignee      string // The dead polecat's full address
	PolecatName   string // Just the polecat name
	Closed        int    // Number of issues closed (molecule + descendants)
	BeadRecovered bool   // Whether the parent bead was reset for re-dispatch
	Error         error
}

// DetectOrphanedMoleculesResult holds aggregate results of the orphan scan.
type DetectOrphanedMoleculesResult struct {
	Checked int                      // Number of polecat-assigned beads checked
	Orphans []OrphanedMoleculeResult // Orphaned molecules found and processed
	Errors  []error
}

// DetectOrphanedMolecules scans for mol-polecat-work molecule instances whose
// owning polecat no longer exists. For each orphaned molecule, it closes the
// molecule and its descendant step issues, unblocking the parent work bead.
//
// Detection chain: hooked/in_progress bead → polecat assignee → check existence →
// read attached_molecule → close molecule + descendants.
//
// This complements DetectZombiePolecats (which scans FROM polecat directories)
// by scanning FROM beads. Once a polecat is nuked and its directory removed,
// DetectZombiePolecats can't see it — but the orphaned molecules remain.
//
// See: https://github.com/steveyegge/gastown/issues/1381
func DetectOrphanedMolecules(workDir, rigName string, router *mail.Router) *DetectOrphanedMoleculesResult {
	result := &DetectOrphanedMoleculesResult{}

	// Find town root for path resolution and session naming
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		townRoot = workDir
	}
	_ = session.InitRegistry(townRoot)

	// Step 1: List beads that could have attached molecules.
	// Slung beads start as status=hooked; polecats may change them to in_progress.
	type beadSummary struct {
		ID       string `json:"id"`
		Assignee string `json:"assignee"`
	}
	var allBeads []beadSummary
	for _, status := range []string{"hooked", "in_progress"} {
		output, err := util.ExecWithOutput(workDir, "bd", "list", "--status="+status, "--json", "--limit=0")
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("listing %s beads: %w", status, err))
			continue
		}
		if output == "" {
			continue
		}
		var items []beadSummary
		if err := json.Unmarshal([]byte(output), &items); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("parsing %s beads: %w", status, err))
			continue
		}
		allBeads = append(allBeads, items...)
	}

	if len(allBeads) == 0 {
		return result
	}

	// Step 2: Check each polecat-assigned bead
	polecatPrefix := rigName + "/polecats/"
	t := tmux.NewTmux()
	polecatsDir := filepath.Join(townRoot, rigName, "polecats")

	for _, b := range allBeads {
		if !strings.HasPrefix(b.Assignee, polecatPrefix) {
			continue
		}

		polecatName := strings.TrimPrefix(b.Assignee, polecatPrefix)
		result.Checked++

		// Check if polecat still has a tmux session
		sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
		hasSession, sessionErr := t.HasSession(sessionName)
		if sessionErr != nil {
			result.Errors = append(result.Errors,
				fmt.Errorf("checking session %s for bead %s: %w", sessionName, b.ID, sessionErr))
			continue
		}
		if hasSession {
			continue // Polecat is alive
		}

		// Check if polecat directory still exists (might be mid-cleanup)
		polecatDir := filepath.Join(polecatsDir, polecatName)
		if _, statErr := os.Stat(polecatDir); statErr == nil {
			continue // Directory exists; DetectZombiePolecats handles these
		} else if !os.IsNotExist(statErr) {
			// Transient error (permission denied, I/O error) — skip to avoid false positive
			result.Errors = append(result.Errors,
				fmt.Errorf("checking polecat dir %s for bead %s: %w", polecatDir, b.ID, statErr))
			continue
		}

		// TOCTOU re-check: polecat could have been recreated between initial
		// checks and now. Re-verify before destructive action.
		if _, statErr := os.Stat(polecatDir); statErr == nil {
			continue // Directory reappeared — skip
		} else if !os.IsNotExist(statErr) {
			result.Errors = append(result.Errors,
				fmt.Errorf("re-checking polecat dir %s for bead %s: %w", polecatDir, b.ID, statErr))
			continue
		}
		if alive, _ := t.HasSession(sessionName); alive {
			continue // Session reappeared — polecat was respawned
		}

		// Polecat is dead and gone — read the full bead to check for attached molecule
		attachedMol := getAttachedMoleculeID(workDir, b.ID)
		if attachedMol == "" {
			continue // No molecule attached
		}

		// Check molecule status — skip if already closed
		molStatus := getBeadStatus(workDir, attachedMol)
		if molStatus == "closed" || molStatus == "" {
			continue
		}

		// Close the orphaned molecule and its descendants
		orphan := OrphanedMoleculeResult{
			BeadID:      b.ID,
			MoleculeID:  attachedMol,
			Assignee:    b.Assignee,
			PolecatName: polecatName,
		}

		closed, closeErr := closeMoleculeWithDescendants(workDir, attachedMol)
		if closeErr != nil {
			orphan.Error = closeErr
			result.Errors = append(result.Errors, closeErr)
		}
		orphan.Closed = closed

		// Reset the parent bead so it can be re-dispatched
		orphan.BeadRecovered = resetAbandonedBead(workDir, rigName, b.ID, polecatName, router)

		result.Orphans = append(result.Orphans, orphan)
	}

	return result
}

// getAttachedMoleculeID reads a bead and returns its attached_molecule ID, if any.
func getAttachedMoleculeID(workDir, beadID string) string {
	output, err := util.ExecWithOutput(workDir, "bd", "show", beadID, "--json")
	if err != nil || output == "" {
		return ""
	}

	var issues []struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(output), &issues); err != nil || len(issues) == 0 {
		return ""
	}

	fields := beads.ParseAttachmentFields(&beads.Issue{Description: issues[0].Description})
	if fields == nil {
		return ""
	}
	return fields.AttachedMolecule
}

// closeMoleculeWithDescendants closes a molecule and all its descendant step
// issues using the bd CLI. Returns the total number of issues closed.
func closeMoleculeWithDescendants(workDir, moleculeID string) (int, error) {
	// Recursively close descendants first (bottom-up)
	closed, descErr := closeDescendantsViaCLI(workDir, moleculeID)

	// Close the molecule itself
	reason := "Orphaned mol-polecat-work — owning polecat no longer exists (issue #1381)"
	if err := util.ExecRun(workDir, "bd", "close", moleculeID, "-r", reason); err != nil {
		closeErr := fmt.Errorf("closing molecule %s: %w", moleculeID, err)
		if descErr != nil {
			return closed, fmt.Errorf("%w; also: %v", closeErr, descErr)
		}
		return closed, closeErr
	}
	closed++

	return closed, descErr
}

// closeDescendantsViaCLI recursively closes descendant issues of a parent
// using bd CLI commands. Returns count of issues closed and any error.
func closeDescendantsViaCLI(workDir, parentID string) (int, error) {
	// List children of this parent
	output, err := util.ExecWithOutput(workDir, "bd", "list", "--parent="+parentID, "--json")
	if err != nil {
		return 0, fmt.Errorf("listing children of %s: %w", parentID, err)
	}
	if output == "" {
		return 0, nil
	}

	var children []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(output), &children); err != nil {
		return 0, fmt.Errorf("parsing children of %s: %w", parentID, err)
	}

	if len(children) == 0 {
		return 0, nil
	}

	// Recursively close grandchildren first
	totalClosed := 0
	var errs []error
	for _, child := range children {
		n, err := closeDescendantsViaCLI(workDir, child.ID)
		totalClosed += n
		if err != nil {
			errs = append(errs, err)
		}
	}

	// Close open direct children
	var idsToClose []string
	for _, child := range children {
		if child.Status != "closed" {
			idsToClose = append(idsToClose, child.ID)
		}
	}

	if len(idsToClose) > 0 {
		reason := "Orphaned mol-polecat-work step — owning polecat no longer exists"
		args := append([]string{"close"}, idsToClose...)
		args = append(args, "-r", reason)
		if err := util.ExecRun(workDir, "bd", args...); err != nil {
			errs = append(errs, fmt.Errorf("closing children of %s: %w", parentID, err))
		} else {
			totalClosed += len(idsToClose)
		}
	}

	if len(errs) > 0 {
		return totalClosed, errs[0]
	}
	return totalClosed, nil
}


// DoneIntent represents a parsed done-intent label from an agent bead.
type DoneIntent struct {
	ExitType  string
	Timestamp time.Time
}

// extractDoneIntent parses a done-intent:<type>:<unix-ts> label from a label list.
// Returns nil if no done-intent label is found or if the label is malformed.
func extractDoneIntent(labels []string) *DoneIntent {
	for _, label := range labels {
		if !strings.HasPrefix(label, "done-intent:") {
			continue
		}
		// Format: done-intent:<type>:<unix-ts>
		parts := strings.SplitN(label, ":", 3)
		if len(parts) != 3 {
			return nil // Malformed
		}
		ts, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return nil // Malformed timestamp
		}
		return &DoneIntent{
			ExitType:  parts[1],
			Timestamp: time.Unix(ts, 0),
		}
	}
	return nil
}

// getAgentBeadLabels reads the labels from an agent bead.
func getAgentBeadLabels(workDir, agentBeadID string) []string {
	output, err := util.ExecWithOutput(workDir, "bd", "show", agentBeadID, "--json")
	if err != nil || output == "" {
		return nil
	}

	var issues []struct {
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(output), &issues); err != nil || len(issues) == 0 {
		return nil
	}

	return issues[0].Labels
}

// sessionRecreated checks whether a tmux session was (re)created after the
// given timestamp. Returns true if the session exists and was created after
// detectedAt, indicating a new session replaced the dead one (TOCTOU guard).
func sessionRecreated(t *tmux.Tmux, sessionName string, detectedAt time.Time) bool {
	alive, err := t.HasSession(sessionName)
	if err != nil || !alive {
		return false // Still dead — not recreated
	}
	// Session exists now. Check if it was created after our detection.
	createdAt, err := session.SessionCreatedAt(sessionName)
	if err != nil {
		// Can't determine creation time — assume recreated to be safe.
		// Better to skip a real zombie than kill a live session.
		return true
	}
	return !createdAt.Before(detectedAt)
}

// findAnyCleanupWisp checks if any cleanup wisp already exists for a polecat,
// regardless of state. Used to prevent duplicate escalation on repeated patrol
// cycles for the same zombie.
func findAnyCleanupWisp(workDir, polecatName string) string {
	output, err := util.ExecWithOutput(workDir, "bd", "list",
		"--label", fmt.Sprintf("cleanup,polecat:%s", polecatName),
		"--status", "open",
		"--json",
	)
	if err != nil {
		return ""
	}
	if output == "" || output == "[]" || output == "null" {
		return ""
	}
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(output), &items); err != nil || len(items) == 0 {
		return ""
	}
	return items[0].ID
}
