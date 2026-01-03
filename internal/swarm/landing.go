package swarm

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// LandingConfig configures the landing protocol.
type LandingConfig struct {
	// TownRoot is the workspace root for mail routing.
	TownRoot string

	// ForceKill kills sessions without graceful shutdown.
	ForceKill bool

	// SkipGitAudit skips the git safety audit.
	SkipGitAudit bool
}

// LandingResult contains the result of a landing operation.
type LandingResult struct {
	SwarmID       string
	Success       bool
	Error         string
	SessionsStopped int
	BranchesCleaned int
	PolecatsAtRisk  []string
}

// GitAuditResult contains the result of a git safety audit.
type GitAuditResult struct {
	Worker          string
	ClonePath       string
	HasUncommitted  bool
	HasUnpushed     bool
	HasStashes      bool
	BeadsOnly       bool // True if changes are only in .beads/
	CodeAtRisk      bool
	Details         string
}

// ExecuteLanding performs the witness landing protocol for a swarm.
func (m *Manager) ExecuteLanding(swarmID string, config LandingConfig) (*LandingResult, error) {
	swarm, err := m.LoadSwarm(swarmID)
	if err != nil {
		return nil, err
	}

	result := &LandingResult{
		SwarmID: swarmID,
	}

	// Phase 1: Stop all polecat sessions
	t := tmux.NewTmux()
	sessMgr := session.NewManager(t, m.rig)

	for _, worker := range swarm.Workers {
		running, _ := sessMgr.IsRunning(worker)
		if running {
			err := sessMgr.Stop(worker, config.ForceKill)
			if err != nil {
				// Continue anyway
			} else {
				result.SessionsStopped++
			}
		}
	}

	// Wait for graceful shutdown
	time.Sleep(2 * time.Second)

	// Phase 2: Git audit (check for code at risk)
	if !config.SkipGitAudit {
		for _, worker := range swarm.Workers {
			audit := m.auditWorkerGit(worker)
			if audit.CodeAtRisk {
				result.PolecatsAtRisk = append(result.PolecatsAtRisk, worker)
			}
		}

		if len(result.PolecatsAtRisk) > 0 {
			result.Success = false
			result.Error = fmt.Sprintf("code at risk for workers: %s",
				strings.Join(result.PolecatsAtRisk, ", "))

			// Notify Mayor
			if config.TownRoot != "" {
				m.notifyMayorCodeAtRisk(config.TownRoot, swarmID, result.PolecatsAtRisk)
			}

			return result, nil
		}
	}

	// Phase 3: Cleanup branches
	if err := m.CleanupBranches(swarmID); err != nil {
		// Log but continue
	}
	result.BranchesCleaned = len(swarm.Tasks) + 1 // tasks + integration

	// Phase 4: Update swarm state
	swarm.State = SwarmLanded
	swarm.UpdatedAt = time.Now()

	// Send landing report to Mayor
	if config.TownRoot != "" {
		m.notifyMayorLanded(config.TownRoot, swarm, result)
	}

	result.Success = true
	return result, nil
}

// auditWorkerGit checks a worker's git state for uncommitted/unpushed work.
func (m *Manager) auditWorkerGit(worker string) GitAuditResult {
	result := GitAuditResult{
		Worker: worker,
	}

	// Get polecat clone path
	clonePath := fmt.Sprintf("%s/polecats/%s", m.rig.Path, worker)
	result.ClonePath = clonePath

	// Check for uncommitted changes
	statusOutput, err := m.gitRunOutput(clonePath, "status", "--porcelain")
	if err == nil && strings.TrimSpace(statusOutput) != "" {
		result.HasUncommitted = true
		// Check if only .beads changes
		result.BeadsOnly = isBeadsOnlyChanges(statusOutput)
	}

	// Check for unpushed commits
	unpushed, err := m.gitRunOutput(clonePath, "log", "--oneline", "@{u}..", "--")
	if err == nil && strings.TrimSpace(unpushed) != "" {
		result.HasUnpushed = true
	}

	// Check for stashes
	stashes, err := m.gitRunOutput(clonePath, "stash", "list")
	if err == nil && strings.TrimSpace(stashes) != "" {
		result.HasStashes = true
	}

	// Determine if code is at risk
	if result.HasUncommitted && !result.BeadsOnly {
		result.CodeAtRisk = true
		result.Details = "uncommitted code changes"
	} else if result.HasUnpushed {
		result.CodeAtRisk = true
		result.Details = "unpushed commits"
	}

	return result
}

// isBeadsOnlyChanges checks if all changes are in .beads/ directory.
func isBeadsOnlyChanges(statusOutput string) bool {
	for _, line := range strings.Split(statusOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Status format: XY filename
		if len(line) > 3 {
			filename := strings.TrimSpace(line[3:])
			if !strings.HasPrefix(filename, ".beads/") {
				return false
			}
		}
	}
	return true
}

// gitRunOutput runs a git command and returns stdout.
func (m *Manager) gitRunOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// notifyMayorCodeAtRisk sends an alert to Mayor about code at risk.
func (m *Manager) notifyMayorCodeAtRisk(_, swarmID string, workers []string) { // townRoot unused: router uses gitDir
	router := mail.NewRouter(m.gitDir)
	msg := &mail.Message{
		From: fmt.Sprintf("%s/refinery", m.rig.Name),
		To:   "mayor/",
		Subject: fmt.Sprintf("Code at risk in swarm %s", swarmID),
		Body: fmt.Sprintf(`Landing blocked for swarm %s.

The following workers have uncommitted or unpushed code:
%s

Manual intervention required.`,
			swarmID, strings.Join(workers, "\n- ")),
		Priority: mail.PriorityHigh,
	}
	_ = router.Send(msg) // best-effort notification
}

// notifyMayorLanded sends a landing report to Mayor.
func (m *Manager) notifyMayorLanded(_ string, swarm *Swarm, result *LandingResult) { // townRoot unused: router uses gitDir
	router := mail.NewRouter(m.gitDir)
	msg := &mail.Message{
		From: fmt.Sprintf("%s/refinery", m.rig.Name),
		To:   "mayor/",
		Subject: fmt.Sprintf("Swarm %s landed", swarm.ID),
		Body: fmt.Sprintf(`Swarm landing complete.

Swarm: %s
Target: %s
Sessions stopped: %d
Branches cleaned: %d
Tasks merged: %d`,
			swarm.ID,
			swarm.TargetBranch,
			result.SessionsStopped,
			result.BranchesCleaned,
			len(swarm.Tasks)),
	}
	_ = router.Send(msg) // best-effort notification
}
