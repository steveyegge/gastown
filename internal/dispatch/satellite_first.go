package dispatch

import "fmt"

// SatelliteFirstPolicy picks the least-loaded satellite with capacity,
// falling back to local if all satellites are full.
type SatelliteFirstPolicy struct{}

func (SatelliteFirstPolicy) Route(ctx RoutingContext) (RoutingResult, error) {
	if len(ctx.Machines) == 0 {
		return RoutingResult{Machine: ""}, nil // no satellites → local
	}

	best := leastLoadedWithCapacity(ctx.Machines)
	if best != nil {
		return RoutingResult{Machine: best.Name}, nil
	}

	// All satellites full — fall back to local
	if ctx.LocalLoad != nil && ctx.LocalLoad.HasCapacity() {
		return RoutingResult{Machine: ""}, nil
	}
	if ctx.LocalLoad == nil {
		return RoutingResult{Machine: ""}, nil // unknown local capacity → assume available
	}

	return RoutingResult{}, fmt.Errorf("all machines at capacity (satellites: %d, local: %d/%d)",
		len(ctx.Machines), ctx.LocalLoad.ActivePolecats, ctx.LocalLoad.MaxPolecats)
}
