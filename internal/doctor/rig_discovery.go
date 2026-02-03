package doctor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// RigSource indicates where a rig was discovered from.
type RigSource int

const (
	// SourceRigsJSON indicates the rig was found in mayor/rigs.json
	SourceRigsJSON RigSource = 1 << iota
	// SourceRoutesJSONL indicates the rig was found in routes.jsonl
	SourceRoutesJSONL
	// SourceBeadsDir indicates the rig was found by directory scan (.beads dir)
	SourceBeadsDir
	// SourceRigJSON indicates the rig was found by directory scan (rig.json file)
	SourceRigJSON
)

// DiscoveredRig represents a rig found during discovery, tracking its sources.
type DiscoveredRig struct {
	Name    string
	Path    string
	Sources RigSource
}

// HasSource checks if the rig was discovered from a specific source.
func (r *DiscoveredRig) HasSource(s RigSource) bool {
	return r.Sources&s != 0
}

// SourceCount returns the number of sources that found this rig.
func (r *DiscoveredRig) SourceCount() int {
	count := 0
	for _, s := range []RigSource{SourceRigsJSON, SourceRoutesJSONL, SourceBeadsDir, SourceRigJSON} {
		if r.HasSource(s) {
			count++
		}
	}
	return count
}

// SourceNames returns human-readable names of all sources.
func (r *DiscoveredRig) SourceNames() []string {
	var names []string
	if r.HasSource(SourceRigsJSON) {
		names = append(names, "rigs.json")
	}
	if r.HasSource(SourceRoutesJSONL) {
		names = append(names, "routes.jsonl")
	}
	if r.HasSource(SourceBeadsDir) {
		names = append(names, ".beads directory")
	}
	if r.HasSource(SourceRigJSON) {
		names = append(names, "rig.json")
	}
	return names
}

// RigDiscoveryResult contains the results of rig discovery.
type RigDiscoveryResult struct {
	// Rigs is a map of rig name to discovered rig info
	Rigs map[string]*DiscoveredRig
	// Conflicts lists rigs found in some sources but not others
	Conflicts []string
}

// RigNames returns a sorted list of all discovered rig names.
func (r *RigDiscoveryResult) RigNames() []string {
	names := make([]string, 0, len(r.Rigs))
	for name := range r.Rigs {
		names = append(names, name)
	}
	return names
}

// RigPaths returns all discovered rig paths.
func (r *RigDiscoveryResult) RigPaths() []string {
	paths := make([]string, 0, len(r.Rigs))
	for _, rig := range r.Rigs {
		paths = append(paths, rig.Path)
	}
	return paths
}

// DiscoverRigs finds all rigs in a town using multiple sources and reports conflicts.
// Sources checked:
//  1. rigs.json registry (mayor/rigs.json)
//  2. routes.jsonl path extraction (.beads/routes.jsonl)
//  3. Directory scan for .beads subdirectories
//  4. Directory scan for rig.json files
func DiscoverRigs(townRoot string) *RigDiscoveryResult {
	result := &RigDiscoveryResult{
		Rigs: make(map[string]*DiscoveredRig),
	}

	// Helper to add or update a rig
	addRig := func(name, path string, source RigSource) {
		if name == "" || name == "mayor" || name == ".beads" || name == ".git" {
			return
		}
		if rig, exists := result.Rigs[name]; exists {
			rig.Sources |= source
		} else {
			result.Rigs[name] = &DiscoveredRig{
				Name:    name,
				Path:    path,
				Sources: source,
			}
		}
	}

	// Source 1: rigs.json registry
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	if rigsConfig, err := config.LoadRigsConfig(rigsPath); err == nil {
		for rigName := range rigsConfig.Rigs {
			rigPath := filepath.Join(townRoot, rigName)
			if _, err := os.Stat(rigPath); err == nil {
				addRig(rigName, rigPath, SourceRigsJSON)
			}
		}
	}

	// Source 2: routes.jsonl (extract rig names from paths)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if routes, err := beads.LoadRoutes(townBeadsDir); err == nil {
		for _, route := range routes {
			if route.Path == "." || route.Path == "" {
				continue // Skip town root
			}
			// Extract rig name (first path component)
			parts := strings.Split(route.Path, "/")
			if len(parts) > 0 && parts[0] != "" {
				rigName := parts[0]
				rigPath := filepath.Join(townRoot, rigName)
				if _, err := os.Stat(rigPath); err == nil {
					addRig(rigName, rigPath, SourceRoutesJSONL)
				}
			}
		}
	}

	// Source 3: Directory scan for .beads subdirectories
	entries, err := os.ReadDir(townRoot)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == "mayor" || name == ".beads" || name == ".git" || strings.HasPrefix(name, ".") {
				continue
			}
			rigPath := filepath.Join(townRoot, name)
			beadsDir := filepath.Join(rigPath, ".beads")
			if _, err := os.Stat(beadsDir); err == nil {
				addRig(name, rigPath, SourceBeadsDir)
			}
		}
	}

	// Source 4: Directory scan for rig.json files
	if entries == nil {
		entries, _ = os.ReadDir(townRoot)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "mayor" || name == ".beads" || name == ".git" || strings.HasPrefix(name, ".") {
			continue
		}
		rigPath := filepath.Join(townRoot, name)
		rigJSON := filepath.Join(rigPath, "rig.json")
		if _, err := os.Stat(rigJSON); err == nil {
			addRig(name, rigPath, SourceRigJSON)
		}
	}

	// Detect conflicts: rigs found in some sources but not all
	for name, rig := range result.Rigs {
		// A "conflict" is when a rig is found by some methods but not rigs.json
		// (the authoritative source)
		if !rig.HasSource(SourceRigsJSON) && rig.SourceCount() > 0 {
			result.Conflicts = append(result.Conflicts, name)
		}
	}

	return result
}

// DiscoverRigPaths is a convenience function that returns just the rig directory paths.
// This is for backward compatibility with existing code that expects []string.
func DiscoverRigPaths(townRoot string) []string {
	result := DiscoverRigs(townRoot)
	return result.RigPaths()
}
