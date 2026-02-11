package witness

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

func TestBuildWitnessStartCommand_UsesRoleConfig(t *testing.T) {
	roleConfig := &beads.RoleConfig{
		StartCommand: "exec run --town {town} --rig {rig} --role {role}",
	}

	got, prompt, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", roleConfig, config.DefaultRuntimeConfig())
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	want := "exec run --town /town --rig gastown --role witness"
	if got != want {
		t.Errorf("buildWitnessStartCommand = %q, want %q", got, want)
	}
	if prompt != "" {
		t.Errorf("prompt = %q, want empty for role-config start command", prompt)
	}
}

func TestBuildWitnessStartCommand_DefaultsToRuntime(t *testing.T) {
	got, prompt, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", nil, config.DefaultRuntimeConfig())
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	if !strings.Contains(got, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", got)
	}
	if !strings.Contains(got, "BD_ACTOR=gastown/witness") {
		t.Errorf("expected BD_ACTOR=gastown/witness in command, got %q", got)
	}
	if prompt == "" {
		t.Error("expected non-empty startup prompt for runtime startup command")
	}
}

func TestBuildWitnessStartCommand_AgentOverrideWins(t *testing.T) {
	roleConfig := &beads.RoleConfig{
		StartCommand: "exec run --role {role}",
	}

	got, prompt, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "codex", roleConfig, config.DefaultRuntimeConfig())
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}
	if strings.Contains(got, "exec run") {
		t.Fatalf("expected agent override to bypass role start_command, got %q", got)
	}
	if !strings.Contains(got, "GT_ROLE=gastown/witness") {
		t.Errorf("expected GT_ROLE=gastown/witness in command, got %q", got)
	}
	if prompt == "" {
		t.Error("expected non-empty startup prompt when using runtime startup command")
	}
}
