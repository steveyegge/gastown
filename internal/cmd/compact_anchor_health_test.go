package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/governance"
)

func TestPromoteWisp_BlockedByAnchorGateSkipsPromotion(t *testing.T) {
	prevAssert := assertAnchorHealthFn
	prevRecordPtr := recordPromotionPtrFn
	prevUpdate := updateWispPersistentFn
	prevComment := addPromotionCommentFn
	prevDryRun := compactDryRun
	t.Cleanup(func() {
		assertAnchorHealthFn = prevAssert
		recordPromotionPtrFn = prevRecordPtr
		updateWispPersistentFn = prevUpdate
		addPromotionCommentFn = prevComment
		compactDryRun = prevDryRun
	})

	compactDryRun = false
	assertAnchorHealthFn = func(townRoot, promotionPointer, lane string) (*governance.AssertResult, error) {
		return &governance.AssertResult{
			Status: governance.AnchorGateStatusFrozenAnchor,
			Mode:   governance.SystemModeAnchorFreeze,
			Reason: "anchor health below H_min",
		}, nil
	}

	updateCalled := false
	updateWispPersistentFn = func(bd *beads.Beads, w *compactIssue) error {
		updateCalled = true
		return nil
	}
	addPromotionCommentFn = func(bd *beads.Beads, w *compactIssue, reason string) {}
	recordPromotionPtrFn = func(townRoot, promotionPointer string) error { return nil }

	result := &compactResult{}
	w := &compactIssue{
		Issue: beads.Issue{
			ID:    "w-100",
			Title: "stuck patrol",
		},
		WispType: "patrol",
	}

	promoteWisp(t.TempDir(), nil, w, "open past TTL", result)

	if updateCalled {
		t.Fatal("promotion update ran even though gate blocked")
	}
	if len(result.Promoted) != 0 {
		t.Fatalf("len(Promoted) = %d, want 0", len(result.Promoted))
	}
	if len(result.Blocked) != 1 {
		t.Fatalf("len(Blocked) = %d, want 1", len(result.Blocked))
	}
}

func TestPromoteWisp_MixedOutcomesKeepBlockedVisible(t *testing.T) {
	prevAssert := assertAnchorHealthFn
	prevRecordPtr := recordPromotionPtrFn
	prevUpdate := updateWispPersistentFn
	prevComment := addPromotionCommentFn
	prevDryRun := compactDryRun
	t.Cleanup(func() {
		assertAnchorHealthFn = prevAssert
		recordPromotionPtrFn = prevRecordPtr
		updateWispPersistentFn = prevUpdate
		addPromotionCommentFn = prevComment
		compactDryRun = prevDryRun
	})

	compactDryRun = false
	call := 0
	assertAnchorHealthFn = func(townRoot, promotionPointer, lane string) (*governance.AssertResult, error) {
		call++
		if call == 1 {
			return &governance.AssertResult{
				Status: governance.AnchorGateStatusOK,
				Mode:   governance.SystemModeNormal,
			}, nil
		}
		return &governance.AssertResult{
			Status: governance.AnchorGateStatusFrozenAnchor,
			Mode:   governance.SystemModeAnchorFreeze,
			Reason: "frozen mode",
		}, nil
	}

	updateCount := 0
	updateWispPersistentFn = func(bd *beads.Beads, w *compactIssue) error {
		updateCount++
		return nil
	}
	addPromotionCommentFn = func(bd *beads.Beads, w *compactIssue, reason string) {}
	recordPromotionPtrFn = func(townRoot, promotionPointer string) error { return nil }

	result := &compactResult{}
	w1 := &compactIssue{Issue: beads.Issue{ID: "w-1", Title: "first"}, WispType: "error"}
	w2 := &compactIssue{Issue: beads.Issue{ID: "w-2", Title: "second"}, WispType: "error"}

	promoteWisp(t.TempDir(), nil, w1, "reason-1", result)
	promoteWisp(t.TempDir(), nil, w2, "reason-2", result)

	if updateCount != 1 {
		t.Fatalf("updateCount = %d, want 1", updateCount)
	}
	if len(result.Promoted) != 1 {
		t.Fatalf("len(Promoted) = %d, want 1", len(result.Promoted))
	}
	if len(result.Blocked) != 1 {
		t.Fatalf("len(Blocked) = %d, want 1", len(result.Blocked))
	}
}

func TestPromoteWisp_GateErrorsFailClosed(t *testing.T) {
	prevAssert := assertAnchorHealthFn
	prevRecordPtr := recordPromotionPtrFn
	prevUpdate := updateWispPersistentFn
	prevComment := addPromotionCommentFn
	prevDryRun := compactDryRun
	t.Cleanup(func() {
		assertAnchorHealthFn = prevAssert
		recordPromotionPtrFn = prevRecordPtr
		updateWispPersistentFn = prevUpdate
		addPromotionCommentFn = prevComment
		compactDryRun = prevDryRun
	})

	compactDryRun = false
	assertAnchorHealthFn = func(townRoot, promotionPointer, lane string) (*governance.AssertResult, error) {
		return nil, fmt.Errorf("state file unreadable")
	}
	recordPromotionPtrFn = func(townRoot, promotionPointer string) error { return nil }

	updateCalled := false
	updateWispPersistentFn = func(bd *beads.Beads, w *compactIssue) error {
		updateCalled = true
		return nil
	}
	addPromotionCommentFn = func(bd *beads.Beads, w *compactIssue, reason string) {}

	result := &compactResult{}
	w := &compactIssue{Issue: beads.Issue{ID: "w-77", Title: "unsafe"}, WispType: "untyped"}
	promoteWisp(t.TempDir(), nil, w, "reason", result)

	if updateCalled {
		t.Fatal("promotion should be blocked when anchor gate errors")
	}
	if len(result.Blocked) != 1 {
		t.Fatalf("len(Blocked) = %d, want 1", len(result.Blocked))
	}
}

func TestPromoteWisp_GateRunsBeforePersistentUpdate(t *testing.T) {
	prevAssert := assertAnchorHealthFn
	prevRecordPtr := recordPromotionPtrFn
	prevUpdate := updateWispPersistentFn
	prevComment := addPromotionCommentFn
	prevDryRun := compactDryRun
	t.Cleanup(func() {
		assertAnchorHealthFn = prevAssert
		recordPromotionPtrFn = prevRecordPtr
		updateWispPersistentFn = prevUpdate
		addPromotionCommentFn = prevComment
		compactDryRun = prevDryRun
	})

	compactDryRun = false
	gateCalled := false
	assertAnchorHealthFn = func(townRoot, promotionPointer, lane string) (*governance.AssertResult, error) {
		gateCalled = true
		return &governance.AssertResult{
			Status: governance.AnchorGateStatusOK,
			Mode:   governance.SystemModeNormal,
		}, nil
	}
	recordPromotionPtrFn = func(townRoot, promotionPointer string) error { return nil }
	addPromotionCommentFn = func(bd *beads.Beads, w *compactIssue, reason string) {}

	updateWispPersistentFn = func(bd *beads.Beads, w *compactIssue) error {
		if !gateCalled {
			t.Fatal("persistent update ran before anchor gate")
		}
		return nil
	}

	result := &compactResult{}
	w := &compactIssue{Issue: beads.Issue{ID: "w-ordered", Title: "ordered"}, WispType: "error"}
	promoteWisp(t.TempDir(), nil, w, "ordered", result)

	if len(result.Promoted) != 1 {
		t.Fatalf("len(Promoted) = %d, want 1", len(result.Promoted))
	}
}

func TestAnchorHealthLatencyBudget_DefaultAndOverride(t *testing.T) {
	prev := os.Getenv("GT_ANCHOR_HEALTH_MAX_LATENCY_MS")
	t.Cleanup(func() {
		if prev == "" {
			_ = os.Unsetenv("GT_ANCHOR_HEALTH_MAX_LATENCY_MS")
		} else {
			_ = os.Setenv("GT_ANCHOR_HEALTH_MAX_LATENCY_MS", prev)
		}
	})

	_ = os.Unsetenv("GT_ANCHOR_HEALTH_MAX_LATENCY_MS")
	if got := anchorHealthLatencyBudgetMs(); got != 250 {
		t.Fatalf("default budget = %d, want 250", got)
	}

	_ = os.Setenv("GT_ANCHOR_HEALTH_MAX_LATENCY_MS", "600")
	if got := anchorHealthLatencyBudgetMs(); got != 600 {
		t.Fatalf("override budget = %d, want 600", got)
	}

	_ = os.Setenv("GT_ANCHOR_HEALTH_MAX_LATENCY_MS", "bad")
	if got := anchorHealthLatencyBudgetMs(); got != 250 {
		t.Fatalf("invalid override budget = %d, want 250", got)
	}
}
