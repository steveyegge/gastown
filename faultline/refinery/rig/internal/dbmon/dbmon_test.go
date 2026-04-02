package dbmon

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockProvider implements DBProvider for testing.
type mockProvider struct {
	mu       sync.Mutex
	targets  []DatabaseTarget
	checks   []CheckResult
	states   map[string]*MonitorState
	writeErr error
}

func newMockProvider(targets []DatabaseTarget) *mockProvider {
	return &mockProvider{
		targets: targets,
		states:  make(map[string]*MonitorState),
	}
}

func (p *mockProvider) ListMonitoredDatabases(_ context.Context) ([]DatabaseTarget, error) {
	return p.targets, nil
}

func (p *mockProvider) WriteCheckResults(_ context.Context, results []CheckResult) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.writeErr != nil {
		return p.writeErr
	}
	p.checks = append(p.checks, results...)
	return nil
}

func (p *mockProvider) LoadMonitorState(_ context.Context, databaseID string) (*MonitorState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.states[databaseID]; ok {
		return s, nil
	}
	return &MonitorState{DatabaseID: databaseID, Status: StatusHealthy}, nil
}

func (p *mockProvider) SaveMonitorState(_ context.Context, state *MonitorState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states[state.DatabaseID] = state
	return nil
}

func (p *mockProvider) getChecks() []CheckResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]CheckResult, len(p.checks))
	copy(out, p.checks)
	return out
}

func (p *mockProvider) getState(id string) *MonitorState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.states[id]
}

func TestEvaluateStatus(t *testing.T) {
	tests := []struct {
		name    string
		results []CheckResult
		want    Status
	}{
		{
			name:    "all ok",
			results: []CheckResult{{Status: CheckOK}, {Status: CheckOK}},
			want:    StatusHealthy,
		},
		{
			name:    "one warning",
			results: []CheckResult{{Status: CheckOK}, {Status: CheckWarning}},
			want:    StatusDegraded,
		},
		{
			name:    "one critical",
			results: []CheckResult{{Status: CheckOK}, {Status: CheckCritical}},
			want:    StatusDown,
		},
		{
			name:    "critical overrides warning",
			results: []CheckResult{{Status: CheckWarning}, {Status: CheckCritical}},
			want:    StatusDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateStatus(tt.results)
			if got != tt.want {
				t.Errorf("evaluateStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonitor_StateTransitions(t *testing.T) {
	provider := newMockProvider([]DatabaseTarget{
		{ID: "db-1", Name: "test-pg", DBType: "postgres", Enabled: true, CheckIntervalS: 1},
	})

	m := New(provider, testLogger(), 5*time.Second)

	var mu sync.Mutex
	var transitions []struct{ old, new Status }

	m.OnStateChange = func(_ DatabaseTarget, oldS, newS Status, _ []CheckResult) {
		mu.Lock()
		defer mu.Unlock()
		transitions = append(transitions, struct{ old, new Status }{oldS, newS})
	}

	// First check: healthy (no transition expected on first run since initial state matches).
	callCount := 0
	m.RegisterChecker("postgres", func(_ context.Context, target DatabaseTarget) []CheckResult {
		callCount++
		if callCount <= 2 {
			return []CheckResult{{DatabaseID: target.ID, CheckType: "connection", Status: CheckOK, CheckedAt: time.Now().UTC()}}
		}
		return []CheckResult{{DatabaseID: target.ID, CheckType: "connection", Status: CheckCritical, Message: "connection refused", CheckedAt: time.Now().UTC()}}
	})

	// Run first tick — healthy, no state change (initial).
	ctx := context.Background()
	m.tick(ctx)

	mu.Lock()
	if len(transitions) != 0 {
		t.Errorf("expected no transitions on first check, got %d", len(transitions))
	}
	mu.Unlock()

	// Run second tick — still healthy, no state change.
	m.lastCheck["db-1"] = time.Time{} // force due
	m.tick(ctx)

	mu.Lock()
	if len(transitions) != 0 {
		t.Errorf("expected no transitions when status stable, got %d", len(transitions))
	}
	mu.Unlock()

	// Third tick — critical, should transition healthy→down.
	m.lastCheck["db-1"] = time.Time{} // force due
	m.tick(ctx)

	mu.Lock()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].old != StatusHealthy || transitions[0].new != StatusDown {
		t.Errorf("expected healthy→down, got %v→%v", transitions[0].old, transitions[0].new)
	}
	mu.Unlock()

	// Verify persisted state.
	state := provider.getState("db-1")
	if state == nil {
		t.Fatal("expected persisted state")
	}
	if state.Status != StatusDown {
		t.Errorf("expected persisted status down, got %v", state.Status)
	}
	if state.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", state.ConsecutiveFailures)
	}
}

func TestMonitor_DisabledTargetsSkipped(t *testing.T) {
	provider := newMockProvider([]DatabaseTarget{
		{ID: "db-1", Name: "disabled", DBType: "postgres", Enabled: false, CheckIntervalS: 1},
	})

	m := New(provider, testLogger(), 5*time.Second)

	checked := false
	m.RegisterChecker("postgres", func(_ context.Context, _ DatabaseTarget) []CheckResult {
		checked = true
		return nil
	})

	m.tick(context.Background())
	if checked {
		t.Error("expected disabled target to be skipped")
	}
}

func TestMonitor_UnregisteredDBType(t *testing.T) {
	provider := newMockProvider([]DatabaseTarget{
		{ID: "db-1", Name: "unknown", DBType: "mongodb", Enabled: true, CheckIntervalS: 1},
	})

	m := New(provider, testLogger(), 5*time.Second)
	// No checker registered for "mongodb".

	m.tick(context.Background())

	checks := provider.getChecks()
	if len(checks) == 0 {
		t.Fatal("expected a critical check result for unregistered type")
	}
	if checks[0].Status != CheckCritical {
		t.Errorf("expected critical status, got %v", checks[0].Status)
	}
}

func TestMonitor_IntervalRespected(t *testing.T) {
	provider := newMockProvider([]DatabaseTarget{
		{ID: "db-1", Name: "test", DBType: "test", Enabled: true, CheckIntervalS: 60},
	})

	m := New(provider, testLogger(), 5*time.Second)

	checkCount := 0
	m.RegisterChecker("test", func(_ context.Context, target DatabaseTarget) []CheckResult {
		checkCount++
		return []CheckResult{{DatabaseID: target.ID, CheckType: "ping", Status: CheckOK, CheckedAt: time.Now().UTC()}}
	})

	// First tick runs the check (never checked before).
	m.tick(context.Background())
	if checkCount != 1 {
		t.Fatalf("expected 1 check on first tick, got %d", checkCount)
	}

	// Second tick should skip (interval not elapsed).
	m.tick(context.Background())
	if checkCount != 1 {
		t.Errorf("expected check to be skipped (interval not elapsed), got %d checks", checkCount)
	}
}

func TestMonitor_WorkerPoolBounds(t *testing.T) {
	// Create more targets than max workers.
	var targets []DatabaseTarget
	for i := 0; i < 20; i++ {
		targets = append(targets, DatabaseTarget{
			ID: "db-" + string(rune('a'+i)), Name: "test", DBType: "test",
			Enabled: true, CheckIntervalS: 1,
		})
	}
	provider := newMockProvider(targets)

	m := New(provider, testLogger(), 5*time.Second)
	m.maxWorkers = 5

	var mu sync.Mutex
	maxConcurrent := 0
	current := 0

	m.RegisterChecker("test", func(_ context.Context, target DatabaseTarget) []CheckResult {
		mu.Lock()
		current++
		if current > maxConcurrent {
			maxConcurrent = current
		}
		mu.Unlock()

		time.Sleep(10 * time.Millisecond)

		mu.Lock()
		current--
		mu.Unlock()

		return []CheckResult{{DatabaseID: target.ID, CheckType: "ping", Status: CheckOK, CheckedAt: time.Now().UTC()}}
	})

	m.tick(context.Background())

	if maxConcurrent > 5 {
		t.Errorf("expected max concurrency <= 5, got %d", maxConcurrent)
	}
	if maxConcurrent == 0 {
		t.Error("expected at least some concurrent checks")
	}
}

func TestMonitor_RunContextCancellation(t *testing.T) {
	provider := newMockProvider(nil)
	m := New(provider, testLogger(), 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Run exited on cancellation — good.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}
}

func TestNewCheckResult(t *testing.T) {
	val := 42.5
	r := NewCheckResult("db-1", "latency", CheckWarning, &val, "slow")
	if r.DatabaseID != "db-1" {
		t.Errorf("unexpected DatabaseID: %s", r.DatabaseID)
	}
	if r.CheckType != "latency" {
		t.Errorf("unexpected CheckType: %s", r.CheckType)
	}
	if r.Status != CheckWarning {
		t.Errorf("unexpected Status: %v", r.Status)
	}
	if r.Value == nil || *r.Value != 42.5 {
		t.Errorf("unexpected Value: %v", r.Value)
	}
	if r.CheckedAt.IsZero() {
		t.Error("expected non-zero CheckedAt")
	}
}
