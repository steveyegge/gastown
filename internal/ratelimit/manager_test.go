package ratelimit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCooldownState(t *testing.T) {
	t.Run("not expired", func(t *testing.T) {
		state := &CooldownState{
			ProfileName: "test",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}

		if state.IsExpired() {
			t.Error("state should not be expired")
		}
	})

	t.Run("expired", func(t *testing.T) {
		state := &CooldownState{
			ProfileName: "test",
			StartedAt:   time.Now().Add(-20 * time.Minute),
			ExpiresAt:   time.Now().Add(-10 * time.Minute),
		}

		if !state.IsExpired() {
			t.Error("state should be expired")
		}
	})
}

func TestManager_SelectFallbackProfile(t *testing.T) {
	tempDir := t.TempDir()

	// Create a config
	cfg := &RateLimitConfig{
		Profiles: map[string]*InstanceProfile{
			"anthropic_a": {Name: "anthropic_a", Provider: "anthropic"},
			"anthropic_b": {Name: "anthropic_b", Provider: "anthropic"},
			"openai_a":    {Name: "openai_a", Provider: "openai"},
		},
		Roles: map[string]*RolePolicy{
			"witness": {
				Role:            "witness",
				FallbackChain:   []string{"anthropic_a", "anthropic_b", "openai_a"},
				CooldownMinutes: 30,
			},
		},
	}

	m := &Manager{
		townRoot:       tempDir,
		config:         cfg,
		cooldowns:      make(map[string]*CooldownState),
		activeProfiles: make(map[string]string),
	}

	t.Run("first fallback when current is first", func(t *testing.T) {
		profile, err := m.SelectFallbackProfile("witness", "anthropic_a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profile.Name != "anthropic_b" {
			t.Errorf("expected anthropic_b, got %s", profile.Name)
		}
	})

	t.Run("skip cooldown profiles", func(t *testing.T) {
		// Put anthropic_b in cooldown
		m.cooldowns["anthropic_b"] = &CooldownState{
			ProfileName: "anthropic_b",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}

		profile, err := m.SelectFallbackProfile("witness", "anthropic_a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should skip anthropic_b (in cooldown) and select openai_a
		if profile.Name != "openai_a" {
			t.Errorf("expected openai_a, got %s", profile.Name)
		}
	})

	t.Run("all in cooldown", func(t *testing.T) {
		// Put all profiles in cooldown
		m.cooldowns["anthropic_a"] = &CooldownState{
			ProfileName: "anthropic_a",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}
		m.cooldowns["anthropic_b"] = &CooldownState{
			ProfileName: "anthropic_b",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}
		m.cooldowns["openai_a"] = &CooldownState{
			ProfileName: "openai_a",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}

		_, err := m.SelectFallbackProfile("witness", "")
		if err == nil {
			t.Error("expected error when all profiles are in cooldown")
		}
	})
}

func TestManager_SelectFallbackProfile_Stickiness(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &RateLimitConfig{
		Profiles: map[string]*InstanceProfile{
			"anthropic_a": {Name: "anthropic_a", Provider: "anthropic"},
			"anthropic_b": {Name: "anthropic_b", Provider: "anthropic"},
			"openai_a":    {Name: "openai_a", Provider: "openai"},
		},
		Roles: map[string]*RolePolicy{
			"deacon": {
				Role:            "deacon",
				FallbackChain:   []string{"anthropic_a", "anthropic_b", "openai_a"},
				CooldownMinutes: 30,
				Stickiness: &StickinessConfig{
					PreferProvider:           "anthropic",
					OnlyFailoverIfAllCooling: true,
				},
			},
		},
	}

	m := &Manager{
		townRoot:       tempDir,
		config:         cfg,
		cooldowns:      make(map[string]*CooldownState),
		activeProfiles: make(map[string]string),
	}

	t.Run("prefer same provider", func(t *testing.T) {
		profile, err := m.SelectFallbackProfile("deacon", "anthropic_a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should select anthropic_b (same provider)
		if profile.Name != "anthropic_b" {
			t.Errorf("expected anthropic_b, got %s", profile.Name)
		}
	})

	t.Run("failover when all preferred in cooldown", func(t *testing.T) {
		// Put both anthropic profiles in cooldown
		m.cooldowns["anthropic_a"] = &CooldownState{
			ProfileName: "anthropic_a",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}
		m.cooldowns["anthropic_b"] = &CooldownState{
			ProfileName: "anthropic_b",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(10 * time.Minute),
		}

		profile, err := m.SelectFallbackProfile("deacon", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should now select openai_a
		if profile.Name != "openai_a" {
			t.Errorf("expected openai_a, got %s", profile.Name)
		}
	})
}

func TestManager_GetTransitionRule(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &RateLimitConfig{
		Profiles: map[string]*InstanceProfile{},
		Roles: map[string]*RolePolicy{
			"deacon": {
				Role: "deacon",
				TransitionRules: []TransitionRule{
					{
						FromProfile:       "anthropic_opus_acctA",
						ToProfile:         "zai_glm",
						OnTrigger:         "rate_limit",
						InjectHookPrelude: "ttc_scale_mcts_then_judge",
					},
					{
						FromProfile:       "*",
						ToProfile:         "minimax_m21",
						OnTrigger:         "rate_limit",
						InjectHookPrelude: "minimax_warmup",
					},
				},
			},
		},
	}

	m := &Manager{
		townRoot:       tempDir,
		config:         cfg,
		cooldowns:      make(map[string]*CooldownState),
		activeProfiles: make(map[string]string),
	}

	t.Run("exact match", func(t *testing.T) {
		rule := m.GetTransitionRule("deacon", "anthropic_opus_acctA", "zai_glm", "rate_limit")
		if rule == nil {
			t.Fatal("expected rule to match")
		}
		if rule.InjectHookPrelude != "ttc_scale_mcts_then_judge" {
			t.Errorf("wrong prelude: %s", rule.InjectHookPrelude)
		}
	})

	t.Run("wildcard match", func(t *testing.T) {
		rule := m.GetTransitionRule("deacon", "any_profile", "minimax_m21", "rate_limit")
		if rule == nil {
			t.Fatal("expected rule to match with wildcard")
		}
		if rule.InjectHookPrelude != "minimax_warmup" {
			t.Errorf("wrong prelude: %s", rule.InjectHookPrelude)
		}
	})

	t.Run("no match", func(t *testing.T) {
		rule := m.GetTransitionRule("deacon", "anthropic_opus_acctA", "zai_glm", "crash")
		if rule != nil {
			t.Error("expected no match for wrong trigger")
		}
	})
}

func TestManager_Cooldown(t *testing.T) {
	tempDir := t.TempDir()

	m := &Manager{
		townRoot:       tempDir,
		config:         NewRateLimitConfig(),
		cooldowns:      make(map[string]*CooldownState),
		activeProfiles: make(map[string]string),
	}

	// Create daemon dir for cooldown state file
	_ = os.MkdirAll(filepath.Join(tempDir, "daemon"), 0755)

	t.Run("start and check cooldown", func(t *testing.T) {
		state := m.StartCooldown("test_profile", "rate_limit", 30)

		if state.ProfileName != "test_profile" {
			t.Errorf("wrong profile name: %s", state.ProfileName)
		}

		if !m.IsProfileInCooldown("test_profile") {
			t.Error("profile should be in cooldown")
		}

		if m.IsProfileInCooldown("other_profile") {
			t.Error("other profile should not be in cooldown")
		}
	})

	t.Run("clear cooldown", func(t *testing.T) {
		m.ClearCooldown("test_profile")

		if m.IsProfileInCooldown("test_profile") {
			t.Error("profile should not be in cooldown after clear")
		}
	})

	t.Run("prune expired", func(t *testing.T) {
		// Add an expired cooldown
		m.cooldowns["expired"] = &CooldownState{
			ProfileName: "expired",
			StartedAt:   time.Now().Add(-1 * time.Hour),
			ExpiresAt:   time.Now().Add(-30 * time.Minute),
		}

		// Add a non-expired cooldown
		m.cooldowns["active"] = &CooldownState{
			ProfileName: "active",
			StartedAt:   time.Now(),
			ExpiresAt:   time.Now().Add(30 * time.Minute),
		}

		pruned := m.PruneExpiredCooldowns()

		if pruned != 1 {
			t.Errorf("expected 1 pruned, got %d", pruned)
		}

		if m.IsProfileInCooldown("expired") {
			t.Error("expired should have been pruned")
		}

		if !m.IsProfileInCooldown("active") {
			t.Error("active should still be in cooldown")
		}
	})
}

func TestManager_HandleRateLimit(t *testing.T) {
	tempDir := t.TempDir()

	// Create daemon dir
	_ = os.MkdirAll(filepath.Join(tempDir, "daemon", "ratelimit-events"), 0755)

	cfg := &RateLimitConfig{
		Type:    "ratelimit-config",
		Version: 1,
		Profiles: map[string]*InstanceProfile{
			"anthropic_a": {Name: "anthropic_a", Provider: "anthropic"},
			"anthropic_b": {Name: "anthropic_b", Provider: "anthropic"},
		},
		Roles: map[string]*RolePolicy{
			"witness": {
				Role:            "witness",
				FallbackChain:   []string{"anthropic_a", "anthropic_b"},
				CooldownMinutes: 30,
			},
		},
		GlobalCooldownMinutes: 30,
	}

	m := &Manager{
		townRoot:       tempDir,
		config:         cfg,
		cooldowns:      make(map[string]*CooldownState),
		activeProfiles: make(map[string]string),
	}

	event := &RateLimitEvent{
		ID:             "test-123",
		Timestamp:      time.Now(),
		Agent:          "gastown/witness",
		Role:           "witness",
		Rig:            "gastown",
		CurrentProfile: "anthropic_a",
		StatusCode:     429,
		ErrorSnippet:   "rate limit exceeded",
	}

	result, err := m.HandleRateLimit(event)
	if err != nil {
		t.Fatalf("HandleRateLimit failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if result.FromProfile != "anthropic_a" {
		t.Errorf("wrong from profile: %s", result.FromProfile)
	}

	if result.ToProfile != "anthropic_b" {
		t.Errorf("wrong to profile: %s", result.ToProfile)
	}

	// Verify cooldown was started
	if !m.IsProfileInCooldown("anthropic_a") {
		t.Error("anthropic_a should be in cooldown")
	}

	// Verify active profile was updated
	if m.activeProfiles["gastown/witness"] != "anthropic_b" {
		t.Error("active profile should be updated to anthropic_b")
	}
}
