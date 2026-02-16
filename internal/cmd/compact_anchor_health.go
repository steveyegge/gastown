package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/governance"
)

var (
	assertAnchorHealthFn = assertAnchorHealth
	recordPromotionPtrFn = recordPromotionPointer
)

func assertAnchorHealth(townRoot, promotionPointer, lane string) (*governance.AssertResult, error) {
	controller := governance.NewController(townRoot, governance.ThresholdsFromEnv())
	start := time.Now()
	result, err := controller.AssertAnchorHealth(governance.AssertInput{
		Lane:             normalizeAnchorLane(lane),
		PromotionPointer: strings.TrimSpace(promotionPointer),
	})
	elapsed := time.Since(start).Milliseconds()
	if result != nil {
		result.LatencyMs = elapsed
	}

	budget := anchorHealthLatencyBudgetMs()
	if elapsed > budget {
		if result == nil {
			result = &governance.AssertResult{
				Status: governance.AnchorGateStatusFrozenAnchor,
				Mode:   governance.SystemModeAnchorFreeze,
			}
		}
		result.Status = governance.AnchorGateStatusFrozenAnchor
		result.Mode = governance.SystemModeAnchorFreeze
		result.LatencyMs = elapsed
		result.Reason = fmt.Sprintf("anchor health gate latency %dms exceeded budget %dms", elapsed, budget)
		return result, fmt.Errorf("anchor health gate latency budget exceeded")
	}

	return result, err
}

func recordPromotionPointer(townRoot, promotionPointer string) error {
	controller := governance.NewController(townRoot, governance.ThresholdsFromEnv())
	return controller.RecordPromotionPointer(strings.TrimSpace(promotionPointer))
}

func normalizeAnchorLane(lane string) string {
	lane = strings.TrimSpace(lane)
	if lane == "" {
		return "wisp_compaction"
	}
	return lane
}

func anchorHealthLatencyBudgetMs() int64 {
	const defaultBudgetMs int64 = 250
	raw := strings.TrimSpace(os.Getenv("GT_ANCHOR_HEALTH_MAX_LATENCY_MS"))
	if raw == "" {
		return defaultBudgetMs
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return defaultBudgetMs
	}
	return v
}
