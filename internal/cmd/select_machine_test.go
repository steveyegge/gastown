package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/dispatch"
)

func testMachinesConfig() *config.MachinesConfig {
	return &config.MachinesConfig{
		Type:    "machines",
		Version: 1,
		Machines: map[string]*config.MachineEntry{
			"mini2": {
				Host:    "100.111.197.110",
				User:    "trilliumsmith",
				Roles:   []string{"worker"},
				Enabled: true,
			},
			"mini3": {
				Host:    "100.111.197.111",
				User:    "trilliumsmith",
				Roles:   []string{"worker"},
				Enabled: true,
			},
			"disabled": {
				Host:    "100.111.197.112",
				Roles:   []string{"worker"},
				Enabled: false,
			},
			"controller": {
				Host:    "100.111.197.113",
				Roles:   []string{"controller"},
				Enabled: true,
			},
		},
		DispatchPolicy: "round-robin",
		DoltHost:       "100.111.197.110",
		DoltPort:       3307,
	}
}

func TestSelectMachine_ExplicitName(t *testing.T) {
	cfg := testMachinesConfig()
	name, m, err := selectMachine(cfg, "mini2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "mini2" {
		t.Errorf("expected mini2, got %s", name)
	}
	if m.Host != "100.111.197.110" {
		t.Errorf("expected host 100.111.197.110, got %s", m.Host)
	}
}

func TestSelectMachine_NotFound(t *testing.T) {
	cfg := testMachinesConfig()
	_, _, err := selectMachine(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent machine")
	}
}

func TestSelectMachine_Disabled(t *testing.T) {
	cfg := testMachinesConfig()
	_, _, err := selectMachine(cfg, "disabled")
	if err == nil {
		t.Fatal("expected error for disabled machine")
	}
}

func TestSelectMachine_NotWorker(t *testing.T) {
	cfg := testMachinesConfig()
	_, _, err := selectMachine(cfg, "controller")
	if err == nil {
		t.Fatal("expected error for non-worker machine")
	}
}

func TestSelectMachine_RoundRobin(t *testing.T) {
	cfg := testMachinesConfig()
	// Reset counter for deterministic test
	atomic.StoreUint64(&machineSelectCounter, 0)

	counts := map[string]int{}
	for i := 0; i < 10; i++ {
		name, _, err := selectMachine(cfg, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[name]++
	}

	// With 2 workers (mini2, mini3), should be roughly even
	if counts["mini2"] != 5 || counts["mini3"] != 5 {
		t.Errorf("expected even distribution, got %v", counts)
	}
}

func TestSelectMachine_NoWorkers(t *testing.T) {
	cfg := &config.MachinesConfig{
		Machines: map[string]*config.MachineEntry{
			"controller": {
				Host:    "100.111.197.113",
				Roles:   []string{"controller"},
				Enabled: true,
			},
		},
	}
	_, _, err := selectMachine(cfg, "")
	if err == nil {
		t.Fatal("expected error when no workers available")
	}
}

func TestSelectMachine_ConcurrentSafety(t *testing.T) {
	cfg := testMachinesConfig()
	atomic.StoreUint64(&machineSelectCounter, 0)

	var wg sync.WaitGroup
	results := make([]string, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name, _, err := selectMachine(cfg, "")
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			results[idx] = name
		}(i)
	}
	wg.Wait()

	// All results should be valid machine names
	for i, name := range results {
		if name != "mini2" && name != "mini3" {
			t.Errorf("result[%d] = %q, expected mini2 or mini3", i, name)
		}
	}
}

func TestSelectMachineRoundRobin_Batch(t *testing.T) {
	cfg := testMachinesConfig()
	beadIDs := []string{"a", "b", "c", "d", "e"}
	assignments, err := selectMachineRoundRobin(cfg, beadIDs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 machines (mini2, mini3)
	if len(assignments) != 2 {
		t.Fatalf("expected 2 machines, got %d", len(assignments))
	}

	total := 0
	for _, beads := range assignments {
		total += len(beads)
	}
	if total != 5 {
		t.Errorf("expected 5 total beads, got %d", total)
	}
}

// writeMachinesJSON writes a machines.json to dir/mayor/machines.json and returns the town root.
func writeMachinesJSON(t *testing.T, cfg *config.MachinesConfig) string {
	t.Helper()
	dir := t.TempDir()
	mayorDir := filepath.Join(dir, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "machines.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveDispatchMachine_ExplicitOverride(t *testing.T) {
	cfg := testMachinesConfig()
	townRoot := writeMachinesJSON(t, cfg)

	name, entry, err := resolveDispatchMachine(townRoot, "gastown", "mini2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "mini2" {
		t.Errorf("expected mini2, got %s", name)
	}
	if entry == nil || entry.Host != "100.111.197.110" {
		t.Errorf("expected mini2 entry, got %v", entry)
	}
}

func TestResolveDispatchMachine_ExplicitNotFound(t *testing.T) {
	cfg := testMachinesConfig()
	townRoot := writeMachinesJSON(t, cfg)

	_, _, err := resolveDispatchMachine(townRoot, "gastown", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent machine")
	}
}

func TestResolveDispatchMachine_NoMachinesJSON(t *testing.T) {
	dir := t.TempDir()

	name, entry, err := resolveDispatchMachine(dir, "gastown", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" || entry != nil {
		t.Errorf("expected local dispatch (no machines.json), got machine=%q", name)
	}
}

func TestResolveDispatchMachine_ExplicitRequiresMachinesJSON(t *testing.T) {
	dir := t.TempDir()

	_, _, err := resolveDispatchMachine(dir, "gastown", "mini2")
	if err == nil {
		t.Fatal("expected error when --machine set but no machines.json")
	}
}

func TestResolveDispatchMachine_LocalOnlyPolicy(t *testing.T) {
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "local-only"
	townRoot := writeMachinesJSON(t, cfg)

	name, entry, err := resolveDispatchMachine(townRoot, "gastown", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" || entry != nil {
		t.Errorf("expected local dispatch with local-only policy, got machine=%q", name)
	}
}

func TestResolveDispatchMachine_InvalidPolicy(t *testing.T) {
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "banana"
	townRoot := writeMachinesJSON(t, cfg)

	// LoadMachinesConfig validates the policy, so this should fail at load time
	_, _, err := resolveDispatchMachine(townRoot, "gastown", "")
	if err == nil {
		t.Fatal("expected error for invalid policy")
	}
}

func TestResolveDispatchMachine_ExplicitOverridesPolicy(t *testing.T) {
	// Even when policy is satellite-only, explicit --machine still wins
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "satellite-only"
	townRoot := writeMachinesJSON(t, cfg)

	name, entry, err := resolveDispatchMachine(townRoot, "gastown", "mini3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "mini3" {
		t.Errorf("expected explicit mini3, got %s", name)
	}
	if entry == nil || entry.Host != "100.111.197.111" {
		t.Errorf("expected mini3 entry, got %v", entry)
	}
}

func TestResolveDispatchMachine_DefaultPolicySatelliteFirst(t *testing.T) {
	// Empty DispatchPolicy should default to satellite-first.
	// Stub buildRoutingContextFn to avoid SSH/tmux.
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "" // empty → defaults to satellite-first
	for _, m := range cfg.Machines {
		m.MaxPolecats = 4
	}
	townRoot := writeMachinesJSON(t, cfg)

	origBuildCtx := buildRoutingContextFn
	defer func() { buildRoutingContextFn = origBuildCtx }()

	buildRoutingContextFn = func(_ *config.MachinesConfig) dispatch.RoutingContext {
		return dispatch.RoutingContext{
			Machines: []dispatch.MachineLoad{
				{Name: "mini2", MaxPolecats: 4, ActivePolecats: 1},
				{Name: "mini3", MaxPolecats: 4, ActivePolecats: 3},
			},
			LocalLoad: &dispatch.MachineLoad{Name: "local", MaxPolecats: 0, ActivePolecats: 2},
		}
	}

	name, entry, err := resolveDispatchMachine(townRoot, "gastown", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// satellite-first should pick mini2 (least loaded satellite)
	if name != "mini2" {
		t.Errorf("expected mini2 (least loaded satellite), got %q", name)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
}

func TestResolveDispatchMachine_PolicyReturnsLocal(t *testing.T) {
	// local-first policy with local capacity → should return local
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "local-first"
	townRoot := writeMachinesJSON(t, cfg)

	origBuildCtx := buildRoutingContextFn
	defer func() { buildRoutingContextFn = origBuildCtx }()

	buildRoutingContextFn = func(_ *config.MachinesConfig) dispatch.RoutingContext {
		return dispatch.RoutingContext{
			Machines: []dispatch.MachineLoad{
				{Name: "mini2", MaxPolecats: 4, ActivePolecats: 0},
			},
			LocalLoad: &dispatch.MachineLoad{Name: "local", MaxPolecats: 0, ActivePolecats: 1},
		}
	}

	name, entry, err := resolveDispatchMachine(townRoot, "gastown", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" || entry != nil {
		t.Errorf("expected local dispatch, got machine=%q", name)
	}
}

func TestResolveDispatchMachine_PolicyError(t *testing.T) {
	// satellite-only with all satellites full → should propagate error
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "satellite-only"
	townRoot := writeMachinesJSON(t, cfg)

	origBuildCtx := buildRoutingContextFn
	defer func() { buildRoutingContextFn = origBuildCtx }()

	buildRoutingContextFn = func(_ *config.MachinesConfig) dispatch.RoutingContext {
		return dispatch.RoutingContext{
			Machines: []dispatch.MachineLoad{
				{Name: "mini2", MaxPolecats: 4, ActivePolecats: 4},
				{Name: "mini3", MaxPolecats: 4, ActivePolecats: 4},
			},
		}
	}

	_, _, err := resolveDispatchMachine(townRoot, "gastown", "")
	if err == nil {
		t.Fatal("expected error when all satellites full with satellite-only policy")
	}
}

func TestLoadMachinesConfig_ValidPoliciesAccepted(t *testing.T) {
	validPolicies := []string{"satellite-first", "local-first", "round-robin", "satellite-only", "local-only"}
	for _, policy := range validPolicies {
		cfg := testMachinesConfig()
		cfg.DispatchPolicy = policy
		townRoot := writeMachinesJSON(t, cfg)

		machines, err := loadMachinesConfig(townRoot)
		if err != nil {
			t.Errorf("policy %q rejected: %v", policy, err)
			continue
		}
		if machines == nil {
			t.Errorf("policy %q: got nil config", policy)
		}
	}
}

func TestLoadMachinesConfig_InvalidPolicyRejected(t *testing.T) {
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "banana"
	townRoot := writeMachinesJSON(t, cfg)

	_, err := loadMachinesConfig(townRoot)
	if err == nil {
		t.Fatal("expected error for invalid policy")
	}
}

func TestMachinesConfig_ProxyURL(t *testing.T) {
	cfg := testMachinesConfig()

	// Hub machine should get loopback
	url := cfg.ProxyURL("100.111.197.110")
	if url != "https://127.0.0.1:9876" {
		t.Errorf("expected loopback URL for hub, got %s", url)
	}

	// Satellite should get hub IP
	url = cfg.ProxyURL("100.111.197.111")
	if url != "https://100.111.197.110:9876" {
		t.Errorf("expected hub IP URL for satellite, got %s", url)
	}
}
