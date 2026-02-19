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

	command, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", roleConfig)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	want := "exec run --town /town --rig gastown --role witness"
	if command != want {
		t.Errorf("buildWitnessStartCommand = %q, want %q", command, want)
	}
}

func TestBuildWitnessStartCommand_DefaultsToRuntime(t *testing.T) {
	command, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", nil)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if !strings.Contains(command, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", command)
	}
	if !strings.Contains(command, "BD_ACTOR=gastown/witness") {
		t.Errorf("expected BD_ACTOR=gastown/witness in command, got %q", command)
	}
}

func TestBuildWitnessStartCommand_AgentOverrideWins(t *testing.T) {
	roleConfig := &beads.RoleConfig{
		StartCommand: "exec run --role {role}",
	}

	command, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "codex", roleConfig)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}
	if strings.Contains(command, "exec run") {
		t.Fatalf("expected agent override to bypass role start_command, got %q", command)
	}
	if !strings.Contains(command, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", command)
	}
}
