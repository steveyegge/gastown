package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// beadInfo holds status and assignee for a bead.
type beadInfo struct {
	Title    string `json:"title"`
	Status   string `json:"status"`
	Assignee string `json:"assignee"`
}

// verifyBeadExists checks that the bead exists using bd show.
// Uses bd's native prefix-based routing via routes.jsonl - do NOT set BEADS_DIR
// as that overrides routing and breaks resolution of rig-level beads.
//
// Uses --no-daemon with --allow-stale to avoid daemon socket timing issues
// while still finding beads when database is out of sync with JSONL.
// For existence checks, stale data is acceptable - we just need to know it exists.
func verifyBeadExists(beadID string) error {
	cmd := exec.Command("bd", "--no-daemon", "show", beadID, "--json", "--allow-stale")
	// Run from town root so bd can find routes.jsonl for prefix-based routing.
	// Do NOT set BEADS_DIR - that overrides routing and breaks rig bead resolution.
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		cmd.Dir = townRoot
	}
	// Use Output() instead of Run() to detect bd --no-daemon exit 0 bug:
	// when issue not found, --no-daemon exits 0 but produces empty stdout.
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("bead '%s' not found (bd show failed)", beadID)
	}
	if len(out) == 0 {
		return fmt.Errorf("bead '%s' not found", beadID)
	}
	return nil
}

// getBeadInfo returns status and assignee for a bead.
// Uses bd's native prefix-based routing via routes.jsonl.
// Uses --no-daemon with --allow-stale for consistency with verifyBeadExists.
func getBeadInfo(beadID string) (*beadInfo, error) {
	cmd := exec.Command("bd", "--no-daemon", "show", beadID, "--json", "--allow-stale")
	// Run from town root so bd can find routes.jsonl for prefix-based routing.
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		cmd.Dir = townRoot
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
	// Handle bd --no-daemon exit 0 bug: when issue not found,
	// --no-daemon exits 0 but produces empty stdout (error goes to stderr).
	if len(out) == 0 {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
	// bd show --json returns an array (issue + dependents), take first element
	var infos []beadInfo
	if err := json.Unmarshal(out, &infos); err != nil {
		return nil, fmt.Errorf("parsing bead info: %w", err)
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
	return &infos[0], nil
}

// storeArgsInBead stores args in the bead's description using attached_args field.
// This enables no-tmux mode where agents discover args via gt prime / bd show.
func storeArgsInBead(beadID, args string) error {
	// Get the bead to preserve existing description content
	showCmd := exec.Command("bd", "--no-daemon", "show", beadID, "--json", "--allow-stale")
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}
	// Handle bd --no-daemon exit 0 bug: empty stdout means not found
	if len(out) == 0 {
		return fmt.Errorf("bead not found")
	}

	// Parse the bead
	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return fmt.Errorf("parsing bead: %w", err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("bead not found")
	}
	issue := &issues[0]

	// Get or create attachment fields
	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}

	// Set the args
	fields.AttachedArgs = args

	// Update the description
	newDesc := beads.SetAttachmentFields(issue, fields)

	// Update the bead
	updateCmd := exec.Command("bd", "--no-daemon", "update", beadID, "--description="+newDesc)
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("updating bead description: %w", err)
	}

	return nil
}

// storeDispatcherInBead stores the dispatcher agent ID in the bead's description.
// This enables polecats to notify the dispatcher when work is complete.
func storeDispatcherInBead(beadID, dispatcher string) error {
	if dispatcher == "" {
		return nil
	}

	// Get the bead to preserve existing description content
	showCmd := exec.Command("bd", "show", beadID, "--json")
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}

	// Parse the bead
	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return fmt.Errorf("parsing bead: %w", err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("bead not found")
	}
	issue := &issues[0]

	// Get or create attachment fields
	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}

	// Set the dispatcher
	fields.DispatchedBy = dispatcher

	// Update the description
	newDesc := beads.SetAttachmentFields(issue, fields)

	// Update the bead
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("updating bead description: %w", err)
	}

	return nil
}

// storeAttachedMoleculeInBead sets the attached_molecule field in a bead's description.
// This is required for gt hook to recognize that a molecule is attached to the bead.
// Called after bonding a formula wisp to a bead via "gt sling <formula> --on <bead>".
func storeAttachedMoleculeInBead(beadID, moleculeID string) error {
	if moleculeID == "" {
		return nil
	}

	// Get the bead to preserve existing description content
	showCmd := exec.Command("bd", "show", beadID, "--json")
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}

	// Parse the bead
	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return fmt.Errorf("parsing bead: %w", err)
	}
	if len(issues) == 0 {
		return fmt.Errorf("bead not found")
	}
	issue := &issues[0]

	// Get or create attachment fields
	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}

	// Set the attached molecule
	fields.AttachedMolecule = moleculeID
	if fields.AttachedAt == "" {
		fields.AttachedAt = time.Now().UTC().Format(time.RFC3339)
	}

	// Update the description
	newDesc := beads.SetAttachmentFields(issue, fields)

	// Update the bead
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("updating bead description: %w", err)
	}

	return nil
}

// injectStartPrompt sends a prompt to the target pane to start working.
// Uses the reliable nudge pattern: literal mode + 500ms debounce + separate Enter.
func injectStartPrompt(pane, beadID, subject, args string) error {
	if pane == "" {
		return fmt.Errorf("no target pane")
	}

	// Skip nudge during tests to prevent agent self-interruption
	if os.Getenv("GT_TEST_NO_NUDGE") != "" {
		return nil
	}

	// Build the prompt to inject
	var prompt string
	if args != "" {
		// Args provided - include them prominently in the prompt
		if subject != "" {
			prompt = fmt.Sprintf("Work slung: %s (%s). Args: %s. Start working now - use these args to guide your execution.", beadID, subject, args)
		} else {
			prompt = fmt.Sprintf("Work slung: %s. Args: %s. Start working now - use these args to guide your execution.", beadID, args)
		}
	} else if subject != "" {
		prompt = fmt.Sprintf("Work slung: %s (%s). Start working on it now - no questions, just begin.", beadID, subject)
	} else {
		prompt = fmt.Sprintf("Work slung: %s. Start working on it now - run `gt hook` to see the hook, then begin.", beadID)
	}

	// Use the reliable nudge pattern (same as gt nudge / tmux.NudgeSession)
	t := tmux.NewTmux()
	return t.NudgePane(pane, prompt)
}

// getSessionFromPane extracts session name from a pane target.
// Pane targets can be:
// - "%9" (pane ID) - need to query tmux for session
// - "gt-rig-name:0.0" (session:window.pane) - extract session name
func getSessionFromPane(pane string) string {
	if strings.HasPrefix(pane, "%") {
		// Pane ID format - query tmux for the session
		cmd := exec.Command("tmux", "display-message", "-t", pane, "-p", "#{session_name}")
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	// Session:window.pane format - extract session name
	if idx := strings.Index(pane, ":"); idx > 0 {
		return pane[:idx]
	}
	return pane
}

// ensureAgentReady waits for an agent to be ready before nudging an existing session.
// Uses a pragmatic approach: wait for the pane to leave a shell, then (Claude-only)
// accept the bypass permissions warning and give it a moment to finish initializing.
func ensureAgentReady(sessionName string) error {
	t := tmux.NewTmux()

	// If an agent is already running, assume it's ready (session was started earlier)
	if t.IsAgentRunning(sessionName) {
		return nil
	}

	// Agent not running yet - wait for it to start (shell → program transition)
	if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		return fmt.Errorf("waiting for agent to start: %w", err)
	}

	// Claude-only: accept bypass permissions warning if present
	if t.IsClaudeRunning(sessionName) {
		_ = t.AcceptBypassPermissionsWarning(sessionName)

		// PRAGMATIC APPROACH: fixed delay rather than prompt detection.
		// Claude startup takes ~5-8 seconds on typical machines.
		time.Sleep(8 * time.Second)
	} else {
		time.Sleep(1 * time.Second)
	}

	return nil
}

// detectCloneRoot finds the root of the current git clone.
func detectCloneRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// detectActor returns the current agent's actor string for event logging.
func detectActor() string {
	roleInfo, err := GetRole()
	if err != nil {
		return "unknown"
	}
	return roleInfo.ActorString()
}

// agentIDToBeadID converts an agent ID to its corresponding agent bead ID.
// All agent beads use hq- prefix and are stored in town beads to avoid
// prefix/database mismatch issues (fix for loc-1augh).
// townRoot is kept for backward compatibility but no longer used.
func agentIDToBeadID(agentID, townRoot string) string {
	_ = townRoot // No longer needed - all agents use hq- prefix

	// Handle simple cases (town-level agents with hq- prefix)
	if agentID == "mayor" {
		return beads.MayorBeadIDTown()
	}
	if agentID == "deacon" {
		return beads.DeaconBeadIDTown()
	}

	// Parse path-style agent IDs
	parts := strings.Split(agentID, "/")
	if len(parts) < 2 {
		return ""
	}

	rig := parts[0]

	// All rig-level agents use hq- prefix in town beads (fix for loc-1augh)
	switch {
	case len(parts) == 2 && parts[1] == "witness":
		return beads.WitnessBeadIDTown(rig)
	case len(parts) == 2 && parts[1] == "refinery":
		return beads.RefineryBeadIDTown(rig)
	case len(parts) == 3 && parts[1] == "crew":
		return beads.CrewBeadIDTown(rig, parts[2])
	case len(parts) == 3 && parts[1] == "polecats":
		return beads.PolecatBeadIDTown(rig, parts[2])
	default:
		return ""
	}
}

// updateAgentHookBead updates the agent bead's state and hook when work is slung.
// This enables the witness to see that each agent is working.
//
// Agent beads use hq- prefix and are stored in town beads (fix for loc-1augh).
// We run bd commands from town root with BEADS_DIR pointing to town beads.
func updateAgentHookBead(agentID, beadID, workDir, townBeadsDir string) {
	_ = workDir // No longer used - agent beads are in town beads

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		// Not in a Gas Town workspace - can't update agent bead
		fmt.Fprintf(os.Stderr, "Warning: couldn't find town root to update agent hook: %v\n", err)
		return
	}

	// Convert agent ID to agent bead ID (now uses hq- prefix)
	// Format examples:
	//   greenplace/crew/max -> hq-greenplace-crew-max
	//   greenplace/polecats/Toast -> hq-greenplace-polecat-Toast
	//   mayor -> hq-mayor
	//   greenplace/witness -> hq-greenplace-witness
	agentBeadID := agentIDToBeadID(agentID, townRoot)
	if agentBeadID == "" {
		return
	}

	// Use town beads for agent bead operations (fix for loc-1augh).
	// townBeadsDir points to the town's .beads directory.
	bd := beads.NewWithBeadsDir(townRoot, townBeadsDir)
	if err := bd.SetHookBead(agentBeadID, beadID); err != nil {
		// Log warning instead of silent ignore - helps debug cross-beads issues
		fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s hook: %v\n", agentBeadID, err)
		return
	}
}

// wakeRigAgents wakes the witness and refinery for a rig after polecat dispatch.
// This ensures the patrol agents are ready to monitor and merge.
func wakeRigAgents(rigName string) {
	// Boot the rig (idempotent - no-op if already running)
	bootCmd := exec.Command("gt", "rig", "boot", rigName)
	_ = bootCmd.Run() // Ignore errors - rig might already be running

	// Nudge witness and refinery to clear any backoff
	t := tmux.NewTmux()
	witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
	refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)

	// Silent nudges - sessions might not exist yet
	_ = t.NudgeSession(witnessSession, "Polecat dispatched - check for work")
	_ = t.NudgeSession(refinerySession, "Polecat dispatched - check for merge requests")
}

// isPolecatTarget checks if the target string refers to a polecat.
// Returns true if the target format is "rig/polecats/name".
// This is used to determine if we should respawn a dead polecat
// instead of failing when slinging work.
func isPolecatTarget(target string) bool {
	parts := strings.Split(target, "/")
	return len(parts) >= 3 && parts[1] == "polecats"
}

// attachPolecatWorkMolecule attaches the mol-polecat-work molecule to a polecat's agent bead.
// This ensures all polecats have the standard work molecule attached for guidance.
// The molecule is attached by storing it in the agent bead's description using attachment fields.
//
// Per issue #288: gt sling should auto-attach mol-polecat-work when slinging to polecats.
// Agent beads use hq- prefix and are stored in town beads (fix for loc-1augh).
func attachPolecatWorkMolecule(targetAgent, hookWorkDir, townRoot string) error {
	_ = hookWorkDir // No longer used - agent beads are in town beads

	// Parse the polecat name from targetAgent (format: "rig/polecats/name")
	parts := strings.Split(targetAgent, "/")
	if len(parts) != 3 || parts[1] != "polecats" {
		return fmt.Errorf("invalid polecat agent format: %s", targetAgent)
	}
	rigName := parts[0]
	polecatName := parts[2]

	// Get the polecat's agent bead ID (uses hq- prefix, fix for loc-1augh)
	// Format: "hq-<rig>-polecat-<name>" (e.g., "hq-gastown-polecat-Toast")
	agentBeadID := beads.PolecatBeadIDTown(rigName, polecatName)

	// Use town beads for agent bead operations (fix for loc-1augh)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	b := beads.NewWithBeadsDir(townRoot, townBeadsDir)

	// Check if molecule is already attached (avoid duplicate attach)
	attachment, err := b.GetAttachment(agentBeadID)
	if err == nil && attachment != nil && attachment.AttachedMolecule != "" {
		// Already has a molecule attached - skip
		return nil
	}

	// Cook the mol-polecat-work formula to ensure the proto exists
	// This is safe to run multiple times - cooking is idempotent
	// Cook from town root since formulas are defined at town level
	cookCmd := exec.Command("bd", "--no-daemon", "cook", "mol-polecat-work")
	cookCmd.Dir = townRoot
	cookCmd.Stderr = os.Stderr
	if err := cookCmd.Run(); err != nil {
		return fmt.Errorf("cooking mol-polecat-work formula: %w", err)
	}

	// Attach the molecule to the polecat's agent bead
	// The molecule ID is the formula name "mol-polecat-work"
	moleculeID := "mol-polecat-work"
	_, err = b.AttachMolecule(agentBeadID, moleculeID)
	if err != nil {
		return fmt.Errorf("attaching molecule %s to %s: %w", moleculeID, agentBeadID, err)
	}

	fmt.Printf("%s Attached %s to %s\n", style.Bold.Render("✓"), moleculeID, agentBeadID)
	return nil
}
