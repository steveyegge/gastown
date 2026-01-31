package mail

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// bdError represents an error from running a bd command.
// It wraps the underlying error and includes the stderr output for inspection.
type bdError struct {
	Err    error
	Stderr string
}

// Error implements the error interface.
func (e *bdError) Error() string {
	if e.Stderr != "" {
		return e.Stderr
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "unknown bd error"
}

// Unwrap returns the underlying error for errors.Is/As compatibility.
func (e *bdError) Unwrap() error {
	return e.Err
}

// ContainsError checks if the stderr message contains the given substring.
func (e *bdError) ContainsError(substr string) bool {
	return strings.Contains(e.Stderr, substr)
}

// runBdCommand executes a bd command with proper environment setup.
// workDir is the directory to run the command in.
// beadsDir is the BEADS_DIR environment variable value.
// extraEnv contains additional environment variables to set (e.g., "BD_IDENTITY=...").
// Returns stdout bytes on success, or a *bdError on failure.
func runBdCommand(args []string, workDir, beadsDir string, extraEnv ...string) ([]byte, error) {
	cmd := exec.Command("bd", args...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = workDir

	env := append(os.Environ(), "BEADS_DIR="+beadsDir)
	env = append(env, extraEnv...)

	// Add dolt server env vars if migrated
	// Derive town root from beadsDir (beadsDir is under townRoot/.beads or townRoot/rig/.beads)
	townRoot := findTownRootFromBeadsDir(beadsDir)
	if townRoot != "" && config.IsDoltServerMode(townRoot) {
		env = append(env,
			"BEADS_DOLT_SERVER_MODE=1",
			"BEADS_DOLT_SERVER_HOST=127.0.0.1",
			fmt.Sprintf("BEADS_DOLT_SERVER_PORT=%d", config.DefaultDoltServerPort),
			"BEADS_DOLT_SERVER_DATABASE="+deriveDatabaseName(townRoot, beadsDir),
		)
	}
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, &bdError{
			Err:    err,
			Stderr: strings.TrimSpace(stderr.String()),
		}
	}

	return stdout.Bytes(), nil
}

// findTownRootFromBeadsDir finds town root from a beads directory path.
// beadsDir is typically like /path/to/town/.beads or /path/to/town/rig/.beads
func findTownRootFromBeadsDir(beadsDir string) string {
	// Walk up from beadsDir looking for mayor/town.json
	dir := filepath.Dir(beadsDir) // parent of .beads
	for dir != "/" && dir != "." {
		if _, err := os.Stat(filepath.Join(dir, "mayor", "town.json")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// deriveDatabaseName determines the dolt database name from beads path.
// Returns "hq" for town-level (.beads directly under town root) or rig name otherwise.
func deriveDatabaseName(townRoot, beadsDir string) string {
	relPath, err := filepath.Rel(townRoot, beadsDir)
	if err != nil || relPath == ".beads" {
		return "hq"
	}
	// For rig paths like "rigname/.beads", extract rig name
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) >= 1 && parts[0] != ".beads" {
		return parts[0]
	}
	return "hq"
}
