package cmd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/cli"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
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
	Title        string          `json:"title"`
	Status       string          `json:"status"`
	Assignee     string          `json:"assignee"`
	Description  string          `json:"description"`
	Dependencies []beads.IssueDep `json:"dependencies,omitempty"`
}

// collectExistingMolecules returns all molecule wisp IDs attached to a bead.
// Checks both dependency bonds (ground truth from bd mol bond) and the
// description's attached_molecule field (metadata pointer). Wisp IDs are
// identified by containing "-wisp-" in their ID.
// Uses Dependencies (structured []IssueDep from bd show --json) rather than
// DependsOn (raw ID list, which is unreliable — see molecule_status.go comments).
func collectExistingMolecules(info *beadInfo) []string {
	seen := make(map[string]bool)
	var molecules []string

	// Check dependency bonds (ground truth - bd mol bond creates these)
	for _, dep := range info.Dependencies {
		if strings.Contains(dep.ID, "-wisp-") && !seen[dep.ID] {
			seen[dep.ID] = true
			molecules = append(molecules, dep.ID)
		}
	}

	// Also check description's attached_molecule (may differ from bonds)
	issue := &beads.Issue{Description: info.Description}
	fields := beads.ParseAttachmentFields(issue)
	if fields != nil && fields.AttachedMolecule != "" && !seen[fields.AttachedMolecule] {
		seen[fields.AttachedMolecule] = true
		molecules = append(molecules, fields.AttachedMolecule)
	}

	return molecules
}

// ensureNoExistingMolecules checks whether a bead already has attached molecules
// and either burns them (with --force) or returns an error. Returns nil when no
// molecules exist or they were successfully burned. Dry-run mode only prints.
func ensureNoExistingMolecules(info *beadInfo, beadID, townRoot string, force, dryRun bool) error {
	existingMolecules := collectExistingMolecules(info)
	if len(existingMolecules) == 0 {
		return nil
	}
	if dryRun {
		fmt.Printf("  Would burn %d stale molecule(s): %s\n",
			len(existingMolecules), strings.Join(existingMolecules, ", "))
		return nil
	}
	if !force {
		return fmt.Errorf("bead %s already has %d attached molecule(s): %s\nUse --force to replace, or --hook-raw-bead to skip formula",
			beadID, len(existingMolecules), strings.Join(existingMolecules, ", "))
	}
	fmt.Printf("  %s Burning %d stale molecule(s) from previous assignment: %s\n",
		style.Warning.Render("⚠"), len(existingMolecules), strings.Join(existingMolecules, ", "))
	if err := burnExistingMolecules(existingMolecules, beadID, townRoot); err != nil {
		return fmt.Errorf("burning stale molecules: %w", err)
	}
	return nil
}

// burnExistingMolecules detaches and burns all molecule wisps attached to a bead.
// First detaches the molecule from the base bead (clears attached_molecule in description),
// then force-closes the orphaned wisp beads. Returns an error if detach fails, since
// proceeding with a stale attached_molecule reference creates harder-to-debug orphans.
func burnExistingMolecules(molecules []string, beadID, townRoot string) error {
	if len(molecules) == 0 {
		return nil
	}
	burnDir := beads.ResolveHookDir(townRoot, beadID, "")

	// Step 1: Detach molecule from the base bead using the Go API (with audit logging
	// and advisory locking). This clears attached_molecule/attached_at from the description.
	// Without this, storeFieldsInBead preserves the stale reference because it only
	// overwrites when updates.AttachedMolecule is non-empty.
	bd := beads.New(burnDir)
	if _, err := bd.DetachMoleculeWithAudit(beadID, beads.DetachOptions{
		Operation: "burn",
		Reason:    "force re-sling: burning stale molecules",
	}); err != nil {
		return fmt.Errorf("detaching molecule from %s: %w", beadID, err)
	}

	// Step 2: Force-close the orphaned wisp beads so they don't linger.
	// Uses --force to handle wisps with open child steps (matching gt done pattern).
	if err := bd.ForceCloseWithReason("burned: force re-sling", molecules...); err != nil {
		fmt.Printf("  %s Could not close molecule wisp(s): %v\n",
			style.Dim.Render("Warning:"), err)
		// Close failure is non-fatal — the detach already succeeded, so the bead
		// is clean. Orphaned wisps will be caught by reactive DetectOrphanedMolecules.
	}

	return nil
}

// verifyBeadExists checks that the bead exists using bd show.
// Uses bd's native prefix-based routing via routes.jsonl - do NOT set BEADS_DIR
// as that overrides routing and breaks resolution of rig-level beads.
//
// Checks bead existence using bd show.
// Resolves the rig directory from the bead's prefix for correct dolt access.
func verifyBeadExists(beadID string) error {
	cmd := exec.Command("bd", "show", beadID, "--json", "--allow-stale")
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
	cmd := exec.Command("bd", "show", beadID, "--json", "--allow-stale")
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

// beadFieldUpdates holds all the fields that need to be stored in a bead's description.
// This enables a single read-modify-write cycle instead of sequential independent updates,
// eliminating the race condition where concurrent writers could overwrite each other's fields.
type beadFieldUpdates struct {
	Dispatcher       string // Agent that dispatched the work
	Args             string // Natural language instructions
	AttachedMolecule string // Wisp root ID
	NoMerge          bool   // Skip merge queue on completion
}

// storeFieldsInBead performs a single read-modify-write to update all attachment fields
// in a bead's description atomically. This replaces the sequential storeDispatcherInBead,
// storeArgsInBead, storeAttachedMoleculeInBead, and storeNoMergeInBead calls that each
// independently read-modify-write and could race under concurrent access.
func storeFieldsInBead(beadID string, updates beadFieldUpdates) error {
	logPath := os.Getenv("GT_TEST_ATTACHED_MOLECULE_LOG")

	issue := &beads.Issue{}
	if logPath == "" {
		// Read the bead once
		showCmd := exec.Command("bd", "show", beadID, "--json", "--allow-stale")
		showCmd.Dir = resolveBeadDir(beadID)
		out, err := showCmd.Output()
		if err != nil {
			return fmt.Errorf("fetching bead: %w", err)
		}
		if len(out) == 0 {
			return fmt.Errorf("bead not found")
		}

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

	// Apply all updates in one pass
	if updates.Dispatcher != "" {
		fields.DispatchedBy = updates.Dispatcher
	}
	if updates.Args != "" {
		fields.AttachedArgs = updates.Args
	}
	if updates.AttachedMolecule != "" {
		fields.AttachedMolecule = updates.AttachedMolecule
		if fields.AttachedAt == "" {
			fields.AttachedAt = time.Now().UTC().Format(time.RFC3339)
		}
	}
	if updates.NoMerge {
		fields.NoMerge = true
	}

	// Write back once
	newDesc := beads.SetAttachmentFields(issue, fields)
	if logPath != "" {
		_ = os.WriteFile(logPath, []byte(newDesc), 0644)
		return nil
	}

	updateCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	updateCmd.Dir = resolveBeadDir(beadID)
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
		prompt = fmt.Sprintf("Work slung: %s. Start working on it now - run `"+cli.Name()+" hook` to see the hook, then begin.", beadID)
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

	if t.IsAgentRunning(sessionName) {
		// Agent process is detected, but it may have just started (fresh spawn).
		// Check session age — if < 15s old, the agent likely isn't ready for input yet.
		if !isSessionYoung(sessionName, 15*time.Second) {
			return nil
		}
		// Fall through to apply startup delay for young sessions.
	} else {
		// Agent not running yet - wait for it to start (shell → program transition)
		if err := t.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
			return fmt.Errorf("waiting for agent to start: %w", err)
		}
	}

	// Accept bypass permissions warning if the agent emits one on startup
	agentName, _ := t.GetEnvironment(sessionName, "GT_AGENT")
	if shouldAcceptPermissionWarning(agentName) {
		_ = t.AcceptBypassPermissionsWarning(sessionName)
	}

	// Use prompt-detection polling instead of fixed sleep.
	// For known presets: uses ReadyPromptPrefix (e.g. "❯ " for Claude) polled every 200ms.
	// For unknown/custom agents: falls back to a 1s fixed delay (mirrors old behavior).
	// Note: uses preset-only resolution (not ResolveRoleAgentConfig) because
	// ensureAgentReady lacks rig/town context — only has the session name.
	effectiveName := agentName
	if effectiveName == "" {
		effectiveName = "claude" // Default sessions without GT_AGENT are Claude
	}
	var rc *config.RuntimeConfig
	if preset := config.GetAgentPreset(config.AgentPreset(effectiveName)); preset != nil {
		rc = config.RuntimeConfigFromPreset(config.AgentPreset(effectiveName))
	} else {
		// Unknown agent — use minimal config: no prompt detection, short fixed delay.
		rc = &config.RuntimeConfig{
			Tmux: &config.RuntimeTmuxConfig{
				ReadyDelayMs: 1000,
			},
		}
	}
	// Ensure a minimum 1s readiness delay for presets without prompt detection.
	// Without this, agents with ReadyPromptPrefix="" and ReadyDelayMs=0
	// (e.g. gemini, cursor) would skip the readiness guard entirely,
	// reintroducing early-input races that this function exists to prevent.
	if rc.Tmux != nil && rc.Tmux.ReadyPromptPrefix == "" && rc.Tmux.ReadyDelayMs < 1000 {
		rc.Tmux.ReadyDelayMs = 1000
	}
	if err := t.WaitForRuntimeReady(sessionName, rc, constants.ClaudeStartTimeout); err != nil {
		// Graceful degradation: warn but proceed (matches original behavior of always continuing)
		fmt.Fprintf(os.Stderr, "Warning: agent readiness detection timed out for %s: %v\n", sessionName, err)
	}

	return nil
}

// isSessionYoung returns true if the tmux session was created less than maxAge ago.
func isSessionYoung(sessionName string, maxAge time.Duration) bool {
	out, err := exec.Command("tmux", "display-message", "-t", sessionName, "-p", "#{session_created}").Output()
	if err != nil {
		return false
	}
	createdUnix, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(createdUnix, 0)) < maxAge
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

	// Queue nudge to witness for cooperative delivery.
	// This avoids interrupting in-flight tool calls.
	witnessSession := session.WitnessSessionName(session.PrefixFor(rigName))
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" {
		if err := nudge.Enqueue(townRoot, witnessSession, nudge.QueuedNudge{
			Sender:  "sling",
			Message: "Polecat dispatched - check for work",
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to queue nudge for %s: %v\n", witnessSession, err)
		}
	} else {
		// Fallback to direct nudge if town root unavailable
		t := tmux.NewTmux()
		_ = t.NudgeSession(witnessSession, "Polecat dispatched - check for work")
	}
}

// nudgeRefinery queues a nudge for the refinery after an MR is created.
// Uses the nudge queue for cooperative delivery so we don't interrupt
// in-flight tool calls. The refinery picks this up at its next turn boundary.
func nudgeRefinery(rigName, message string) {
	refinerySession := session.RefinerySessionName(session.PrefixFor(rigName))

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

	// Queue for cooperative delivery at refinery's next turn boundary
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" {
		if err := nudge.Enqueue(townRoot, refinerySession, nudge.QueuedNudge{
			Sender:  "sling",
			Message: message,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to queue nudge for %s: %v\n", refinerySession, err)
		}
	} else {
		// Fallback to direct nudge if town root unavailable
		t := tmux.NewTmux()
		_ = t.NudgeSession(refinerySession, message)
	}
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
		cookCmd.Env = append(os.Environ(), "GT_ROOT="+townRoot)
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
// townRoot is required for GT_ROOT so bd can find town-level formulas.
func CookFormula(formulaName, workDir, townRoot string) error {
	cookCmd := exec.Command("bd", "cook", formulaName)
	cookCmd.Dir = workDir
	cookCmd.Env = append(os.Environ(), "GT_ROOT="+townRoot)
	cookCmd.Stderr = os.Stderr
	return cookCmd.Run()
}

// isHookedAgentDeadFn is a seam for tests. Production uses isHookedAgentDead.
var isHookedAgentDeadFn = isHookedAgentDead

// isHookedAgentDead checks if the tmux session for a hooked assignee is dead.
// Used by sling to auto-force re-sling when the previous agent has no active session (gt-pqf9x).
// Returns true if the session is confirmed dead. Returns false if alive or if we
// can't determine liveness (conservative: don't auto-force on uncertainty).
func isHookedAgentDead(assignee string) bool {
	sessionName, _ := assigneeToSessionName(assignee)
	if sessionName == "" {
		return false // Unknown format, can't determine
	}
	t := tmux.NewTmux()
	alive, err := t.HasSession(sessionName)
	if err != nil {
		return false // tmux not available or error, be conservative
	}
	return !alive
}

// hookBeadWithRetry hooks a bead to a target agent with exponential backoff retry
// and post-hook verification. This ensures the hook sticks even under Dolt concurrency.
// Fails fast on configuration/initialization errors (gt-2ra).
// See: https://github.com/steveyegge/gastown/issues/148
func hookBeadWithRetry(beadID, targetAgent, hookDir string) error {
	const maxRetries = 10
	const baseBackoff = 500 * time.Millisecond
	const maxBackoff = 30 * time.Second
	skipVerify := os.Getenv("GT_TEST_SKIP_HOOK_VERIFY") != ""

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		hookCmd := exec.Command("bd", "update", beadID, "--status=hooked", "--assignee="+targetAgent)
		hookCmd.Dir = hookDir
		hookCmd.Stderr = os.Stderr
		if err := hookCmd.Run(); err != nil {
			lastErr = err
			// Fail fast on config/init errors — retrying won't help (gt-2ra)
			if isSlingConfigError(err) {
				return fmt.Errorf("hooking bead failed (DB not initialized — not retrying): %w", err)
			}
			if attempt < maxRetries {
				backoff := slingBackoff(attempt, baseBackoff, maxBackoff)
				fmt.Printf("%s Hook attempt %d failed, retrying in %v...\n", style.Warning.Render("⚠"), attempt, backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("hooking bead after %d attempts: %w", maxRetries, err)
		}

		if skipVerify {
			break
		}

		verifyInfo, verifyErr := getBeadInfo(beadID)
		if verifyErr != nil {
			lastErr = fmt.Errorf("verifying hook: %w", verifyErr)
			if attempt < maxRetries {
				backoff := slingBackoff(attempt, baseBackoff, maxBackoff)
				fmt.Printf("%s Hook verification failed, retrying in %v...\n", style.Warning.Render("⚠"), backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("verifying hook after %d attempts: %w", maxRetries, lastErr)
		}

		if verifyInfo.Status != "hooked" || verifyInfo.Assignee != targetAgent {
			lastErr = fmt.Errorf("hook did not stick: status=%s, assignee=%s (expected hooked, %s)",
				verifyInfo.Status, verifyInfo.Assignee, targetAgent)
			if attempt < maxRetries {
				backoff := slingBackoff(attempt, baseBackoff, maxBackoff)
				fmt.Printf("%s %v, retrying in %v...\n", style.Warning.Render("⚠"), lastErr, backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("hook failed after %d attempts: %w", maxRetries, lastErr)
		}

		break
	}

	return nil
}

// slingBackoff calculates exponential backoff with ±25% jitter for a given attempt (1-indexed).
// Formula: base * 2^(attempt-1) * (1 ± 25% random), capped at max.
func slingBackoff(attempt int, base, max time.Duration) time.Duration { //nolint:unparam // base is parameterized for testability
	backoff := base
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff > max {
			backoff = max
			break
		}
	}
	// Apply ±25% jitter
	jitter := 1.0 + (rand.Float64()-0.5)*0.5 // range [0.75, 1.25]
	result := time.Duration(float64(backoff) * jitter)
	if result > max {
		result = max
	}
	return result
}

// isSlingConfigError returns true if the error indicates a configuration or
// initialization problem rather than a transient failure. Config errors should
// NOT be retried because they will fail identically on every attempt (gt-2ra).
func isSlingConfigError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not initialized") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "table not found") ||
		strings.Contains(msg, "issue_prefix") ||
		strings.Contains(msg, "no database") ||
		strings.Contains(msg, "database not found") ||
		strings.Contains(msg, "connection refused")
}

// loadRigCommandVars reads rig settings and returns --var key=value strings
// for all configured build pipeline commands (setup, typecheck, lint, test, build).
// Only non-empty commands are included; empty means "skip" in the formula.
func loadRigCommandVars(townRoot, rig string) []string {
	if townRoot == "" || rig == "" {
		return nil
	}
	settingsPath := filepath.Join(townRoot, rig, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil || settings == nil || settings.MergeQueue == nil {
		return nil
	}
	mq := settings.MergeQueue
	var vars []string
	if mq.SetupCommand != "" {
		vars = append(vars, fmt.Sprintf("setup_command=%s", mq.SetupCommand))
	}
	if mq.TypecheckCommand != "" {
		vars = append(vars, fmt.Sprintf("typecheck_command=%s", mq.TypecheckCommand))
	}
	if mq.LintCommand != "" {
		vars = append(vars, fmt.Sprintf("lint_command=%s", mq.LintCommand))
	}
	if mq.TestCommand != "" {
		vars = append(vars, fmt.Sprintf("test_command=%s", mq.TestCommand))
	}
	if mq.BuildCommand != "" {
		vars = append(vars, fmt.Sprintf("build_command=%s", mq.BuildCommand))
	}
	return vars
}

// shouldAcceptPermissionWarning checks if the agent emits a bypass-permissions
// warning on startup that needs to be acknowledged via tmux.
func shouldAcceptPermissionWarning(agentName string) bool {
	if agentName == "" {
		agentName = "claude" // Default sessions without GT_AGENT are Claude
	}
	preset := config.GetAgentPresetByName(agentName)
	if preset == nil {
		return false
	}
	return preset.EmitsPermissionWarning
}
