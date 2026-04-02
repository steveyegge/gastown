package integrations

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/outdoorsea/faultline/internal/notify"
)

// mockIntegration records calls for testing.
type mockIntegration struct {
	typ           IntegrationType
	newIssues     []notify.Event
	resolved      []notify.Event
	regressions   []notify.Event
	failOnNew     bool
}

func (m *mockIntegration) Type() IntegrationType { return m.typ }

func (m *mockIntegration) OnNewIssue(_ context.Context, event notify.Event) error {
	m.newIssues = append(m.newIssues, event)
	if m.failOnNew {
		return json.Unmarshal([]byte(`invalid`), &struct{}{})
	}
	return nil
}

func (m *mockIntegration) OnResolved(_ context.Context, event notify.Event) error {
	m.resolved = append(m.resolved, event)
	return nil
}

func (m *mockIntegration) OnRegression(_ context.Context, event notify.Event) error {
	m.regressions = append(m.regressions, event)
	return nil
}

func TestIsValidType(t *testing.T) {
	if !IsValidType(TypeGitHubIssues) {
		t.Error("github_issues should be valid")
	}
	if !IsValidType(TypePagerDuty) {
		t.Error("pagerduty should be valid")
	}
	if IsValidType("unknown") {
		t.Error("unknown should not be valid")
	}
}

func TestRegisterAndNew(t *testing.T) {
	// Register a test factory.
	testType := IntegrationType("test_type")
	Register(testType, func(config json.RawMessage) (Integration, error) {
		return &mockIntegration{typ: testType}, nil
	})
	defer delete(registry, testType)

	if !Registered(testType) {
		t.Fatal("expected test_type to be registered")
	}

	intg, err := New(testType, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if intg.Type() != testType {
		t.Errorf("expected type %s, got %s", testType, intg.Type())
	}
}

func TestNewUnknownType(t *testing.T) {
	_, err := New("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestDispatchRouting(t *testing.T) {
	m := &mockIntegration{typ: "test"}

	Dispatch(context.Background(), m, notify.Event{Type: notify.EventNewIssue, Title: "new"})
	Dispatch(context.Background(), m, notify.Event{Type: notify.EventResolved, Title: "resolved"})
	Dispatch(context.Background(), m, notify.Event{Type: notify.EventRegression, Title: "regressed"})

	if len(m.newIssues) != 1 {
		t.Errorf("expected 1 new issue event, got %d", len(m.newIssues))
	}
	if len(m.resolved) != 1 {
		t.Errorf("expected 1 resolved event, got %d", len(m.resolved))
	}
	if len(m.regressions) != 1 {
		t.Errorf("expected 1 regression event, got %d", len(m.regressions))
	}
}

// mockConfigProvider returns fixed configs for testing the Dispatcher.
type mockConfigProvider struct {
	configs []Config
}

func (m *mockConfigProvider) ListEnabledIntegrations(_ context.Context, _ int64) ([]Config, error) {
	return m.configs, nil
}

func TestDispatcherNotifyIntegrations(t *testing.T) {
	testType := IntegrationType("dispatch_test")
	var called int
	Register(testType, func(config json.RawMessage) (Integration, error) {
		called++
		return &mockIntegration{typ: testType}, nil
	})
	defer delete(registry, testType)

	provider := &mockConfigProvider{
		configs: []Config{
			{ID: "a", IntegrationType: string(testType), Enabled: true, Config: json.RawMessage(`{}`)},
			{ID: "b", IntegrationType: string(testType), Enabled: true, Config: json.RawMessage(`{}`)},
		},
	}

	d := NewDispatcher(provider, slog.Default())
	d.NotifyIntegrations(context.Background(), notify.Event{
		Type:      notify.EventNewIssue,
		ProjectID: 1,
		Title:     "test issue",
	})

	if called != 2 {
		t.Errorf("expected 2 integration instantiations, got %d", called)
	}
}

func TestDispatcherSkipsUnregisteredType(t *testing.T) {
	provider := &mockConfigProvider{
		configs: []Config{
			{ID: "x", IntegrationType: "totally_unknown", Enabled: true, Config: json.RawMessage(`{}`)},
		},
	}

	d := NewDispatcher(provider, slog.Default())
	// Should not panic — just log a warning.
	d.NotifyIntegrations(context.Background(), notify.Event{
		Type:      notify.EventNewIssue,
		ProjectID: 1,
	})
}
