package conductor

import (
	"testing"
)

func testPlan(t *testing.T) *Plan {
	t.Helper()
	input := PlanInput{
		BeadID:      "gt-abc",
		Title:       "Test Feature",
		RigName:     "gastown",
		Specialties: []string{"frontend", "backend", "tests", "security", "docs"},
	}
	plan, err := GeneratePlan(input)
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	return plan
}

func TestNewFeatureState(t *testing.T) {
	plan := testPlan(t)
	state := NewFeatureState(plan)

	if state.FeatureName != "test-feature" {
		t.Errorf("FeatureName = %q", state.FeatureName)
	}
	if state.Status != StatusPlanning {
		t.Errorf("Status = %q, want planning", state.Status)
	}
	if state.CurrentPhase != PhaseExamine {
		t.Errorf("CurrentPhase = %d, want %d", state.CurrentPhase, PhaseExamine)
	}
	if len(state.SubBeadStates) != len(plan.SubBeads) {
		t.Errorf("SubBeadStates count = %d, want %d", len(state.SubBeadStates), len(plan.SubBeads))
	}
	for _, sbs := range state.SubBeadStates {
		if sbs.Status != SubBeadPending {
			t.Errorf("sub-bead %q status = %q, want pending", sbs.Branch, sbs.Status)
		}
	}
}

func TestGetSubBeadState(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	sbs := state.GetSubBeadState("test-feature/harden")
	if sbs == nil {
		t.Fatal("GetSubBeadState returned nil for existing branch")
	}
	if sbs.Phase != PhaseHarden {
		t.Errorf("Phase = %d, want %d", sbs.Phase, PhaseHarden)
	}

	if state.GetSubBeadState("nonexistent") != nil {
		t.Error("GetSubBeadState should return nil for missing branch")
	}
}

func TestPhaseComplete(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	// Initially nothing is complete
	if state.PhaseComplete(PhaseHarden) {
		t.Error("harden should not be complete initially")
	}

	// Mark harden done
	state.MarkSubBeadDone("test-feature/harden")
	if !state.PhaseComplete(PhaseHarden) {
		t.Error("harden should be complete after marking done")
	}
}

func TestPhaseComplete_MultipleSubBeads(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	// Implement has frontend + backend sub-beads
	state.MarkSubBeadDone("test-feature/frontend")
	if state.PhaseComplete(PhaseImplement) {
		t.Error("implement should not be complete with only frontend done")
	}

	state.MarkSubBeadDone("test-feature/backend")
	if !state.PhaseComplete(PhaseImplement) {
		t.Error("implement should be complete with both done")
	}
}

func TestPhaseFailed(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	if state.PhaseFailed(PhaseHarden) {
		t.Error("harden should not be failed initially")
	}

	state.MarkSubBeadFailed("test-feature/harden", "tests failed")
	if !state.PhaseFailed(PhaseHarden) {
		t.Error("harden should be failed")
	}
}

func TestReadyToDispatch_InitialState(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	ready := state.ReadyToDispatch()
	// Only harden should be ready (no deps)
	if len(ready) != 1 {
		t.Fatalf("ReadyToDispatch() returned %d, want 1", len(ready))
	}
	if ready[0].Branch != "test-feature/harden" {
		t.Errorf("ready[0].Branch = %q, want test-feature/harden", ready[0].Branch)
	}
}

func TestReadyToDispatch_AfterHarden(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	state.MarkSubBeadDispatched("test-feature/harden", "gt-h1", "tests-1")
	state.MarkSubBeadDone("test-feature/harden")

	ready := state.ReadyToDispatch()
	// Modernize should now be ready (depends on harden)
	if len(ready) != 1 {
		t.Fatalf("ReadyToDispatch() returned %d, want 1", len(ready))
	}
	if ready[0].Branch != "test-feature/modernize" {
		t.Errorf("ready[0].Branch = %q, want test-feature/modernize", ready[0].Branch)
	}
}

func TestReadyToDispatch_AfterSpecify(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	// Complete phases up through specify
	state.MarkSubBeadDone("test-feature/harden")
	state.MarkSubBeadDone("test-feature/modernize")
	state.MarkSubBeadDone("test-feature/specify")

	ready := state.ReadyToDispatch()
	// Frontend + backend should both be ready (parallel implement)
	if len(ready) != 2 {
		t.Fatalf("ReadyToDispatch() returned %d, want 2 (frontend + backend)", len(ready))
	}

	branches := map[string]bool{}
	for _, r := range ready {
		branches[r.Branch] = true
	}
	if !branches["test-feature/frontend"] {
		t.Error("frontend should be ready")
	}
	if !branches["test-feature/backend"] {
		t.Error("backend should be ready")
	}
}

func TestReadyToDispatch_SecurityWaitsForAllImpl(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	state.MarkSubBeadDone("test-feature/harden")
	state.MarkSubBeadDone("test-feature/modernize")
	state.MarkSubBeadDone("test-feature/specify")
	state.MarkSubBeadDone("test-feature/frontend")
	// Backend NOT done

	ready := state.ReadyToDispatch()
	for _, r := range ready {
		if r.Branch == "test-feature/security" {
			t.Error("security should not be ready until all implement branches done")
		}
	}

	// Now complete backend
	state.MarkSubBeadDone("test-feature/backend")
	ready = state.ReadyToDispatch()
	found := false
	for _, r := range ready {
		if r.Branch == "test-feature/security" {
			found = true
		}
	}
	if !found {
		t.Error("security should be ready after all implement branches done")
	}
}

func TestAdvancePhase_ExamineToHarden(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusInProgress

	next := state.AdvancePhase()
	if next != PhaseHarden {
		t.Errorf("AdvancePhase() = %d, want %d (harden)", next, PhaseHarden)
	}
	if state.CurrentPhase != PhaseHarden {
		t.Errorf("CurrentPhase = %d, want %d", state.CurrentPhase, PhaseHarden)
	}
}

func TestAdvancePhase_HardenToModernize(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusInProgress
	state.CurrentPhase = PhaseHarden

	state.MarkSubBeadDone("test-feature/harden")

	next := state.AdvancePhase()
	if next != PhaseModernize {
		t.Errorf("AdvancePhase() = %d, want %d", next, PhaseModernize)
	}
}

func TestAdvancePhase_SpecifyTriggersApproval(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusInProgress
	state.CurrentPhase = PhaseSpecify

	state.MarkSubBeadDone("test-feature/specify")

	next := state.AdvancePhase()
	if next != 0 {
		t.Errorf("AdvancePhase() = %d, want 0 (approval gate)", next)
	}
	if state.Status != StatusAwaitingApproval {
		t.Errorf("Status = %q, want awaiting_approval", state.Status)
	}
}

func TestAdvancePhase_AfterApproval(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusAwaitingApproval
	state.CurrentPhase = PhaseSpecify

	state.MarkSubBeadDone("test-feature/specify")

	err := state.ApproveSpecification()
	if err != nil {
		t.Fatalf("ApproveSpecification: %v", err)
	}

	next := state.AdvancePhase()
	if next != PhaseImplement {
		t.Errorf("AdvancePhase() = %d, want %d", next, PhaseImplement)
	}
	if state.Status != StatusInProgress {
		t.Errorf("Status = %q, want in_progress", state.Status)
	}
}

func TestAdvancePhase_DocumentToComplete(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusInProgress
	state.CurrentPhase = PhaseDocument

	state.MarkSubBeadDone("test-feature/docs")

	next := state.AdvancePhase()
	if next != 0 {
		t.Errorf("AdvancePhase() = %d, want 0 (complete)", next)
	}
	if state.Status != StatusComplete {
		t.Errorf("Status = %q, want complete", state.Status)
	}
}

func TestAdvancePhase_FailedBlocksAdvance(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusInProgress
	state.CurrentPhase = PhaseHarden

	state.MarkSubBeadFailed("test-feature/harden", "coverage too low")

	next := state.AdvancePhase()
	if next != 0 {
		t.Errorf("AdvancePhase() should return 0 when phase failed, got %d", next)
	}
}

func TestAdvancePhase_EscalatedBlocksAdvance(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Escalate("artisan stuck")

	next := state.AdvancePhase()
	if next != 0 {
		t.Error("AdvancePhase() should return 0 when escalated")
	}
}

func TestEscalate(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Escalate("tests artisan crashed")

	if state.Status != StatusEscalated {
		t.Errorf("Status = %q, want escalated", state.Status)
	}
	if state.EscalationReason != "tests artisan crashed" {
		t.Errorf("EscalationReason = %q", state.EscalationReason)
	}
}

func TestApproveSpecification_WrongStatus(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusInProgress

	err := state.ApproveSpecification()
	if err == nil {
		t.Error("ApproveSpecification should error when not awaiting approval")
	}
}

func TestApproveSpecification_WrongPhase(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	state.Status = StatusAwaitingApproval
	state.CurrentPhase = PhaseHarden

	err := state.ApproveSpecification()
	if err == nil {
		t.Error("ApproveSpecification should error when not in specify phase")
	}
}

func TestMarkSubBeadDispatched(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	err := state.MarkSubBeadDispatched("test-feature/harden", "gt-h1", "tests-1")
	if err != nil {
		t.Fatalf("MarkSubBeadDispatched: %v", err)
	}

	sbs := state.GetSubBeadState("test-feature/harden")
	if sbs.Status != SubBeadDispatched {
		t.Errorf("Status = %q", sbs.Status)
	}
	if sbs.BeadID != "gt-h1" {
		t.Errorf("BeadID = %q", sbs.BeadID)
	}
	if sbs.ArtisanName != "tests-1" {
		t.Errorf("ArtisanName = %q", sbs.ArtisanName)
	}
	if sbs.DispatchedAt == nil {
		t.Error("DispatchedAt should be set")
	}
}

func TestMarkSubBeadDispatched_NotFound(t *testing.T) {
	state := NewFeatureState(testPlan(t))
	err := state.MarkSubBeadDispatched("nonexistent", "gt-1", "worker")
	if err == nil {
		t.Error("expected error for missing branch")
	}
}

func TestMarkSubBeadDone(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	err := state.MarkSubBeadDone("test-feature/harden")
	if err != nil {
		t.Fatalf("MarkSubBeadDone: %v", err)
	}

	sbs := state.GetSubBeadState("test-feature/harden")
	if sbs.Status != SubBeadDone {
		t.Errorf("Status = %q", sbs.Status)
	}
	if sbs.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestMarkSubBeadFailed(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	err := state.MarkSubBeadFailed("test-feature/harden", "coverage 85%")
	if err != nil {
		t.Fatalf("MarkSubBeadFailed: %v", err)
	}

	sbs := state.GetSubBeadState("test-feature/harden")
	if sbs.Status != SubBeadFailed {
		t.Errorf("Status = %q", sbs.Status)
	}
	if sbs.FailureReason != "coverage 85%" {
		t.Errorf("FailureReason = %q", sbs.FailureReason)
	}
}

func TestStateStore_SaveAndLoad(t *testing.T) {
	store := NewStateStore(t.TempDir())
	state := NewFeatureState(testPlan(t))

	if err := store.Save(state); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("test-feature")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.FeatureName != state.FeatureName {
		t.Errorf("FeatureName = %q", loaded.FeatureName)
	}
	if loaded.Status != state.Status {
		t.Errorf("Status = %q", loaded.Status)
	}
	if len(loaded.SubBeadStates) != len(state.SubBeadStates) {
		t.Errorf("SubBeadStates count = %d", len(loaded.SubBeadStates))
	}
}

func TestStateStore_Load_NotFound(t *testing.T) {
	store := NewStateStore(t.TempDir())
	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("Load should error for missing feature")
	}
}

func TestStateStore_List(t *testing.T) {
	store := NewStateStore(t.TempDir())

	// Empty
	names, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("List() returned %d, want 0", len(names))
	}

	// Save two features
	plan1 := testPlan(t)
	store.Save(NewFeatureState(plan1))

	plan2 := testPlan(t)
	plan2.FeatureName = "other-feature"
	store.Save(NewFeatureState(plan2))

	names, err = store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("List() returned %d, want 2", len(names))
	}
}

func TestFullLifecycle(t *testing.T) {
	state := NewFeatureState(testPlan(t))

	// Start: planning → examining → in_progress
	state.Status = StatusExamining
	state.Status = StatusInProgress

	// Phase 1 → 2
	next := state.AdvancePhase()
	if next != PhaseHarden {
		t.Fatalf("expected harden, got %d", next)
	}

	// Dispatch and complete harden
	state.MarkSubBeadDispatched("test-feature/harden", "gt-h1", "tests-1")
	state.MarkSubBeadDone("test-feature/harden")

	// Phase 2 → 3
	next = state.AdvancePhase()
	if next != PhaseModernize {
		t.Fatalf("expected modernize, got %d", next)
	}

	state.MarkSubBeadDone("test-feature/modernize")

	// Phase 3 → 4
	next = state.AdvancePhase()
	if next != PhaseSpecify {
		t.Fatalf("expected specify, got %d", next)
	}

	state.MarkSubBeadDone("test-feature/specify")

	// Phase 4 → awaiting approval
	next = state.AdvancePhase()
	if next != 0 || state.Status != StatusAwaitingApproval {
		t.Fatalf("expected approval gate, got phase=%d status=%s", next, state.Status)
	}

	// User approves
	state.ApproveSpecification()

	// Phase 4 → 5
	next = state.AdvancePhase()
	if next != PhaseImplement {
		t.Fatalf("expected implement, got %d", next)
	}

	// Complete both implement branches
	state.MarkSubBeadDone("test-feature/frontend")
	state.MarkSubBeadDone("test-feature/backend")

	// Phase 5 → 6
	next = state.AdvancePhase()
	if next != PhaseSecure {
		t.Fatalf("expected secure, got %d", next)
	}

	state.MarkSubBeadDone("test-feature/security")

	// Phase 6 → 7
	next = state.AdvancePhase()
	if next != PhaseDocument {
		t.Fatalf("expected document, got %d", next)
	}

	state.MarkSubBeadDone("test-feature/docs")

	// Phase 7 → complete
	next = state.AdvancePhase()
	if next != 0 || state.Status != StatusComplete {
		t.Fatalf("expected complete, got phase=%d status=%s", next, state.Status)
	}
}
