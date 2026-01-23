package witness

import (
	"encoding/json"
	"testing"
)

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
