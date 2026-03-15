package dispatch

import (
	"testing"
)

func satMachines(loads ...int) []MachineLoad {
	names := []string{"mini2", "mini3", "mini4"}
	var machines []MachineLoad
	for i, active := range loads {
		machines = append(machines, MachineLoad{
			Name:           names[i%len(names)],
			MaxPolecats:    4,
			ActivePolecats: active,
		})
	}
	return machines
}

func localLoad(active, max int) *MachineLoad {
	return &MachineLoad{Name: "local", MaxPolecats: max, ActivePolecats: active}
}

// --- Resolve ---

func TestResolve_AllPolicies(t *testing.T) {
	for _, name := range ValidPolicies {
		p, err := Resolve(string(name))
		if err != nil {
			t.Errorf("Resolve(%q) error: %v", name, err)
		}
		if p == nil {
			t.Errorf("Resolve(%q) returned nil", name)
		}
	}
}

func TestResolve_Unknown(t *testing.T) {
	_, err := Resolve("banana")
	if err == nil {
		t.Fatal("expected error for unknown policy")
	}
}

func TestIsValidPolicy(t *testing.T) {
	if !IsValidPolicy("satellite-first") {
		t.Error("satellite-first should be valid")
	}
	if IsValidPolicy("nope") {
		t.Error("nope should not be valid")
	}
}

// --- SatelliteFirst ---

func TestSatelliteFirst_PicksLeastLoaded(t *testing.T) {
	p := SatelliteFirstPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(3, 1, 2), // mini2=3, mini3=1, mini4=2
		LocalLoad: localLoad(0, 4),
	}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "mini3" {
		t.Errorf("expected mini3 (least loaded), got %q", r.Machine)
	}
}

func TestSatelliteFirst_FallbackToLocal(t *testing.T) {
	p := SatelliteFirstPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(4, 4, 4), // all full
		LocalLoad: localLoad(1, 4),
	}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "" {
		t.Errorf("expected local fallback, got %q", r.Machine)
	}
}

func TestSatelliteFirst_NoSatellites(t *testing.T) {
	p := SatelliteFirstPolicy{}
	ctx := RoutingContext{LocalLoad: localLoad(0, 4)}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "" {
		t.Errorf("expected local, got %q", r.Machine)
	}
}

func TestSatelliteFirst_AllFull(t *testing.T) {
	p := SatelliteFirstPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(4, 4),
		LocalLoad: localLoad(4, 4),
	}
	_, err := p.Route(ctx)
	if err == nil {
		t.Fatal("expected error when all machines full")
	}
}

func TestSatelliteFirst_UnlimitedCapacity(t *testing.T) {
	p := SatelliteFirstPolicy{}
	ctx := RoutingContext{
		Machines: []MachineLoad{
			{Name: "mini2", MaxPolecats: 0, ActivePolecats: 100}, // unlimited
		},
	}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "mini2" {
		t.Errorf("expected mini2 (unlimited), got %q", r.Machine)
	}
}

// --- LocalFirst ---

func TestLocalFirst_PrefersLocal(t *testing.T) {
	p := LocalFirstPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(0, 0),
		LocalLoad: localLoad(1, 4),
	}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "" {
		t.Errorf("expected local, got %q", r.Machine)
	}
}

func TestLocalFirst_OverflowToSatellite(t *testing.T) {
	p := LocalFirstPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(1, 3),
		LocalLoad: localLoad(4, 4), // full
	}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "mini2" {
		t.Errorf("expected mini2 (least loaded satellite), got %q", r.Machine)
	}
}

func TestLocalFirst_NilLocalLoad(t *testing.T) {
	p := LocalFirstPolicy{}
	ctx := RoutingContext{Machines: satMachines(0)}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "" {
		t.Errorf("expected local (nil = assume available), got %q", r.Machine)
	}
}

func TestLocalFirst_AllFull(t *testing.T) {
	p := LocalFirstPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(4, 4),
		LocalLoad: localLoad(4, 4),
	}
	_, err := p.Route(ctx)
	if err == nil {
		t.Fatal("expected error when all full")
	}
}

// --- RoundRobin ---

func TestRoundRobin_DistributesEvenly(t *testing.T) {
	ResetRoundRobinCounter()
	p := RoundRobinPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(0, 0),
		LocalLoad: localLoad(0, 4),
	}

	counts := map[string]int{}
	for i := 0; i < 9; i++ {
		r, err := p.Route(ctx)
		if err != nil {
			t.Fatal(err)
		}
		counts[r.Machine]++
	}

	// 3 candidates (local, mini2, mini3), 9 calls → 3 each
	if counts[""] != 3 || counts["mini2"] != 3 || counts["mini3"] != 3 {
		t.Errorf("expected 3 each, got local=%d mini2=%d mini3=%d",
			counts[""], counts["mini2"], counts["mini3"])
	}
}

func TestRoundRobin_SkipsFull(t *testing.T) {
	ResetRoundRobinCounter()
	p := RoundRobinPolicy{}
	ctx := RoutingContext{
		Machines: []MachineLoad{
			{Name: "mini2", MaxPolecats: 4, ActivePolecats: 4}, // full
			{Name: "mini3", MaxPolecats: 4, ActivePolecats: 0},
		},
		LocalLoad: localLoad(4, 4), // full
	}

	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "mini3" {
		t.Errorf("expected mini3 (only with capacity), got %q", r.Machine)
	}
}

func TestRoundRobin_NoCandidates(t *testing.T) {
	ResetRoundRobinCounter()
	p := RoundRobinPolicy{}
	ctx := RoutingContext{}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "" {
		t.Errorf("expected local fallback, got %q", r.Machine)
	}
}

func TestRoundRobin_AllFull(t *testing.T) {
	ResetRoundRobinCounter()
	p := RoundRobinPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(4, 4),
		LocalLoad: localLoad(4, 4),
	}
	_, err := p.Route(ctx)
	if err == nil {
		t.Fatal("expected error when all full")
	}
}

// --- SatelliteOnly ---

func TestSatelliteOnly_PicksSatellite(t *testing.T) {
	p := SatelliteOnlyPolicy{}
	ctx := RoutingContext{Machines: satMachines(0, 2)}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "mini2" {
		t.Errorf("expected mini2, got %q", r.Machine)
	}
}

func TestSatelliteOnly_NoSatellites(t *testing.T) {
	p := SatelliteOnlyPolicy{}
	ctx := RoutingContext{}
	_, err := p.Route(ctx)
	if err == nil {
		t.Fatal("expected error with no satellites")
	}
}

func TestSatelliteOnly_AllFull(t *testing.T) {
	p := SatelliteOnlyPolicy{}
	ctx := RoutingContext{Machines: satMachines(4, 4)}
	_, err := p.Route(ctx)
	if err == nil {
		t.Fatal("expected error when all full")
	}
}

// --- LocalOnly ---

func TestLocalOnly_AlwaysLocal(t *testing.T) {
	p := LocalOnlyPolicy{}
	ctx := RoutingContext{Machines: satMachines(0, 0)}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "" {
		t.Errorf("expected local, got %q", r.Machine)
	}
}

// --- SatelliteFirst additional edge cases ---

func TestSatelliteFirst_NilLocalLoad_SatellitesFull(t *testing.T) {
	// When satellites are full and LocalLoad is nil, should fall back to local
	// (nil = unknown capacity = assume available).
	p := SatelliteFirstPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(4, 4), // all full
		LocalLoad: nil,
	}
	r, err := p.Route(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if r.Machine != "" {
		t.Errorf("expected local fallback (nil LocalLoad), got %q", r.Machine)
	}
}

// --- LocalFirst additional edge cases ---

func TestLocalFirst_NoSatellites_LocalFull(t *testing.T) {
	p := LocalFirstPolicy{}
	ctx := RoutingContext{
		Machines:  nil,              // no satellites
		LocalLoad: localLoad(4, 4), // full
	}
	_, err := p.Route(ctx)
	if err == nil {
		t.Fatal("expected error when local full and no satellites")
	}
}

// --- RoundRobin additional edge cases ---

func TestRoundRobin_SatellitesOnly_NilLocalLoad(t *testing.T) {
	ResetRoundRobinCounter()
	p := RoundRobinPolicy{}
	ctx := RoutingContext{
		Machines:  satMachines(0, 0),
		LocalLoad: nil, // no local
	}

	counts := map[string]int{}
	for i := 0; i < 4; i++ {
		r, err := p.Route(ctx)
		if err != nil {
			t.Fatal(err)
		}
		counts[r.Machine]++
	}

	// 2 candidates (mini2, mini3), 4 calls → 2 each
	if counts["mini2"] != 2 || counts["mini3"] != 2 {
		t.Errorf("expected 2 each, got mini2=%d mini3=%d",
			counts["mini2"], counts["mini3"])
	}
}

// --- leastLoadedWithCapacity ---

func TestLeastLoadedWithCapacity_Tiebreaker(t *testing.T) {
	// Equal loads: should pick first in slice order (deterministic)
	machines := []MachineLoad{
		{Name: "alpha", MaxPolecats: 4, ActivePolecats: 2},
		{Name: "beta", MaxPolecats: 4, ActivePolecats: 2},
	}
	best := leastLoadedWithCapacity(machines)
	if best == nil {
		t.Fatal("expected a result")
	}
	if best.Name != "alpha" {
		t.Errorf("expected alpha (first with equal load), got %q", best.Name)
	}
}

func TestLeastLoadedWithCapacity_AllFull(t *testing.T) {
	machines := []MachineLoad{
		{Name: "a", MaxPolecats: 2, ActivePolecats: 2},
		{Name: "b", MaxPolecats: 3, ActivePolecats: 5},
	}
	best := leastLoadedWithCapacity(machines)
	if best != nil {
		t.Errorf("expected nil when all full, got %q", best.Name)
	}
}

func TestLeastLoadedWithCapacity_Empty(t *testing.T) {
	best := leastLoadedWithCapacity(nil)
	if best != nil {
		t.Error("expected nil for empty slice")
	}
}

// --- MachineLoad.HasCapacity ---

func TestHasCapacity(t *testing.T) {
	tests := []struct {
		name   string
		load   MachineLoad
		expect bool
	}{
		{"unlimited", MachineLoad{MaxPolecats: 0, ActivePolecats: 99}, true},
		{"under limit", MachineLoad{MaxPolecats: 4, ActivePolecats: 2}, true},
		{"at limit", MachineLoad{MaxPolecats: 4, ActivePolecats: 4}, false},
		{"over limit", MachineLoad{MaxPolecats: 4, ActivePolecats: 5}, false},
		{"zero of zero", MachineLoad{MaxPolecats: 0, ActivePolecats: 0}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.load.HasCapacity(); got != tt.expect {
				t.Errorf("HasCapacity() = %v, want %v", got, tt.expect)
			}
		})
	}
}
