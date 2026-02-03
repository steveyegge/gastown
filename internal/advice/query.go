package advice

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// AdviceBead represents an advice bead from bd advice list --json.
// We only include the fields we need for hook execution.
type AdviceBead struct {
	ID                  string `json:"id"`
	Title               string `json:"title"`
	Priority            int    `json:"priority"`
	AdviceHookCommand   string `json:"advice_hook_command"`
	AdviceHookTrigger   string `json:"advice_hook_trigger"`
	AdviceHookTimeout   int    `json:"advice_hook_timeout"`
	AdviceHookOnFailure string `json:"advice_hook_on_failure"`
}

// QueryHooks queries advice hooks for an agent at a specific trigger point.
// It calls `bd advice list --for=<agentID> --json` and filters to hooks
// matching the requested trigger.
//
// Returns hooks sorted by priority (lower first).
func QueryHooks(agentID, trigger string) ([]*Hook, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agentID is required")
	}
	if trigger == "" {
		return nil, fmt.Errorf("trigger is required")
	}
	if !IsValidTrigger(trigger) {
		return nil, fmt.Errorf("invalid trigger: %s", trigger)
	}

	// Run bd advice list --for=<agent> --json
	cmd := exec.Command("bd", "advice", "list", "--for="+agentID, "--json")
	output, err := cmd.Output()
	if err != nil {
		// Check if it's just no advice found (empty result)
		if exitErr, ok := err.(*exec.ExitError); ok {
			// bd returns exit code 0 even with no results, so this is a real error
			return nil, fmt.Errorf("bd advice list failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("bd advice list failed: %w", err)
	}

	// Handle empty output (no advice)
	output = []byte(strings.TrimSpace(string(output)))
	if len(output) == 0 || string(output) == "[]" || string(output) == "null" {
		return nil, nil
	}

	// Parse JSON output
	var beads []AdviceBead
	if err := json.Unmarshal(output, &beads); err != nil {
		return nil, fmt.Errorf("parsing bd advice list output: %w", err)
	}

	// Filter to hooks matching the trigger
	var hooks []*Hook
	for _, bead := range beads {
		// Skip beads without hook commands
		if bead.AdviceHookCommand == "" {
			continue
		}
		// Skip beads that don't match the trigger
		if bead.AdviceHookTrigger != trigger {
			continue
		}

		hook := &Hook{
			ID:        bead.ID,
			Title:     bead.Title,
			Command:   bead.AdviceHookCommand,
			Trigger:   bead.AdviceHookTrigger,
			Timeout:   bead.AdviceHookTimeout,
			OnFailure: bead.AdviceHookOnFailure,
			Priority:  bead.Priority,
		}

		// Apply defaults
		if hook.Timeout <= 0 {
			hook.Timeout = DefaultTimeout
		}
		if hook.OnFailure == "" {
			hook.OnFailure = OnFailureWarn
		}

		hooks = append(hooks, hook)
	}

	// Sort by priority (lower first)
	sort.Slice(hooks, func(i, j int) bool {
		return hooks[i].Priority < hooks[j].Priority
	})

	return hooks, nil
}

// RunHooksForTrigger is a convenience function that queries and runs all hooks
// for a given agent and trigger. It returns all results and any blocking error.
//
// workDir is the directory to execute hooks in.
// agentID is the agent's identifier (e.g., "gastown/polecats/furiosa").
// trigger is the lifecycle trigger (e.g., TriggerBeforeCommit).
func RunHooksForTrigger(workDir, agentID, trigger string) ([]*HookResult, error) {
	hooks, err := QueryHooks(agentID, trigger)
	if err != nil {
		return nil, fmt.Errorf("querying hooks: %w", err)
	}

	if len(hooks) == 0 {
		return nil, nil
	}

	runner := NewRunner(workDir, agentID)
	return runner.RunAll(hooks)
}
