//go:build windows

package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func killProcessGroup(pgid int) {
	proc, err := os.FindProcess(pgid)
	if err != nil {
		return
	}
	_ = proc.Kill()
}

// getParentPID returns the parent process ID (PPID) for a given PID.
// On Windows, this is not used for PGID verification, so we return empty string.
func getParentPID(pid string) string {
	return ""
}

// getProcessGroupID returns the process group ID (PGID) for a given PID.
// Windows doesn't expose POSIX process groups, so we treat the PID as the PGID.
func getProcessGroupID(pid string) string {
	pid = strings.TrimSpace(pid)
	if pid == "" {
		return ""
	}

	pidInt, err := strconv.Atoi(pid)
	if err != nil || pidInt <= 0 {
		return ""
	}

	exists, err := processExists(pidInt)
	if err != nil || !exists {
		return ""
	}

	return pid
}

// getProcessGroupMembers returns all PIDs in a process group.
// On Windows, we model the group as just the PID itself.
func getProcessGroupMembers(pgid string) []string {
	pgid = strings.TrimSpace(pgid)
	if pgid == "" {
		return nil
	}

	pgidInt, err := strconv.Atoi(pgid)
	if err != nil || pgidInt <= 0 {
		return nil
	}

	exists, err := processExists(pgidInt)
	if err != nil || !exists {
		return nil
	}

	return []string{pgid}
}

func processExists(pid int) (bool, error) {
	filter := fmt.Sprintf("PID eq %d", pid)
	out, err := exec.Command("tasklist", "/FI", filter, "/FO", "CSV", "/NH").Output()
	if err != nil {
		return false, err
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		return false, nil
	}
	if strings.HasPrefix(text, "INFO:") {
		return false, nil
	}

	return true, nil
}
