// Package dispatch provides pluggable routing policies for satellite transport.
// It is pure logic — no SSH, no tmux, no cmd dependencies.
package dispatch

import "fmt"

// PolicyName identifies a dispatch routing strategy.
type PolicyName string

const (
	PolicySatelliteFirst PolicyName = "satellite-first"
	PolicyLocalFirst     PolicyName = "local-first"
	PolicyRoundRobin     PolicyName = "round-robin"
	PolicySatelliteOnly  PolicyName = "satellite-only"
	PolicyLocalOnly      PolicyName = "local-only"
)

// ValidPolicies lists all recognized policy names.
var ValidPolicies = []PolicyName{
	PolicySatelliteFirst,
	PolicyLocalFirst,
	PolicyRoundRobin,
	PolicySatelliteOnly,
	PolicyLocalOnly,
}

// MachineLoad describes the current load on one machine.
type MachineLoad struct {
	Name           string // machine name or "local"
	MaxPolecats    int    // 0 = unlimited
	ActivePolecats int
}

// HasCapacity reports whether the machine can accept another polecat.
func (m MachineLoad) HasCapacity() bool {
	return m.MaxPolecats == 0 || m.ActivePolecats < m.MaxPolecats
}

// RoutingContext provides load data for a routing decision.
type RoutingContext struct {
	Machines  []MachineLoad // satellite workers, sorted by name
	LocalLoad *MachineLoad  // nil if local capacity unknown
}

// RoutingResult holds the outcome of a routing decision.
type RoutingResult struct {
	Machine string // machine name, or "" for local
}

// Policy makes a routing decision given current load data.
type Policy interface {
	Route(ctx RoutingContext) (RoutingResult, error)
}

// Resolve maps a policy name to its implementation.
func Resolve(name string) (Policy, error) {
	switch PolicyName(name) {
	case PolicySatelliteFirst:
		return SatelliteFirstPolicy{}, nil
	case PolicyLocalFirst:
		return LocalFirstPolicy{}, nil
	case PolicyRoundRobin:
		return RoundRobinPolicy{}, nil
	case PolicySatelliteOnly:
		return SatelliteOnlyPolicy{}, nil
	case PolicyLocalOnly:
		return LocalOnlyPolicy{}, nil
	default:
		return nil, fmt.Errorf("unknown dispatch policy %q", name)
	}
}

// IsValidPolicy reports whether the given name is a recognized policy.
func IsValidPolicy(name string) bool {
	_, err := Resolve(name)
	return err == nil
}

// leastLoadedWithCapacity returns the machine with the most remaining capacity.
// Returns nil if no machine has capacity.
func leastLoadedWithCapacity(machines []MachineLoad) *MachineLoad {
	var best *MachineLoad
	bestRemaining := -1
	for i := range machines {
		m := &machines[i]
		if !m.HasCapacity() {
			continue
		}
		remaining := m.MaxPolecats - m.ActivePolecats
		if m.MaxPolecats == 0 {
			remaining = 1<<31 - 1 // unlimited = very large
		}
		if remaining > bestRemaining {
			best = m
			bestRemaining = remaining
		}
	}
	return best
}
