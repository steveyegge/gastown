package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// ojDaemonStatusTimeout is how long we wait for `oj daemon status --json`.
const ojDaemonStatusTimeout = 10 * time.Second

// ojStatusTimeout is how long we wait for `oj status --json`.
const ojStatusTimeout = 10 * time.Second

// ojDaemonStatus represents the JSON output of `oj daemon status --json`.
type ojDaemonStatus struct {
	Status         string `json:"status"`
	Version        string `json:"version"`
	UptimeSecs     uint64 `json:"uptime_secs"`
	Uptime         string `json:"uptime"`
	JobsActive     int    `json:"jobs_active"`
	SessionsActive int    `json:"sessions_active"`
	OrphanCount    int    `json:"orphan_count"`
}

// ojStatusOverview represents the JSON output of `oj status --json`.
type ojStatusOverview struct {
	UptimeSecs uint64              `json:"uptime_secs"`
	Namespaces []ojNamespaceStatus `json:"namespaces"`
}

// ojNamespaceStatus represents a namespace in the OJ status overview.
type ojNamespaceStatus struct {
	Namespace  string          `json:"namespace"`
	ActiveJobs []ojJobEntry    `json:"active_jobs"`
	Workers    []ojWorkerEntry `json:"workers"`
	Queues     []ojQueueEntry  `json:"queues"`
}

// ojJobEntry is a minimal representation of an active job.
type ojJobEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Step string `json:"step"`
}

// ojWorkerEntry is a minimal representation of a worker.
type ojWorkerEntry struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Active      int    `json:"active"`
	Concurrency int    `json:"concurrency"`
}

// ojQueueEntry is a minimal representation of a queue.
type ojQueueEntry struct {
	Name    string `json:"name"`
	Pending int    `json:"pending"`
	Active  int    `json:"active"`
	Dead    int    `json:"dead"`
}

// OjDaemonCheck verifies the OJ (Odd Jobs) daemon is running and reports health metrics.
type OjDaemonCheck struct {
	BaseCheck
}

// NewOjDaemonCheck creates a new OJ daemon health check.
func NewOjDaemonCheck() *OjDaemonCheck {
	return &OjDaemonCheck{
		BaseCheck: BaseCheck{
			CheckName:        "oj-daemon",
			CheckDescription: "Check if OJ daemon is running and healthy",
			CheckCategory:    CategoryInfrastructure,
		},
	}
}

// Run checks the OJ daemon status.
func (c *OjDaemonCheck) Run(ctx *CheckContext) *CheckResult {
	// Skip if OJ dispatch is not enabled
	if os.Getenv("GT_SLING_OJ") != "1" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "OJ dispatch not enabled (GT_SLING_OJ != 1)",
		}
	}

	// Check if oj binary exists
	ojPath, err := exec.LookPath("oj")
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "OJ binary not found in PATH",
			FixHint: "Install OJ or ensure 'oj' is in your PATH",
		}
	}
	_ = ojPath

	// Query daemon status
	status, err := queryOjDaemonStatus()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "OJ daemon not reachable",
			Details: []string{err.Error()},
			FixHint: "Start OJ daemon with 'oj daemon start'",
		}
	}

	// Daemon responded but might report not_running
	if status.Status == "not_running" {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "OJ daemon is not running",
			FixHint: "Start OJ daemon with 'oj daemon start'",
		}
	}

	// Daemon is running - gather detailed status
	details := []string{}
	if status.Version != "" {
		details = append(details, "Version: "+status.Version)
	}
	if status.Uptime != "" {
		details = append(details, "Uptime: "+status.Uptime)
	}
	details = append(details, "Active jobs: "+strconv.Itoa(status.JobsActive))
	details = append(details, "Active sessions: "+strconv.Itoa(status.SessionsActive))

	if status.OrphanCount > 0 {
		details = append(details, fmt.Sprintf("Orphaned jobs: %d (run 'oj daemon orphans')", status.OrphanCount))
	}

	// Get extended status with worker/queue details
	overview, err := queryOjStatusOverview()
	if err == nil {
		totalWorkers := 0
		totalQueuePending := 0
		for _, ns := range overview.Namespaces {
			totalWorkers += len(ns.Workers)
			for _, q := range ns.Queues {
				totalQueuePending += q.Pending
			}
		}
		if totalWorkers > 0 {
			details = append(details, "Workers: "+strconv.Itoa(totalWorkers))
		}
		if totalQueuePending > 0 {
			details = append(details, "Queue depth: "+strconv.Itoa(totalQueuePending)+" pending")
		}
	}

	// Check for warning conditions
	if status.OrphanCount > 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: fmt.Sprintf("OJ daemon running but has %d orphaned job(s)", status.OrphanCount),
			Details: details,
			FixHint: "Run 'oj daemon orphans' to investigate orphaned jobs",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: fmt.Sprintf("OJ daemon running (%d active job(s), uptime %s)", status.JobsActive, status.Uptime),
		Details: details,
	}
}

// queryOjDaemonStatus runs `oj daemon status --json` and parses the result.
func queryOjDaemonStatus() (*ojDaemonStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ojDaemonStatusTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "oj", "daemon", "status", "--json")
	cmd.Env = os.Environ()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("oj daemon status: %w", err)
	}

	var status ojDaemonStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("parsing oj daemon status output: %w", err)
	}

	return &status, nil
}

// queryOjStatusOverview runs `oj status --json` and parses the result.
func queryOjStatusOverview() (*ojStatusOverview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ojStatusTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "oj", "status", "--json")
	cmd.Env = os.Environ()
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("oj status: %w", err)
	}

	var overview ojStatusOverview
	if err := json.Unmarshal(output, &overview); err != nil {
		return nil, fmt.Errorf("parsing oj status output: %w", err)
	}

	return &overview, nil
}
