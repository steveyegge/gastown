package witness

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/runtime"
)

func TestBuildWitnessStartCommand_UsesRoleConfig(t *testing.T) {
	roleConfig := &beads.RoleConfig{
		StartCommand: "exec run --town {town} --rig {rig} --role {role}",
	}

	result, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", roleConfig, &runtime.StartupFallbackInfo{})
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	want := "exec run --town /town --rig gastown --role witness"
	if result.command != want {
		t.Errorf("buildWitnessStartCommand = %q, want %q", result.command, want)
	}
}

func TestBuildWitnessStartCommand_DefaultsToRuntime(t *testing.T) {
	result, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", nil, &runtime.StartupFallbackInfo{})
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if !strings.Contains(result.command, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", result.command)
	}
	if !strings.Contains(result.command, "BD_ACTOR=gastown/witness") {
		t.Errorf("expected BD_ACTOR=gastown/witness in command, got %q", result.command)
	}
}

func TestBuildWitnessStartCommand_AgentOverrideWins(t *testing.T) {
	roleConfig := &beads.RoleConfig{
		StartCommand: "exec run --role {role}",
	}

	result, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "codex", roleConfig, &runtime.StartupFallbackInfo{})
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}
	if strings.Contains(result.command, "exec run") {
		t.Fatalf("expected agent override to bypass role start_command, got %q", result.command)
	}
	if !strings.Contains(result.command, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", result.command)
	}
}
