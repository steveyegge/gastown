package autopilot

import (
	"os/exec"
	"strconv"
	"strings"
)

// totalMemoryBytes returns total physical memory on macOS via sysctl.
func totalMemoryBytes() uint64 {
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// availableMemoryBytes returns available memory on macOS via vm_stat.
// Uses free + inactive pages as the available pool.
func availableMemoryBytes() uint64 {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return 0
	}

	var freePages, inactivePages uint64
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Pages free:") {
			freePages = parseVMStatValue(line)
		} else if strings.HasPrefix(line, "Pages inactive:") {
			inactivePages = parseVMStatValue(line)
		}
	}

	pageSize := uint64(16384) // default for Apple Silicon
	out2, err := exec.Command("sysctl", "-n", "hw.pagesize").Output()
	if err == nil {
		if ps, err := strconv.ParseUint(strings.TrimSpace(string(out2)), 10, 64); err == nil {
			pageSize = ps
		}
	}

	return (freePages + inactivePages) * pageSize
}

// parseVMStatValue extracts the numeric value from a vm_stat line like "Pages free: 12345."
func parseVMStatValue(line string) uint64 {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return 0
	}
	s := strings.TrimSpace(parts[1])
	s = strings.TrimSuffix(s, ".")
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// loadAverage1 returns the 1-minute load average on macOS via sysctl.
func loadAverage1() float64 {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0
	}
	// Output format: "{ 1.23 4.56 7.89 }"
	s := strings.TrimSpace(string(out))
	s = strings.Trim(s, "{ }")
	fields := strings.Fields(s)
	if len(fields) < 1 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}

// countActiveSessions counts active Claude agent tmux sessions.
// Uses tmux list-sessions to find sessions with agent role markers.
func countActiveSessions() int {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if isAgentSession(line) {
			count++
		}
	}
	return count
}

// isAgentSession returns true if the tmux session name looks like a Gas Town agent.
func isAgentSession(name string) bool {
	markers := []string{"mayor", "witness", "refinery", "polecat", "deacon", "crew", "boot", "dog"}
	for _, m := range markers {
		if strings.Contains(name, m) {
			return true
		}
	}
	return false
}
