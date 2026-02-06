package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// LocalDoltCheck warns if a local dolt sql-server process is running when
// BD_DAEMON_HOST is set (K8s mode). Local Dolt should not run when the
// system is configured to use a remote Dolt instance. (gt-c9esrg)
type LocalDoltCheck struct {
	BaseCheck
}

// NewLocalDoltCheck creates a new local Dolt process check.
func NewLocalDoltCheck() *LocalDoltCheck {
	return &LocalDoltCheck{
		BaseCheck: BaseCheck{
			CheckName:        "local-dolt",
			CheckDescription: "Check for local dolt sql-server when K8s mode is configured",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks if a local dolt sql-server is running when BD_DAEMON_HOST is set.
func (c *LocalDoltCheck) Run(ctx *CheckContext) *CheckResult {
	remoteHost := os.Getenv("BD_DAEMON_HOST")
	if remoteHost == "" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "BD_DAEMON_HOST not set (not in K8s mode)",
		}
	}

	// Check for local dolt sql-server processes
	// #nosec G204 -- pgrep is a fixed command with fixed arguments
	out, err := exec.Command("pgrep", "-af", "dolt sql-server").Output()
	if err != nil {
		// pgrep returns exit code 1 when no processes found - that's the good case
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("No local dolt sql-server running (K8s mode: %s)", remoteHost),
		}
	}

	// Found local dolt processes - this is a problem in K8s mode
	lines := strings.TrimSpace(string(out))
	var pids []string
	for _, line := range strings.Split(lines, "\n") {
		if line != "" {
			pids = append(pids, strings.Fields(line)[0])
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: fmt.Sprintf("Local dolt sql-server running (PID %s) but BD_DAEMON_HOST=%s is set", strings.Join(pids, ","), remoteHost),
		FixHint: fmt.Sprintf("Kill local dolt: kill %s", strings.Join(pids, " ")),
		Details: strings.Split(lines, "\n"),
	}
}
