package doltserver

import (
	"bytes"
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

// FindRemote returns the name and URL of the first configured remote in a Dolt database.
// Returns ("", "", nil) if no remotes are configured.
func FindRemote(dbDir string) (name, url string, err error) {
	cmd := exec.Command("dolt", "remote", "-v")
	cmd.Dir = dbDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("dolt remote -v: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	// Parse output lines looking for any remote URL.
	// Dolt format: "origin https://doltremoteapi.dolthub.com/org/repo {}"
	// Git format:  "origin  https://... (push)"
	// Remote names may be "origin", "github", or any user-defined name.
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return parts[0], parts[1], nil
		}
	}

	return "", "", nil
}

// HasRemote checks whether a Dolt database directory has any remote configured.
// Returns the push URL if found, or empty string if no remote exists.
// Deprecated: use FindRemote for both the remote name and URL.
func HasRemote(dbDir string) (string, error) {
	_, url, err := FindRemote(dbDir)
	return url, err
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

// PushDatabase pushes a Dolt database directory to the specified remote's main branch.
// If force is true, uses --force.
func PushDatabase(dbDir, remote string, force bool) error {
	args := []string{"push", remote, "main"}
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

		// Check for remote (any name — "origin", "github", etc.)
		remoteName, remoteURL, err := FindRemote(dbDir)
		if err != nil {
			result.Error = fmt.Errorf("checking remote: %w", err)
			results = append(results, result)
			continue
		}
		result.Remote = remoteURL

		if remoteURL == "" {
			// Auto-setup DoltHub remote if credentials are available.
			token := DoltHubToken()
			org := DoltHubOrg()
			if token != "" && org != "" {
				if err := SetupDoltHubRemote(dbDir, org, db, token); err != nil {
					// Setup failed — skip this database for now.
					result.Error = fmt.Errorf("auto-setup DoltHub remote: %w", err)
					results = append(results, result)
					continue
				}
				// Remote is now configured; re-read it.
				remoteName, remoteURL, err = FindRemote(dbDir)
				if err != nil || remoteURL == "" {
					result.Error = fmt.Errorf("remote not found after auto-setup")
					results = append(results, result)
					continue
				}
				result.Remote = remoteURL
			} else {
				result.Skipped = true
				results = append(results, result)
				continue
			}
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
		if err := PushDatabase(dbDir, remoteName, opts.Force); err != nil {
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
	// Resolve the beads directory for this rig (read-only — never create dirs during purge)
	beadsDir := FindRigBeadsDir(townRoot, dbName)

	// Check that the beads directory actually exists on disk.
	// FindRigBeadsDir returns a path even for non-existent directories,
	// so we must verify existence explicitly.
	if _, err := os.Stat(beadsDir); err != nil {
		if os.IsNotExist(err) {
			return 0, nil // no beads dir — nothing to purge
		}
		return 0, fmt.Errorf("checking beads dir for %s: %w", dbName, err)
	}

	// Skip databases with uninitialized beads dirs (no metadata.json).
	// An empty .beads/ directory causes bd to attempt a fresh bootstrap,
	// which hangs waiting on dolt init or lock acquisition.
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	if info, err := os.Stat(metadataPath); err != nil {
		if os.IsNotExist(err) {
			return 0, nil // not initialized — nothing to purge
		}
		return 0, fmt.Errorf("checking metadata for %s: %w", dbName, err)
	} else if info.IsDir() {
		return 0, fmt.Errorf("metadata.json for %s is a directory", dbName)
	}

	// Build bd purge command with safety-net timeout.
	// bd purge v2 uses batched SQL (completes in seconds), but we keep a
	// generous timeout as a circuit breaker against future regressions.
	// --allow-stale prevents failures when database is temporarily stale,
	// consistent with all other bd invocations in the codebase.
	args := []string{"--allow-stale", "purge", "--json"}
	if dryRun {
		args = append(args, "--dry-run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.Dir = filepath.Dir(beadsDir) // run from parent of .beads
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return 0, fmt.Errorf("bd purge for %s: timed out after 60s", dbName)
	}
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		return 0, fmt.Errorf("bd purge for %s: %w (%s)", dbName, err, errMsg)
	}

	// Parse JSON output (from stdout only) to get purged count.
	// bd may emit non-JSON warning lines before the JSON object,
	// so extract the first JSON object from stdout.
	jsonBytes := extractJSON(stdout.Bytes())
	var result struct {
		PurgedCount *int `json:"purged_count"`
	}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return 0, fmt.Errorf("bd purge for %s: unexpected output format: %s", dbName, strings.TrimSpace(stdout.String()))
	}

	// Warn if purged_count field was missing from the JSON response — may indicate
	// a schema mismatch (e.g., field renamed). An explicit 0 is a valid success case.
	if result.PurgedCount == nil {
		fmt.Fprintf(os.Stderr, "Warning: bd purge for %s: purged_count field missing (raw: %s)\n", dbName, strings.TrimSpace(stdout.String()))
		return 0, nil
	}

	return *result.PurgedCount, nil
}

// extractJSON finds the first JSON object in raw output that may contain
// non-JSON preamble (warnings, debug lines). Returns data from the first '{' onward,
// letting json.Unmarshal handle end-detection (it stops at the end of the first valid
// JSON value and tolerates trailing content).
func extractJSON(data []byte) []byte {
	start := bytes.IndexByte(data, '{')
	if start < 0 {
		return data
	}
	return data[start:]
}
