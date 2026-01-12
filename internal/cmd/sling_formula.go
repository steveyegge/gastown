package cmd

import (
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
// Flow: cook → wisp → spawn (if rig target) → attach to hook → nudge
//
// When target is a rig, we create the wisp BEFORE spawning the polecat so we can
// pass HookBead atomically at spawn time. This ensures the polecat's agent bead
// always has a valid hook_bead from the start, avoiding race conditions where
// gt prime might run before the hook is set.
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

	// Check if target is a rig early - we need to know this to reorder operations
	var targetIsRig bool
	var rigName string
	if target != "" && target != "." {
		if _, isDog := IsDogTarget(target); !isDog {
			rigName, targetIsRig = IsRigName(target)
		}
	}

	// Resolve target agent and pane
	var targetAgent string
	var targetPane string
	var wispRootID string

	// For dry run, handle all target types and return early
	if slingDryRun {
		if target == "" {
			targetAgent, targetPane, _, err = resolveSelfTarget()
			if err != nil {
				return err
			}
		} else if target == "." {
			targetAgent, targetPane, _, err = resolveSelfTarget()
			if err != nil {
				return fmt.Errorf("resolving self for '.' target: %w", err)
			}
		} else if dogName, isDog := IsDogTarget(target); isDog {
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
		} else if targetIsRig {
			fmt.Printf("Would spawn fresh polecat in rig '%s'\n", rigName)
			targetAgent = fmt.Sprintf("%s/polecats/<new>", rigName)
			targetPane = "<new-pane>"
		} else {
			targetAgent, targetPane, _, err = resolveTargetAgent(target)
			if err != nil {
				return fmt.Errorf("resolving target: %w", err)
			}
		}
		fmt.Printf("%s Slinging formula %s to %s...\n", style.Bold.Render("🎯"), formulaName, targetAgent)
		fmt.Printf("Would cook formula: %s\n", formulaName)
		fmt.Printf("Would create wisp and pin to: %s\n", targetAgent)
		for _, v := range slingVars {
			fmt.Printf("  --var %s\n", v)
		}
		fmt.Printf("Would nudge pane: %s\n", targetPane)
		return nil
	}

	// For rig targets, create wisp BEFORE spawning polecat so we can pass HookBead
	if targetIsRig {
		fmt.Printf("%s Slinging formula %s to %s...\n", style.Bold.Render("🎯"), formulaName, rigName)

		// Step 1: Cook the formula (ensures proto exists)
		fmt.Printf("  Cooking formula...\n")
		cookArgs := []string{"--no-daemon", "cook", formulaName}
		cookCmd := exec.Command("bd", cookArgs...)
		cookCmd.Stderr = os.Stderr
		if err := cookCmd.Run(); err != nil {
			return fmt.Errorf("cooking formula: %w", err)
		}

		// Step 2: Create wisp instance BEFORE spawning polecat
		fmt.Printf("  Creating wisp...\n")
		wispArgs := []string{"--no-daemon", "mol", "wisp", formulaName}
		for _, v := range slingVars {
			wispArgs = append(wispArgs, "--var", v)
		}
		wispArgs = append(wispArgs, "--json")

		wispCmd := exec.Command("bd", wispArgs...)
		wispCmd.Stderr = os.Stderr
		wispOut, err := wispCmd.Output()
		if err != nil {
			return fmt.Errorf("creating wisp: %w", err)
		}

		wispRootID, err = parseWispIDFromJSON(wispOut)
		if err != nil {
			return fmt.Errorf("parsing wisp output: %w", err)
		}
		fmt.Printf("%s Wisp created: %s\n", style.Bold.Render("✓"), wispRootID)

		// Step 3: Spawn polecat WITH HookBead set atomically
		fmt.Printf("Target is rig '%s', spawning fresh polecat...\n", rigName)
		spawnOpts := SlingSpawnOptions{
			Force:    slingForce,
			Account:  slingAccount,
			Create:   slingCreate,
			Agent:    slingAgent,
			HookBead: wispRootID, // Set atomically at spawn time
		}
		spawnInfo, spawnErr := SpawnPolecatForSling(rigName, spawnOpts)
		if spawnErr != nil {
			return fmt.Errorf("spawning polecat: %w", spawnErr)
		}
		targetAgent = spawnInfo.AgentID()
		targetPane = spawnInfo.Pane

		// Wake witness and refinery to monitor the new polecat
		wakeRigAgents(rigName)
	} else {
		// Non-rig targets: resolve target first, then cook/wisp
		if target == "" {
			targetAgent, targetPane, _, err = resolveSelfTarget()
			if err != nil {
				return err
			}
		} else if target == "." {
			targetAgent, targetPane, _, err = resolveSelfTarget()
			if err != nil {
				return fmt.Errorf("resolving self for '.' target: %w", err)
			}
		} else if dogName, isDog := IsDogTarget(target); isDog {
			dispatchInfo, dispatchErr := DispatchToDog(dogName, slingCreate)
			if dispatchErr != nil {
				return fmt.Errorf("dispatching to dog: %w", dispatchErr)
			}
			targetAgent = dispatchInfo.AgentID
			targetPane = dispatchInfo.Pane
			fmt.Printf("Dispatched to dog %s\n", dispatchInfo.DogName)
		} else {
			// Slinging to an existing agent
			targetAgent, targetPane, _, err = resolveTargetAgent(target)
			if err != nil {
				return fmt.Errorf("resolving target: %w", err)
			}
		}

		fmt.Printf("%s Slinging formula %s to %s...\n", style.Bold.Render("🎯"), formulaName, targetAgent)

		// Step 1: Cook the formula (ensures proto exists)
		fmt.Printf("  Cooking formula...\n")
		cookArgs := []string{"--no-daemon", "cook", formulaName}
		cookCmd := exec.Command("bd", cookArgs...)
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
		wispCmd.Stderr = os.Stderr
		wispOut, err := wispCmd.Output()
		if err != nil {
			return fmt.Errorf("creating wisp: %w", err)
		}

		wispRootID, err = parseWispIDFromJSON(wispOut)
		if err != nil {
			return fmt.Errorf("parsing wisp output: %w", err)
		}
		fmt.Printf("%s Wisp created: %s\n", style.Bold.Render("✓"), wispRootID)
	}

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

	// Step 4: Nudge to start (graceful if no tmux)
	if targetPane == "" {
		fmt.Printf("%s No pane to nudge (agent will discover work via gt prime)\n", style.Dim.Render("○"))
		return nil
	}

	var prompt string
	if slingArgs != "" {
		prompt = fmt.Sprintf("Formula %s slung. Args: %s. Run `gt hook` to see your hook, then execute using these args.", formulaName, slingArgs)
	} else {
		prompt = fmt.Sprintf("Formula %s slung. Run `gt hook` to see your hook, then execute the steps.", formulaName)
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
