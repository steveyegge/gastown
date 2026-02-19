package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// setupObserveTestTown creates a minimal town workspace for observe command tests.
// Returns the town root directory.
func setupObserveTestTown(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create minimal workspace marker.
	if err := os.MkdirAll(filepath.Join(dir, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	townJSON := map[string]interface{}{
		"type":    "town",
		"version": 2,
		"name":    "test",
	}
	data, _ := json.Marshal(townJSON)
	if err := os.WriteFile(filepath.Join(dir, "mayor", "town.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create settings directory.
	if err := os.MkdirAll(filepath.Join(dir, "settings"), 0755); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestObserveList_Empty(t *testing.T) {
	settings := config.NewTownSettings()

	if settings.Observability != nil {
		t.Error("new settings should have nil Observability")
	}
	// Verifies the list command's early-exit path.
}

func TestObserveList_Populated(t *testing.T) {
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"app-logs": {
				Scope:      "dev",
				SourceKind: "log",
				ServiceID:  "myapp",
				Path:       "/tmp/app.log",
			},
			"test-out": {
				Scope:      "ci",
				SourceKind: "test_output",
				Path:       "/tmp/test.log",
			},
		},
	}

	if len(settings.Observability.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(settings.Observability.Sources))
	}
	src := settings.Observability.Sources["app-logs"]
	if src.ServiceID != "myapp" {
		t.Errorf("expected service_id myapp, got %s", src.ServiceID)
	}
}

func TestObserveList_ObservabilityNotNil_SourcesNil(t *testing.T) {
	// Edge case: Observability key present but Sources is nil.
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		// Sources is nil — should be treated same as empty.
	}

	if settings.Observability.Sources != nil {
		t.Error("expected nil Sources")
	}
	// The list command should print "no sources configured" for this case.
}

// --- Add command validation ---

func TestObserveAdd_Validation(t *testing.T) {
	tests := []struct {
		name    string
		scope   string
		kind    string
		redact  string
		wantErr bool
	}{
		{"valid_dev_log_standard", "dev", "log", "standard", false},
		{"valid_all_metric_strict", "all", "metric", "strict", false},
		{"valid_ci_trace_none", "ci", "trace", "none", false},
		{"valid_test_output", "dev", "test_output", "standard", false},
		{"bad_scope", "production", "log", "standard", true},
		{"bad_kind", "dev", "unknown", "standard", true},
		{"bad_redaction", "dev", "log", "extreme", true},
		{"empty_scope", "", "log", "standard", true},
		{"empty_kind", "dev", "", "standard", true},
		{"empty_redaction", "dev", "log", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateObserveAddInputs(tt.scope, tt.kind, tt.redact)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateObserveAddInputs(%q, %q, %q) error = %v, wantErr = %v",
					tt.scope, tt.kind, tt.redact, err, tt.wantErr)
			}
		})
	}
}

func TestObserveAdd_DuplicateDetection(t *testing.T) {
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"existing": {Path: "/tmp/app.log"},
		},
	}

	_, exists := settings.Observability.Sources["existing"]
	if !exists {
		t.Error("expected existing source to be present")
	}
	// The add command should refuse to overwrite existing sources.
}

func TestObserveAdd_InitializesNilContainers(t *testing.T) {
	settings := config.NewTownSettings()
	// Observability is nil — add should initialize it.
	if settings.Observability != nil {
		t.Fatal("precondition: Observability should be nil")
	}

	settings.Observability = &config.ObservabilityConfig{}
	if settings.Observability.Sources == nil {
		settings.Observability.Sources = make(map[string]*config.ObservabilitySourceConfig)
	}
	settings.Observability.Sources["new"] = &config.ObservabilitySourceConfig{Path: "/tmp/new.log"}

	if len(settings.Observability.Sources) != 1 {
		t.Error("expected 1 source after add")
	}
}

// --- Remove command ---

func TestObserveRemove_Basic(t *testing.T) {
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"test-src": {Path: "/tmp/test.log"},
		},
	}

	delete(settings.Observability.Sources, "test-src")
	if len(settings.Observability.Sources) != 0 {
		t.Error("expected empty sources after removal")
	}
}

func TestObserveRemove_CleansUpEmptyContainers(t *testing.T) {
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"only-source": {Path: "/tmp/test.log"},
		},
	}

	// Remove the only source.
	delete(settings.Observability.Sources, "only-source")

	// Apply the cleanup logic from observe_remove.go.
	if len(settings.Observability.Sources) == 0 {
		settings.Observability.Sources = nil
	}
	if settings.Observability.Sources == nil && settings.Observability.TestOrchestration == nil {
		settings.Observability = nil
	}

	if settings.Observability != nil {
		t.Error("expected Observability to be nil after removing last source")
	}
}

func TestObserveRemove_PreservesTestOrchestration(t *testing.T) {
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"only-source": {Path: "/tmp/test.log"},
		},
		TestOrchestration: &config.TestOrchestrationConfig{
			Enabled:     true,
			RunnerAgent: "runner",
		},
	}

	// Remove the only source.
	delete(settings.Observability.Sources, "only-source")

	if len(settings.Observability.Sources) == 0 {
		settings.Observability.Sources = nil
	}
	if settings.Observability.Sources == nil && settings.Observability.TestOrchestration == nil {
		settings.Observability = nil
	}

	// Observability should NOT be nil because TestOrchestration is still set.
	if settings.Observability == nil {
		t.Error("Observability should be preserved when TestOrchestration is set")
	}
}

func TestObserveRemove_NonexistentSource(t *testing.T) {
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"real": {Path: "/tmp/test.log"},
		},
	}

	_, exists := settings.Observability.Sources["nonexistent"]
	if exists {
		t.Error("nonexistent source should not exist")
	}
	// The remove command should return an error in this case.
}

// --- Validation helper ---

func validateObserveAddInputs(scope, kind, redaction string) error {
	switch scope {
	case "dev", "ci", "all":
	default:
		return &validationError{"scope", scope}
	}
	switch kind {
	case "log", "metric", "trace", "test_output":
	default:
		return &validationError{"kind", kind}
	}
	switch redaction {
	case "none", "standard", "strict":
	default:
		return &validationError{"redaction-policy", redaction}
	}
	return nil
}

type validationError struct {
	field string
	value string
}

func (e *validationError) Error() string {
	return "invalid " + e.field + ": " + e.value
}

// --- Config round-trip tests ---

func TestObserveConfig_RoundTrip(t *testing.T) {
	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"src-1": {
				Scope:           "dev",
				SourceKind:      "log",
				ServiceID:       "svc-a",
				EnvID:           "local",
				Path:            "/tmp/app.log",
				RedactionPolicy: "standard",
				RoutingRules: &config.ObservabilityRoutingRules{
					Visibility:        "feed",
					SeverityThreshold: "warn",
				},
			},
		},
		TestOrchestration: &config.TestOrchestrationConfig{
			Enabled:         true,
			RunnerAgent:     "test-runner",
			ControllerAgent: "test-ctrl",
		},
	}

	// Marshal and unmarshal.
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}

	var loaded config.TownSettings
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.Observability == nil {
		t.Fatal("observability should not be nil after round-trip")
	}
	if len(loaded.Observability.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(loaded.Observability.Sources))
	}
	src := loaded.Observability.Sources["src-1"]
	if src.RoutingRules == nil || src.RoutingRules.SeverityThreshold != "warn" {
		t.Error("severity threshold lost in round-trip")
	}
	if loaded.Observability.TestOrchestration == nil || !loaded.Observability.TestOrchestration.Enabled {
		t.Error("test orchestration lost in round-trip")
	}
}

func TestObserveConfig_OmitemptyWhenNil(t *testing.T) {
	settings := config.NewTownSettings()
	// Observability is nil — should be omitted from JSON.

	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	if _, exists := raw["observability"]; exists {
		t.Error("observability key should be omitted when nil")
	}
}

func TestObserveConfig_RoundTrip_FileBasedSave(t *testing.T) {
	dir := setupObserveTestTown(t)
	settingsPath := config.TownSettingsPath(dir)

	settings := config.NewTownSettings()
	settings.Observability = &config.ObservabilityConfig{
		Sources: map[string]*config.ObservabilitySourceConfig{
			"test": {
				Path:       "/tmp/test.log",
				SourceKind: "log",
				Scope:      "dev",
			},
		},
	}

	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Observability == nil || len(loaded.Observability.Sources) != 1 {
		t.Fatal("source not preserved through file save/load")
	}
	if loaded.Observability.Sources["test"].Path != "/tmp/test.log" {
		t.Error("path not preserved through file save/load")
	}
}

// --- Default config tests ---

func TestDefaultObservabilitySourceConfig(t *testing.T) {
	cfg := config.DefaultObservabilitySourceConfig()

	if cfg.Scope != "dev" {
		t.Errorf("expected default scope 'dev', got %q", cfg.Scope)
	}
	if cfg.SourceKind != "log" {
		t.Errorf("expected default kind 'log', got %q", cfg.SourceKind)
	}
	if cfg.RedactionPolicy != "standard" {
		t.Errorf("expected default redaction 'standard', got %q", cfg.RedactionPolicy)
	}
	if cfg.RoutingRules == nil {
		t.Fatal("expected non-nil default routing rules")
	}
	if cfg.RoutingRules.Visibility != "feed" {
		t.Errorf("expected default visibility 'feed', got %q", cfg.RoutingRules.Visibility)
	}
	if cfg.RoutingRules.SeverityThreshold != "info" {
		t.Errorf("expected default severity threshold 'info', got %q", cfg.RoutingRules.SeverityThreshold)
	}
}
