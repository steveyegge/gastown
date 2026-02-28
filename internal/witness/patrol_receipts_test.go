package witness

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestBuildPatrolReceipt_StaleVerdictFromHookBead(t *testing.T) {
	receipt := BuildPatrolReceipt("gastown", ZombieResult{
		PolecatName: "atlas",
		AgentState:  "idle",
		HookBead:    "gt-abc123",
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
	receipt := BuildPatrolReceipt("gastown", ZombieResult{
		PolecatName: "nux",
		AgentState:  "running",
		Error:       errors.New("nuke failed"),
	})

	if receipt.Evidence.Error != "nuke failed" {
		t.Fatalf("Evidence.Error = %q, want %q", receipt.Evidence.Error, "nuke failed")
	}
}

func TestReceiptVerdictForZombie_AllStates(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		hookBead string
		want     PatrolVerdict
	}{
		// States from getAgentBeadState (active work indicators)
		{name: "working without hook", state: "working", want: PatrolVerdictStale},
		{name: "working with hook", state: "working", hookBead: "gt-1", want: PatrolVerdictStale},
		{name: "running without hook", state: "running", want: PatrolVerdictStale},
		{name: "running with hook", state: "running", hookBead: "gt-1", want: PatrolVerdictStale},
		{name: "spawning without hook", state: "spawning", want: PatrolVerdictStale},
		{name: "spawning with hook", state: "spawning", hookBead: "gt-1", want: PatrolVerdictStale},

		// Synthetic states from DetectZombiePolecats early-return paths
		{name: "stuck-in-done without hook", state: "stuck-in-done", want: PatrolVerdictStale},
		{name: "stuck-in-done with hook", state: "stuck-in-done", hookBead: "gt-1", want: PatrolVerdictStale},
		{name: "agent-dead-in-session without hook", state: "agent-dead-in-session", want: PatrolVerdictStale},
		{name: "agent-dead-in-session with hook", state: "agent-dead-in-session", hookBead: "gt-1", want: PatrolVerdictStale},
		{name: "bead-closed-still-running without hook", state: "bead-closed-still-running", want: PatrolVerdictStale},
		{name: "bead-closed-still-running with hook", state: "bead-closed-still-running", hookBead: "gt-1", want: PatrolVerdictStale},
		{name: "done-intent-dead without hook", state: "done-intent-dead", want: PatrolVerdictStale},
		{name: "done-intent-dead with hook", state: "done-intent-dead", hookBead: "gt-1", want: PatrolVerdictStale},

		// Non-active states → orphan (no hook bead)
		{name: "idle without hook", state: "idle", want: PatrolVerdictOrphan},
		{name: "empty state without hook", state: "", want: PatrolVerdictOrphan},
		{name: "unknown state without hook", state: "something-new", want: PatrolVerdictOrphan},

		// Non-active states with hook bead → stale (hook bead overrides)
		{name: "idle with hook", state: "idle", hookBead: "gt-1", want: PatrolVerdictStale},
		{name: "empty state with hook", state: "", hookBead: "gt-1", want: PatrolVerdictStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := receiptVerdictForZombie(ZombieResult{
				AgentState: tt.state,
				HookBead:   tt.hookBead,
			})
			if got != tt.want {
				t.Errorf("receiptVerdictForZombie(state=%q, hookBead=%q) = %q, want %q",
					tt.state, tt.hookBead, got, tt.want)
			}
		})
	}
}

func TestBuildPatrolReceipts_NilAndEmpty(t *testing.T) {
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
	receipts := BuildPatrolReceipts("gastown", &DetectZombiePolecatsResult{
		Zombies: []ZombieResult{
			{
				PolecatName: "atlas",
				AgentState:  "working",
				HookBead:    "gt-123",
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
	receipts := BuildPatrolReceipts("gastown", &DetectZombiePolecatsResult{
		Zombies: []ZombieResult{
			{
				PolecatName: "atlas",
				AgentState:  "working",
				HookBead:    "gt-123",
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
