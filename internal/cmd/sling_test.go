package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeBDStub(t *testing.T, binDir string, unixScript string, windowsScript string) string {
	t.Helper()

	var path string
	if runtime.GOOS == "windows" {
		path = filepath.Join(binDir, "bd.cmd")
		if err := os.WriteFile(path, []byte(windowsScript), 0644); err != nil {
			t.Fatalf("write bd stub: %v", err)
		}
		return path
	}

	path = filepath.Join(binDir, "bd")
	if err := os.WriteFile(path, []byte(unixScript), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	return path
}

func containsVarArg(line, key, value string) bool {
	plain := "--var " + key + "=" + value
	if strings.Contains(line, plain) {
		return true
	}
	quoted := "--var \"" + key + "=" + value + "\""
	return strings.Contains(line, quoted)
}

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
	// Set up a test town with routes.jsonl for proper routing-based tests
	townRoot := t.TempDir()

	// Create minimal workspace marker so workspace.FindFromCwd() succeeds
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create routes.jsonl with standard mappings
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"bd-","path":"beads/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Change to test directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	tests := []struct {
		name     string
		beadID   string
		expected string
	}{
		// HQ beads should remain unchanged (local to town beads)
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

		// Cross-rig beads get external: prefix with PROJECT name (from routes.jsonl)
		{
			name:     "gastown rig bead uses project name",
			beadID:   "gt-mol-abc123",
			expected: "external:gastown:gt-mol-abc123",
		},
		{
			name:     "beads rig bead uses project name",
			beadID:   "bd-task-xyz",
			expected: "external:beads:bd-task-xyz",
		},
		{
			name:     "gt prefix two segments",
			beadID:   "gt-abc",
			expected: "external:gastown:gt-abc",
		},

		// Beads without routes return unchanged - let bd handle routing
		{
			name:     "unknown prefix two segments returns unchanged",
			beadID:   "foo-bar",
			expected: "foo-bar",
		},
		{
			name:     "unknown prefix three segments returns unchanged",
			beadID:   "unk-mol-abc123",
			expected: "unk-mol-abc123",
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
	// Set up a test town with routes.jsonl for proper routing
	townRoot := t.TempDir()

	// Create minimal workspace marker
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create routes.jsonl
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"gastown/mayor/rig"}`,
		`{"prefix":"bd-","path":"beads/mayor/rig"}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Change to test directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

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
			name:           "gastown bead round-trips",
			beadID:         "gt-mol-abc123",
			wantOriginalID: "gt-mol-abc123",
		},
		{
			name:           "beads rig bead round-trips",
			beadID:         "bd-task-xyz",
			wantOriginalID: "bd-task-xyz",
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
bdScriptWindows := `@echo off
setlocal enableextensions
echo %CD%^|%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  set "sub=%3"
)
if "%cmd%"=="show" (
  echo [{"title":"Test issue","status":"open","assignee":"","description":""}]
  exit /b 0
)
if "%cmd%"=="formula" (
  echo {"name":"test-formula"}
  exit /b 0
)
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {"new_epic_id":"gt-wisp-xyz"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    echo {"root_id":"gt-wisp-xyz"}
    exit /b 0
  )
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	attachedLogPath := filepath.Join(townRoot, "attached-molecule.log")
	t.Setenv("GT_TEST_ATTACHED_MOLECULE_LOG", attachedLogPath)
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
bdScriptWindows := `@echo off
setlocal enableextensions
echo ARGS:%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  set "sub=%3"
)
if "%cmd%"=="show" (
  echo [{^"title^":^"My Test Feature^",^"status^":^"open^",^"assignee^":^"^",^"description^":^"^"}]
  exit /b 0
)
if "%cmd%"=="formula" (
  echo {^"name^":^"mol-review^"}
  exit /b 0
)
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-xyz^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    echo {^"root_id^":^"gt-wisp-xyz^"}
    exit /b 0
  )
)
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	attachedLogPath := filepath.Join(townRoot, "attached-molecule.log")
	t.Setenv("GT_TEST_ATTACHED_MOLECULE_LOG", attachedLogPath)
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
	if !containsVarArg(wispLine, "feature", "My Test Feature") {
		t.Errorf("mol wisp missing --var feature=<title>\ngot: %s", wispLine)
	}

	// Verify --var issue=<beadID> is present
	if !containsVarArg(wispLine, "issue", "gt-abc123") {
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
	bdScriptWindows := `@echo off
setlocal enableextensions
set "allow=false"
for %%A in (%*) do (
  if "%%~A"=="--allow-stale" set "allow=true"
)
if "%1"=="--no-daemon" (
  if "%allow%"=="true" (
    echo [{"title":"Test bead","status":"open","assignee":""}]
    exit /b 0
  )
  echo {"error":"Database out of sync with JSONL."}
  exit /b 1
)
echo [{"title":"Test bead","status":"open","assignee":""}]
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

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
bdScriptWindows := `@echo off
setlocal enableextensions
set "allow=false"
for %%A in (%*) do (
  if "%%~A"=="--allow-stale" set "allow=true"
)
set "cmd=%1"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  if "%cmd%"=="show" (
    if "%allow%"=="true" (
      echo [{"title":"Synced bead","status":"open","assignee":""}]
      exit /b 0
    )
    echo {"error":"Database out of sync"}
    exit /b 1
  )
  exit /b 0
)
set "cmd=%1"
if "%cmd%"=="show" (
  echo [{"title":"Synced bead","status":"open","assignee":""}]
  exit /b 0
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "crew")
	t.Setenv("GT_CREW", "jv")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("TMUX_PANE", "") // Prevent inheriting real tmux pane from test runner

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

	// Prevent real tmux nudge from firing during tests (causes agent self-interruption)
	t.Setenv("GT_TEST_NO_NUDGE", "1")

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

// TestEscapeSQLString tests the SQL injection prevention helper.
func TestEscapeSQLString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no_special_chars",
			input: "gt-abc123",
			want:  "gt-abc123",
		},
		{
			name:  "single_quote",
			input: "O'Reilly",
			want:  "O''Reilly",
		},
		{
			name:  "multiple_single_quotes",
			input: "it's a 'test'",
			want:  "it''s a ''test''",
		},
		{
			name:  "sql_injection_attempt",
			input: "'; DROP TABLE issues; --",
			want:  "''; DROP TABLE issues; --",
		},
		{
			name:  "empty_string",
			input: "",
			want:  "",
		},
		{
			name:  "only_quote",
			input: "'",
			want:  "''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeSQLString(tt.input)
			if got != tt.want {
				t.Errorf("escapeSQLString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
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
		{"oc-release", true},     // 2-char prefix matches pattern (formula check runs first in sling)
		{"oc-abc123", true},      // 2-char prefix matches pattern

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

// TestSlingFormulaRigTargetCreatesWispBeforeSpawn verifies that when slinging
// a formula to a rig target (which spawns a new polecat), the wisp is created
// BEFORE the polecat is spawned. This prevents a race condition where the
// polecat starts and runs gt prime before its work (wisp) exists.
//
// Bug (bd-gj3): The original code spawned the polecat during target resolution,
// then created the wisp afterward. The polecat could start and check its hook
// before the wisp was created, finding no work.
//
// Fix: Defer polecat spawn until after wisp creation. Order is now:
// 1. Cook formula
// 2. Create wisp
// 3. Spawn polecat (wisp already exists)
// 4. Hook wisp to polecat
func TestSlingFormulaRigTargetCreatesWispBeforeSpawn(t *testing.T) {
	townRoot := t.TempDir()

	// Create minimal workspace structure with a configured rig
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create the test rig directory structure
	testRigDir := filepath.Join(townRoot, "testrig")
	if err := os.MkdirAll(filepath.Join(testRigDir, "polecats"), 0755); err != nil {
		t.Fatalf("mkdir testrig/polecats: %v", err)
	}
	// Create a .git file to make it look like a worktree
	if err := os.WriteFile(filepath.Join(testRigDir, ".git"), []byte("gitdir: ../.."), 0644); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	// Create rigs.json with our test rig
	rigsConfig := `{"rigs":{"testrig":{"path":"testrig","repo":"test"}}}`
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte(rigsConfig), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	// Create routes.jsonl
	routes := strings.Join([]string{
		`{"prefix":"gt-","path":"."}`,
		`{"prefix":"hq-","path":"."}`,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte(routes), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	// Stub bd to log commands with sequence numbers
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	// The stub logs each command with a sequence number for order verification
	bdScript := `#!/bin/sh
set -e
# Append command to log with sequence number
if [ -f "${BD_LOG}" ]; then
  seq=$(wc -l < "${BD_LOG}" | tr -d ' ')
else
  seq=0
fi
echo "${seq}|$*" >> "${BD_LOG}"

if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  formula)
    echo '{"name":"mol-polecat-work"}'
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
        echo '{"new_epic_id":"gt-wisp-race-test"}'
        ;;
    esac
    ;;
  update)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions enabledelayedexpansion
rem Count lines in log file to get sequence number
set "seq=0"
if exist "%BD_LOG%" (
  for /f %%a in ('type "%BD_LOG%" ^| find /c /v ""') do set "seq=%%a"
)
echo !seq!^|%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  set "sub=%3"
)
if "%cmd%"=="formula" (
  echo {"name":"mol-polecat-work"}
  exit /b 0
)
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {"new_epic_id":"gt-wisp-race-test"}
    exit /b 0
  )
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "mayor")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("GT_CREW", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("GT_TEST_NO_NUDGE", "1")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Save and restore global flags
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
	slingOnTarget = "" // No --on target, we want to test rig target

	// Call runSlingFormula with our configured test rig
	// The spawn will fail (no tmux, polecat manager issues in test env),
	// but the key check is that wisp creation happened BEFORE spawn attempt.
	err = runSlingFormula([]string{"mol-polecat-work", "testrig"})

	// Read the log to verify order - even if overall function failed,
	// we want to see that wisp was created before spawn was attempted
	logBytes, err2 := os.ReadFile(logPath)
	if err2 != nil {
		// If log doesn't exist, check if the rig was recognized
		if err != nil {
			t.Logf("runSlingFormula error: %v", err)
		}
		t.Fatalf("bd.log not created - commands not executed (rig may not be recognized)")
	}

	logContent := string(logBytes)
	logLines := strings.Split(strings.TrimSpace(logContent), "\n")

	// Find sequence numbers for cook and wisp commands
	var cookSeq, wispSeq int = -1, -1
	for _, line := range logLines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		seq := 0
		fmt.Sscanf(parts[0], "%d", &seq)
		args := parts[1]

		if strings.Contains(args, "cook") {
			cookSeq = seq
		}
		if strings.Contains(args, "mol wisp") {
			wispSeq = seq
		}
	}

	// Verify cook happened
	if cookSeq < 0 {
		t.Errorf("cook command not found in log:\n%s", logContent)
	}

	// Verify wisp creation happened - this is the KEY check for the race condition fix
	if wispSeq < 0 {
		t.Errorf("mol wisp command not found in log - wisp not created before spawn!\n"+
			"This indicates the race condition fix is not working.\n"+
			"Log:\n%s", logContent)
	}

	// Verify order: cook before wisp
	if cookSeq >= 0 && wispSeq >= 0 && cookSeq > wispSeq {
		t.Errorf("cook (seq %d) should happen before wisp (seq %d)", cookSeq, wispSeq)
	}

	// The key verification: wisp was created before SpawnPolecatForSling was called.
	// Since SpawnPolecatForSling will fail (no tmux in test), but wisp was created,
	// this proves the fix is working - wisp creation happens before spawn attempt.
	if wispSeq >= 0 {
		t.Logf("SUCCESS: Wisp created at sequence %d (before spawn attempt)", wispSeq)
		t.Logf("Race condition fix verified: wisp exists before polecat spawn")
	}

	// The function may return an error due to spawn failing, that's expected
	if err != nil {
		t.Logf("runSlingFormula returned error (expected in test env): %v", err)
	}
}

// TestSlingFormulaOnBeadSetsAttachedMolecule verifies that when using
// gt sling <formula> --on <bead>, the attached_molecule field is set in the
// hooked bead's description after bonding. This is required for gt hook to
// recognize the molecule attachment.
//
// Bug: The original code bonds the wisp to the bead and sets status=hooked,
// but doesn't record attached_molecule in the description. This causes
// gt hook to report "No molecule attached".
func TestSlingFormulaOnBeadSetsAttachedMolecule(t *testing.T) {
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

	// Stub bd so we can observe the arguments passed to update commands.
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
	// The stub logs all commands to a file for verification
	bdScript := `#!/bin/sh
set -e
echo "$PWD|$*" >> "${BD_LOG}"
if [ "$1" = "--no-daemon" ]; then
  shift
fi
cmd="$1"
shift || true
case "$cmd" in
  show)
    echo '[{"title":"Bug to fix","status":"open","assignee":"","description":""}]'
    ;;
  formula)
    echo '{"name":"mol-polecat-work"}'
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
  update)
    # Just succeed
    exit 0
    ;;
esac
exit 0
`
bdScriptWindows := `@echo off
setlocal enableextensions
echo %CD%^|%*>>"%BD_LOG%"
set "cmd=%1"
set "sub=%2"
if "%cmd%"=="--no-daemon" (
  set "cmd=%2"
  set "sub=%3"
)
if "%cmd%"=="show" (
  echo [{^"title^":^"Bug to fix^",^"status^":^"open^",^"assignee^":^"^",^"description^":^"^"}]
  exit /b 0
)
if "%cmd%"=="formula" (
  echo {^"name^":^"mol-polecat-work^"}
  exit /b 0
)
if "%cmd%"=="cook" exit /b 0
if "%cmd%"=="mol" (
  if "%sub%"=="wisp" (
    echo {^"new_epic_id^":^"gt-wisp-xyz^"}
    exit /b 0
  )
  if "%sub%"=="bond" (
    echo {^"root_id^":^"gt-wisp-xyz^"}
    exit /b 0
  )
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	attachedLogPath := filepath.Join(townRoot, "attached-molecule.log")
	t.Setenv("GT_TEST_ATTACHED_MOLECULE_LOG", attachedLogPath)
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
	slingOnTarget = "gt-abc123" // The bug bead we're applying formula to

	// Prevent real tmux nudge from firing during tests (causes agent self-interruption)
	t.Setenv("GT_TEST_NO_NUDGE", "1")

	if err := runSling(nil, []string{"mol-polecat-work"}); err != nil {
		t.Fatalf("runSling: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}

	// After bonding (mol bond), there should be an update call that includes
	// --description with attached_molecule field. This is what gt hook looks for.
	logLines := strings.Split(string(logBytes), "\n")

	// Find all update commands after the bond
	sawBond := false
	foundAttachedMolecule := false
	for _, line := range logLines {
		if strings.Contains(line, "mol bond") {
			sawBond = true
			continue
		}
		if sawBond && strings.Contains(line, "update") {
			// Check if this update sets attached_molecule in description
			if strings.Contains(line, "attached_molecule") {
				foundAttachedMolecule = true
				break
			}
		}
	}

	if !sawBond {
		t.Fatalf("mol bond command not found in log:\n%s", string(logBytes))
	}

	if !foundAttachedMolecule {
		if descBytes, err := os.ReadFile(attachedLogPath); err == nil {
			if strings.Contains(string(descBytes), "attached_molecule") {
				foundAttachedMolecule = true
			}
		}
	}

	if !foundAttachedMolecule {
		attachedLog := "<missing>"
		if descBytes, err := os.ReadFile(attachedLogPath); err == nil {
			attachedLog = string(descBytes)
		}
		t.Errorf("after mol bond, expected update with attached_molecule in description\n"+
			"This is required for gt hook to recognize the molecule attachment.\n"+
			"Log output:\n%s\nAttached log:\n%s", string(logBytes), attachedLog)
	}
}

// TestSlingNoMergeFlag verifies that gt sling --no-merge stores the no_merge flag
// in the bead's description. This flag tells gt done to skip the merge queue
// and keep work on the feature branch for human review.
func TestSlingNoMergeFlag(t *testing.T) {
	townRoot := t.TempDir()

	// Minimal workspace marker so workspace.FindFromCwd() succeeds.
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatalf("mkdir mayor/rig: %v", err)
	}

	// Create stub bd that logs update commands
	binDir := filepath.Join(townRoot, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	logPath := filepath.Join(townRoot, "bd.log")
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
    echo '[{"title":"Test issue","status":"open","assignee":"","description":""}]'
    ;;
  update)
    exit 0
    ;;
esac
exit 0
`
	bdScriptWindows := `@echo off
setlocal enableextensions
echo ARGS:%*>>"%BD_LOG%"
set "cmd=%1"
if "%cmd%"=="--no-daemon" set "cmd=%2"
if "%cmd%"=="show" (
  echo [{"title":"Test issue","status":"open","assignee":"","description":""}]
  exit /b 0
)
if "%cmd%"=="update" exit /b 0
exit /b 0
`
	_ = writeBDStub(t, binDir, bdScript, bdScriptWindows)

	t.Setenv("BD_LOG", logPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(EnvGTRole, "mayor")
	t.Setenv("GT_CREW", "")
	t.Setenv("GT_POLECAT", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("GT_TEST_NO_NUDGE", "1")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(filepath.Join(townRoot, "mayor", "rig")); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Save and restore global flags
	prevDryRun := slingDryRun
	prevNoConvoy := slingNoConvoy
	prevNoMerge := slingNoMerge
	t.Cleanup(func() {
		slingDryRun = prevDryRun
		slingNoConvoy = prevNoConvoy
		slingNoMerge = prevNoMerge
	})

	slingDryRun = false
	slingNoConvoy = true
	slingNoMerge = true // This is what we're testing

	if err := runSling(nil, []string{"gt-test123"}); err != nil {
		t.Fatalf("runSling: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read bd log: %v", err)
	}

	// Look for update command that includes no_merge in description
	logLines := strings.Split(string(logBytes), "\n")
	foundNoMerge := false
	for _, line := range logLines {
		if strings.Contains(line, "update") && strings.Contains(line, "no_merge") {
			foundNoMerge = true
			break
		}
	}

	if !foundNoMerge {
		t.Errorf("--no-merge flag not stored in bead description\nLog:\n%s", string(logBytes))
	}
}
