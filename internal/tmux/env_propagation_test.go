package tmux

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// readIfExists reads path. If the file does not exist yet, it returns
// (nil, fs.ErrNotExist) so callers can distinguish "not yet written" from
// "wrote empty content".
func readIfExists(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil && errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return data, err
}

// TestNewSessionWithCommandAndEnv_SubprocessInheritsEnv verifies the gt-neycp
// fix: env vars passed via tmux -e flags reach SUBPROCESSES spawned inside the
// pane, not just the initial shell. This is the contract bd subprocess of
// Claude depends on — without it, bd auto-discovers a per-rig embedded Dolt
// instead of connecting to the central server.
//
// The pane runs a shell that spawns a child shell which writes the value of
// BEADS_DOLT_PORT (as seen by the child) to a file. We then verify the file
// contains the expected port. Subprocess inheritance is what matters here.
func TestNewSessionWithCommandAndEnv_SubprocessInheritsEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell command; skipping on Windows")
	}

	tm := newTestTmux(t)
	sessionName := "gt-test-env-prop-" + t.Name()
	_ = tm.KillSession(sessionName)
	defer func() { _ = tm.KillSession(sessionName) }()

	workDir := t.TempDir()
	outFile := filepath.Join(workDir, "subprocess_env.txt")

	// Outer shell is what tmux spawns. It then forks a child shell that prints
	// BEADS_DOLT_PORT (its inherited env) to outFile. The variable expansion
	// happens in the outer shell, so the value reflects the outer shell's env
	// — which must come from the tmux session env (-e flags).
	cmd := `sh -c 'sh -c "printf %s \"$BEADS_DOLT_PORT\" > ` + outFile + `"; sleep 30'`

	envVars := map[string]string{
		"BEADS_DOLT_PORT":       "3307",
		"BEADS_DOLT_AUTO_START": "0",
		"GT_ROLE":               "witness",
	}

	if err := tm.NewSessionWithCommandAndEnv(sessionName, workDir, cmd, envVars); err != nil {
		t.Fatalf("NewSessionWithCommandAndEnv: %v", err)
	}

	// Wait for subprocess to write the file.
	deadline := time.Now().Add(3 * time.Second)
	var got []byte
	for time.Now().Before(deadline) {
		data, err := readIfExists(outFile)
		if err == nil && len(data) > 0 {
			got = data
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if strings.TrimSpace(string(got)) != "3307" {
		t.Errorf("subprocess BEADS_DOLT_PORT = %q, want %q (env did not propagate to pane subprocess)", string(got), "3307")
	}
}
