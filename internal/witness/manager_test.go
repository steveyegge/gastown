package witness

import (
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestBuildWitnessStartCommand_UsesRoleConfig(t *testing.T) {
	t.Parallel()
	roleCfg := &beads.RoleConfig{
		StartCommand: "exec run --town {town} --rig {rig} --role {role}",
	}

	got, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", "", roleCfg)
	if err != nil {
		t.Fatalf("buildWitnessStartCommand: %v", err)
	}

	want := "exec run --town /town --rig gastown --role witness"
	if got != want {
		t.Errorf("buildWitnessStartCommand = %q, want %q", got, want)
	}
}

func TestBuildWitnessStartCommand_DefaultsToRuntime(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	roleCfg := &beads.RoleConfig{
		StartCommand: "exec run --role {role}",
	}

	got, err := buildWitnessStartCommand("/town/rig", "gastown", "/town", "", "codex", roleCfg)
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

func TestBuildWitnessSessionEnv_MergesRoleConfigAndOverrides(t *testing.T) {
	t.Parallel()
	roleCfg := &beads.RoleConfig{
		EnvVars: map[string]string{
			"WITNESS_HOME": "{town}/{rig}/{role}",
			"GT_ROLE":      "should-be-overridden",
		},
	}

	got := buildWitnessSessionEnv("gastown", "/town", "gt-witness", "", "run-123", nil, roleCfg, []string{
		"CUSTOM_FLAG=1",
		"GT_ROLE=override/role",
	})

	if got["GT_RUN"] != "run-123" {
		t.Fatalf("GT_RUN = %q, want %q", got["GT_RUN"], "run-123")
	}
	if got["WITNESS_HOME"] != "/town/gastown/witness" {
		t.Fatalf("WITNESS_HOME = %q, want %q", got["WITNESS_HOME"], "/town/gastown/witness")
	}
	if got["CUSTOM_FLAG"] != "1" {
		t.Fatalf("CUSTOM_FLAG = %q, want %q", got["CUSTOM_FLAG"], "1")
	}
	if got["GT_ROLE"] != "override/role" {
		t.Fatalf("GT_ROLE = %q, want %q", got["GT_ROLE"], "override/role")
	}
	if got["BD_ACTOR"] != "gastown/witness" {
		t.Fatalf("BD_ACTOR = %q, want %q", got["BD_ACTOR"], "gastown/witness")
	}
}
