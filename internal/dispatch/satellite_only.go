package dispatch

import "fmt"

// SatelliteOnlyPolicy routes to the least-loaded satellite with capacity.
// Unlike SatelliteFirstPolicy, it errors instead of falling back to local.
type SatelliteOnlyPolicy struct{}

func (SatelliteOnlyPolicy) Route(ctx RoutingContext) (RoutingResult, error) {
	if len(ctx.Machines) == 0 {
		return RoutingResult{}, fmt.Errorf("satellite-only policy: no satellites configured")
	}

	best := leastLoadedWithCapacity(ctx.Machines)
	if best != nil {
		return RoutingResult{Machine: best.Name}, nil
	}

	return RoutingResult{}, fmt.Errorf("satellite-only policy: all %d satellites at capacity", len(ctx.Machines))
}
