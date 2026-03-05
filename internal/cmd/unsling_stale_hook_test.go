package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
)

func TestRunUnsling_ClearsAgentSlotWhenStaleHookOnlyInWisps(t *testing.T) {
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	crewDir := filepath.Join(townRoot, "barnaby", "crew", "bob")
	rigDir := filepath.Join(townRoot, "barnaby", "mayor", "rig")
	rigBeadsDir := filepath.Join(rigDir, ".beads")

	for _, dir := range []string{
		filepath.Join(townRoot, "mayor"),
		crewDir,
		rigBeadsDir,
		filepath.Join(townRoot, ".beads"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	if err := beads.WriteRoutes(
		filepath.Join(townRoot, ".beads"),
		[]beads.Route{
			{Prefix: "hq-", Path: "."},
			{Prefix: "ba-", Path: "barnaby/mayor/rig"},
		},
	); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	agentBeadID := "ba-barnaby-crew-bob"
	staleHookID := "ba-closed-123"
	slotState := filepath.Join(tmpDir, "slot_state.txt")
	if err := os.WriteFile(slotState, []byte(staleHookID), 0644); err != nil {
		t.Fatalf("write slot state: %v", err)
	}

	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	bdStub := filepath.Join(binDir, "bd")
	stubScript := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
STATE_FILE=%q
AGENT_ID=%q

# Skip global flags (e.g., --allow-stale, --json).
while [[ $# -gt 0 ]]; do
  case "$1" in
    --*) shift ;;
    *) break ;;
  esac
done

cmd="${1:-}"
shift || true

case "$cmd" in
  show)
    id="${1:-}"
    if [[ "$id" == "$AGENT_ID" ]]; then
      # Repro shape: bd show does NOT surface hook_bead from wisps slot.
      echo '[{"id":"'"$AGENT_ID"'","title":"Crew bob","status":"open","issue_type":"agent","hook_bead":""}]'
      exit 0
    fi
    echo '[]'
    exit 0
    ;;
  list)
    # Repro shape: no status=hooked bead visible for this assignee.
    echo '[]'
    exit 0
    ;;
  slot)
    sub="${1:-}"
    agent="${2:-}"
    slot="${3:-}"
    if [[ "$sub" == "clear" && "$agent" == "$AGENT_ID" && "$slot" == "hook" ]]; then
      : > "$STATE_FILE"
      echo "ok"
      exit 0
    fi
    echo "unsupported slot command: $sub $agent $slot" >&2
    exit 1
    ;;
  sql)
    hook="$(cat "$STATE_FILE" 2>/dev/null || true)"
    echo '[{"id":"'"$AGENT_ID"'","hook_bead":"'"$hook"'"}]'
    exit 0
    ;;
  *)
    # no-op for unrelated calls.
    echo '[]'
    exit 0
    ;;
esac
`, slotState, agentBeadID)
	if err := os.WriteFile(bdStub, []byte(stubScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(crewDir); err != nil {
		t.Fatalf("chdir crew dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldCwd) })

	before, err := readHookSlotViaBD(rigDir, agentBeadID)
	if err != nil {
		t.Fatalf("read hook slot before unsling: %v", err)
	}
	if before != staleHookID {
		t.Fatalf("precondition failed: hook slot = %q, want %q", before, staleHookID)
	}

	cmd := &cobra.Command{}
	if err := runUnslingWith(cmd, nil, false, false); err != nil {
		t.Fatalf("runUnslingWith returned error: %v", err)
	}

	after, err := readHookSlotViaBD(rigDir, agentBeadID)
	if err != nil {
		t.Fatalf("read hook slot after unsling: %v", err)
	}
	if after != "" {
		t.Fatalf("expected runUnslingWith to clear stale hook slot, got %q", after)
	}
}

func readHookSlotViaBD(workDir, agentBeadID string) (string, error) {
	q := fmt.Sprintf("SELECT id, hook_bead FROM wisps WHERE id = '%s'", strings.ReplaceAll(agentBeadID, "'", "''"))
	out, err := execCommand(workDir, "bd", "sql", "--json", q)
	if err != nil {
		return "", err
	}

	var rows []struct {
		ID       string `json:"id"`
		HookBead string `json:"hook_bead"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return "", fmt.Errorf("parse sql output: %w (raw=%s)", err, out)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no rows for agent %s", agentBeadID)
	}
	return rows[0].HookBead, nil
}

func execCommand(workDir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // test helper with static command name
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s %v failed: %w (%s)", name, args, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
