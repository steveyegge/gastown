package beads

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func installMockBDFixedShowOutput(t *testing.T, showOutput string) {
	t.Helper()

	binDir := t.TempDir()
	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(binDir, "bd.cmd")
		script := "@echo off\r\n" +
			"setlocal EnableDelayedExpansion\r\n" +
			"set \"cmd=\"\r\n" +
			":findcmd\r\n" +
			"if \"%~1\"==\"\" goto havecmd\r\n" +
			"set \"arg=%~1\"\r\n" +
			"if /I \"!arg:~0,2!\"==\"--\" (\r\n" +
			"  shift\r\n" +
			"  goto findcmd\r\n" +
			")\r\n" +
			"set \"cmd=%~1\"\r\n" +
			":havecmd\r\n" +
			"if /I \"%cmd%\"==\"version\" exit /b 0\r\n" +
			"if /I \"%cmd%\"==\"show\" (\r\n" +
			"  echo(%MOCK_BD_SHOW_OUTPUT%\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"exit /b 0\r\n"
		if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
			t.Fatalf("write mock bd: %v", err)
		}
		t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("MOCK_BD_SHOW_OUTPUT", showOutput)
		return
	}

	script := `#!/bin/sh
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  version)
    exit 0
    ;;
  show)
    printf '%s\n' "$MOCK_BD_SHOW_OUTPUT"
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	scriptPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MOCK_BD_SHOW_OUTPUT", showOutput)
}

func installMockBDForAgentStateUpdate(t *testing.T, showOutput, logPath string) {
	t.Helper()

	binDir := t.TempDir()
	if runtime.GOOS == "windows" {
		scriptPath := filepath.Join(binDir, "bd.cmd")
		script := "@echo off\r\n" +
			"setlocal EnableDelayedExpansion\r\n" +
			"set \"cmd=\"\r\n" +
			"set \"id=\"\r\n" +
			"set \"sql=\"\r\n" +
			":findcmd\r\n" +
			"if \"%~1\"==\"\" goto havecmd\r\n" +
			"set \"arg=%~1\"\r\n" +
			"if /I \"!arg:~0,2!\"==\"--\" (\r\n" +
			"  shift\r\n" +
			"  goto findcmd\r\n" +
			")\r\n" +
			"set \"cmd=%~1\"\r\n" +
			"set \"id=%~2\"\r\n" +
			"set \"sql=%~2\"\r\n" +
			":havecmd\r\n" +
			"if /I \"%cmd%\"==\"version\" (\r\n" +
			"  >> \"%MOCK_BD_LOG%\" echo(--allow-stale version\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"if /I \"%cmd%\"==\"sql\" (\r\n" +
			"  >> \"%MOCK_BD_LOG%\" echo(--allow-stale sql %sql%\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"if /I \"%cmd%\"==\"update\" (\r\n" +
			"  >> \"%MOCK_BD_LOG%\" echo(update %id% --description=\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"if /I \"%cmd%\"==\"show\" (\r\n" +
			"  >> \"%MOCK_BD_LOG%\" echo(--allow-stale show %id% --json\r\n" +
			"  echo(%MOCK_BD_SHOW_OUTPUT%\r\n" +
			"  exit /b 0\r\n" +
			")\r\n" +
			"if /I \"%cmd%\"==\"version\" exit /b 0\r\n" +
			"exit /b 0\r\n"
		if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
			t.Fatalf("write mock bd: %v", err)
		}
		t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("MOCK_BD_SHOW_OUTPUT", showOutput)
		t.Setenv("MOCK_BD_LOG", logPath)
		return
	}

	script := `#!/bin/sh
printf '%s\n' "$*" >> "$MOCK_BD_LOG"
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  version)
    exit 0
    ;;
  show)
    printf '%s\n' "$MOCK_BD_SHOW_OUTPUT"
    exit 0
    ;;
  sql|update)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	scriptPath := filepath.Join(binDir, "bd")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MOCK_BD_SHOW_OUTPUT", showOutput)
	t.Setenv("MOCK_BD_LOG", logPath)
}

func TestGetAgentBead_PrefersStructuredAgentState(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	installMockBDFixedShowOutput(t, `[{"id":"gt-gastown-polecat-nux","title":"Polecat nux","issue_type":"agent","labels":["gt:agent"],"description":"role_type: polecat\nrig: gastown\nagent_state: spawning\nhook_bead: null","agent_state":"idle"}]`)

	bd := NewIsolated(tmpDir)
	issue, fields, err := bd.GetAgentBead("gt-gastown-polecat-nux")
	if err != nil {
		t.Fatalf("GetAgentBead: %v", err)
	}
	if issue == nil {
		t.Fatal("GetAgentBead returned nil issue")
	}
	if fields == nil {
		t.Fatal("GetAgentBead returned nil fields")
	}
	if issue.AgentState != "idle" {
		t.Fatalf("issue.AgentState = %q, want %q", issue.AgentState, "idle")
	}
	if fields.AgentState != "idle" {
		t.Fatalf("fields.AgentState = %q, want %q", fields.AgentState, "idle")
	}
}

func TestGetAgentBead_FallsBackToDescriptionAgentState(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	installMockBDFixedShowOutput(t, `[{"id":"gt-gastown-polecat-nux","title":"Polecat nux","issue_type":"agent","labels":["gt:agent"],"description":"role_type: polecat\nrig: gastown\nagent_state: spawning\nhook_bead: null"}]`)

	bd := NewIsolated(tmpDir)
	_, fields, err := bd.GetAgentBead("gt-gastown-polecat-nux")
	if err != nil {
		t.Fatalf("GetAgentBead: %v", err)
	}
	if fields == nil {
		t.Fatal("GetAgentBead returned nil fields")
	}
	if fields.AgentState != "spawning" {
		t.Fatalf("fields.AgentState = %q, want %q", fields.AgentState, "spawning")
	}
}

// TestUpdateAgentState_UsesSQLAndDoesNotCallMissingBDSubcommand verifies that
// UpdateAgentState does NOT call the missing `bd agent state` subcommand and
// DOES update the description field (gt-eii: SQL column path removed in favour
// of description-only updates via UpdateAgentDescriptionFields).
func TestUpdateAgentState_UsesSQLAndDoesNotCallMissingBDSubcommand(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	logPath := filepath.Join(tmpDir, "bd.log")
	installMockBDForAgentStateUpdate(
		t,
		`[{"id":"gt-gastown-polecat-nux","title":"Polecat nux","issue_type":"agent","labels":["gt:agent"],"description":"Polecat nux\n\nrole_type: polecat\nrig: gastown\nagent_state: spawning\nhook_bead: null","agent_state":"spawning"}]`,
		logPath,
	)

	bd := NewIsolated(tmpDir)
	if err := bd.UpdateAgentState("gt-gastown-polecat-nux", "working"); err != nil {
		t.Fatalf("UpdateAgentState: %v", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logOutput := string(logBytes)

	// gt-eii: SQL column path removed — no direct bd sql call expected.
	if strings.Contains(logOutput, "sql UPDATE issues SET agent_state") {
		t.Fatalf("unexpected SQL agent_state update (removed in gt-eii) in log:\n%s", logOutput)
	}
	if strings.Contains(logOutput, "agent state gt-gastown-polecat-nux working") {
		t.Fatalf("unexpected deprecated bd agent state call in log:\n%s", logOutput)
	}
	if !strings.Contains(logOutput, "update gt-gastown-polecat-nux --description=") {
		t.Fatalf("expected description update in log, got:\n%s", logOutput)
	}
}

func TestIsAgentBeadByID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		// Full-form IDs (prefix != rig): prefix-rig-role[-name]
		{name: "full witness", id: "gt-gastown-witness", want: true},
		{name: "full refinery", id: "gt-gastown-refinery", want: true},
		{name: "full crew with name", id: "gt-gastown-crew-krystian", want: true},
		{name: "full polecat with name", id: "gt-gastown-polecat-Toast", want: true},
		{name: "full deacon", id: "sh-shippercrm-deacon", want: true},
		{name: "full mayor", id: "ax-axon-mayor", want: true},

		// Collapsed-form IDs (prefix == rig): prefix-role[-name]
		// These have only 2 parts for witness/refinery, must still be detected.
		{name: "collapsed witness", id: "bcc-witness", want: true},
		{name: "collapsed refinery", id: "bcc-refinery", want: true},
		{name: "collapsed crew with name", id: "bcc-crew-krystian", want: true},
		{name: "collapsed polecat with name", id: "bcc-polecat-obsidian", want: true},

		// Non-agent IDs
		{name: "regular issue", id: "gt-12345", want: false},
		{name: "task bead", id: "bcc-fix-button-color", want: false},
		{name: "single part", id: "witness", want: false},
		{name: "empty string", id: "", want: false},
		{name: "patrol molecule", id: "mol-patrol-abc123", want: false},
		{name: "merge request", id: "gt-mr-1234", want: false},

		// Edge cases
		{name: "role in first position", id: "witness-something", want: false},
		{name: "beads prefix collapsed", id: "bd-beads-witness", want: true},
		{name: "beads crew", id: "bd-beads-crew-krystian", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAgentBeadByID(tt.id)
			if got != tt.want {
				t.Errorf("isAgentBeadByID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestMergeAgentBeadSources(t *testing.T) {
	t.Run("issues override duplicate wisp ids", func(t *testing.T) {
		issuesByID := map[string]*Issue{
			"hq-deacon": {ID: "hq-deacon", Type: "agent", Labels: []string{"gt:agent"}},
		}
		wispsByID := map[string]*Issue{
			"hq-deacon": {ID: "hq-deacon"},
		}

		merged := mergeAgentBeadSources(issuesByID, wispsByID)
		if len(merged) != 1 {
			t.Fatalf("len(merged) = %d, want 1", len(merged))
		}
		if merged["hq-deacon"].Type != "agent" {
			t.Fatalf("merged issue type = %q, want %q", merged["hq-deacon"].Type, "agent")
		}
		if len(merged["hq-deacon"].Labels) != 1 || merged["hq-deacon"].Labels[0] != "gt:agent" {
			t.Fatalf("merged labels = %v, want [gt:agent]", merged["hq-deacon"].Labels)
		}
	})

	t.Run("wisps are included when missing from issues", func(t *testing.T) {
		issuesByID := map[string]*Issue{
			"hq-mayor": {ID: "hq-mayor", Type: "agent", Labels: []string{"gt:agent"}},
		}
		wispsByID := map[string]*Issue{
			"bom-bti_ops_match-witness": {ID: "bom-bti_ops_match-witness"},
		}

		merged := mergeAgentBeadSources(issuesByID, wispsByID)
		if len(merged) != 2 {
			t.Fatalf("len(merged) = %d, want 2", len(merged))
		}
		if _, ok := merged["hq-mayor"]; !ok {
			t.Fatalf("expected hq-mayor in merged set")
		}
		if _, ok := merged["bom-bti_ops_match-witness"]; !ok {
			t.Fatalf("expected bom-bti_ops_match-witness in merged set")
		}
	})

	t.Run("handles nil maps", func(t *testing.T) {
		merged := mergeAgentBeadSources(nil, nil)
		if len(merged) != 0 {
			t.Fatalf("len(merged) = %d, want 0", len(merged))
		}
	})
}

// TestUpdateAgentState_UsesDescriptionNotBdAgentState is a regression test for gt-eii.
// UpdateAgentState must NOT call `bd agent state` (which doesn't exist in bd)
// and instead use UpdateAgentDescriptionFields (i.e., bd update --description).
func TestUpdateAgentState_UsesDescriptionNotBdAgentState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script mock not supported on windows")
	}

	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	argsFile := filepath.Join(tmpDir, "bd_calls.txt")
	showOutput := `[{"id":"gt-gastown-polecat-nux","title":"gt-gastown-polecat-nux","issue_type":"agent","labels":["gt:agent"],"description":"gt-gastown-polecat-nux\n\nrole_type: polecat\nrig: gastown\nagent_state: spawning\nhook_bead: null\ncleanup_status: null\nactive_mr: null\nnotification_level: null"}]`

	// Mock bd that records all calls and serves show output.
	// Returns exit code 1 if called with "agent" subcommand (regression guard).
	script := `#!/bin/sh
printf '%s\n' "$@" >> ` + argsFile + `
printf '\n---\n' >> ` + argsFile + `
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) cmd="$arg"; break ;;
  esac
done
case "$cmd" in
  agent)
    # bd has no "agent" subcommand — fail to catch regressions
    echo "Error: unknown command \"agent\" for \"bd\"" >&2
    exit 1
    ;;
  version)
    exit 0
    ;;
  show)
    printf '%s\n' "$MOCK_BD_SHOW_OUTPUT"
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	scriptPath := filepath.Join(t.TempDir(), "bd")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(scriptPath)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MOCK_BD_SHOW_OUTPUT", showOutput)

	bd := NewIsolated(tmpDir)
	err := bd.UpdateAgentState("gt-gastown-polecat-nux", "working")
	if err != nil {
		t.Fatalf("UpdateAgentState returned unexpected error: %v", err)
	}

	// Verify that bd was NOT called with "agent" subcommand.
	calls, readErr := os.ReadFile(argsFile)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("reading bd calls file: %v", readErr)
	}
	if strings.Contains(string(calls), "\nagent\n") {
		t.Errorf("UpdateAgentState called `bd agent ...` which does not exist in bd (gt-eii regression):\n%s", calls)
	}
}
