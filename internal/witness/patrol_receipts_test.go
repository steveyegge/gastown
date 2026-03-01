package witness

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestBuildPatrolReceipt_StaleVerdictFromHookBead(t *testing.T) {
	t.Parallel()
	receipt := BuildPatrolReceipt("gastown", ZombieResult{
		PolecatName: "atlas",
		AgentState:  "idle",
		HookBead:    "gt-abc123",
		WasActive:   true,
		Action:      "restarted",
	})

	if receipt.Verdict != PatrolVerdictStale {
		t.Fatalf("Verdict = %q, want %q", receipt.Verdict, PatrolVerdictStale)
	}
	if receipt.RecommendedAction != "restarted" {
		t.Fatalf("RecommendedAction = %q, want %q", receipt.RecommendedAction, "restarted")
	}
}

func TestBuildPatrolReceipt_OrphanVerdictWithoutHookedWork(t *testing.T) {
	t.Parallel()
	receipt := BuildPatrolReceipt("gastown", ZombieResult{
		PolecatName: "echo",
		AgentState:  "idle",
		Action:      "cleanup-wisp-created",
	})

	if receipt.Verdict != PatrolVerdictOrphan {
		t.Fatalf("Verdict = %q, want %q", receipt.Verdict, PatrolVerdictOrphan)
	}
}

func TestBuildPatrolReceipt_ErrorIncludedInEvidence(t *testing.T) {
	t.Parallel()
	receipt := BuildPatrolReceipt("gastown", ZombieResult{
		PolecatName: "nux",
		AgentState:  "running",
		WasActive:   true,
		Error:       errors.New("nuke failed"),
	})

	if receipt.Evidence.Error != "nuke failed" {
		t.Fatalf("Evidence.Error = %q, want %q", receipt.Evidence.Error, "nuke failed")
	}
}

func TestReceiptVerdictForZombie_AllStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		state     string
		hookBead  string
		wasActive bool
		want      PatrolVerdict
	}{
		// WasActive=true → stale (set by detection code for active/synthetic states)
		{name: "active working", state: "working", wasActive: true, want: PatrolVerdictStale},
		{name: "active with hook", state: "working", hookBead: "gt-1", wasActive: true, want: PatrolVerdictStale},
		{name: "active running", state: "running", wasActive: true, want: PatrolVerdictStale},
		{name: "active spawning", state: "spawning", wasActive: true, want: PatrolVerdictStale},
		{name: "stuck-in-done", state: "stuck-in-done", wasActive: true, want: PatrolVerdictStale},
		{name: "agent-dead-in-session", state: "agent-dead-in-session", wasActive: true, want: PatrolVerdictStale},
		{name: "bead-closed-still-running", state: "bead-closed-still-running", wasActive: true, want: PatrolVerdictStale},
		{name: "done-intent-dead", state: "done-intent-dead", wasActive: true, want: PatrolVerdictStale},
		{name: "idle with hook (active)", state: "idle", hookBead: "gt-1", wasActive: true, want: PatrolVerdictStale},

		// WasActive=false → orphan (no evidence of recent work)
		{name: "idle without hook", state: "idle", want: PatrolVerdictOrphan},
		{name: "empty state", state: "", want: PatrolVerdictOrphan},
		{name: "unknown state", state: "something-new", want: PatrolVerdictOrphan},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := receiptVerdictForZombie(ZombieResult{
				AgentState: tt.state,
				HookBead:   tt.hookBead,
				WasActive:  tt.wasActive,
			})
			if got != tt.want {
				t.Errorf("receiptVerdictForZombie(wasActive=%v, state=%q) = %q, want %q",
					tt.wasActive, tt.state, got, tt.want)
			}
		})
	}
}

func TestBuildPatrolReceipts_NilAndEmpty(t *testing.T) {
	t.Parallel()
	if got := BuildPatrolReceipts("rig", nil); got != nil {
		t.Errorf("BuildPatrolReceipts(nil) = %v, want nil", got)
	}
	if got := BuildPatrolReceipts("rig", &DetectZombiePolecatsResult{}); got != nil {
		t.Errorf("BuildPatrolReceipts(empty) = %v, want nil", got)
	}
	if got := BuildPatrolReceipts("rig", &DetectZombiePolecatsResult{Zombies: []ZombieResult{}}); got != nil {
		t.Errorf("BuildPatrolReceipts(empty zombies) = %v, want nil", got)
	}
}

func TestBuildPatrolReceipts_JSONShape(t *testing.T) {
	t.Parallel()
	receipts := BuildPatrolReceipts("gastown", &DetectZombiePolecatsResult{
		Zombies: []ZombieResult{
			{
				PolecatName: "atlas",
				AgentState:  "working",
				HookBead:    "gt-123",
				WasActive:   true,
				Action:      "restarted",
			},
		},
	})
	if len(receipts) != 1 {
		t.Fatalf("len(receipts) = %d, want 1", len(receipts))
	}

	data, err := json.Marshal(receipts[0])
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded["verdict"] != string(PatrolVerdictStale) {
		t.Fatalf("decoded verdict = %v, want %q", decoded["verdict"], PatrolVerdictStale)
	}
	if decoded["recommended_action"] != "restarted" {
		t.Fatalf("decoded recommended_action = %v, want %q", decoded["recommended_action"], "restarted")
	}
	evidence, ok := decoded["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("decoded evidence missing or wrong type: %#v", decoded["evidence"])
	}
	if evidence["hook_bead"] != "gt-123" {
		t.Fatalf("decoded evidence.hook_bead = %v, want %q", evidence["hook_bead"], "gt-123")
	}
}

func TestBuildPatrolReceipts_DeterministicStaleOrphanOrdering(t *testing.T) {
	t.Parallel()
	receipts := BuildPatrolReceipts("gastown", &DetectZombiePolecatsResult{
		Zombies: []ZombieResult{
			{
				PolecatName: "atlas",
				AgentState:  "working",
				HookBead:    "gt-123",
				WasActive:   true,
				Action:      "restarted",
			},
			{
				PolecatName: "echo",
				AgentState:  "idle",
				Action:      "cleanup-wisp-created",
			},
		},
	})
	if len(receipts) != 2 {
		t.Fatalf("len(receipts) = %d, want 2", len(receipts))
	}
	if receipts[0].Polecat != "atlas" || receipts[0].Verdict != PatrolVerdictStale {
		t.Fatalf("first receipt = %+v, want polecat=atlas verdict=%q", receipts[0], PatrolVerdictStale)
	}
	if receipts[1].Polecat != "echo" || receipts[1].Verdict != PatrolVerdictOrphan {
		t.Fatalf("second receipt = %+v, want polecat=echo verdict=%q", receipts[1], PatrolVerdictOrphan)
	}
}
