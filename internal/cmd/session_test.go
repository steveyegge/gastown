package cmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/polecat"
)

func TestSessionInfoJSONOutput(t *testing.T) {
	info := &polecat.SessionInfo{
		Polecat:   "alpha",
		SessionID: "gt-alpha",
		Running:   true,
		RigName:   "gastown",
		Attached:  false,
		Created:   time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC),
		Windows:   1,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["polecat"] != "alpha" {
		t.Errorf("polecat = %v, want alpha", parsed["polecat"])
	}
	if parsed["session_id"] != "gt-alpha" {
		t.Errorf("session_id = %v, want gt-alpha", parsed["session_id"])
	}
	if parsed["running"] != true {
		t.Errorf("running = %v, want true", parsed["running"])
	}
	if parsed["rig_name"] != "gastown" {
		t.Errorf("rig_name = %v, want gastown", parsed["rig_name"])
	}
}

func TestSessionStatusCmdJSONFlagWiring(t *testing.T) {
	// Verify --json flag is registered on the session status command.
	// This catches regressions where flag binding is accidentally removed,
	// which would silently break formulas that depend on --json output.
	f := sessionStatusCmd.Flags().Lookup("json")
	if f == nil {
		t.Fatal("session status command missing --json flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--json default = %q, want \"false\"", f.DefValue)
	}
}

func TestSessionInfoJSONOutputNotRunning(t *testing.T) {
	info := &polecat.SessionInfo{
		Polecat:   "beta",
		SessionID: "gt-beta",
		Running:   false,
		RigName:   "testrig",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["running"] != false {
		t.Errorf("running = %v, want false", parsed["running"])
	}
}

func TestSelectSessionIssue(t *testing.T) {
	tests := []struct {
		name          string
		explicit      string
		polecatIssue  string
		polecatBranch string
		agentHook     string
		want          string
	}{
		{
			name:          "explicit issue wins",
			explicit:      "tg-explicit",
			polecatIssue:  "tg-live",
			polecatBranch: "polecat/furiosa/tg-branch@mk123",
			agentHook:     "tg-hook",
			want:          "tg-explicit",
		},
		{
			name:          "active polecat issue wins over branch and hook",
			polecatIssue:  "tg-live",
			polecatBranch: "polecat/furiosa/tg-branch@mk123",
			agentHook:     "tg-hook",
			want:          "tg-live",
		},
		{
			name:          "issue branch wins over stale hook",
			polecatBranch: "polecat/furiosa/tg-branch@mk123",
			agentHook:     "tg-stale",
			want:          "tg-branch",
		},
		{
			name:          "agent hook rescues base branch restart",
			polecatBranch: "codex/gamejam-webgpu-rig",
			agentHook:     "gj-er8.11",
			want:          "gj-er8.11",
		},
		{
			name: "empty when no source exists",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectSessionIssue(tt.explicit, tt.polecatIssue, tt.polecatBranch, tt.agentHook)
			if got != tt.want {
				t.Errorf("selectSessionIssue() = %q, want %q", got, tt.want)
			}
		})
	}
}
