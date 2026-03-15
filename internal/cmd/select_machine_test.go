package cmd

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
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
