package autopilot

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// totalMemoryBytes returns total physical memory on Linux from /proc/meminfo.
func totalMemoryBytes() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb uint64
			_, err := fmt.Sscanf(line, "MemTotal: %d kB", &kb)
			if err != nil {
				return 0
			}
			return kb * 1024
		}
	}
	return 0
}

// availableMemoryBytes returns available memory on Linux from /proc/meminfo.
func availableMemoryBytes() uint64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			var kb uint64
			_, err := fmt.Sscanf(line, "MemAvailable: %d kB", &kb)
			if err != nil {
				return 0
			}
			return kb * 1024
		}
	}
	return 0
}

// loadAverage1 returns the 1-minute load average on Linux from /proc/loadavg.
func loadAverage1() float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	var load1 float64
	_, _ = fmt.Sscanf(string(data), "%f", &load1)
	return load1
}

// countActiveSessions counts active Claude agent tmux sessions.
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
