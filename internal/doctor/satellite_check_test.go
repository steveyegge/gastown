package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

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

func testMachinesConfig() *config.MachinesConfig {
	return &config.MachinesConfig{
		Type:    "machines",
		Version: 1,
		Machines: map[string]*config.MachineEntry{
			"mini2": {
				Host:        "100.111.197.110",
				User:        "b",
				MaxPolecats: 3,
				Roles:       []string{"worker"},
				Enabled:     true,
			},
			"mini3": {
				Host:        "100.86.9.58",
				User:        "2020mini_2",
				MaxPolecats: 3,
				Roles:       []string{"worker"},
				Enabled:     true,
			},
		},
		DispatchPolicy: "satellite-first",
		DoltHost:       "100.111.197.110",
		DoltPort:       3307,
	}
}

// --- MachinesConfigCheck ---

func TestMachinesConfigCheck_NoFile(t *testing.T) {
	ctx := &CheckContext{TownRoot: t.TempDir()}
	check := NewMachinesConfigCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing file, got %v: %s", result.Status, result.Message)
	}
}

func TestMachinesConfigCheck_ValidConfig(t *testing.T) {
	cfg := testMachinesConfig()
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewMachinesConfigCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK, got %v: %s", result.Status, result.Message)
	}
}

func TestMachinesConfigCheck_InvalidPolicy(t *testing.T) {
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "banana"
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewMachinesConfigCheck()
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected Error for invalid policy, got %v: %s", result.Status, result.Message)
	}
}

func TestMachinesConfigCheck_EmptyPolicyDefaultsSatelliteFirst(t *testing.T) {
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = ""
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewMachinesConfigCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK, got %v: %s", result.Status, result.Message)
	}
}

func TestMachinesConfigCheck_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	mayorDir := filepath.Join(dir, "mayor")
	os.MkdirAll(mayorDir, 0o755)
	os.WriteFile(filepath.Join(mayorDir, "machines.json"), []byte("{bad json"), 0o644)

	ctx := &CheckContext{TownRoot: dir}
	check := NewMachinesConfigCheck()
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected Error for bad JSON, got %v: %s", result.Status, result.Message)
	}
}

// --- SatelliteSSHCheck ---

func TestSatelliteSSHCheck_NoFile(t *testing.T) {
	ctx := &CheckContext{TownRoot: t.TempDir()}
	check := NewSatelliteSSHCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing file, got %v: %s", result.Status, result.Message)
	}
}

func TestSatelliteSSHCheck_NoWorkers(t *testing.T) {
	cfg := &config.MachinesConfig{
		Machines: map[string]*config.MachineEntry{
			"controller": {
				Host:    "100.111.197.113",
				Roles:   []string{"controller"},
				Enabled: true,
			},
		},
	}
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewSatelliteSSHCheck()
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected Warning for no workers, got %v: %s", result.Status, result.Message)
	}
}

// --- SatelliteProxyCheck ---

func TestSatelliteProxyCheck_NoFile(t *testing.T) {
	ctx := &CheckContext{TownRoot: t.TempDir()}
	check := NewSatelliteProxyCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing file, got %v: %s", result.Status, result.Message)
	}
}

// --- SatelliteCapacityCheck ---

func TestSatelliteCapacityCheck_NoFile(t *testing.T) {
	ctx := &CheckContext{TownRoot: t.TempDir()}
	check := NewSatelliteCapacityCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing file, got %v: %s", result.Status, result.Message)
	}
}

func TestSatelliteCapacityCheck_NoWorkers(t *testing.T) {
	cfg := &config.MachinesConfig{
		Machines: map[string]*config.MachineEntry{},
	}
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewSatelliteCapacityCheck()
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Errorf("expected Warning for no workers, got %v: %s", result.Status, result.Message)
	}
}

// --- DispatchPolicyCheck ---

func TestDispatchPolicyCheck_NoFile(t *testing.T) {
	ctx := &CheckContext{TownRoot: t.TempDir()}
	check := NewDispatchPolicyCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK for missing file, got %v: %s", result.Status, result.Message)
	}
}

func TestDispatchPolicyCheck_SatelliteFirst(t *testing.T) {
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "satellite-first"
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewDispatchPolicyCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK, got %v: %s", result.Status, result.Message)
	}
}

func TestDispatchPolicyCheck_LocalOnly(t *testing.T) {
	cfg := testMachinesConfig()
	cfg.DispatchPolicy = "local-only"
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewDispatchPolicyCheck()
	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected OK, got %v: %s", result.Status, result.Message)
	}
}

func TestDispatchPolicyCheck_SatelliteOnlyNoWorkers(t *testing.T) {
	cfg := &config.MachinesConfig{
		Machines:       map[string]*config.MachineEntry{},
		DispatchPolicy: "satellite-only",
	}
	townRoot := writeMachinesJSON(t, cfg)
	ctx := &CheckContext{TownRoot: townRoot}

	check := NewDispatchPolicyCheck()
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Errorf("expected Error for satellite-only with no workers, got %v: %s", result.Status, result.Message)
	}
}

// --- helpers ---

func TestSortedKeys(t *testing.T) {
	m := map[string]*config.MachineEntry{
		"charlie": {},
		"alpha":   {},
		"bravo":   {},
	}
	keys := sortedKeys(m)
	if len(keys) != 3 || keys[0] != "alpha" || keys[1] != "bravo" || keys[2] != "charlie" {
		t.Errorf("expected sorted keys, got %v", keys)
	}
}
