package cmd

import (
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
