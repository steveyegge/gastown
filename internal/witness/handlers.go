package witness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
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

	// Ephemeral model: try to auto-nuke immediately regardless of MR status
	// If cleanup_status is clean, the branch is pushed and polecat is recyclable.
	// The MR will be processed independently by the Refinery.
	nukeResult := AutoNukeIfClean(workDir, rigName, payload.PolecatName)
	if nukeResult.Nuked {
		result.Handled = true
		if hasPendingMR {
			// Ephemeral model: polecat nuked, MR continues in Refinery
			result.Action = fmt.Sprintf("auto-nuked %s (ephemeral: exit=%s, MR=%s): %s", payload.PolecatName, payload.Exit, payload.MRID, nukeResult.Reason)
		} else {
			result.Action = fmt.Sprintf("auto-nuked %s (exit=%s, no MR): %s", payload.PolecatName, payload.Exit, nukeResult.Reason)
		}
		return result
	}
	if nukeResult.Error != nil {
		// Nuke failed - fall through to create wisp for manual cleanup
		result.Error = nukeResult.Error
	}

	// Couldn't auto-nuke (dirty state or verification failed) - create wisp for manual intervention
	// Note: Even with pending MR, if we can't auto-nuke it means something is wrong
	// (uncommitted changes, unpushed commits, etc.) that needs attention.
	wispID, err := createCleanupWisp(workDir, payload.PolecatName, payload.IssueID, payload.Branch)
	if err != nil {
		result.Error = fmt.Errorf("creating cleanup wisp: %w", err)
		return result
	}

	result.Handled = true
	result.WispCreated = wispID
	if hasPendingMR {
		result.Action = fmt.Sprintf("created cleanup wisp %s for %s (MR=%s, needs intervention: %s)", wispID, payload.PolecatName, payload.MRID, nukeResult.Reason)
	} else {
		result.Action = fmt.Sprintf("created cleanup wisp %s for %s (needs manual cleanup: %s)", wispID, payload.PolecatName, nukeResult.Reason)
	}

	return result
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

	cmd := exec.Command("bd", "create", //nolint:gosec // G204: args are constructed internally
		"--wisp",
		"--title", title,
		"--description", description,
		"--labels", labels,
	)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return "", err
	}

	// Extract wisp ID from output (bd create outputs "Created: <id>")
	output := strings.TrimSpace(stdout.String())
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

	cmd := exec.Command("bd", "create", //nolint:gosec // G204: args are constructed internally
		"--wisp",
		"--title", title,
		"--description", description,
		"--labels", labels,
	)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return "", err
	}

	output := strings.TrimSpace(stdout.String())
	if strings.HasPrefix(output, "Created:") {
		return strings.TrimSpace(strings.TrimPrefix(output, "Created:")), nil
	}

	return output, nil
}

// findCleanupWisp finds an existing cleanup wisp for a polecat.
func findCleanupWisp(workDir, polecatName string) (string, error) {
	cmd := exec.Command("bd", "list", //nolint:gosec // G204: bd is a trusted internal tool
		"--wisp",
		"--labels", fmt.Sprintf("polecat:%s,state:merge-requested", polecatName),
		"--status", "open",
		"--json",
	)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Empty result is fine
		if strings.Contains(stderr.String(), "no issues found") {
			return "", nil
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%s", errMsg)
		}
		return "", err
	}

	// Parse JSON to get the wisp ID
	output := strings.TrimSpace(stdout.String())
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
	// Construct agent bead ID using the rig's configured prefix
	// This supports non-gt prefixes like "bd-" for the beads rig
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		// Fall back to default prefix
		townRoot = workDir
	}
	prefix := beads.GetPrefixForRig(townRoot, rigName)
	agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)

	cmd := exec.Command("bd", "show", agentBeadID, "--json") //nolint:gosec // G204: agentBeadID is validated internally
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Agent bead doesn't exist or bd failed - return empty (unknown status)
		return ""
	}

	output := stdout.Bytes()
	if len(output) == 0 {
		return ""
	}

	// Parse the JSON response
	var resp agentBeadResponse
	if err := json.Unmarshal(output, &resp); err != nil {
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
	cmd := exec.Command("bd", "show", wispID, "--json")
	cmd.Dir = workDir

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("getting wisp: %w", err)
	}

	// Extract polecat name from existing labels for the update
	output := stdout.String()
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

	updateCmd := exec.Command("bd", "update", wispID, "--labels", newLabels) //nolint:gosec // G204: args are constructed internally
	updateCmd.Dir = workDir

	var stderr bytes.Buffer
	updateCmd.Stderr = &stderr

	if err := updateCmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}

	return nil
}

// NukePolecat executes the actual nuke operation for a polecat.
// This kills the tmux session, removes the worktree, and cleans up beads.
// Should only be called after all safety checks pass.
func NukePolecat(workDir, rigName, polecatName string) error {
	address := fmt.Sprintf("%s/%s", rigName, polecatName)

	cmd := exec.Command("gt", "polecat", "nuke", address) //nolint:gosec // G204: address is constructed from validated internal data
	cmd.Dir = workDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("nuke failed: %s", errMsg)
		}
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
// This is used for idle polecats with no pending MR - they can be nuked immediately.
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

// verifyCommitOnMain checks if the polecat's current commit is on main.
// This prevents nuking a polecat whose work wasn't actually merged.
//
// Returns:
//   - true, nil: commit is verified on main
//   - false, nil: commit is NOT on main (don't nuke!)
//   - false, error: couldn't verify (treat as unsafe)
func verifyCommitOnMain(workDir, rigName, polecatName string) (bool, error) {
	// Find town root from workDir
	townRoot, err := workspace.Find(workDir)
	if err != nil || townRoot == "" {
		return false, fmt.Errorf("finding town root: %v", err)
	}

	// Construct polecat path: <townRoot>/<rigName>/polecats/<polecatName>
	polecatPath := filepath.Join(townRoot, rigName, "polecats", polecatName)

	// Get git for the polecat worktree
	g := git.NewGit(polecatPath)

	// Get the current HEAD commit SHA
	commitSHA, err := g.Rev("HEAD")
	if err != nil {
		return false, fmt.Errorf("getting polecat HEAD: %w", err)
	}

	// Verify it's an ancestor of main (i.e., it's been merged)
	// We use the polecat's git context to check main
	isOnMain, err := g.IsAncestor(commitSHA, "origin/main")
	if err != nil {
		// Try without origin/ prefix in case remote isn't set up
		isOnMain, err = g.IsAncestor(commitSHA, "main")
		if err != nil {
			return false, fmt.Errorf("checking if commit is on main: %w", err)
		}
	}

	return isOnMain, nil
}
