package crew

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCrewWorker_Summary(t *testing.T) {
	now := time.Now()
	worker := &CrewWorker{
		Name:      "test-worker",
		Rig:       "gastown",
		ClonePath: "/path/to/clone",
		Branch:    "main",
		CreatedAt: now,
		UpdatedAt: now,
	}

	summary := worker.Summary()

	if summary.Name != worker.Name {
		t.Errorf("Summary.Name = %q, want %q", summary.Name, worker.Name)
	}
	if summary.Branch != worker.Branch {
		t.Errorf("Summary.Branch = %q, want %q", summary.Branch, worker.Branch)
	}
}

func TestCrewWorker_JSONMarshaling(t *testing.T) {
	now := time.Now().Round(time.Second) // Round for JSON precision
	worker := &CrewWorker{
		Name:      "test-worker",
		Rig:       "gastown",
		ClonePath: "/path/to/clone",
		Branch:    "feature-branch",
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Marshal to JSON
	data, err := json.Marshal(worker)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal back
	var unmarshaled CrewWorker
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if unmarshaled.Name != worker.Name {
		t.Errorf("After round-trip: Name = %q, want %q", unmarshaled.Name, worker.Name)
	}
	if unmarshaled.Rig != worker.Rig {
		t.Errorf("After round-trip: Rig = %q, want %q", unmarshaled.Rig, worker.Rig)
	}
	if unmarshaled.Branch != worker.Branch {
		t.Errorf("After round-trip: Branch = %q, want %q", unmarshaled.Branch, worker.Branch)
	}
}

func TestSummary_JSONMarshaling(t *testing.T) {
	summary := Summary{
		Name:   "worker-1",
		Branch: "main",
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var unmarshaled Summary
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if unmarshaled.Name != summary.Name {
		t.Errorf("After round-trip: Name = %q, want %q", unmarshaled.Name, summary.Name)
	}
	if unmarshaled.Branch != summary.Branch {
		t.Errorf("After round-trip: Branch = %q, want %q", unmarshaled.Branch, summary.Branch)
	}
}

func TestCrewWorker_ZeroValues(t *testing.T) {
	var worker CrewWorker

	// Test zero value behavior
	if worker.Name != "" {
		t.Errorf("zero value CrewWorker.Name should be empty, got %q", worker.Name)
	}

	summary := worker.Summary()
	if summary.Name != "" {
		t.Errorf("Summary of zero value CrewWorker should have empty Name, got %q", summary.Name)
	}
}
