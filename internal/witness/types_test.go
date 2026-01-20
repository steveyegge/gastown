package witness

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/agent"
)

func TestWitness_ZeroValues(t *testing.T) {
	var w Witness

	if w.RigName != "" {
		t.Errorf("zero value Witness.RigName should be empty, got %q", w.RigName)
	}
	if w.State != "" {
		t.Errorf("zero value Witness.State should be empty, got %q", w.State)
	}
	if w.StartedAt != nil {
		t.Error("zero value Witness.StartedAt should be nil")
	}
}

func TestWitness_JSONMarshaling(t *testing.T) {
	now := time.Now().Round(time.Second)
	w := Witness{
		RigName:           "gastown",
		State:             agent.StateRunning,
		StartedAt:         &now,
		MonitoredPolecats: []string{"keeper", "valkyrie"},
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
		t.Errorf("RigName = %q, want %q", unmarshaled.RigName, w.RigName)
	}
	if unmarshaled.State != w.State {
		t.Errorf("State = %q, want %q", unmarshaled.State, w.State)
	}
	if len(unmarshaled.MonitoredPolecats) != len(w.MonitoredPolecats) {
		t.Errorf("MonitoredPolecats length = %d, want %d", len(unmarshaled.MonitoredPolecats), len(w.MonitoredPolecats))
	}
}

func TestWitness_SetRunning(t *testing.T) {
	var w Witness
	now := time.Now()

	w.SetRunning(now)

	if w.State != agent.StateRunning {
		t.Errorf("State = %q, want %q", w.State, agent.StateRunning)
	}
	if w.StartedAt == nil {
		t.Error("StartedAt should not be nil after SetRunning")
	}
	if !w.StartedAt.Equal(now) {
		t.Errorf("StartedAt = %v, want %v", w.StartedAt, now)
	}
}

func TestWitness_SetStopped(t *testing.T) {
	w := Witness{
		State: agent.StateRunning,
	}

	w.SetStopped()

	if w.State != agent.StateStopped {
		t.Errorf("State = %q, want %q", w.State, agent.StateStopped)
	}
}

func TestWitness_IsRunning(t *testing.T) {
	tests := []struct {
		name     string
		state    agent.State
		expected bool
	}{
		{"running", agent.StateRunning, true},
		{"stopped", agent.StateStopped, false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := Witness{State: tt.state}
			if got := w.IsRunning(); got != tt.expected {
				t.Errorf("IsRunning() = %v, want %v", got, tt.expected)
			}
		})
	}
}
