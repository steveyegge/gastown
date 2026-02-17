package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/lock"
	"github.com/steveyegge/gastown/internal/state"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var primeHookMode bool
var primeDryRun bool
var primeState bool
var primeStateJSON bool
var primeExplain bool

// primeHookSource stores the SessionStart source ("startup", "resume", "clear", "compact")
// when running in hook mode. Used to provide lighter output on compaction/resume.
var primeHookSource string

// Role represents a detected agent role.
type Role string

const (
	RoleMayor    Role = "mayor"
	RoleDeacon   Role = "deacon"
	RoleBoot     Role = "boot"
	RoleWitness  Role = "witness"
	RoleRefinery Role = "refinery"
	RolePolecat  Role = "polecat"
	RoleCrew     Role = "crew"
	RoleUnknown  Role = "unknown"
)

var primeCmd = &cobra.Command{
	Use:     "prime",
	GroupID: GroupDiag,
	Short:   "Output role context for current directory",
	Long: `Detect the agent role from the current directory and output context.

Role detection:
  - Town root → Neutral (no role inferred; use GT_ROLE)
  - mayor/ or <rig>/mayor/ → Mayor context
  - <rig>/witness/rig/ → Witness context
  - <rig>/refinery/rig/ → Refinery context
  - <rig>/polecats/<name>/ → Polecat context

This command is typically used in shell prompts or agent initialization.

HOOK MODE (--hook):
  When called as an LLM runtime hook, use --hook to enable session ID handling.
  This reads session metadata from stdin and persists it for the session.

  Claude Code integration (in .claude/settings.json):
    "SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hook"}]}]

  Claude Code sends JSON on stdin:
    {"session_id": "uuid", "transcript_path": "/path", "source": "startup|resume"}

  Other agents can set GT_SESSION_ID environment variable instead.`,
	RunE: runPrime,
}

func init() {
	primeCmd.Flags().BoolVar(&primeHookMode, "hook", false,
		"Hook mode: read session ID from stdin JSON (for LLM runtime hooks)")
	primeCmd.Flags().BoolVar(&primeDryRun, "dry-run", false,
		"Show what would be injected without side effects (no marker removal, no bd prime, no mail)")
	primeCmd.Flags().BoolVar(&primeState, "state", false,
		"Show detected session state only (normal/post-handoff/crash/autonomous)")
	primeCmd.Flags().BoolVar(&primeStateJSON, "json", false,
		"Output state as JSON (requires --state)")
	primeCmd.Flags().BoolVar(&primeExplain, "explain", false,
		"Show why each section was included")
	rootCmd.AddCommand(primeCmd)
}

// RoleContext is an alias for RoleInfo for backward compatibility.
// New code should use RoleInfo directly.
type RoleContext = RoleInfo

func runPrime(cmd *cobra.Command, args []string) error {
	if err := validatePrimeFlags(); err != nil {
		return err
	}

	cwd, townRoot, err := resolvePrimeWorkspace()
	if err != nil {
		return err
	}
	if townRoot == "" {
		return nil // Silent exit - not in workspace and not enabled
	}

	if primeHookMode {
		handlePrimeHookMode(townRoot, cwd)
	}

	// Check for handoff marker (prevents handoff loop bug)
	if primeDryRun {
		checkHandoffMarkerDryRun(cwd)
	} else {
		checkHandoffMarker(cwd)
	}

	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		return fmt.Errorf("detecting role: %w", err)
	}

	warnRoleMismatch(roleInfo, cwd)

	ctx := RoleContext{
		Role:     roleInfo.Role,
		Rig:      roleInfo.Rig,
		Polecat:  roleInfo.Polecat,
		TownRoot: townRoot,
		WorkDir:  cwd,
	}

	// --state mode: output state only and exit
	if primeState {
		outputState(ctx, primeStateJSON)
		return nil
	}

	if err := setupPrimeSession(ctx, roleInfo); err != nil {
		return err
	}

	// Compact/resume: lighter prime that skips verbose role context.
	// The agent already has role docs in compressed memory — just restore
	// identity, hook status, and any new mail.
	if isCompactResume() {
		return runPrimeCompactResume(ctx, cwd)
	}

	if err := outputRoleContext(ctx); err != nil {
		return err
	}

	hasSlungWork := checkSlungWork(ctx)
	explain(hasSlungWork, "Autonomous mode: hooked/in-progress work detected")

	outputMoleculeContext(ctx)
	outputCheckpointContext(ctx)
	runPrimeExternalTools(cwd)

	if ctx.Role == RoleMayor {
		checkPendingEscalations(ctx)
	}

	if !hasSlungWork {
		explain(true, "Startup directive: normal mode (no hooked work)")
		outputStartupDirective(ctx)
	}

	return nil
}

// runPrimeCompactResume runs a lighter prime after compaction or resume.
// The agent already has full role context in compressed memory. This just
// restores identity, checks hook/work status, and injects any new mail.
func runPrimeCompactResume(ctx RoleContext, cwd string) error {
	// Brief identity confirmation
	actor := getAgentIdentity(ctx)
	fmt.Printf("\n> **Recovery**: Context %s complete. You are **%s** (%s).\n",
		primeHookSource, actor, ctx.Role)

	// Session metadata for seance
	outputSessionMetadata(ctx)

	// Check for hooked work — critical for resuming after compaction
	hasSlungWork := checkSlungWork(ctx)

	// Molecule progress if available
	outputMoleculeContext(ctx)

	// Inject any mail that arrived during compaction
	if !primeDryRun {
		runMailCheckInject(cwd)
	}

	// Startup directive if no hooked work
	if !hasSlungWork {
		outputStartupDirective(ctx)
	}

	return nil
}

// validatePrimeFlags checks that CLI flag combinations are valid.
func validatePrimeFlags() error {
	if primeState && (primeHookMode || primeDryRun || primeExplain) {
		return fmt.Errorf("--state cannot be combined with other flags (except --json)")
	}
	if primeStateJSON && !primeState {
		return fmt.Errorf("--json requires --state")
	}
	return nil
}

// resolvePrimeWorkspace finds the cwd and town root for prime.
// Returns empty townRoot (not an error) when not in a workspace and not enabled.
func resolvePrimeWorkspace() (cwd, townRoot string, err error) {
	cwd, err = os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err = workspace.FindFromCwd()
	if err != nil {
		return "", "", fmt.Errorf("finding workspace: %w", err)
	}

	// "Discover, Don't Track" principle:
	// If in a workspace, proceed. If not, check global enabled state.
	if townRoot == "" {
		if !state.IsEnabled() {
			return cwd, "", nil // Signal caller to exit silently
		}
		return "", "", fmt.Errorf("not in a Gas Town workspace")
	}

	return cwd, townRoot, nil
}

// handlePrimeHookMode reads session ID from stdin and persists it.
// Called when --hook flag is set for LLM runtime hook integration.
func handlePrimeHookMode(townRoot, cwd string) {
	sessionID, source := readHookSessionID()
	if !primeDryRun {
		persistSessionID(townRoot, sessionID)
		if cwd != townRoot {
			persistSessionID(cwd, sessionID)
		}
	}
	_ = os.Setenv("GT_SESSION_ID", sessionID)
	_ = os.Setenv("CLAUDE_SESSION_ID", sessionID) // Legacy compatibility

	// Store source for compact/resume detection in runPrime
	primeHookSource = source

	explain(true, "Session beacon: hook mode enabled, session ID from stdin")
	fmt.Printf("[session:%s]\n", sessionID)
	if source != "" {
		fmt.Printf("[source:%s]\n", source)
	}
}

// isCompactResume returns true if the current prime is running after compaction or resume.
// In these cases, the agent already has role context in compressed memory and only needs
// a brief identity confirmation plus hook/work status.
func isCompactResume() bool {
	return primeHookSource == "compact" || primeHookSource == "resume"
}

// warnRoleMismatch outputs a prominent warning if GT_ROLE disagrees with cwd detection.
func warnRoleMismatch(roleInfo RoleInfo, cwd string) {
	if !roleInfo.Mismatch {
		return
	}
	fmt.Printf("\n%s\n", style.Bold.Render("⚠️  ROLE/LOCATION MISMATCH"))
	fmt.Printf("You are %s (from $GT_ROLE) but your cwd suggests %s.\n",
		style.Bold.Render(string(roleInfo.Role)),
		style.Bold.Render(string(roleInfo.CwdRole)))
	fmt.Printf("Expected home: %s\n", roleInfo.Home)
	fmt.Printf("Actual cwd:    %s\n", cwd)
	fmt.Println()
	fmt.Println("This can cause commands to misbehave. Either:")
	fmt.Println("  1. cd to your home directory, OR")
	fmt.Println("  2. Use absolute paths for gt/bd commands")
	fmt.Println()
}

// setupPrimeSession handles identity locking, beads redirect, and session events.
// Skipped entirely in dry-run mode.
func setupPrimeSession(ctx RoleContext, roleInfo RoleInfo) error {
	if primeDryRun {
		return nil
	}
	if err := acquireIdentityLock(ctx); err != nil {
		return err
	}
	if !roleInfo.Mismatch {
		ensureBeadsRedirect(ctx)
	}
	emitSessionEvent(ctx)
	return nil
}

// outputRoleContext emits session metadata and all role/context output sections.
func outputRoleContext(ctx RoleContext) error {
	explain(true, "Session metadata: always included for seance discovery")
	outputSessionMetadata(ctx)

	explain(true, fmt.Sprintf("Role context: detected role is %s", ctx.Role))
	if err := outputPrimeContext(ctx); err != nil {
		return err
	}

	outputContextFile(ctx)
	outputHandoffContent(ctx)
	outputAttachmentStatus(ctx)
	return nil
}

// runPrimeExternalTools runs bd prime and gt mail check --inject.
// Skipped in dry-run mode with explain output.
func runPrimeExternalTools(cwd string) {
	if primeDryRun {
		explain(true, "bd prime: skipped in dry-run mode")
		explain(true, "gt mail check --inject: skipped in dry-run mode")
		return
	}
	runBdPrime(cwd)
	runMailCheckInject(cwd)
}

// runBdPrime runs `bd prime` and outputs the result.
// This provides beads workflow context to the agent.
func runBdPrime(workDir string) {
	cmd := exec.Command("bd", "prime")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Skip if bd prime fails (beads might not be available)
		// But log stderr if present for debugging
		if errMsg := strings.TrimSpace(stderr.String()); errMsg != "" {
			fmt.Fprintf(os.Stderr, "bd prime: %s\n", errMsg)
		}
		return
	}

	output := strings.TrimSpace(stdout.String())
	if output != "" {
		fmt.Println()
		fmt.Println(output)
	}
}

// runMailCheckInject runs `gt mail check --inject` and outputs the result.
// This injects any pending mail into the agent's context.
func runMailCheckInject(workDir string) {
	cmd := exec.Command("gt", "mail", "check", "--inject")
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Skip if mail check fails, but log stderr for debugging
		if errMsg := strings.TrimSpace(stderr.String()); errMsg != "" {
			fmt.Fprintf(os.Stderr, "gt mail check: %s\n", errMsg)
		}
		return
	}

	output := strings.TrimSpace(stdout.String())
	if output != "" {
		fmt.Println()
		fmt.Println(output)
	}
}

// checkSlungWork checks for hooked work on the agent's hook.
// If found, displays AUTONOMOUS WORK MODE and tells the agent to execute immediately.
// Returns true if hooked work was found (caller should skip normal startup directive).
func checkSlungWork(ctx RoleContext) bool {
	hookedBead := findAgentWork(ctx)
	if hookedBead == nil {
		return false
	}

	attachment := beads.ParseAttachmentFields(hookedBead)
	hasMolecule := attachment != nil && attachment.AttachedMolecule != ""

	outputAutonomousDirective(ctx, hookedBead, hasMolecule)
	outputHookedBeadDetails(hookedBead)

	if hasMolecule {
		outputMoleculeWorkflow(ctx, attachment)
	} else {
		outputBeadPreview(hookedBead)
	}

	return true
}

// findAgentWork looks up hooked or in-progress beads assigned to this agent.
// Primary: reads hook_bead from the agent bead (same strategy as detectSessionState/gt hook).
// Fallback: queries by assignee for agents without an agent bead.
// Returns nil if no work is found.
func findAgentWork(ctx RoleContext) *beads.Issue {
	agentID := getAgentIdentity(ctx)
	if agentID == "" {
		return nil
	}

	b := beads.New(ctx.WorkDir)

	// Primary: agent bead's hook_bead field (authoritative, set by bd slot set during sling)
	agentBeadID := buildAgentBeadID(agentID, ctx.Role, ctx.TownRoot)
	if agentBeadID != "" {
		agentBeadDir := beads.ResolveHookDir(ctx.TownRoot, agentBeadID, ctx.WorkDir)
		ab := beads.New(agentBeadDir)
		if agentBead, err := ab.Show(agentBeadID); err == nil && agentBead != nil && agentBead.HookBead != "" {
			hookBeadDir := beads.ResolveHookDir(ctx.TownRoot, agentBead.HookBead, ctx.WorkDir)
			hb := beads.New(hookBeadDir)
			if hookBead, err := hb.Show(agentBead.HookBead); err == nil && hookBead != nil &&
				(hookBead.Status == beads.StatusHooked || hookBead.Status == "in_progress") {
				return hookBead
			}
		}
	}

	// Fallback: query by assignee
	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		return nil
	}

	// Fall back to in_progress beads (session interrupted before completion)
	if len(hookedBeads) == 0 {
		inProgressBeads, err := b.List(beads.ListOptions{
			Status:   "in_progress",
			Assignee: agentID,
			Priority: -1,
		})
		if err != nil || len(inProgressBeads) == 0 {
			return nil
		}
		hookedBeads = inProgressBeads
	}

	return hookedBeads[0]
}

// outputAutonomousDirective displays the AUTONOMOUS WORK MODE header and instructions.
func outputAutonomousDirective(ctx RoleContext, hookedBead *beads.Issue, hasMolecule bool) {
	roleAnnounce := buildRoleAnnouncement(ctx)

	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render("## 🚨 AUTONOMOUS WORK MODE 🚨"))
	fmt.Println("Work is on your hook. After announcing your role, begin IMMEDIATELY.")
	fmt.Println()
	fmt.Println("This is physics, not politeness. Gas Town is a steam engine - you are a piston.")
	fmt.Println("Every moment you wait is a moment the engine stalls. Other agents may be")
	fmt.Println("blocked waiting on YOUR output. The hook IS your assignment. RUN IT.")
	fmt.Println()
	fmt.Println("Remember: Every completion is recorded in the capability ledger. Your work")
	fmt.Println("history is visible, and quality matters. Execute with care - you're building")
	fmt.Println("a track record that proves autonomous execution works at scale.")
	fmt.Println()
	fmt.Println("1. Announce: \"" + roleAnnounce + "\" (ONE line, no elaboration)")

	if hasMolecule {
		fmt.Println("2. This bead has an ATTACHED MOLECULE (formula workflow)")
		fmt.Println("3. Work through molecule steps in order - see CURRENT STEP below")
		fmt.Println("4. Close each step with `bd close <step-id>`, then check `bd mol current` for next step")
	} else {
		fmt.Printf("2. Then IMMEDIATELY run: `bd show %s`\n", hookedBead.ID)
		fmt.Println("3. Begin execution - no waiting for user input")
	}
	fmt.Println()
	fmt.Println("**DO NOT:**")
	fmt.Println("- Wait for user response after announcing")
	fmt.Println("- Ask clarifying questions")
	fmt.Println("- Describe what you're going to do")
	fmt.Println("- Check mail first (hook takes priority)")
	if hasMolecule {
		fmt.Println("- Skip molecule steps or work on the base bead directly")
	}
	fmt.Println()
}

// outputHookedBeadDetails displays the hooked bead's ID, title, and description summary.
func outputHookedBeadDetails(hookedBead *beads.Issue) {
	fmt.Printf("%s\n\n", style.Bold.Render("## Hooked Work"))
	fmt.Printf("  Bead ID: %s\n", style.Bold.Render(hookedBead.ID))
	fmt.Printf("  Title: %s\n", hookedBead.Title)
	if hookedBead.Description != "" {
		lines := strings.Split(hookedBead.Description, "\n")
		maxLines := 5
		if len(lines) > maxLines {
			lines = lines[:maxLines]
			lines = append(lines, "...")
		}
		fmt.Println("  Description:")
		for _, line := range lines {
			fmt.Printf("    %s\n", line)
		}
	}
	fmt.Println()
}

// outputMoleculeWorkflow displays attached molecule context with current step.
func outputMoleculeWorkflow(ctx RoleContext, attachment *beads.AttachmentFields) {
	fmt.Printf("%s\n\n", style.Bold.Render("## 🧬 ATTACHED MOLECULE (FORMULA WORKFLOW)"))
	fmt.Printf("Molecule ID: %s\n", attachment.AttachedMolecule)
	if attachment.AttachedArgs != "" {
		fmt.Printf("\n%s\n", style.Bold.Render("📋 ARGS (use these to guide execution):"))
		fmt.Printf("  %s\n", attachment.AttachedArgs)
	}
	fmt.Println()

	showMoleculeExecutionPrompt(ctx.WorkDir, attachment.AttachedMolecule)

	fmt.Println()
	fmt.Printf("%s\n", style.Bold.Render("⚠️  IMPORTANT: Follow the molecule steps above, NOT the base bead."))
	fmt.Println("The base bead is just a container. The molecule steps define your workflow.")
}

// outputBeadPreview runs `bd show` and displays a truncated preview of the bead.
func outputBeadPreview(hookedBead *beads.Issue) {
	fmt.Println("**Bead details:**")
	cmd := exec.Command("bd", "show", hookedBead.ID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errMsg := strings.TrimSpace(stderr.String()); errMsg != "" {
			fmt.Fprintf(os.Stderr, "  bd show %s: %s\n", hookedBead.ID, errMsg)
		} else {
			fmt.Fprintf(os.Stderr, "  bd show %s: %v\n", hookedBead.ID, err)
		}
	} else {
		lines := strings.Split(stdout.String(), "\n")
		maxLines := 15
		if len(lines) > maxLines {
			lines = lines[:maxLines]
			lines = append(lines, "...")
		}
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Println()
}

// buildRoleAnnouncement creates the role announcement string for autonomous mode.
func buildRoleAnnouncement(ctx RoleContext) string {
	switch ctx.Role {
	case RoleMayor:
		return "Mayor, checking in."
	case RoleDeacon:
		return "Deacon, checking in."
	case RoleBoot:
		return "Boot, checking in."
	case RoleWitness:
		return fmt.Sprintf("%s Witness, checking in.", ctx.Rig)
	case RoleRefinery:
		return fmt.Sprintf("%s Refinery, checking in.", ctx.Rig)
	case RolePolecat:
		return fmt.Sprintf("%s Polecat %s, checking in.", ctx.Rig, ctx.Polecat)
	case RoleCrew:
		return fmt.Sprintf("%s Crew %s, checking in.", ctx.Rig, ctx.Polecat)
	default:
		return "Agent, checking in."
	}
}

// getGitRoot returns the root of the current git repository.
func getGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// getAgentIdentity returns the agent identity string for hook lookup.
func getAgentIdentity(ctx RoleContext) string {
	switch ctx.Role {
	case RoleCrew:
		return fmt.Sprintf("%s/crew/%s", ctx.Rig, ctx.Polecat)
	case RolePolecat:
		return fmt.Sprintf("%s/polecats/%s", ctx.Rig, ctx.Polecat)
	case RoleMayor:
		return "mayor"
	case RoleDeacon:
		return "deacon"
	case RoleBoot:
		return "boot"
	case RoleWitness:
		return fmt.Sprintf("%s/witness", ctx.Rig)
	case RoleRefinery:
		return fmt.Sprintf("%s/refinery", ctx.Rig)
	default:
		return ""
	}
}

// acquireIdentityLock checks and acquires the identity lock for worker roles.
// This prevents multiple agents from claiming the same worker identity.
// Returns an error if another agent already owns this identity.
func acquireIdentityLock(ctx RoleContext) error {
	// Only lock worker roles (polecat, crew)
	// Infrastructure roles (mayor, witness, refinery, deacon) are singletons
	// managed by tmux session names, so they don't need file-based locks
	if ctx.Role != RolePolecat && ctx.Role != RoleCrew {
		return nil
	}

	// Create lock for this worker directory
	l := lock.New(ctx.WorkDir)

	// Determine session ID from environment or context
	sessionID := os.Getenv("TMUX_PANE")
	if sessionID == "" {
		// Fall back to a descriptive identifier
		sessionID = fmt.Sprintf("%s/%s", ctx.Rig, ctx.Polecat)
	}

	// Try to acquire the lock
	if err := l.Acquire(sessionID); err != nil {
		if errors.Is(err, lock.ErrLocked) {
			// Another agent owns this identity
			fmt.Printf("\n%s\n\n", style.Bold.Render("⚠️  IDENTITY COLLISION DETECTED"))
			fmt.Printf("Another agent already claims this worker identity.\n\n")

			// Show lock details
			if info, readErr := l.Read(); readErr == nil {
				fmt.Printf("Lock holder:\n")
				fmt.Printf("  PID: %d\n", info.PID)
				fmt.Printf("  Session: %s\n", info.SessionID)
				fmt.Printf("  Acquired: %s\n", info.AcquiredAt.Format("2006-01-02 15:04:05"))
				fmt.Println()
			}

			fmt.Printf("To resolve:\n")
			fmt.Printf("  1. Find the other session and close it, OR\n")
			fmt.Printf("  2. Run: gt doctor --fix (cleans stale locks)\n")
			fmt.Printf("  3. If lock is stale: rm %s/.runtime/agent.lock\n", ctx.WorkDir)
			fmt.Println()

			return fmt.Errorf("cannot claim identity %s/%s: %w", ctx.Rig, ctx.Polecat, err)
		}
		return fmt.Errorf("acquiring identity lock: %w", err)
	}

	return nil
}

// getAgentBeadID returns the agent bead ID for the current role.
// Town-level agents (mayor, deacon) use hq- prefix; rig-scoped agents use the rig's prefix.
// Returns empty string for unknown roles.
func getAgentBeadID(ctx RoleContext) string {
	switch ctx.Role {
	case RoleMayor:
		return beads.MayorBeadIDTown()
	case RoleDeacon:
		return beads.DeaconBeadIDTown()
	case RoleBoot:
		// Boot uses deacon's bead since it's a deacon subprocess
		return beads.DeaconBeadIDTown()
	case RoleWitness:
		if ctx.Rig != "" {
			prefix := beads.GetPrefixForRig(ctx.TownRoot, ctx.Rig)
			return beads.WitnessBeadIDWithPrefix(prefix, ctx.Rig)
		}
		return ""
	case RoleRefinery:
		if ctx.Rig != "" {
			prefix := beads.GetPrefixForRig(ctx.TownRoot, ctx.Rig)
			return beads.RefineryBeadIDWithPrefix(prefix, ctx.Rig)
		}
		return ""
	case RolePolecat:
		if ctx.Rig != "" && ctx.Polecat != "" {
			prefix := beads.GetPrefixForRig(ctx.TownRoot, ctx.Rig)
			return beads.PolecatBeadIDWithPrefix(prefix, ctx.Rig, ctx.Polecat)
		}
		return ""
	case RoleCrew:
		if ctx.Rig != "" && ctx.Polecat != "" {
			prefix := beads.GetPrefixForRig(ctx.TownRoot, ctx.Rig)
			return beads.CrewBeadIDWithPrefix(prefix, ctx.Rig, ctx.Polecat)
		}
		return ""
	default:
		return ""
	}
}

// ensureBeadsRedirect ensures the .beads/redirect file exists for worktree-based roles.
// This handles cases where git clean or other operations delete the redirect file.
// Uses the shared SetupRedirect helper which handles both tracked and local beads.
func ensureBeadsRedirect(ctx RoleContext) {
	// Only applies to worktree-based roles that use shared beads
	if ctx.Role != RoleCrew && ctx.Role != RolePolecat && ctx.Role != RoleRefinery {
		return
	}

	// Check if redirect already exists
	redirectPath := filepath.Join(ctx.WorkDir, ".beads", "redirect")
	if _, err := os.Stat(redirectPath); err == nil {
		return // Redirect exists, nothing to do
	}

	// Use shared helper - silently ignore errors during prime
	_ = beads.SetupRedirect(ctx.TownRoot, ctx.WorkDir)
}

// checkPendingEscalations queries for open escalation beads and displays them prominently.
// This is called on Mayor startup to surface issues needing human attention.
func checkPendingEscalations(ctx RoleContext) {
	// Query for open escalations using bd list with tag filter
	cmd := exec.Command("bd", "list", "--status=open", "--tag=escalation", "--json")
	cmd.Dir = ctx.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Silently skip - escalation check is best-effort
		return
	}

	// Parse JSON output
	var escalations []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Priority    int    `json:"priority"`
		Description string `json:"description"`
		Created     string `json:"created"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &escalations); err != nil || len(escalations) == 0 {
		// No escalations or parse error
		return
	}

	// Count by severity
	critical := 0
	high := 0
	medium := 0
	for _, e := range escalations {
		switch e.Priority {
		case 0:
			critical++
		case 1:
			high++
		default:
			medium++
		}
	}

	// Display prominently
	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render("## 🚨 PENDING ESCALATIONS"))
	fmt.Printf("There are %d escalation(s) awaiting human attention:\n\n", len(escalations))

	if critical > 0 {
		fmt.Printf("  🔴 CRITICAL: %d\n", critical)
	}
	if high > 0 {
		fmt.Printf("  🟠 HIGH: %d\n", high)
	}
	if medium > 0 {
		fmt.Printf("  🟡 MEDIUM: %d\n", medium)
	}
	fmt.Println()

	// Show first few escalations
	maxShow := 5
	if len(escalations) < maxShow {
		maxShow = len(escalations)
	}
	for i := 0; i < maxShow; i++ {
		e := escalations[i]
		severity := "MEDIUM"
		switch e.Priority {
		case 0:
			severity = "CRITICAL"
		case 1:
			severity = "HIGH"
		}
		fmt.Printf("  • [%s] %s (%s)\n", severity, e.Title, e.ID)
	}
	if len(escalations) > maxShow {
		fmt.Printf("  ... and %d more\n", len(escalations)-maxShow)
	}
	fmt.Println()

	fmt.Println("**Action required:** Review escalations with `bd list --tag=escalation`")
	fmt.Println("Close resolved ones with `bd close <id> --reason \"resolution\"`")
	fmt.Println()
}
