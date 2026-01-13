package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWispIDFromJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantID  string
		wantErr bool
	}{
		{
			name:   "new_epic_id",
			json:   `{"new_epic_id":"gt-wisp-abc","created":7,"phase":"vapor"}`,
			wantID: "gt-wisp-abc",
		},
		{
			name:   "root_id legacy",
			json:   `{"root_id":"gt-wisp-legacy"}`,
			wantID: "gt-wisp-legacy",
		},
		{
			name:   "result_id forward compat",
			json:   `{"result_id":"gt-wisp-result"}`,
			wantID: "gt-wisp-result",
		},
		{
			name:   "precedence prefers new_epic_id",
			json:   `{"root_id":"gt-wisp-legacy","new_epic_id":"gt-wisp-new"}`,
			wantID: "gt-wisp-new",
		},
		{
			name:    "missing id keys",
			json:    `{"created":7,"phase":"vapor"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			json:    `{"new_epic_id":`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := parseWispIDFromJSON([]byte(tt.json))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWispIDFromJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if gotID != tt.wantID {
				t.Fatalf("parseWispIDFromJSON() id = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}

func TestFormatTrackBeadID(t *testing.T) {
	tests := []struct {
		name     string
		beadID   string
		expected string
	}{
		// HQ beads should remain unchanged
		{
			name:     "hq bead unchanged",
			beadID:   "hq-abc123",
			expected: "hq-abc123",
		},
		{
			name:     "hq convoy unchanged",
			beadID:   "hq-cv-xyz789",
			expected: "hq-cv-xyz789",
		},

		// Cross-rig beads get external: prefix
		{
			name:     "gastown rig bead",
			beadID:   "gt-mol-abc123",
			expected: "external:gt-mol:gt-mol-abc123",
		},
		{
			name:     "beads rig task",
			beadID:   "beads-task-xyz",
			expected: "external:beads-task:beads-task-xyz",
		},
		{
			name:     "two segment ID",
			beadID:   "foo-bar",
			expected: "external:foo-bar:foo-bar",
		},

		// Edge cases
		{
			name:     "single segment fallback",
			beadID:   "orphan",
			expected: "orphan",
		},
		{
			name:     "empty string fallback",
			beadID:   "",
			expected: "",
		},
		{
			name:     "many segments",
			beadID:   "a-b-c-d-e-f",
			expected: "external:a-b:a-b-c-d-e-f",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTrackBeadID(tt.beadID)
			if result != tt.expected {
				t.Errorf("formatTrackBeadID(%q) = %q, want %q", tt.beadID, result, tt.expected)
			}
		})
	}
}

// TestFormatTrackBeadIDConsumerCompatibility verifies that the external ref format
// produced by formatTrackBeadID can be correctly parsed by the consumer pattern
// used in convoy.go, model.go, feed/convoy.go, and web/fetcher.go.
func TestFormatTrackBeadIDConsumerCompatibility(t *testing.T) {
	// Consumer pattern from convoy.go:1062-1068:
	// if strings.HasPrefix(issueID, "external:") {
	//     parts := strings.SplitN(issueID, ":", 3)
	//     if len(parts) == 3 {
	//         issueID = parts[2] // Extract the actual issue ID
	//     }
	// }

	tests := []struct {
		name           string
		beadID         string
		wantOriginalID string
	}{
		{
			name:           "cross-rig bead round-trips",
			beadID:         "gt-mol-abc123",
			wantOriginalID: "gt-mol-abc123",
		},
		{
			name:           "beads rig bead round-trips",
			beadID:         "beads-task-xyz",
			wantOriginalID: "beads-task-xyz",
		},
		{
			name:           "hq bead unchanged",
			beadID:         "hq-abc123",
			wantOriginalID: "hq-abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := formatTrackBeadID(tt.beadID)

			// Simulate consumer parsing logic
			parsed := formatted
			if len(formatted) > 9 && formatted[:9] == "external:" {
				parts := make([]string, 0, 3)
				start := 0
				count := 0
				for i := 0; i < len(formatted) && count < 2; i++ {
					if formatted[i] == ':' {
						parts = append(parts, formatted[start:i])
						start = i + 1
						count++
					}
				}
				if count == 2 {
					parts = append(parts, formatted[start:])
				}
				if len(parts) == 3 {
					parsed = parts[2]
				}
			}

			if parsed != tt.wantOriginalID {
				t.Errorf("round-trip failed: formatTrackBeadID(%q) = %q, parsed back to %q, want %q",
					tt.beadID, formatted, parsed, tt.wantOriginalID)
			}
		})
	}
}

func TestSlingFormulaOnBeadRoutesBDCommandsToTargetRig(t *testing.T) {
	townRoot := t.TempDir()

	// Minimal workspace marker so workspace.FindFromCwd() succeeds.
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create a rig path that owns gt-* beads, and a routes.jsonl pointing to it.
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Stub bd so we can observe the working directory for cook/wisp/bond.
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdPath := filepath.Join(binDir, "bd")
	bdScript := `#!/bin/sh
set -e
echo "$(pwd)|$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Test issue","status":"open","assignee":"","description":""}]'
    ;;
  formula)
    # formula show <name> - must output something for verifyFormulaExists
    echo '{"name":"test-formula"}'
    exit 0
    ;;
  cook)
    exit 0
    ;;
  mol)
    sub="$1"
    shift || true
    case "$sub" in
      wisp)
        echo '{"new_epic_id":"gt-wisp-xyz"}'
        ;;
      bond)
        echo '{"root_id":"gt-wisp-xyz"}'
        ;;
    esac
    ;;
esac
exit 0
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "mayor")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")
	t.Setenv("TMUX_PANE", "") // Prevent inheriting real tmux pane from test runner

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Ensure we don't leak global flag state across tests.
	prevOn := slingOnTarget
	prevVars := slingVars
	prevDryRun := slingDryRun
	prevNoConvoy := slingNoConvoy
	t.Cleanup(func() {
		slingOnTarget = prevOn
		slingVars = prevVars
		slingDryRun = prevDryRun
		slingNoConvoy = prevNoConvoy
	})

	slingDryRun = false
	slingNoConvoy = true
	slingVars = nil
	slingOnTarget = "gt-abc123"

	// Prevent real tmux nudge from firing during tests (causes agent self-interruption)
	t.Setenv("GT_TEST_NO_NUDGE", "1")

	if err := runSling(nil, []string{"mol-review"}); err != nil {
		t.Fatalf("runSling: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}
	logLines := strings.Split(strings.TrimSpace(string(logBytes)), "\n")

	wantDir := rigDir
	if resolved, err := filepath.EvalSymlinks(wantDir); err == nil {
		wantDir = resolved
	}
	gotCook := false
	gotWisp := false
	gotBond := false

	for _, line := range logLines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		dir := parts[0]
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			dir = resolved
		}
		args := parts[1]

		switch {
		case strings.Contains(args, " cook "):
			gotCook = true
			// cook doesn't need database context, runs from cwd
		case strings.Contains(args, " mol wisp "):
			gotWisp = true
			if dir != wantDir {
				t.Fatalf("bd mol wisp ran in %q, want %q (args: %q)", dir, wantDir, args)
			}
		case strings.Contains(args, " mol bond "):
			gotBond = true
			if dir != wantDir {
				t.Fatalf("bd mol bond ran in %q, want %q (args: %q)", dir, wantDir, args)
			}
		}
	}

	if !gotCook || !gotWisp || !gotBond {
		t.Fatalf("missing expected bd commands: cook=%v wisp=%v bond=%v (log: %q)", gotCook, gotWisp, gotBond, string(logBytes))
	}
}

// TestSlingFormulaOnBeadPassesFeatureAndIssueVars verifies that when using
// gt sling <formula> --on <bead>, both --var feature=<title> and --var issue=<beadID>
// are passed to the bd mol wisp command.
func TestSlingFormulaOnBeadPassesFeatureAndIssueVars(t *testing.T) {
	townRoot := t.TempDir()

	// Minimal workspace marker so workspace.FindFromCwd() succeeds.
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create a rig path that owns gt-* beads, and a routes.jsonl pointing to it.
	rigDir := filepath.Join(townRoot, "gastown", "mayor", "rig")
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatalf("mkdir rigDir: %v", err)
	}
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Stub bd so we can observe the arguments passed to mol wisp.
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	bdPath := filepath.Join(binDir, "bd")
	// The stub returns a specific title so we can verify it appears in --var feature=
	bdScript := `#!/bin/sh
set -e
echo "ARGS:$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"My Test Feature","status":"open","assignee":"","description":""}]'
    ;;
  formula)
    # formula show <name> - must output something for verifyFormulaExists
    echo '{"name":"mol-review"}'
    exit 0
    ;;
  cook)
    exit 0
    ;;
  mol)
    sub="$1"
    shift || true
    case "$sub" in
      wisp)
        echo '{"new_epic_id":"gt-wisp-xyz"}'
        ;;
      bond)
        echo '{"root_id":"gt-wisp-xyz"}'
        ;;
    esac
    ;;
esac
exit 0
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "mayor")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")
	t.Setenv("TMUX_PANE", "") // Prevent inheriting real tmux pane from test runner

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Ensure we don't leak global flag state across tests.
	prevOn := slingOnTarget
	prevVars := slingVars
	prevDryRun := slingDryRun
	prevNoConvoy := slingNoConvoy
	t.Cleanup(func() {
		slingOnTarget = prevOn
		slingVars = prevVars
		slingDryRun = prevDryRun
		slingNoConvoy = prevNoConvoy
	})

	slingDryRun = false
	slingNoConvoy = true
	slingVars = nil
	slingOnTarget = "gt-abc123"

	// Prevent real tmux nudge from firing during tests (causes agent self-interruption)
	t.Setenv("GT_TEST_NO_NUDGE", "1")

	if err := runSling(nil, []string{"mol-review"}); err != nil {
		t.Fatalf("runSling: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}

	// Find the mol wisp command and verify both --var arguments
	logLines := strings.Split(string(logBytes), "\n")
	var wispLine string
	for _, line := range logLines {
		if strings.Contains(line, "mol wisp") {
			wispLine = line
			break
		}
	}

	if wispLine == "" {
		t.Fatalf("mol wisp command not found in log: %s", string(logBytes))
	}

	// Verify --var feature=<title> is present
	if !strings.Contains(wispLine, "--var feature=My Test Feature") {
		t.Errorf("mol wisp missing --var feature=<title>\ngot: %s", wispLine)
	}

	// Verify --var issue=<beadID> is present
	if !strings.Contains(wispLine, "--var issue=gt-abc123") {
		t.Errorf("mol wisp missing --var issue=<beadID>\ngot: %s", wispLine)
	}
}

// TestVerifyBeadExistsAllowStale reproduces the bug in gtl-ncq where beads
// visible via regular bd show fail with --no-daemon due to database sync issues.
// The fix uses --allow-stale to skip the sync check for existence verification.
func TestVerifyBeadExistsAllowStale(t *testing.T) {
	townRoot := t.TempDir()

	// Create minimal workspace structure
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create a stub bd that simulates the sync issue:
	// - --no-daemon without --allow-stale fails (database out of sync)
	// - --no-daemon with --allow-stale succeeds (skips sync check)
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	bdPath := filepath.Join(binDir, "bd")
	bdScript := `#!/bin/sh
# Check for --allow-stale flag
allow_stale=false
for arg in "$@"; do
  if [ "$arg" = "--allow-stale" ]; then
    allow_stale=true
  fi
done

if [ "$1" = "--no-daemon" ]; then
  if [ "$allow_stale" = "true" ]; then
    # --allow-stale skips sync check, succeeds
    echo '[{"title":"Test bead","status":"open","assignee":""}]'
    exit 0
  else
    # Without --allow-stale, fails with sync error
    echo '{"error":"Database out of sync with JSONL."}'
    exit 1
  fi
fi
# Daemon mode works
echo '[{"title":"Test bead","status":"open","assignee":""}]'
exit 0
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// EXPECTED: verifyBeadExists should use --no-daemon --allow-stale and succeed
	beadID := "jv-v599"
	err = verifyBeadExists(beadID)
	if err != nil {
		t.Errorf("verifyBeadExists(%q) failed: %v\nExpected --allow-stale to skip sync check", beadID, err)
	}
}

// TestSlingWithAllowStale tests the full gt sling flow with --allow-stale fix.
// This is an integration test for the gtl-ncq bug.
func TestSlingWithAllowStale(t *testing.T) {
	townRoot := t.TempDir()

	// Create minimal workspace structure
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create stub bd that respects --allow-stale
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	bdPath := filepath.Join(binDir, "bd")
	bdScript := `#!/bin/sh
# Check for --allow-stale flag
allow_stale=false
for arg in "$@"; do
  if [ "$arg" = "--allow-stale" ]; then
    allow_stale=true
  fi
done

if [ "$1" = "--no-daemon" ]; then
  shift
  cmd="$1"
  if [ "$cmd" = "show" ]; then
    if [ "$allow_stale" = "true" ]; then
      echo '[{"title":"Synced bead","status":"open","assignee":""}]'
      exit 0
    fi
    echo '{"error":"Database out of sync"}'
    exit 1
  fi
  exit 0
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Synced bead","status":"open","assignee":""}]'
    ;;
  update)
    exit 0
    ;;
esac
exit 0
`
	if err := os.WriteFile(bdPath, []byte(bdScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "crew")
	t.Setenv("GT_CREW", "jv")
	t.Setenv("GT_POLECAT", "")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Save and restore global flags
	prevDryRun := slingDryRun
	prevNoConvoy := slingNoConvoy
	t.Cleanup(func() {
		slingDryRun = prevDryRun
		slingNoConvoy = prevNoConvoy
	})

	slingDryRun = true
	slingNoConvoy = true

	// EXPECTED: gt sling should use daemon mode and succeed
	// ACTUAL: verifyBeadExists uses --no-daemon and fails with sync error
	beadID := "jv-v599"
	err = runSling(nil, []string{beadID})
	if err != nil {
		// Check if it's the specific error we're testing for
		if strings.Contains(err.Error(), "is not a valid bead or formula") {
			t.Errorf("gt sling failed to recognize bead %q: %v\nExpected to use daemon mode, but used --no-daemon which fails when DB out of sync", beadID, err)
		} else {
			// Some other error - might be expected in dry-run mode
			t.Logf("gt sling returned error (may be expected in test): %v", err)
		}
	}
}

// TestLooksLikeBeadID tests the bead ID pattern recognition function.
// This ensures gt sling accepts bead IDs even when routing-based verification fails.
// Fixes: gt sling bd-ka761 failing with 'not a valid bead or formula'
//
// Note: looksLikeBeadID is a fallback check in sling. The actual sling flow is:
// 1. Try verifyBeadExists (routing-based lookup)
// 2. Try verifyFormulaExists (formula check)
// 3. Fall back to looksLikeBeadID pattern match
// So "mol-release" matches the pattern but won't be treated as bead in practice
// because it would be caught by formula verification first.
func TestLooksLikeBeadID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Valid bead IDs - should return true
		{"gt-abc123", true},
		{"bd-ka761", true},
		{"hq-cv-abc", true},
		{"ap-qtsup.16", true},
		{"beads-xyz", true},
		{"jv-v599", true},
		{"gt-9e8s5", true},
		{"hq-00gyg", true},

		// Short prefixes that match pattern (but may be formulas in practice)
		{"mol-release", true},    // 3-char prefix matches pattern (formula check runs first in sling)
		{"mol-abc123", true},     // 3-char prefix matches pattern

		// Non-bead strings - should return false
		{"formula-name", false},  // "formula" is 7 chars (> 5)
		{"mayor", false},         // no hyphen
		{"gastown", false},       // no hyphen
		{"deacon/dogs", false},   // contains slash
		{"", false},              // empty
		{"-abc", false},          // starts with hyphen
		{"GT-abc", false},        // uppercase prefix
		{"123-abc", false},       // numeric prefix
		{"a-", false},            // nothing after hyphen
		{"aaaaaa-b", false},      // prefix too long (6 chars)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := looksLikeBeadID(tt.input)
			if got != tt.want {
				t.Errorf("looksLikeBeadID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
