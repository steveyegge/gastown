package config

import "testing"

func TestQualityTierValidation(t *testing.T) {
	for _, tier := range ValidQualityTiers() {
		if !IsValidQualityTier(tier) {
			t.Errorf("ValidQualityTiers() includes %q but IsValidQualityTier returns false", tier)
		}
	}

	if IsValidQualityTier("") {
		t.Error("empty string should not be a valid quality tier")
	}
	if IsValidQualityTier("premium") {
		t.Error("unknown tier should not be valid")
	}
}

func TestQualityTierRequirements(t *testing.T) {
	tests := []struct {
		tier         QualityTier
		peerReview   bool
		designReview bool
	}{
		{QualityStandard, false, false},
		{QualityReviewed, true, false},
		{QualityFull, true, true},
	}

	for _, tt := range tests {
		if got := tt.tier.RequiresPeerReview(); got != tt.peerReview {
			t.Errorf("%s.RequiresPeerReview() = %v, want %v", tt.tier, got, tt.peerReview)
		}
		if got := tt.tier.RequiresDesignReview(); got != tt.designReview {
			t.Errorf("%s.RequiresDesignReview() = %v, want %v", tt.tier, got, tt.designReview)
		}
	}
}

func TestQualityTierFormulas(t *testing.T) {
	if f := QualityStandard.ReviewFormula(); f != "" {
		t.Errorf("standard tier should have no review formula, got %q", f)
	}
	if f := QualityReviewed.ReviewFormula(); f != "mol-peer-review-gate" {
		t.Errorf("reviewed tier review formula = %q, want mol-peer-review-gate", f)
	}
	if f := QualityFull.DesignReviewFormula(); f != "mol-peer-review-design" {
		t.Errorf("full tier design review formula = %q, want mol-peer-review-design", f)
	}
	if f := QualityReviewed.DesignReviewFormula(); f != "" {
		t.Errorf("reviewed tier should have no design review formula, got %q", f)
	}
}

func TestGetQualityTierDefault(t *testing.T) {
	// Nil config defaults to standard
	var c *MergeQueueConfig
	if got := c.GetQualityTier(); got != QualityStandard {
		t.Errorf("nil config GetQualityTier() = %q, want %q", got, QualityStandard)
	}

	// Empty config defaults to standard
	c = &MergeQueueConfig{}
	if got := c.GetQualityTier(); got != QualityStandard {
		t.Errorf("empty config GetQualityTier() = %q, want %q", got, QualityStandard)
	}

	// PeerReview enabled maps to reviewed
	c = &MergeQueueConfig{PeerReview: boolPtr(true)}
	if got := c.GetQualityTier(); got != QualityReviewed {
		t.Errorf("peer review enabled GetQualityTier() = %q, want %q", got, QualityReviewed)
	}
}

func TestResolveQualityTier(t *testing.T) {
	rigConfig := &MergeQueueConfig{PeerReview: boolPtr(true)}

	// Bead override takes precedence
	tier, err := ResolveQualityTier("full", rigConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != QualityFull {
		t.Errorf("bead override should win, got %q", tier)
	}

	// No override uses rig default (peer review on = reviewed)
	tier, err = ResolveQualityTier("", rigConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != QualityReviewed {
		t.Errorf("should use rig default, got %q", tier)
	}

	// No override, no rig config uses built-in default
	tier, err = ResolveQualityTier("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tier != QualityStandard {
		t.Errorf("should use built-in default, got %q", tier)
	}

	// Invalid override returns error
	_, err = ResolveQualityTier("premium", rigConfig)
	if err == nil {
		t.Error("invalid tier override should return error")
	}
}
