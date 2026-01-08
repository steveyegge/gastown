package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Collector gathers signals from various sources for status detection.
type Collector struct {
	townRoot string
}

// NewCollector creates a new signal collector for the given town root.
func NewCollector(townRoot string) *Collector {
	return &Collector{townRoot: townRoot}
}

// WorkerInfo identifies a worker for signal collection.
type WorkerInfo struct {
	// Rig is the rig name (e.g., "gastown").
	Rig string

	// Name is the polecat name (e.g., "angharad").
	Name string

	// IssueID is the assigned issue ID (e.g., "gt-4i1").
	IssueID string

	// WorktreePath is the path to the worker's git worktree.
	WorktreePath string
}

// CollectSignals gathers all signals for a worker.
func (c *Collector) CollectSignals(worker WorkerInfo) Signals {
	signals := Signals{}

	// Collect git commit signal
	signals.GitCommit = c.collectGitCommit(worker.WorktreePath)

	// Collect beads update signal
	if worker.IssueID != "" {
		updatedAt, isBlocked, isClosed := c.collectBeadsInfo(worker.IssueID)
		signals.BeadsUpdate = updatedAt
		signals.IsBlocked = isBlocked
		signals.IsClosed = isClosed
	}

	// Collect tmux session signal
	sessionName := formatSessionName(worker.Rig, worker.Name)
	signals.SessionActivity, signals.SessionExists = c.collectSessionActivity(sessionName)

	return signals
}

// collectGitCommit gets the timestamp of the most recent commit in the worktree.
func (c *Collector) collectGitCommit(worktreePath string) *time.Time {
	if worktreePath == "" {
		return nil
	}

	// Get the timestamp of the most recent commit
	// Format: %ct gives Unix timestamp
	cmd := exec.Command("git", "log", "-1", "--format=%ct")
	cmd.Dir = worktreePath

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil
	}

	var timestamp int64
	if _, err := fmt.Sscanf(strings.TrimSpace(stdout.String()), "%d", &timestamp); err != nil || timestamp == 0 {
		return nil
	}

	t := time.Unix(timestamp, 0)
	return &t
}

// collectBeadsInfo gets the issue's updated_at timestamp, blocked status, and closed status.
func (c *Collector) collectBeadsInfo(issueID string) (*time.Time, bool, bool) {
	// Use bd show --json to get issue details
	cmd := exec.Command("bd", "show", issueID, "--json")
	if c.townRoot != "" {
		cmd.Dir = c.townRoot
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, false, false
	}

	// Parse JSON response
	var issues []struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		UpdatedAt string `json:"updated_at"`
		DependsOn []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"depends_on"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil || len(issues) == 0 {
		return nil, false, false
	}

	issue := issues[0]

	// Parse updated_at timestamp
	var updatedAt *time.Time
	if issue.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, issue.UpdatedAt); err == nil {
			updatedAt = &t
		}
	}

	// Check if closed
	isClosed := issue.Status == "closed"

	// Check if blocked by any open dependencies
	isBlocked := false
	for _, dep := range issue.DependsOn {
		if dep.Status != "closed" {
			isBlocked = true
			break
		}
	}

	return updatedAt, isBlocked, isClosed
}

// collectSessionActivity gets the tmux session's last activity timestamp.
func (c *Collector) collectSessionActivity(sessionName string) (*time.Time, bool) {
	// Query tmux for session activity
	// Format: session_activity returns unix timestamp
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}|#{session_activity}",
		"-f", fmt.Sprintf("#{==:#{session_name},%s}", sessionName))

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, false
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, false
	}

	// Parse output: "gt-gastown-angharad|1704312345"
	parts := strings.Split(output, "|")
	if len(parts) < 2 {
		return nil, true // Session exists but can't parse activity
	}

	var activityUnix int64
	if _, err := fmt.Sscanf(parts[1], "%d", &activityUnix); err != nil || activityUnix == 0 {
		return nil, true // Session exists but invalid timestamp
	}

	t := time.Unix(activityUnix, 0)
	return &t, true
}

// formatSessionName constructs the tmux session name from rig and polecat names.
func formatSessionName(rig, polecat string) string {
	return fmt.Sprintf("gt-%s-%s", rig, polecat)
}

// ParseAssignee extracts rig and polecat from an assignee string.
// Format: "rigname/polecats/polecatname" -> (rig, polecat)
func ParseAssignee(assignee string) (rig, polecat string, ok bool) {
	parts := strings.Split(assignee, "/")
	if len(parts) != 3 || parts[1] != "polecats" {
		return "", "", false
	}
	return parts[0], parts[2], true
}

// WorktreePathForPolecat returns the worktree path for a polecat.
func WorktreePathForPolecat(townRoot, rig, polecat string) string {
	return filepath.Join(townRoot, rig, "polecats", polecat)
}
