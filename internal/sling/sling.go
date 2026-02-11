package sling

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/bdcmd"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
)

// Sling hooks a bead to a target agent, optionally spawning a polecat.
// This replaces the exec.Command("gt", "sling", ...) pattern in the RPC server
// with direct function calls that return structured data.
func Sling(opts SlingOptions) (*SlingResult, error) {
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	townRoot := opts.TownRoot
	if townRoot == "" {
		return nil, fmt.Errorf("town_root is required")
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// Ensure beads.role=maintainer is set
	townGit := git.NewGit(townRoot)
	_ = townGit.SetConfig("beads.role", "maintainer")

	// Normalize target: trim trailing slashes
	target := strings.TrimRight(opts.Target, "/")
	beadID := opts.BeadID

	// Verify bead exists and check status
	if err := VerifyBeadExists(beadID, townRoot); err != nil {
		return nil, fmt.Errorf("bead not found: %w", err)
	}

	info, err := GetBeadInfo(beadID, townRoot)
	if err != nil {
		return nil, fmt.Errorf("checking bead status: %w", err)
	}
	if info.Status == "closed" {
		return nil, fmt.Errorf("bead %s is already closed", beadID)
	}
	if (info.Status == "pinned" || info.Status == "hooked") && !opts.Force {
		assignee := info.Assignee
		if assignee == "" {
			assignee = "(unknown)"
		}
		return nil, fmt.Errorf("bead %s is already %s to %s, use --force to re-sling", beadID, info.Status, assignee)
	}

	// Handle --force when bead is already hooked
	if info.Status == "hooked" && opts.Force && info.Assignee != "" {
		fmt.Fprintf(out, "Bead already hooked to %s, forcing reassignment...\n", info.Assignee)
		handleForceUnhook(beadID, info.Assignee, target, townRoot, out)
	}

	// Resolve target
	var targetAgent string
	var targetPane string
	var hookWorkDir string
	var hookSetAtomically bool
	var deferredRigName string
	var deferredSpawnOpts SpawnOptions
	var polecatSpawned bool
	var polecatName string
	var crewTargetRig, crewTargetName string

	if target != "" {
		if rig, name, ok := ParseCrewTarget(target); ok {
			crewTargetRig, crewTargetName = rig, name
		}

		if rigName, isRig := IsRigName(target, townRoot); isRig {
			fmt.Fprintf(out, "Target is rig '%s', will spawn polecat after validation...\n", rigName)
			deferredRigName = rigName
			deferredSpawnOpts = SpawnOptions{
				Force:           opts.Force,
				Account:         opts.Account,
				Create:          opts.Create,
				Agent:           opts.Agent,
				ExecutionTarget: opts.ExecutionTarget,
			}
			targetAgent = fmt.Sprintf("%s/polecats/<pending>", rigName)
		} else if _, isDog := IsDogTarget(target); isDog {
			// Dog dispatch - for now, build agent ID directly
			targetAgent = target
			if strings.HasPrefix(strings.ToLower(target), "dog:") {
				name := strings.TrimPrefix(strings.ToLower(target), "dog:")
				if name != "" {
					targetAgent = fmt.Sprintf("deacon/dogs/%s", name)
				} else {
					targetAgent = "deacon/dogs/<pool>"
				}
			}
		} else if _, _, ok := ParseCrewTarget(target); ok {
			// Crew target: build agent ID directly
			targetAgent = target
		} else if opts.ResolveTarget != nil {
			// Existing agent: use resolver
			var resolveWorkDir string
			targetAgent, targetPane, resolveWorkDir, err = opts.ResolveTarget(target)
			if err != nil {
				if IsPolecatTarget(target) {
					parts := strings.Split(target, "/")
					if len(parts) >= 3 && parts[1] == "polecats" {
						rigName := parts[0]
						deferredRigName = rigName
						deferredSpawnOpts = SpawnOptions{
							Force:           opts.Force,
							Account:         opts.Account,
							Create:          opts.Create,
							Agent:           opts.Agent,
							ExecutionTarget: opts.ExecutionTarget,
						}
						targetAgent = fmt.Sprintf("%s/polecats/<pending>", rigName)
					} else {
						return nil, fmt.Errorf("resolving target: %w", err)
					}
				} else {
					return nil, fmt.Errorf("resolving target: %w", err)
				}
			}
			if resolveWorkDir != "" {
				hookWorkDir = resolveWorkDir
			}
		} else {
			// No resolver available - use target as-is
			targetAgent = target
		}
	} else if opts.ResolveSelf != nil {
		var selfWorkDir string
		targetAgent, targetPane, selfWorkDir, err = opts.ResolveSelf()
		if err != nil {
			return nil, err
		}
		if selfWorkDir != "" {
			hookWorkDir = selfWorkDir
		}
	} else {
		return nil, fmt.Errorf("target is required (no self-resolution available)")
	}

	result := &SlingResult{
		BeadID:    beadID,
		BeadTitle: info.Title,
	}

	// Convoy handling
	convoyID, convoyCreated := handleConvoy(opts, beadID, info.Title, targetAgent, townRoot, out)
	result.ConvoyID = convoyID
	result.ConvoyCreated = convoyCreated

	// Auto-apply mol-polecat-work for polecat targets
	formulaName := ""
	attachedMoleculeID := ""
	if !opts.HookRawBead && strings.Contains(targetAgent, "/polecats/") {
		formulaName = "mol-polecat-work"
		fmt.Fprintf(out, "  Auto-applying %s for polecat work...\n", formulaName)
	}

	// Formula instantiation
	if formulaName != "" {
		fmt.Fprintf(out, "  Instantiating formula %s...\n", formulaName)
		fResult, fErr := InstantiateFormulaOnBead(formulaName, beadID, info.Title, hookWorkDir, townRoot, false, opts.Vars)
		if fErr != nil {
			return nil, fmt.Errorf("instantiating formula %s: %w", formulaName, fErr)
		}
		attachedMoleculeID = fResult.WispRootID
	}

	// Execute deferred polecat spawn
	if deferredRigName != "" {
		deferredSpawnOpts.HookBead = beadID

		fmt.Fprintf(out, "  Spawning polecat in %s...\n", deferredRigName)
		spawnInfo, spawnErr := SpawnPolecatForSling(deferredRigName, deferredSpawnOpts)
		if spawnErr != nil {
			return nil, fmt.Errorf("spawning polecat: %w", spawnErr)
		}
		targetAgent = spawnInfo.AgentID()
		hookWorkDir = spawnInfo.ClonePath
		hookSetAtomically = true
		polecatSpawned = true
		polecatName = spawnInfo.PolecatName

		if spawnInfo.K8sSpawn {
			result.K8sSpawn = true
		}

		WakeRigAgents(deferredRigName)
	}

	result.TargetAgent = targetAgent
	result.PolecatSpawned = polecatSpawned
	result.PolecatName = polecatName
	result.TargetPane = targetPane

	// Hook the bead
	if err := HookBead(beadID, targetAgent, townRoot, hookWorkDir, out); err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "Work attached to hook (status=hooked)\n")

	// Send crew mail notification
	if crewTargetName != "" {
		router := mail.NewRouter(townRoot)
		crewAddress := fmt.Sprintf("%s/crew/%s", crewTargetRig, crewTargetName)
		workMsg := &mail.Message{
			From:       "gt-sling",
			To:         crewAddress,
			Subject:    fmt.Sprintf("WORK: %s", info.Title),
			Body:       fmt.Sprintf("Bead: %s\nWork is on your hook.", beadID),
			Type:       mail.TypeTask,
			Priority:   mail.PriorityNormal,
			SkipNotify: true,
		}
		if sendErr := router.Send(workMsg); sendErr != nil {
			fmt.Fprintf(out, "Warning: could not send mail to crew: %v\n", sendErr)
		}
	}

	// Log event
	_ = events.LogFeed(events.TypeSling, "gt-rpc", events.SlingPayload(beadID, targetAgent))

	// Update agent hook bead
	if !hookSetAtomically {
		UpdateAgentHookBead(targetAgent, beadID, hookWorkDir, townBeadsDir)
	}

	// Store metadata
	storeMetadata(opts, beadID, attachedMoleculeID, out)

	return result, nil
}

// SlingFormula handles formula slinging (standalone formula or formula-on-bead).
func SlingFormula(opts FormulaOptions) (*FormulaResult, error) {
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	townRoot := opts.TownRoot
	if townRoot == "" {
		return nil, fmt.Errorf("town_root is required")
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")

	formula := opts.Formula
	target := strings.TrimRight(opts.Target, "/")

	// Verify formula exists
	if err := VerifyFormulaExists(formula); err != nil {
		return nil, err
	}

	// If formula-on-bead mode (OnBead is set)
	if opts.OnBead != "" {
		return slingFormulaOnBead(opts, out, townRoot, townBeadsDir)
	}

	// Standalone formula mode
	return slingStandaloneFormula(opts, out, townRoot, townBeadsDir, formula, target)
}

func slingFormulaOnBead(opts FormulaOptions, out io.Writer, townRoot, townBeadsDir string) (*FormulaResult, error) {
	beadID := opts.OnBead

	// Verify bead exists
	if err := VerifyBeadExists(beadID, townRoot); err != nil {
		return nil, err
	}

	info, err := GetBeadInfo(beadID, townRoot)
	if err != nil {
		return nil, err
	}

	// Resolve target
	var targetAgent, targetPane, hookWorkDir string
	target := strings.TrimRight(opts.Target, "/")
	var deferredRigName string
	var deferredSpawnOpts SpawnOptions
	var polecatSpawned bool
	var polecatNameResult string

	if target != "" {
		if rigName, isRig := IsRigName(target, townRoot); isRig {
			deferredRigName = rigName
			deferredSpawnOpts = SpawnOptions{
				Force:   opts.Force,
				Account: opts.Account,
				Create:  opts.Create,
				Agent:   opts.Agent,
			}
			targetAgent = fmt.Sprintf("%s/polecats/<pending>", rigName)
		} else if opts.ResolveTarget != nil {
			var resolveWorkDir string
			targetAgent, targetPane, resolveWorkDir, err = opts.ResolveTarget(target)
			if err != nil {
				return nil, fmt.Errorf("resolving target: %w", err)
			}
			if resolveWorkDir != "" {
				hookWorkDir = resolveWorkDir
			}
		} else {
			targetAgent = target
		}
	} else if opts.ResolveSelf != nil {
		var selfWorkDir string
		targetAgent, targetPane, selfWorkDir, err = opts.ResolveSelf()
		if err != nil {
			return nil, err
		}
		_ = selfWorkDir
	} else {
		return nil, fmt.Errorf("target is required")
	}

	// Instantiate formula on bead
	fResult, fErr := InstantiateFormulaOnBead(opts.Formula, beadID, info.Title, hookWorkDir, townRoot, false, opts.Vars)
	if fErr != nil {
		return nil, fmt.Errorf("instantiating formula: %w", fErr)
	}

	attachedMoleculeID := fResult.WispRootID

	// Deferred spawn after formula instantiation
	if deferredRigName != "" {
		deferredSpawnOpts.HookBead = beadID
		spawnInfo, spawnErr := SpawnPolecatForSling(deferredRigName, deferredSpawnOpts)
		if spawnErr != nil {
			return nil, fmt.Errorf("spawning polecat: %w", spawnErr)
		}
		targetAgent = spawnInfo.AgentID()
		hookWorkDir = spawnInfo.ClonePath
		polecatSpawned = true
		polecatNameResult = spawnInfo.PolecatName
		WakeRigAgents(deferredRigName)
	}

	// Hook the bead
	hookDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)
	_ = hookDir
	if err := HookBead(beadID, targetAgent, townRoot, hookWorkDir, out); err != nil {
		return nil, err
	}

	// Store metadata
	_ = events.LogFeed(events.TypeSling, "gt-rpc", events.SlingPayload(beadID, targetAgent))
	UpdateAgentHookBead(targetAgent, beadID, hookWorkDir, townBeadsDir)
	_ = StoreAttachedMoleculeInBead(beadID, attachedMoleculeID)

	if opts.Args != "" {
		_ = StoreArgsInBead(beadID, opts.Args)
	}

	return &FormulaResult{
		WispID:         fResult.WispRootID,
		TargetAgent:    targetAgent,
		BeadID:         beadID,
		PolecatSpawned: polecatSpawned,
		PolecatName:    polecatNameResult,
		TargetPane:     targetPane,
	}, nil
}

func slingStandaloneFormula(opts FormulaOptions, out io.Writer, townRoot, townBeadsDir, formula, target string) (*FormulaResult, error) {
	// Resolve target
	var targetAgent, targetPane string
	var deferredRigName string
	var deferredSpawnOpts SpawnOptions
	var polecatSpawned bool
	var polecatNameResult string
	var err error

	if target != "" {
		if rigName, isRig := IsRigName(target, townRoot); isRig {
			deferredRigName = rigName
			deferredSpawnOpts = SpawnOptions{
				Force:   opts.Force,
				Account: opts.Account,
				Create:  opts.Create,
				Agent:   opts.Agent,
			}
			targetAgent = fmt.Sprintf("%s/polecats/<pending>", rigName)
		} else if opts.ResolveTarget != nil {
			targetAgent, targetPane, _, err = opts.ResolveTarget(target)
			if err != nil {
				return nil, fmt.Errorf("resolving target: %w", err)
			}
		} else {
			targetAgent = target
		}
	} else if opts.ResolveSelf != nil {
		targetAgent, targetPane, _, err = opts.ResolveSelf()
		if err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("target is required")
	}

	// Cook formula
	cookErr := CookFormula(formula, townRoot)
	if cookErr != nil {
		return nil, fmt.Errorf("cooking formula: %w", cookErr)
	}

	// Create wisp
	wispArgs := []string{"mol", "wisp", formula}
	for _, v := range opts.Vars {
		wispArgs = append(wispArgs, "--var", v)
	}
	wispArgs = append(wispArgs, "--json")
	wispCmd := newBDCommand(wispArgs...)
	wispCmd.Stderr = os.Stderr
	wispOut, err := wispCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("creating wisp: %w", err)
	}

	wispRootID, err := parseWispIDFromJSON(wispOut)
	if err != nil {
		return nil, fmt.Errorf("parsing wisp output: %w", err)
	}

	// Deferred spawn after wisp creation
	if deferredRigName != "" {
		deferredSpawnOpts.HookBead = wispRootID
		spawnInfo, spawnErr := SpawnPolecatForSling(deferredRigName, deferredSpawnOpts)
		if spawnErr != nil {
			return nil, fmt.Errorf("spawning polecat: %w", spawnErr)
		}
		targetAgent = spawnInfo.AgentID()
		polecatSpawned = true
		polecatNameResult = spawnInfo.PolecatName
		WakeRigAgents(deferredRigName)
	}

	// Hook the wisp
	if err := HookBead(wispRootID, targetAgent, townRoot, "", out); err != nil {
		return nil, fmt.Errorf("hooking wisp bead: %w", err)
	}

	// Metadata
	_ = events.LogFeed(events.TypeSling, "gt-rpc", events.SlingPayload(wispRootID, targetAgent))
	UpdateAgentHookBead(targetAgent, wispRootID, "", townBeadsDir)
	_ = StoreDispatcherInBead(wispRootID, "gt-rpc")

	if opts.Args != "" {
		_ = StoreArgsInBead(wispRootID, opts.Args)
	}
	_ = StoreAttachedMoleculeInBead(wispRootID, wispRootID)

	return &FormulaResult{
		WispID:         wispRootID,
		TargetAgent:    targetAgent,
		PolecatSpawned: polecatSpawned,
		PolecatName:    polecatNameResult,
		TargetPane:     targetPane,
	}, nil
}

// SlingBatch slings multiple beads to a rig, each getting its own polecat.
func SlingBatch(opts BatchOptions) (*BatchResult, error) {
	out := opts.Output
	if out == nil {
		out = io.Discard
	}

	townRoot := opts.TownRoot
	if townRoot == "" {
		return nil, fmt.Errorf("town_root is required")
	}
	townBeadsDir := filepath.Join(townRoot, ".beads")

	// Ensure beads.role=maintainer
	townGit := git.NewGit(townRoot)
	_ = townGit.SetConfig("beads.role", "maintainer")

	// Validate all beads exist first
	for _, beadID := range opts.BeadIDs {
		if err := VerifyBeadExists(beadID, townRoot); err != nil {
			return nil, fmt.Errorf("bead '%s' not found", beadID)
		}
	}

	// Cook mol-polecat-work formula once
	formulaName := "mol-polecat-work"
	formulaCooked := false
	workDir := beads.ResolveHookDir(townRoot, opts.BeadIDs[0], "")
	if err := CookFormula(formulaName, workDir); err != nil {
		fmt.Fprintf(out, "Warning: could not cook formula: %v\n", err)
	} else {
		formulaCooked = true
	}

	result := &BatchResult{
		Results: make([]*BatchSlingResult, 0, len(opts.BeadIDs)),
	}

	for _, beadID := range opts.BeadIDs {
		bResult := &BatchSlingResult{
			BeadID:  beadID,
			Success: false,
		}

		info, err := GetBeadInfo(beadID, townRoot)
		if err != nil {
			bResult.Error = err.Error()
			result.Results = append(result.Results, bResult)
			result.FailureCount++
			continue
		}

		if info.Status == "pinned" && !opts.Force {
			bResult.Error = "already pinned"
			result.Results = append(result.Results, bResult)
			result.FailureCount++
			continue
		}

		// Spawn polecat
		spawnOpts := SpawnOptions{
			Force:    opts.Force,
			Account:  opts.Account,
			Create:   opts.Create,
			HookBead: beadID,
			Agent:    opts.Agent,
		}
		spawnInfo, err := SpawnPolecatForSling(opts.Rig, spawnOpts)
		if err != nil {
			bResult.Error = err.Error()
			result.Results = append(result.Results, bResult)
			result.FailureCount++
			continue
		}

		targetAgent := spawnInfo.AgentID()
		hookWorkDir := spawnInfo.ClonePath
		bResult.TargetAgent = targetAgent
		bResult.PolecatName = spawnInfo.PolecatName

		// Convoy handling
		if !opts.NoConvoy {
			if opts.Convoy != "" {
				_ = AddToConvoy(opts.Convoy, beadID, townRoot)
			} else {
				existingConvoy := IsTrackedByConvoy(beadID, townRoot)
				if existingConvoy == "" {
					convoyID, _ := CreateAutoConvoy(beadID, info.Title, targetAgent, townRoot, ConvoyOptions{
						Owned:         opts.Owned,
						MergeStrategy: opts.MergeStrategy,
					})
					if convoyID != "" && result.ConvoyID == "" {
						result.ConvoyID = convoyID
					}
				}
			}
		}

		// Apply formula
		beadToHook := beadID
		attachedMoleculeID := ""
		if formulaCooked {
			fResult, fErr := InstantiateFormulaOnBead(formulaName, beadID, info.Title, hookWorkDir, townRoot, true, opts.Vars)
			if fErr != nil {
				fmt.Fprintf(out, "Warning: could not apply formula: %v\n", fErr)
			} else {
				beadToHook = fResult.BeadToHook
				attachedMoleculeID = fResult.WispRootID
			}
		}

		// Hook
		if err := HookBead(beadToHook, targetAgent, townRoot, hookWorkDir, out); err != nil {
			bResult.Error = "hook failed"
			result.Results = append(result.Results, bResult)
			result.FailureCount++
			continue
		}

		// Metadata
		_ = events.LogFeed(events.TypeSling, "gt-rpc", events.SlingPayload(beadToHook, targetAgent))
		UpdateAgentHookBead(targetAgent, beadToHook, hookWorkDir, townBeadsDir)

		if attachedMoleculeID != "" {
			_ = StoreAttachedMoleculeInBead(beadToHook, attachedMoleculeID)
		}
		if opts.Args != "" {
			_ = StoreArgsInBead(beadID, opts.Args)
		}

		bResult.Success = true
		result.Results = append(result.Results, bResult)
		result.SuccessCount++
	}

	// Wake rig agents once at end
	WakeRigAgents(opts.Rig)

	return result, nil
}

func handleForceUnhook(beadID, oldAssignee, newTarget, townRoot string, out io.Writer) {
	parts := strings.Split(oldAssignee, "/")
	if len(parts) >= 3 && parts[1] == "polecats" {
		rigName := parts[0]
		polecatName := parts[2]

		router := mail.NewRouter(townRoot)
		shutdownMsg := &mail.Message{
			From:    "gt-sling",
			To:      fmt.Sprintf("%s/witness", rigName),
			Subject: fmt.Sprintf("LIFECYCLE:Shutdown %s", polecatName),
			Body:    fmt.Sprintf("Reason: work_reassigned\nBead: %s\nNewAssignee: %s", beadID, newTarget),
			Type:    mail.TypeTask,
		}
		if err := router.Send(shutdownMsg); err != nil {
			fmt.Fprintf(out, "Warning: could not send shutdown to witness: %v\n", err)
		}
	}

	// Unhook bead from old owner
	client := beads.New(beads.GetTownBeadsPath(townRoot))
	openStatus := "open"
	emptyAssignee := ""
	_ = client.Update(beadID, beads.UpdateOptions{
		Status:   &openStatus,
		Assignee: &emptyAssignee,
	})
}

func handleConvoy(opts SlingOptions, beadID, title, targetAgent, townRoot string, out io.Writer) (convoyID string, created bool) {
	if opts.NoConvoy {
		return "", false
	}

	if opts.Convoy != "" {
		if err := AddToConvoy(opts.Convoy, beadID, townRoot); err != nil {
			fmt.Fprintf(out, "Warning: could not add to convoy: %v\n", err)
		}
		return opts.Convoy, false
	}

	existingConvoy := IsTrackedByConvoy(beadID, townRoot)
	if existingConvoy != "" {
		return existingConvoy, false
	}

	id, err := CreateAutoConvoy(beadID, title, targetAgent, townRoot, ConvoyOptions{
		Owned:         opts.Owned,
		MergeStrategy: opts.MergeStrategy,
	})
	if err != nil {
		fmt.Fprintf(out, "Warning: could not create auto-convoy: %v\n", err)
		return "", false
	}
	return id, true
}

func storeMetadata(opts SlingOptions, beadID, attachedMoleculeID string, out io.Writer) {
	if err := StoreDispatcherInBead(beadID, "gt-rpc"); err != nil {
		fmt.Fprintf(out, "Warning: could not store dispatcher: %v\n", err)
	}

	if opts.Args != "" {
		if err := StoreArgsInBead(beadID, opts.Args); err != nil {
			fmt.Fprintf(out, "Warning: could not store args: %v\n", err)
		}
	}

	if opts.NoMerge {
		_ = StoreNoMergeInBead(beadID, true)
	}

	if opts.MergeStrategy != "" {
		_ = StoreMergeStrategyInBead(beadID, opts.MergeStrategy)
	}

	if opts.Owned {
		_ = StoreConvoyOwnedInBead(beadID, true)
	}

	if attachedMoleculeID != "" {
		_ = StoreAttachedMoleculeInBead(beadID, attachedMoleculeID)
	}
}

// newBDCommand creates an exec.Cmd for "bd" with the given args.
// Uses bdcmd.Command for proper daemon env propagation.
func newBDCommand(args ...string) *exec.Cmd {
	return bdcmd.Command(args...)
}
