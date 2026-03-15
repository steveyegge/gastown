package dispatch

import "fmt"

// LocalFirstPolicy prefers local dispatch, overflowing to the least-loaded
// satellite when local capacity is exhausted.
type LocalFirstPolicy struct{}

func (LocalFirstPolicy) Route(ctx RoutingContext) (RoutingResult, error) {
	// Prefer local if it has capacity (or capacity is unknown)
	if ctx.LocalLoad == nil || ctx.LocalLoad.HasCapacity() {
		return RoutingResult{Machine: ""}, nil
	}

	// Local full — overflow to satellite
	if len(ctx.Machines) > 0 {
		best := leastLoadedWithCapacity(ctx.Machines)
		if best != nil {
			return RoutingResult{Machine: best.Name}, nil
		}
	}

	return RoutingResult{}, fmt.Errorf("all machines at capacity (local: %d/%d, satellites: %d)",
		ctx.LocalLoad.ActivePolecats, ctx.LocalLoad.MaxPolecats, len(ctx.Machines))
}
