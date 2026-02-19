package tmuxinator

import (
	"os"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

func TestFromSessionConfig_RequiresSessionID(t *testing.T) {
	_, err := FromSessionConfig(SessionConfig{
		WorkDir: "/tmp",
		Role:    "polecat",
	})
	if err == nil {
		t.Fatal("expected error for missing SessionID")
	}
	if !strings.Contains(err.Error(), "SessionID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFromSessionConfig_RequiresWorkDir(t *testing.T) {
	_, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-test",
		Role:      "polecat",
	})
	if err == nil {
		t.Fatal("expected error for missing WorkDir")
	}
}

func TestFromSessionConfig_RequiresRole(t *testing.T) {
	_, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-test",
		WorkDir:   "/tmp",
	})
	if err == nil {
		t.Fatal("expected error for missing Role")
	}
}

func TestFromSessionConfig_BasicConfig(t *testing.T) {
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-myrig-Toast",
		WorkDir:   "/home/user/gt/myrig/polecats/Toast",
		Role:      "polecat",
		TownRoot:  "/home/user/gt",
		RigName:   "myrig",
		AgentName: "Toast",
		Command:   "claude --dangerously-skip-permissions -p 'hello'",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Name != "gt-myrig-Toast" {
		t.Errorf("Name = %q, want %q", cfg.Name, "gt-myrig-Toast")
	}
	if cfg.Root != "/home/user/gt/myrig/polecats/Toast" {
		t.Errorf("Root = %q", cfg.Root)
	}
	if cfg.Attach {
		t.Error("Attach should be false")
	}
	if len(cfg.Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(cfg.Windows))
	}
	if cfg.Windows[0].Name != "agent" {
		t.Errorf("window name = %q", cfg.Windows[0].Name)
	}
	if len(cfg.Windows[0].Panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(cfg.Windows[0].Panes))
	}
	if cfg.Windows[0].Panes[0] != "claude --dangerously-skip-permissions -p 'hello'" {
		t.Errorf("pane command = %q", cfg.Windows[0].Panes[0])
	}
}

func TestFromSessionConfig_EnvVars(t *testing.T) {
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-myrig-Toast",
		WorkDir:   "/tmp",
		Role:      "polecat",
		TownRoot:  "/home/user/gt",
		RigName:   "myrig",
		AgentName: "Toast",
		Command:   "echo test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that environment variables are set in on_project_start
	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	// Should contain GT_ROLE for polecat
	if !strings.Contains(yaml, "GT_ROLE") {
		t.Error("YAML should contain GT_ROLE")
	}
	if !strings.Contains(yaml, "set-environment") {
		t.Error("YAML should contain set-environment commands")
	}
}

func TestFromSessionConfig_Theme(t *testing.T) {
	theme := tmux.Theme{Name: "ocean", BG: "#1e3a5f", FG: "#e0e0e0"}
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-myrig-Toast",
		WorkDir:   "/tmp",
		Role:      "polecat",
		RigName:   "myrig",
		AgentName: "Toast",
		Command:   "echo test",
		Theme:     &theme,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	// Should contain theme settings
	if !strings.Contains(yaml, "status-style") {
		t.Error("YAML should contain status-style")
	}
	if !strings.Contains(yaml, "#1e3a5f") {
		t.Error("YAML should contain theme background color")
	}
	// Should contain mouse mode
	if !strings.Contains(yaml, "mouse on") {
		t.Error("YAML should contain mouse on")
	}
	// Should contain key bindings
	if !strings.Contains(yaml, "bind-key") {
		t.Error("YAML should contain key bindings")
	}
	// Should contain status-left with emoji
	if !strings.Contains(yaml, "status-left") {
		t.Error("YAML should contain status-left")
	}
}

func TestFromSessionConfig_NoTheme(t *testing.T) {
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-test",
		WorkDir:   "/tmp",
		Role:      "polecat",
		Command:   "echo test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	// Without theme, should not contain status-style or bindings
	if strings.Contains(yaml, "status-style") {
		t.Error("YAML should not contain status-style without theme")
	}
	if strings.Contains(yaml, "bind-key") {
		t.Error("YAML should not contain key bindings without theme")
	}
}

func TestFromSessionConfig_RemainOnExit(t *testing.T) {
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID:    "gt-test",
		WorkDir:      "/tmp",
		Role:         "deacon",
		Command:      "echo test",
		RemainOnExit: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "remain-on-exit on") {
		t.Error("YAML should contain remain-on-exit on")
	}
}

func TestFromSessionConfig_AutoRespawn(t *testing.T) {
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID:   "gt-test",
		WorkDir:     "/tmp",
		Role:        "deacon",
		Command:     "echo test",
		AutoRespawn: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "pane-died") {
		t.Error("YAML should contain pane-died hook")
	}
	if !strings.Contains(yaml, "respawn-pane") {
		t.Error("YAML should contain respawn-pane command")
	}
}

func TestFromSessionConfig_ExtraEnv(t *testing.T) {
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-test",
		WorkDir:   "/tmp",
		Role:      "polecat",
		RigName:   "myrig",
		AgentName: "Toast",
		Command:   "echo test",
		ExtraEnv: map[string]string{
			"BD_BRANCH":           "feature/test",
			"BD_DOLT_AUTO_COMMIT": "off",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	if !strings.Contains(yaml, "BD_BRANCH") {
		t.Error("YAML should contain extra env var BD_BRANCH")
	}
	if !strings.Contains(yaml, "BD_DOLT_AUTO_COMMIT") {
		t.Error("YAML should contain extra env var BD_DOLT_AUTO_COMMIT")
	}
}

func TestFromSessionConfig_DeaconRole(t *testing.T) {
	theme := tmux.DeaconTheme()
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID:    "hq-deacon",
		WorkDir:      "/home/user/gt/deacon",
		Role:         "deacon",
		TownRoot:     "/home/user/gt",
		Command:      "claude --dangerously-skip-permissions -p 'patrol'",
		Theme:        &theme,
		RemainOnExit: true,
		AutoRespawn:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	// Deacon should have remain-on-exit, auto-respawn, theme
	if !strings.Contains(yaml, "remain-on-exit on") {
		t.Error("deacon YAML should contain remain-on-exit")
	}
	if !strings.Contains(yaml, "pane-died") {
		t.Error("deacon YAML should contain auto-respawn hook")
	}
	if !strings.Contains(yaml, "status-style") {
		t.Error("deacon YAML should contain theme")
	}
}

func TestFromSessionConfig_MayorRole(t *testing.T) {
	theme := tmux.MayorTheme()
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID: "hq-mayor",
		WorkDir:   "/home/user/gt/mayor",
		Role:      "mayor",
		TownRoot:  "/home/user/gt",
		Command:   "claude --dangerously-skip-permissions -p 'start'",
		Theme:     &theme,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	// Mayor is town-level (no rig), should use worker name in status-left
	if !strings.Contains(yaml, "status-left") {
		t.Error("mayor YAML should contain status-left")
	}
}

func TestFromSessionConfig_CrewRole(t *testing.T) {
	cfg, err := FromSessionConfig(SessionConfig{
		SessionID: "gt-myrig-gus",
		WorkDir:   "/home/user/gt/myrig/crew/gus/rig",
		Role:      "crew",
		TownRoot:  "/home/user/gt",
		RigName:   "myrig",
		AgentName: "gus",
		Command:   "claude --dangerously-skip-permissions -p 'ready'",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}
	yaml := string(yamlBytes)

	// Crew env vars
	if !strings.Contains(yaml, "GT_CREW") {
		t.Error("crew YAML should contain GT_CREW env var")
	}
}

func TestToYAML_WellFormed(t *testing.T) {
	cfg := &Config{
		Name:   "test-session",
		Root:   "/tmp",
		Attach: false,
		OnProjectStart: []string{
			"tmux set-environment -t test-session GT_ROLE polecat",
		},
		Windows: []Window{
			{
				Name:  "agent",
				Panes: []string{"echo hello"},
			},
		},
	}

	yamlBytes, err := cfg.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML error: %v", err)
	}

	yaml := string(yamlBytes)

	// Basic structure checks
	if !strings.Contains(yaml, "name: test-session") {
		t.Errorf("YAML should contain name")
	}
	if !strings.Contains(yaml, "root: /tmp") {
		t.Errorf("YAML should contain root")
	}
	if !strings.Contains(yaml, "attach: false") {
		t.Errorf("YAML should contain attach: false")
	}
	if !strings.Contains(yaml, "on_project_start:") {
		t.Errorf("YAML should contain on_project_start")
	}
	if !strings.Contains(yaml, "windows:") {
		t.Errorf("YAML should contain windows")
	}
}

func TestRoleIcon(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"mayor", "🎩"},
		{"deacon", "🐺"},
		{"witness", "🦉"},
		{"refinery", "🏭"},
		{"crew", "👷"},
		{"polecat", "😺"},
		{"coordinator", "🎩"},
		{"health-check", "🐺"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := roleIcon(tt.role)
		if got != tt.want {
			t.Errorf("roleIcon(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"", "''"},
		{"has space", "'has space'"},
		{"has'quote", "'has'\\''quote'"},
		{"bg=#1e3a5f,fg=#e0e0e0", "'bg=#1e3a5f,fg=#e0e0e0'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWriteToFile(t *testing.T) {
	cfg := &Config{
		Name:   "test",
		Root:   "/tmp",
		Attach: false,
		Windows: []Window{
			{Name: "main", Panes: []string{"echo test"}},
		},
	}

	path := t.TempDir() + "/test.yml"
	if err := cfg.WriteToFile(path); err != nil {
		t.Fatalf("WriteToFile error: %v", err)
	}

	// Verify file exists and has content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if len(data) == 0 {
		t.Error("written file is empty")
	}
	if !strings.Contains(string(data), "name: test") {
		t.Error("written file should contain config")
	}
}
