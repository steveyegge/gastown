package cmd

import (
	"bytes"
	"encoding/json"
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
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
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
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("loading machines config: %w", err)
	}
	return cfg, nil
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
	hubTarget := machines.DoltHost
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
	cmd := fmt.Sprintf(
		`curl -sf -X POST http://127.0.0.1:9877/v1/admin/issue-cert -d '{"rig":"%s","name":"%s"}'`,
		rigName, polecatName,
	)
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
	cmd := fmt.Sprintf(
		`curl -sf -X POST http://127.0.0.1:9877/v1/admin/deny-cert -d '{"serial":"%s"}'`,
		serial,
	)
	_, err := runSSH(hubHost, cmd, 15*time.Second)
	return err
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
	// Build the bootstrap script to run on the target.
	// Cert material is piped via stdin (never in process args).
	script := fmt.Sprintf(`
set -e
CERT_DIR=$(mktemp -d)
chmod 700 "$CERT_DIR"

# Read cert material from stdin (JSON)
CERT_JSON=$(cat)
echo "$CERT_JSON" | jq -r .cert > "$CERT_DIR/cert.pem"
echo "$CERT_JSON" | jq -r .key  > "$CERT_DIR/key.pem"
echo "$CERT_JSON" | jq -r .ca   > "$CERT_DIR/ca.pem"
chmod 600 "$CERT_DIR/key.pem"

cd %s

# Spawn polecat with pre-allocated name
gt polecat spawn %s --name %s --bead %s --dolt-host %s --dolt-port %d --json 2>/dev/null

# Set proxy env vars in tmux session
SESS="gt-%s-p-%s"
tmux setenv -t "$SESS" GT_PROXY_URL "%s"
tmux setenv -t "$SESS" GT_PROXY_CERT "$CERT_DIR/cert.pem"
tmux setenv -t "$SESS" GT_PROXY_KEY "$CERT_DIR/key.pem"
tmux setenv -t "$SESS" GT_PROXY_CA "$CERT_DIR/ca.pem"

# Output session info as JSON
echo '{"session_name":"'"$SESS"'","cert_dir":"'"$CERT_DIR"'"}'
`,
		townRoot,
		rigName, polecatName, opts.HookBead, doltHost, doltPort,
		rigName, polecatName,
		proxyURL,
	)

	// Pipe cert material via stdin
	certJSON, err := json.Marshal(cert)
	if err != nil {
		return nil, fmt.Errorf("marshaling cert: %w", err)
	}

	stdout, err := runSSHWithStdin(sshTarget, script, certJSON, 120*time.Second)
	if err != nil {
		return nil, err
	}

	// Parse the last JSON line from output
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	lastLine := lines[len(lines)-1]

	var info struct {
		SessionName string `json:"session_name"`
		CertDir     string `json:"cert_dir"`
	}
	if err := json.Unmarshal([]byte(lastLine), &info); err != nil {
		return nil, fmt.Errorf("parsing spawn output: %w\noutput: %s", err, stdout)
	}

	return &SatelliteSpawnResult{
		RigName:     rigName,
		PolecatName: polecatName,
		SessionName: info.SessionName,
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
