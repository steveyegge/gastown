package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestConvoyStage_CrossRigWaves is the regression test for hq-mtc.
//
// Scenario: three beads in three different rigs, with a linear cross-rig dep
// chain mo-bmx ← fr-2p0 ← svh-0m5 (read "X depends on Y").
//
// Expected: gt convoy stage produces 3 waves: [mo-bmx] → [fr-2p0] → [svh-0m5]
// Bug (before beads fix): bd dep list returned [] for all three (cross-rig
// rows silently dropped), so the wave walker saw zero edges and produced 1
// wave with all 3 in parallel.
//
// This test stubs `bd` so it returns the dep edges that the fixed beads CLI
// would now emit (with cross-rig placeholders), and asserts the wave layering
// is correct.
func TestConvoyStage_CrossRigWaves(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, _ := makeRoutingTownWorkspace(t)

	// Set up routes.jsonl so resolveBeadDir works for each prefix.
	// (We don't need actual rig dirs for the stub — bd is stubbed and never
	// reads from disk. We just need resolveBeadDir to return *something*
	// non-empty so the per-bead routing branch is exercised.)
	routes := `{"prefix":"mo-","path":"modules"}
{"prefix":"fr-","path":"frontend"}
{"prefix":"svh-","path":"services_helm"}
`
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}
	for _, sub := range []string{"modules", "frontend", "services_helm"} {
		if err := os.MkdirAll(filepath.Join(townRoot, sub, ".beads"), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}

	chdirConvoyTest(t, townRoot)

	// Stub bd: respond to `bd show <id> --json` and `bd dep list <id> --json`.
	// The dep list responses include the cross-rig edge that the fixed beads
	// CLI now emits (placeholder Issue with id + dependency_type).
	scriptBody := `
case "$*" in
  "show mo-bmx --json")
    echo '[{"id":"mo-bmx","title":"modules phase","status":"open","issue_type":"task"}]'
    ;;
  "show fr-2p0 --json")
    echo '[{"id":"fr-2p0","title":"frontend phase","status":"open","issue_type":"task"}]'
    ;;
  "show svh-0m5 --json")
    echo '[{"id":"svh-0m5","title":"services helm phase","status":"open","issue_type":"task"}]'
    ;;
  "dep list mo-bmx --json")
    echo '[]'
    ;;
  "dep list fr-2p0 --json")
    echo '[{"id":"mo-bmx","dependency_type":"blocks"}]'
    ;;
  "dep list svh-0m5 --json")
    echo '[{"id":"fr-2p0","dependency_type":"blocks"}]'
    ;;
  *)
    echo "unexpected bd invocation: $*" >&2
    exit 1
    ;;
esac
`
	writeRoutingBdStub(t, scriptBody)

	// Drive collectTaskListBeads → buildConvoyDAG → computeWaves directly.
	taskIDs := []string{"mo-bmx", "fr-2p0", "svh-0m5"}
	beads, deps, err := collectTaskListBeads(taskIDs)
	if err != nil {
		t.Fatalf("collectTaskListBeads: %v", err)
	}
	if len(beads) != 3 {
		t.Fatalf("expected 3 beads, got %d: %+v", len(beads), beads)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 cross-rig deps (fr-2p0→mo-bmx, svh-0m5→fr-2p0), got %d: %+v", len(deps), deps)
	}

	dag := buildConvoyDAG(beads, deps)
	waves, _, err := computeWaves(dag)
	if err != nil {
		t.Fatalf("computeWaves: %v", err)
	}

	if len(waves) != 3 {
		var summary string
		for i, w := range waves {
			summary += fmt.Sprintf("\n  Wave %d: %v", i+1, waveTaskIDs(w))
		}
		t.Fatalf("expected 3 waves (linear cross-rig chain), got %d:%s", len(waves), summary)
	}

	expect := [][]string{{"mo-bmx"}, {"fr-2p0"}, {"svh-0m5"}}
	for i, want := range expect {
		got := waveTaskIDs(waves[i])
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("wave %d: expected %v, got %v", i+1, want, got)
		}
	}
}

// waveTaskIDs extracts the task IDs from a Wave for assertion purposes.
// (Helper kept here to avoid coupling to internals of other test files.)
func waveTaskIDs(w Wave) []string {
	ids := make([]string, 0, len(w.Tasks))
	ids = append(ids, w.Tasks...)
	return ids
}
