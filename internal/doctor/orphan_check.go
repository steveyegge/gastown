package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// OrphanSessionCheck detects orphaned tmux sessions that don't match
// the expected Gas Town session naming patterns.
type OrphanSessionCheck struct {
	FixableCheck
	orphanSessions []string // Cached during Run for use in Fix
}

// NewOrphanSessionCheck creates a new orphan session check.
func NewOrphanSessionCheck() *OrphanSessionCheck {
	return &OrphanSessionCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "orphan-sessions",
				CheckDescription: "Detect orphaned tmux sessions",
			},
		},
	}
}

// Run checks for orphaned Gas Town tmux sessions.
func (c *OrphanSessionCheck) Run(ctx *CheckContext) *CheckResult {
	t := tmux.NewTmux()

	sessions, err := t.ListSessions()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not list tmux sessions",
			Details: []string{err.Error()},
		}
	}

	if len(sessions) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No tmux sessions found",
		}
	}

	// Get list of valid rigs
	validRigs := c.getValidRigs(ctx.TownRoot)

	// Get session names for mayor/deacon
	mayorSession := session.MayorSessionName()
	deaconSession := session.DeaconSessionName()

	// Check each session
	var orphans []string
	var validCount int

	for _, sess := range sessions {
		if sess == "" {
			continue
		}

		// Only check gt-* sessions (Gas Town sessions)
		if !strings.HasPrefix(sess, "gt-") {
			continue
		}

		if c.isValidSession(sess, validRigs, mayorSession, deaconSession) {
			validCount++
		} else {
			orphans = append(orphans, sess)
		}
	}

	// Cache orphans for Fix
	c.orphanSessions = orphans

	if len(orphans) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d Gas Town sessions are valid", validCount),
		}
	}

	details := make([]string, len(orphans))
	for i, session := range orphans {
		details[i] = fmt.Sprintf("Orphan: %s", session)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d orphaned session(s)", len(orphans)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to kill orphaned sessions",
	}
}

// Fix kills all orphaned sessions, except crew sessions which are protected.
func (c *OrphanSessionCheck) Fix(ctx *CheckContext) error {
	if len(c.orphanSessions) == 0 {
		return nil
	}

	t := tmux.NewTmux()
	var lastErr error

	for _, session := range c.orphanSessions {
		// SAFEGUARD: Never auto-kill crew sessions.
		// Crew workers are human-managed and require explicit action.
		if isCrewSession(session) {
			continue
		}
		if err := t.KillSession(session); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// isCrewSession returns true if the session name matches the crew pattern.
// Crew sessions are gt-<rig>-crew-<name> and are protected from auto-cleanup.
func isCrewSession(session string) bool {
	// Pattern: gt-<rig>-crew-<name>
	// Example: gt-gastown-crew-joe
	parts := strings.Split(session, "-")
	if len(parts) >= 4 && parts[0] == "gt" && parts[2] == "crew" {
		return true
	}
	return false
}

// getValidRigs returns a list of valid rig names from the workspace.
func (c *OrphanSessionCheck) getValidRigs(townRoot string) []string {
	var rigs []string

	// Read rigs.json if it exists
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	if _, err := os.Stat(rigsPath); err == nil {
		// For simplicity, just scan directories at town root that look like rigs
		entries, err := os.ReadDir(townRoot)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() != "mayor" && entry.Name() != ".beads" && !strings.HasPrefix(entry.Name(), ".") {
					// Check if it looks like a rig (has polecats/ or crew/ directory)
					polecatsDir := filepath.Join(townRoot, entry.Name(), "polecats")
					crewDir := filepath.Join(townRoot, entry.Name(), "crew")
					if _, err := os.Stat(polecatsDir); err == nil {
						rigs = append(rigs, entry.Name())
					} else if _, err := os.Stat(crewDir); err == nil {
						rigs = append(rigs, entry.Name())
					}
				}
			}
		}
	}

	return rigs
}

// isValidSession checks if a session name matches expected Gas Town patterns.
// Valid patterns:
//   - gt-{town}-mayor (dynamic based on town name)
//   - gt-{town}-deacon (dynamic based on town name)
//   - gt-<rig>-witness
//   - gt-<rig>-refinery
//   - gt-<rig>-<polecat> (where polecat is any name)
//
// Note: We can't verify polecat names without reading state, so we're permissive.
func (c *OrphanSessionCheck) isValidSession(sess string, validRigs []string, mayorSession, deaconSession string) bool {
	// Mayor session is always valid (dynamic name based on town)
	if mayorSession != "" && sess == mayorSession {
		return true
	}

	// Deacon session is always valid (dynamic name based on town)
	if deaconSession != "" && sess == deaconSession {
		return true
	}

	// For rig-specific sessions, extract rig name
	// Pattern: gt-<rig>-<role>
	parts := strings.SplitN(sess, "-", 3)
	if len(parts) < 3 {
		// Invalid format - must be gt-<rig>-<something>
		return false
	}

	rigName := parts[1]

	// Check if this rig exists
	rigFound := false
	for _, r := range validRigs {
		if r == rigName {
			rigFound = true
			break
		}
	}

	if !rigFound {
		// Unknown rig - this is an orphan
		return false
	}

	role := parts[2]

	// witness and refinery are valid roles
	if role == "witness" || role == "refinery" {
		return true
	}

	// Any other name is assumed to be a polecat or crew member
	// We can't easily verify without reading state, so accept it
	return true
}

// OrphanProcessCheck detects orphaned Claude/claude-code processes
// that are not associated with a Gas Town tmux session.
type OrphanProcessCheck struct {
	FixableCheck
	orphanPIDs []int // Cached during Run for use in Fix
}

// NewOrphanProcessCheck creates a new orphan process check.
func NewOrphanProcessCheck() *OrphanProcessCheck {
	return &OrphanProcessCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "orphan-processes",
				CheckDescription: "Detect orphaned Claude processes",
			},
		},
	}
}

// Run checks for orphaned Claude processes.
func (c *OrphanProcessCheck) Run(ctx *CheckContext) *CheckResult {
	// Get list of tmux session PIDs
	tmuxPIDs, err := c.getTmuxSessionPIDs()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not get tmux session info",
			Details: []string{err.Error()},
		}
	}

	// Find Claude processes
	claudeProcs, err := c.findClaudeProcesses()
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "Could not list Claude processes",
			Details: []string{err.Error()},
		}
	}

	if len(claudeProcs) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No Claude processes found",
		}
	}

	// Check which Claude processes are orphaned
	var orphans []processInfo
	var validCount int

	for _, proc := range claudeProcs {
		if c.isOrphanProcess(proc, tmuxPIDs) {
			orphans = append(orphans, proc)
		} else {
			validCount++
		}
	}

	// Cache orphan PIDs for Fix
	c.orphanPIDs = make([]int, len(orphans))
	for i, p := range orphans {
		c.orphanPIDs[i] = p.pid
	}

	if len(orphans) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d Claude processes have valid parents", validCount),
		}
	}

	details := make([]string, len(orphans))
	for i, proc := range orphans {
		details[i] = fmt.Sprintf("PID %d: %s (parent: %d)", proc.pid, proc.cmd, proc.ppid)
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("Found %d orphaned Claude process(es)", len(orphans)),
		Details: details,
		FixHint: "Run 'gt doctor --fix' to kill orphaned processes",
	}
}

// Fix kills orphaned processes, with safeguards for crew sessions.
func (c *OrphanProcessCheck) Fix(ctx *CheckContext) error {
	if len(c.orphanPIDs) == 0 {
		return nil
	}

	// SAFEGUARD: Get crew session pane PIDs to avoid killing crew processes.
	// Even if a process appears orphaned, if its parent is a crew session pane,
	// we should not kill it (the detection might be wrong).
	crewPanePIDs := c.getCrewSessionPanePIDs()

	var lastErr error
	for _, pid := range c.orphanPIDs {
		// Check if this process has a crew session ancestor
		if c.hasCrewAncestor(pid, crewPanePIDs) {
			// Skip - this process might belong to a crew session
			continue
		}

		proc, err := os.FindProcess(pid)
		if err != nil {
			lastErr = err
			continue
		}
		if err := proc.Signal(os.Interrupt); err != nil {
			// Try SIGKILL if SIGINT fails
			if killErr := proc.Kill(); killErr != nil {
				lastErr = killErr
			}
		}
	}

	return lastErr
}

// getCrewSessionPanePIDs returns pane PIDs for all crew sessions.
func (c *OrphanProcessCheck) getCrewSessionPanePIDs() map[int]bool {
	pids := make(map[int]bool)

	t := tmux.NewTmux()
	sessions, err := t.ListSessions()
	if err != nil {
		return pids
	}

	for _, session := range sessions {
		if !isCrewSession(session) {
			continue
		}
		// Get pane PIDs for this crew session
		out, err := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_pid}").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			var pid int
			if _, err := fmt.Sscanf(line, "%d", &pid); err == nil {
				pids[pid] = true
			}
		}
	}

	return pids
}

// hasCrewAncestor checks if a process has a crew session pane as an ancestor.
func (c *OrphanProcessCheck) hasCrewAncestor(pid int, crewPanePIDs map[int]bool) bool {
	if len(crewPanePIDs) == 0 {
		return false
	}

	// Walk up the process tree
	currentPID := pid
	visited := make(map[int]bool)

	for currentPID > 1 && !visited[currentPID] {
		visited[currentPID] = true

		// Check if this PID is a crew pane
		if crewPanePIDs[currentPID] {
			return true
		}

		// Get parent PID
		out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", currentPID), "-o", "ppid=").Output() //nolint:gosec // G204: PID is numeric from internal state
		if err != nil {
			break
		}

		var ppid int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &ppid); err != nil {
			break
		}
		currentPID = ppid
	}

	return false
}

type processInfo struct {
	pid  int
	ppid int
	cmd  string
}

// getTmuxSessionPIDs returns PIDs of all tmux server processes and pane shell PIDs.
func (c *OrphanProcessCheck) getTmuxSessionPIDs() (map[int]bool, error) { //nolint:unparam // error return kept for future use
	// Get tmux server PID and all pane PIDs
	pids := make(map[int]bool)

	// Find tmux server processes using ps instead of pgrep.
	// pgrep -x tmux is unreliable on macOS - it often misses the actual server.
	// We use ps with awk to find processes where comm is exactly "tmux".
	out, err := exec.Command("sh", "-c", `ps ax -o pid,comm | awk '$2 == "tmux" || $2 ~ /\/tmux$/ { print $1 }'`).Output()
	if err != nil {
		// No tmux server running
		return pids, nil
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var pid int
		if _, err := fmt.Sscanf(line, "%d", &pid); err == nil {
			pids[pid] = true
		}
	}

	// Also get shell PIDs inside tmux panes
	t := tmux.NewTmux()
	sessions, _ := t.ListSessions()
	for _, session := range sessions {
		// Get pane PIDs for this session
		out, err := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_pid}").Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			var pid int
			if _, err := fmt.Sscanf(line, "%d", &pid); err == nil {
				pids[pid] = true
			}
		}
	}

	return pids, nil
}

// findClaudeProcesses finds all running claude/claude-code CLI processes.
// Excludes Claude.app desktop application and its helpers.
func (c *OrphanProcessCheck) findClaudeProcesses() ([]processInfo, error) {
	var procs []processInfo

	// Use ps to find claude processes
	// Look for both "claude" and "claude-code" in command
	out, err := exec.Command("ps", "-eo", "pid,ppid,comm").Output()
	if err != nil {
		return nil, err
	}

	// Regex to match claude CLI processes (not Claude.app)
	// Match: "claude" or paths ending in "/claude"
	claudePattern := regexp.MustCompile(`(?i)(^claude$|/claude$)`)

	// Pattern to exclude Claude.app and related desktop processes
	excludePattern := regexp.MustCompile(`(?i)(Claude\.app|claude-native|chrome-native)`)

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		// Check if command matches claude CLI
		cmd := strings.Join(fields[2:], " ")

		// Skip desktop app processes
		if excludePattern.MatchString(cmd) {
			continue
		}

		// Only match CLI claude processes
		if !claudePattern.MatchString(cmd) {
			continue
		}

		var pid, ppid int
		if _, err := fmt.Sscanf(fields[0], "%d", &pid); err != nil {
			continue
		}
		if _, err := fmt.Sscanf(fields[1], "%d", &ppid); err != nil {
			continue
		}

		procs = append(procs, processInfo{
			pid:  pid,
			ppid: ppid,
			cmd:  cmd,
		})
	}

	return procs, nil
}

// isOrphanProcess checks if a Claude process is orphaned.
// A process is orphaned if its parent (or ancestor) is not a tmux session.
func (c *OrphanProcessCheck) isOrphanProcess(proc processInfo, tmuxPIDs map[int]bool) bool {
	// Walk up the process tree looking for a tmux parent
	currentPPID := proc.ppid
	visited := make(map[int]bool)

	for currentPPID > 1 && !visited[currentPPID] {
		visited[currentPPID] = true

		// Check if this is a tmux process
		if tmuxPIDs[currentPPID] {
			return false // Has tmux ancestor, not orphaned
		}

		// Get parent's parent
		out, err := exec.Command("ps", "-p", fmt.Sprintf("%d", currentPPID), "-o", "ppid=").Output() //nolint:gosec // G204: PID is numeric from internal state
		if err != nil {
			break
		}

		var nextPPID int
		if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &nextPPID); err != nil {
			break
		}
		currentPPID = nextPPID
	}

	return true // No tmux ancestor found
}
