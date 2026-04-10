package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"

	"github.com/steveyegge/gastown/internal/bus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test doubles ---

// fakeReader simulates tmux capture-pane output. Each call to CaptureOutput
// returns the next queued output for that session, allowing tests to simulate
// a polecat progressing through steps.
type fakeReader struct {
	mu      sync.Mutex
	outputs map[string][]string // session -> queued outputs
}

func newFakeReader() *fakeReader {
	return &fakeReader{outputs: make(map[string][]string)}
}

func (r *fakeReader) Enqueue(session string, output string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outputs[session] = append(r.outputs[session], output)
}

func (r *fakeReader) CaptureOutput(session string, lines int) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q := r.outputs[session]
	if len(q) == 0 {
		return "", nil
	}
	out := q[0]
	r.outputs[session] = q[1:]
	return out, nil
}

// fakeWriter records SendStep calls so tests can assert what instructions
// were sent to the polecat.
type fakeWriter struct {
	mu    sync.Mutex
	sends []sendRecord
}

type sendRecord struct {
	Session     string
	Instruction string
}

func (w *fakeWriter) SendStep(session string, instruction string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sends = append(w.sends, sendRecord{Session: session, Instruction: instruction})
	return nil
}

func (w *fakeWriter) Sends() []sendRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make([]sendRecord, len(w.sends))
	copy(cp, w.sends)
	return cp
}

// fakeStepProvider models a linear sequence of formula steps. NextStep returns
// the step after currentStepID, or ("","",nil) when the formula is done.
type fakeStepProvider struct {
	// steps is an ordered list of (stepID, instruction) pairs per formula.
	formulas map[string][]stepEntry
}

type stepEntry struct {
	ID          string
	Instruction string
}

func (p *fakeStepProvider) NextStep(formulaName, currentStepID string) (string, string, error) {
	steps, ok := p.formulas[formulaName]
	if !ok {
		return "", "", fmt.Errorf("unknown formula: %s", formulaName)
	}
	for i, s := range steps {
		if s.ID == currentStepID && i+1 < len(steps) {
			next := steps[i+1]
			return next.ID, next.Instruction, nil
		}
	}
	return "", "", nil // no more steps
}

// eventCollector subscribes to bus events and collects them for assertions.
type eventCollector struct {
	mu     sync.Mutex
	events []bus.StepEvent
}

func (c *eventCollector) handler(ev bus.StepEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *eventCollector) Events() []bus.StepEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]bus.StepEvent, len(c.events))
	copy(cp, c.events)
	return cp
}

func testLogger() *log.Logger {
	return log.New(os.Stderr, "test-orch: ", log.LstdFlags)
}

// --- Integration tests ---

// TestOrchestratorAdvancesAcrossSteps validates the core multi-turn pipeline:
// polecat outputs STEP_COMPLETE → orchestrator captures it → pattern matches
// the body → routes to advance → sends the next step instruction via tmux.
// This simulates a polecat advancing through 3 steps of mol-polecat-work-tdd.
func TestOrchestratorAdvancesAcrossSteps(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "load-context", Instruction: "Load context and verify assignment"},
				{ID: "branch-setup", Instruction: "Set up working branch"},
				{ID: "implement.write-tests", Instruction: "Write failing tests"},
			},
		},
	}

	b := bus.NewLocalBus()
	collector := &eventCollector{}
	b.Subscribe("orchestrator:step:gastown-prime", collector.handler)

	orch := New(DefaultConfig(), reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-1234", "gastown-prime", "mol-polecat-work-tdd", "load-context")

	ctx := context.Background()

	// --- Tick 1: polecat outputs STEP_COMPLETE load-context with test-pass body ---
	reader.Enqueue("polecat-1234", "Running gt prime...\nLoaded context successfully\nok  \tgithub.com/test/pkg\t0.5s\nSTEP_COMPLETE load-context\n")

	orch.Tick(ctx)

	sends := writer.Sends()
	require.Len(t, sends, 1, "expected one SendStep after first STEP_COMPLETE")
	assert.Equal(t, "polecat-1234", sends[0].Session)
	assert.Equal(t, "Set up working branch", sends[0].Instruction)

	events := collector.Events()
	require.Len(t, events, 1)
	assert.Equal(t, bus.StepAdvanced, events[0].Type)
	assert.Equal(t, "load-context", events[0].StepID)
	assert.Equal(t, "mol-polecat-work-tdd", events[0].Formula)

	// --- Tick 2: polecat outputs STEP_COMPLETE branch-setup ---
	reader.Enqueue("polecat-1234", "Fetching origin...\n[main abc1234] initial\nSTEP_COMPLETE branch-setup\n")

	orch.Tick(ctx)

	sends = writer.Sends()
	require.Len(t, sends, 2, "expected two SendStep calls total after second STEP_COMPLETE")
	assert.Equal(t, "Write failing tests", sends[1].Instruction)

	events = collector.Events()
	require.Len(t, events, 2)
	assert.Equal(t, bus.StepAdvanced, events[1].Type)
	assert.Equal(t, "branch-setup", events[1].StepID)
}

// TestOrchestratorCompletesOnFinalStep verifies that when a polecat completes
// the last step in a formula, the orchestrator emits StepCompleted and
// unregisters the session (no SendStep).
func TestOrchestratorCompletesOnFinalStep(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "submit-and-exit", Instruction: "Submit work and self-clean"},
			},
		},
	}

	b := bus.NewLocalBus()
	collector := &eventCollector{}
	b.Subscribe("orchestrator:step:gastown-prime", collector.handler)

	orch := New(DefaultConfig(), reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-final", "gastown-prime", "mol-polecat-work-tdd", "submit-and-exit")

	ctx := context.Background()

	// Polecat completes the final step with a git push in the body.
	reader.Enqueue("polecat-final", "Pushing branch...\n[new branch]   polecat/fix -> polecat/fix\nSTEP_COMPLETE submit-and-exit\n")

	orch.Tick(ctx)

	// No SendStep — there's no next step.
	assert.Empty(t, writer.Sends(), "should not send step after final completion")

	events := collector.Events()
	require.Len(t, events, 1)
	assert.Equal(t, bus.StepCompleted, events[0].Type)
	assert.Equal(t, "submit-and-exit", events[0].StepID)

	// Session should be unregistered — a subsequent tick should be a no-op.
	reader.Enqueue("polecat-final", "anything\nSTEP_COMPLETE submit-and-exit\n")
	orch.Tick(ctx)

	// Still only 1 event — session was unregistered.
	assert.Len(t, collector.Events(), 1, "unregistered session should produce no new events")
}

// TestOrchestratorRetriesOnFailure verifies that when a polecat's step output
// contains a test failure pattern, the orchestrator routes to retry and sends
// a retry instruction.
func TestOrchestratorRetriesOnFailure(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "implement.verify-red", Instruction: "Verify tests fail (red)"},
				{ID: "implement.implement", Instruction: "Implement to green"},
			},
		},
	}

	b := bus.NewLocalBus()
	collector := &eventCollector{}
	b.Subscribe("orchestrator:step:gastown-prime", collector.handler)

	orch := New(DefaultConfig(), reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-retry", "gastown-prime", "mol-polecat-work-tdd", "implement.verify-red")

	ctx := context.Background()

	// Polecat outputs STEP_COMPLETE but the body shows test failure.
	reader.Enqueue("polecat-retry", "Running tests...\nFAIL\tgithub.com/test/pkg\t0.3s\nSTEP_COMPLETE implement.verify-red\n")

	orch.Tick(ctx)

	sends := writer.Sends()
	require.Len(t, sends, 1)
	assert.Contains(t, sends[0].Instruction, "retry")
	assert.Contains(t, sends[0].Instruction, "implement.verify-red")

	events := collector.Events()
	require.Len(t, events, 1)
	assert.Equal(t, bus.StepRetried, events[0].Type)
}

// TestOrchestratorEscalatesAfterMaxRetries verifies that after exceeding
// MaxRetries, the orchestrator escalates instead of retrying.
func TestOrchestratorEscalatesAfterMaxRetries(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "implement.verify-green", Instruction: "Verify all tests pass"},
				{ID: "implement.refactor", Instruction: "Refactor"},
			},
		},
	}

	cfg := DefaultConfig()
	cfg.MaxRetries = 2

	b := bus.NewLocalBus()
	collector := &eventCollector{}
	b.Subscribe("orchestrator:step:gastown-prime", collector.handler)

	orch := New(cfg, reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-esc", "gastown-prime", "mol-polecat-work-tdd", "implement.verify-green")

	ctx := context.Background()

	failBody := "Running tests...\nFAIL\tgithub.com/test/pkg\t0.1s\nSTEP_COMPLETE implement.verify-green\n"

	// Attempt 1 → retry
	reader.Enqueue("polecat-esc", failBody)
	orch.Tick(ctx)
	require.Len(t, collector.Events(), 1)
	assert.Equal(t, bus.StepRetried, collector.Events()[0].Type)

	// Attempt 2 → retry (need different output to avoid dedup)
	reader.Enqueue("polecat-esc", "Retrying...\nFAIL\tgithub.com/test/pkg\t0.2s\nSTEP_COMPLETE implement.verify-green\n")
	orch.Tick(ctx)
	require.Len(t, collector.Events(), 2)
	assert.Equal(t, bus.StepRetried, collector.Events()[1].Type)

	// Attempt 3 → escalate (exceeds MaxRetries=2)
	reader.Enqueue("polecat-esc", "Retrying again...\nFAIL\tgithub.com/test/pkg\t0.3s\nSTEP_COMPLETE implement.verify-green\n")
	orch.Tick(ctx)
	require.Len(t, collector.Events(), 3)
	assert.Equal(t, bus.StepEscalated, collector.Events()[2].Type)
	assert.Contains(t, collector.Events()[2].Detail, "max retries")
}

// TestOrchestratorTriagesAmbiguousOutput verifies that when pattern matching
// cannot classify the output, the orchestrator routes to triage (Haiku) and
// acts on the triage verdict.
func TestOrchestratorTriagesAmbiguousOutput(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "self-review", Instruction: "Self-review changes"},
				{ID: "build-check", Instruction: "Build and sanity check"},
			},
		},
	}

	triage := &mockTriageClient{
		result: TriageResult{Verdict: TriageSuccess, Reason: "review looks complete"},
	}

	b := bus.NewLocalBus()
	collector := &eventCollector{}
	b.Subscribe("orchestrator:step:gastown-prime", collector.handler)

	orch := New(DefaultConfig(), reader, writer, steps, triage, b, testLogger())
	orch.Register("polecat-triage", "gastown-prime", "mol-polecat-work-tdd", "self-review")

	ctx := context.Background()

	// Polecat outputs STEP_COMPLETE but the body doesn't match any known pattern.
	reader.Enqueue("polecat-triage", "Reviewed diff... looks good, no issues found.\nSTEP_COMPLETE self-review\n")

	orch.Tick(ctx)

	// Triage was called.
	assert.True(t, triage.called)

	// Triage said success → should advance to next step.
	sends := writer.Sends()
	require.Len(t, sends, 1)
	assert.Equal(t, "Build and sanity check", sends[0].Instruction)

	events := collector.Events()
	// Should have triage event followed by implicit advance.
	require.GreaterOrEqual(t, len(events), 1)
	foundTriage := false
	for _, ev := range events {
		if ev.Type == bus.StepTriaged {
			foundTriage = true
		}
	}
	assert.True(t, foundTriage, "expected a triage event")
}

// TestOrchestratorDeduplicated verifies that identical consecutive tmux output
// is deduplicated — the orchestrator only acts when output changes.
func TestOrchestratorDeduplicated(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "load-context", Instruction: "Load context"},
				{ID: "branch-setup", Instruction: "Set up branch"},
			},
		},
	}

	b := bus.NewLocalBus()
	orch := New(DefaultConfig(), reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-dedup", "gastown-prime", "mol-polecat-work-tdd", "load-context")

	ctx := context.Background()
	output := "ok  \tgithub.com/test/pkg\t0.1s\nSTEP_COMPLETE load-context\n"

	// Enqueue the same output twice — simulate two ticks seeing the same pane.
	reader.Enqueue("polecat-dedup", output)
	reader.Enqueue("polecat-dedup", output)

	orch.Tick(ctx)
	orch.Tick(ctx)

	// Only one SendStep despite two ticks with the same output.
	assert.Len(t, writer.Sends(), 1, "dedup should prevent double-processing of identical output")
}

// TestOrchestratorNoMarkerIsNoop verifies that a tick with no STEP_COMPLETE
// marker in the output is a no-op — the polecat is still working.
func TestOrchestratorNoMarkerIsNoop(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "load-context", Instruction: "Load context"},
			},
		},
	}

	b := bus.NewLocalBus()
	orch := New(DefaultConfig(), reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-noop", "gastown-prime", "mol-polecat-work-tdd", "load-context")

	ctx := context.Background()

	// Output with no marker — polecat is still working.
	reader.Enqueue("polecat-noop", "Compiling...\nstill working...\n")

	orch.Tick(ctx)

	assert.Empty(t, writer.Sends(), "no marker means no action")
}

// TestOrchestratorFullTDDCycle runs a simulated 5-step TDD cycle
// (write-tests → verify-red → implement → verify-green → refactor)
// to validate multi-turn orchestration across all TDD expansion steps.
//
// Each step body uses a recognizable success pattern so the matcher classifies
// it as CategorySuccess and the router advances. This tests the happy path
// where every step succeeds and the orchestrator drives through all 5 steps.
func TestOrchestratorFullTDDCycle(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "implement.write-tests", Instruction: "Write failing tests"},
				{ID: "implement.verify-red", Instruction: "Verify tests fail (red)"},
				{ID: "implement.implement", Instruction: "Implement to green"},
				{ID: "implement.verify-green", Instruction: "Verify all tests pass (green)"},
				{ID: "implement.refactor", Instruction: "Refactor"},
			},
		},
	}

	b := bus.NewLocalBus()
	collector := &eventCollector{}
	b.Subscribe("orchestrator:step:gastown-prime", collector.handler)

	orch := New(DefaultConfig(), reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-tdd", "gastown-prime", "mol-polecat-work-tdd", "implement.write-tests")

	ctx := context.Background()

	// Each step body includes a commit pattern — matches OutcomeCommit (CategorySuccess).
	// This is the simplest unambiguous success signal for steps that don't naturally
	// produce test runner output.
	stepOutputs := []struct {
		body   string
		stepID string
	}{
		{
			body:   "Writing tests...\n[polecat/fix abc1234] test: add failing tests\nSTEP_COMPLETE implement.write-tests\n",
			stepID: "implement.write-tests",
		},
		{
			body:   "Tests failed as expected.\n[polecat/fix bcd2345] test: verify red\nSTEP_COMPLETE implement.verify-red\n",
			stepID: "implement.verify-red",
		},
		{
			body:   "Implementing solution...\n[polecat/fix cde3456] feat: implement feature\nSTEP_COMPLETE implement.implement\n",
			stepID: "implement.implement",
		},
		{
			body:   "ok  \tgithub.com/test/pkg\t0.5s\nSTEP_COMPLETE implement.verify-green\n",
			stepID: "implement.verify-green",
		},
		{
			// Final step — no next step, should complete.
			body:   "No refactoring needed.\n[polecat/fix def4567] refactor: clean up\nSTEP_COMPLETE implement.refactor\n",
			stepID: "implement.refactor",
		},
	}

	expectedInstructions := []string{
		"Verify tests fail (red)",
		"Implement to green",
		"Verify all tests pass (green)",
		"Refactor",
		// No instruction for the last step — it completes.
	}

	for i, so := range stepOutputs {
		reader.Enqueue("polecat-tdd", so.body)
		orch.Tick(ctx)

		events := collector.Events()
		require.Len(t, events, i+1, "expected %d events after step %d (%s)", i+1, i+1, so.stepID)

		if i < len(stepOutputs)-1 {
			// Intermediate steps should advance.
			assert.Equal(t, bus.StepAdvanced, events[i].Type, "step %d (%s) should advance", i+1, so.stepID)

			sends := writer.Sends()
			require.Len(t, sends, i+1, "expected %d sends after step %d", i+1, i+1)
			assert.Equal(t, expectedInstructions[i], sends[i].Instruction)
		} else {
			// Final step should complete.
			assert.Equal(t, bus.StepCompleted, events[i].Type, "final step should complete")
		}
	}

	// Verify total: 4 advances + 1 completion.
	sends := writer.Sends()
	assert.Len(t, sends, 4, "should have 4 SendStep calls (not 5 — final step completes without sending)")
}

// TestOrchestratorMultiSessionIndependence verifies that the orchestrator
// handles multiple concurrent polecat sessions independently.
func TestOrchestratorMultiSessionIndependence(t *testing.T) {
	reader := newFakeReader()
	writer := &fakeWriter{}
	steps := &fakeStepProvider{
		formulas: map[string][]stepEntry{
			"mol-polecat-work-tdd": {
				{ID: "load-context", Instruction: "Load context"},
				{ID: "branch-setup", Instruction: "Set up branch"},
				{ID: "implement.write-tests", Instruction: "Write failing tests"},
			},
		},
	}

	b := bus.NewLocalBus()
	collector := &eventCollector{}
	b.Subscribe("orchestrator:step:rig-a", collector.handler)
	b.Subscribe("orchestrator:step:rig-b", collector.handler)

	orch := New(DefaultConfig(), reader, writer, steps, nil, b, testLogger())
	orch.Register("polecat-a", "rig-a", "mol-polecat-work-tdd", "load-context")
	orch.Register("polecat-b", "rig-b", "mol-polecat-work-tdd", "load-context")

	ctx := context.Background()

	// Only polecat-a completes its step.
	reader.Enqueue("polecat-a", "ok  \tgithub.com/test/a\t0.1s\nSTEP_COMPLETE load-context\n")
	reader.Enqueue("polecat-b", "still working on context...\n")

	orch.Tick(ctx)

	sends := writer.Sends()
	require.Len(t, sends, 1, "only polecat-a should advance")
	assert.Equal(t, "polecat-a", sends[0].Session)
	assert.Equal(t, "Set up branch", sends[0].Instruction)

	// Now polecat-b completes.
	reader.Enqueue("polecat-a", "") // no new output
	reader.Enqueue("polecat-b", "ok  \tgithub.com/test/b\t0.2s\nSTEP_COMPLETE load-context\n")

	orch.Tick(ctx)

	sends = writer.Sends()
	require.Len(t, sends, 2, "polecat-b should now advance too")
	assert.Equal(t, "polecat-b", sends[1].Session)
	assert.Equal(t, "Set up branch", sends[1].Instruction)

	// Verify events were published to the correct rig channels.
	events := collector.Events()
	require.Len(t, events, 2)
	assert.Equal(t, "rig-a", events[0].Rig)
	assert.Equal(t, "rig-b", events[1].Rig)
}
