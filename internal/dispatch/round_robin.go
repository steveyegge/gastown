package dispatch

import (
	"fmt"
	"sync/atomic"
)

// roundRobinCounter provides atomic round-robin across all machines.
var roundRobinCounter uint64

// RoundRobinPolicy distributes work across all machines (including local)
// in round-robin order, skipping machines that are at capacity.
type RoundRobinPolicy struct{}

func (RoundRobinPolicy) Route(ctx RoutingContext) (RoutingResult, error) {
	// Build candidate list: satellites first, local last (operator's
	// laptop is the overflow bucket, not a primary compute target).
	type candidate struct {
		name string // "" = local
		load MachineLoad
	}
	var candidates []candidate

	for _, m := range ctx.Machines {
		candidates = append(candidates, candidate{name: m.Name, load: m})
	}
	if ctx.LocalLoad != nil {
		candidates = append(candidates, candidate{name: "", load: *ctx.LocalLoad})
	}

	if len(candidates) == 0 {
		return RoutingResult{Machine: ""}, nil // nothing configured → local
	}

	// Try each candidate starting from the counter position
	idx := atomic.AddUint64(&roundRobinCounter, 1) - 1
	for i := 0; i < len(candidates); i++ {
		c := candidates[(int(idx)+i)%len(candidates)]
		if c.load.HasCapacity() {
			return RoutingResult{Machine: c.name}, nil
		}
	}

	return RoutingResult{}, fmt.Errorf("all %d machines at capacity", len(candidates))
}

// ResetRoundRobinCounter resets the counter (for tests).
func ResetRoundRobinCounter() {
	atomic.StoreUint64(&roundRobinCounter, 0)
}
