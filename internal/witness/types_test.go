package witness

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
)

func TestStateTypeAlias(t *testing.T) {
	// Verify State is an alias for agent.State
	var s State = agent.StateRunning
	if s != agent.StateRunning {
		t.Errorf("State type alias not working correctly")
	}
}

func TestStateConstants(t *testing.T) {
	tests := []struct {
		name   string
		state  State
		parent agent.State
	}{
		{"StateStopped", StateStopped, agent.StateStopped},
		{"StateRunning", StateRunning, agent.StateRunning},
		{"StatePaused", StatePaused, agent.StatePaused},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.state != tt.parent {
				t.Errorf("State constant %s = %v, want %v", tt.name, tt.state, tt.parent)
			}
		})
	}
}

func TestWitness_ZeroValues(t *testing.T) {
	var w Witness

	if w.RigName != "" {
		t.Errorf("zero value Witness.RigName should be empty, got %q", w.RigName)
	}
	if w.State != "" {
		t.Errorf("zero value Witness.State should be empty, got %q", w.State)
	}
	if w.PID != 0 {
		t.Errorf("zero value Witness.PID should be 0, got %d", w.PID)
	}
	if w.StartedAt != nil {
		t.Error("zero value Witness.StartedAt should be nil")
	}
}

func TestWitness_JSONMarshaling(t *testing.T) {
	now := time.Now().Round(time.Second)
	w := Witness{
		RigName:    "gastown",
		State:      StateRunning,
		PID:        12345,
		StartedAt:  &now,
		MonitoredPolecats: []string{"keeper", "valkyrie"},
		Config: WitnessConfig{
			MaxWorkers:   4,
			SpawnDelayMs: 5000,
			AutoSpawn:    true,
		},
		SpawnedIssues: []string{"hq-abc123"},
	}

	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var unmarshaled Witness
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if unmarshaled.RigName != w.RigName {
		t.Errorf("After round-trip: RigName = %q, want %q", unmarshaled.RigName, w.RigName)
	}
	if unmarshaled.State != w.State {
		t.Errorf("After round-trip: State = %v, want %v", unmarshaled.State, w.State)
	}
	if unmarshaled.PID != w.PID {
		t.Errorf("After round-trip: PID = %d, want %d", unmarshaled.PID, w.PID)
	}
}

func TestWitnessConfig_ZeroValues(t *testing.T) {
	var cfg WitnessConfig

	if cfg.MaxWorkers != 0 {
		t.Errorf("zero value WitnessConfig.MaxWorkers should be 0, got %d", cfg.MaxWorkers)
	}
	if cfg.SpawnDelayMs != 0 {
		t.Errorf("zero value WitnessConfig.SpawnDelayMs should be 0, got %d", cfg.SpawnDelayMs)
	}
	if cfg.AutoSpawn {
		t.Error("zero value WitnessConfig.AutoSpawn should be false")
	}
}

func TestWitnessConfig_JSONMarshaling(t *testing.T) {
	cfg := WitnessConfig{
		MaxWorkers:   8,
		SpawnDelayMs: 10000,
		AutoSpawn:    false,
		EpicID:       "epic-123",
		IssuePrefix:  "gt-",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var unmarshaled WitnessConfig
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if unmarshaled.MaxWorkers != cfg.MaxWorkers {
		t.Errorf("After round-trip: MaxWorkers = %d, want %d", unmarshaled.MaxWorkers, cfg.MaxWorkers)
	}
	if unmarshaled.SpawnDelayMs != cfg.SpawnDelayMs {
		t.Errorf("After round-trip: SpawnDelayMs = %d, want %d", unmarshaled.SpawnDelayMs, cfg.SpawnDelayMs)
	}
	if unmarshaled.AutoSpawn != cfg.AutoSpawn {
		t.Errorf("After round-trip: AutoSpawn = %v, want %v", unmarshaled.AutoSpawn, cfg.AutoSpawn)
	}
	if unmarshaled.EpicID != cfg.EpicID {
		t.Errorf("After round-trip: EpicID = %q, want %q", unmarshaled.EpicID, cfg.EpicID)
	}
	if unmarshaled.IssuePrefix != cfg.IssuePrefix {
		t.Errorf("After round-trip: IssuePrefix = %q, want %q", unmarshaled.IssuePrefix, cfg.IssuePrefix)
	}
}

func TestWitnessConfig_OmitEmpty(t *testing.T) {
	cfg := WitnessConfig{
		MaxWorkers:   4,
		SpawnDelayMs: 5000,
		AutoSpawn:    true,
		// EpicID and IssuePrefix left empty
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() to map error = %v", err)
	}

	// Empty fields should be omitted
	if _, exists := raw["epic_id"]; exists {
		t.Error("Field 'epic_id' should be omitted when empty")
	}
	if _, exists := raw["issue_prefix"]; exists {
		t.Error("Field 'issue_prefix' should be omitted when empty")
	}

	// Required fields should be present
	requiredFields := []string{"max_workers", "spawn_delay_ms", "auto_spawn"}
	for _, field := range requiredFields {
		if _, exists := raw[field]; !exists {
			t.Errorf("Required field '%s' should be present", field)
		}
	}
}

func TestWitness_OmitEmpty(t *testing.T) {
	w := Witness{
		RigName: "gastown",
		State:   StateRunning,
		// PID, StartedAt, MonitoredPolecats, SpawnedIssues left empty/nil
	}

	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal() to map error = %v", err)
	}

	// Empty optional fields should be omitted
	if _, exists := raw["pid"]; exists {
		t.Error("Field 'pid' should be omitted when zero")
	}
	if _, exists := raw["started_at"]; exists {
		t.Error("Field 'started_at' should be omitted when nil")
	}
	if _, exists := raw["monitored_polecats"]; exists {
		t.Error("Field 'monitored_polecats' should be omitted when nil/empty")
	}
	if _, exists := raw["spawned_issues"]; exists {
		t.Error("Field 'spawned_issues' should be omitted when nil/empty")
	}
}

func TestWitness_WithMonitoredPolecats(t *testing.T) {
	w := Witness{
		RigName:    "gastown",
		State:      StateRunning,
		MonitoredPolecats: []string{"keeper", "valkyrie", "nux"},
	}

	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var unmarshaled Witness
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(unmarshaled.MonitoredPolecats) != 3 {
		t.Errorf("After round-trip: MonitoredPolecats length = %d, want 3", len(unmarshaled.MonitoredPolecats))
	}
}
