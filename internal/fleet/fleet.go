package fleet

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
)

// SpawnResult holds the outcome of a remote polecat spawn.
type SpawnResult struct {
	Machine     string `json:"machine"`
	RigName     string `json:"rig_name"`
	PolecatName string `json:"polecat_name"`
	SessionName string `json:"session_name"`
	ClonePath   string `json:"clone_path"`
	BaseBranch  string `json:"base_branch"`
	Branch      string `json:"branch"`
}

// MachineStatus holds status information for a fleet machine.
type MachineStatus struct {
	Name      string        `json:"name"`
	Host      string        `json:"host"`
	Reachable bool          `json:"reachable"`
	Latency   time.Duration `json:"latency"`
	Enabled   bool          `json:"enabled"`
	Roles     []string      `json:"roles"`
	Sessions  []string      `json:"sessions,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// LoadConfig loads the fleet configuration from the standard location.
func LoadConfig(townRoot string) (*config.FleetConfig, error) {
	path := constants.MayorFleetPath(townRoot)
	return config.LoadFleetConfig(path)
}

// selectCounter tracks round-robin position across calls within a process.
var selectCounter uint64

// SelectMachine picks the next machine to dispatch to based on round-robin.
// If machineName is non-empty, it selects that specific machine.
// Returns the machine name and entry, or an error if no suitable machine is available.
func SelectMachine(fleet *config.FleetConfig, machineName string) (string, *config.MachineEntry, error) {
	if machineName != "" {
		m, ok := fleet.Machines[machineName]
		if !ok {
			return "", nil, fmt.Errorf("machine %q not found in fleet", machineName)
		}
		if !m.Enabled {
			return "", nil, fmt.Errorf("machine %q is disabled", machineName)
		}
		if !m.IsWorker() {
			return "", nil, fmt.Errorf("machine %q is not a worker (roles: %v)", machineName, m.Roles)
		}
		return machineName, m, nil
	}

	workers := fleet.WorkerMachines()
	if len(workers) == 0 {
		return "", nil, fmt.Errorf("no enabled worker machines in fleet")
	}

	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)

	// Round-robin: use modulo to distribute across workers
	idx := selectCounter % uint64(len(names))
	selectCounter++

	return names[idx], workers[names[idx]], nil
}

// SelectMachineRoundRobin picks machines round-robin for a batch of beads.
// Returns a map of machine name -> list of bead IDs to dispatch there.
func SelectMachineRoundRobin(fleet *config.FleetConfig, beadIDs []string) (map[string][]string, error) {
	workers := fleet.WorkerMachines()
	if len(workers) == 0 {
		return nil, fmt.Errorf("no enabled worker machines in fleet")
	}

	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)

	assignments := make(map[string][]string)
	for i, beadID := range beadIDs {
		machine := names[i%len(names)]
		assignments[machine] = append(assignments[machine], beadID)
	}
	return assignments, nil
}

// SpawnRemote dispatches a polecat spawn to a remote machine via SSH.
// It runs `gt fleet spawn-local` on the satellite, passing Dolt connection info.
func SpawnRemote(fleet *config.FleetConfig, machineName string, machine *config.MachineEntry, rigName string, beadID string, opts SpawnRemoteOptions) (*SpawnResult, error) {
	// Build the remote command — all interpolated values are shell-escaped
	// to prevent command injection via crafted config values.
	townRoot := machine.TownRoot
	if townRoot == "" {
		townRoot = "~/gt"
	}

	args := []string{
		"cd", shellescape.Quote(townRoot), "&&",
		shellescape.Quote(machine.GtBin()), "fleet", "spawn-local", shellescape.Quote(rigName),
		"--bead", shellescape.Quote(beadID),
		"--json",
	}

	if fleet.DoltHost != "" {
		args = append(args, "--dolt-host", shellescape.Quote(fleet.DoltHost))
	}
	if fleet.DoltPort > 0 {
		args = append(args, "--dolt-port", fmt.Sprintf("%d", fleet.DoltPort))
	}
	if opts.Account != "" {
		args = append(args, "--account", shellescape.Quote(opts.Account))
	}
	if opts.Agent != "" {
		args = append(args, "--agent", shellescape.Quote(opts.Agent))
	}
	if opts.BaseBranch != "" {
		args = append(args, "--base-branch", shellescape.Quote(opts.BaseBranch))
	}
	if opts.Force {
		args = append(args, "--force")
	}

	cmd := strings.Join(args, " ")
	result, err := RunSSH(machine.SSHTarget(), cmd, 120*time.Second)
	if err != nil {
		return nil, fmt.Errorf("remote spawn on %s: %w", machineName, err)
	}

	// Parse JSON result from satellite
	var spawnResult SpawnResult
	if err := json.Unmarshal([]byte(result.Stdout), &spawnResult); err != nil {
		return nil, fmt.Errorf("parsing spawn result from %s: %w\nstdout: %s\nstderr: %s",
			machineName, err, result.Stdout, result.Stderr)
	}
	spawnResult.Machine = machineName

	return &spawnResult, nil
}

// SpawnRemoteOptions holds options for remote polecat spawn.
type SpawnRemoteOptions struct {
	Account    string
	Agent      string
	BaseBranch string
	Force      bool
}

// PingAll checks SSH connectivity to all machines in the fleet.
func PingAll(fleet *config.FleetConfig) []MachineStatus {
	results := make([]MachineStatus, 0, len(fleet.Machines))
	for name, m := range fleet.Machines {
		status := MachineStatus{
			Name:    name,
			Host:    m.Host,
			Enabled: m.Enabled,
			Roles:   m.Roles,
		}
		latency, err := Ping(m.SSHTarget())
		status.Latency = latency
		if err != nil {
			status.Error = err.Error()
		} else {
			status.Reachable = true
		}
		results = append(results, status)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results
}

// ListSessions lists tmux sessions on a remote machine matching the gt pattern.
func ListSessions(machine *config.MachineEntry) ([]string, error) {
	result, err := RunSSH(machine.SSHTarget(), "tmux list-sessions -F '#{session_name}' 2>/dev/null | grep '^gt-' || true", 15*time.Second)
	if err != nil {
		return nil, err
	}
	stdout := strings.TrimSpace(result.Stdout)
	if stdout == "" {
		return nil, nil
	}
	return strings.Split(stdout, "\n"), nil
}
