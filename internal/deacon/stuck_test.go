package deacon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultStuckConfig(t *testing.T) {
	config := DefaultStuckConfig()

	if config.PingTimeout != DefaultPingTimeout {
		t.Errorf("PingTimeout = %v, want %v", config.PingTimeout, DefaultPingTimeout)
	}
	if config.ConsecutiveFailures != DefaultConsecutiveFailures {
		t.Errorf("ConsecutiveFailures = %v, want %v", config.ConsecutiveFailures, DefaultConsecutiveFailures)
	}
	if config.Cooldown != DefaultCooldown {
		t.Errorf("Cooldown = %v, want %v", config.Cooldown, DefaultCooldown)
	}
}

func TestHealthCheckStateFile(t *testing.T) {
	path := HealthCheckStateFile("/tmp/test-town")
	expected := "/tmp/test-town/deacon/health-check-state.json"
	if filepath.ToSlash(path) != expected {
		t.Errorf("HealthCheckStateFile = %q, want %q", path, expected)
	}
}

func TestLoadHealthCheckState_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	state, err := LoadHealthCheckState(tmpDir)
	if err != nil {
		t.Fatalf("LoadHealthCheckState() error = %v", err)
	}
	if state.Agents == nil {
		t.Error("Agents map should be initialized")
	}
	if len(state.Agents) != 0 {
		t.Errorf("Expected empty agents map, got %d entries", len(state.Agents))
	}
}

func TestSaveAndLoadHealthCheckState(t *testing.T) {
	tmpDir := t.TempDir()

	// Create state with some data
	state := &HealthCheckState{
		Agents: map[string]*AgentHealthState{
			"gastown/polecats/max": {
				AgentID:             "gastown/polecats/max",
				ConsecutiveFailures: 2,
				ForceKillCount:      1,
			},
		},
	}

	// Save
	if err := SaveHealthCheckState(tmpDir, state); err != nil {
		t.Fatalf("SaveHealthCheckState() error = %v", err)
	}

	// Verify file exists
	stateFile := HealthCheckStateFile(tmpDir)
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Fatal("State file was not created")
	}

	// Load
	loaded, err := LoadHealthCheckState(tmpDir)
	if err != nil {
		t.Fatalf("LoadHealthCheckState() error = %v", err)
	}

	// Verify loaded data
	agent := loaded.Agents["gastown/polecats/max"]
	if agent == nil {
		t.Fatal("Agent not found in loaded state")
	}
	if agent.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", agent.ConsecutiveFailures)
	}
	if agent.ForceKillCount != 1 {
		t.Errorf("ForceKillCount = %d, want 1", agent.ForceKillCount)
	}
}

func TestGetAgentState(t *testing.T) {
	state := &HealthCheckState{}

	// First call creates the agent
	agent1 := state.GetAgentState("test/agent")
	if agent1 == nil {
		t.Fatal("GetAgentState returned nil")
	}
	if agent1.AgentID != "test/agent" {
		t.Errorf("AgentID = %q, want %q", agent1.AgentID, "test/agent")
	}

	// Second call returns same agent
	agent2 := state.GetAgentState("test/agent")
	if agent1 != agent2 {
		t.Error("GetAgentState should return the same pointer")
	}
}

func TestAgentHealthState_RecordPing(t *testing.T) {
	agent := &AgentHealthState{}

	before := time.Now()
	agent.RecordPing()
	after := time.Now()

	if agent.LastPingTime.Before(before) || agent.LastPingTime.After(after) {
		t.Error("LastPingTime should be set to current time")
	}
}

func TestAgentHealthState_RecordResponse(t *testing.T) {
	agent := &AgentHealthState{
		ConsecutiveFailures: 5,
	}

	before := time.Now()
	agent.RecordResponse()
	after := time.Now()

	if agent.LastResponseTime.Before(before) || agent.LastResponseTime.After(after) {
		t.Error("LastResponseTime should be set to current time")
	}
	if agent.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures should be reset to 0, got %d", agent.ConsecutiveFailures)
	}
}

func TestAgentHealthState_RecordFailure(t *testing.T) {
	agent := &AgentHealthState{
		ConsecutiveFailures: 2,
	}

	agent.RecordFailure()

	if agent.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", agent.ConsecutiveFailures)
	}
}

func TestAgentHealthState_RecordForceKill(t *testing.T) {
	agent := &AgentHealthState{
		ConsecutiveFailures: 5,
		ForceKillCount:      2,
	}

	before := time.Now()
	agent.RecordForceKill()
	after := time.Now()

	if agent.LastForceKillTime.Before(before) || agent.LastForceKillTime.After(after) {
		t.Error("LastForceKillTime should be set to current time")
	}
	if agent.ForceKillCount != 3 {
		t.Errorf("ForceKillCount = %d, want 3", agent.ForceKillCount)
	}
	if agent.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures should be reset to 0, got %d", agent.ConsecutiveFailures)
	}
}

func TestAgentHealthState_IsInCooldown(t *testing.T) {
	cooldown := 5 * time.Minute

	tests := []struct {
		name              string
		lastForceKillTime time.Time
		want              bool
	}{
		{
			name:              "no force-kill history",
			lastForceKillTime: time.Time{},
			want:              false,
		},
		{
			name:              "recently killed",
			lastForceKillTime: time.Now().Add(-1 * time.Minute),
			want:              true,
		},
		{
			name:              "cooldown expired",
			lastForceKillTime: time.Now().Add(-10 * time.Minute),
			want:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &AgentHealthState{
				LastForceKillTime: tt.lastForceKillTime,
			}
			if got := agent.IsInCooldown(cooldown); got != tt.want {
				t.Errorf("IsInCooldown() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentHealthState_CooldownRemaining(t *testing.T) {
	cooldown := 5 * time.Minute

	tests := []struct {
		name              string
		lastForceKillTime time.Time
		wantZero          bool
	}{
		{
			name:              "no force-kill history",
			lastForceKillTime: time.Time{},
			wantZero:          true,
		},
		{
			name:              "recently killed",
			lastForceKillTime: time.Now().Add(-1 * time.Minute),
			wantZero:          false,
		},
		{
			name:              "cooldown expired",
			lastForceKillTime: time.Now().Add(-10 * time.Minute),
			wantZero:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &AgentHealthState{
				LastForceKillTime: tt.lastForceKillTime,
			}
			got := agent.CooldownRemaining(cooldown)
			if tt.wantZero && got != 0 {
				t.Errorf("CooldownRemaining() = %v, want 0", got)
			}
			if !tt.wantZero && got == 0 {
				t.Error("CooldownRemaining() = 0, want non-zero")
			}
		})
	}
}

func TestAgentHealthState_ShouldForceKill(t *testing.T) {
	tests := []struct {
		name      string
		failures  int
		threshold int
		want      bool
	}{
		{
			name:      "below threshold",
			failures:  2,
			threshold: 3,
			want:      false,
		},
		{
			name:      "at threshold",
			failures:  3,
			threshold: 3,
			want:      true,
		},
		{
			name:      "above threshold",
			failures:  5,
			threshold: 3,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := &AgentHealthState{
				ConsecutiveFailures: tt.failures,
			}
			if got := agent.ShouldForceKill(tt.threshold); got != tt.want {
				t.Errorf("ShouldForceKill() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveHealthCheckState_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nonexistent", "deacon")

	state := &HealthCheckState{
		Agents: make(map[string]*AgentHealthState),
	}

	// Should create the directory structure
	if err := SaveHealthCheckState(filepath.Join(tmpDir, "nonexistent"), state); err != nil {
		t.Fatalf("SaveHealthCheckState() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("Directory should have been created")
	}
}
