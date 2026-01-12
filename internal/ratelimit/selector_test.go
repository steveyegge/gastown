package ratelimit

import (
	"testing"
	"time"
)

func TestSelector_SelectNext_FirstAvailable(t *testing.T) {
	s := NewSelector()
	s.SetPolicy("polecat", RolePolicy{
		FallbackChain:   []string{"anthropic_a", "openai_a", "anthropic_b"},
		CooldownMinutes: 5,
	})

	event := &RateLimitEvent{Profile: "anthropic_a"}
	next, err := s.SelectNext("polecat", "anthropic_a", event)
	if err != nil {
		t.Fatalf("SelectNext error: %v", err)
	}
	if next != "openai_a" {
		t.Errorf("SelectNext = %q, want %q", next, "openai_a")
	}
}

func TestSelector_SelectNext_SkipsCoolingDown(t *testing.T) {
	s := NewSelector()
	s.SetPolicy("polecat", RolePolicy{
		FallbackChain:   []string{"anthropic_a", "openai_a", "anthropic_b"},
		CooldownMinutes: 5,
	})

	// Mark openai_a as cooling down
	s.MarkCooldown("openai_a", time.Now().Add(10*time.Minute))

	event := &RateLimitEvent{Profile: "anthropic_a"}
	next, err := s.SelectNext("polecat", "anthropic_a", event)
	if err != nil {
		t.Fatalf("SelectNext error: %v", err)
	}
	// Should skip openai_a and return anthropic_b
	if next != "anthropic_b" {
		t.Errorf("SelectNext = %q, want %q", next, "anthropic_b")
	}
}

func TestSelector_SelectNext_AllCooling(t *testing.T) {
	s := NewSelector()
	s.SetPolicy("polecat", RolePolicy{
		FallbackChain:   []string{"anthropic_a", "openai_a"},
		CooldownMinutes: 5,
	})

	// Mark all other profiles as cooling
	s.MarkCooldown("openai_a", time.Now().Add(10*time.Minute))

	event := &RateLimitEvent{Profile: "anthropic_a"}
	_, err := s.SelectNext("polecat", "anthropic_a", event)
	if err != ErrAllProfilesCooling {
		t.Errorf("SelectNext error = %v, want %v", err, ErrAllProfilesCooling)
	}
}

func TestSelector_SelectNext_RespectsOrder(t *testing.T) {
	s := NewSelector()
	s.SetPolicy("polecat", RolePolicy{
		FallbackChain:   []string{"preferred", "backup1", "backup2"},
		CooldownMinutes: 5,
	})

	// Current is "preferred", should get "backup1" not "backup2"
	event := &RateLimitEvent{Profile: "preferred"}
	next, err := s.SelectNext("polecat", "preferred", event)
	if err != nil {
		t.Fatalf("SelectNext error: %v", err)
	}
	if next != "backup1" {
		t.Errorf("SelectNext = %q, want %q", next, "backup1")
	}
}

func TestSelector_IsAvailable(t *testing.T) {
	s := NewSelector()

	// Fresh profile should be available
	if !s.IsAvailable("test_profile") {
		t.Error("fresh profile should be available")
	}

	// Mark as cooling down
	s.MarkCooldown("test_profile", time.Now().Add(5*time.Minute))
	if s.IsAvailable("test_profile") {
		t.Error("cooling profile should not be available")
	}

	// Mark with past time (expired)
	s.MarkCooldown("test_profile", time.Now().Add(-1*time.Minute))
	if !s.IsAvailable("test_profile") {
		t.Error("expired cooldown should be available")
	}
}

func TestSelector_MarkCooldown(t *testing.T) {
	s := NewSelector()

	future := time.Now().Add(10 * time.Minute)
	s.MarkCooldown("test_profile", future)

	if s.IsAvailable("test_profile") {
		t.Error("profile should not be available after MarkCooldown")
	}
}

func TestSelector_ClearCooldown(t *testing.T) {
	s := NewSelector()

	s.MarkCooldown("test_profile", time.Now().Add(10*time.Minute))
	if s.IsAvailable("test_profile") {
		t.Error("profile should not be available after MarkCooldown")
	}

	s.ClearCooldown("test_profile")
	if !s.IsAvailable("test_profile") {
		t.Error("profile should be available after ClearCooldown")
	}
}

func TestSelector_DefaultPolicy(t *testing.T) {
	// No policies defined
	s := NewSelector()

	event := &RateLimitEvent{Profile: "unknown_profile"}
	_, err := s.SelectNext("unknown_role", "unknown_profile", event)

	// Should return error since there's no fallback chain
	if err != ErrAllProfilesCooling {
		t.Errorf("SelectNext with no policy should return ErrAllProfilesCooling, got %v", err)
	}
}
