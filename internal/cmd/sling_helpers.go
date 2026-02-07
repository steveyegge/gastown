package cmd

import (
	"github.com/steveyegge/gastown/internal/cli"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// resolveBeadDir returns the directory to run bd commands for a given bead ID.
// Uses prefix-based routing to find the correct rig directory.
// Falls back to rigs.json prefix mapping, then town root.
func resolveBeadDir(beadID string) string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "."
	}
	prefix := beads.ExtractPrefix(beadID)
	if rigPath := beads.GetRigPathForPrefix(townRoot, prefix); rigPath != "" {
		return rigPath
	}
	// Fallback: consult rigs.json for prefix-to-rig mapping
	if rigDir := resolveBeadDirFromRigsJSON(townRoot, prefix); rigDir != "" {
		return rigDir
	}
	return townRoot
}

// resolveBeadDirFromRigsJSON looks up the rig directory from rigs.json using prefix.
func resolveBeadDirFromRigsJSON(townRoot, prefix string) string {
	rigsPath := townRoot + "/mayor/rigs.json"
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return ""
	}
	var rigsFile struct {
		Rigs map[string]struct {
			Beads struct {
				Prefix string `json:"prefix"`
			} `json:"beads"`
		} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &rigsFile); err != nil {
		return ""
	}
	// prefix includes trailing hyphen (e.g., "bd-"), rigs.json stores without (e.g., "bd")
	trimmedPrefix := strings.TrimSuffix(prefix, "-")
	for rigName, rigConfig := range rigsFile.Rigs {
		if rigConfig.Beads.Prefix == trimmedPrefix {
			// Return mayor/rig path within the rig (where .beads/ lives)
			return townRoot + "/" + rigName + "/mayor/rig"
		}
	}
	return ""
}

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
// Checks bead existence using bd show.
// Resolves the rig directory from the bead's prefix for correct dolt access.
func verifyBeadExists(beadID string) error {
	cmd := exec.Command("bd", "--no-daemon", "show", beadID, "--json", "--allow-stale")
	cmd.Dir = resolveBeadDir(beadID)
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
// Resolves the rig directory from the bead's prefix for correct dolt access.
func getBeadInfo(beadID string) (*beadInfo, error) {
	cmd := exec.Command("bd", "--no-daemon", "show", beadID, "--json", "--allow-stale")
	cmd.Dir = resolveBeadDir(beadID)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bead '%s' not found", beadID)
	}
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
	showCmd.Dir = resolveBeadDir(beadID)
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("fetching bead: %w", err)
	}
	if len(out) == 0 {
		return fmt.Errorf("bead not found")
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

	// Update the bead
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
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
	logPath := os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG")
	if logPath != "" {
		_ = os.WriteFile(logPath, []byte("called"), 0644)
	}

	issue := &beads.Issue{}
	if logPath == "" {
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

	// Update the bead
	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("updating bead description: %w", err)
	}

	return nil
}

// storeNoMergeInBead sets the no_merge field in a bead's description.
// When set, gt done will skip the merge queue and keep work on the feature branch.
// This is useful for upstream contributions or when human review is needed before merge.
func storeNoMergeInBead(beadID string, noMerge bool) error {
	if !noMerge {
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

	// Set the no_merge flag
	fields.NoMerge = true

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
		prompt = fmt.Sprintf("Work slung: %s. Start working on it now - run `" + cli.Name() + " hook` to see the hook, then begin.", beadID)
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
	agentName, _ := t.GetEnvironment(sessionName, "GT_AGENT")
	if agentName == "" || agentName == "claude" {
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
// Uses canonical naming: prefix-rig-role-name
// Town-level agents (Mayor, Deacon) use hq- prefix and are stored in town beads.
// Rig-level agents use the rig's configured prefix (default "gt-").
// townRoot is needed to look up the rig's configured prefix.
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
	prefix := beads.GetPrefixForRig(townRoot, rig)

	switch {
	case len(parts) == 2 && parts[1] == "witness":
		return beads.WitnessBeadIDWithPrefix(prefix, rig)
	case len(parts) == 2 && parts[1] == "refinery":
		return beads.RefineryBeadIDWithPrefix(prefix, rig)
	case len(parts) == 3 && parts[1] == "crew":
		return beads.CrewBeadIDWithPrefix(prefix, rig, parts[2])
	case len(parts) == 3 && parts[1] == "polecats":
		return beads.PolecatBeadIDWithPrefix(prefix, rig, parts[2])
	case len(parts) == 3 && parts[0] == "deacon" && parts[1] == "dogs":
		// Dogs are town-level agents with hq- prefix
		return beads.DogBeadIDTown(parts[2])
	default:
		return ""
	}
}

// updateAgentHookBead updates the agent bead's state and hook when work is slung.
// This enables the witness to see that each agent is working.
//
// We run from the polecat's workDir (which redirects to the rig's beads database)
// WITHOUT setting BEADS_DIR, so the redirect mechanism works for gt-* agent beads.
//
// For rig-level beads (same database), we set the hook_bead slot directly.
// For cross-database scenarios (agent in rig db, hook bead in town db),
// the slot set may fail - this is handled gracefully with a warning.
// The work is still correctly attached via `bd update <bead> --assignee=<agent>`.
func updateAgentHookBead(agentID, beadID, workDir, townBeadsDir string) {
	_ = townBeadsDir // Not used - BEADS_DIR breaks redirect mechanism

	// Determine the directory to run bd commands from:
	// - If workDir is provided (polecat's clone path), use it for redirect-based routing
	// - Otherwise fall back to town root
	bdWorkDir := workDir
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		// Not in a Gas Town workspace - can't update agent bead
		fmt.Fprintf(os.Stderr, "Warning: couldn't find town root to update agent hook: %v\n", err)
		return
	}
	if bdWorkDir == "" {
		bdWorkDir = townRoot
	}

	// Convert agent ID to agent bead ID
	// Format examples (canonical: prefix-rig-role-name):
	//   greenplace/crew/max -> gt-greenplace-crew-max
	//   greenplace/polecats/Toast -> gt-greenplace-polecat-Toast
	//   mayor -> hq-mayor
	//   greenplace/witness -> gt-greenplace-witness
	agentBeadID := agentIDToBeadID(agentID, townRoot)
	if agentBeadID == "" {
		return
	}

	// Resolve the correct working directory for the agent bead.
	// Agent beads with rig-level prefixes (e.g., go-) live in rig databases,
	// not the town database. Use prefix-based resolution to find the correct path.
	// This fixes go-19z: bd slot commands failing for go-* prefixed beads.
	agentWorkDir := beads.ResolveHookDir(townRoot, agentBeadID, bdWorkDir)

	// Run from agentWorkDir WITHOUT BEADS_DIR to enable redirect-based routing.
	// Set hook_bead to the slung work (gt-zecmc: removed agent_state update).
	// Agent liveness is observable from tmux - no need to record it in bead.
	// For cross-database scenarios, slot set may fail gracefully (warning only).
	bd := beads.New(agentWorkDir)
	if err := bd.SetHookBead(agentBeadID, beadID); err != nil {
		// Log warning instead of silent ignore - helps debug cross-beads issues
		fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s hook: %v\n", agentBeadID, err)
		// Dogs created before canonical IDs need recreation: gt dog rm <name> && gt dog add <name>
		if strings.Contains(agentBeadID, "-dog-") {
			fmt.Fprintf(os.Stderr, "  (Old dog? Recreate with: gt dog rm <name> && gt dog add <name>)\n")
		}
		return
	}
}

// wakeRigAgents wakes the witness for a rig after polecat dispatch.
// This ensures the witness is ready to monitor. The refinery is nudged
// separately when an MR is actually created (by nudgeRefinery).
func wakeRigAgents(rigName string) {
	// Boot the rig (idempotent - no-op if already running)
	bootCmd := exec.Command("gt", "rig", "boot", rigName)
	_ = bootCmd.Run() // Ignore errors - rig might already be running

	// Nudge witness to clear any backoff
	t := tmux.NewTmux()
	witnessSession := fmt.Sprintf("gt-%s-witness", rigName)

	// Silent nudge - session might not exist yet
	_ = t.NudgeSession(witnessSession, "Polecat dispatched - check for work")
}

// nudgeRefinery wakes the refinery for a rig after an MR is created.
// This ensures the refinery picks up the new merge request promptly
// instead of waiting for its next poll cycle.
func nudgeRefinery(rigName, message string) {
	refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)

	// Test hook: log nudge for test observability (same pattern as GT_TEST_ATTACHED_MOLECULE_LOG)
	if logPath := os.Getenv("GT_TEST_NUDGE_LOG"); logPath != "" {
		entry := fmt.Sprintf("nudge:%s:%s\n", refinerySession, message)
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = f.WriteString(entry)
			_ = f.Close()
		}
		return // Don't actually nudge tmux in tests
	}

	t := tmux.NewTmux()
	_ = t.NudgeSession(refinerySession, message)
}

// isPolecatTarget checks if the target string refers to a polecat.
// Returns true if the target format is "rig/polecats/name".
// This is used to determine if we should respawn a dead polecat
// instead of failing when slinging work.
func isPolecatTarget(target string) bool {
	parts := strings.Split(target, "/")
	return len(parts) >= 3 && parts[1] == "polecats"
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
//   - extraVars: additional --var values supplied by the user
//
// Returns the wisp root ID which should be hooked.
func InstantiateFormulaOnBead(formulaName, beadID, title, hookWorkDir, townRoot string, skipCook bool, extraVars []string) (*FormulaOnBeadResult, error) {
	// Route bd mutations (wisp/bond) to the correct beads context for the target bead.
	formulaWorkDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)

	// Step 1: Cook the formula (ensures proto exists)
	if !skipCook {
		cookCmd := exec.Command("bd", "cook", formulaName)
		cookCmd.Dir = formulaWorkDir
		cookCmd.Stderr = os.Stderr
		if err := cookCmd.Run(); err != nil {
			return nil, fmt.Errorf("cooking formula %s: %w", formulaName, err)
		}
	}

	// Step 2: Create wisp with feature and issue variables from bead
	featureVar := fmt.Sprintf("feature=%s", title)
	issueVar := fmt.Sprintf("issue=%s", beadID)
	wispArgs := []string{"mol", "wisp", formulaName, "--var", featureVar, "--var", issueVar}
	for _, variable := range extraVars {
		wispArgs = append(wispArgs, "--var", variable)
	}
	wispArgs = append(wispArgs, "--json")
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
	bondArgs := []string{"mol", "bond", wispRootID, beadID, "--json"}
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
	cookCmd := exec.Command("bd", "cook", formulaName)
	cookCmd.Dir = workDir
	cookCmd.Stderr = os.Stderr
	return cookCmd.Run()
}
