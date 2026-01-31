// Package beads provides custom type management for agent beads.
package beads

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/steveyegge/gastown/internal/constants"
)

// typesSentinel is a marker file indicating custom types have been configured.
// This persists across CLI invocations to avoid redundant bd config calls.
const typesSentinel = ".gt-types-configured"

// ensuredDirs tracks which beads directories have been ensured this session.
// This provides fast in-memory caching for multiple creates in the same CLI run.
var (
	ensuredDirs = make(map[string]bool)
	ensuredMu   sync.Mutex
)

// FindTownRoot walks up from startDir to find the Gas Town root directory.
// The town root is identified by the presence of mayor/town.json.
// Returns empty string if not found (reached filesystem root).
func FindTownRoot(startDir string) string {
	dir := startDir
	for {
		townFile := filepath.Join(dir, "mayor", "town.json")
		if _, err := os.Stat(townFile); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // Reached filesystem root
		}
		dir = parent
	}
}

// ResolveRoutingTarget determines which beads directory a bead ID will route to.
// It extracts the prefix from the bead ID and looks up the corresponding route.
// Returns the resolved beads directory path, following any redirects.
//
// If townRoot is empty or prefix is not found, falls back to the provided fallbackDir.
func ResolveRoutingTarget(townRoot, beadID, fallbackDir string) string {
	if townRoot == "" {
		return fallbackDir
	}

	// Extract prefix from bead ID (e.g., "gt-gastown-polecat-Toast" -> "gt-")
	prefix := ExtractPrefix(beadID)
	if prefix == "" {
		return fallbackDir
	}

	// Look up rig path for this prefix
	rigPath := GetRigPathForPrefix(townRoot, prefix)
	if rigPath == "" {
		return fallbackDir
	}

	// Resolve redirects and get final beads directory
	beadsDir := ResolveBeadsDir(rigPath)
	if beadsDir == "" {
		return fallbackDir
	}

	return beadsDir
}

// EnsureCustomTypes ensures the target beads directory has custom types configured.
// Uses a two-level caching strategy:
//   - In-memory cache for multiple creates in the same CLI invocation
//   - Sentinel file on disk for persistence across CLI invocations
//
// This function is thread-safe and idempotent.
func EnsureCustomTypes(beadsDir string) error {
	if beadsDir == "" {
		return fmt.Errorf("empty beads directory")
	}

	ensuredMu.Lock()
	defer ensuredMu.Unlock()

	// Fast path: in-memory cache (same CLI invocation)
	if ensuredDirs[beadsDir] {
		return nil
	}

	// Fast path: sentinel file exists (previous CLI invocation)
	sentinelPath := filepath.Join(beadsDir, typesSentinel)
	if _, err := os.Stat(sentinelPath); err == nil {
		ensuredDirs[beadsDir] = true
		return nil
	}

	// Verify beads directory exists
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return fmt.Errorf("beads directory does not exist: %s", beadsDir)
	}

	// Configure custom types via bd CLI
	typesList := strings.Join(constants.BeadsCustomTypesList(), ",")
	cmd := exec.Command(resolvedBdPath, "config", "set", "types.custom", typesList)
	cmd.Dir = beadsDir
	// Set BEADS_DIR explicitly to ensure bd operates on the correct database
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("configure custom types in %s: %s: %w",
			beadsDir, strings.TrimSpace(string(output)), err)
	}

	// Write sentinel file (best effort - don't fail if this fails)
	// The sentinel contains a version marker for future compatibility
	_ = os.WriteFile(sentinelPath, []byte("v1\n"), 0644)

	ensuredDirs[beadsDir] = true
	return nil
}

// ResetEnsuredDirs clears the in-memory cache of ensured directories.
// This is primarily useful for testing.
func ResetEnsuredDirs() {
	ensuredMu.Lock()
	defer ensuredMu.Unlock()
	ensuredDirs = make(map[string]bool)
}
