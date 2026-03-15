package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/dispatch"
	"github.com/steveyegge/gastown/internal/session"
)

// --- machines-config check ---

// MachinesConfigCheck validates that machines.json exists, parses, and has
// a valid dispatch policy.
type MachinesConfigCheck struct {
	BaseCheck
}

func NewMachinesConfigCheck() *MachinesConfigCheck {
	return &MachinesConfigCheck{
		BaseCheck: BaseCheck{
			CheckName:        "machines-config",
			CheckDescription: "Validate machines.json and dispatch policy",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

func (c *MachinesConfigCheck) Run(ctx *CheckContext) *CheckResult {
	path := constants.MayorMachinesPath(ctx.TownRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "No machines.json (single-machine mode)",
			}
		}
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot read machines.json: %v", err),
		}
	}

	var cfg config.MachinesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Invalid JSON: %v", err),
			FixHint: "Fix the JSON syntax in mayor/machines.json",
		}
	}

	// Validate policy
	policy := cfg.DispatchPolicy
	if policy == "" {
		policy = "satellite-first (default)"
	} else if !dispatch.IsValidPolicy(cfg.DispatchPolicy) {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Invalid dispatch_policy %q", cfg.DispatchPolicy),
			Details: []string{
				"Valid policies: satellite-first, local-first, round-robin, satellite-only, local-only",
			},
		}
	}

	workers := cfg.WorkerMachines()
	details := []string{
		fmt.Sprintf("Policy: %s", policy),
		fmt.Sprintf("Workers: %d enabled", len(workers)),
	}
	for name, m := range workers {
		maxStr := "unlimited"
		if m.MaxPolecats > 0 {
			maxStr = fmt.Sprintf("%d", m.MaxPolecats)
		}
		details = append(details, fmt.Sprintf("  %s: %s (max %s polecats)", name, m.Host, maxStr))
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("%d workers, policy=%s", len(workers), policy),
		Details: details,
	}
}

// --- satellite-ssh check ---

// SatelliteSSHCheck verifies SSH connectivity to each enabled worker.
type SatelliteSSHCheck struct {
	BaseCheck
}

func NewSatelliteSSHCheck() *SatelliteSSHCheck {
	return &SatelliteSSHCheck{
		BaseCheck: BaseCheck{
			CheckName:        "satellite-ssh",
			CheckDescription: "Check SSH connectivity to satellite workers",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

func (c *SatelliteSSHCheck) Run(ctx *CheckContext) *CheckResult {
	machines, err := loadMachinesForDoctor(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: err.Error(),
		}
	}
	if machines == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No machines.json (skipped)",
		}
	}

	workers := machines.WorkerMachines()
	if len(workers) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No enabled workers in machines.json",
		}
	}

	var details []string
	failures := 0
	names := sortedKeys(workers)

	for _, name := range names {
		entry := workers[name]
		target := entry.SSHTarget()
		start := time.Now()
		err := sshPing(target)
		elapsed := time.Since(start)

		if err != nil {
			failures++
			details = append(details, fmt.Sprintf("%s (%s): UNREACHABLE — %v", name, target, err))
		} else {
			details = append(details, fmt.Sprintf("%s (%s): OK (%s)", name, target, elapsed.Round(time.Millisecond)))
		}
	}

	if failures > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("%d/%d workers unreachable", failures, len(workers)),
			Details: details,
			FixHint: "Check Tailscale connectivity and SSH config",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("%d/%d workers reachable", len(workers), len(workers)),
		Details: details,
	}
}

// --- satellite-proxy check ---

// SatelliteProxyCheck verifies the mTLS proxy is reachable from the hub.
type SatelliteProxyCheck struct {
	BaseCheck
}

func NewSatelliteProxyCheck() *SatelliteProxyCheck {
	return &SatelliteProxyCheck{
		BaseCheck: BaseCheck{
			CheckName:        "satellite-proxy",
			CheckDescription: "Check mTLS proxy reachability",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

func (c *SatelliteProxyCheck) Run(ctx *CheckContext) *CheckResult {
	machines, err := loadMachinesForDoctor(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: err.Error(),
		}
	}
	if machines == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No machines.json (skipped)",
		}
	}

	// Proxy admin API is on 127.0.0.1:9877 on the hub machine.
	// We check if the hub is reachable via SSH and the admin port responds.
	hubTarget := machines.HubSSHTarget()
	if hubTarget == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Cannot determine hub machine (dolt_host not in machines)",
		}
	}

	// Check admin API on hub via SSH curl
	out, err := sshRun(hubTarget,
		"curl -s -o /dev/null -w '%{http_code}' --connect-timeout 3 http://127.0.0.1:9877/v1/admin/issue-cert 2>/dev/null || echo FAIL",
		10*time.Second)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot reach hub (%s): %v", hubTarget, err),
			FixHint: "Ensure proxy server is running on the hub machine",
		}
	}

	out = strings.TrimSpace(out)
	// Admin API returns 405 for GET (it wants POST), which proves it's listening.
	// Any HTTP response means the proxy is alive.
	if out == "FAIL" || out == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Proxy admin API not responding on hub",
			Details: []string{fmt.Sprintf("Hub: %s, port 9877", hubTarget)},
			FixHint: "Start the proxy server on the hub machine",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("Proxy admin API responding on hub (HTTP %s)", out),
	}
}

// --- satellite-capacity check ---

// SatelliteCapacityCheck reports current load vs MaxPolecats on each worker.
type SatelliteCapacityCheck struct {
	BaseCheck
}

func NewSatelliteCapacityCheck() *SatelliteCapacityCheck {
	return &SatelliteCapacityCheck{
		BaseCheck: BaseCheck{
			CheckName:        "satellite-capacity",
			CheckDescription: "Check satellite worker load vs capacity",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

func (c *SatelliteCapacityCheck) Run(ctx *CheckContext) *CheckResult {
	machines, err := loadMachinesForDoctor(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: err.Error(),
		}
	}
	if machines == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No machines.json (skipped)",
		}
	}

	workers := machines.WorkerMachines()
	if len(workers) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "No enabled workers",
		}
	}

	var details []string
	totalActive := 0
	totalCapacity := 0
	atCapacity := 0
	countErrors := 0
	names := sortedKeys(workers)

	for _, name := range names {
		entry := workers[name]
		active, err := countRemotePolecatsForDoctor(entry)
		if err != nil {
			countErrors++
			details = append(details, fmt.Sprintf("%s: ERROR counting — %v", name, err))
			continue
		}

		maxStr := "unlimited"
		capStr := ""
		if entry.MaxPolecats > 0 {
			maxStr = fmt.Sprintf("%d", entry.MaxPolecats)
			totalCapacity += entry.MaxPolecats
			if active >= entry.MaxPolecats {
				atCapacity++
				capStr = " [AT CAPACITY]"
			}
		}
		totalActive += active
		details = append(details, fmt.Sprintf("%s: %d/%s active polecats%s", name, active, maxStr, capStr))
	}

	status := StatusOK
	msg := fmt.Sprintf("%d active across %d workers", totalActive, len(workers))

	if countErrors > 0 {
		status = StatusWarning
		msg += fmt.Sprintf(" (%d unreachable)", countErrors)
	}
	if atCapacity > 0 && atCapacity == len(workers)-countErrors {
		status = StatusWarning
		msg = fmt.Sprintf("All reachable workers at capacity (%d active)", totalActive)
	} else if atCapacity > 0 {
		msg += fmt.Sprintf(" (%d at capacity)", atCapacity)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  status,
		Message: msg,
		Details: details,
	}
}

// --- dispatch-policy check ---

// DispatchPolicyCheck confirms the configured policy resolves and can produce
// a routing decision with the current machine set.
type DispatchPolicyCheck struct {
	BaseCheck
}

func NewDispatchPolicyCheck() *DispatchPolicyCheck {
	return &DispatchPolicyCheck{
		BaseCheck: BaseCheck{
			CheckName:        "dispatch-policy",
			CheckDescription: "Verify dispatch policy produces valid routing decisions",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

func (c *DispatchPolicyCheck) Run(ctx *CheckContext) *CheckResult {
	machines, err := loadMachinesForDoctor(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: err.Error(),
		}
	}
	if machines == nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No machines.json (local-only dispatch)",
		}
	}

	policyName := machines.DispatchPolicy
	if policyName == "" {
		policyName = "satellite-first"
	}

	policy, err := dispatch.Resolve(policyName)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Cannot resolve policy %q: %v", policyName, err),
		}
	}

	// Simulate a routing decision with synthetic idle load
	workers := machines.WorkerMachines()
	var machineLoads []dispatch.MachineLoad
	names := sortedKeys(workers)
	for _, name := range names {
		entry := workers[name]
		machineLoads = append(machineLoads, dispatch.MachineLoad{
			Name:           name,
			MaxPolecats:    entry.MaxPolecats,
			ActivePolecats: 0, // simulate idle
		})
	}

	routingCtx := dispatch.RoutingContext{
		Machines: machineLoads,
		LocalLoad: &dispatch.MachineLoad{
			Name:           "local",
			MaxPolecats:    0,
			ActivePolecats: 0,
		},
	}

	result, err := policy.Route(routingCtx)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: fmt.Sprintf("Policy %q cannot route even at idle: %v", policyName, err),
			FixHint: "Check that machines.json has enabled workers",
		}
	}

	target := "local"
	if result.Machine != "" {
		target = result.Machine
	}

	details := []string{
		fmt.Sprintf("Policy: %s", policyName),
		fmt.Sprintf("Idle routing target: %s", target),
		fmt.Sprintf("Available workers: %d", len(workers)),
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("Policy %q routes to %s at idle", policyName, target),
		Details: details,
	}
}

// --- helpers ---

// loadMachinesForDoctor loads machines.json, returning nil if absent.
func loadMachinesForDoctor(townRoot string) (*config.MachinesConfig, error) {
	path := constants.MayorMachinesPath(townRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading machines.json: %w", err)
	}
	var cfg config.MachinesConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing machines.json: %w", err)
	}
	return &cfg, nil
}

// sshPing checks SSH connectivity with a short timeout.
func sshPing(target string) error {
	cmd := exec.Command("ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		target, "echo ok")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sshRun runs a command on a remote machine via SSH with a timeout.
func sshRun(target, remoteCmd string, timeout time.Duration) (string, error) {
	cmd := exec.Command("ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes",
		target, remoteCmd)

	timer := time.AfterFunc(timeout, func() { _ = cmd.Process.Kill() })
	out, err := cmd.CombinedOutput()
	timer.Stop()

	if err != nil {
		return "", fmt.Errorf("%v (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// countRemotePolecatsForDoctor SSHes to a machine and counts polecat tmux sessions.
func countRemotePolecatsForDoctor(entry *config.MachineEntry) (int, error) {
	target := entry.SSHTarget()
	out, err := sshRun(target, "tmux list-sessions -F '#{session_name}' 2>/dev/null || true", 10*time.Second)
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

// sortedKeys returns sorted map keys.
func sortedKeys(m map[string]*config.MachineEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
