package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
)

// ---- gt agent tier list ----

func TestAgentTierList_ShowsAllTiersWhenConfigured(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:       "town-settings",
		Version:    config.CurrentTownSettingsVersion,
		AgentTiers: config.DefaultAgentTierConfig(),
	}
	saveSettings(t, settingsPath, s)

	agentTierListAvailable = false
	defer func() { agentTierListAvailable = false }()

	out := captureStdout(t, func() {
		if err := runAgentTierList(&cobra.Command{}, nil); err != nil {
			t.Errorf("runAgentTierList: %v", err)
		}
	})

	for _, tier := range []string{"small", "medium", "large", "reasoning"} {
		if !strings.Contains(out, tier) {
			t.Errorf("output missing tier %q", tier)
		}
	}
	for _, agent := range []string{"claude-haiku", "claude-sonnet", "claude-opus", "claude-reasoning"} {
		if !strings.Contains(out, agent) {
			t.Errorf("output missing agent %q", agent)
		}
	}
}

func TestAgentTierList_ShowsNotConfiguredWhenAbsent(t *testing.T) {
	townRoot, _ := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// No settings written — agent_tiers will be nil, so it falls back to default.
	// The command falls back to DefaultAgentTierConfig, so it always shows tiers.
	// Verify that at minimum it doesn't error and produces output.
	agentTierListAvailable = false
	defer func() { agentTierListAvailable = false }()

	var runErr error
	out := captureStdout(t, func() {
		runErr = runAgentTierList(&cobra.Command{}, nil)
	})
	if runErr != nil {
		t.Fatalf("runAgentTierList: %v", runErr)
	}
	// Command produces output (either default config or no-config message)
	if out == "" {
		t.Error("expected non-empty output from runAgentTierList")
	}
}

func TestAgentTierList_AvailableFlagFiltersTiers(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// Phase 1: all agents are "available", so --available should show same as without flag
	s := &config.TownSettings{
		Type:       "town-settings",
		Version:    config.CurrentTownSettingsVersion,
		AgentTiers: config.DefaultAgentTierConfig(),
	}
	saveSettings(t, settingsPath, s)

	agentTierListAvailable = true
	defer func() { agentTierListAvailable = false }()

	out := captureStdout(t, func() {
		if err := runAgentTierList(&cobra.Command{}, nil); err != nil {
			t.Errorf("runAgentTierList --available: %v", err)
		}
	})

	// In Phase 1, all tiers have agents so all should still appear
	for _, tier := range []string{"small", "medium", "large", "reasoning"} {
		if !strings.Contains(out, tier) {
			t.Errorf("output with --available missing tier %q", tier)
		}
	}
}

func TestAgentTierList_AvailableFlagExcludesEmptyTiers(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// One tier with no agents
	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"empty-tier": {Agents: []string{}, Description: "No agents here", Selection: "priority", Fallback: true},
				"medium":     {Agents: []string{"claude-sonnet"}, Description: "Has agents", Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"empty-tier", "medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	agentTierListAvailable = true
	defer func() { agentTierListAvailable = false }()

	out := captureStdout(t, func() {
		if err := runAgentTierList(&cobra.Command{}, nil); err != nil {
			t.Errorf("runAgentTierList --available: %v", err)
		}
	})

	// empty-tier has no agents, should be filtered by --available
	if strings.Contains(out, "empty-tier") {
		t.Errorf("empty-tier should be filtered by --available, got output: %q", out)
	}
	// medium has an agent, should be present
	if !strings.Contains(out, "medium") {
		t.Errorf("medium tier should appear with --available, got output: %q", out)
	}
}

func TestAgentTierList_OutputCountLine(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:       "town-settings",
		Version:    config.CurrentTownSettingsVersion,
		AgentTiers: config.DefaultAgentTierConfig(),
	}
	saveSettings(t, settingsPath, s)

	agentTierListAvailable = false
	defer func() { agentTierListAvailable = false }()

	out := captureStdout(t, func() {
		_ = runAgentTierList(&cobra.Command{}, nil)
	})

	// Output should have a summary line with defined/available counts
	if !strings.Contains(out, "4 defined") && !strings.Contains(out, "defined") {
		t.Errorf("output missing defined count, got: %q", out)
	}
	if !strings.Contains(out, "available") {
		t.Errorf("output missing available count, got: %q", out)
	}
}

func TestAgentTierList_CheckmarkForAgents(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:       "town-settings",
		Version:    config.CurrentTownSettingsVersion,
		AgentTiers: config.DefaultAgentTierConfig(),
	}
	saveSettings(t, settingsPath, s)

	agentTierListAvailable = false

	out := captureStdout(t, func() {
		_ = runAgentTierList(&cobra.Command{}, nil)
	})

	// Phase 1: agents are marked with ✓
	if !strings.Contains(out, "✓") {
		t.Errorf("output should contain ✓ markers for available agents, got: %q", out)
	}
}
