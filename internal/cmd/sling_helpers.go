package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
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
// Uses the daemon when available for proper beads directory discovery.
// The daemon correctly handles .beads/redirect files and Dolt-based storage.
//
// CRITICAL: Does NOT set cmd.Dir. Let bd discover beads directory from cwd.
// Setting cmd.Dir breaks beads discovery in Dolt-based setups.
func verifyBeadExists(beadID string) error {
	cmd := exec.Command("bd", "show", beadID, "--json")
	// IMPORTANT: Do NOT set cmd.Dir - let bd discover beads directory from cwd.
	// Setting cmd.Dir breaks .beads/redirect file discovery with Dolt.

	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("bead '%s' not found", beadID)
	}
	return nil
}

// getBeadInfo returns status and assignee for a bead.
// Uses the daemon when available for consistency with verifyBeadExists.
// Does NOT set cmd.Dir to avoid breaking .beads/redirect discovery with Dolt.
func getBeadInfo(beadID string) (*beadInfo, error) {
	cmd := exec.Command("bd", "show", beadID, "--json")
	// IMPORTANT: Do NOT set cmd.Dir - let bd discover beads directory from cwd.
	out, err := cmd.Output()
	if err != nil {
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
	// Resolve the correct rig directory for this bead's prefix.
	// This ensures bd can find the bead in cross-rig scenarios (e.g., bd- beads from gastown).
	workDir := ""
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		workDir = beads.ResolveHookDir(townRoot, beadID, "")
	}

	// Get the bead to preserve existing description content
	showCmd := exec.Command("bd", "show", beadID, "--json")
	if workDir != "" {
		showCmd.Dir = workDir
	}
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}

	// Parse the bead
	var issues []beads.Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		if os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG") == "" {
			return fmt.Errorf("parsing bead: %w", err)
		}
	}
	issue := &beads.Issue{}
	if len(issues) > 0 {
		issue = &issues[0]
	} else if os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG") == "" {
		return fmt.Errorf("bead not found")
	}

	// Get or create attachment fields
	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		fields = &beads.AttachmentFields{}
	}

	// Set the args
	fields.AttachedArgs = args

	// Update the description
	newDesc := beads.SetAttachmentFields(issue, fields)
	if logPath := os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG"); logPath != "" {
		_ = os.WriteFile(logPath, []byte(newDesc), 0644)
	}

	// Update the bead (use same workDir as the show command)
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	if workDir != "" {
		updateCmd.Dir = workDir
	}
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

	// Resolve the correct rig directory for this bead's prefix.
	// This ensures bd can find the bead in cross-rig scenarios (e.g., bd- beads from gastown).
	workDir := ""
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		workDir = beads.ResolveHookDir(townRoot, beadID, "")
	}

	// Get the bead to preserve existing description content
	showCmd := exec.Command("bd", "show", beadID, "--json")
	if workDir != "" {
		showCmd.Dir = workDir
	}
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

	// Update the bead (use same workDir as the show command)
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	if workDir != "" {
		updateCmd.Dir = workDir
	}
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
	logPath := os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG")
	if logPath != "" {
		_ = os.WriteFile(logPath, []byte("called"), 0644)
	}

	// Resolve the correct rig directory for this bead's prefix.
	// This ensures bd can find the bead in cross-rig scenarios (e.g., bd- beads from gastown).
	workDir := ""
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		workDir = beads.ResolveHookDir(townRoot, beadID, "")
	}

	issue := &beads.Issue{}
	if logPath == "" {
		// Get the bead to preserve existing description content
		showCmd := exec.Command("bd", "show", beadID, "--json")
		if workDir != "" {
			showCmd.Dir = workDir
		}
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
		issue = &issues[0]
	}

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
	if logPath != "" {
		_ = os.WriteFile(logPath, []byte(newDesc), 0644)
	}

	// Update the bead (use same workDir as the show command)
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	if workDir != "" {
		updateCmd.Dir = workDir
	}
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
// Agent bead prefixes vary by role:
// - Mayor, Deacon: hq- prefix (town-level)
// - Witness, Refinery, Polecat: hq- prefix (town-level, fix for loc-1augh)
// - Crew: rig prefix (e.g., gt- for gastown) - created by crew_add.go with rig prefix
func agentIDToBeadID(agentID, townRoot string) string {
	// Normalize: strip trailing slash (resolveSelfTarget returns "mayor/" not "mayor")
	agentID = strings.TrimSuffix(agentID, "/")

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

	// Get the rig's prefix for crew beads (they use rig prefix, not hq-)
	rigPrefix := config.GetRigPrefix(townRoot, rig)

	switch {
	case len(parts) == 2 && parts[1] == "witness":
		// Witness uses hq- prefix (town-level)
		return beads.WitnessBeadIDTown(rig)
	case len(parts) == 2 && parts[1] == "refinery":
		// Refinery uses hq- prefix (town-level)
		return beads.RefineryBeadIDTown(rig)
	case len(parts) == 3 && parts[1] == "crew":
		// FIX (hq-8af330.6): Crew beads use rig prefix, not hq-.
		// Crew beads are created by crew_add.go with CrewBeadIDWithPrefix(prefix, rig, name).
		return beads.CrewBeadIDWithPrefix(rigPrefix, rig, parts[2])
	case len(parts) == 3 && parts[1] == "polecats":
		// Polecats use hq- prefix (town-level, fix for loc-1augh)
		return beads.PolecatBeadIDTown(rig, parts[2])
	default:
		return ""
	}
}

// updateAgentHookBead updates the agent bead's state and hook when work is slung.
// This enables the witness to see that each agent is working.
//
// Fix hq-cc7214.26: Auto-create agent bead if it doesn't exist.
// This handles cases where crew members exist but their agent beads weren't created.
func updateAgentHookBead(agentID, beadID, workDir, _ string) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		// Not in a Gas Town workspace - can't update agent bead
		fmt.Fprintf(os.Stderr, "Warning: couldn't find town root to update agent hook: %v\n", err)
		return
	}

	// Convert agent ID to agent bead ID
	// Agent bead prefixes depend on agent type:
	//   - Crew: uses rig prefix (e.g., gt-gastown-crew-max)
	//   - Polecats: uses hq- prefix (e.g., hq-gastown-polecat-Toast)
	//   - Witness/Refinery: uses hq- prefix
	//   - Mayor/Deacon: uses hq- prefix
	agentBeadID := agentIDToBeadID(agentID, townRoot)
	if agentBeadID == "" {
		return
	}

	// Resolve the correct working directory for the agent bead.
	// Agent beads with rig-level prefixes (e.g., go-) live in rig databases,
	// not the town database. Use prefix-based resolution to find the correct path.
	// This fixes go-19z: bd slot commands failing for go-* prefixed beads.
	agentWorkDir := beads.ResolveHookDir(townRoot, agentBeadID, workDir)

	// Run from agentWorkDir WITHOUT BEADS_DIR to enable redirect-based routing.
	// Set hook_bead to the slung work (gt-zecmc: removed agent_state update).
	// Agent liveness is observable from tmux - no need to record it in bead.
	// For cross-database scenarios, slot set may fail gracefully (warning only).
	bd := beads.New(agentWorkDir)
	if err := bd.SetHookBead(agentBeadID, beadID); err != nil {
		// Fix hq-cc7214.26: If the agent bead doesn't exist, try to auto-create it
		if strings.Contains(err.Error(), "issue not found") || strings.Contains(err.Error(), "no issue found") {
			if autoCreated := ensureAgentBeadExists(bd, agentID, agentBeadID, townRoot); autoCreated {
				// Retry setting the hook after creating the bead
				if retryErr := bd.SetHookBead(agentBeadID, beadID); retryErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s hook after creating bead: %v\n", agentBeadID, retryErr)
				}
				return
			}
		}
		// Log warning instead of silent ignore - helps debug cross-beads issues
		fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s hook: %v\n", agentBeadID, err)
		return
	}
}

// ensureAgentBeadExists creates an agent bead if it doesn't exist.
// Returns true if a bead was created, false otherwise.
// Fix for hq-cc7214.26: Sling fails when agent bead doesn't exist.
func ensureAgentBeadExists(bd *beads.Beads, agentID, agentBeadID, _ string) bool {
	// Parse agent ID to determine role type
	parts := strings.Split(strings.TrimSuffix(agentID, "/"), "/")
	if len(parts) < 2 {
		return false
	}

	var roleType, rigName, agentName string

	switch {
	case len(parts) == 3 && parts[1] == "crew":
		roleType = "crew"
		rigName = parts[0]
		agentName = parts[2]
	case len(parts) == 3 && parts[1] == "polecats":
		roleType = "polecat"
		rigName = parts[0]
		agentName = parts[2]
	case len(parts) == 2 && parts[1] == "witness":
		roleType = "witness"
		rigName = parts[0]
	case len(parts) == 2 && parts[1] == "refinery":
		roleType = "refinery"
		rigName = parts[0]
	default:
		return false
	}

	// Build description based on role type
	var desc string
	switch roleType {
	case "crew":
		desc = fmt.Sprintf("Crew worker %s in %s - human-managed persistent workspace.", agentName, rigName)
	case "polecat":
		desc = fmt.Sprintf("Polecat %s in %s - ephemeral worker agent.", agentName, rigName)
	case "witness":
		desc = fmt.Sprintf("Witness for %s - monitors polecats and handles escalations.", rigName)
	case "refinery":
		desc = fmt.Sprintf("Refinery for %s - merges completed work to main.", rigName)
	}

	fields := &beads.AgentFields{
		RoleType:   roleType,
		Rig:        rigName,
		AgentState: "idle",
	}

	if _, err := bd.CreateAgentBead(agentBeadID, desc, fields); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't auto-create agent bead %s: %v\n", agentBeadID, err)
		return false
	}

	fmt.Printf("  Auto-created agent bead: %s\n", agentBeadID)
	return true
}

// wakeRigAgents wakes the witness and refinery for a rig after polecat dispatch.
// This ensures the patrol agents are ready to monitor and merge.
//
// Issue hq-cc7214.7: gt rig boot requires finding the workspace from cwd.
// We must set bootCmd.Dir to townRoot so the subprocess can find the workspace.
func wakeRigAgents(rigName string) {
	// Find town root for setting working directory
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		// Can't find workspace - log and skip boot
		fmt.Fprintf(os.Stderr, "Warning: wakeRigAgents: can't find workspace: %v\n", err)
		return
	}

	// Boot the rig (idempotent - no-op if already running)
	// Must set Dir so gt rig boot can find the workspace via workspace.FindFromCwdOrError()
	bootCmd := exec.Command("gt", "rig", "boot", rigName)
	bootCmd.Dir = townRoot
	bootCmd.Stderr = os.Stderr // Surface any errors for debugging
	if err := bootCmd.Run(); err != nil {
		// Non-fatal: rig might already be running, or there may be transient issues
		fmt.Fprintf(os.Stderr, "Warning: wakeRigAgents: gt rig boot %s failed: %v\n", rigName, err)
	}

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

// isCrewTarget checks if the target string refers to a crew member.
// Returns true if the target format is "rig/crew/name".
// This is used to determine if we should auto-start a crew session
// instead of failing when slinging work.
func isCrewTarget(target string) bool {
	parts := strings.Split(target, "/")
	return len(parts) >= 3 && parts[1] == "crew"
}

// parseCrewTarget extracts rig and crew name from a crew target string.
// Returns empty strings if target is not a crew target.
func parseCrewTarget(target string) (rigName, crewName string, ok bool) {
	parts := strings.Split(target, "/")
	if len(parts) >= 3 && parts[1] == "crew" {
		return parts[0], parts[2], true
	}
	return "", "", false
}

// FormulaOnBeadResult contains the result of instantiating a formula on a bead.
type FormulaOnBeadResult struct {
	WispRootID string // The wisp root ID (compound root after bonding)
	BeadToHook string // The bead ID to hook (BASE bead, not wisp - lifecycle fix)
}

// InstantiateFormulaOnBead creates a wisp from a formula, bonds it to a bead.
// This is the formula-on-bead pattern used by issue #288 for auto-applying mol-polecat-work.
//
// Parameters:
//   - formulaName: the formula to instantiate (e.g., "mol-polecat-work")
//   - beadID: the base bead to bond the wisp to
//   - title: the bead title (used for --var feature=<title>)
//   - hookWorkDir: working directory for bd commands (polecat's worktree)
//   - townRoot: the town root directory
//   - skipCook: if true, skip cooking (for batch mode optimization where cook happens once)
//
// Returns the wisp root ID which should be hooked.
func InstantiateFormulaOnBead(formulaName, beadID, title, hookWorkDir, townRoot string, skipCook bool) (*FormulaOnBeadResult, error) {
	// Route bd mutations (wisp/bond) to the correct beads context for the target bead.
	formulaWorkDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)

	// Step 1: Cook the formula (ensures proto exists)
	if !skipCook {
		cookCmd := exec.Command("bd", "--no-daemon", "cook", formulaName)
		cookCmd.Dir = formulaWorkDir
		cookCmd.Stderr = os.Stderr
		if err := cookCmd.Run(); err != nil {
			return nil, fmt.Errorf("cooking formula %s: %w", formulaName, err)
		}
	}

	// Step 2: Create wisp with feature and issue variables from bead
	featureVar := fmt.Sprintf("feature=%s", title)
	issueVar := fmt.Sprintf("issue=%s", beadID)
	wispArgs := []string{"--no-daemon", "mol", "wisp", formulaName, "--var", featureVar, "--var", issueVar, "--json"}
	wispCmd := exec.Command("bd", wispArgs...)
	wispCmd.Dir = formulaWorkDir
	wispCmd.Env = append(os.Environ(), "GT_ROOT="+townRoot)
	wispCmd.Stderr = os.Stderr
	wispOut, err := wispCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("creating wisp for formula %s: %w", formulaName, err)
	}

	// Parse wisp output to get the root ID
	wispRootID, err := parseWispIDFromJSON(wispOut)
	if err != nil {
		return nil, fmt.Errorf("parsing wisp output: %w", err)
	}

	// Step 3: Bond wisp to original bead (creates compound)
	bondArgs := []string{"--no-daemon", "mol", "bond", wispRootID, beadID, "--json"}
	bondCmd := exec.Command("bd", bondArgs...)
	bondCmd.Dir = formulaWorkDir
	bondCmd.Stderr = os.Stderr
	bondOut, err := bondCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bonding formula to bead: %w", err)
	}

	// Parse bond output - the wisp root becomes the compound root
	var bondResult struct {
		RootID string `json:"root_id"`
	}
	if err := json.Unmarshal(bondOut, &bondResult); err == nil && bondResult.RootID != "" {
		wispRootID = bondResult.RootID
	}

	return &FormulaOnBeadResult{
		WispRootID: wispRootID,
		BeadToHook: beadID, // Hook the BASE bead (lifecycle fix: wisp is attached_molecule)
	}, nil
}

// CookFormula cooks a formula to ensure its proto exists.
// This is useful for batch mode where we cook once before processing multiple beads.
func CookFormula(formulaName, workDir string) error {
	cookCmd := exec.Command("bd", "--no-daemon", "cook", formulaName)
	cookCmd.Dir = workDir
	cookCmd.Stderr = os.Stderr
	return cookCmd.Run()
}
