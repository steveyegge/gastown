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
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
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
  - Town root, mayor/, or <rig>/mayor/ → Mayor context
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
	// Validate flag combinations: --state is exclusive (except --json)
	if primeState && (primeHookMode || primeDryRun || primeExplain) {
		return fmt.Errorf("--state cannot be combined with other flags (except --json)")
	}
	// --json requires --state
	if primeStateJSON && !primeState {
		return fmt.Errorf("--json requires --state")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}

	// "Discover, Don't Track" principle:
	// - If we're in a workspace, proceed - the workspace's existence IS the enable signal
	// - If we're NOT in a workspace, check the global enabled state
	// This ensures a missing/stale state file doesn't break workspace users
	if townRoot == "" {
		// Not in a workspace - check global enabled state
		// (This matters for hooks that might run from random directories)
		if !state.IsEnabled() {
			return nil // Silent exit - not in workspace and not enabled
		}
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Handle hook mode: read session ID from stdin and persist it
	if primeHookMode {
		sessionID, source := readHookSessionID()
		if !primeDryRun {
			persistSessionID(townRoot, sessionID)
			if cwd != townRoot {
				persistSessionID(cwd, sessionID)
			}
		}
		// Set environment for this process (affects event emission below)
		_ = os.Setenv("GT_SESSION_ID", sessionID)
		_ = os.Setenv("CLAUDE_SESSION_ID", sessionID) // Legacy compatibility
		// Output session beacon
		explain(true, "Session beacon: hook mode enabled, session ID from stdin")
		fmt.Printf("[session:%s]\n", sessionID)
		if source != "" {
			fmt.Printf("[source:%s]\n", source)
		}
	}

	// Check for handoff marker (prevents handoff loop bug)
	// In dry-run mode, use the non-mutating version
	if primeDryRun {
		checkHandoffMarkerDryRun(cwd)
	} else {
		checkHandoffMarker(cwd)
	}

	// Get role using env-aware detection
	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		return fmt.Errorf("detecting role: %w", err)
	}

	// Warn prominently if there's a role/cwd mismatch
	if roleInfo.Mismatch {
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

	// Build RoleContext for compatibility with existing code
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

	// Check and acquire identity lock for worker roles
	if !primeDryRun {
		if err := acquireIdentityLock(ctx); err != nil {
			return err
		}
	}

	// Ensure beads redirect exists for worktree-based roles
	// Skip if there's a role/location mismatch to avoid creating bad redirects
	if !roleInfo.Mismatch && !primeDryRun {
		ensureBeadsRedirect(ctx)
	}

	// Emit session_start event for seance discovery
	if !primeDryRun {
		emitSessionEvent(ctx)
	}

	// Output session metadata for seance discovery
	explain(true, "Session metadata: always included for seance discovery")
	outputSessionMetadata(ctx)

	// Output context
	explain(true, fmt.Sprintf("Role context: detected role is %s", ctx.Role))
	if err := outputPrimeContext(ctx); err != nil {
		return err
	}

	// Output handoff content if present
	outputHandoffContent(ctx)

	// Output attachment status (for autonomous work detection)
	outputAttachmentStatus(ctx)

	// Check for slung work on hook (from gt sling)
	// If found, we're in autonomous mode - skip normal startup directive
	hasSlungWork := checkSlungWork(ctx)
	explain(hasSlungWork, "Autonomous mode: hooked/in-progress work detected")

	// Output molecule context if working on a molecule step
	outputMoleculeContext(ctx)

	// Output previous session checkpoint for crash recovery
	outputCheckpointContext(ctx)

	// Run bd prime to output beads workflow context
	if !primeDryRun {
		runBdPrime(cwd)
	} else {
		explain(true, "bd prime: skipped in dry-run mode")
	}

	// Run gt mail check --inject to inject any pending mail
	if !primeDryRun {
		runMailCheckInject(cwd)
	} else {
		explain(true, "gt mail check --inject: skipped in dry-run mode")
	}

	// For Mayor, check for pending escalations
	if ctx.Role == RoleMayor {
		checkPendingEscalations(ctx)
	}

	// Output startup directive for roles that should announce themselves
	// Skip if in autonomous mode (slung work provides its own directive)
	if !hasSlungWork {
		explain(true, "Startup directive: normal mode (no hooked work)")
		outputStartupDirective(ctx)
	}

	return nil
}

func detectRole(cwd, townRoot string) RoleInfo {
	ctx := RoleInfo{
		Role:     RoleUnknown,
		TownRoot: townRoot,
		WorkDir:  cwd,
		Source:   "cwd",
	}

	// Get relative path from town root
	relPath, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return ctx
	}

	// Normalize and split path
	relPath = filepath.ToSlash(relPath)
	parts := strings.Split(relPath, "/")

	// Check for mayor role
	// At town root, or in mayor/ or mayor/rig/
	if relPath == "." || relPath == "" {
		ctx.Role = RoleMayor
		return ctx
	}
	if len(parts) >= 1 && parts[0] == "mayor" {
		ctx.Role = RoleMayor
		return ctx
	}

	// Check for boot role: deacon/dogs/boot/
	// Must check before deacon since boot is under deacon directory
	if len(parts) >= 3 && parts[0] == "deacon" && parts[1] == "dogs" && parts[2] == "boot" {
		ctx.Role = RoleBoot
		return ctx
	}

	// Check for deacon role: deacon/
	if len(parts) >= 1 && parts[0] == "deacon" {
		ctx.Role = RoleDeacon
		return ctx
	}

	// At this point, first part should be a rig name
	if len(parts) < 1 {
		return ctx
	}
	rigName := parts[0]
	ctx.Rig = rigName

	// Check for mayor: <rig>/mayor/ or <rig>/mayor/rig/
	if len(parts) >= 2 && parts[1] == "mayor" {
		ctx.Role = RoleMayor
		return ctx
	}

	// Check for witness: <rig>/witness/rig/
	if len(parts) >= 2 && parts[1] == "witness" {
		ctx.Role = RoleWitness
		return ctx
	}

	// Check for refinery: <rig>/refinery/rig/
	if len(parts) >= 2 && parts[1] == "refinery" {
		ctx.Role = RoleRefinery
		return ctx
	}

	// Check for polecat: <rig>/polecats/<name>/
	if len(parts) >= 3 && parts[1] == "polecats" {
		ctx.Role = RolePolecat
		ctx.Polecat = parts[2]
		return ctx
	}

	// Check for crew: <rig>/crew/<name>/
	if len(parts) >= 3 && parts[1] == "crew" {
		ctx.Role = RoleCrew
		ctx.Polecat = parts[2] // Use Polecat field for crew member name
		return ctx
	}

	// Default: could be rig root - treat as unknown
	return ctx
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
	// Determine agent identity
	agentID := getAgentIdentity(ctx)
	if agentID == "" {
		return false
	}

	b := beads.New(ctx.WorkDir)

	// PREFERRED: Read hook_bead from agent bead (authoritative source)
	// This prevents race conditions when polecat names are recycled - the old
	// beads may still have assignee set to the recycled name, but the agent
	// bead's hook_bead field is atomically updated by gt sling.
	var hookedBead *beads.Issue
	townRoot, _ := findTownRoot()
	agentBeadID := buildAgentBeadIDFromContext(ctx, townRoot)
	if agentBeadID != "" {
		agentBead, err := b.Show(agentBeadID)
		if err == nil && agentBead != nil && agentBead.Type == "agent" && agentBead.HookBead != "" {
			// Found hook_bead in agent bead - fetch the actual bead
			hookedBead, err = b.Show(agentBead.HookBead)
			if err != nil && townRoot != "" {
				// Try town beads if not found in rig beads
				townB := beads.New(townRoot)
				hookedBead, _ = townB.Show(agentBead.HookBead)
			}
		}
	}

	// FALLBACK: Query by assignee (legacy behavior, may have race conditions)
	// Only used if hook_bead is not set or agent bead doesn't exist
	if hookedBead == nil {
		hookedBeads, err := b.List(beads.ListOptions{
			Status:   beads.StatusHooked,
			Assignee: agentID,
			Priority: -1,
		})
		if err != nil {
			return false
		}

		// If no hooked beads found, also check in_progress beads assigned to this agent.
		// This handles the case where work was claimed (status changed to in_progress)
		// but the session was interrupted before completion. The hook should persist.
		if len(hookedBeads) == 0 {
			inProgressBeads, err := b.List(beads.ListOptions{
				Status:   "in_progress",
				Assignee: agentID,
				Priority: -1,
			})
			if err != nil || len(inProgressBeads) == 0 {
				return false
			}
			hookedBeads = inProgressBeads
		}

		// Use the first hooked bead (agents typically have one)
		hookedBead = hookedBeads[0]
	}

	// Build the role announcement string
	roleAnnounce := buildRoleAnnouncement(ctx)

	// Found hooked work! Display AUTONOMOUS MODE prominently
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
	fmt.Printf("2. Then IMMEDIATELY run: `bd show %s`\n", hookedBead.ID)
	fmt.Println("3. Begin execution - no waiting for user input")
	fmt.Println()
	fmt.Println("**DO NOT:**")
	fmt.Println("- Wait for user response after announcing")
	fmt.Println("- Ask clarifying questions")
	fmt.Println("- Describe what you're going to do")
	fmt.Println("- Check mail first (hook takes priority)")
	fmt.Println()

	// Show the hooked work details
	fmt.Printf("%s\n\n", style.Bold.Render("## Hooked Work"))
	fmt.Printf("  Bead ID: %s\n", style.Bold.Render(hookedBead.ID))
	fmt.Printf("  Title: %s\n", hookedBead.Title)
	if hookedBead.Description != "" {
		// Show first few lines of description
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

	// Show bead preview using bd show
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

	// Check for attached molecule and show execution prompt
	// This was missing for hooked beads (only worked for pinned beads).
	// With formula-on-bead, the base bead is hooked with attached_molecule pointing to wisp.
	attachment := beads.ParseAttachmentFields(hookedBead)
	if attachment != nil && attachment.AttachedMolecule != "" {
		fmt.Printf("%s\n\n", style.Bold.Render("## 🎯 ATTACHED MOLECULE"))
		fmt.Printf("Molecule: %s\n", attachment.AttachedMolecule)
		if attachment.AttachedArgs != "" {
			fmt.Printf("\n%s\n", style.Bold.Render("📋 ARGS (use these to guide execution):"))
			fmt.Printf("  %s\n", attachment.AttachedArgs)
		}
		fmt.Println()

		// Show current step from molecule
		showMoleculeExecutionPrompt(ctx.WorkDir, attachment.AttachedMolecule)
	}

	return true
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

// buildAgentBeadIDFromContext constructs the agent bead ID from a RoleContext.
// Uses canonical naming: prefix-rig-role-name
// Town-level agents use hq- prefix; rig-level agents use rig's prefix.
func buildAgentBeadIDFromContext(ctx RoleContext, townRoot string) string {
	getPrefix := func(rig string) string {
		return config.GetRigPrefix(townRoot, rig)
	}

	switch ctx.Role {
	case RoleMayor:
		return beads.MayorBeadIDTown()
	case RoleDeacon:
		return beads.DeaconBeadIDTown()
	case RoleWitness:
		if ctx.Rig != "" {
			return beads.WitnessBeadIDWithPrefix(getPrefix(ctx.Rig), ctx.Rig)
		}
		return ""
	case RoleRefinery:
		if ctx.Rig != "" {
			return beads.RefineryBeadIDWithPrefix(getPrefix(ctx.Rig), ctx.Rig)
		}
		return ""
	case RolePolecat:
		if ctx.Rig != "" && ctx.Polecat != "" {
			return beads.PolecatBeadIDWithPrefix(getPrefix(ctx.Rig), ctx.Rig, ctx.Polecat)
		}
		return ""
	case RoleCrew:
		if ctx.Rig != "" && ctx.Polecat != "" {
			return beads.CrewBeadIDWithPrefix(getPrefix(ctx.Rig), ctx.Rig, ctx.Polecat)
		}
		return ""
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
			// Polecat agent beads use hq- prefix for town beads (fix for gt-myc)
			return beads.PolecatBeadIDTown(ctx.Rig, ctx.Polecat)
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
// Also ensures beads.role=maintainer is set to prevent writes going to ~/.beads-planning.
func ensureBeadsRedirect(ctx RoleContext) {
	// Only applies to worktree-based roles that use shared beads
	if ctx.Role != RoleCrew && ctx.Role != RolePolecat && ctx.Role != RoleRefinery {
		return
	}

	// Check if redirect already exists
	redirectPath := filepath.Join(ctx.WorkDir, ".beads", "redirect")
	if _, err := os.Stat(redirectPath); err != nil {
		// Use shared helper - silently ignore errors during prime
		_ = beads.SetupRedirect(ctx.TownRoot, ctx.WorkDir)
	}

	// Always ensure beads.role=maintainer is set to prevent beads from routing
	// writes to ~/.beads-planning (the default for HTTPS-cloned repos without credentials).
	// This fixes existing workers that were created before the fix. See: gt-3ml66
	g := git.NewGit(ctx.WorkDir)
	_ = g.SetConfig("beads.role", "maintainer")
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
