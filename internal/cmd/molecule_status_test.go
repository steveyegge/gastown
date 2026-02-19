package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRunMoleculeStatusTownBeadsFallback verifies that when a polecat has an hq-*
// bead hooked in town beads (not rig beads), runMoleculeStatus finds it via the
// town beads fallback.
//
// Repro for gt-jmbx: gt sling hq-<id> gastown -> polecat's gt mol status returns
// 'Nothing on hook' because the fallback only checked local (rig) beads.
func TestRunMoleculeStatusTownBeadsFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script stub not supported on windows")
	}

	// Set up temp workspace
	townRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	// Create mayor/town.json so workspace.FindFromCwd() can detect town root
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	// Create town-level .beads directory
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir town .beads: %v", err)
	}

	// Create routes.jsonl with hq- -> town and gt- -> gastown rig
	routesContent := `{"prefix":"hq-","path":"."}` + "\n" +
		`{"prefix":"gt-","path":"gastown/mayor/rig"}` + "\n"
	if err := os.WriteFile(filepath.Join(townBeadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Create gastown rig structure
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}
	rigBeadsDir := filepath.Join(rigDir, ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir rigBeadsDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "config.yaml"), []byte("prefix: gt\n"), 0644); err != nil {
		t.Fatalf("write rig config: %v", err)
	}

	// Create polecat worktree with redirect to rig/.beads
	polecatDir := filepath.Join(townRoot, "gastown", "polecats", "toast")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatalf("mkdir polecatDir: %v", err)
	}
	polecatBeadsDir := filepath.Join(polecatDir, ".beads")
	if err := os.MkdirAll(polecatBeadsDir, 0755); err != nil {
		t.Fatalf("mkdir polecatBeadsDir: %v", err)
	}
	// redirect: gastown/polecats/toast/.beads/redirect -> ../../mayor/rig/.beads
	if err := os.WriteFile(filepath.Join(polecatBeadsDir, "redirect"), []byte("../../mayor/rig/.beads"), 0644); err != nil {
		t.Fatalf("write redirect: %v", err)
	}

	// Create stub bin dir and bd script
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}

	// The hq- bead ID to hook to the polecat
	hookedBeadID := "hq-abc123"

	// Stub bd:
	// - show on agent bead -> not found (trigger FALLBACK path, no hook_bead field)
	// - list --status=hooked from rig beads -> empty (hq- bead not in rig)
	// - list --status=in_progress from rig beads -> empty
	// - list --status=hooked from TOWN beads -> returns the hq- bead
	bdScript := `#!/bin/sh
# Strip --allow-stale if present
if [ "$1" = "--allow-stale" ]; then
  shift
fi
cmd="$1"
shift

case "$cmd" in
  show)
    # Agent bead not found - triggers FALLBACK path
    echo "error: not found" >&2
    exit 1
    ;;
  list)
    # Check if querying town beads vs rig beads
    if echo "$BEADS_DIR" | grep -q "` + townRoot + `/.beads"; then
      # Town beads: return the hooked bead
      echo '[{"id":"` + hookedBeadID + `","title":"Cross-rig task","status":"hooked","assignee":"gastown/polecats/toast","description":"","type":"task","labels":[]}]'
    else
      # Rig beads: return empty
      echo '[]'
    fi
    ;;
  *)
    echo '[]'
    ;;
esac
exit 0
`
	stubPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(stubPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	// Set up env vars
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "polecat")
	t.Setenv("GT_POLECAT", "toast")
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_CREW", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("BEADS_DIR", "")

	// Run from polecat directory so workspace detection works
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("restore wd: %v", err)
		}
	})
	if err := os.Chdir(polecatDir); err != nil {
		t.Fatalf("chdir to polecat dir: %v", err)
	}

	// Capture output
	var captured strings.Builder
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	// Run molecule status
	runErr := runMoleculeStatus(nil, nil)

	w.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	captured.Write(buf[:n])

	if runErr != nil {
		t.Fatalf("runMoleculeStatus returned error: %v", runErr)
	}

	output := captured.String()
	// Should show the hooked bead, not "Nothing on hook"
	if strings.Contains(output, "Nothing on hook") {
		t.Errorf("expected town beads fallback to find hooked bead, but got 'Nothing on hook'\nOutput:\n%s", output)
	}
	if !strings.Contains(output, hookedBeadID) {
		t.Errorf("expected output to contain hooked bead ID %q\nOutput:\n%s", hookedBeadID, output)
	}
}
