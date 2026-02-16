package config

import (
	"testing"
)

func TestValidCostTiers(t *testing.T) {
	t.Parallel()
	tiers := ValidCostTiers()
	if len(tiers) != 3 {
		t.Fatalf("ValidCostTiers() returned %d tiers, want 3", len(tiers))
	}
	expected := map[string]bool{"standard": true, "economy": true, "budget": true}
	for _, tier := range tiers {
		if !expected[tier] {
			t.Errorf("unexpected tier %q", tier)
		}
	}
}

func TestIsValidTier(t *testing.T) {
	t.Parallel()
	tests := []struct {
		tier string
		want bool
	}{
		{"standard", true},
		{"economy", true},
		{"budget", true},
		{"premium", false},
		{"", false},
		{"Standard", false}, // case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			t.Parallel()
			if got := IsValidTier(tt.tier); got != tt.want {
				t.Errorf("IsValidTier(%q) = %v, want %v", tt.tier, got, tt.want)
			}
		})
	}
}

func TestCostTierRoleAgents(t *testing.T) {
	t.Parallel()

	t.Run("standard returns empty map", func(t *testing.T) {
		t.Parallel()
		ra := CostTierRoleAgents(TierStandard)
		if ra == nil {
			t.Fatal("standard tier returned nil, want empty map")
		}
		if len(ra) != 0 {
			t.Errorf("standard tier has %d entries, want 0", len(ra))
		}
	})

	t.Run("economy has correct assignments", func(t *testing.T) {
		t.Parallel()
		ra := CostTierRoleAgents(TierEconomy)
		if ra == nil {
			t.Fatal("economy tier returned nil")
		}
		expected := map[string]string{
			"mayor":    "claude-sonnet",
			"deacon":   "claude-haiku",
			"witness":  "claude-sonnet",
			"refinery": "claude-sonnet",
		}
		for role, want := range expected {
			if got := ra[role]; got != want {
				t.Errorf("economy[%q] = %q, want %q", role, got, want)
			}
		}
		// polecat and crew should not be in the map
		if _, ok := ra["polecat"]; ok {
			t.Error("economy tier should not include polecat")
		}
		if _, ok := ra["crew"]; ok {
			t.Error("economy tier should not include crew")
		}
	})

	t.Run("budget has correct assignments", func(t *testing.T) {
		t.Parallel()
		ra := CostTierRoleAgents(TierBudget)
		if ra == nil {
			t.Fatal("budget tier returned nil")
		}
		expected := map[string]string{
			"mayor":    "claude-sonnet",
			"deacon":   "claude-haiku",
			"witness":  "claude-haiku",
			"refinery": "claude-haiku",
			"polecat":  "claude-sonnet",
			"crew":     "claude-sonnet",
		}
		for role, want := range expected {
			if got := ra[role]; got != want {
				t.Errorf("budget[%q] = %q, want %q", role, got, want)
			}
		}
	})

	t.Run("invalid tier returns nil", func(t *testing.T) {
		t.Parallel()
		ra := CostTierRoleAgents("invalid")
		if ra != nil {
			t.Error("invalid tier should return nil")
		}
	})
}

func TestCostTierAgents(t *testing.T) {
	t.Parallel()

	t.Run("standard returns empty map", func(t *testing.T) {
		t.Parallel()
		agents := CostTierAgents(TierStandard)
		if agents == nil {
			t.Fatal("standard tier returned nil, want empty map")
		}
		if len(agents) != 0 {
			t.Errorf("standard tier has %d agents, want 0", len(agents))
		}
	})

	t.Run("economy returns sonnet and haiku presets", func(t *testing.T) {
		t.Parallel()
		agents := CostTierAgents(TierEconomy)
		if agents == nil {
			t.Fatal("economy tier returned nil")
		}
		if _, ok := agents["claude-sonnet"]; !ok {
			t.Error("economy tier missing claude-sonnet agent")
		}
		if _, ok := agents["claude-haiku"]; !ok {
			t.Error("economy tier missing claude-haiku agent")
		}
	})

	t.Run("budget returns sonnet and haiku presets", func(t *testing.T) {
		t.Parallel()
		agents := CostTierAgents(TierBudget)
		if agents == nil {
			t.Fatal("budget tier returned nil")
		}
		if _, ok := agents["claude-sonnet"]; !ok {
			t.Error("budget tier missing claude-sonnet agent")
		}
		if _, ok := agents["claude-haiku"]; !ok {
			t.Error("budget tier missing claude-haiku agent")
		}
	})

	t.Run("sonnet preset has correct args", func(t *testing.T) {
		t.Parallel()
		agents := CostTierAgents(TierEconomy)
		sonnet := agents["claude-sonnet"]
		if sonnet.Command != "claude" {
			t.Errorf("sonnet Command = %q, want %q", sonnet.Command, "claude")
		}
		found := false
		for i, arg := range sonnet.Args {
			if arg == "--model" && i+1 < len(sonnet.Args) && sonnet.Args[i+1] == "sonnet" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("sonnet Args %v missing --model sonnet", sonnet.Args)
		}
	})

	t.Run("haiku preset has correct args", func(t *testing.T) {
		t.Parallel()
		agents := CostTierAgents(TierEconomy)
		haiku := agents["claude-haiku"]
		if haiku.Command != "claude" {
			t.Errorf("haiku Command = %q, want %q", haiku.Command, "claude")
		}
		found := false
		for i, arg := range haiku.Args {
			if arg == "--model" && i+1 < len(haiku.Args) && haiku.Args[i+1] == "haiku" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("haiku Args %v missing --model haiku", haiku.Args)
		}
	})
}

func TestApplyCostTier(t *testing.T) {
	t.Parallel()

	t.Run("applies economy tier", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		if err := ApplyCostTier(settings, TierEconomy); err != nil {
			t.Fatalf("ApplyCostTier: %v", err)
		}
		if settings.CostTier != "economy" {
			t.Errorf("CostTier = %q, want %q", settings.CostTier, "economy")
		}
		if settings.RoleAgents["mayor"] != "claude-sonnet" {
			t.Errorf("RoleAgents[mayor] = %q, want %q", settings.RoleAgents["mayor"], "claude-sonnet")
		}
		if settings.Agents["claude-sonnet"] == nil {
			t.Error("Agents[claude-sonnet] is nil")
		}
		if settings.Agents["claude-haiku"] == nil {
			t.Error("Agents[claude-haiku] is nil")
		}
	})

	t.Run("standard tier clears preset agents", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		// First apply economy
		if err := ApplyCostTier(settings, TierEconomy); err != nil {
			t.Fatalf("ApplyCostTier economy: %v", err)
		}
		// Then switch to standard
		if err := ApplyCostTier(settings, TierStandard); err != nil {
			t.Fatalf("ApplyCostTier standard: %v", err)
		}
		if settings.CostTier != "standard" {
			t.Errorf("CostTier = %q, want %q", settings.CostTier, "standard")
		}
		if len(settings.RoleAgents) != 0 {
			t.Errorf("RoleAgents should be empty, got %v", settings.RoleAgents)
		}
		if _, ok := settings.Agents["claude-sonnet"]; ok {
			t.Error("standard tier should remove claude-sonnet agent")
		}
		if _, ok := settings.Agents["claude-haiku"]; ok {
			t.Error("standard tier should remove claude-haiku agent")
		}
	})

	t.Run("standard preserves non-tier agents", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		settings.Agents = map[string]*RuntimeConfig{
			"custom-agent":  {Command: "custom"},
			"claude-sonnet": claudeSonnetPreset(),
		}
		if err := ApplyCostTier(settings, TierStandard); err != nil {
			t.Fatalf("ApplyCostTier: %v", err)
		}
		if settings.Agents["custom-agent"] == nil {
			t.Error("standard tier should preserve custom-agent")
		}
		if _, ok := settings.Agents["claude-sonnet"]; ok {
			t.Error("standard tier should remove claude-sonnet")
		}
	})

	t.Run("invalid tier returns error", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		err := ApplyCostTier(settings, "invalid")
		if err == nil {
			t.Error("expected error for invalid tier")
		}
	})
}

func TestGetCurrentTier(t *testing.T) {
	t.Parallel()

	t.Run("detects standard tier", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		settings.CostTier = "standard"
		settings.RoleAgents = map[string]string{}
		if got := GetCurrentTier(settings); got != "standard" {
			t.Errorf("GetCurrentTier = %q, want %q", got, "standard")
		}
	})

	t.Run("detects economy tier", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		if err := ApplyCostTier(settings, TierEconomy); err != nil {
			t.Fatalf("ApplyCostTier: %v", err)
		}
		if got := GetCurrentTier(settings); got != "economy" {
			t.Errorf("GetCurrentTier = %q, want %q", got, "economy")
		}
	})

	t.Run("detects budget tier", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		if err := ApplyCostTier(settings, TierBudget); err != nil {
			t.Fatalf("ApplyCostTier: %v", err)
		}
		if got := GetCurrentTier(settings); got != "budget" {
			t.Errorf("GetCurrentTier = %q, want %q", got, "budget")
		}
	})

	t.Run("returns empty for custom config", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		settings.RoleAgents = map[string]string{
			"mayor": "some-custom-agent",
		}
		if got := GetCurrentTier(settings); got != "" {
			t.Errorf("GetCurrentTier = %q, want empty string for custom config", got)
		}
	})

	t.Run("detects stale CostTier field", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		settings.CostTier = "economy" // says economy
		settings.RoleAgents = map[string]string{
			"mayor": "some-custom-agent", // but actually custom
		}
		// Should detect mismatch and infer from RoleAgents
		if got := GetCurrentTier(settings); got != "" {
			t.Errorf("GetCurrentTier = %q, want empty string for stale CostTier", got)
		}
	})

	t.Run("infers tier without CostTier field", func(t *testing.T) {
		t.Parallel()
		settings := NewTownSettings()
		// Set RoleAgents matching economy tier but without CostTier field
		settings.RoleAgents = map[string]string{
			"mayor":    "claude-sonnet",
			"deacon":   "claude-haiku",
			"witness":  "claude-sonnet",
			"refinery": "claude-sonnet",
		}
		if got := GetCurrentTier(settings); got != "economy" {
			t.Errorf("GetCurrentTier = %q, want %q (inferred)", got, "economy")
		}
	})
}

func TestRoleAgentsMatch(t *testing.T) {
	t.Parallel()

	t.Run("nil and empty are equal", func(t *testing.T) {
		t.Parallel()
		if !roleAgentsMatch(nil, map[string]string{}) {
			t.Error("nil and empty map should match")
		}
	})

	t.Run("identical maps match", func(t *testing.T) {
		t.Parallel()
		a := map[string]string{"mayor": "claude", "witness": "gemini"}
		b := map[string]string{"mayor": "claude", "witness": "gemini"}
		if !roleAgentsMatch(a, b) {
			t.Error("identical maps should match")
		}
	})

	t.Run("different lengths don't match", func(t *testing.T) {
		t.Parallel()
		a := map[string]string{"mayor": "claude"}
		b := map[string]string{"mayor": "claude", "witness": "gemini"}
		if roleAgentsMatch(a, b) {
			t.Error("different length maps should not match")
		}
	})

	t.Run("different values don't match", func(t *testing.T) {
		t.Parallel()
		a := map[string]string{"mayor": "claude"}
		b := map[string]string{"mayor": "gemini"}
		if roleAgentsMatch(a, b) {
			t.Error("different values should not match")
		}
	})
}

func TestTierDescription(t *testing.T) {
	t.Parallel()
	for _, tier := range ValidCostTiers() {
		t.Run(tier, func(t *testing.T) {
			t.Parallel()
			desc := TierDescription(CostTier(tier))
			if desc == "" || desc == "Unknown tier" {
				t.Errorf("TierDescription(%q) = %q, want meaningful description", tier, desc)
			}
		})
	}
}

func TestFormatTierRoleTable(t *testing.T) {
	t.Parallel()

	t.Run("valid tier returns formatted output", func(t *testing.T) {
		t.Parallel()
		output := FormatTierRoleTable(TierEconomy)
		if output == "" {
			t.Error("FormatTierRoleTable returned empty for economy tier")
		}
		// Should contain all roles
		for _, role := range []string{"mayor", "deacon", "witness", "refinery", "polecat", "crew"} {
			if !contains(output, role) {
				t.Errorf("output missing role %q", role)
			}
		}
	})

	t.Run("invalid tier returns empty", func(t *testing.T) {
		t.Parallel()
		output := FormatTierRoleTable("invalid")
		if output != "" {
			t.Errorf("FormatTierRoleTable(invalid) = %q, want empty", output)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
