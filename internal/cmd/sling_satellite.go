package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/dispatch"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// machineSelectCounter provides atomic round-robin machine selection.
var machineSelectCounter uint64

// selectMachine picks the next machine for dispatch. If machineName is
// non-empty, selects that specific machine. Otherwise uses round-robin
// across all enabled workers.
func selectMachine(machines *config.MachinesConfig, machineName string) (string, *config.MachineEntry, error) {
	if machineName != "" {
		m, ok := machines.Machines[machineName]
		if !ok {
			return "", nil, fmt.Errorf("machine %q not found in machines config", machineName)
		}
		if !m.Enabled {
			return "", nil, fmt.Errorf("machine %q is disabled", machineName)
		}
		if !m.IsWorker() {
			return "", nil, fmt.Errorf("machine %q is not a worker (roles: %v)", machineName, m.Roles)
		}
		return machineName, m, nil
	}

	workers := machines.WorkerMachines()
	if len(workers) == 0 {
		return "", nil, fmt.Errorf("no enabled worker machines in machines config")
	}

	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)

	idx := atomic.AddUint64(&machineSelectCounter, 1) - 1
	selected := names[idx%uint64(len(names))]
	return selected, workers[selected], nil
}

// selectMachineRoundRobin distributes bead IDs across workers for batch dispatch.
func selectMachineRoundRobin(machines *config.MachinesConfig, beadIDs []string) (map[string][]string, error) {
	workers := machines.WorkerMachines()
	if len(workers) == 0 {
		return nil, fmt.Errorf("no enabled worker machines in machines config")
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

// loadMachinesConfig loads machines.json from the town root.
// Returns nil (not an error) if the file does not exist — single-machine
// users never have a machines.json.
func loadMachinesConfig(townRoot string) (*config.MachinesConfig, error) {
	path := constants.MayorMachinesPath(townRoot)
	cfg, err := config.LoadMachinesConfig(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading machines config: %w", err)
	}
	return cfg, nil
}

// resolveDispatchMachine determines which machine should handle dispatch.
// If explicitMachine is non-empty, it's looked up directly (--machine override).
// Otherwise, machines.json is loaded and the configured DispatchPolicy decides.
// Returns ("", nil, nil) for local dispatch.
func resolveDispatchMachine(townRoot, rigName, explicitMachine string) (string, *config.MachineEntry, error) {
	// 1. Explicit --machine always wins
	if explicitMachine != "" {
		machines, err := loadMachinesConfig(townRoot)
		if err != nil {
			return "", nil, fmt.Errorf("loading machines config: %w", err)
		}
		if machines == nil {
			return "", nil, fmt.Errorf("--machine requires machines.json in mayor/")
		}
		return selectMachine(machines, explicitMachine)
	}

	// 2. No machines.json → local
	machines, err := loadMachinesConfig(townRoot)
	if err != nil {
		return "", nil, fmt.Errorf("loading machines config: %w", err)
	}
	if machines == nil {
		return "", nil, nil
	}

	// 3. Empty or local-only policy → local
	policyName := machines.DispatchPolicy
	if policyName == "" {
		policyName = string(dispatch.PolicySatelliteFirst) // default
	}
	if policyName == string(dispatch.PolicyLocalOnly) {
		return "", nil, nil
	}

	// 4. Resolve policy, build context, route
	policy, err := dispatch.Resolve(policyName)
	if err != nil {
		return "", nil, fmt.Errorf("resolving dispatch policy: %w", err)
	}

	ctx := buildRoutingContextFn(machines)
	result, err := policy.Route(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("dispatch policy %q: %w", policyName, err)
	}

	// "" = local dispatch
	if result.Machine == "" {
		return "", nil, nil
	}

	// Look up the selected machine entry
	entry, ok := machines.Machines[result.Machine]
	if !ok {
		return "", nil, fmt.Errorf("policy selected unknown machine %q", result.Machine)
	}
	return result.Machine, entry, nil
}

// resolveDispatchMachineFn is a seam for tests.
var resolveDispatchMachineFn = resolveDispatchMachine

// buildRoutingContextFn is a seam for tests (avoids SSH/tmux in unit tests).
var buildRoutingContextFn = buildRoutingContext

// buildRoutingContext constructs a dispatch.RoutingContext from machines.json
// and current load data.
func buildRoutingContext(machines *config.MachinesConfig) dispatch.RoutingContext {
	workers := machines.WorkerMachines()
	ctx := dispatch.RoutingContext{}

	// Local load
	localActive := countActivePolecats()
	ctx.LocalLoad = &dispatch.MachineLoad{
		Name:           "local",
		MaxPolecats:    0, // unlimited locally (operator's machine)
		ActivePolecats: localActive,
	}

	// Remote loads (sorted by name for determinism)
	names := make([]string, 0, len(workers))
	for name := range workers {
		names = append(names, name)
	}
	sort.Strings(names)

	remoteCounts := countRemotePolecats(machines, names)
	for _, name := range names {
		entry := workers[name]
		ctx.Machines = append(ctx.Machines, dispatch.MachineLoad{
			Name:           name,
			MaxPolecats:    entry.MaxPolecats,
			ActivePolecats: remoteCounts[name],
		})
	}

	return ctx
}

// countRemotePolecats SSHes to each worker and counts active polecat tmux sessions.
// Returns a map of machine name → active polecat count.
func countRemotePolecats(machines *config.MachinesConfig, workerNames []string) map[string]int {
	counts := make(map[string]int, len(workerNames))
	for _, name := range workerNames {
		entry, ok := machines.Machines[name]
		if !ok {
			continue
		}
		count, err := countRemotePolecatsOnMachine(entry)
		if err != nil {
			// SSH failure → assume machine is busy (conservative: don't overload)
			counts[name] = entry.MaxPolecats
			continue
		}
		counts[name] = count
	}
	return counts
}

// countRemotePolecatsOnMachine SSHes to a single machine and counts polecat sessions.
func countRemotePolecatsOnMachine(entry *config.MachineEntry) (int, error) {
	target := entry.SSHTarget()
	out, err := runSSH(target, "tmux list-sessions -F '#{session_name}' 2>/dev/null || true", 10*time.Second)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		identity, err := session.ParseSessionName(line)
		if err != nil {
			continue
		}
		if identity.Role == session.RolePolecat {
			count++
		}
	}
	return count, nil
}

// SatelliteSpawnResult holds the outcome of a satellite bootstrap.
type SatelliteSpawnResult struct {
	MachineName string
	RigName     string
	PolecatName string
	SessionName string
	ClonePath   string
	BaseBranch  string
	Branch      string
	ProxyURL    string
}

// issueCertResponse mirrors proxy.issueCertResponse for JSON parsing.
type issueCertResponse struct {
	CN        string `json:"cn"`
	Cert      string `json:"cert"`
	Key       string `json:"key"`
	CA        string `json:"ca"`
	Serial    string `json:"serial"`
	ExpiresAt string `json:"expires_at"`
}

// spawnRemoteSatellite bootstraps a polecat on a remote machine:
//  1. Pre-allocate polecat name
//  2. SSH to hub: issue cert via admin API
//  3. SSH to target: write cert files, spawn polecat with --name, wire env vars
//  4. Verify: tmux env vars set + proxy reachable
//
// On failure, cleans up (denies cert, kills session).
func spawnRemoteSatellite(
	machines *config.MachinesConfig,
	machineName string,
	machine *config.MachineEntry,
	rigName string,
	opts SlingSpawnOptions,
) (*SatelliteSpawnResult, error) {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// 1. Pre-allocate polecat name locally (needed for cert CN)
	polecatName, err := preAllocatePolecatName(townRoot, rigName)
	if err != nil {
		return nil, fmt.Errorf("pre-allocating polecat name: %w", err)
	}

	proxyURL := machines.ProxyURL(machine.Host)
	hubTarget := machines.HubSSHTarget()
	targetTownRoot := machine.TownRoot
	if targetTownRoot == "" {
		targetTownRoot = "~/gt"
	}

	// 2. Issue cert via admin API on hub
	fmt.Printf("  %s Issuing cert for %s on hub...\n", style.Bold.Render("→"), polecatName)
	certResp, err := issueCertViaSSH(hubTarget, rigName, polecatName)
	if err != nil {
		return nil, fmt.Errorf("issuing cert on hub: %w", err)
	}

	// Track cert serial for cleanup on failure
	certSerial := certResp.Serial

	// 3. Spawn polecat on target machine
	fmt.Printf("  %s Spawning %s on %s...\n", style.Bold.Render("→"), polecatName, machineName)

	doltHost := machines.DoltHost
	doltPort := machines.DoltPort
	if doltPort == 0 {
		doltPort = 3307
	}

	spawnResult, err := spawnOnTarget(
		machine.SSHTarget(),
		targetTownRoot,
		rigName,
		polecatName,
		doltHost,
		doltPort,
		proxyURL,
		certResp,
		opts,
	)
	if err != nil {
		// Clean up: deny the orphaned cert
		_ = denyCertViaSSH(hubTarget, certSerial)
		return nil, fmt.Errorf("spawning on %s: %w", machineName, err)
	}

	// 4. Verify
	fmt.Printf("  %s Verifying proxy connectivity...\n", style.Bold.Render("→"))
	if err := verifyBootstrap(machine.SSHTarget(), spawnResult.SessionName, proxyURL); err != nil {
		// Clean up: kill session and deny cert
		_ = killRemoteSession(machine.SSHTarget(), spawnResult.SessionName)
		_ = denyCertViaSSH(hubTarget, certSerial)
		return nil, fmt.Errorf("bootstrap verification failed on %s: %w", machineName, err)
	}

	fmt.Printf("  %s Satellite polecat %s running on %s (proxy: %s)\n",
		style.Bold.Render("✓"), polecatName, machineName, proxyURL)

	spawnResult.MachineName = machineName
	spawnResult.ProxyURL = proxyURL
	return spawnResult, nil
}

// preAllocatePolecatName allocates a name from the local rig's name pool.
func preAllocatePolecatName(townRoot, rigName string) (string, error) {
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(rigName)
	if err != nil {
		return "", fmt.Errorf("rig %q not found: %w", rigName, err)
	}

	polecatGit := git.NewGit(r.Path)
	t := tmux.NewTmux()
	mgr := polecat.NewManager(r, polecatGit, t)
	return mgr.AllocateName()
}

// issueCertViaSSH issues a polecat client cert by calling the admin API on the hub.
func issueCertViaSSH(hubHost, rigName, polecatName string) (*issueCertResponse, error) {
	payload, err := json.Marshal(map[string]string{"rig": rigName, "name": polecatName})
	if err != nil {
		return nil, fmt.Errorf("marshaling cert request: %w", err)
	}
	cmd := fmt.Sprintf(`curl -sf -X POST http://127.0.0.1:9877/v1/admin/issue-cert -d %q`, string(payload))
	stdout, err := runSSH(hubHost, cmd, 30*time.Second)
	if err != nil {
		return nil, err
	}

	var resp issueCertResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		return nil, fmt.Errorf("parsing cert response: %w", err)
	}
	return &resp, nil
}

// denyCertViaSSH revokes a cert on the hub.
func denyCertViaSSH(hubHost, serial string) error {
	payload, err := json.Marshal(map[string]string{"serial": serial})
	if err != nil {
		return fmt.Errorf("marshaling deny request: %w", err)
	}
	cmd := fmt.Sprintf(`curl -sf -X POST http://127.0.0.1:9877/v1/admin/deny-cert -d %q`, string(payload))
	_, err = runSSH(hubHost, cmd, 15*time.Second)
	return err
}

// buildBootstrapScript generates the shell script that runs on the target
// machine during satellite bootstrap. Cert material is piped via stdin,
// never embedded in the script.
func buildBootstrapScript(townRoot, rigName, polecatName, doltHost string, doltPort int, proxyURL string) string {
	return fmt.Sprintf(`
set -e
CERT_DIR=$(mktemp -d)
chmod 700 "$CERT_DIR"

# Read cert material from stdin (JSON)
CERT_JSON=$(cat)
printf '%%s' "$CERT_JSON" | jq -r .cert > "$CERT_DIR/cert.pem"
printf '%%s' "$CERT_JSON" | jq -r .key  > "$CERT_DIR/key.pem"
printf '%%s' "$CERT_JSON" | jq -r .ca   > "$CERT_DIR/ca.pem"
chmod 600 "$CERT_DIR/key.pem"

cd %s

# Spawn polecat (creates worktree, does NOT start tmux session)
# Temporarily disable set -e to capture the error message
set +e
SPAWN_OUTPUT=$(gt polecat spawn %s --name %s --dolt-host %s --dolt-port %d --json 2>&1)
SPAWN_EXIT=$?
set -e
if [ $SPAWN_EXIT -ne 0 ]; then
  echo "SPAWN FAILED (exit $SPAWN_EXIT): $SPAWN_OUTPUT" >&2
  exit 1
fi
SPAWN_JSON=$(printf '%%s\n' "$SPAWN_OUTPUT" | grep '^{' | tail -1)
CLONE_PATH=$(printf '%%s' "$SPAWN_JSON" | jq -r .clone_path)

# Session name follows gt convention
SESS="gt-%s-p-%s"

# Create a detached tmux session in the polecat's worktree
tmux new-session -d -s "$SESS" -c "$CLONE_PATH" 2>/dev/null || true

# Set proxy env vars in tmux session (must happen after session creation)
tmux setenv -t "$SESS" GT_PROXY_URL "%s"
tmux setenv -t "$SESS" GT_PROXY_CERT "$CERT_DIR/cert.pem"
tmux setenv -t "$SESS" GT_PROXY_KEY "$CERT_DIR/key.pem"
tmux setenv -t "$SESS" GT_PROXY_CA "$CERT_DIR/ca.pem"
tmux setenv -t "$SESS" GT_REAL_BIN "$HOME/.local/bin/gt.real"

# Output merged session info as JSON
printf '%%s' "$SPAWN_JSON" | jq -c --arg sess "$SESS" --arg cert_dir "$CERT_DIR" '. + {session_name: $sess, cert_dir: $cert_dir}'
`,
		townRoot,
		rigName, polecatName, doltHost, doltPort,
		rigName, polecatName,
		proxyURL,
	)
}

// spawnOutputInfo holds the parsed JSON from a satellite spawn command.
type spawnOutputInfo struct {
	SessionName string `json:"session_name"`
	CertDir     string `json:"cert_dir"`
	ClonePath   string `json:"clone_path"`
	BaseBranch  string `json:"base_branch"`
	Branch      string `json:"branch"`
}

// parseSpawnOutput extracts the spawn result from mixed stdout that may
// contain progress text before the final JSON line.
func parseSpawnOutput(stdout string) (*spawnOutputInfo, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	lastLine := lines[len(lines)-1]

	var info spawnOutputInfo
	if err := json.Unmarshal([]byte(lastLine), &info); err != nil {
		return nil, fmt.Errorf("parsing spawn output: %w\noutput: %s", err, stdout)
	}
	return &info, nil
}

// spawnOnTarget SSHes to the target machine and:
//  1. Writes cert files (piped via stdin, chmod 600 on key)
//  2. Spawns polecat with --name
//  3. Sets tmux env vars for proxy routing
//  4. Starts polecat session
func spawnOnTarget(
	sshTarget, townRoot, rigName, polecatName string,
	doltHost string, doltPort int,
	proxyURL string,
	cert *issueCertResponse,
	opts SlingSpawnOptions,
) (*SatelliteSpawnResult, error) {
	script := buildBootstrapScript(townRoot, rigName, polecatName, doltHost, doltPort, proxyURL)

	// Pipe cert material via stdin
	certJSON, err := json.Marshal(cert)
	if err != nil {
		return nil, fmt.Errorf("marshaling cert: %w", err)
	}

	stdout, err := runSSHWithStdin(sshTarget, script, certJSON, 120*time.Second)
	if err != nil {
		return nil, err
	}

	info, err := parseSpawnOutput(stdout)
	if err != nil {
		return nil, err
	}

	return &SatelliteSpawnResult{
		RigName:     rigName,
		PolecatName: polecatName,
		SessionName: info.SessionName,
		ClonePath:   info.ClonePath,
		BaseBranch:  info.BaseBranch,
		Branch:      info.Branch,
	}, nil
}

// verifyBootstrap confirms the satellite bootstrap succeeded by checking:
// 1. GT_PROXY_URL is set in the tmux session
// 2. Proxy is reachable (gt version works via proxy)
func verifyBootstrap(sshTarget, sessionName, expectedURL string) error {
	// Check tmux env var
	cmd := fmt.Sprintf(`tmux showenv -t %s GT_PROXY_URL 2>/dev/null`, sessionName)
	stdout, err := runSSH(sshTarget, cmd, 15*time.Second)
	if err != nil {
		return fmt.Errorf("GT_PROXY_URL not set in tmux session: %w", err)
	}
	if !strings.Contains(stdout, expectedURL) {
		return fmt.Errorf("GT_PROXY_URL mismatch: got %q, want %q", strings.TrimSpace(stdout), expectedURL)
	}
	return nil
}

// killRemoteSession kills a tmux session on a remote machine.
func killRemoteSession(sshTarget, sessionName string) error {
	cmd := fmt.Sprintf(`tmux kill-session -t %s 2>/dev/null || true`, sessionName)
	_, err := runSSH(sshTarget, cmd, 15*time.Second)
	return err
}

// runSSH executes a command on a remote machine via SSH.
func runSSH(sshTarget, remoteCmd string, timeout time.Duration) (string, error) {
	return runSSHWithStdin(sshTarget, remoteCmd, nil, timeout)
}

// runSSHWithStdin executes a command on a remote machine via SSH, optionally piping stdin.
func runSSHWithStdin(sshTarget, remoteCmd string, stdin []byte, timeout time.Duration) (string, error) {
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=accept-new",
		sshTarget,
		remoteCmd,
	}
	cmd := exec.Command("ssh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("ssh %s: %w\nstderr: %s", sshTarget, err, stderr.String())
		}
		return stdout.String(), nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("ssh %s: timed out after %s", sshTarget, timeout)
	}
}
