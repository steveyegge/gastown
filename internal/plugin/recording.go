package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
)

// RunResult represents the outcome of a plugin execution.
type RunResult string

const (
	ResultSuccess RunResult = "success"
	ResultFailure RunResult = "failure"
	ResultSkipped RunResult = "skipped"
)

// PluginRunRecord represents data for creating a plugin run bead.
type PluginRunRecord struct {
	PluginName string
	RigName    string
	Result     RunResult
	Body       string
}

// PluginRunBead represents a recorded plugin run from the ledger.
type PluginRunBead struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	Labels    []string  `json:"labels"`
	Result    RunResult `json:"-"` // Parsed from labels
}

// Recorder handles plugin run recording and querying.
type Recorder struct {
	townRoot string
}

// NewRecorder creates a new plugin run recorder.
func NewRecorder(townRoot string) *Recorder {
	return &Recorder{townRoot: townRoot}
}

// RecordRun creates an ephemeral bead for a plugin run.
// This is pure data writing - the caller decides what result to record.
func (r *Recorder) RecordRun(record PluginRunRecord) (string, error) {
	title := fmt.Sprintf("Plugin run: %s", record.PluginName)

	// Build labels
	labels := []string{
		"type:plugin-run",
		fmt.Sprintf("plugin:%s", record.PluginName),
		fmt.Sprintf("result:%s", record.Result),
	}
	if record.RigName != "" {
		labels = append(labels, fmt.Sprintf("rig:%s", record.RigName))
	}

	// Build bd create command
	args := []string{
		"create",
		"--ephemeral",
		"--json",
		"--title=" + title,
	}
	for _, label := range labels {
		args = append(args, "-l", label)
	}
	if record.Body != "" {
		args = append(args, "--description="+record.Body)
	}

	cmd := exec.Command("bd", args...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = r.townRoot
	// Set BEADS_DIR explicitly to prevent inherited env vars from causing
	// prefix mismatches when redirects are in play.
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(r.townRoot))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("creating plugin run bead: %s: %w", stderr.String(), err)
	}

	// Parse created bead ID from JSON output
	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return "", fmt.Errorf("parsing bd create output: %w", err)
	}

	return result.ID, nil
}

// GetLastRun returns the most recent run for a plugin.
// Returns nil if no runs found.
func (r *Recorder) GetLastRun(pluginName string) (*PluginRunBead, error) {
	runs, err := r.queryRuns(pluginName, 1, "")
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return runs[0], nil
}

// GetRunsSince returns all runs for a plugin since the given duration.
// Duration format: "1h", "24h", "7d", etc.
func (r *Recorder) GetRunsSince(pluginName string, since string) ([]*PluginRunBead, error) {
	return r.queryRuns(pluginName, 0, since)
}

// queryRuns queries plugin run beads from the ledger.
func (r *Recorder) queryRuns(pluginName string, limit int, since string) ([]*PluginRunBead, error) {
	args := []string{
		"list",
		"--json",
		"--all", // Include closed beads too
		"-l", "type:plugin-run",
		"-l", fmt.Sprintf("plugin:%s", pluginName),
	}
	if limit > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", limit))
	}
	if since != "" {
		// Convert duration like "1h" to created-after format
		// bd supports relative dates with - prefix (e.g., -1h, -24h)
		sinceArg := since
		if !strings.HasPrefix(since, "-") {
			sinceArg = "-" + since
		}
		args = append(args, "--created-after="+sinceArg)
	}

	cmd := exec.Command("bd", args...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = r.townRoot
	// Set BEADS_DIR explicitly to prevent inherited env vars from causing
	// prefix mismatches when redirects are in play.
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beads.ResolveBeadsDir(r.townRoot))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Empty result is OK (no runs found)
		if stderr.Len() == 0 || stdout.String() == "[]\n" {
			return nil, nil
		}
		return nil, fmt.Errorf("querying plugin runs: %s: %w", stderr.String(), err)
	}

	// Parse JSON output
	var beads []struct {
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		CreatedAt string   `json:"created_at"`
		Labels    []string `json:"labels"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &beads); err != nil {
		// Empty array is valid
		if stdout.String() == "[]\n" || stdout.Len() == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	// Convert to PluginRunBead with parsed result
	runs := make([]*PluginRunBead, 0, len(beads))
	for _, b := range beads {
		run := &PluginRunBead{
			ID:     b.ID,
			Title:  b.Title,
			Labels: b.Labels,
		}

		// Parse created_at
		if t, err := time.Parse(time.RFC3339, b.CreatedAt); err == nil {
			run.CreatedAt = t
		}

		// Extract result from labels
		for _, label := range b.Labels {
			if len(label) > 7 && label[:7] == "result:" {
				run.Result = RunResult(label[7:])
				break
			}
		}

		runs = append(runs, run)
	}

	return runs, nil
}

// CountRunsSince returns the count of runs for a plugin since the given duration.
// This is useful for cooldown gate evaluation.
func (r *Recorder) CountRunsSince(pluginName string, since string) (int, error) {
	runs, err := r.GetRunsSince(pluginName, since)
	if err != nil {
		return 0, err
	}
	return len(runs), nil
}
