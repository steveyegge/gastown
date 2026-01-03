// Package rig provides rig management functionality.
package rig

import (
	"github.com/steveyegge/gastown/internal/config"
)

// Rig represents a managed repository in the workspace.
type Rig struct {
	// Name is the rig identifier (directory name).
	Name string `json:"name"`

	// Path is the absolute path to the rig directory.
	Path string `json:"path"`

	// GitURL is the remote repository URL.
	GitURL string `json:"git_url"`

	// LocalRepo is an optional local repository used for reference clones.
	LocalRepo string `json:"local_repo,omitempty"`

	// Config is the rig-level configuration.
	Config *config.BeadsConfig `json:"config,omitempty"`

	// Polecats is the list of polecat names in this rig.
	Polecats []string `json:"polecats,omitempty"`

	// Crew is the list of crew worker names in this rig.
	// Crew workers are user-managed persistent workspaces.
	Crew []string `json:"crew,omitempty"`

	// HasWitness indicates if the rig has a witness agent.
	HasWitness bool `json:"has_witness"`

	// HasRefinery indicates if the rig has a refinery agent.
	HasRefinery bool `json:"has_refinery"`

	// HasMayor indicates if the rig has a mayor clone.
	HasMayor bool `json:"has_mayor"`
}

// AgentDirs are the standard agent directories in a rig.
// Note: witness doesn't have a /rig subdirectory (no clone needed).
var AgentDirs = []string{
	"polecats",
	"crew",
	"refinery/rig",
	"witness",
	"mayor/rig",
}

// RigSummary provides a concise overview of a rig.
type RigSummary struct {
	Name         string `json:"name"`
	PolecatCount int    `json:"polecat_count"`
	CrewCount    int    `json:"crew_count"`
	HasWitness   bool   `json:"has_witness"`
	HasRefinery  bool   `json:"has_refinery"`
}

// Summary returns a RigSummary for this rig.
func (r *Rig) Summary() RigSummary {
	return RigSummary{
		Name:         r.Name,
		PolecatCount: len(r.Polecats),
		CrewCount:    len(r.Crew),
		HasWitness:   r.HasWitness,
		HasRefinery:  r.HasRefinery,
	}
}

// BeadsPath returns the path to use for beads operations.
// Returns the mayor/rig clone path if available (has proper sync-branch config),
// otherwise falls back to the rig root path.
// This ensures beads commands read from a location with git-synced beads data.
func (r *Rig) BeadsPath() string {
	if r.HasMayor {
		return r.Path + "/mayor/rig"
	}
	return r.Path
}
