package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
)

// setupTierTestTown creates a minimal Gas Town workspace for tier tests.
func setupTierTestTown(t *testing.T) (townRoot string, settingsPath string) {
	t.Helper()
	townRoot = t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	townCfg := &config.TownConfig{
		Type:       "town",
		Version:    config.CurrentTownVersion,
		Name:       "test-town",
		PublicName: "Test Town",
		CreatedAt:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := config.SaveTownConfig(filepath.Join(mayorDir, "town.json"), townCfg); err != nil {
		t.Fatalf("save town.json: %v", err)
	}

	rigsCfg := &config.RigsConfig{
		Version: 1,
		Rigs:    make(map[string]config.RigEntry),
	}
	if err := config.SaveRigsConfig(filepath.Join(mayorDir, "rigs.json"), rigsCfg); err != nil {
		t.Fatalf("save rigs.json: %v", err)
	}

	settingsPath = config.TownSettingsPath(townRoot)
	return townRoot, settingsPath
}

// chdirTo changes to dir and restores on cleanup.
func chdirTo(t *testing.T, dir string) {
	t.Helper()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
}

// loadSettings loads settings from the given path (must exist).
func loadSettings(t *testing.T, settingsPath string) *config.TownSettings {
	t.Helper()
	s, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	return s
}

// saveSettings writes settings to disk.
func saveSettings(t *testing.T, settingsPath string, s *config.TownSettings) {
	t.Helper()
	if err := config.SaveTownSettings(settingsPath, s); err != nil {
		t.Fatalf("save settings: %v", err)
	}
}

// settingsJSON returns the raw JSON of the settings file for deep inspection.
func settingsJSON(t *testing.T, settingsPath string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	return out
}

// ---- gt config agent tiers init ----

func TestConfigAgentTiersInit_CreatesDefault(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	err := runConfigAgentTiersInit(&cobra.Command{}, nil)
	if err != nil {
		t.Fatalf("runConfigAgentTiersInit: %v", err)
	}

	s := loadSettings(t, settingsPath)
	if s.AgentTiers == nil {
		t.Fatal("AgentTiers is nil after init")
	}

	def := config.DefaultAgentTierConfig()
	for name := range def.Tiers {
		if _, ok := s.AgentTiers.Tiers[name]; !ok {
			t.Errorf("default tier %q missing after init", name)
		}
	}
	if len(s.AgentTiers.TierOrder) != len(def.TierOrder) {
		t.Errorf("TierOrder len = %d, want %d", len(s.AgentTiers.TierOrder), len(def.TierOrder))
	}
}

func TestConfigAgentTiersInit_NoopWhenAlreadyConfigured(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// Pre-write a custom tier config
	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"custom": {Description: "Custom tier", Agents: []string{"my-agent"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"custom"},
		},
	}
	saveSettings(t, settingsPath, s)

	out := captureStdout(t, func() {
		if err := runConfigAgentTiersInit(&cobra.Command{}, nil); err != nil {
			t.Errorf("runConfigAgentTiersInit: %v", err)
		}
	})

	if !strings.Contains(out, "already configured") {
		t.Errorf("expected 'already configured' message, got: %q", out)
	}

	// Config must be unchanged
	after := loadSettings(t, settingsPath)
	if _, ok := after.AgentTiers.Tiers["custom"]; !ok {
		t.Error("custom tier was overwritten by init")
	}
	if _, ok := after.AgentTiers.Tiers["small"]; ok {
		t.Error("default tier 'small' should not exist when init is a no-op")
	}
}

func TestConfigAgentTiersInit_WrittenConfigMatchesDefault(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	if err := runConfigAgentTiersInit(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runConfigAgentTiersInit: %v", err)
	}

	s := loadSettings(t, settingsPath)
	def := config.DefaultAgentTierConfig()

	for name, wantTier := range def.Tiers {
		gotTier, ok := s.AgentTiers.Tiers[name]
		if !ok {
			t.Errorf("tier %q missing", name)
			continue
		}
		if gotTier.Description != wantTier.Description {
			t.Errorf("tier %q description = %q, want %q", name, gotTier.Description, wantTier.Description)
		}
		if len(gotTier.Agents) != len(wantTier.Agents) {
			t.Errorf("tier %q agents count = %d, want %d", name, len(gotTier.Agents), len(wantTier.Agents))
		}
		if gotTier.Fallback != wantTier.Fallback {
			t.Errorf("tier %q fallback = %v, want %v", name, gotTier.Fallback, wantTier.Fallback)
		}
	}
}

// ---- gt config agent tiers show ----

func TestConfigAgentTiersShow_WhenPresent(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:       "town-settings",
		Version:    config.CurrentTownSettingsVersion,
		AgentTiers: config.DefaultAgentTierConfig(),
	}
	saveSettings(t, settingsPath, s)

	out := captureStdout(t, func() {
		if err := runConfigAgentTiersShow(&cobra.Command{}, nil); err != nil {
			t.Errorf("runConfigAgentTiersShow: %v", err)
		}
	})

	// Output must contain tier names, agents, selection, role defaults
	for name := range s.AgentTiers.Tiers {
		if !strings.Contains(out, name) {
			t.Errorf("output missing tier name %q", name)
		}
	}
	if !strings.Contains(out, "priority") {
		t.Errorf("output missing selection strategy 'priority'")
	}
	if !strings.Contains(out, "polecat") {
		t.Errorf("output missing role default 'polecat'")
	}
}

func TestConfigAgentTiersShow_WhenAbsent(t *testing.T) {
	townRoot, _ := setupTierTestTown(t)
	chdirTo(t, townRoot)

	out := captureStdout(t, func() {
		if err := runConfigAgentTiersShow(&cobra.Command{}, nil); err != nil {
			t.Errorf("runConfigAgentTiersShow: %v", err)
		}
	})

	if !strings.Contains(out, "No agent tiers configured") {
		t.Errorf("expected 'No agent tiers configured' message, got: %q", out)
	}
}

func TestConfigAgentTiersShow_OutputIncludesAllTiers(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:       "town-settings",
		Version:    config.CurrentTownSettingsVersion,
		AgentTiers: config.DefaultAgentTierConfig(),
	}
	saveSettings(t, settingsPath, s)

	out := captureStdout(t, func() {
		_ = runConfigAgentTiersShow(&cobra.Command{}, nil)
	})

	for _, tier := range []string{"small", "medium", "large", "reasoning"} {
		if !strings.Contains(out, tier) {
			t.Errorf("output missing tier %q", tier)
		}
	}
	// Agents
	for _, agent := range []string{"claude-haiku", "claude-sonnet", "claude-opus", "claude-reasoning"} {
		if !strings.Contains(out, agent) {
			t.Errorf("output missing agent %q", agent)
		}
	}
	// Role defaults
	for _, role := range []string{"mayor", "polecat", "witness"} {
		if !strings.Contains(out, role) {
			t.Errorf("output missing role %q", role)
		}
	}
}

// ---- gt config agent tiers set ----

func TestConfigAgentTiersSet_CreatesNewTier(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// Set required flag state
	tiersSetAgent = "claude-sonnet"
	tiersSetDescription = "My new tier"
	tiersSetSelection = "priority"
	tiersSetFallback = ""
	defer func() {
		tiersSetAgent = ""
		tiersSetDescription = ""
		tiersSetSelection = ""
		tiersSetFallback = ""
	}()

	cmd := configAgentTiersSetCmd
	err := runConfigAgentTiersSet(cmd, []string{"newtier"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersSet: %v", err)
	}

	s := loadSettings(t, settingsPath)
	tier, ok := s.AgentTiers.Tiers["newtier"]
	if !ok {
		t.Fatal("newtier not created")
	}
	if tier.Description != "My new tier" {
		t.Errorf("Description = %q, want 'My new tier'", tier.Description)
	}
	if len(tier.Agents) != 1 || tier.Agents[0] != "claude-sonnet" {
		t.Errorf("Agents = %v, want [claude-sonnet]", tier.Agents)
	}
}

func TestConfigAgentTiersSet_UpdatesExistingTier(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// Write existing config
	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"medium": {
					Description: "Old description",
					Agents:      []string{"claude-haiku"},
					Selection:   "priority",
					Fallback:    true,
				},
			},
			TierOrder: []string{"medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	tiersSetDescription = "New description"
	tiersSetAgent = ""
	tiersSetSelection = ""
	tiersSetFallback = ""
	defer func() { tiersSetDescription = "" }()

	err := runConfigAgentTiersSet(configAgentTiersSetCmd, []string{"medium"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersSet: %v", err)
	}

	after := loadSettings(t, settingsPath)
	tier := after.AgentTiers.Tiers["medium"]
	if tier.Description != "New description" {
		t.Errorf("Description = %q, want 'New description'", tier.Description)
	}
	// Agent unchanged
	if len(tier.Agents) != 1 || tier.Agents[0] != "claude-haiku" {
		t.Errorf("Agents should be unchanged, got %v", tier.Agents)
	}
}

func TestConfigAgentTiersSet_NewTierAppendedToTierOrder(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// Existing tiers
	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"small": {Agents: []string{"claude-haiku"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"small"},
		},
	}
	saveSettings(t, settingsPath, s)

	tiersSetAgent = "claude-sonnet"
	tiersSetDescription = ""
	tiersSetSelection = ""
	tiersSetFallback = ""
	defer func() { tiersSetAgent = "" }()

	err := runConfigAgentTiersSet(configAgentTiersSetCmd, []string{"medium"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersSet: %v", err)
	}

	after := loadSettings(t, settingsPath)
	order := after.AgentTiers.TierOrder
	if len(order) != 2 {
		t.Fatalf("TierOrder len = %d, want 2", len(order))
	}
	if order[0] != "small" || order[1] != "medium" {
		t.Errorf("TierOrder = %v, want [small medium]", order)
	}
}

func TestConfigAgentTiersSet_InvalidSelectionStrategy(t *testing.T) {
	townRoot, _ := setupTierTestTown(t)
	chdirTo(t, townRoot)

	tiersSetSelection = "random"
	tiersSetAgent = ""
	tiersSetDescription = ""
	tiersSetFallback = ""
	defer func() { tiersSetSelection = "" }()

	err := runConfigAgentTiersSet(configAgentTiersSetCmd, []string{"mytier"})
	if err == nil {
		t.Fatal("expected error for invalid selection strategy")
	}
	if !strings.Contains(err.Error(), "invalid selection") {
		t.Errorf("error = %v, want 'invalid selection'", err)
	}
}

func TestConfigAgentTiersSet_FallbackFalse(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	tiersSetAgent = "claude-opus"
	tiersSetDescription = ""
	tiersSetSelection = ""
	tiersSetFallback = "false"
	defer func() {
		tiersSetAgent = ""
		tiersSetFallback = ""
	}()

	// Mark the flag as changed
	cmd := configAgentTiersSetCmd
	if err := cmd.Flags().Set("fallback", "false"); err != nil {
		t.Fatalf("set fallback flag: %v", err)
	}
	t.Cleanup(func() { cmd.Flags().Lookup("fallback").Changed = false })

	err := runConfigAgentTiersSet(cmd, []string{"reasoning"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersSet: %v", err)
	}

	after := loadSettings(t, settingsPath)
	tier := after.AgentTiers.Tiers["reasoning"]
	if tier.Fallback {
		t.Error("Fallback should be false")
	}
}

// ---- gt config agent tiers remove ----

func TestConfigAgentTiersRemove_RemovesTier(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"small":  {Agents: []string{"claude-haiku"}, Selection: "priority", Fallback: true},
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"small", "medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersRemove(&cobra.Command{}, []string{"small"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersRemove: %v", err)
	}

	after := loadSettings(t, settingsPath)
	if _, ok := after.AgentTiers.Tiers["small"]; ok {
		t.Error("tier 'small' still exists after removal")
	}
	for _, n := range after.AgentTiers.TierOrder {
		if n == "small" {
			t.Error("tier 'small' still in TierOrder after removal")
		}
	}
}

func TestConfigAgentTiersRemove_CleansUpRoleDefaults(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"small":  {Agents: []string{"claude-haiku"}, Selection: "priority", Fallback: true},
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"small", "medium"},
			RoleDefaults: map[string]string{
				"witness": "small",
				"polecat": "medium",
			},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersRemove(&cobra.Command{}, []string{"small"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersRemove: %v", err)
	}

	after := loadSettings(t, settingsPath)
	if _, ok := after.AgentTiers.RoleDefaults["witness"]; ok {
		t.Error("role 'witness' still references removed tier 'small'")
	}
	// Unrelated role default should survive
	if after.AgentTiers.RoleDefaults["polecat"] != "medium" {
		t.Errorf("role 'polecat' mapping was unexpectedly changed")
	}
}

func TestConfigAgentTiersRemove_ErrorOnNonExistent(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers:     map[string]*config.AgentTier{},
			TierOrder: []string{},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersRemove(&cobra.Command{}, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for non-existent tier")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

// ---- gt config agent tiers set-role ----

func TestConfigAgentTiersSetRole_MapsRole(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersSetRole(&cobra.Command{}, []string{"polecat", "medium"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersSetRole: %v", err)
	}

	after := loadSettings(t, settingsPath)
	if after.AgentTiers.RoleDefaults["polecat"] != "medium" {
		t.Errorf("RoleDefaults[polecat] = %q, want 'medium'", after.AgentTiers.RoleDefaults["polecat"])
	}
}

func TestConfigAgentTiersSetRole_ErrorOnNonExistentTier(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers:     map[string]*config.AgentTier{},
			TierOrder: []string{},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersSetRole(&cobra.Command{}, []string{"polecat", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for non-existent tier")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

func TestConfigAgentTiersSetRole_OverwritesExisting(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"small":  {Agents: []string{"claude-haiku"}, Selection: "priority", Fallback: true},
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder:    []string{"small", "medium"},
			RoleDefaults: map[string]string{"witness": "small"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersSetRole(&cobra.Command{}, []string{"witness", "medium"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersSetRole: %v", err)
	}

	after := loadSettings(t, settingsPath)
	if after.AgentTiers.RoleDefaults["witness"] != "medium" {
		t.Errorf("RoleDefaults[witness] = %q, want 'medium'", after.AgentTiers.RoleDefaults["witness"])
	}
}

// ---- gt config agent tiers add-agent / remove-agent ----

func TestConfigAgentTiersAddAgent_AppendsAgent(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersAddAgent(&cobra.Command{}, []string{"medium", "claude-haiku"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersAddAgent: %v", err)
	}

	after := loadSettings(t, settingsPath)
	agents := after.AgentTiers.Tiers["medium"].Agents
	if len(agents) != 2 {
		t.Fatalf("agents count = %d, want 2", len(agents))
	}
	if agents[1] != "claude-haiku" {
		t.Errorf("agents[1] = %q, want 'claude-haiku'", agents[1])
	}
}

func TestConfigAgentTiersAddAgent_RejectsDuplicates(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersAddAgent(&cobra.Command{}, []string{"medium", "claude-sonnet"})
	if err == nil {
		t.Fatal("expected error for duplicate agent")
	}
	if !strings.Contains(err.Error(), "already in tier") {
		t.Errorf("error = %v, want 'already in tier'", err)
	}
}

func TestConfigAgentTiersRemoveAgent_RemovesAgent(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"medium": {Agents: []string{"claude-sonnet", "claude-haiku"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersRemoveAgent(&cobra.Command{}, []string{"medium", "claude-haiku"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersRemoveAgent: %v", err)
	}

	after := loadSettings(t, settingsPath)
	agents := after.AgentTiers.Tiers["medium"].Agents
	if len(agents) != 1 || agents[0] != "claude-sonnet" {
		t.Errorf("agents = %v, want [claude-sonnet]", agents)
	}
}

func TestConfigAgentTiersRemoveAgent_ErrorIfWouldLeaveEmpty(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersRemoveAgent(&cobra.Command{}, []string{"medium", "claude-sonnet"})
	if err == nil {
		t.Fatal("expected error when removing last agent")
	}
	if !strings.Contains(err.Error(), "no agents") {
		t.Errorf("error = %v, want mention of 'no agents'", err)
	}
}

func TestConfigAgentTiersRemoveAgent_ErrorOnNonExistentTier(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers:     map[string]*config.AgentTier{},
			TierOrder: []string{},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersRemoveAgent(&cobra.Command{}, []string{"nonexistent", "claude-sonnet"})
	if err == nil {
		t.Fatal("expected error for non-existent tier")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

// ---- gt config agent tiers set-order ----

func TestConfigAgentTiersSetOrder_SetsOrder(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"small":  {Agents: []string{"claude-haiku"}, Selection: "priority", Fallback: true},
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"medium", "small"},
		},
	}
	saveSettings(t, settingsPath, s)

	err := runConfigAgentTiersSetOrder(&cobra.Command{}, []string{"small", "medium"})
	if err != nil {
		t.Fatalf("runConfigAgentTiersSetOrder: %v", err)
	}

	after := loadSettings(t, settingsPath)
	order := after.AgentTiers.TierOrder
	if len(order) != 2 || order[0] != "small" || order[1] != "medium" {
		t.Errorf("TierOrder = %v, want [small medium]", order)
	}
}

func TestConfigAgentTiersSetOrder_ErrorIfTiersMismatch(t *testing.T) {
	townRoot, settingsPath := setupTierTestTown(t)
	chdirTo(t, townRoot)

	s := &config.TownSettings{
		Type:    "town-settings",
		Version: config.CurrentTownSettingsVersion,
		AgentTiers: &config.AgentTierConfig{
			Tiers: map[string]*config.AgentTier{
				"small":  {Agents: []string{"claude-haiku"}, Selection: "priority", Fallback: true},
				"medium": {Agents: []string{"claude-sonnet"}, Selection: "priority", Fallback: true},
			},
			TierOrder: []string{"small", "medium"},
		},
	}
	saveSettings(t, settingsPath, s)

	// Missing 'medium' from set-order args
	err := runConfigAgentTiersSetOrder(&cobra.Command{}, []string{"small"})
	if err == nil {
		t.Fatal("expected error when tier names don't match Tiers map")
	}

	// Nonexistent tier in args
	err = runConfigAgentTiersSetOrder(&cobra.Command{}, []string{"small", "medium", "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent tier in order args")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found'", err)
	}
}

// ---- validateIdentifier ----

func TestValidateIdentifier_Valid(t *testing.T) {
	cases := []struct {
		kind        string
		name        string
		extraAllowed string
	}{
		{"tier name", "small", ""},
		{"tier name", "my-tier", ""},
		{"tier name", "tier_1", ""},
		{"tier name", "CamelCase", ""},
		{"agent name", "claude-sonnet", "."},
		{"agent name", "claude-haiku-4-5-20251001", "."},
		{"agent name", "my.agent.v2", "."},
		{"role name", "polecat", "/"},
		{"role name", "gastown/polecats/nux", "/"},
		{"role name", "mayor", "/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateIdentifier(tc.kind, tc.name, tc.extraAllowed); err != nil {
				t.Errorf("validateIdentifier(%q, %q, %q) = %v, want nil", tc.kind, tc.name, tc.extraAllowed, err)
			}
		})
	}
}

func TestValidateIdentifier_Invalid(t *testing.T) {
	cases := []struct {
		kind        string
		name        string
		extraAllowed string
		wantErr     string
	}{
		{"tier name", "", "", "must not be empty"},
		{"tier name", "has space", "", "whitespace"},
		{"tier name", "has\ttab", "", "whitespace"},
		{"tier name", "has\nnewline", "", "whitespace"},
		{"tier name", "has@special", "", "invalid character"},
		{"tier name", "has!bang", "", "invalid character"},
		{"tier name", "tier/slash", "", "invalid character"},
		{"agent name", "", ".", "must not be empty"},
		{"agent name", "agent name", ".", "whitespace"},
		{"agent name", "agent@bad", ".", "invalid character"},
		{"role name", "", "/", "must not be empty"},
		{"role name", "role name", "/", "whitespace"},
		{"role name", "role@bad", "/", "invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.kind+"/"+tc.name, func(t *testing.T) {
			err := validateIdentifier(tc.kind, tc.name, tc.extraAllowed)
			if err == nil {
				t.Fatalf("validateIdentifier(%q, %q, %q) = nil, want error containing %q", tc.kind, tc.name, tc.extraAllowed, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestConfigAgentTiersSet_RejectsInvalidTierName(t *testing.T) {
	townRoot, _ := setupTierTestTown(t)
	chdirTo(t, townRoot)

	for _, badName := range []string{"has space", "has@special", ""} {
		err := runConfigAgentTiersSet(configAgentTiersSetCmd, []string{badName})
		if err == nil {
			t.Errorf("runConfigAgentTiersSet(%q): expected error, got nil", badName)
		}
	}
}

func TestConfigAgentTiersSetRole_RejectsInvalidNames(t *testing.T) {
	townRoot, _ := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// Invalid role name
	err := runConfigAgentTiersSetRole(&cobra.Command{}, []string{"bad role", "medium"})
	if err == nil {
		t.Error("expected error for role name with space")
	}

	// Invalid tier name
	err = runConfigAgentTiersSetRole(&cobra.Command{}, []string{"polecat", "bad tier"})
	if err == nil {
		t.Error("expected error for tier name with space")
	}
}

func TestConfigAgentTiersAddAgent_RejectsInvalidNames(t *testing.T) {
	townRoot, _ := setupTierTestTown(t)
	chdirTo(t, townRoot)

	// Invalid tier name
	err := runConfigAgentTiersAddAgent(&cobra.Command{}, []string{"bad tier", "claude-sonnet"})
	if err == nil {
		t.Error("expected error for tier name with space")
	}

	// Invalid agent name
	err = runConfigAgentTiersAddAgent(&cobra.Command{}, []string{"medium", "bad agent"})
	if err == nil {
		t.Error("expected error for agent name with space")
	}
}
