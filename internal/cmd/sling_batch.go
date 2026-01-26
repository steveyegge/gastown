package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
)

// runBatchSling handles slinging multiple beads to a rig.
// Each bead gets its own freshly spawned polecat.
func runBatchSling(beadIDs []string, rigName string, townBeadsDir string) error {
	// Validate all beads exist before spawning any polecats
	for _, beadID := range beadIDs {
		if err := verifyBeadExists(beadID); err != nil {
			return fmt.Errorf("bead '%s' not found", beadID)
		}
	}

	if slingDryRun {
		fmt.Printf("%s Batch slinging %d beads to rig '%s':\n", style.Bold.Render("🎯"), len(beadIDs), rigName)
		fmt.Printf("  Would cook mol-polecat-work formula once\n")
		for _, beadID := range beadIDs {
			fmt.Printf("  Would spawn polecat and apply mol-polecat-work to: %s\n", beadID)
		}
		return nil
	}

	fmt.Printf("%s Batch slinging %d beads to rig '%s'...\n", style.Bold.Render("🎯"), len(beadIDs), rigName)

	// Issue #288: Auto-apply mol-polecat-work for batch sling
	// Cook once before the loop for efficiency
	townRoot := filepath.Dir(townBeadsDir)
	formulaName := "mol-polecat-work"
	formulaCooked := false

	// Track results for summary
	type slingResult struct {
		beadID  string
		polecat string
		success bool
		errMsg  string
	}
	results := make([]slingResult, 0, len(beadIDs))

	// Spawn a polecat for each bead and sling it
	for i, beadID := range beadIDs {
		fmt.Printf("\n[%d/%d] Slinging %s...\n", i+1, len(beadIDs), beadID)

		// Check bead status
		info, err := getBeadInfo(beadID)
		if err != nil {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: err.Error()})
			fmt.Printf("  %s Could not get bead info: %v\n", style.Dim.Render("✗"), err)
			continue
		}

		if info.Status == "pinned" && !slingForce {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: "already pinned"})
			fmt.Printf("  %s Already pinned (use --force to re-sling)\n", style.Dim.Render("✗"))
			continue
		}

		// Spawn a fresh polecat
		spawnOpts := SlingSpawnOptions{
			Force:    slingForce,
			Account:  slingAccount,
			Create:   slingCreate,
			HookBead: beadID, // Set atomically at spawn time
			Agent:    slingAgent,
		}
		spawnInfo, err := SpawnPolecatForSling(rigName, spawnOpts)
		if err != nil {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: err.Error()})
			fmt.Printf("  %s Failed to spawn polecat: %v\n", style.Dim.Render("✗"), err)
			continue
		}

		targetAgent := spawnInfo.AgentID()
		hookWorkDir := spawnInfo.ClonePath

		// Auto-convoy: check if issue is already tracked
		if !slingNoConvoy {
			existingConvoy := isTrackedByConvoy(beadID)
			if existingConvoy == "" {
				convoyID, err := createAutoConvoy(beadID, info.Title)
				if err != nil {
					fmt.Printf("  %s Could not create auto-convoy: %v\n", style.Dim.Render("Warning:"), err)
				} else {
					fmt.Printf("  %s Created convoy 🚚 %s\n", style.Bold.Render("→"), convoyID)
				}
			} else {
				fmt.Printf("  %s Already tracked by convoy %s\n", style.Dim.Render("○"), existingConvoy)
			}
		}

		// Issue #288: Apply mol-polecat-work via formula-on-bead pattern
		// Cook once (lazy), then instantiate for each bead
		if !formulaCooked {
			workDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)
			if err := CookFormula(formulaName, workDir); err != nil {
				fmt.Printf("  %s Could not cook formula %s: %v\n", style.Dim.Render("Warning:"), formulaName, err)
				// Fall back to raw hook if formula cook fails
			} else {
				formulaCooked = true
			}
		}

		beadToHook := beadID
		attachedMoleculeID := ""
		if formulaCooked {
			result, err := InstantiateFormulaOnBead(formulaName, beadID, info.Title, hookWorkDir, townRoot, true, slingVars)
			if err != nil {
				fmt.Printf("  %s Could not apply formula: %v (hooking raw bead)\n", style.Dim.Render("Warning:"), err)
			} else {
				fmt.Printf("  %s Formula %s applied\n", style.Bold.Render("✓"), formulaName)
				beadToHook = result.BeadToHook
				attachedMoleculeID = result.WispRootID
			}
		}

		// Hook the bead (or wisp compound if formula was applied)
		hookCmd := exec.Command("bd", "--no-daemon", "update", beadToHook, "--status=hooked", "--assignee="+targetAgent)
		hookCmd.Dir = beads.ResolveHookDir(townRoot, beadToHook, hookWorkDir)
		hookCmd.Stderr = os.Stderr
		if err := hookCmd.Run(); err != nil {
			results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: false, errMsg: "hook failed"})
			fmt.Printf("  %s Failed to hook bead: %v\n", style.Dim.Render("✗"), err)
			continue
		}

		fmt.Printf("  %s Work attached to %s\n", style.Bold.Render("✓"), spawnInfo.PolecatName)

		// Log sling event
		actor := detectActor()
		_ = events.LogFeed(events.TypeSling, actor, events.SlingPayload(beadToHook, targetAgent))

		// Update agent bead state
		updateAgentHookBead(targetAgent, beadToHook, hookWorkDir, townBeadsDir)

		// Store attached molecule in the hooked bead
		if attachedMoleculeID != "" {
			if err := storeAttachedMoleculeInBead(beadToHook, attachedMoleculeID); err != nil {
				fmt.Printf("  %s Could not store attached_molecule: %v\n", style.Dim.Render("Warning:"), err)
			}
		}

		// Store args if provided
		if slingArgs != "" {
			if err := storeArgsInBead(beadID, slingArgs); err != nil {
				fmt.Printf("  %s Could not store args: %v\n", style.Dim.Render("Warning:"), err)
			}
		}

		// Nudge the polecat
		if spawnInfo.Pane != "" {
			if err := injectStartPrompt(spawnInfo.Pane, beadID, slingSubject, slingArgs); err != nil {
				fmt.Printf("  %s Could not nudge (agent will discover via gt prime)\n", style.Dim.Render("○"))
			} else {
				fmt.Printf("  %s Start prompt sent\n", style.Bold.Render("▶"))
			}
		}

		results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: true})
	}

	// Wake witness and refinery once at the end
	wakeRigAgents(rigName)

	// Print summary
	successCount := 0
	for _, r := range results {
		if r.success {
			successCount++
		}
	}

	fmt.Printf("\n%s Batch sling complete: %d/%d succeeded\n", style.Bold.Render("📊"), successCount, len(beadIDs))
	if successCount < len(beadIDs) {
		for _, r := range results {
			if !r.success {
				fmt.Printf("  %s %s: %s\n", style.Dim.Render("✗"), r.beadID, r.errMsg)
			}
		}
	}

	return nil
}
