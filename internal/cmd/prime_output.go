package cmd

import (
	"github.com/steveyegge/gastown/internal/cli"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/checkpoint"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/workspace"
)

// outputPrimeContext outputs the role-specific context using templates or fallback.
// Returns true if templates were used (caller should skip duplicate startup directive).
func outputPrimeContext(ctx RoleContext) (bool, error) {
	// Try to use templates first
	tmpl, err := templates.New()
	if err != nil {
		// Fall back to hardcoded output if templates fail
		return false, outputPrimeContextFallback(ctx)
	}

	// Map role to template name
	var roleName string
	switch ctx.Role {
	case RoleMayor:
		roleName = "mayor"
	case RoleDeacon:
		roleName = "deacon"
	case RoleWitness:
		roleName = "witness"
	case RoleRefinery:
		roleName = "refinery"
	case RolePolecat:
		roleName = "polecat"
	case RoleCrew:
		roleName = "crew"
	case RoleBoot:
		roleName = "boot"
	default:
		// Unknown role - use fallback
		return false, outputPrimeContextFallback(ctx)
	}

	// Build template data
	// Get town name for session names
	townName, _ := workspace.GetTownName(ctx.TownRoot)

	// Get default branch from rig config (default to "main" if not set)
	defaultBranch := "main"
	if ctx.Rig != "" && ctx.TownRoot != "" {
		rigPath := filepath.Join(ctx.TownRoot, ctx.Rig)
		if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.DefaultBranch != "" {
			defaultBranch = rigCfg.DefaultBranch
		}
	}

	data := templates.RoleData{
		Role:          roleName,
		RigName:       ctx.Rig,
		TownRoot:      ctx.TownRoot,
		TownName:      townName,
		WorkDir:       ctx.WorkDir,
		DefaultBranch: defaultBranch,
		Polecat:       ctx.Polecat,
		MayorSession:  session.MayorSessionName(),
		DeaconSession: session.DeaconSessionName(),
	}

	// Render and output
	output, err := tmpl.RenderRole(roleName, data)
	if err != nil {
		return false, fmt.Errorf("rendering template: %w", err)
	}

	fmt.Print(output)
	return true, nil
}

func outputPrimeContextFallback(ctx RoleContext) error {
	switch ctx.Role {
	case RoleMayor:
		outputMayorContext(ctx)
	case RoleWitness:
		outputWitnessContext(ctx)
	case RoleRefinery:
		outputRefineryContext(ctx)
	case RolePolecat:
		outputPolecatContext(ctx)
	case RoleCrew:
		outputCrewContext(ctx)
	case RoleBoot:
		outputBootContext(ctx)
	default:
		outputUnknownContext(ctx)
	}
	return nil
}

func outputMayorContext(ctx RoleContext) {
	c := cli.Name()
	fmt.Printf("%s\n\n", style.Bold.Render("# Mayor Context"))
	fmt.Println("Global coordinator. Delegate via `" + c + " sling`, monitor with `" + c + " status`.")
	fmt.Printf("Commands: `%s mail inbox`, `%s rig list`, `bd ready`\n", c, c)
	fmt.Println()
	outputCommandQuickReference(ctx)
	fmt.Printf("Town root: %s\n", style.Dim.Render(ctx.TownRoot))
}

func outputWitnessContext(ctx RoleContext) {
	c := cli.Name()
	fmt.Printf("%s\n\n", style.Bold.Render("# Witness Context"))
	fmt.Printf("Rig %s pit boss. Monitor polecats, nudge stuck workers, escalate to Mayor.\n", style.Bold.Render(ctx.Rig))
	fmt.Printf("Commands: `%s polecat list`, `%s peek`, `%s nudge`\n", c, c, c)
	fmt.Println()
	outputCommandQuickReference(ctx)
	fmt.Printf("Rig: %s\n", style.Dim.Render(ctx.Rig))
}

func outputRefineryContext(ctx RoleContext) {
	c := cli.Name()
	fmt.Printf("%s\n\n", style.Bold.Render("# Refinery Context"))
	fmt.Printf("Rig %s merge processor. Queue scan → test → merge → land.\n", style.Bold.Render(ctx.Rig))
	fmt.Printf("Commands: `%s mq list`, `%s merge next`\n", c, c)
	fmt.Println()
	outputCommandQuickReference(ctx)
	fmt.Printf("Rig: %s\n", style.Dim.Render(ctx.Rig))
}

func outputPolecatContext(ctx RoleContext) {
	c := cli.Name()
	fmt.Printf("%s\n\n", style.Bold.Render("# Polecat Context"))
	fmt.Printf("Polecat **%s** in rig %s. Ephemeral worker.\n",
		style.Bold.Render(ctx.Polecat), style.Bold.Render(ctx.Rig))
	fmt.Printf("Commands: `%s mail inbox`, `bd show <id>`, `bd close <id>`, `%s done`\n", c, c)
	fmt.Println()
	outputCommandQuickReference(ctx)
	fmt.Printf("Polecat: %s | Rig: %s\n",
		style.Dim.Render(ctx.Polecat), style.Dim.Render(ctx.Rig))
}

func outputCrewContext(ctx RoleContext) {
	c := cli.Name()
	fmt.Printf("%s\n\n", style.Bold.Render("# Crew Worker Context"))
	fmt.Printf("Crew **%s** in rig %s. Persistent workspace, user-managed.\n",
		style.Bold.Render(ctx.Polecat), style.Bold.Render(ctx.Rig))
	fmt.Printf("Commands: `%s mail inbox`, `bd ready`, `bd show <id>`, `bd close <id>`\n", c)
	fmt.Println()
	outputCommandQuickReference(ctx)
	fmt.Printf("Crew: %s | Rig: %s\n",
		style.Dim.Render(ctx.Polecat), style.Dim.Render(ctx.Rig))
}

func outputBootContext(ctx RoleContext) {
	c := cli.Name()
	fmt.Printf("%s\n\n", style.Bold.Render("# Boot Watchdog Context"))
	fmt.Printf("Ephemeral Deacon triage. Commands: `%s boot triage`, `%s deacon status`\n", c, c)
	fmt.Println()
	outputCommandQuickReference(ctx)
	fmt.Printf("Town root: %s\n", style.Dim.Render(ctx.TownRoot))
}

func outputUnknownContext(ctx RoleContext) {
	fmt.Printf("%s\n\n", style.Bold.Render("# Gas Town Context"))
	fmt.Println("Unknown role. Set GT_ROLE or cd into an agent directory.")
	if ctx.Rig != "" {
		fmt.Printf("Rig: %s\n", style.Bold.Render(ctx.Rig))
	}
	fmt.Printf("Town root: %s\n", style.Dim.Render(ctx.TownRoot))
}

// outputCommandQuickReference outputs a compact role-aware command cheatsheet (fallback only).
func outputCommandQuickReference(ctx RoleContext) {
	c := cli.Name()
	fmt.Println("## Commands")
	// Common across all roles
	fmt.Printf("Nudge: `%s nudge <target> \"msg\"` (NOT tmux send-keys). Issues: `bd create \"title\"`\n", c)

	switch ctx.Role {
	case RoleMayor:
		fmt.Printf("Dispatch: `%s sling <bead> <rig>`. Kill: `%s polecat nuke <r>/<n> --force`\n", c, c)
	case RoleCrew:
		fmt.Printf("Dispatch: `%s sling <bead> <rig>`. Stop self: `%s crew stop %s`\n", c, c, ctx.Polecat)
	case RolePolecat:
		fmt.Printf("Done: `%s done`. Steps: `bd mol current`. Escalate: `%s escalate \"desc\" -s HIGH`\n", c, c)
	case RoleWitness:
		fmt.Printf("Kill: `%s polecat nuke %s/<n> --force`. Peek: `%s peek %s/<n> 50`\n", c, ctx.Rig, c, ctx.Rig)
	case RoleRefinery:
		fmt.Printf("Queue: `%s mq list %s`\n", c, ctx.Rig)
	case RoleDeacon:
		fmt.Printf("Rig: `%s rig start/stop/park/dock <rig>`\n", c)
	case RoleBoot:
		fmt.Printf("Triage: `%s boot triage`. Health: `%s deacon status`\n", c, c)
	}
	fmt.Println("Lifecycle: park/unpark (temp), dock/undock (persist), stop/start (immediate)")
	fmt.Println()
}

// outputContextFile reads and displays the CONTEXT.md file from the town root.
// This provides a simple plugin point for operators to inject custom instructions
// that all agents (including polecats) will see during priming.
func outputContextFile(ctx RoleContext) {
	contextPath := filepath.Join(ctx.TownRoot, "CONTEXT.md")
	data, err := os.ReadFile(contextPath)
	if err != nil {
		explain(true, "CONTEXT.md: not found at "+contextPath)
		return
	}
	explain(true, "CONTEXT.md: found at "+contextPath+", injecting contents")
	fmt.Println()
	fmt.Print(string(data))
}

// outputHandoffContent reads and displays the pinned handoff bead for the role.
func outputHandoffContent(ctx RoleContext) {
	if ctx.Role == RoleUnknown {
		return
	}

	// Get role key for handoff bead lookup
	roleKey := string(ctx.Role)

	bd := beads.New(ctx.TownRoot)
	issue, err := bd.FindHandoffBead(roleKey)
	if err != nil {
		// Silently skip if beads lookup fails (might not be a beads repo)
		return
	}
	if issue == nil || issue.Description == "" {
		// No handoff content
		return
	}

	// Display handoff content
	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render("## 🤝 Handoff from Previous Session"))
	fmt.Println(issue.Description)
	fmt.Println()
	fmt.Println(style.Dim.Render("(Clear with: gt rig reset --handoff)"))
}

// outputStartupDirective outputs role-specific startup instructions.
// Only used in fallback mode (templates not available) or compact/resume.
func outputStartupDirective(ctx RoleContext) {
	c := cli.Name()
	announce := buildRoleAnnouncement(ctx)

	fmt.Println()
	fmt.Println("---")
	fmt.Println()

	switch ctx.Role {
	case RolePolecat:
		fmt.Println("**STARTUP**: No work on hook. Check mail above → execute or `" + c + " done`.")
	case RoleBoot:
		fmt.Println("**STARTUP**: Run `" + c + " boot triage` immediately, then exit.")
	case RoleDeacon:
		paused, _, _ := deacon.IsPaused(ctx.TownRoot)
		if paused {
			return
		}
		fmt.Printf("**STARTUP**: Announce \"%s\" → `%s deacon heartbeat` → `%s hook` → run or `%s patrol new`\n", announce, c, c, c)
	default:
		fmt.Printf("**STARTUP**: Announce \"%s\" → `%s hook` → run hooked work or check mail\n", announce, c)
	}
}

// outputAttachmentStatus checks for attached work molecule and outputs status.
// This is key for the autonomous overnight work pattern.
// The Propulsion Principle: "If you find something on your hook, YOU RUN IT."
func outputAttachmentStatus(ctx RoleContext) {
	// Skip only unknown roles - all valid roles can have pinned work
	if ctx.Role == RoleUnknown {
		return
	}

	// Check for pinned beads with attachments
	b := beads.New(ctx.WorkDir)

	// Build assignee string based on role (same as getAgentIdentity)
	assignee := getAgentIdentity(ctx)
	if assignee == "" {
		return
	}

	// Find pinned beads for this agent
	pinnedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusPinned,
		Assignee: assignee,
		Priority: -1,
	})
	if err != nil || len(pinnedBeads) == 0 {
		// No pinned beads - interactive mode
		return
	}

	// Check first pinned bead for attachment
	attachment := beads.ParseAttachmentFields(pinnedBeads[0])
	if attachment == nil || attachment.AttachedMolecule == "" {
		// No attachment - interactive mode
		return
	}

	// Has attached work - output with current step
	fmt.Println()
	fmt.Printf("%s\n", style.Bold.Render("## 🎯 ATTACHED WORK"))
	fmt.Printf("Bead: %s | Molecule: %s\n", pinnedBeads[0].ID, attachment.AttachedMolecule)
	if attachment.AttachedArgs != "" {
		fmt.Printf("Args: %s\n", attachment.AttachedArgs)
	}
	fmt.Println()

	// Show current step from molecule
	showMoleculeExecutionPrompt(ctx.WorkDir, attachment.AttachedMolecule)
}

// outputHandoffWarning outputs the post-handoff warning message.
func outputHandoffWarning(prevSession string) {
	fmt.Println()
	fmt.Printf("%s", style.Bold.Render("✅ HANDOFF COMPLETE"))
	if prevSession != "" {
		fmt.Printf(" (from %s)", prevSession)
	}
	fmt.Println()
	fmt.Println("DO NOT run /handoff again. Check hook and mail instead.")
	fmt.Println()
}

// outputState outputs only the session state (for --state flag).
// If jsonOutput is true, outputs JSON format instead of key:value.
func outputState(ctx RoleContext, jsonOutput bool) {
	state := detectSessionState(ctx)

	if jsonOutput {
		data, err := json.Marshal(state)
		if err != nil {
			// Fall back to plain text on error
			fmt.Printf("state: %s\n", state.State)
			fmt.Printf("role: %s\n", state.Role)
			return
		}
		fmt.Println(string(data))
		return
	}

	fmt.Printf("state: %s\n", state.State)
	fmt.Printf("role: %s\n", state.Role)

	switch state.State {
	case "post-handoff":
		if state.PrevSession != "" {
			fmt.Printf("prev_session: %s\n", state.PrevSession)
		}
	case "crash-recovery":
		if state.CheckpointAge != "" {
			fmt.Printf("checkpoint_age: %s\n", state.CheckpointAge)
		}
	case "autonomous":
		if state.HookedBead != "" {
			fmt.Printf("hooked_bead: %s\n", state.HookedBead)
		}
	}
}

// outputCheckpointContext reads and displays any previous session checkpoint.
// This enables crash recovery by showing what the previous session was working on.
func outputCheckpointContext(ctx RoleContext) {
	// Only applies to polecats and crew workers
	if ctx.Role != RolePolecat && ctx.Role != RoleCrew {
		return
	}

	// Read checkpoint
	cp, err := checkpoint.Read(ctx.WorkDir)
	if err != nil {
		// Silently ignore read errors
		return
	}
	if cp == nil {
		// No checkpoint exists
		return
	}

	// Check if checkpoint is stale (older than 24 hours)
	if cp.IsStale(24 * time.Hour) {
		// Remove stale checkpoint
		_ = checkpoint.Remove(ctx.WorkDir)
		return
	}

	// Display checkpoint context (compact)
	fmt.Println()
	fmt.Printf("%s (%s ago)\n", style.Bold.Render("## 📌 Checkpoint"), cp.Age().Round(time.Minute))
	if cp.StepTitle != "" {
		fmt.Printf("  Task: %s\n", cp.StepTitle)
	}
	if cp.MoleculeID != "" {
		fmt.Printf("  Mol: %s", cp.MoleculeID)
		if cp.CurrentStep != "" {
			fmt.Printf(" step: %s", cp.CurrentStep)
		}
		fmt.Println()
	}
	if cp.HookedBead != "" {
		fmt.Printf("  Hook: %s\n", cp.HookedBead)
	}
	if cp.Branch != "" {
		fmt.Printf("  Branch: %s\n", cp.Branch)
	}
	if len(cp.ModifiedFiles) > 0 {
		fmt.Printf("  Modified: %d files\n", len(cp.ModifiedFiles))
	}
	if cp.Notes != "" {
		fmt.Printf("  Notes: %s\n", cp.Notes)
	}
	fmt.Println()
}

// outputDeaconPausedMessage outputs a prominent PAUSED message for the Deacon.
func outputDeaconPausedMessage(state *deacon.PauseState) {
	fmt.Println()
	fmt.Printf("%s\n", style.Bold.Render("## ⏸️  DEACON PAUSED"))
	fmt.Printf("NO patrol actions. Wait for `%s deacon resume`.\n", cli.Name())
	if state.Reason != "" {
		fmt.Printf("Reason: %s\n", state.Reason)
	}
	fmt.Printf("Since: %s", state.PausedAt.Format(time.RFC3339))
	if state.PausedBy != "" {
		fmt.Printf(" by %s", state.PausedBy)
	}
	fmt.Println()
}

// explain outputs an explanatory message if --explain mode is enabled.
func explain(condition bool, reason string) {
	if primeExplain && condition {
		fmt.Printf("\n[EXPLAIN] %s\n", reason)
	}
}
