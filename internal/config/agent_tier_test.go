package config

import (
	"testing"
)

// registerTestAgents registers a set of test agent presets for use in agent tier tests.
// Returns a slice of agent names that were registered.
func registerTestAgents(t *testing.T) []string {
	t.Helper()
	agents := []string{"test-agent-a", "test-agent-b", "test-agent-c", "test-agent-d"}
	for _, name := range agents {
		RegisterAgentForTesting(name, AgentPresetInfo{
			Name:    AgentPreset(name),
			Command: "claude",
			Args:    []string{"--dangerously-skip-permissions"},
		})
	}
	return agents
}

// buildTestTierConfig builds an AgentTierConfig using registered test agents.
// Tier order: low → mid → high (small to large).
func buildTestTierConfig() *AgentTierConfig {
	return &AgentTierConfig{
		Tiers: map[string]*AgentTier{
			"low": {
				Description: "Low tier",
				Agents:      []string{"test-agent-a", "test-agent-b"},
				Selection:   "priority",
				Fallback:    true,
			},
			"mid": {
				Description: "Mid tier",
				Agents:      []string{"test-agent-c"},
				Selection:   "priority",
				Fallback:    true,
			},
			"high": {
				Description: "High tier",
				Agents:      []string{"test-agent-d"},
				Selection:   "priority",
				Fallback:    false,
			},
		},
		TierOrder: []string{"low", "mid", "high"},
		RoleDefaults: map[string]string{
			"worker": "low",
			"lead":   "high",
		},
	}
}

// --- DefaultAgentTierConfig tests ---

func TestDefaultAgentTierConfig(t *testing.T) {
	t.Parallel()

	t.Run("returns 4 tiers", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		if len(cfg.Tiers) != 4 {
			t.Fatalf("DefaultAgentTierConfig() has %d tiers, want 4", len(cfg.Tiers))
		}
	})

	t.Run("all tiers have non-empty Description", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		for name, tier := range cfg.Tiers {
			if tier.Description == "" {
				t.Errorf("tier %q has empty Description", name)
			}
		}
	})

	t.Run("all tiers have non-empty Agents", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		for name, tier := range cfg.Tiers {
			if len(tier.Agents) == 0 {
				t.Errorf("tier %q has empty Agents list", name)
			}
		}
	})

	t.Run("all tiers have non-empty Selection", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		for name, tier := range cfg.Tiers {
			if tier.Selection == "" {
				t.Errorf("tier %q has empty Selection", name)
			}
		}
	})

	t.Run("TierOrder contains all tier names", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		inOrder := make(map[string]bool, len(cfg.TierOrder))
		for _, name := range cfg.TierOrder {
			inOrder[name] = true
		}
		for name := range cfg.Tiers {
			if !inOrder[name] {
				t.Errorf("tier %q is missing from TierOrder", name)
			}
		}
		if len(cfg.TierOrder) != len(cfg.Tiers) {
			t.Errorf("TierOrder has %d entries, Tiers has %d", len(cfg.TierOrder), len(cfg.Tiers))
		}
	})

	t.Run("TierOrder is small medium large reasoning", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		want := []string{"small", "medium", "large", "reasoning"}
		if len(cfg.TierOrder) != len(want) {
			t.Fatalf("TierOrder = %v, want %v", cfg.TierOrder, want)
		}
		for i, name := range want {
			if cfg.TierOrder[i] != name {
				t.Errorf("TierOrder[%d] = %q, want %q", i, cfg.TierOrder[i], name)
			}
		}
	})

	t.Run("RoleDefaults maps expected roles", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		expectedRoles := []string{"mayor", "crew", "polecat", "refinery", "witness", "deacon", "dogs"}
		for _, role := range expectedRoles {
			if _, ok := cfg.RoleDefaults[role]; !ok {
				t.Errorf("RoleDefaults missing role %q", role)
			}
		}
	})

	t.Run("reasoning tier has Fallback=false", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		reasoning := cfg.Tiers["reasoning"]
		if reasoning == nil {
			t.Fatal("reasoning tier not found")
		}
		if reasoning.Fallback {
			t.Error("reasoning tier Fallback should be false (highest tier)")
		}
	})

	t.Run("lower tiers have Fallback=true", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		for _, name := range []string{"small", "medium", "large"} {
			tier := cfg.Tiers[name]
			if tier == nil {
				t.Fatalf("tier %q not found", name)
			}
			if !tier.Fallback {
				t.Errorf("tier %q Fallback should be true", name)
			}
		}
	})
}

// --- Validate tests ---

func TestAgentTierConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("default config passes validation", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() on default config: %v", err)
		}
	})

	t.Run("nil config passes validation", func(t *testing.T) {
		t.Parallel()
		var cfg *AgentTierConfig
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() on nil config: %v", err)
		}
	})

	t.Run("missing tier in TierOrder fails", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"a": {Description: "A", Agents: []string{"x"}, Selection: "priority"},
				"b": {Description: "B", Agents: []string{"y"}, Selection: "priority"},
			},
			TierOrder: []string{"a", "nonexistent"},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Validate() should fail when TierOrder references nonexistent tier")
		}
	})

	t.Run("invalid selection strategy fails", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"a": {Description: "A", Agents: []string{"x"}, Selection: "invalid-strategy"},
			},
			TierOrder: []string{"a"},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Validate() should fail on invalid selection strategy")
		}
	})

	t.Run("valid selection strategies pass", func(t *testing.T) {
		t.Parallel()
		for _, sel := range []string{"priority", "round-robin", ""} {
			t.Run(sel, func(t *testing.T) {
				t.Parallel()
				cfg := &AgentTierConfig{
					Tiers: map[string]*AgentTier{
						"a": {Description: "A", Agents: []string{"x"}, Selection: sel},
					},
					TierOrder: []string{"a"},
				}
				if err := cfg.Validate(); err != nil {
					t.Errorf("Validate() should pass for selection %q: %v", sel, err)
				}
			})
		}
	})

	t.Run("empty TierOrder passes when no tiers", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers:     map[string]*AgentTier{},
			TierOrder: []string{},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() on empty config: %v", err)
		}
	})

	t.Run("RoleDefaults with invalid tier fails", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"a": {Description: "A", Agents: []string{"x"}, Selection: "priority"},
			},
			TierOrder: []string{"a"},
			RoleDefaults: map[string]string{
				"worker": "nonexistent",
			},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Validate() should fail when RoleDefaults references nonexistent tier")
		}
	})

	t.Run("nil tier value in Tiers map fails", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"a": nil,
			},
			TierOrder: []string{"a"},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Validate() should fail when Tiers map contains a nil value")
		}
	})

	t.Run("duplicate tier in TierOrder fails", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"a": {Description: "A", Agents: []string{"x"}, Selection: "priority"},
			},
			TierOrder: []string{"a", "a"},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Validate() should fail on duplicate tier names in TierOrder")
		}
	})
}

// --- ResolveTierToRuntimeConfig tests ---

func TestResolveTierToRuntimeConfig(t *testing.T) {
	registerTestAgents(t)
	cfg := buildTestTierConfig()

	t.Run("priority selection returns first agent", func(t *testing.T) {
		t.Parallel()
		rc, err := cfg.ResolveTierToRuntimeConfig("low", nil)
		if err != nil {
			t.Fatalf("ResolveTierToRuntimeConfig: %v", err)
		}
		if rc == nil {
			t.Fatal("returned nil RuntimeConfig")
		}
	})

	t.Run("priority with exclusions skips excluded returns next", func(t *testing.T) {
		t.Parallel()
		excluded := map[string]bool{"test-agent-a": true}
		rc, err := cfg.ResolveTierToRuntimeConfig("low", excluded)
		if err != nil {
			t.Fatalf("ResolveTierToRuntimeConfig: %v", err)
		}
		if rc == nil {
			t.Fatal("returned nil RuntimeConfig")
		}
	})

	t.Run("unknown tier returns error", func(t *testing.T) {
		t.Parallel()
		_, err := cfg.ResolveTierToRuntimeConfig("nonexistent", nil)
		if err == nil {
			t.Error("expected error for unknown tier")
		}
	})

	t.Run("nil excludedAgents treated as empty set", func(t *testing.T) {
		t.Parallel()
		rc, err := cfg.ResolveTierToRuntimeConfig("low", nil)
		if err != nil {
			t.Fatalf("nil excludedAgents should not fail: %v", err)
		}
		if rc == nil {
			t.Fatal("returned nil RuntimeConfig")
		}
	})

	t.Run("all agents excluded with Fallback=true falls back", func(t *testing.T) {
		t.Parallel()
		// Exclude all agents in "low" tier — should fall back to "mid"
		excluded := map[string]bool{
			"test-agent-a": true,
			"test-agent-b": true,
		}
		rc, err := cfg.ResolveTierToRuntimeConfig("low", excluded)
		if err != nil {
			t.Fatalf("should fall back to mid tier: %v", err)
		}
		if rc == nil {
			t.Fatal("returned nil RuntimeConfig")
		}
	})

	t.Run("all agents excluded with Fallback=false returns error", func(t *testing.T) {
		t.Parallel()
		// Exclude all agents in "high" tier which has Fallback=false
		excluded := map[string]bool{"test-agent-d": true}
		_, err := cfg.ResolveTierToRuntimeConfig("high", excluded)
		if err == nil {
			t.Error("expected error when all agents excluded and Fallback=false")
		}
	})

	t.Run("fallback chain walks up TierOrder", func(t *testing.T) {
		t.Parallel()
		// Exclude all agents in "low" and "mid" — should reach "high"
		excluded := map[string]bool{
			"test-agent-a": true,
			"test-agent-b": true,
			"test-agent-c": true,
		}
		rc, err := cfg.ResolveTierToRuntimeConfig("low", excluded)
		if err != nil {
			t.Fatalf("should walk up to high tier: %v", err)
		}
		if rc == nil {
			t.Fatal("returned nil RuntimeConfig")
		}
	})

	t.Run("fallback exhaustion all tiers excluded returns error", func(t *testing.T) {
		t.Parallel()
		// Exclude all agents in all tiers
		excluded := map[string]bool{
			"test-agent-a": true,
			"test-agent-b": true,
			"test-agent-c": true,
			"test-agent-d": true,
		}
		_, err := cfg.ResolveTierToRuntimeConfig("low", excluded)
		if err == nil {
			t.Error("expected error when all tiers exhausted")
		}
	})

	t.Run("nil config returns error", func(t *testing.T) {
		t.Parallel()
		var nilCfg *AgentTierConfig
		_, err := nilCfg.ResolveTierToRuntimeConfig("low", nil)
		if err == nil {
			t.Error("nil config should return error")
		}
	})

	t.Run("nil tier in map returns error not panic", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"nilTier": nil,
			},
			TierOrder: []string{"nilTier"},
		}
		_, err := cfg.ResolveTierToRuntimeConfig("nilTier", nil)
		if err == nil {
			t.Error("nil tier value should return error, not panic")
		}
	})
}

func TestResolveTierToRuntimeConfig_RoundRobin(t *testing.T) {
	registerTestAgents(t)

	t.Run("round-robin cycles through agents across calls", func(t *testing.T) {
		// Each AgentTierConfig has its own per-tier round-robin counter.
		// Use a fresh config so the counter starts at 0.
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"rr": {
					Description: "Round-robin tier",
					Agents:      []string{"test-agent-a", "test-agent-b"},
					Selection:   "round-robin",
					Fallback:    false,
				},
			},
			TierOrder: []string{"rr"},
		}

		// Call twice: counter starts at 0 so first picks index 0, second picks index 1.
		_, err := cfg.ResolveTierToRuntimeConfig("rr", nil)
		if err != nil {
			t.Fatalf("first call: %v", err)
		}
		_, err = cfg.ResolveTierToRuntimeConfig("rr", nil)
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		// Call a third time to wrap around: should succeed (index 0 again).
		_, err = cfg.ResolveTierToRuntimeConfig("rr", nil)
		if err != nil {
			t.Fatalf("third call (wrap): %v", err)
		}
	})

	t.Run("round-robin with exclusions skips excluded agents", func(t *testing.T) {
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"rr": {
					Description: "Round-robin tier",
					Agents:      []string{"test-agent-a", "test-agent-b", "test-agent-c"},
					Selection:   "round-robin",
					Fallback:    false,
				},
			},
			TierOrder: []string{"rr"},
		}

		// Exclude two of three — only "test-agent-c" available.
		excluded := map[string]bool{
			"test-agent-a": true,
			"test-agent-b": true,
		}
		rc, err := cfg.ResolveTierToRuntimeConfig("rr", excluded)
		if err != nil {
			t.Fatalf("round-robin with exclusions: %v", err)
		}
		if rc == nil {
			t.Fatal("returned nil RuntimeConfig")
		}
	})
}

// --- ResolveTierForRole tests ---

func TestResolveTierForRole(t *testing.T) {
	t.Parallel()
	cfg := buildTestTierConfig()

	t.Run("known role returns correct tier", func(t *testing.T) {
		t.Parallel()
		if got := cfg.ResolveTierForRole("worker"); got != "low" {
			t.Errorf("ResolveTierForRole(worker) = %q, want %q", got, "low")
		}
		if got := cfg.ResolveTierForRole("lead"); got != "high" {
			t.Errorf("ResolveTierForRole(lead) = %q, want %q", got, "high")
		}
	})

	t.Run("unknown role returns empty string", func(t *testing.T) {
		t.Parallel()
		if got := cfg.ResolveTierForRole("unknown-role"); got != "" {
			t.Errorf("ResolveTierForRole(unknown-role) = %q, want empty string", got)
		}
	})

	t.Run("nil config returns empty string", func(t *testing.T) {
		t.Parallel()
		var nilCfg *AgentTierConfig
		if got := nilCfg.ResolveTierForRole("worker"); got != "" {
			t.Errorf("nil config ResolveTierForRole = %q, want empty string", got)
		}
	})

	t.Run("default config maps expected roles", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		tests := []struct {
			role string
			want string
		}{
			{"mayor", "large"},
			{"crew", "large"},
			{"polecat", "medium"},
			{"refinery", "medium"},
			{"witness", "small"},
			{"deacon", "small"},
			{"dogs", "small"},
		}
		for _, tt := range tests {
			t.Run(tt.role, func(t *testing.T) {
				t.Parallel()
				if got := cfg.ResolveTierForRole(tt.role); got != tt.want {
					t.Errorf("ResolveTierForRole(%q) = %q, want %q", tt.role, got, tt.want)
				}
			})
		}
	})
}

// --- UpOneTier tests ---

func TestUpOneTier(t *testing.T) {
	t.Parallel()
	cfg := buildTestTierConfig()

	t.Run("returns next tier in TierOrder", func(t *testing.T) {
		t.Parallel()
		if got := cfg.UpOneTier("low"); got != "mid" {
			t.Errorf("UpOneTier(low) = %q, want %q", got, "mid")
		}
		if got := cfg.UpOneTier("mid"); got != "high" {
			t.Errorf("UpOneTier(mid) = %q, want %q", got, "high")
		}
	})

	t.Run("last tier returns empty string", func(t *testing.T) {
		t.Parallel()
		if got := cfg.UpOneTier("high"); got != "" {
			t.Errorf("UpOneTier(high) = %q, want empty string", got)
		}
	})

	t.Run("unknown tier returns empty string", func(t *testing.T) {
		t.Parallel()
		if got := cfg.UpOneTier("nonexistent"); got != "" {
			t.Errorf("UpOneTier(nonexistent) = %q, want empty string", got)
		}
	})
}

// --- HasTier tests ---

func TestHasTier(t *testing.T) {
	t.Parallel()
	cfg := buildTestTierConfig()

	t.Run("true for existing tier", func(t *testing.T) {
		t.Parallel()
		if !cfg.HasTier("low") {
			t.Error("HasTier(low) = false, want true")
		}
	})

	t.Run("false for missing tier", func(t *testing.T) {
		t.Parallel()
		if cfg.HasTier("nonexistent") {
			t.Error("HasTier(nonexistent) = true, want false")
		}
	})

	t.Run("nil config returns false", func(t *testing.T) {
		t.Parallel()
		var nilCfg *AgentTierConfig
		if nilCfg.HasTier("low") {
			t.Error("nil config HasTier should return false")
		}
	})
}

// --- TierNames tests ---

func TestTierNames(t *testing.T) {
	t.Parallel()

	t.Run("returns TierOrder sequence when set", func(t *testing.T) {
		t.Parallel()
		cfg := buildTestTierConfig()
		names := cfg.TierNames()
		want := []string{"low", "mid", "high"}
		if len(names) != len(want) {
			t.Fatalf("TierNames() = %v, want %v", names, want)
		}
		for i, name := range want {
			if names[i] != name {
				t.Errorf("TierNames()[%d] = %q, want %q", i, names[i], name)
			}
		}
	})

	t.Run("returns keys when TierOrder is empty", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"a": {Description: "A", Agents: []string{"x"}},
				"b": {Description: "B", Agents: []string{"y"}},
			},
			TierOrder: []string{},
		}
		names := cfg.TierNames()
		if len(names) != 2 {
			t.Fatalf("TierNames() = %v, want 2 entries", names)
		}
	})

	t.Run("nil config returns nil", func(t *testing.T) {
		t.Parallel()
		var nilCfg *AgentTierConfig
		if names := nilCfg.TierNames(); names != nil {
			t.Errorf("nil config TierNames() = %v, want nil", names)
		}
	})
}

// --- BuildTierSummaries tests ---

func TestBuildTierSummaries(t *testing.T) {
	t.Parallel()

	t.Run("returns name+description pairs in TierOrder", func(t *testing.T) {
		t.Parallel()
		cfg := buildTestTierConfig()
		summaries := cfg.BuildTierSummaries()
		if len(summaries) != 3 {
			t.Fatalf("BuildTierSummaries() = %d entries, want 3", len(summaries))
		}
		want := []TierSummary{
			{Name: "low", Description: "Low tier"},
			{Name: "mid", Description: "Mid tier"},
			{Name: "high", Description: "High tier"},
		}
		for i, s := range want {
			if summaries[i].Name != s.Name {
				t.Errorf("summaries[%d].Name = %q, want %q", i, summaries[i].Name, s.Name)
			}
			if summaries[i].Description != s.Description {
				t.Errorf("summaries[%d].Description = %q, want %q", i, summaries[i].Description, s.Description)
			}
		}
	})

	t.Run("nil config returns nil", func(t *testing.T) {
		t.Parallel()
		var nilCfg *AgentTierConfig
		if s := nilCfg.BuildTierSummaries(); s != nil {
			t.Errorf("nil config BuildTierSummaries() = %v, want nil", s)
		}
	})

	t.Run("empty tiers returns nil", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{Tiers: map[string]*AgentTier{}}
		if s := cfg.BuildTierSummaries(); s != nil {
			t.Errorf("empty tiers BuildTierSummaries() = %v, want nil", s)
		}
	})

	t.Run("nil tier value in map is skipped not panicked", func(t *testing.T) {
		t.Parallel()
		cfg := &AgentTierConfig{
			Tiers: map[string]*AgentTier{
				"a": {Description: "A", Agents: []string{"x"}},
				"b": nil,
			},
			TierOrder: []string{"a", "b"},
		}
		summaries := cfg.BuildTierSummaries()
		if len(summaries) != 1 {
			t.Fatalf("BuildTierSummaries() = %d entries, want 1 (nil tier skipped)", len(summaries))
		}
		if summaries[0].Name != "a" {
			t.Errorf("BuildTierSummaries()[0].Name = %q, want %q", summaries[0].Name, "a")
		}
	})

	t.Run("default config summaries have non-empty descriptions", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultAgentTierConfig()
		summaries := cfg.BuildTierSummaries()
		if len(summaries) != 4 {
			t.Fatalf("BuildTierSummaries() = %d entries, want 4", len(summaries))
		}
		for _, s := range summaries {
			if s.Name == "" {
				t.Error("summary has empty Name")
			}
			if s.Description == "" {
				t.Errorf("summary %q has empty Description", s.Name)
			}
		}
	})
}
