package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

func TestTaskAgeFromBead_UnknownWhenMissingCreatedAt(t *testing.T) {
	age, known := taskAgeFromBead(&beadInfo{})
	if known {
		t.Fatalf("known = %v, want false", known)
	}
	if age != 0 {
		t.Fatalf("age = %v, want 0", age)
	}
}

func TestTaskAgeFromBead_KnownWhenCreatedAtPresent(t *testing.T) {
	originalNow := nowForStillness
	t.Cleanup(func() { nowForStillness = originalNow })

	fixedNow := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	nowForStillness = func() time.Time { return fixedNow }

	age, known := taskAgeFromBead(&beadInfo{
		CreatedAt: fixedNow.Add(-2 * time.Hour).Format(time.RFC3339),
	})
	if !known {
		t.Fatalf("known = %v, want true", known)
	}
	if age.Round(time.Minute) != 2*time.Hour {
		t.Fatalf("age = %v, want %v", age.Round(time.Minute), 2*time.Hour)
	}
}

func TestDecideStillnessOutcome_DefaultBiasWaitUnderAmbiguity(t *testing.T) {
	decision, reason := decideStillnessOutcome(
		0, // taskAge unknown/fresh
		false,
		1,   // single attempt in window
		-1,  // unknown load (estimation unavailable)
		0,   // no reversals
		100, // full coherence
		false,
	)

	if decision != stillnessDecisionWait {
		t.Fatalf("decision = %s, want %s (reason=%s)", decision, stillnessDecisionWait, reason)
	}
	if !strings.Contains(reason, "score=") {
		t.Fatalf("reason should include score details, got: %q", reason)
	}
}

func TestDecideStillnessOutcome_ActsOnStrongSignal(t *testing.T) {
	decision, reason := decideStillnessOutcome(
		7*time.Hour, // old task
		true,
		0,  // no churn
		0,  // idle target
		0,  // no reversals
		90, // high coherence
		false,
	)

	if decision != stillnessDecisionAct {
		t.Fatalf("decision = %s, want %s (reason=%s)", decision, stillnessDecisionAct, reason)
	}
}

func TestDecideStillnessOutcome_RefusesOnDissolvedCoherence(t *testing.T) {
	decision, reason := decideStillnessOutcome(
		2*time.Hour,
		true,
		1,
		0,
		0,
		stillnessDissolveThreshold,
		false,
	)

	if decision != stillnessDecisionRefuse {
		t.Fatalf("decision = %s, want %s (reason=%s)", decision, stillnessDecisionRefuse, reason)
	}
	if !strings.Contains(reason, "coherence") {
		t.Fatalf("expected coherence refusal reason, got: %q", reason)
	}
}

func TestShouldApplyStillnessGate_ProductionIgnoresDisableOverride(t *testing.T) {
	t.Setenv("GT_GOVERNANCE_ENV", "production")
	t.Setenv("GT_STILLNESS_GATE", "off")
	t.Setenv("GT_ROLE", "mayor")

	if !shouldApplyStillnessGate() {
		t.Fatal("expected stillness gate to remain enabled in production")
	}
}

func TestShouldApplyStillnessGate_NonProductionAllowsDisableOverride(t *testing.T) {
	t.Setenv("GT_GOVERNANCE_ENV", "dev")
	t.Setenv("GT_STILLNESS_GATE", "off")
	t.Setenv("GT_ROLE", "mayor")

	if shouldApplyStillnessGate() {
		t.Fatal("expected stillness gate disable override outside production")
	}
}

func TestShouldApplyStillnessGate_ProductionDisableOverrideEmitsGovernanceEvent(t *testing.T) {
	t.Setenv("GT_GOVERNANCE_ENV", "production")
	t.Setenv("GT_STILLNESS_GATE", "off")
	t.Setenv("GT_ROLE", "mayor")

	prevAudit := logGovernanceAuditFn
	t.Cleanup(func() { logGovernanceAuditFn = prevAudit })

	var (
		called    bool
		eventType string
		payload   map[string]interface{}
	)
	logGovernanceAuditFn = func(et, actor string, p map[string]interface{}) error {
		called = true
		eventType = et
		payload = p
		return nil
	}

	if !shouldApplyStillnessGate() {
		t.Fatal("expected stillness gate to remain enabled in production")
	}
	if !called {
		t.Fatal("expected governance audit event for blocked disable override")
	}
	if eventType != events.TypeAnchorHealthGate {
		t.Fatalf("eventType = %q, want %q", eventType, events.TypeAnchorHealthGate)
	}
	if got, _ := payload["phase"].(string); got != "stillness_disable_blocked" {
		t.Fatalf("phase = %q, want %q", got, "stillness_disable_blocked")
	}
}

func TestShouldApplyStillnessGate_NonProductionDisableOverrideDoesNotEmitGovernanceEvent(t *testing.T) {
	t.Setenv("GT_GOVERNANCE_ENV", "dev")
	t.Setenv("GT_STILLNESS_GATE", "off")
	t.Setenv("GT_ROLE", "mayor")

	prevAudit := logGovernanceAuditFn
	t.Cleanup(func() { logGovernanceAuditFn = prevAudit })

	called := false
	logGovernanceAuditFn = func(et, actor string, p map[string]interface{}) error {
		called = true
		return nil
	}

	if shouldApplyStillnessGate() {
		t.Fatal("expected stillness gate disable override outside production")
	}
	if called {
		t.Fatal("did not expect governance audit event outside production")
	}
}
