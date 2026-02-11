package doltserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SyncOptions controls the behavior of SyncDatabases.
type SyncOptions struct {
	// Force enables --force on dolt push.
	Force bool

	// DryRun prints what would be pushed without actually pushing.
	DryRun bool

	// Filter restricts sync to a single database name. Empty means all.
	Filter string
}

// SyncResult records the outcome of syncing a single database.
type SyncResult struct {
	// Database is the rig database name.
	Database string

	// Pushed is true if dolt push succeeded.
	Pushed bool

	// Skipped is true if the database was skipped (e.g., no remote configured).
	Skipped bool

	// DryRun is true if this was a dry-run (no actual push).
	DryRun bool

	// Error is non-nil if the push failed.
	Error error

	// Remote is the origin push URL, or empty if none configured.
	Remote string
}

// HasRemote checks whether a Dolt database directory has an "origin" remote configured.
// Returns the push URL if found, or empty string if no origin remote exists.
func HasRemote(dbDir string) (string, error) {
	cmd := exec.Command("dolt", "remote", "-v")
	cmd.Dir = dbDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("dolt remote -v: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	// Parse output lines looking for origin remote URL.
	// Dolt format: "origin https://doltremoteapi.dolthub.com/org/repo {}"
	// Git format:  "origin  https://... (push)"
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "origin") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return parts[1], nil
		}
	}

	return "", nil
}

// CommitWorkingSet stages and commits any uncommitted changes in a Dolt database directory.
// Treats "nothing to commit" as success (not an error).
func CommitWorkingSet(dbDir string) error {
	// Stage all changes
	addCmd := exec.Command("dolt", "add", ".")
	addCmd.Dir = dbDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dolt add: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	// Commit (may fail with "nothing to commit" which is fine)
	commitCmd := exec.Command("dolt", "commit", "-m", "gt dolt sync: auto-commit working changes")
	commitCmd.Dir = dbDir
	output, err := commitCmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		// "nothing to commit" or "no changes added" is success — no changes to push
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "nothing to commit") || strings.Contains(lower, "no changes added") {
			return nil
		}
		return fmt.Errorf("dolt commit: %w (%s)", err, msg)
	}

	return nil
}

// PushDatabase pushes a Dolt database directory to origin main.
// If force is true, uses --force.
func PushDatabase(dbDir string, force bool) error {
	args := []string{"push", "origin", "main"}
	if force {
		args = append(args, "--force")
	}

	cmd := exec.Command("dolt", args...)
	cmd.Dir = dbDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dolt push: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// SyncDatabases iterates all databases (or a filtered subset), checks for remotes,
// commits working changes, and pushes to origin. Never fails fast — collects all results.
func SyncDatabases(townRoot string, opts SyncOptions) []SyncResult {
	databases, err := ListDatabases(townRoot)
	if err != nil {
		return []SyncResult{{
			Database: "(list)",
			Error:    fmt.Errorf("listing databases: %w", err),
		}}
	}

	var results []SyncResult

	for _, db := range databases {
		// Apply filter if set
		if opts.Filter != "" && db != opts.Filter {
			continue
		}

		dbDir := RigDatabaseDir(townRoot, db)
		result := SyncResult{Database: db}

		// Check for remote
		remote, err := HasRemote(dbDir)
		if err != nil {
			result.Error = fmt.Errorf("checking remote: %w", err)
			results = append(results, result)
			continue
		}
		result.Remote = remote

		if remote == "" {
			result.Skipped = true
			results = append(results, result)
			continue
		}

		if opts.DryRun {
			result.DryRun = true
			results = append(results, result)
			continue
		}

		// Commit working set
		if err := CommitWorkingSet(dbDir); err != nil {
			result.Error = fmt.Errorf("committing: %w", err)
			results = append(results, result)
			continue
		}

		// Push
		if err := PushDatabase(dbDir, opts.Force); err != nil {
			result.Error = err
			results = append(results, result)
			continue
		}

		result.Pushed = true
		results = append(results, result)
	}

	return results
}

// PurgeClosedEphemerals runs "bd purge" for a specific rig database to remove
// closed ephemeral beads (wisps, convoys) before pushing to DoltHub.
// Returns the number of beads purged and any error encountered.
// Errors are non-fatal — the caller should log them but continue with sync.
// Must be called while the Dolt server is still running (bd purge needs SQL access).
func PurgeClosedEphemerals(townRoot, dbName string, dryRun bool) (int, error) {
	// Resolve the beads directory for this rig
	beadsDir, err := FindOrCreateRigBeadsDir(townRoot, dbName)
	if err != nil {
		return 0, fmt.Errorf("resolving beads dir for %s: %w", dbName, err)
	}

	// Skip databases with uninitialized beads dirs (no metadata.json).
	// An empty .beads/ directory causes bd to attempt a fresh bootstrap,
	// which hangs waiting on dolt init or lock acquisition.
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		return 0, nil // not initialized — nothing to purge
	}

	// Build bd purge command with timeout to prevent one slow DB from stalling sync.
	args := []string{"purge", "--json"}
	if dryRun {
		args = append(args, "--dry-run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.Dir = filepath.Dir(beadsDir) // run from parent of .beads
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return 0, fmt.Errorf("bd purge for %s: timed out after 60s (possible lock contention)", dbName)
	}
	if err != nil {
		return 0, fmt.Errorf("bd purge for %s: %w (%s)", dbName, err, strings.TrimSpace(string(output)))
	}

	// Parse JSON output to get purged count
	var result struct {
		PurgedCount int `json:"purged_count"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		// bd purge succeeded but we couldn't parse output — not fatal
		return 0, nil
	}

	return result.PurgedCount, nil
}
