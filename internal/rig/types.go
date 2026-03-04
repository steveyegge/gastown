// Package rig provides rig management functionality.
package rig

import (
	"os"
	"path/filepath"

	"github.com/steveyegge/gastown/internal/config"
)

// Rig represents a managed repository in the workspace.
type Rig struct {
	// Name is the rig identifier (directory name).
	Name string `json:"name"`

	// Path is the absolute path to the rig directory.
	Path string `json:"path"`

	// GitURL is the remote repository URL (fetch/pull).
	GitURL string `json:"git_url"`

	// PushURL is an optional push URL for read-only upstreams.
	// When set, polecats push here instead of to GitURL (e.g., personal fork).
	PushURL string `json:"push_url,omitempty"`

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
// Checks mayor/rig/ first (where .beads/ with routes and config lives
// in rigs that track .beads/ in their repo), then falls back to the
// rig root path. This matches the resolution logic in rig_helpers.go.
func (r *Rig) BeadsPath() string {
	mayorRig := filepath.Join(r.Path, "mayor", "rig")
	if info, err := os.Stat(mayorRig); err == nil && info.IsDir() {
		beadsDir := filepath.Join(mayorRig, ".beads")
		if _, err := os.Stat(beadsDir); err == nil {
			return mayorRig
		}
	}
	return r.Path
}

// DefaultBranch returns the configured default branch for this rig.
// Falls back to "main" if not configured or if config cannot be loaded.
func (r *Rig) DefaultBranch() string {
	cfg, err := LoadRigConfig(r.Path)
	if err != nil || cfg.DefaultBranch == "" {
		return "main"
	}
	return cfg.DefaultBranch
}
