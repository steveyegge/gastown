package autopilot

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStrategyIsValid(t *testing.T) {
	tests := []struct {
		s    Strategy
		want bool
	}{
		{StrategyPolecats, true},
		{StrategyCrew, true},
		{StrategyHybrid, true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := tt.s.IsValid(); got != tt.want {
			t.Errorf("Strategy(%q).IsValid() = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestResourceSnapshotMemoryHelpers(t *testing.T) {
	s := &ResourceSnapshot{
		MemoryTotalBytes:     8 * 1024 * 1024 * 1024, // 8 GB
		MemoryAvailableBytes: 2 * 1024 * 1024 * 1024, // 2 GB
	}

	if got := s.MemoryTotalGB(); got != 8.0 {
		t.Errorf("MemoryTotalGB() = %v, want 8.0", got)
	}
	if got := s.MemoryAvailableGB(); got != 2.0 {
		t.Errorf("MemoryAvailableGB() = %v, want 2.0", got)
	}
}

func TestDefaultGovernorConfig(t *testing.T) {
	cfg := DefaultGovernorConfig()

	if cfg.PollInterval != 60*time.Second {
		t.Errorf("PollInterval = %v, want 60s", cfg.PollInterval)
	}
	if cfg.MemoryFloorGB != 1.0 {
		t.Errorf("MemoryFloorGB = %v, want 1.0", cfg.MemoryFloorGB)
	}
	if cfg.MemoryResumeGB != 1.5 {
		t.Errorf("MemoryResumeGB = %v, want 1.5", cfg.MemoryResumeGB)
	}
	if cfg.CPUCeiling != 85.0 {
		t.Errorf("CPUCeiling = %v, want 85.0", cfg.CPUCeiling)
	}
	if cfg.CPUFloor != 75.0 {
		t.Errorf("CPUFloor = %v, want 75.0", cfg.CPUFloor)
	}
	if cfg.MaxPolecats != 0 {
		t.Errorf("MaxPolecats = %v, want 0 (caller must set)", cfg.MaxPolecats)
	}
}

func TestAutopilotPlanJSONRoundtrip(t *testing.T) {
	plan := AutopilotPlan{
		EpicTitle: "implement computer-use verification",
		Tasks: []TaskNode{
			{
				ID:    "t1",
				Title: "create data types",
				Type:  "task",
			},
			{
				ID:        "t2",
				Title:     "implement resource snapshot",
				Type:      "task",
				DependsOn: []string{"t1"},
				Priority:  1,
			},
		},
		Strategy:       StrategyPolecats,
		MaxConcurrency: 2,
		Reasoning:      "independent tasks suitable for parallel polecats",
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded AutopilotPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.EpicTitle != plan.EpicTitle {
		t.Errorf("EpicTitle = %q, want %q", decoded.EpicTitle, plan.EpicTitle)
	}
	if len(decoded.Tasks) != 2 {
		t.Fatalf("len(Tasks) = %d, want 2", len(decoded.Tasks))
	}
	if decoded.Tasks[1].DependsOn[0] != "t1" {
		t.Errorf("Tasks[1].DependsOn[0] = %q, want %q", decoded.Tasks[1].DependsOn[0], "t1")
	}
	if decoded.Strategy != StrategyPolecats {
		t.Errorf("Strategy = %q, want %q", decoded.Strategy, StrategyPolecats)
	}
	if decoded.MaxConcurrency != 2 {
		t.Errorf("MaxConcurrency = %d, want 2", decoded.MaxConcurrency)
	}
}

func TestGovernorDecisionJSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	d := GovernorDecision{
		Action: GovernorPause,
		Reason: "memory below floor",
		Snapshot: ResourceSnapshot{
			Timestamp:            now,
			MemoryAvailableBytes: 512 * 1024 * 1024, // 512 MB
			CPUUsagePercent:      45.0,
		},
		CurrentPolecats: 3,
		Timestamp:       now,
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded GovernorDecision
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Action != GovernorPause {
		t.Errorf("Action = %q, want %q", decoded.Action, GovernorPause)
	}
	if decoded.CurrentPolecats != 3 {
		t.Errorf("CurrentPolecats = %d, want 3", decoded.CurrentPolecats)
	}
}

func TestTaskNodeOmitsEmptyFields(t *testing.T) {
	node := TaskNode{
		ID:    "t1",
		Title: "simple task",
		Type:  "task",
	}

	data, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Verify omitted fields don't appear in JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	for _, field := range []string{"depends_on", "priority", "estimated_minutes"} {
		if _, ok := raw[field]; ok {
			t.Errorf("field %q should be omitted when zero/empty", field)
		}
	}
}
