package cmd

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/workspace"
)

// getRig finds the town root and retrieves the specified rig.
// This is the common boilerplate extracted from get*Manager functions.
// Returns the town root path and rig instance.
//
// Resolution order:
//  1. Rig registry config beads (daemon, if connected)
//  2. Filesystem fallback (mayor/rigs.json)
func getRig(rigName string) (string, *rig.Rig, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return "", nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigsConfig, err := loadRigsConfigBeadsFirst(townRoot)
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

// loadRigsConfigBeadsFirst loads rig registry from config beads first,
// falling back to the filesystem rigs.json if the daemon is unreachable.
func loadRigsConfigBeadsFirst(townRoot string) (*config.RigsConfig, error) {
	// Try to fetch rig registry config beads from the daemon.
	var entries []config.RigBeadEntry
	bd := beads.New(townRoot)
	issues, err := bd.ListConfigBeadsByCategory(beads.ConfigCategoryRigRegistry)
	if err == nil && len(issues) > 0 {
		entries = make([]config.RigBeadEntry, 0, len(issues))
		for _, issue := range issues {
			fields := beads.ParseConfigFields(issue.Description)
			entries = append(entries, config.RigBeadEntry{
				BeadID:   issue.ID,
				Metadata: fields.Metadata,
			})
		}
	}

	// Load town name for bead ID parsing.
	townName := ""
	townCfg, err := config.LoadTownConfig(constants.MayorTownPath(townRoot))
	if err == nil {
		townName = townCfg.Name
	}

	return config.LoadRigsConfigWithFallback(entries, townName, townRoot)
}
