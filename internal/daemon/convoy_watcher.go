package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ConvoyWatcher monitors bd activity for issue closes and triggers convoy completion checks.
// When an issue closes, it checks if the issue is tracked by any convoy and runs the
// completion check if all tracked issues are now closed.
type ConvoyWatcher struct {
	townRoot string
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	logger   func(format string, args ...interface{})
}

// bdActivityEvent represents an event from bd activity --json.
type bdActivityEvent struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	IssueID   string `json:"issue_id"`
	Symbol    string `json:"symbol"`
	Message   string `json:"message"`
	OldStatus string `json:"old_status,omitempty"`
	NewStatus string `json:"new_status,omitempty"`
}

// NewConvoyWatcher creates a new convoy watcher.
func NewConvoyWatcher(townRoot string, logger func(format string, args ...interface{})) *ConvoyWatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConvoyWatcher{
		townRoot: townRoot,
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
	}
}

// Start begins the convoy watcher goroutine.
func (w *ConvoyWatcher) Start() error {
	w.wg.Add(1)
	go w.run()
	return nil
}

// Stop gracefully stops the convoy watcher.
func (w *ConvoyWatcher) Stop() {
	w.cancel()
	w.wg.Wait()
}

// run is the main watcher loop.
func (w *ConvoyWatcher) run() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			// Start bd activity --follow --town --json
			if err := w.watchActivity(); err != nil {
				w.logger("convoy watcher: bd activity error: %v, restarting in 5s", err)
				// Wait before retry, but respect context cancellation
				select {
				case <-w.ctx.Done():
					return
				case <-time.After(5 * time.Second):
					// Continue to retry
				}
			}
		}
	}
}

// watchActivity starts bd activity and processes events until error or context cancellation.
func (w *ConvoyWatcher) watchActivity() error {
	cmd := exec.CommandContext(w.ctx, "bd", "activity", "--follow", "--town", "--json")
	cmd.Dir = w.townRoot
	cmd.Env = os.Environ() // Inherit PATH to find bd executable

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting bd activity: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-w.ctx.Done():
			_ = cmd.Process.Kill()
			return nil
		default:
		}

		line := scanner.Text()
		w.processLine(line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading bd activity: %w", err)
	}

	return cmd.Wait()
}

// processLine processes a single line from bd activity (NDJSON format).
func (w *ConvoyWatcher) processLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	var event bdActivityEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return // Skip malformed lines
	}

	// Only interested in status changes to closed
	if event.Type != "status" || event.NewStatus != "closed" {
		return
	}

	w.logger("convoy watcher: detected close of %s", event.IssueID)

	// Check if this issue is tracked by any convoy
	convoyIDs := w.getTrackingConvoys(event.IssueID)
	if len(convoyIDs) == 0 {
		return
	}

	w.logger("convoy watcher: %s is tracked by %d convoy(s): %v", event.IssueID, len(convoyIDs), convoyIDs)

	// Check each tracking convoy for completion
	for _, convoyID := range convoyIDs {
		w.checkConvoyCompletion(convoyID)
	}
}

// getTrackingConvoys returns convoy IDs that track the given issue.
// Uses bd dep list to query the dependency graph.
func (w *ConvoyWatcher) getTrackingConvoys(issueID string) []string {
	// Query for convoys that track this issue (direction=up finds dependents)
	cmd := exec.Command("bd", "dep", "list", issueID, "--direction=up", "-t", "tracks", "--json")
	cmd.Dir = w.townRoot
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil
	}

	var results []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil
	}

	convoyIDs := make([]string, 0, len(results))
	for _, r := range results {
		convoyIDs = append(convoyIDs, r.ID)
	}
	return convoyIDs
}

// checkConvoyCompletion checks if all issues tracked by a convoy are closed.
// If so, runs gt convoy check to close the convoy.
func (w *ConvoyWatcher) checkConvoyCompletion(convoyID string) {
	// First check if the convoy is still open
	showCmd := exec.Command("bd", "show", convoyID, "--json")
	showCmd.Dir = w.townRoot
	showCmd.Env = os.Environ()
	var stdout bytes.Buffer
	showCmd.Stdout = &stdout

	if err := showCmd.Run(); err != nil {
		return
	}

	var convoyStatus []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoyStatus); err != nil || len(convoyStatus) == 0 {
		return
	}

	if convoyStatus[0].Status == "closed" {
		return // Already closed
	}

	// Run gt convoy check with specific convoy ID for targeted check
	// This reuses the existing logic which handles notifications, etc.
	w.logger("convoy watcher: running completion check for %s", convoyID)

	checkCmd := exec.Command("gt", "convoy", "check", convoyID)
	checkCmd.Dir = w.townRoot
	checkCmd.Env = os.Environ() // Inherit PATH to find gt executable
	var checkStdout, checkStderr bytes.Buffer
	checkCmd.Stdout = &checkStdout
	checkCmd.Stderr = &checkStderr

	if err := checkCmd.Run(); err != nil {
		w.logger("convoy watcher: gt convoy check failed: %v: %s", err, checkStderr.String())
		return
	}

	if output := checkStdout.String(); output != "" && !strings.Contains(output, "No convoys ready") {
		w.logger("convoy watcher: %s", strings.TrimSpace(output))
	}
}
