package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/workspace"
)

// checkRigNotParkedOrDocked checks if a rig is parked or docked and returns
// an error if so. This prevents starting agents on rigs that have been
// intentionally taken offline.
func checkRigNotParkedOrDocked(rigName string) error {
	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	if IsRigParked(townRoot, rigName) {
		return fmt.Errorf("rig '%s' is parked - use 'gt rig unpark %s' first", rigName, rigName)
	}

	prefix := "gt"
	if r.Config != nil && r.Config.Prefix != "" {
		prefix = r.Config.Prefix
	}

	if IsRigDocked(townRoot, rigName, prefix) {
		return fmt.Errorf("rig '%s' is docked - use 'gt rig undock %s' first", rigName, rigName)
	}

	return nil
}

// getRig finds the town root and retrieves the specified rig.
// This is the common boilerplate extracted from get*Manager functions.
// Returns the town root path and rig instance.
func getRig(rigName string) (string, *rig.Rig, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigsConfigPath := constants.MayorRigsPath(townRoot)
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return "", nil, fmt.Errorf("rig '%s' not found", rigName)
	}

	return townRoot, r, nil
}

// hasRigBeadLabel checks if a rig's identity bead has a specific label.
// Returns false if the rig config or bead can't be loaded (safe default).
func hasRigBeadLabel(townRoot, rigName, label string) bool {
	rigPath := filepath.Join(townRoot, rigName)
	rigCfg, err := rig.LoadRigConfig(rigPath)
	if err != nil || rigCfg.Beads == nil {
		return false
	}

	beadsPath := filepath.Join(rigPath, "mayor", "rig")
	if _, err := os.Stat(beadsPath); err != nil {
		beadsPath = rigPath
	}

	bd := beads.New(beadsPath)
	rigBeadID := beads.RigBeadIDWithPrefix(rigCfg.Beads.Prefix, rigName)

	rigBead, err := bd.Show(rigBeadID)
	if err != nil {
		return false
	}

	for _, l := range rigBead.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// IsRigParkedOrDocked checks if a rig is parked or docked by any mechanism
// (wisp ephemeral state or persistent bead labels). Returns (blocked, reason).
// This is the single entry point for sling/dispatch to check rig availability.
func IsRigParkedOrDocked(townRoot, rigName string) (bool, string) {
	if IsRigParked(townRoot, rigName) {
		return true, "parked"
	}

	// Check docked via bead label (persistent)
	rigPath := filepath.Join(townRoot, rigName)
	rigCfg, err := rig.LoadRigConfig(rigPath)
	if err != nil || rigCfg.Beads == nil {
		return false, ""
	}

	if IsRigDocked(townRoot, rigName, rigCfg.Beads.Prefix) {
		return true, "docked"
	}

	return false, ""
}
