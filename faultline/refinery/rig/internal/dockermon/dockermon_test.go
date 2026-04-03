package dockermon

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Mock Docker client ---

type mockDocker struct {
	mu         sync.Mutex
	containers []types.Container
	inspected  map[string]types.ContainerJSON
	stats      map[string]*dockerStatsJSON
	eventsCh   chan events.Message
	errCh      chan error
	listErr    error
	inspectErr error
	statsErr   error
}

func newMockDocker() *mockDocker {
	return &mockDocker{
		inspected: make(map[string]types.ContainerJSON),
		stats:     make(map[string]*dockerStatsJSON),
		eventsCh:  make(chan events.Message, 10),
		errCh:     make(chan error, 1),
	}
}

func (m *mockDocker) ContainerList(_ context.Context, _ container.ListOptions) ([]types.Container, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.containers, nil
}

func (m *mockDocker) ContainerInspect(_ context.Context, containerID string) (types.ContainerJSON, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inspectErr != nil {
		return types.ContainerJSON{}, m.inspectErr
	}
	if info, ok := m.inspected[containerID]; ok {
		return info, nil
	}
	return types.ContainerJSON{}, nil
}

func (m *mockDocker) ContainerStatsOneShot(_ context.Context, containerID string) (container.StatsResponseReader, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.statsErr != nil {
		return container.StatsResponseReader{}, m.statsErr
	}
	s, ok := m.stats[containerID]
	if !ok {
		s = &dockerStatsJSON{}
	}
	data, _ := json.Marshal(s)
	return container.StatsResponseReader{
		Body: io.NopCloser(bytes.NewReader(data)),
	}, nil
}

func (m *mockDocker) Events(_ context.Context, _ events.ListOptions) (<-chan events.Message, <-chan error) {
	return m.eventsCh, m.errCh
}

func (m *mockDocker) Close() error { return nil }

// --- Mock DB provider ---

type mockProvider struct {
	mu         sync.Mutex
	containers map[string]*Container
	checks     []CheckResult
	states     map[string]*MonitorState
	writeErr   error
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		containers: make(map[string]*Container),
		states:     make(map[string]*MonitorState),
	}
}

func (p *mockProvider) UpsertContainer(_ context.Context, c *Container) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.containers[c.ContainerID] = c
	return nil
}

func (p *mockProvider) ListContainers(_ context.Context) ([]Container, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []Container
	for _, c := range p.containers {
		out = append(out, *c)
	}
	return out, nil
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

func (p *mockProvider) LoadMonitorState(_ context.Context, containerID string) (*MonitorState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.states[containerID]; ok {
		return s, nil
	}
	return &MonitorState{ContainerID: containerID, Status: StatusHealthy}, nil
}

func (p *mockProvider) SaveMonitorState(_ context.Context, state *MonitorState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states[state.ContainerID] = state
	return nil
}

func (p *mockProvider) MarkContainerLastSeen(_ context.Context, _ string, _ time.Time) error {
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

// --- Tests ---

func TestEvaluateStatus(t *testing.T) {
	tests := []struct {
		name    string
		results []CheckResult
		want    Status
	}{
		{
			name:    "no results",
			results: nil,
			want:    StatusHealthy,
		},
		{
			name: "all ok",
			results: []CheckResult{
				{Status: CheckOK},
				{Status: CheckOK},
			},
			want: StatusHealthy,
		},
		{
			name: "warning downgrades to degraded",
			results: []CheckResult{
				{Status: CheckOK},
				{Status: CheckWarning},
			},
			want: StatusDegraded,
		},
		{
			name: "critical downgrades to down",
			results: []CheckResult{
				{Status: CheckOK},
				{Status: CheckCritical},
			},
			want: StatusDown,
		},
		{
			name: "critical takes precedence over warning",
			results: []CheckResult{
				{Status: CheckWarning},
				{Status: CheckCritical},
			},
			want: StatusDown,
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

func TestDefaultThresholds(t *testing.T) {
	th := DefaultThresholds()
	if th.MemoryWarning != 0.80 {
		t.Errorf("MemoryWarning = %v, want 0.80", th.MemoryWarning)
	}
	if th.MemoryCritical != 0.95 {
		t.Errorf("MemoryCritical = %v, want 0.95", th.MemoryCritical)
	}
	if th.CPUWarning != 0.50 {
		t.Errorf("CPUWarning = %v, want 0.50", th.CPUWarning)
	}
	if th.RestartCritical != 5 {
		t.Errorf("RestartCritical = %v, want 5", th.RestartCritical)
	}
}

func TestDiscoverContainers(t *testing.T) {
	docker := newMockDocker()
	docker.containers = []types.Container{
		{
			ID:     "abc123",
			Names:  []string{"/my-app"},
			Image:  "myimage:latest",
			Labels: map[string]string{"faultline.monitor": "true", "com.docker.compose.service": "web"},
		},
	}
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	err := mon.discoverContainers(context.Background())
	if err != nil {
		t.Fatalf("discoverContainers: %v", err)
	}

	mon.mu.RLock()
	count := len(mon.containers)
	mon.mu.RUnlock()
	if count != 1 {
		t.Errorf("tracked containers = %d, want 1", count)
	}

	provider.mu.Lock()
	upserted := len(provider.containers)
	provider.mu.Unlock()
	if upserted != 1 {
		t.Errorf("upserted containers = %d, want 1", upserted)
	}
}

func TestDiscoverContainers_SocketUnavailable(t *testing.T) {
	docker := newMockDocker()
	docker.listErr = io.ErrUnexpectedEOF
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	err := mon.discoverContainers(context.Background())
	if err == nil {
		t.Fatal("expected error when socket unavailable")
	}
}

func TestCheckContainer_MemoryWarning(t *testing.T) {
	docker := newMockDocker()
	docker.stats["docker-123"] = &dockerStatsJSON{
		MemoryStats: struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		}{Usage: 850, Limit: 1000},
	}

	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{
		ID:          "c1",
		ContainerID: "docker-123",
	}
	results := mon.checkContainer(context.Background(), c)

	found := false
	for _, r := range results {
		if r.CheckType == "memory" {
			found = true
			if r.Status != CheckWarning {
				t.Errorf("memory status = %v, want %v (85%% usage)", r.Status, CheckWarning)
			}
		}
	}
	if !found {
		t.Error("no memory check result")
	}
}

func TestCheckContainer_MemoryCritical(t *testing.T) {
	docker := newMockDocker()
	docker.stats["docker-123"] = &dockerStatsJSON{
		MemoryStats: struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		}{Usage: 960, Limit: 1000},
	}

	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{
		ID:          "c1",
		ContainerID: "docker-123",
	}
	results := mon.checkContainer(context.Background(), c)

	for _, r := range results {
		if r.CheckType == "memory" && r.Status != CheckCritical {
			t.Errorf("memory status = %v, want %v (96%% usage)", r.Status, CheckCritical)
		}
	}
}

func TestCheckContainer_CPUThrottling(t *testing.T) {
	docker := newMockDocker()
	docker.stats["docker-123"] = &dockerStatsJSON{
		CPUStats: struct {
			ThrottlingData struct {
				ThrottledPeriods uint64 `json:"throttled_periods"`
				Periods          uint64 `json:"periods"`
			} `json:"throttling_data"`
		}{
			ThrottlingData: struct {
				ThrottledPeriods uint64 `json:"throttled_periods"`
				Periods          uint64 `json:"periods"`
			}{ThrottledPeriods: 60, Periods: 100},
		},
	}

	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{
		ID:          "c1",
		ContainerID: "docker-123",
	}
	results := mon.checkContainer(context.Background(), c)

	found := false
	for _, r := range results {
		if r.CheckType == "cpu" {
			found = true
			if r.Status != CheckWarning {
				t.Errorf("cpu status = %v, want %v (60%% throttled)", r.Status, CheckWarning)
			}
		}
	}
	if !found {
		t.Error("no cpu check result")
	}
}

func TestProcessResults_StateTransition(t *testing.T) {
	docker := newMockDocker()
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	var transitions []struct{ old, new Status }
	mon.OnStateChange = func(_ Container, old, new Status) {
		transitions = append(transitions, struct{ old, new Status }{old, new})
	}

	c := Container{ID: "c1", ContainerName: "test-app"}

	// Start healthy, then critical → should transition to down
	results := []CheckResult{
		{ContainerID: "c1", CheckType: "memory", Status: CheckCritical, Message: "high mem"},
	}
	mon.processResults(context.Background(), c, results)

	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].old != StatusHealthy || transitions[0].new != StatusDown {
		t.Errorf("transition = %v→%v, want healthy→down", transitions[0].old, transitions[0].new)
	}

	// Now send OK → should transition back to healthy
	results = []CheckResult{
		{ContainerID: "c1", CheckType: "memory", Status: CheckOK},
	}
	mon.processResults(context.Background(), c, results)

	if len(transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(transitions))
	}
	if transitions[1].old != StatusDown || transitions[1].new != StatusHealthy {
		t.Errorf("transition = %v→%v, want down→healthy", transitions[1].old, transitions[1].new)
	}
}

func TestHandleEvent_DieNonZero(t *testing.T) {
	docker := newMockDocker()
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{ID: "c1", ContainerID: "docker-abc", ContainerName: "my-app"}
	mon.mu.Lock()
	mon.containers["docker-abc"] = c
	mon.mu.Unlock()

	msg := events.Message{
		Action: events.ActionDie,
		Actor: events.Actor{
			ID:         "docker-abc",
			Attributes: map[string]string{"faultline.monitor": "true", "exitCode": "137"},
		},
	}
	mon.handleEvent(context.Background(), msg)

	checks := provider.getChecks()
	if len(checks) == 0 {
		t.Fatal("no check results after die event")
	}
	if checks[0].CheckType != "stopped" {
		t.Errorf("check type = %v, want stopped", checks[0].CheckType)
	}
	if checks[0].Status != CheckCritical {
		t.Errorf("check status = %v, want critical", checks[0].Status)
	}
}

func TestHandleEvent_OOM(t *testing.T) {
	docker := newMockDocker()
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{ID: "c1", ContainerID: "docker-abc", ContainerName: "my-app", ServiceName: "web"}
	mon.mu.Lock()
	mon.containers["docker-abc"] = c
	mon.mu.Unlock()

	msg := events.Message{
		Action: events.ActionOOM,
		Actor: events.Actor{
			ID:         "docker-abc",
			Attributes: map[string]string{"faultline.monitor": "true"},
		},
	}
	mon.handleEvent(context.Background(), msg)

	checks := provider.getChecks()
	if len(checks) == 0 {
		t.Fatal("no check results after OOM event")
	}
	if checks[0].CheckType != "memory" || checks[0].Status != CheckCritical {
		t.Errorf("OOM check = %v/%v, want memory/critical", checks[0].CheckType, checks[0].Status)
	}
}

func TestHandleEvent_IgnoresUnlabeled(t *testing.T) {
	docker := newMockDocker()
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	msg := events.Message{
		Action: events.ActionDie,
		Actor: events.Actor{
			ID:         "docker-xyz",
			Attributes: map[string]string{"exitCode": "1"},
		},
	}
	mon.handleEvent(context.Background(), msg)

	checks := provider.getChecks()
	if len(checks) != 0 {
		t.Errorf("expected no checks for unlabeled container, got %d", len(checks))
	}
}

func TestContainerFromDocker(t *testing.T) {
	now := time.Now().UTC()
	dc := types.Container{
		ID:     "abc123",
		Names:  []string{"/my-app"},
		Image:  "myimage:v1",
		Labels: map[string]string{"com.docker.compose.service": "web"},
	}

	c := containerFromDocker(dc, now)
	if c.ContainerName != "my-app" {
		t.Errorf("name = %q, want %q (leading slash stripped)", c.ContainerName, "my-app")
	}
	if c.ContainerID != "abc123" {
		t.Errorf("docker id = %q, want %q", c.ContainerID, "abc123")
	}
	if c.ServiceName != "web" {
		t.Errorf("service = %q, want %q", c.ServiceName, "web")
	}
	if c.ID == "" {
		t.Error("generated ID should not be empty")
	}
}

func TestFingerprint(t *testing.T) {
	fp1 := Fingerprint("my-app", "memory")
	fp2 := Fingerprint("my-app", "memory")
	fp3 := Fingerprint("my-app", "cpu")

	if fp1 != fp2 {
		t.Error("same inputs should produce same fingerprint")
	}
	if fp1 == fp3 {
		t.Error("different inputs should produce different fingerprints")
	}
}

func TestDecodeStats(t *testing.T) {
	raw := dockerStatsJSON{
		MemoryStats: struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		}{Usage: 500, Limit: 1000},
		CPUStats: struct {
			ThrottlingData struct {
				ThrottledPeriods uint64 `json:"throttled_periods"`
				Periods          uint64 `json:"periods"`
			} `json:"throttling_data"`
		}{
			ThrottlingData: struct {
				ThrottledPeriods uint64 `json:"throttled_periods"`
				Periods          uint64 `json:"periods"`
			}{ThrottledPeriods: 10, Periods: 100},
		},
	}

	data, _ := json.Marshal(raw)
	stats, err := decodeStats(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decodeStats: %v", err)
	}
	if stats.MemoryUsage != 500 {
		t.Errorf("memory usage = %d, want 500", stats.MemoryUsage)
	}
	if stats.MemoryLimit != 1000 {
		t.Errorf("memory limit = %d, want 1000", stats.MemoryLimit)
	}
	if stats.ThrottledPeriods != 10 {
		t.Errorf("throttled = %d, want 10", stats.ThrottledPeriods)
	}
}

func TestNewCheckResult(t *testing.T) {
	val := 0.85
	r := NewCheckResult("c1", "memory", CheckWarning, &val, "high memory")
	if r.ContainerID != "c1" || r.CheckType != "memory" || r.Status != CheckWarning {
		t.Errorf("NewCheckResult fields wrong: %+v", r)
	}
	if r.Value == nil || *r.Value != 0.85 {
		t.Errorf("value = %v, want 0.85", r.Value)
	}
}

func TestPollOnce(t *testing.T) {
	docker := newMockDocker()
	docker.stats["docker-123"] = &dockerStatsJSON{
		MemoryStats: struct {
			Usage uint64 `json:"usage"`
			Limit uint64 `json:"limit"`
		}{Usage: 500, Limit: 1000},
	}

	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{
		ID:            "c1",
		ContainerID:   "docker-123",
		ContainerName: "test-app",
	}
	mon.mu.Lock()
	mon.containers["docker-123"] = c
	mon.mu.Unlock()

	mon.pollOnce(context.Background())

	checks := provider.getChecks()
	if len(checks) == 0 {
		t.Fatal("pollOnce produced no check results")
	}
}

func TestHandleContainerStop(t *testing.T) {
	docker := newMockDocker()
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{ID: "c1", ContainerID: "docker-abc", ContainerName: "my-app"}
	mon.mu.Lock()
	mon.containers["docker-abc"] = c
	mon.mu.Unlock()

	mon.handleContainerStop(context.Background(), "docker-abc", time.Now().UTC())

	mon.mu.RLock()
	_, exists := mon.containers["docker-abc"]
	mon.mu.RUnlock()
	if exists {
		t.Error("container should be removed from tracking after stop")
	}
}

func TestSetters(t *testing.T) {
	mon := New(newMockDocker(), newMockProvider(), testLogger())
	mon.SetPollInterval(10 * time.Second)
	if mon.pollInterval != 10*time.Second {
		t.Errorf("poll interval = %v, want 10s", mon.pollInterval)
	}
	mon.SetRetryInterval(120 * time.Second)
	if mon.retryInterval != 120*time.Second {
		t.Errorf("retry interval = %v, want 120s", mon.retryInterval)
	}
	th := Thresholds{MemoryWarning: 0.70}
	mon.SetThresholds(th)
	if mon.thresholds.MemoryWarning != 0.70 {
		t.Errorf("thresholds not set")
	}
}

func TestHandleContainerStop_RemovesFromMap(t *testing.T) {
	docker := newMockDocker()
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	// Note: handleContainerStop deletes from map first, then tries to look up
	// container (which will be nil). This tests the nil-safe path.
	mon.handleContainerStop(context.Background(), "nonexistent", time.Now().UTC())
	// Should not panic
}

func TestHandleEvent_IgnoreZeroExitDie(t *testing.T) {
	docker := newMockDocker()
	provider := newMockProvider()
	mon := New(docker, provider, testLogger())

	c := &Container{ID: "c1", ContainerID: "docker-abc"}
	mon.mu.Lock()
	mon.containers["docker-abc"] = c
	mon.mu.Unlock()

	msg := events.Message{
		Action: events.ActionDie,
		Actor: events.Actor{
			ID:         "docker-abc",
			Attributes: map[string]string{"faultline.monitor": "true", "exitCode": "0"},
		},
	}
	mon.handleEvent(context.Background(), msg)

	checks := provider.getChecks()
	if len(checks) != 0 {
		t.Errorf("exit code 0 should not produce check results, got %d", len(checks))
	}
}

// Verify the unused sql import is valid by checking Container struct uses it
func TestContainerThresholdsType(t *testing.T) {
	c := Container{}
	_ = c.Thresholds // sql.NullString
	if c.Thresholds != (sql.NullString{}) {
		t.Error("default NullString should be zero value")
	}
}
