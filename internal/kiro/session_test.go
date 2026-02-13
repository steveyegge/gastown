package kiro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/session"
)

// ---------- 1. TestKiroPresetConfiguration ----------

func TestKiroPresetConfiguration(t *testing.T) {
	t.Parallel()
	info := config.GetAgentPreset(config.AgentKiro)
	if info == nil {
		t.Fatal("GetAgentPreset(AgentKiro) returned nil")
	}

	// Command
	if info.Command != "kiro" {
		t.Errorf("Command = %q, want %q", info.Command, "kiro")
	}

	// Args
	wantArgs := []string{"--trust-all-tools"}
	if len(info.Args) != len(wantArgs) {
		t.Fatalf("Args = %v, want %v", info.Args, wantArgs)
	}
	for i, a := range wantArgs {
		if info.Args[i] != a {
			t.Errorf("Args[%d] = %q, want %q", i, info.Args[i], a)
		}
	}

	// SupportsHooks
	if !info.SupportsHooks {
		t.Error("SupportsHooks = false, want true")
	}

	// ResumeFlag
	if info.ResumeFlag != "--resume" {
		t.Errorf("ResumeFlag = %q, want %q", info.ResumeFlag, "--resume")
	}

	// ProcessNames
	wantProcessNames := []string{"kiro", "node"}
	if len(info.ProcessNames) != len(wantProcessNames) {
		t.Fatalf("ProcessNames = %v, want %v", info.ProcessNames, wantProcessNames)
	}
	for i, pn := range wantProcessNames {
		if info.ProcessNames[i] != pn {
			t.Errorf("ProcessNames[%d] = %q, want %q", i, info.ProcessNames[i], pn)
		}
	}
}

// ---------- 2. TestKiroAutonomousRoles ----------

func TestKiroAutonomousRoles(t *testing.T) {
	t.Parallel()
	autonomousRoles := []string{"polecat", "witness", "refinery", "deacon"}
	for _, role := range autonomousRoles {
		t.Run(role, func(t *testing.T) {
			if got := RoleTypeFor(role); got != Autonomous {
				t.Errorf("RoleTypeFor(%q) = %q, want %q", role, got, Autonomous)
			}
		})
	}
}

// ---------- 3. TestKiroInteractiveRoles ----------

func TestKiroInteractiveRoles(t *testing.T) {
	t.Parallel()
	interactiveRoles := []string{"mayor", "crew"}
	for _, role := range interactiveRoles {
		t.Run(role, func(t *testing.T) {
			if got := RoleTypeFor(role); got != Interactive {
				t.Errorf("RoleTypeFor(%q) = %q, want %q", role, got, Interactive)
			}
		})
	}
}

// ---------- 4. TestKiroAutonomousSettings ----------

func TestKiroAutonomousSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := EnsureSettingsAt(dir, Autonomous, ".kiro", "settings.json"); err != nil {
		t.Fatalf("EnsureSettingsAt(Autonomous) error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".kiro", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings file: %v", err)
	}
	content := string(data)

	// Autonomous SessionStart should include gt mail check --inject
	if !strings.Contains(content, "gt mail check --inject") {
		t.Error("autonomous settings missing 'gt mail check --inject' in SessionStart")
	}

	// Verify it appears in the SessionStart section specifically
	// by parsing the JSON and checking the SessionStart hook command
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings JSON: %v", err)
	}
	sessionStartHooks, ok := settings.Hooks["SessionStart"]
	if !ok || len(sessionStartHooks) == 0 {
		t.Fatal("no SessionStart hooks found")
	}
	found := false
	for _, entry := range sessionStartHooks {
		for _, h := range entry.Hooks {
			if strings.Contains(h.Command, "gt mail check --inject") {
				found = true
			}
		}
	}
	if !found {
		t.Error("autonomous SessionStart hook command does not contain 'gt mail check --inject'")
	}
}

// ---------- 5. TestKiroInteractiveSettings ----------

func TestKiroInteractiveSettings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := EnsureSettingsAt(dir, Interactive, ".kiro", "settings.json"); err != nil {
		t.Fatalf("EnsureSettingsAt(Interactive) error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".kiro", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings file: %v", err)
	}

	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings JSON: %v", err)
	}

	// SessionStart should NOT include gt mail check --inject
	sessionStartHooks := settings.Hooks["SessionStart"]
	for _, entry := range sessionStartHooks {
		for _, h := range entry.Hooks {
			if strings.Contains(h.Command, "gt mail check --inject") {
				t.Error("interactive SessionStart should NOT contain 'gt mail check --inject'")
			}
		}
	}

	// UserPromptSubmit SHOULD include gt mail check --inject
	userPromptHooks := settings.Hooks["UserPromptSubmit"]
	if len(userPromptHooks) == 0 {
		t.Fatal("no UserPromptSubmit hooks found in interactive settings")
	}
	found := false
	for _, entry := range userPromptHooks {
		for _, h := range entry.Hooks {
			if strings.Contains(h.Command, "gt mail check --inject") {
				found = true
			}
		}
	}
	if !found {
		t.Error("interactive UserPromptSubmit should contain 'gt mail check --inject'")
	}
}

// ---------- 6. TestKiroSettingsNoClaudeReferences ----------

func TestKiroSettingsNoClaudeReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		roleType RoleType
	}{
		{"autonomous", Autonomous},
		{"interactive", Interactive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := EnsureSettingsAt(dir, tt.roleType, ".kiro", "settings.json"); err != nil {
				t.Fatalf("EnsureSettingsAt(%s) error = %v", tt.name, err)
			}

			data, err := os.ReadFile(filepath.Join(dir, ".kiro", "settings.json"))
			if err != nil {
				t.Fatalf("reading settings: %v", err)
			}
			content := strings.ToLower(string(data))

			// Must NOT contain "claude" anywhere
			if strings.Contains(content, "claude") {
				t.Errorf("%s settings contain reference to 'claude' - Kiro must operate independently", tt.name)
			}

			// Must NOT contain ".claude" directory references
			if strings.Contains(content, ".claude") {
				t.Errorf("%s settings contain reference to '.claude' - Kiro uses .kiro directory", tt.name)
			}
		})
	}
}

// ---------- 7. TestKiroHookLifecycle ----------

func TestKiroHookLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		roleType RoleType
	}{
		{"autonomous", Autonomous},
		{"interactive", Interactive},
	}

	requiredEvents := []string{"SessionStart", "UserPromptSubmit", "PreCompact", "Stop"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := EnsureSettingsAt(dir, tt.roleType, ".kiro", "settings.json"); err != nil {
				t.Fatalf("EnsureSettingsAt(%s) error = %v", tt.name, err)
			}

			data, err := os.ReadFile(filepath.Join(dir, ".kiro", "settings.json"))
			if err != nil {
				t.Fatalf("reading settings: %v", err)
			}

			var settings struct {
				Hooks map[string]json.RawMessage `json:"hooks"`
			}
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatalf("parsing settings JSON: %v", err)
			}

			for _, event := range requiredEvents {
				if _, ok := settings.Hooks[event]; !ok {
					t.Errorf("%s settings missing required hook event %q", tt.name, event)
				}
			}
		})
	}
}

// ---------- 8. TestKiroSettingsFilePermissions ----------

func TestKiroSettingsFilePermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		roleType RoleType
	}{
		{"autonomous", Autonomous},
		{"interactive", Interactive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := EnsureSettingsAt(dir, tt.roleType, ".kiro", "settings.json"); err != nil {
				t.Fatalf("EnsureSettingsAt(%s) error = %v", tt.name, err)
			}

			path := filepath.Join(dir, ".kiro", "settings.json")
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat settings file: %v", err)
			}

			if perm := info.Mode().Perm(); perm != 0600 {
				t.Errorf("file permissions = %o, want 0600", perm)
			}
		})
	}
}

// ---------- 9. TestKiroSessionBeacon ----------

func TestKiroSessionBeacon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     session.BeaconConfig
		wantSub []string
	}{
		{
			name: "polecat assigned with mol-id",
			cfg: session.BeaconConfig{
				Recipient: "gastown/polecats/bravo",
				Sender:    "deacon",
				Topic:     "assigned",
				MolID:     "gt-kiro01",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/polecats/bravo",
				"<- deacon",
				"assigned:gt-kiro01",
				"Work is on your hook",
				"gt hook",
			},
		},
		{
			name: "polecat cold-start",
			cfg: session.BeaconConfig{
				Recipient: "gastown/polecats/bravo",
				Sender:    "mayor",
				Topic:     "cold-start",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/polecats/bravo",
				"<- mayor",
				"cold-start",
				"Check your hook and mail",
			},
		},
		{
			name: "polecat start includes fallback",
			cfg: session.BeaconConfig{
				Recipient: "gastown/polecats/bravo",
				Sender:    "human",
				Topic:     "start",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/polecats/bravo",
				"<- human",
				"start",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := session.FormatStartupBeacon(tt.cfg)
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("FormatStartupBeacon() = %q, missing %q", got, sub)
				}
			}
		})
	}
}

// ---------- 10. TestKiroNonInteractiveConfig ----------

func TestKiroNonInteractiveConfig(t *testing.T) {
	t.Parallel()
	info := config.GetAgentPreset(config.AgentKiro)
	if info == nil {
		t.Fatal("GetAgentPreset(AgentKiro) returned nil")
	}

	if info.NonInteractive == nil {
		t.Fatal("Kiro NonInteractive config is nil")
	}

	if info.NonInteractive.PromptFlag != "-p" {
		t.Errorf("NonInteractive.PromptFlag = %q, want %q", info.NonInteractive.PromptFlag, "-p")
	}

	if info.NonInteractive.OutputFlag != "--output json" {
		t.Errorf("NonInteractive.OutputFlag = %q, want %q", info.NonInteractive.OutputFlag, "--output json")
	}
}
