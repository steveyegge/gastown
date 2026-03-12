// Tests for trust.go — trust tier escalation logic.
// Covers the pure evaluation function (no Dolt needed) and edge cases
// around tier boundaries, time requirements, and multi-criteria checks.
package wasteland

import (
	"testing"
	"time"
)

func TestTierString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tier TrustTier
		want string
	}{
		{TierDrifter, "Drifter"},
		{TierRegistered, "Registered"},
		{TierContributor, "Contributor"},
		{TierWarChief, "War Chief"},
		{TrustTier(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("TrustTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestDefaultTierRequirements(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	if len(reqs) != 3 {
		t.Fatalf("expected 3 tier requirement sets, got %d", len(reqs))
	}

	// Verify tiers are in ascending order
	for i := 1; i < len(reqs); i++ {
		if reqs[i].Tier <= reqs[i-1].Tier {
			t.Errorf("tier requirements not in ascending order at index %d", i)
		}
	}

	// War Chief should have the strictest requirements
	wc := reqs[2]
	if wc.MinCompletions < reqs[1].MinCompletions {
		t.Error("War Chief should require more completions than Contributor")
	}
	if wc.MinAvgQuality <= 0 {
		t.Error("War Chief should require minimum quality")
	}
}

// TestEscalation_RegisteredToContributor verifies the Registered → Contributor
// promotion path with all criteria met.
func TestEscalation_RegisteredToContributor(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "alice",
		CurrentTier:        TierRegistered,
		TierSince:          time.Now().Add(-30 * 24 * time.Hour), // 30 days ago
		CompletionCount:    5,
		StampCount:         8,
		AvgQuality:         4.2,
		DistinctValidators: 3,
	}

	result := EvaluateEscalation(profile, reqs)
	if !result.Eligible {
		t.Errorf("expected eligible for Contributor, got reasons: %v", result.Reasons)
	}
	if result.NextTier != TierContributor {
		t.Errorf("next tier should be Contributor, got %s", result.NextTier)
	}
}

// TestEscalation_InsufficientCompletions verifies that low completion count
// blocks promotion even when other criteria are met.
func TestEscalation_InsufficientCompletions(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "bob",
		CurrentTier:        TierRegistered,
		TierSince:          time.Now().Add(-30 * 24 * time.Hour),
		CompletionCount:    1, // below 3 required
		StampCount:         5,
		DistinctValidators: 3,
	}

	result := EvaluateEscalation(profile, reqs)
	if result.Eligible {
		t.Error("should not be eligible with 1 completion")
	}
	if len(result.Reasons) == 0 {
		t.Error("should have failure reasons")
	}
}

// TestEscalation_SingleValidatorBlocked verifies that having all stamps from
// one validator blocks promotion. This is the anti-collusion gate.
func TestEscalation_SingleValidatorBlocked(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "colluder",
		CurrentTier:        TierRegistered,
		TierSince:          time.Now().Add(-30 * 24 * time.Hour),
		CompletionCount:    10,
		StampCount:         10,
		AvgQuality:         5.0,
		DistinctValidators: 1, // only one validator — suspicious
	}

	result := EvaluateEscalation(profile, reqs)
	if result.Eligible {
		t.Error("should not be eligible with only 1 distinct validator")
	}
}

// TestEscalation_TimeGateBlocked verifies the minimum time-in-tier requirement
// prevents rapid trust farming.
func TestEscalation_TimeGateBlocked(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "speedrunner",
		CurrentTier:        TierRegistered,
		TierSince:          time.Now().Add(-1 * time.Hour), // just promoted
		CompletionCount:    10,
		StampCount:         10,
		DistinctValidators: 5,
	}

	result := EvaluateEscalation(profile, reqs)
	if result.Eligible {
		t.Error("should not be eligible — hasn't been Registered long enough")
	}
}

// TestEscalation_ContributorToWarChief verifies the Contributor → War Chief
// path requires quality scores.
func TestEscalation_ContributorToWarChief(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "veteran",
		CurrentTier:        TierContributor,
		TierSince:          time.Now().Add(-60 * 24 * time.Hour),
		CompletionCount:    15,
		StampCount:         25,
		AvgQuality:         4.5,
		DistinctValidators: 8,
	}

	result := EvaluateEscalation(profile, reqs)
	if !result.Eligible {
		t.Errorf("expected eligible for War Chief, got reasons: %v", result.Reasons)
	}
	if result.NextTier != TierWarChief {
		t.Errorf("next tier should be War Chief, got %s", result.NextTier)
	}
}

// TestEscalation_LowQualityBlocksWarChief verifies that high volume with
// low quality doesn't earn War Chief. This prevents grinding.
func TestEscalation_LowQualityBlocksWarChief(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "grinder",
		CurrentTier:        TierContributor,
		TierSince:          time.Now().Add(-60 * 24 * time.Hour),
		CompletionCount:    50,
		StampCount:         50,
		AvgQuality:         2.0, // below 3.5 threshold
		DistinctValidators: 10,
	}

	result := EvaluateEscalation(profile, reqs)
	if result.Eligible {
		t.Error("should not be eligible with low avg quality")
	}
}

// TestEscalation_MaxTierNoPromotion verifies that War Chiefs can't
// escalate further.
func TestEscalation_MaxTierNoPromotion(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "chief",
		CurrentTier:        TierWarChief,
		CompletionCount:    100,
		StampCount:         200,
		DistinctValidators: 50,
	}

	result := EvaluateEscalation(profile, reqs)
	if result.Eligible {
		t.Error("War Chief should not be promotable")
	}
	if result.Reasons[0] != "already at maximum tier" {
		t.Errorf("expected max tier reason, got %q", result.Reasons[0])
	}
}

// TestEscalation_DrifterNeedsNoRequirements verifies that Drifter → Registered
// is essentially automatic (handled by gt wl join).
func TestEscalation_DrifterNeedsNoRequirements(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:      "newbie",
		CurrentTier: TierDrifter,
		TierSince:   time.Now(),
	}

	result := EvaluateEscalation(profile, reqs)
	if !result.Eligible {
		t.Errorf("Drifter should be eligible for Registered: %v", result.Reasons)
	}
}

// TestEscalation_MultipleFailures verifies that all unmet criteria are
// reported, not just the first one. This gives rigs clear feedback on
// what they need to improve.
func TestEscalation_MultipleFailures(t *testing.T) {
	t.Parallel()

	reqs := DefaultTierRequirements()
	profile := RigTrustProfile{
		Handle:             "struggling",
		CurrentTier:        TierRegistered,
		TierSince:          time.Now(), // just joined
		CompletionCount:    0,
		StampCount:         0,
		DistinctValidators: 0,
	}

	result := EvaluateEscalation(profile, reqs)
	if result.Eligible {
		t.Error("should not be eligible with zero everything")
	}
	// Should have at least 3 failures: completions, stamps, validators
	if len(result.Reasons) < 3 {
		t.Errorf("expected at least 3 failure reasons, got %d: %v", len(result.Reasons), result.Reasons)
	}
}

// TestLoadRigTrustProfile_InvalidHandle verifies SQL injection prevention.
func TestLoadRigTrustProfile_InvalidHandle(t *testing.T) {
	t.Parallel()

	_, err := LoadRigTrustProfile("/usr/bin/dolt", "/tmp", "'; DROP TABLE rigs; --")
	if err == nil {
		t.Error("expected error for malicious handle")
	}
}
