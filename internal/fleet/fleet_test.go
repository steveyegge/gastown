package fleet

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

func newTestFleet(machines map[string]*config.MachineEntry) *config.FleetConfig {
	return &config.FleetConfig{
		Machines:       machines,
		DispatchPolicy: "round-robin",
	}
}

func workerMachine(host string) *config.MachineEntry {
	return &config.MachineEntry{
		Host:    host,
		Roles:   []string{"worker"},
		Enabled: true,
	}
}

// mockSSH replaces runSSH for testing. Returns cleanup func.
func mockSSH(fn SSHExecutor) func() {
	orig := runSSH
	runSSH = fn
	return func() { runSSH = orig }
}

// --- SelectMachine tests ---

func TestSelectMachine_ExplicitName(t *testing.T) {
	fc := newTestFleet(map[string]*config.MachineEntry{
		"alpha": workerMachine("10.0.0.1"),
		"beta":  workerMachine("10.0.0.2"),
	})

	name, m, err := SelectMachine(fc, "beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "beta" {
		t.Errorf("expected beta, got %s", name)
	}
	if m.Host != "10.0.0.2" {
		t.Errorf("expected host 10.0.0.2, got %s", m.Host)
	}
}

func TestSelectMachine_NotFound(t *testing.T) {
	fc := newTestFleet(map[string]*config.MachineEntry{
		"alpha": workerMachine("10.0.0.1"),
	})

	_, _, err := SelectMachine(fc, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent machine")
	}
}

func TestSelectMachine_Disabled(t *testing.T) {
	m := workerMachine("10.0.0.1")
	m.Enabled = false
	fc := newTestFleet(map[string]*config.MachineEntry{"alpha": m})

	_, _, err := SelectMachine(fc, "alpha")
	if err == nil {
		t.Fatal("expected error for disabled machine")
	}
}

func TestSelectMachine_NotWorker(t *testing.T) {
	fc := newTestFleet(map[string]*config.MachineEntry{
		"primary": {Host: "10.0.0.1", Roles: []string{"command"}, Enabled: true},
	})

	_, _, err := SelectMachine(fc, "primary")
	if err == nil {
		t.Fatal("expected error for non-worker machine")
	}
}

func TestSelectMachine_RoundRobin(t *testing.T) {
	fc := newTestFleet(map[string]*config.MachineEntry{
		"alpha": workerMachine("10.0.0.1"),
		"beta":  workerMachine("10.0.0.2"),
		"gamma": workerMachine("10.0.0.3"),
	})

	// Reset counter for deterministic test
	selectCounter = 0

	seen := make(map[string]int)
	for i := 0; i < 6; i++ {
		name, _, err := SelectMachine(fc, "")
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		seen[name]++
	}

	// Each of 3 machines should be selected exactly twice in 6 iterations
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if seen[name] != 2 {
			t.Errorf("machine %s selected %d times, expected 2", name, seen[name])
		}
	}
}

func TestSelectMachine_NoWorkers(t *testing.T) {
	fc := newTestFleet(map[string]*config.MachineEntry{
		"primary": {Host: "10.0.0.1", Roles: []string{"command"}, Enabled: true},
	})

	_, _, err := SelectMachine(fc, "")
	if err == nil {
		t.Fatal("expected error with no worker machines")
	}
}

// --- SelectMachineRoundRobin tests ---

func TestSelectMachineRoundRobin_Distribution(t *testing.T) {
	fc := newTestFleet(map[string]*config.MachineEntry{
		"alpha": workerMachine("10.0.0.1"),
		"beta":  workerMachine("10.0.0.2"),
	})

	beads := []string{"bead-1", "bead-2", "bead-3", "bead-4"}
	assignments, err := SelectMachineRoundRobin(fc, beads)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each machine should get 2 beads
	if len(assignments["alpha"]) != 2 {
		t.Errorf("alpha got %d beads, expected 2", len(assignments["alpha"]))
	}
	if len(assignments["beta"]) != 2 {
		t.Errorf("beta got %d beads, expected 2", len(assignments["beta"]))
	}
}

func TestSelectMachineRoundRobin_NoWorkers(t *testing.T) {
	fc := newTestFleet(map[string]*config.MachineEntry{})
	_, err := SelectMachineRoundRobin(fc, []string{"bead-1"})
	if err == nil {
		t.Fatal("expected error with no workers")
	}
}

// --- SpawnRemote tests (mock SSH) ---

func TestSpawnRemote_Success(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		result := SpawnResult{
			RigName:     "gastown",
			PolecatName: "nux",
			SessionName: "gt-nux",
			ClonePath:   "/home/user/gt/gastown/polecats/nux/gastown",
			BaseBranch:  "main",
			Branch:      "polecat/nux/test-abc",
		}
		out, _ := json.Marshal(result)
		return &SSHResult{Stdout: string(out)}, nil
	})
	defer cleanup()

	fc := &config.FleetConfig{
		Machines: map[string]*config.MachineEntry{
			"mini": {Host: "10.0.0.2", Roles: []string{"worker"}, Enabled: true},
		},
		DoltHost: "10.0.0.1",
		DoltPort: 3307,
	}

	result, err := SpawnRemote(fc, "mini", fc.Machines["mini"], "gastown", "gt-abc", SpawnRemoteOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RigName != "gastown" {
		t.Errorf("expected rig gastown, got %s", result.RigName)
	}
	if result.PolecatName != "nux" {
		t.Errorf("expected polecat nux, got %s", result.PolecatName)
	}
	if result.Machine != "mini" {
		t.Errorf("expected machine mini, got %s", result.Machine)
	}
}

func TestSpawnRemote_SSHFailure(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return nil, fmt.Errorf("SSH command failed (exit 255): Connection refused")
	})
	defer cleanup()

	fc := &config.FleetConfig{
		Machines: map[string]*config.MachineEntry{
			"dead": {Host: "10.0.0.99", Roles: []string{"worker"}, Enabled: true},
		},
	}

	_, err := SpawnRemote(fc, "dead", fc.Machines["dead"], "gastown", "gt-abc", SpawnRemoteOptions{})
	if err == nil {
		t.Fatal("expected error for SSH failure")
	}
	if !strings.Contains(err.Error(), "Connection refused") {
		t.Errorf("expected Connection refused in error, got: %v", err)
	}
}

func TestSpawnRemote_BadJSON(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return &SSHResult{Stdout: "not json", Stderr: "some warning"}, nil
	})
	defer cleanup()

	fc := &config.FleetConfig{
		Machines: map[string]*config.MachineEntry{
			"mini": {Host: "10.0.0.2", Roles: []string{"worker"}, Enabled: true},
		},
	}

	_, err := SpawnRemote(fc, "mini", fc.Machines["mini"], "gastown", "gt-abc", SpawnRemoteOptions{})
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
	if !strings.Contains(err.Error(), "parsing spawn result") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestSpawnRemote_ShellEscaping(t *testing.T) {
	var capturedCmd string
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		capturedCmd = command
		return nil, fmt.Errorf("mock: not a real connection")
	})
	defer cleanup()

	fc := &config.FleetConfig{
		Machines: map[string]*config.MachineEntry{
			"test": {
				Host:     "10.0.0.1",
				TownRoot: "/tmp/test dir; rm -rf /",
				Roles:    []string{"worker"},
				Enabled:  true,
			},
		},
		DoltHost: "10.0.0.1; evil",
	}

	SpawnRemote(fc, "test", fc.Machines["test"],
		"rig; inject", "bead$(whoami)", SpawnRemoteOptions{
			Account: "acc; evil",
			Agent:   "agent$(id)",
		})

	// shellescape.Quote wraps values in single quotes.
	// Verify dangerous values appear only inside single quotes, not bare.
	if strings.Contains(capturedCmd, "'/tmp/test dir; rm -rf /'") {
		// Good: properly quoted
	} else {
		t.Errorf("town root not properly quoted in command: %s", capturedCmd)
	}
	if strings.Contains(capturedCmd, "'10.0.0.1; evil'") {
		// Good: properly quoted
	} else {
		t.Errorf("dolt host not properly quoted in command: %s", capturedCmd)
	}
	if strings.Contains(capturedCmd, "'bead$(whoami)'") {
		// Good: properly quoted
	} else {
		t.Errorf("bead ID not properly quoted in command: %s", capturedCmd)
	}
	if strings.Contains(capturedCmd, "'agent$(id)'") {
		// Good: properly quoted
	} else {
		t.Errorf("agent not properly quoted in command: %s", capturedCmd)
	}
}

func TestSpawnRemote_Options(t *testing.T) {
	var capturedCmd string
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		capturedCmd = command
		return nil, fmt.Errorf("mock")
	})
	defer cleanup()

	fc := &config.FleetConfig{
		Machines: map[string]*config.MachineEntry{
			"mini": {Host: "10.0.0.2", Roles: []string{"worker"}, Enabled: true},
		},
		DoltHost: "10.0.0.1",
		DoltPort: 3307,
	}

	SpawnRemote(fc, "mini", fc.Machines["mini"], "myrig", "bead-1", SpawnRemoteOptions{
		Account:    "trillium",
		Agent:      "polecat",
		BaseBranch: "develop",
		Force:      true,
	})

	for _, want := range []string{"--dolt-host", "--dolt-port", "3307", "--account", "--agent", "--base-branch", "--force", "--json"} {
		if !strings.Contains(capturedCmd, want) {
			t.Errorf("expected %q in command, got: %s", want, capturedCmd)
		}
	}
}

// --- Ping tests (mock SSH) ---

func TestPing_Success(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return &SSHResult{Stdout: "ok\n"}, nil
	})
	defer cleanup()

	latency, err := Ping("10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestPing_Failure(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return nil, fmt.Errorf("Connection refused")
	})
	defer cleanup()

	_, err := Ping("10.0.0.99")
	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
}

func TestPing_BadResponse(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return &SSHResult{Stdout: "not ok"}, nil
	})
	defer cleanup()

	_, err := Ping("10.0.0.1")
	if err == nil {
		t.Fatal("expected error for bad ping response")
	}
}

// --- PingAll tests (mock SSH) ---

func TestPingAll(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		if strings.Contains(target, "10.0.0.1") {
			return &SSHResult{Stdout: "ok\n"}, nil
		}
		return nil, fmt.Errorf("Connection refused")
	})
	defer cleanup()

	fc := newTestFleet(map[string]*config.MachineEntry{
		"alive": workerMachine("10.0.0.1"),
		"dead":  workerMachine("10.0.0.2"),
	})

	results := PingAll(fc)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Results are sorted by name
	if results[0].Name != "alive" || !results[0].Reachable {
		t.Errorf("expected alive to be reachable, got %+v", results[0])
	}
	if results[1].Name != "dead" || results[1].Reachable {
		t.Errorf("expected dead to be unreachable, got %+v", results[1])
	}
}

// --- ListSessions tests (mock SSH) ---

func TestListSessions_Success(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return &SSHResult{Stdout: "gt-nux\ngt-toast\ngt-furiosa\n"}, nil
	})
	defer cleanup()

	m := workerMachine("10.0.0.1")
	sessions, err := ListSessions(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	if sessions[0] != "gt-nux" {
		t.Errorf("expected gt-nux, got %s", sessions[0])
	}
}

func TestListSessions_Empty(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return &SSHResult{Stdout: ""}, nil
	})
	defer cleanup()

	m := workerMachine("10.0.0.1")
	sessions, err := ListSessions(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sessions != nil {
		t.Errorf("expected nil for empty sessions, got %v", sessions)
	}
}

func TestListSessions_SSHError(t *testing.T) {
	cleanup := mockSSH(func(target, command string, timeout time.Duration) (*SSHResult, error) {
		return nil, fmt.Errorf("Connection refused")
	})
	defer cleanup()

	m := workerMachine("10.0.0.1")
	_, err := ListSessions(m)
	if err == nil {
		t.Fatal("expected error for SSH failure")
	}
}
