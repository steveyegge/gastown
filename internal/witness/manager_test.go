package witness

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestBuildWitnessStartCommand_UsesRoleConfig(t *testing.T) {
	roleConfig := &beads.RoleConfig{
		StartCommand: "exec run --town {town} --rig {rig} --role {role}",
	}

	got, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", "", roleConfig)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	want := "exec run --town /town --rig gastown --role witness"
	if got != want {
		t.Errorf("buildWitnessStartCommand = %q, want %q", got, want)
	}
}

func TestBuildWitnessStartCommand_DefaultsToRuntime(t *testing.T) {
	got, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", "", nil)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if !strings.Contains(got, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", got)
	}
	if !strings.Contains(got, "BD_ACTOR=gastown/witness") {
		t.Errorf("expected BD_ACTOR=gastown/witness in command, got %q", got)
	}
}

func TestBuildWitnessStartCommand_AgentOverrideWins(t *testing.T) {
	roleConfig := &beads.RoleConfig{
		StartCommand: "exec run --role {role}",
	}

	got, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "codex", "", roleConfig)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}
	if strings.Contains(got, "exec run") {
		t.Fatalf("expected agent override to bypass role start_command, got %q", got)
	}
	if !strings.Contains(got, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", got)
	}
}

func TestBuildWitnessStartCommand_IncludesClaudeConfigDir(t *testing.T) {
	got, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", "/custom/config", nil)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if !strings.Contains(got, "CLAUDE_CONFIG_DIR=/custom/config") {
		t.Errorf("expected CLAUDE_CONFIG_DIR=/custom/config in command, got %q", got)
	}
}

func TestBuildWitnessStartCommand_OmitsClaudeConfigDirWhenEmpty(t *testing.T) {
	got, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", "", nil)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if strings.Contains(got, "CLAUDE_CONFIG_DIR") {
		t.Errorf("expected no CLAUDE_CONFIG_DIR when empty, got %q", got)
	}
}
