package beads

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestGetAgentBead_PrefersDescriptionAgentState verifies that description-parsed
// agent_state is authoritative over the database column (gt-3hn3).
// The agent_state column is no longer maintained since bd removed `bd agent state`.
func TestGetAgentBead_PrefersDescriptionAgentState(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Description says "working" (truth), DB column says "spawning" (stale)
	installMockBDFixedShowOutput(t, `[{"id":"gt-gastown-polecat-nux","title":"Polecat nux","issue_type":"agent","labels":["gt:agent"],"description":"role_type: polecat\nrig: gastown\nagent_state: working\nhook_bead: null","agent_state":"spawning"}]`)

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
	// Column still shows stale value
	if issue.AgentState != "spawning" {
		t.Fatalf("issue.AgentState = %q, want %q (raw column)", issue.AgentState, "spawning")
	}
	// Fields should use description (authoritative), not column
	if fields.AgentState != "working" {
		t.Fatalf("fields.AgentState = %q, want %q (from description)", fields.AgentState, "working")
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
