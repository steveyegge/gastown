package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/session"
)

// LifecycleHygieneCheck detects and cleans up stale lifecycle state.
// This can happen when:
// - Lifecycle messages weren't properly deleted after processing
// - Agent state.json has stuck requesting_* flags
// - Session was manually killed without clearing state
type LifecycleHygieneCheck struct {
	FixableCheck
	staleMessages   []staleMessage
	stuckStateFiles []stuckState
}

type staleMessage struct {
	ID      string
	Subject string
	From    string
}

type stuckState struct {
	stateFile string
	identity  string
	flag      string
}

// NewLifecycleHygieneCheck creates a new lifecycle hygiene check.
func NewLifecycleHygieneCheck() *LifecycleHygieneCheck {
	return &LifecycleHygieneCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "lifecycle-hygiene",
				CheckDescription: "Check for stale lifecycle messages and stuck state flags",
			},
		},
	}
}

// Run checks for stale lifecycle state.
func (c *LifecycleHygieneCheck) Run(ctx *CheckContext) *CheckResult {
	c.staleMessages = nil
	c.stuckStateFiles = nil

	var details []string

	// Check for stale lifecycle messages in deacon inbox
	staleCount := c.checkDeaconInbox(ctx)
	if staleCount > 0 {
		details = append(details, fmt.Sprintf("%d stale lifecycle message(s) in deacon inbox", staleCount))
	}

	// Check for stuck requesting_* flags in state files
	stuckCount := c.checkStateFiles(ctx)
	if stuckCount > 0 {
		details = append(details, fmt.Sprintf("%d agent(s) with stuck requesting_* flags", stuckCount))
	}

	total := staleCount + stuckCount
	if total == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No stale lifecycle state found",
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d lifecycle hygiene issue(s)", total),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to clean up",
	}
}

// checkDeaconInbox looks for stale lifecycle messages.
func (c *LifecycleHygieneCheck) checkDeaconInbox(ctx *CheckContext) int {
	// Get deacon inbox via gt mail
	cmd := exec.Command("gt", "mail", "inbox", "--identity", "deacon/", "--json")
	cmd.Dir = ctx.TownRoot

	output, err := cmd.Output()
	if err != nil {
		return 0 // Can't check, assume OK
	}

	if len(output) == 0 || string(output) == "[]" || string(output) == "[]\n" {
		return 0
	}

	var messages []struct {
		ID      string `json:"id"`
		From    string `json:"from"`
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(output, &messages); err != nil {
		return 0
	}

	// Look for lifecycle messages
	for _, msg := range messages {
		if strings.HasPrefix(strings.ToLower(msg.Subject), "lifecycle:") {
			c.staleMessages = append(c.staleMessages, staleMessage{
				ID:      msg.ID,
				Subject: msg.Subject,
				From:    msg.From,
			})
		}
	}

	return len(c.staleMessages)
}

// checkStateFiles looks for stuck requesting_* flags in state.json files.
func (c *LifecycleHygieneCheck) checkStateFiles(ctx *CheckContext) int {
	stateFiles := c.findStateFiles(ctx.TownRoot)

	for _, sf := range stateFiles {
		data, err := os.ReadFile(sf.path)
		if err != nil {
			continue
		}

		var state map[string]interface{}
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}

		// Check for any requesting_* flags
		for key, val := range state {
			if strings.HasPrefix(key, "requesting_") {
				if boolVal, ok := val.(bool); ok && boolVal {
					// Found a stuck flag - verify session is actually healthy
					if c.isSessionHealthy(sf.identity, ctx.TownRoot) {
						c.stuckStateFiles = append(c.stuckStateFiles, stuckState{
							stateFile: sf.path,
							identity:  sf.identity,
							flag:      key,
						})
					}
				}
			}
		}
	}

	return len(c.stuckStateFiles)
}

type stateFileInfo struct {
	path     string
	identity string
}

// findStateFiles locates all state.json files for agents.
func (c *LifecycleHygieneCheck) findStateFiles(townRoot string) []stateFileInfo {
	var files []stateFileInfo

	// Mayor state
	mayorState := filepath.Join(townRoot, "mayor", "state.json")
	if _, err := os.Stat(mayorState); err == nil {
		files = append(files, stateFileInfo{path: mayorState, identity: "mayor"})
	}

	// Scan rigs for witness, refinery, and crew state files
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return files
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "mayor" {
			continue
		}

		rigName := entry.Name()
		rigPath := filepath.Join(townRoot, rigName)

		// Witness state
		witnessState := filepath.Join(rigPath, "witness", "state.json")
		if _, err := os.Stat(witnessState); err == nil {
			files = append(files, stateFileInfo{
				path:     witnessState,
				identity: rigName + "-witness",
			})
		}

		// Refinery state
		refineryState := filepath.Join(rigPath, "refinery", "state.json")
		if _, err := os.Stat(refineryState); err == nil {
			files = append(files, stateFileInfo{
				path:     refineryState,
				identity: rigName + "-refinery",
			})
		}

		// Crew state files
		crewPath := filepath.Join(rigPath, "crew")
		crewEntries, err := os.ReadDir(crewPath)
		if err != nil {
			continue
		}
		for _, crew := range crewEntries {
			if !crew.IsDir() || strings.HasPrefix(crew.Name(), ".") {
				continue
			}
			crewState := filepath.Join(crewPath, crew.Name(), "state.json")
			if _, err := os.Stat(crewState); err == nil {
				files = append(files, stateFileInfo{
					path:     crewState,
					identity: rigName + "-crew-" + crew.Name(),
				})
			}
		}
	}

	return files
}

// isSessionHealthy checks if the tmux session for this identity exists and is running.
func (c *LifecycleHygieneCheck) isSessionHealthy(identity, townRoot string) bool {
	sessionName := identityToSessionName(identity, townRoot)
	if sessionName == "" {
		return false
	}

	// Check if session exists
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	return cmd.Run() == nil
}

// identityToSessionName converts an identity to its tmux session name.
func identityToSessionName(identity, townRoot string) string {
	switch identity {
	case "mayor":
		return session.MayorSessionName()
	default:
		if strings.HasSuffix(identity, "-witness") ||
			strings.HasSuffix(identity, "-refinery") ||
			strings.Contains(identity, "-crew-") {
			return "gt-" + identity
		}
		return ""
	}
}

// Fix cleans up stale lifecycle state.
func (c *LifecycleHygieneCheck) Fix(ctx *CheckContext) error {
	var errors []string

	// Delete stale lifecycle messages
	for _, msg := range c.staleMessages {
		cmd := exec.Command("gt", "mail", "delete", msg.ID) //nolint:gosec // G204: msg.ID is from internal state, not user input
		cmd.Dir = ctx.TownRoot
		if err := cmd.Run(); err != nil {
			errors = append(errors, fmt.Sprintf("failed to delete message %s: %v", msg.ID, err))
		}
	}

	// Clear stuck requesting_* flags
	for _, stuck := range c.stuckStateFiles {
		if err := c.clearRequestingFlag(stuck); err != nil {
			errors = append(errors, fmt.Sprintf("failed to clear %s in %s: %v", stuck.flag, stuck.identity, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

// clearRequestingFlag removes the stuck requesting_* flag from a state file.
func (c *LifecycleHygieneCheck) clearRequestingFlag(stuck stuckState) error {
	data, err := os.ReadFile(stuck.stateFile)
	if err != nil {
		return err
	}

	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	// Remove the requesting flag and any associated timestamp
	delete(state, stuck.flag)
	delete(state, "requesting_time")

	newData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(stuck.stateFile, newData, 0644)
}
