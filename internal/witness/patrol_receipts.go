package witness

import "strings"

// PatrolVerdict classifies witness patrol outcomes for machine consumers.
type PatrolVerdict string

const (
	PatrolVerdictStale  PatrolVerdict = "stale"
	PatrolVerdictOrphan PatrolVerdict = "orphan"
)

// PatrolReceiptEvidence captures the primary evidence fields for a verdict.
type PatrolReceiptEvidence struct {
	AgentState    string `json:"agent_state,omitempty"`
	HookBead      string `json:"hook_bead,omitempty"`
	BeadRecovered bool   `json:"bead_recovered"`
	Error         string `json:"error,omitempty"`
}

// PatrolReceipt is a machine-readable witness patrol verdict with recommended action.
type PatrolReceipt struct {
	Rig               string                `json:"rig"`
	Polecat           string                `json:"polecat"`
	Verdict           PatrolVerdict         `json:"verdict"`
	RecommendedAction string                `json:"recommended_action"`
	Evidence          PatrolReceiptEvidence `json:"evidence"`
}

func receiptVerdictForZombie(z ZombieResult) PatrolVerdict {
	if z.WasActive {
		return PatrolVerdictStale
	}
	return PatrolVerdictOrphan
}

// BuildPatrolReceipt projects a zombie patrol result into a stable JSON-ready receipt.
func BuildPatrolReceipt(rigName string, z ZombieResult) PatrolReceipt {
	action := strings.TrimSpace(z.Action)
	if action == "" {
		action = "investigate"
	}

	receipt := PatrolReceipt{
		Rig:               rigName,
		Polecat:           z.PolecatName,
		Verdict:           receiptVerdictForZombie(z),
		RecommendedAction: action,
		Evidence: PatrolReceiptEvidence{
			AgentState:    z.AgentState,
			HookBead:      z.HookBead,
			BeadRecovered: z.BeadRecovered,
		},
	}

	if z.Error != nil {
		receipt.Evidence.Error = z.Error.Error()
	}

	return receipt
}

// BuildPatrolReceipts returns machine-readable patrol verdicts for all detected zombies.
func BuildPatrolReceipts(rigName string, result *DetectZombiePolecatsResult) []PatrolReceipt {
	if result == nil || len(result.Zombies) == 0 {
		return nil
	}
	receipts := make([]PatrolReceipt, 0, len(result.Zombies))
	for _, zombie := range result.Zombies {
		receipts = append(receipts, BuildPatrolReceipt(rigName, zombie))
	}
	return receipts
}
