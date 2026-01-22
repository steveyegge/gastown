package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
func (w *ConvoyWatcher) getTrackingConvoys(issueID string) []string {
	// Use bd CLI instead of direct sqlite3 access to support both SQLite and Dolt backends
	// Get all convoy-type issues, then filter for those that track this issue
	cmd := exec.Command("bd", "list", "--type", "convoy", "--json", "--limit", "0")
	cmd.Dir = w.townRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil
	}

	var convoys []struct {
		ID           string   `json:"id"`
		Dependencies []string `json:"dependencies,omitempty"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		return nil
	}

	// For each convoy, check if it tracks the given issue
	convoyIDs := make([]string, 0)
	for _, convoy := range convoys {
		// Get dependencies for this convoy
		depCmd := exec.Command("bd", "dep", "list", convoy.ID, "--json")
		depCmd.Dir = w.townRoot
		var depStdout bytes.Buffer
		depCmd.Stdout = &depStdout

		if err := depCmd.Run(); err != nil {
			continue
		}

		var deps []struct {
			DependsOnID string `json:"depends_on_id"`
			Type        string `json:"type"`
		}
		if err := json.Unmarshal(depStdout.Bytes(), &deps); err != nil {
			continue
		}

		// Check if any "tracks" dependency matches our issueID
		for _, dep := range deps {
			if dep.Type == "tracks" {
				// Handle both direct ID and external reference format
				dependsOn := dep.DependsOnID
				if dependsOn == issueID || strings.HasSuffix(dependsOn, ":"+issueID) {
					convoyIDs = append(convoyIDs, convoy.ID)
					break
				}
			}
		}
	}
	return convoyIDs
}

// checkConvoyCompletion checks if all issues tracked by a convoy are closed.
// If so, runs gt convoy check to close the convoy.
func (w *ConvoyWatcher) checkConvoyCompletion(convoyID string) {
	// Use bd CLI instead of direct sqlite3 access to support both SQLite and Dolt backends
	cmd := exec.Command("bd", "show", "--json", convoyID)
	cmd.Dir = w.townRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return
	}

	var convoy struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoy); err != nil {
		return
	}

	if convoy.Status == "closed" {
		return // Already closed
	}

	// Run gt convoy check with specific convoy ID for targeted check
	// This reuses the existing logic which handles notifications, etc.
	w.logger("convoy watcher: running completion check for %s", convoyID)

	checkCmd := exec.Command("gt", "convoy", "check", convoyID)
	checkCmd.Dir = w.townRoot
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
