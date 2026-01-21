package cmd

import (
	"encoding/json"
	"fmt"
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
func outputPrimeContext(ctx RoleContext) error {
	// Try to use templates first
	tmpl, err := templates.New()
	if err != nil {
		// Fall back to hardcoded output if templates fail
		return outputPrimeContextFallback(ctx)
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
	default:
		// Unknown role - use fallback
		return outputPrimeContextFallback(ctx)
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
		return fmt.Errorf("rendering template: %w", err)
	}

	fmt.Print(output)
	return nil
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
	default:
		outputUnknownContext(ctx)
	}
	return nil
}

func outputMayorContext(ctx RoleContext) {
	fmt.Printf("%s\n\n", style.Bold.Render("# Mayor Context"))
	fmt.Println("You are the **Mayor** - the global coordinator of Gas Town.")
	fmt.Println()
	fmt.Println("## Responsibilities")
	fmt.Println("- Coordinate work across all rigs")
	fmt.Println("- Delegate to Refineries, not directly to polecats")
	fmt.Println("- Monitor overall system health")
	fmt.Println()
	fmt.Println("## Key Commands")
	fmt.Println("- `gt mail inbox` - Check your messages")
	fmt.Println("- `gt mail read <id>` - Read a specific message")
	fmt.Println("- `gt status` - Show overall town status")
	fmt.Println("- `gt rig list` - List all rigs")
	fmt.Println("- `bd ready` - Issues ready to work")
	fmt.Println()
	fmt.Println("## Hookable Mail")
	fmt.Println("Mail can be hooked for ad-hoc instructions: `gt hook attach <mail-id>`")
	fmt.Println("If mail is on your hook, read and execute its instructions (GUPP applies).")
	fmt.Println()
	fmt.Println("## Startup")
	fmt.Println("Check for handoff messages with 🤝 HANDOFF in subject - continue predecessor's work.")
	fmt.Println()
	fmt.Printf("Town root: %s\n", style.Dim.Render(ctx.TownRoot))
}

func outputWitnessContext(ctx RoleContext) {
	fmt.Printf("%s\n\n", style.Bold.Render("# Witness Context"))
	fmt.Printf("You are the **Witness** for rig: %s\n\n", style.Bold.Render(ctx.Rig))
	fmt.Println("## Responsibilities")
	fmt.Println("- Monitor polecat health via heartbeat")
	fmt.Println("- Spawn replacement agents for stuck polecats")
	fmt.Println("- Report rig status to Mayor")
	fmt.Println()
	fmt.Println("## Key Commands")
	fmt.Println("- `gt witness status` - Show witness status")
	fmt.Println("- `gt polecat list` - List polecats in this rig")
	fmt.Println()
	fmt.Println("## Hookable Mail")
	fmt.Println("Mail can be hooked for ad-hoc instructions: `gt hook attach <mail-id>`")
	fmt.Println("If mail is on your hook, read and execute its instructions (GUPP applies).")
	fmt.Println()
	fmt.Printf("Rig: %s\n", style.Dim.Render(ctx.Rig))
}

func outputRefineryContext(ctx RoleContext) {
	fmt.Printf("%s\n\n", style.Bold.Render("# Refinery Context"))
	fmt.Printf("You are the **Refinery** for rig: %s\n\n", style.Bold.Render(ctx.Rig))
	fmt.Println("## Responsibilities")
	fmt.Println("- Process the merge queue for this rig")
	fmt.Println("- Merge polecat work to integration branch")
	fmt.Println("- Resolve merge conflicts")
	fmt.Println("- Land completed swarms to main")
	fmt.Println()
	fmt.Println("## Key Commands")
	fmt.Println("- `gt merge queue` - Show pending merges")
	fmt.Println("- `gt merge next` - Process next merge")
	fmt.Println()
	fmt.Println("## Hookable Mail")
	fmt.Println("Mail can be hooked for ad-hoc instructions: `gt hook attach <mail-id>`")
	fmt.Println("If mail is on your hook, read and execute its instructions (GUPP applies).")
	fmt.Println()
	fmt.Printf("Rig: %s\n", style.Dim.Render(ctx.Rig))
}

func outputPolecatContext(ctx RoleContext) {
	fmt.Printf("%s\n\n", style.Bold.Render("# Polecat Context"))
	fmt.Printf("You are polecat **%s** in rig: %s\n\n",
		style.Bold.Render(ctx.Polecat), style.Bold.Render(ctx.Rig))
	fmt.Println("## Startup Protocol")
	fmt.Println("1. Run `gt prime` - loads context and checks mail automatically")
	fmt.Println("2. Check inbox - if mail shown, read with `gt mail read <id>`")
	fmt.Println("3. Look for '📋 Work Assignment' messages for your task")
	fmt.Println("4. If no mail, check `bd list --status=in_progress` for existing work")
	fmt.Println()
	fmt.Println("## Key Commands")
	fmt.Println("- `gt mail inbox` - Check your inbox for work assignments")
	fmt.Println("- `bd show <issue>` - View your assigned issue")
	fmt.Println("- `bd close <issue>` - Mark issue complete")
	fmt.Println("- `gt done` - Signal work ready for merge")
	fmt.Println()
	fmt.Println("## Hookable Mail")
	fmt.Println("Mail can be hooked for ad-hoc instructions: `gt hook attach <mail-id>`")
	fmt.Println("If mail is on your hook, read and execute its instructions (GUPP applies).")
	fmt.Println()
	fmt.Printf("Polecat: %s | Rig: %s\n",
		style.Dim.Render(ctx.Polecat), style.Dim.Render(ctx.Rig))
}

func outputCrewContext(ctx RoleContext) {
	fmt.Printf("%s\n\n", style.Bold.Render("# Crew Worker Context"))
	fmt.Printf("You are crew worker **%s** in rig: %s\n\n",
		style.Bold.Render(ctx.Polecat), style.Bold.Render(ctx.Rig))
	fmt.Println("## About Crew Workers")
	fmt.Println("- Persistent workspace (not auto-garbage-collected)")
	fmt.Println("- User-managed (not Witness-monitored)")
	fmt.Println("- Long-lived identity across sessions")
	fmt.Println()
	fmt.Println("## Key Commands")
	fmt.Println("- `gt mail inbox` - Check your inbox")
	fmt.Println("- `bd ready` - Available issues")
	fmt.Println("- `bd show <issue>` - View issue details")
	fmt.Println("- `bd close <issue>` - Mark issue complete")
	fmt.Println()
	fmt.Println("## Hookable Mail")
	fmt.Println("Mail can be hooked for ad-hoc instructions: `gt hook attach <mail-id>`")
	fmt.Println("If mail is on your hook, read and execute its instructions (GUPP applies).")
	fmt.Println()
	fmt.Printf("Crew: %s | Rig: %s\n",
		style.Dim.Render(ctx.Polecat), style.Dim.Render(ctx.Rig))
}

func outputUnknownContext(ctx RoleContext) {
	fmt.Printf("%s\n\n", style.Bold.Render("# Gas Town Context"))
	fmt.Println("Could not determine specific role from current directory.")
	fmt.Println()
	if ctx.Rig != "" {
		fmt.Printf("You appear to be in rig: %s\n\n", style.Bold.Render(ctx.Rig))
	}
	fmt.Println("Navigate to a specific agent directory:")
	fmt.Println("- `<rig>/polecats/<name>/` - Polecat role")
	fmt.Println("- `<rig>/witness/rig/` - Witness role")
	fmt.Println("- `<rig>/refinery/rig/` - Refinery role")
	fmt.Println("- Town root or `mayor/` - Mayor role")
	fmt.Println()
	fmt.Printf("Town root: %s\n", style.Dim.Render(ctx.TownRoot))
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

// outputStartupDirective outputs role-specific instructions for the agent.
// This tells agents like Mayor to announce themselves on startup.
func outputStartupDirective(ctx RoleContext) {
	switch ctx.Role {
	case RoleMayor:
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("**STARTUP PROTOCOL**: You are the Mayor. Please:")
		fmt.Println("1. Announce: \"Mayor, checking in.\"")
		fmt.Println("2. Check mail: `gt mail inbox` - look for 🤝 HANDOFF messages")
		fmt.Println("3. Check for attached work: `gt hook`")
		fmt.Println("   - If mol attached → **RUN IT** (no human input needed)")
		fmt.Println("   - If no mol → await user instruction")
	case RoleWitness:
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("**STARTUP PROTOCOL**: You are the Witness. Please:")
		fmt.Println("1. Announce: \"Witness, checking in.\"")
		fmt.Println("2. Check mail: `gt mail inbox` - look for 🤝 HANDOFF messages")
		fmt.Println("3. Check for attached patrol: `gt hook`")
		fmt.Println("   - If mol attached → **RUN IT** (resume from current step)")
		fmt.Println("   - If no mol → create patrol: `bd mol wisp mol-witness-patrol`")
	case RolePolecat:
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("**STARTUP PROTOCOL**: You are a polecat. Please:")
		fmt.Printf("1. Announce: \"%s Polecat %s, checking in.\"\n", ctx.Rig, ctx.Polecat)
		fmt.Println("2. Check mail: `gt mail inbox`")
		fmt.Println("3. If there's a 🤝 HANDOFF message, read it for context")
		fmt.Println("4. Check for attached work: `gt hook`")
		fmt.Println("   - If mol attached → **RUN IT** (you were spawned with this work)")
		fmt.Println("   - If no mol → ERROR: polecats must have work attached; escalate to Witness")
	case RoleRefinery:
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("**STARTUP PROTOCOL**: You are the Refinery. Please:")
		fmt.Println("1. Announce: \"Refinery, checking in.\"")
		fmt.Println("2. Check mail: `gt mail inbox` - look for 🤝 HANDOFF messages")
		fmt.Println("3. Check for attached patrol: `gt hook`")
		fmt.Println("   - If mol attached → **RUN IT** (resume from current step)")
		fmt.Println("   - If no mol → create patrol: `bd mol wisp mol-refinery-patrol`")
	case RoleCrew:
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("**STARTUP PROTOCOL**: You are a crew worker. Please:")
		fmt.Printf("1. Announce: \"%s Crew %s, checking in.\"\n", ctx.Rig, ctx.Polecat)
		fmt.Println("2. Check mail: `gt mail inbox`")
		fmt.Println("3. If there's a 🤝 HANDOFF message, read it and continue the work")
		fmt.Println("4. Check for attached work: `gt hook`")
		fmt.Println("   - If attachment found → **RUN IT** (no human input needed)")
		fmt.Println("   - If no attachment → await user instruction")
	case RoleDeacon:
		// Skip startup protocol if paused - the pause message was already shown
		paused, _, _ := deacon.IsPaused(ctx.TownRoot)
		if paused {
			return
		}
		fmt.Println()
		fmt.Println("---")
		fmt.Println()
		fmt.Println("**STARTUP PROTOCOL**: You are the Deacon. Please:")
		fmt.Println("1. Announce: \"Deacon, checking in.\"")
		fmt.Println("2. Signal awake: `gt deacon heartbeat \"starting patrol\"`")
		fmt.Println("3. Check mail: `gt mail inbox` - look for 🤝 HANDOFF messages")
		fmt.Println("4. Check for attached patrol: `gt hook`")
		fmt.Println("   - If mol attached → **RUN IT** (resume from current step)")
		fmt.Println("   - If no mol → create patrol: `bd mol wisp mol-deacon-patrol`")
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

	// Has attached work - output prominently with current step
	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render("## 🎯 ATTACHED WORK DETECTED"))
	fmt.Printf("Pinned bead: %s\n", pinnedBeads[0].ID)
	fmt.Printf("Attached molecule: %s\n", attachment.AttachedMolecule)
	if attachment.AttachedAt != "" {
		fmt.Printf("Attached at: %s\n", attachment.AttachedAt)
	}
	if attachment.AttachedArgs != "" {
		fmt.Println()
		fmt.Printf("%s\n", style.Bold.Render("📋 ARGS (use these to guide execution):"))
		fmt.Printf("  %s\n", attachment.AttachedArgs)
	}
	fmt.Println()

	// Show current step from molecule
	showMoleculeExecutionPrompt(ctx.WorkDir, attachment.AttachedMolecule)
}

// outputHandoffWarning outputs the post-handoff warning message.
func outputHandoffWarning(prevSession string) {
	fmt.Println()
	fmt.Println(style.Bold.Render("╔══════════════════════════════════════════════════════════════════╗"))
	fmt.Println(style.Bold.Render("║  ✅ HANDOFF COMPLETE - You are the NEW session                   ║"))
	fmt.Println(style.Bold.Render("╚══════════════════════════════════════════════════════════════════╝"))
	fmt.Println()
	if prevSession != "" {
		fmt.Printf("Your predecessor (%s) handed off to you.\n", prevSession)
	}
	fmt.Println()
	fmt.Println(style.Bold.Render("⚠️  DO NOT run /handoff - that was your predecessor's action."))
	fmt.Println("   The /handoff you see in context is NOT a request for you.")
	fmt.Println()
	fmt.Println("Instead: Check your hook (`gt mol status`) and mail (`gt mail inbox`).")
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

	// Display checkpoint context
	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render("## 📌 Previous Session Checkpoint"))
	fmt.Printf("A previous session left a checkpoint %s ago.\n\n", cp.Age().Round(time.Minute))

	if cp.StepTitle != "" {
		fmt.Printf("  **Working on:** %s\n", cp.StepTitle)
	}
	if cp.MoleculeID != "" {
		fmt.Printf("  **Molecule:** %s\n", cp.MoleculeID)
	}
	if cp.CurrentStep != "" {
		fmt.Printf("  **Step:** %s\n", cp.CurrentStep)
	}
	if cp.HookedBead != "" {
		fmt.Printf("  **Hooked bead:** %s\n", cp.HookedBead)
	}
	if cp.Branch != "" {
		fmt.Printf("  **Branch:** %s\n", cp.Branch)
	}
	if len(cp.ModifiedFiles) > 0 {
		fmt.Printf("  **Modified files:** %d\n", len(cp.ModifiedFiles))
		// Show first few files
		maxShow := 5
		if len(cp.ModifiedFiles) < maxShow {
			maxShow = len(cp.ModifiedFiles)
		}
		for i := 0; i < maxShow; i++ {
			fmt.Printf("    - %s\n", cp.ModifiedFiles[i])
		}
		if len(cp.ModifiedFiles) > maxShow {
			fmt.Printf("    ... and %d more\n", len(cp.ModifiedFiles)-maxShow)
		}
	}
	if cp.Notes != "" {
		fmt.Printf("  **Notes:** %s\n", cp.Notes)
	}
	fmt.Println()

	fmt.Println("Use this context to resume work. The checkpoint will be updated as you progress.")
	fmt.Println()
}

// outputDeaconPausedMessage outputs a prominent PAUSED message for the Deacon.
// When paused, the Deacon must not perform any patrol actions.
func outputDeaconPausedMessage(state *deacon.PauseState) {
	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render("## ⏸️  DEACON PAUSED"))
	fmt.Println("You are paused and must NOT perform any patrol actions.")
	fmt.Println()
	if state.Reason != "" {
		fmt.Printf("Reason: %s\n", state.Reason)
	}
	fmt.Printf("Paused at: %s\n", state.PausedAt.Format(time.RFC3339))
	if state.PausedBy != "" {
		fmt.Printf("Paused by: %s\n", state.PausedBy)
	}
	fmt.Println()
	fmt.Println("Wait for human to run `gt deacon resume` before working.")
	fmt.Println()
	fmt.Println("**DO NOT:**")
	fmt.Println("- Create patrol molecules")
	fmt.Println("- Run heartbeats")
	fmt.Println("- Check agent health")
	fmt.Println("- Take any autonomous actions")
	fmt.Println()
	fmt.Println("You may respond to direct human questions.")
}

// explain outputs an explanatory message if --explain mode is enabled.
func explain(condition bool, reason string) {
	if primeExplain && condition {
		fmt.Printf("\n[EXPLAIN] %s\n", reason)
	}
}
