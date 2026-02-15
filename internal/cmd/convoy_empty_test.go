package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mockBdForConvoyTest creates a fake bd binary tailored for convoy empty-check
// tests. The script handles show, dep, close, and list subcommands.
// closeLogPath is the file where close commands are logged for verification.
func mockBdForConvoyTest(t *testing.T, convoyID, convoyTitle string) (binDir, townBeads, closeLogPath string) {
	t.Helper()

	binDir = t.TempDir()
	townRoot := t.TempDir()
	townBeads = filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatalf("mkdir townBeads: %v", err)
	}

	closeLogPath = filepath.Join(binDir, "bd-close.log")

	bdPath := filepath.Join(binDir, "bd")
	if runtime.GOOS == "windows" {
		t.Skip("skipping convoy empty test on Windows")
	}

	// Shell script that handles the bd subcommands needed by
	// checkSingleConvoy and findStrandedConvoys.
	script := `#!/bin/sh
CLOSE_LOG="` + closeLogPath + `"
CONVOY_ID="` + convoyID + `"
CONVOY_TITLE="` + convoyTitle + `"

# Find the actual subcommand (skip global flags like --allow-stale)
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;; # skip flags
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  show)
    # Return convoy JSON
    echo '[{"id":"'"$CONVOY_ID"'","title":"'"$CONVOY_TITLE"'","status":"open","issue_type":"convoy"}]'
    exit 0
    ;;
  dep)
    # Return empty tracked issues
    echo '[]'
    exit 0
    ;;
  close)
    # Log the close command for verification
    echo "$@" >> "$CLOSE_LOG"
    exit 0
    ;;
  list)
    # Return one open convoy
    echo '[{"id":"'"$CONVOY_ID"'","title":"'"$CONVOY_TITLE"'"}]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}

	// Prepend mock bd to PATH
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return binDir, townBeads, closeLogPath
}

func TestCheckSingleConvoy_EmptyConvoyAutoCloses(t *testing.T) {
	_, townBeads, closeLogPath := mockBdForConvoyTest(t, "hq-empty1", "Empty test convoy")

	err := checkSingleConvoy(townBeads, "hq-empty1", false)
	if err != nil {
		t.Fatalf("checkSingleConvoy() error: %v", err)
	}

	// Verify bd close was called with the empty-convoy reason
	data, err := os.ReadFile(closeLogPath)
	if err != nil {
		t.Fatalf("reading close log: %v", err)
	}
	log := string(data)
	if !strings.Contains(log, "hq-empty1") {
		t.Errorf("close log should contain convoy ID, got: %q", log)
	}
	if !strings.Contains(log, "Empty convoy") {
		t.Errorf("close log should contain empty-convoy reason, got: %q", log)
	}
}

func TestCheckSingleConvoy_EmptyConvoyDryRun(t *testing.T) {
	_, townBeads, closeLogPath := mockBdForConvoyTest(t, "hq-empty2", "Dry run convoy")

	err := checkSingleConvoy(townBeads, "hq-empty2", true)
	if err != nil {
		t.Fatalf("checkSingleConvoy() dry-run error: %v", err)
	}

	// In dry-run mode, bd close should NOT be called
	_, err = os.ReadFile(closeLogPath)
	if err == nil {
		t.Error("dry-run should not call bd close, but close log exists")
	}
}

func TestFindStrandedConvoys_EmptyConvoyFlagged(t *testing.T) {
	_, townBeads, _ := mockBdForConvoyTest(t, "hq-empty3", "Stranded empty convoy")

	stranded, err := findStrandedConvoys(townBeads)
	if err != nil {
		t.Fatalf("findStrandedConvoys() error: %v", err)
	}

	if len(stranded) != 1 {
		t.Fatalf("expected 1 stranded convoy, got %d", len(stranded))
	}

	s := stranded[0]
	if s.ID != "hq-empty3" {
		t.Errorf("stranded convoy ID = %q, want %q", s.ID, "hq-empty3")
	}
	if s.ReadyCount != 0 {
		t.Errorf("stranded ReadyCount = %d, want 0", s.ReadyCount)
	}
	if len(s.ReadyIssues) != 0 {
		t.Errorf("stranded ReadyIssues = %v, want empty", s.ReadyIssues)
	}
}

// TestFindStrandedConvoys_MixedConvoys verifies that findStrandedConvoys
// correctly returns both empty (cleanup) and feedable (has ready issues)
// convoys, and that the JSON output shape is correct for each type.
func TestFindStrandedConvoys_MixedConvoys(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping convoy test on Windows")
	}

	binDir := t.TempDir()
	townRoot := t.TempDir()
	townBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatalf("mkdir townBeads: %v", err)
	}

	bdPath := filepath.Join(binDir, "bd")

	// Mock bd that returns two convoys: one empty, one with a ready issue.
	// Uses positional arg parsing to dispatch on convoy ID for dep commands.
	script := `#!/bin/sh
# Collect positional args (skip flags)
i=0
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) eval "pos$i=\"$arg\""; i=$((i+1)) ;;
  esac
done

case "$pos0" in
  list)
    echo '[{"id":"hq-empty-mix","title":"Empty convoy"},{"id":"hq-feed-mix","title":"Feedable convoy"}]'
    exit 0
    ;;
  dep)
    # pos2 is the convoy ID (dep list <convoy-id> ...)
    case "$pos2" in
      hq-empty-mix)
        echo '[]'
        ;;
      hq-feed-mix)
        echo '[{"id":"gt-ready1","title":"Ready issue","status":"open","issue_type":"task","assignee":"","dependency_type":"tracks"}]'
        ;;
      *)
        echo '[]'
        ;;
    esac
    exit 0
    ;;
  show)
    # Return issue details for any show query
    echo '[{"id":"gt-ready1","title":"Ready issue","status":"open","issue_type":"task","assignee":"","blocked_by":[],"blocked_by_count":0,"dependencies":[]}]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stranded, err := findStrandedConvoys(townBeads)
	if err != nil {
		t.Fatalf("findStrandedConvoys() error: %v", err)
	}

	if len(stranded) != 2 {
		t.Fatalf("expected 2 stranded convoys, got %d", len(stranded))
	}

	// Build a map for easier assertions
	byID := map[string]strandedConvoyInfo{}
	for _, s := range stranded {
		byID[s.ID] = s
	}

	// Verify empty convoy
	empty, ok := byID["hq-empty-mix"]
	if !ok {
		t.Fatal("missing empty convoy hq-empty-mix in stranded results")
	}
	if empty.ReadyCount != 0 {
		t.Errorf("empty convoy ReadyCount = %d, want 0", empty.ReadyCount)
	}
	if len(empty.ReadyIssues) != 0 {
		t.Errorf("empty convoy ReadyIssues = %v, want empty", empty.ReadyIssues)
	}

	// Verify feedable convoy
	feedable, ok := byID["hq-feed-mix"]
	if !ok {
		t.Fatal("missing feedable convoy hq-feed-mix in stranded results")
	}
	if feedable.ReadyCount != 1 {
		t.Errorf("feedable convoy ReadyCount = %d, want 1", feedable.ReadyCount)
	}
	if len(feedable.ReadyIssues) != 1 || feedable.ReadyIssues[0] != "gt-ready1" {
		t.Errorf("feedable convoy ReadyIssues = %v, want [gt-ready1]", feedable.ReadyIssues)
	}

	// Verify JSON encoding shape — empty slice encodes as [] not null
	jsonBytes, err := json.Marshal(stranded)
	if err != nil {
		t.Fatalf("json.Marshal(stranded): %v", err)
	}
	jsonStr := string(jsonBytes)
	if strings.Contains(jsonStr, `"ready_issues":null`) {
		t.Error("JSON output contains ready_issues:null — should be [] for empty convoys")
	}
}
