package conductor

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/artisan"
)

// RouteResult maps a sub-bead to the artisan that should handle it.
type RouteResult struct {
	SubBead     SubBead
	ArtisanName string
	ArtisanRig  string
}

// Router selects artisans for sub-beads based on specialty matching.
type Router struct {
	manager *artisan.Manager
}

// NewRouter creates a router for the given rig.
func NewRouter(manager *artisan.Manager) *Router {
	return &Router{manager: manager}
}

// Route assigns each sub-bead to an available artisan by specialty.
// Returns an error if any sub-bead cannot be routed (missing specialty).
func (r *Router) Route(subBeads []SubBead) ([]RouteResult, error) {
	workers, err := r.manager.List()
	if err != nil {
		return nil, fmt.Errorf("listing artisans: %w", err)
	}

	// Index artisans by specialty
	bySpecialty := make(map[string][]*artisan.Worker)
	for _, w := range workers {
		bySpecialty[w.Specialty] = append(bySpecialty[w.Specialty], w)
	}

	// Track assignment counts for load balancing
	assignmentCount := make(map[string]int)

	var results []RouteResult
	var unroutable []string

	for _, sb := range subBeads {
		candidates := bySpecialty[sb.Specialty]
		if len(candidates) == 0 {
			unroutable = append(unroutable, fmt.Sprintf("%s (needs %s artisan)", sb.Title, sb.Specialty))
			continue
		}

		// Pick the least-loaded artisan of this specialty
		best := pickLeastLoaded(candidates, assignmentCount)
		assignmentCount[best.Name]++

		results = append(results, RouteResult{
			SubBead:     sb,
			ArtisanName: best.Name,
			ArtisanRig:  best.Rig,
		})
	}

	if len(unroutable) > 0 {
		return results, &UnroutableError{Items: unroutable}
	}

	return results, nil
}

// pickLeastLoaded selects the artisan with the fewest assignments in this plan.
func pickLeastLoaded(candidates []*artisan.Worker, counts map[string]int) *artisan.Worker {
	best := candidates[0]
	bestCount := counts[best.Name]

	for _, c := range candidates[1:] {
		if counts[c.Name] < bestCount {
			best = c
			bestCount = counts[c.Name]
		}
	}

	return best
}

// UnroutableError is returned when sub-beads cannot be matched to artisans.
type UnroutableError struct {
	Items []string
}

func (e *UnroutableError) Error() string {
	return fmt.Sprintf("cannot route %d sub-bead(s): %v", len(e.Items), e.Items)
}
