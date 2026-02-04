package cmd

import (
	"github.com/steveyegge/gastown/internal/cli"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

type wispCreateJSON struct {
	NewEpicID string `json:"new_epic_id"`
	RootID    string `json:"root_id"`
	ResultID  string `json:"result_id"`
}

func parseWispIDFromJSON(jsonOutput []byte) (string, error) {
	var result wispCreateJSON
	if err := json.Unmarshal(jsonOutput, &result); err != nil {
		return "", fmt.Errorf("parsing wisp JSON: %w (output: %s)", err, trimJSONForError(jsonOutput))
	}

	switch {
	case result.NewEpicID != "":
		return result.NewEpicID, nil
	case result.RootID != "":
		return result.RootID, nil
	case result.ResultID != "":
		return result.ResultID, nil
	default:
		return "", fmt.Errorf("wisp JSON missing id field (expected one of new_epic_id, root_id, result_id); output: %s", trimJSONForError(jsonOutput))
	}
}

func trimJSONForError(jsonOutput []byte) string {
	s := strings.TrimSpace(string(jsonOutput))
	const maxLen = 500
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// verifyFormulaExists checks that the formula exists using bd formula show.
// Formulas are TOML files (.formula.toml).
// Uses --no-daemon with --allow-stale for consistency with verifyBeadExists.
func verifyFormulaExists(formulaName string) error {
	// Try bd formula show (handles all formula file formats)
	// Use Output() instead of Run() to detect bd --no-daemon exit 0 bug:
	// when formula not found, --no-daemon may exit 0 but produce empty stdout.
	cmd := exec.Command("bd", "--no-daemon", "formula", "show", formulaName, "--allow-stale")
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		return nil
	}

	// Try with mol- prefix
	cmd = exec.Command("bd", "--no-daemon", "formula", "show", "mol-"+formulaName, "--allow-stale")
	if out, err := cmd.Output(); err == nil && len(out) > 0 {
		return nil
	}

	return fmt.Errorf("formula '%s' not found (check 'bd formula list')", formulaName)
}

// runSlingFormula handles standalone formula slinging.
// Flow: cook → wisp → attach to hook → nudge
func runSlingFormula(args []string) error {
	formulaName := args[0]

	// Get town root early - needed for BEADS_DIR when running bd commands
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// Determine target (self or specified)
	var target string
	if len(args) > 1 {
		target = args[1]
	}

	// Resolve target agent and pane
	var targetAgent string
	var targetPane string
	var delayedDogInfo *DogDispatchInfo // For delayed session start after hook is set
	var formulaWorkDir string            // Working directory for bd cook/wisp (routes to correct rig beads)
	var isSelfSling bool                 // True if slinging to self (skip nudge - agent already knows)

	if target != "" {
		// Resolve "." to current agent identity (like git's "." meaning current directory)
		if target == "." {
			targetAgent, targetPane, formulaWorkDir, err = resolveSelfTarget()
			if err != nil {
				return fmt.Errorf("resolving self for '.' target: %w", err)
			}
			isSelfSling = true
		} else if dogName, isDog := IsDogTarget(target); isDog {
			if slingDryRun {
				if dogName == "" {
					fmt.Printf("Would dispatch to idle dog in kennel\n")
				} else {
					fmt.Printf("Would dispatch to dog '%s'\n", dogName)
				}
				targetAgent = fmt.Sprintf("deacon/dogs/%s", dogName)
				if dogName == "" {
					targetAgent = "deacon/dogs/<idle>"
				}
				targetPane = "<dog-pane>"
			} else {
				// Dispatch to dog with delayed session start
				// Session starts after hook is set to avoid race condition
				dispatchOpts := DogDispatchOptions{
					Create:            slingCreate,
					WorkDesc:          formulaName,
					DelaySessionStart: true,
				}
				dispatchInfo, dispatchErr := DispatchToDog(dogName, dispatchOpts)
				if dispatchErr != nil {
					return fmt.Errorf("dispatching to dog: %w", dispatchErr)
				}
				targetAgent = dispatchInfo.AgentID
				delayedDogInfo = dispatchInfo // Store for later session start
				fmt.Printf("Dispatched to dog %s (session start delayed)\n", dispatchInfo.DogName)
			}
		} else if rigName, isRig := IsRigName(target); isRig {
			// Check if target is a rig name (auto-spawn polecat)
			if slingDryRun {
				// Dry run - just indicate what would happen
				fmt.Printf("Would spawn fresh polecat in rig '%s'\n", rigName)
				targetAgent = fmt.Sprintf("%s/polecats/<new>", rigName)
				targetPane = "<new-pane>"
			} else {
				// Spawn a fresh polecat in the rig
				fmt.Printf("Target is rig '%s', spawning fresh polecat...\n", rigName)
				spawnOpts := SlingSpawnOptions{
					Force:   slingForce,
					Account: slingAccount,
					Create:  slingCreate,
					Agent:   slingAgent,
				}
				spawnInfo, spawnErr := SpawnPolecatForSling(rigName, spawnOpts)
				if spawnErr != nil {
					return fmt.Errorf("spawning polecat: %w", spawnErr)
				}
				targetAgent = spawnInfo.AgentID()
				targetPane = spawnInfo.Pane
				formulaWorkDir = spawnInfo.ClonePath // Route bd commands to rig beads

				// Wake witness and refinery to monitor the new polecat (G11: skip if --no-boot)
				if !slingNoBoot {
					wakeRigAgents(rigName)
				}
			}
		} else {
			// Slinging to an existing agent
			targetAgent, targetPane, formulaWorkDir, err = resolveTargetAgent(target)
			if err != nil {
				return fmt.Errorf("resolving target: %w", err)
			}
		}
	} else {
		// Slinging to self
		targetAgent, targetPane, formulaWorkDir, err = resolveSelfTarget()
		if err != nil {
			return err
		}
		isSelfSling = true
	}

	fmt.Printf("%s Slinging formula %s to %s...\n", style.Bold.Render("🎯"), formulaName, targetAgent)

	if slingDryRun {
		fmt.Printf("Would cook formula: %s\n", formulaName)
		fmt.Printf("Would create wisp and pin to: %s\n", targetAgent)
		for _, v := range slingVars {
			fmt.Printf("  --var %s\n", v)
		}
		fmt.Printf("Would nudge pane: %s\n", targetPane)
		return nil
	}

	// Resolve working directory for bd commands (routes to correct rig beads)
	// Fall back to townRoot (HQ beads) if no specific rig directory was determined
	if formulaWorkDir == "" {
		formulaWorkDir = townRoot
	}

	// Step 1: Cook the formula (ensures proto exists)
	fmt.Printf("  Cooking formula...\n")
	cookArgs := []string{"--no-daemon", "cook", formulaName}
	cookCmd := exec.Command("bd", cookArgs...)
	cookCmd.Dir = formulaWorkDir
	cookCmd.Stderr = os.Stderr
	if err := cookCmd.Run(); err != nil {
		return fmt.Errorf("cooking formula: %w", err)
	}

	// Step 2: Create wisp instance (ephemeral)
	fmt.Printf("  Creating wisp...\n")
	wispArgs := []string{"--no-daemon", "mol", "wisp", formulaName}
	for _, v := range slingVars {
		wispArgs = append(wispArgs, "--var", v)
	}
	wispArgs = append(wispArgs, "--json")

	wispCmd := exec.Command("bd", wispArgs...)
	wispCmd.Dir = formulaWorkDir
	wispCmd.Stderr = os.Stderr // Show wisp errors to user
	wispOut, err := wispCmd.Output()
	if err != nil {
		return fmt.Errorf("creating wisp: %w", err)
	}

	// Parse wisp output to get the root ID
	wispRootID, err := parseWispIDFromJSON(wispOut)
	if err != nil {
		return fmt.Errorf("parsing wisp output: %w", err)
	}

	fmt.Printf("%s Wisp created: %s\n", style.Bold.Render("✓"), wispRootID)
	attachedMoleculeID := wispRootID

	// Step 3: Hook the wisp bead using bd update.
	// See: https://github.com/steveyegge/gastown/issues/148
	hookCmd := exec.Command("bd", "--no-daemon", "update", wispRootID, "--status=hooked", "--assignee="+targetAgent)
	hookCmd.Dir = beads.ResolveHookDir(townRoot, wispRootID, "")
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		return fmt.Errorf("hooking wisp bead: %w", err)
	}
	fmt.Printf("%s Attached to hook (status=hooked)\n", style.Bold.Render("✓"))

	// Log sling event to activity feed (formula slinging)
	actor := detectActor()
	payload := events.SlingPayload(wispRootID, targetAgent)
	payload["formula"] = formulaName
	_ = events.LogFeed(events.TypeSling, actor, payload)

	// Update agent bead's hook_bead field (ZFC: agents track their current work)
	// Note: formula slinging uses town root as workDir (no polecat-specific path)
	updateAgentHookBead(targetAgent, wispRootID, "", townBeadsDir)

	// Store dispatcher in bead description (enables completion notification to dispatcher)
	if err := storeDispatcherInBead(wispRootID, actor); err != nil {
		// Warn but don't fail - polecat will still complete work
		fmt.Printf("%s Could not store dispatcher in bead: %v\n", style.Dim.Render("Warning:"), err)
	}

	// Store args in wisp bead if provided (no-tmux mode: beads as data plane)
	if slingArgs != "" {
		if err := storeArgsInBead(wispRootID, slingArgs); err != nil {
			fmt.Printf("%s Could not store args in bead: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			fmt.Printf("%s Args stored in bead (durable)\n", style.Bold.Render("✓"))
		}
	}

	// Record the attached molecule after other description updates to avoid overwrite.
	if attachedMoleculeID != "" {
		if err := storeAttachedMoleculeInBead(wispRootID, attachedMoleculeID); err != nil {
			// Warn but don't fail - polecat can still work through steps
			fmt.Printf("%s Could not store attached_molecule: %v\n", style.Dim.Render("Warning:"), err)
		}
	}

	// Start delayed dog session now that hook is set
	// This ensures dog sees the hook when gt prime runs on session start
	if delayedDogInfo != nil {
		pane, err := delayedDogInfo.StartDelayedSession()
		if err != nil {
			return fmt.Errorf("starting delayed dog session: %w", err)
		}
		targetPane = pane
	}

	// Step 4: Nudge to start (graceful if no tmux)
	// Skip for self-sling - agent is currently processing the sling command and will see
	// the hooked work on next turn. Nudging would inject text while agent is busy.
	if isSelfSling {
		fmt.Printf("%s Self-sling: work hooked, will process on next turn\n", style.Dim.Render("○"))
		return nil
	}
	if targetPane == "" {
		fmt.Printf("%s No pane to nudge (agent will discover work via gt prime)\n", style.Dim.Render("○"))
		return nil
	}

	// Skip nudge during tests to prevent agent self-interruption
	if os.Getenv("GT_TEST_NO_NUDGE") != "" {
		return nil
	}

	var prompt string
	if slingArgs != "" {
		prompt = fmt.Sprintf("Formula %s slung. Args: %s. Run `" + cli.Name() + " hook` to see your hook, then execute using these args.", formulaName, slingArgs)
	} else {
		prompt = fmt.Sprintf("Formula %s slung. Run `" + cli.Name() + " hook` to see your hook, then execute the steps.", formulaName)
	}
	t := tmux.NewTmux()
	if err := t.NudgePane(targetPane, prompt); err != nil {
		// Graceful fallback for no-tmux mode
		fmt.Printf("%s Could not nudge (no tmux?): %v\n", style.Dim.Render("○"), err)
		fmt.Printf("  Agent will discover work via gt prime / bd show\n")
	} else {
		fmt.Printf("%s Nudged to start\n", style.Bold.Render("▶"))
	}

	return nil
}
