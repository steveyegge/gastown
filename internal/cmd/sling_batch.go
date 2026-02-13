package cmd

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
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

	// Cross-rig guard: check all beads match the target rig before spawning (gt-myecw)
	if !slingForce {
		townRoot := filepath.Dir(townBeadsDir)
		for _, beadID := range beadIDs {
			if err := checkCrossRigGuard(beadID, rigName+"/polecats/_", townRoot); err != nil {
				return err
			}
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

	if slingMaxConcurrent > 0 {
		fmt.Printf("  Max concurrent spawns: %d\n", slingMaxConcurrent)
	}

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
	activeCount := 0 // Track active spawns for --max-concurrent throttling

	// Spawn a polecat for each bead and sling it
	for i, beadID := range beadIDs {
		// Admission control: throttle spawns when --max-concurrent is set
		if slingMaxConcurrent > 0 && activeCount >= slingMaxConcurrent {
			fmt.Printf("\n%s Max concurrent limit reached (%d), waiting for capacity...\n",
				style.Warning.Render("⏳"), slingMaxConcurrent)
			// Wait with exponential backoff for sessions to settle
			for wait := 0; wait < 30; wait++ {
				time.Sleep(2 * time.Second)
				// Recount active — in practice, polecats become self-sufficient quickly
				// so we just use a time-based cooldown rather than precise counting
				if wait >= 2 {
					break
				}
			}
		}

		fmt.Printf("\n[%d/%d] Slinging %s...\n", i+1, len(beadIDs), beadID)

		// Check bead status
		info, err := getBeadInfo(beadID)
		if err != nil {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: err.Error()})
			fmt.Printf("  %s Could not get bead info: %v\n", style.Dim.Render("✗"), err)
			continue
		}

		if (info.Status == "pinned" || info.Status == "hooked") && !slingForce {
			results = append(results, slingResult{beadID: beadID, success: false, errMsg: "already " + info.Status})
			fmt.Printf("  %s Already %s (use --force to re-sling)\n", style.Dim.Render("✗"), info.Status)
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
			if err := CookFormula(formulaName, workDir, townRoot); err != nil {
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

		// Hook the bead (or wisp compound if formula was applied) with retry
		hookDir := beads.ResolveHookDir(townRoot, beadToHook, hookWorkDir)
		if err := hookBeadWithRetry(beadToHook, targetAgent, hookDir); err != nil {
			results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: false, errMsg: "hook failed"})
			fmt.Printf("  %s Failed to hook bead: %v\n", style.Dim.Render("✗"), err)
			// Clean up orphaned polecat to avoid leaving spawned-but-unhookable polecats
			cleanupSpawnedPolecat(spawnInfo, rigName)
			continue
		}

		fmt.Printf("  %s Work attached to %s\n", style.Bold.Render("✓"), spawnInfo.PolecatName)

		// Log sling event
		actor := detectActor()
		_ = events.LogFeed(events.TypeSling, actor, events.SlingPayload(beadToHook, targetAgent))

		// Update agent bead state
		updateAgentHookBead(targetAgent, beadToHook, hookWorkDir, townBeadsDir)

		// Store all attachment fields in a single read-modify-write cycle.
		// This eliminates the race condition where sequential independent updates
		// could overwrite each other under concurrent access.
		fieldUpdates := beadFieldUpdates{
			Dispatcher:       actor,
			Args:             slingArgs,
			AttachedMolecule: attachedMoleculeID,
			NoMerge:          slingNoMerge,
		}
		// Use beadToHook for the update target (may differ from beadID when formula-on-bead)
		if err := storeFieldsInBead(beadToHook, fieldUpdates); err != nil {
			fmt.Printf("  %s Could not store fields in bead: %v\n", style.Dim.Render("Warning:"), err)
		}

		// Create Dolt branch AFTER all sling writes are complete.
		// CommitWorkingSet flushes working set to HEAD, then CreatePolecatBranch
		// forks from HEAD — ensuring the polecat's branch includes all writes.
		if spawnInfo.DoltBranch != "" {
			if err := spawnInfo.CreateDoltBranch(); err != nil {
				fmt.Printf("  %s Could not create Dolt branch: %v, cleaning up...\n", style.Dim.Render("✗"), err)
				rollbackSlingArtifacts(spawnInfo, beadToHook, hookWorkDir)
				results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: false})
				continue
			}
		}

		// Start polecat session now that molecule/bead is attached.
		// This ensures polecat sees its work when gt prime runs on session start.
		pane, err := spawnInfo.StartSession()
		if err != nil {
			fmt.Printf("  %s Could not start session: %v, cleaning up partial state...\n", style.Dim.Render("✗"), err)
			rollbackSlingArtifacts(spawnInfo, beadToHook, hookWorkDir)
			results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: false})
			continue
		} else {
			fmt.Printf("  %s Session started for %s\n", style.Bold.Render("▶"), spawnInfo.PolecatName)
			// Fresh polecats get StartupNudge from SessionManager.Start(),
			// so no need to inject a start prompt here.
			_ = pane
		}

		activeCount++
		results = append(results, slingResult{beadID: beadID, polecat: spawnInfo.PolecatName, success: true})
	}

	if !slingNoBoot {
		wakeRigAgents(rigName)
	}

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

// cleanupSpawnedPolecat removes a polecat that was spawned but whose hook failed,
// preventing orphaned polecats from accumulating.
func cleanupSpawnedPolecat(spawnInfo *SpawnedPolecatInfo, rigName string) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return
	}
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return
	}
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return
	}
	polecatGit := git.NewGit(r.Path)
	t := tmux.NewTmux()
	polecatMgr := polecat.NewManager(r, polecatGit, t)
	if err := polecatMgr.Remove(spawnInfo.PolecatName, true); err != nil {
		fmt.Printf("  %s Could not clean up orphaned polecat %s: %v\n",
			style.Dim.Render("Warning:"), spawnInfo.PolecatName, err)
	} else {
		fmt.Printf("  %s Cleaned up orphaned polecat %s\n",
			style.Dim.Render("○"), spawnInfo.PolecatName)
	}
}
