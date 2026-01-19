//go:build !windows

package util

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// minOrphanAge is the minimum age (in seconds) a process must be before
// we consider it orphaned. This prevents race conditions with newly spawned
// processes and avoids killing legitimate short-lived subagents.
const minOrphanAge = 60

// getGasTownSessionPIDs returns a set of PIDs belonging to valid Gas Town tmux sessions.
// This prevents killing Claude processes that are part of witness/refinery/deacon sessions
// even if they temporarily show TTY "?" during startup or session transitions.
func getGasTownSessionPIDs() map[int]bool {
	pids := make(map[int]bool)

	// Get list of Gas Town tmux sessions (gt-* and hq-*)
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").Output()
	if err != nil {
		return pids // tmux not available or no sessions
	}

	var gasTownSessions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "gt-") || strings.HasPrefix(line, "hq-") {
			gasTownSessions = append(gasTownSessions, line)
		}
	}

	// For each Gas Town session, get the PIDs of processes in its panes
	for _, session := range gasTownSessions {
		out, err := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_pid}").Output()
		if err != nil {
			continue
		}
		for _, pidStr := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
				pids[pid] = true
				// Also add child processes of the pane shell
				addChildPIDs(pid, pids)
			}
		}
	}

	return pids
}

// addChildPIDs adds all descendant PIDs of a process to the set.
// This catches Claude processes spawned by the shell in a tmux pane.
func addChildPIDs(parentPID int, pids map[int]bool) {
	// Use pgrep to find children (more reliable than parsing ps output)
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		return
	}
	for _, pidStr := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			pids[pid] = true
			// Recurse to get grandchildren
			addChildPIDs(pid, pids)
		}
	}
}

// sigkillGracePeriod is how long (in seconds) we wait after sending SIGTERM
// before escalating to SIGKILL. If a process was sent SIGTERM and is still
// around after this period, we use SIGKILL on the next cleanup cycle.
const sigkillGracePeriod = 60

// orphanStateFile returns the path to the state file that tracks PIDs we've
// sent signals to. Uses $XDG_RUNTIME_DIR if available, otherwise /tmp.
func orphanStateFile() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, "gastown-orphan-state")
}

// signalState tracks what signal was last sent to a PID and when.
type signalState struct {
	Signal    string    // "SIGTERM" or "SIGKILL"
	Timestamp time.Time // When the signal was sent
}

// loadOrphanState reads the state file and returns the current signal state
// for each tracked PID. Automatically cleans up entries for dead processes.
func loadOrphanState() map[int]signalState {
	state := make(map[int]signalState)

	f, err := os.Open(orphanStateFile())
	if err != nil {
		return state // File doesn't exist yet, that's fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) != 3 {
			continue
		}
		pid, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		sig := parts[1]
		ts, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}

		// Only keep if process still exists
		if err := syscall.Kill(pid, 0); err == nil || err == syscall.EPERM {
			state[pid] = signalState{Signal: sig, Timestamp: time.Unix(ts, 0)}
		}
	}

	return state
}

// saveOrphanState writes the current signal state to the state file.
func saveOrphanState(state map[int]signalState) error {
	f, err := os.Create(orphanStateFile())
	if err != nil {
		return err
	}
	defer f.Close()

	for pid, s := range state {
		fmt.Fprintf(f, "%d %s %d\n", pid, s.Signal, s.Timestamp.Unix())
	}
	return nil
}

// processExists checks if a process is still running.
func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// parseEtime parses ps etime format into seconds.
// Format: [[DD-]HH:]MM:SS
// Examples: "01:23" (83s), "01:02:03" (3723s), "2-01:02:03" (176523s)
func parseEtime(etime string) (int, error) {
	var days, hours, minutes, seconds int

	// Check for days component (DD-HH:MM:SS)
	if idx := strings.Index(etime, "-"); idx != -1 {
		d, err := strconv.Atoi(etime[:idx])
		if err != nil {
			return 0, fmt.Errorf("parsing days: %w", err)
		}
		days = d
		etime = etime[idx+1:]
	}

	// Split remaining by colons
	parts := strings.Split(etime, ":")
	switch len(parts) {
	case 2: // MM:SS
		m, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("parsing minutes: %w", err)
		}
		s, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("parsing seconds: %w", err)
		}
		minutes, seconds = m, s
	case 3: // HH:MM:SS
		h, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("parsing hours: %w", err)
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("parsing minutes: %w", err)
		}
		s, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, fmt.Errorf("parsing seconds: %w", err)
		}
		hours, minutes, seconds = h, m, s
	default:
		return 0, fmt.Errorf("unexpected etime format: %s", etime)
	}

	return days*86400 + hours*3600 + minutes*60 + seconds, nil
}

// OrphanedProcess represents a claude process running without a controlling terminal.
type OrphanedProcess struct {
	PID int
	Cmd string
	Age int // Age in seconds
}

// FindOrphanedClaudeProcesses finds claude/codex processes without a controlling terminal.
// These are typically subagent processes spawned by Claude Code's Task tool that didn't
// clean up properly after completion.
//
// Detection is based on TTY column: processes with TTY "?" have no controlling terminal.
// This is safer than process tree walking because:
// - Legitimate terminal sessions always have a TTY (pts/*)
// - Orphaned subagents have no TTY (?)
// - Won't accidentally kill user's personal claude instances in terminals
//
// Additionally, processes must be older than minOrphanAge seconds to be considered
// orphaned. This prevents race conditions with newly spawned processes.
func FindOrphanedClaudeProcesses() ([]OrphanedProcess, error) {
	// Get PIDs belonging to valid Gas Town tmux sessions.
	// These should not be killed even if they show TTY "?" during startup.
	gasTownPIDs := getGasTownSessionPIDs()

	// Use ps to get PID, TTY, command, and elapsed time for all processes
	// TTY "?" indicates no controlling terminal
	// etime is elapsed time in [[DD-]HH:]MM:SS format (portable across Linux/macOS)
	out, err := exec.Command("ps", "-eo", "pid,tty,comm,etime").Output()
	if err != nil {
		return nil, fmt.Errorf("listing processes: %w", err)
	}

	var orphans []OrphanedProcess
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue // Header line or invalid PID
		}

		tty := fields[1]
		cmd := fields[2]
		etimeStr := fields[3]

		// Only look for claude/codex processes without a TTY
		// Linux shows "?" for no TTY, macOS shows "??"
		if tty != "?" && tty != "??" {
			continue
		}

		// Match claude or codex command names
		cmdLower := strings.ToLower(cmd)
		if cmdLower != "claude" && cmdLower != "claude-code" && cmdLower != "codex" {
			continue
		}

		// Skip processes that belong to valid Gas Town tmux sessions.
		// This prevents killing witnesses/refineries/deacon during startup
		// when they may temporarily show TTY "?".
		if gasTownPIDs[pid] {
			continue
		}

		// Skip processes younger than minOrphanAge seconds
		// This prevents killing newly spawned subagents and reduces false positives
		age, err := parseEtime(etimeStr)
		if err != nil {
			continue
		}
		if age < minOrphanAge {
			continue
		}

		orphans = append(orphans, OrphanedProcess{
			PID: pid,
			Cmd: cmd,
			Age: age,
		})
	}

	return orphans, nil
}

// CleanupResult describes what happened to an orphaned process.
type CleanupResult struct {
	Process OrphanedProcess
	Signal  string // "SIGTERM", "SIGKILL", or "UNKILLABLE"
	Error   error
}

// CleanupOrphanedClaudeProcesses finds and kills orphaned claude/codex processes.
//
// Uses a state machine to escalate signals:
//  1. First encounter → SIGTERM, record in state file
//  2. Next cycle, still alive after grace period → SIGKILL, update state
//  3. Next cycle, still alive after SIGKILL → log as unkillable, remove from state
//
// Returns the list of cleanup results and any error encountered.
func CleanupOrphanedClaudeProcesses() ([]CleanupResult, error) {
	orphans, err := FindOrphanedClaudeProcesses()
	if err != nil {
		return nil, err
	}

	// Load previous state
	state := loadOrphanState()
	now := time.Now()

	var results []CleanupResult
	var lastErr error

	// Track which PIDs we're still working on
	activeOrphans := make(map[int]bool)
	for _, o := range orphans {
		activeOrphans[o.PID] = true
	}

	// First pass: check state for PIDs that died (cleanup) or need escalation
	for pid, s := range state {
		if !activeOrphans[pid] {
			// Process died, remove from state
			delete(state, pid)
			continue
		}

		// Process still alive - check if we need to escalate
		elapsed := now.Sub(s.Timestamp).Seconds()

		if s.Signal == "SIGKILL" {
			// Already sent SIGKILL and it's still alive - unkillable
			results = append(results, CleanupResult{
				Process: OrphanedProcess{PID: pid, Cmd: "claude"},
				Signal:  "UNKILLABLE",
				Error:   fmt.Errorf("process %d survived SIGKILL", pid),
			})
			delete(state, pid) // Remove from tracking, nothing more we can do
			delete(activeOrphans, pid)
			continue
		}

		if s.Signal == "SIGTERM" && elapsed >= float64(sigkillGracePeriod) {
			// Sent SIGTERM but still alive after grace period - escalate to SIGKILL
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
				if err != syscall.ESRCH {
					lastErr = fmt.Errorf("SIGKILL PID %d: %w", pid, err)
				}
				delete(state, pid)
				delete(activeOrphans, pid)
				continue
			}
			state[pid] = signalState{Signal: "SIGKILL", Timestamp: now}
			results = append(results, CleanupResult{
				Process: OrphanedProcess{PID: pid, Cmd: "claude"},
				Signal:  "SIGKILL",
			})
			delete(activeOrphans, pid)
		}
		// If SIGTERM was recent, leave it alone - check again next cycle
	}

	// Second pass: send SIGTERM to new orphans not yet in state
	for _, orphan := range orphans {
		if !activeOrphans[orphan.PID] {
			continue // Already handled above
		}
		if _, exists := state[orphan.PID]; exists {
			continue // Already in state, waiting for grace period
		}

		// New orphan - send SIGTERM
		if err := syscall.Kill(orphan.PID, syscall.SIGTERM); err != nil {
			if err != syscall.ESRCH {
				lastErr = fmt.Errorf("SIGTERM PID %d: %w", orphan.PID, err)
			}
			continue
		}
		state[orphan.PID] = signalState{Signal: "SIGTERM", Timestamp: now}
		results = append(results, CleanupResult{
			Process: orphan,
			Signal:  "SIGTERM",
		})
	}

	// Save updated state
	if err := saveOrphanState(state); err != nil {
		if lastErr == nil {
			lastErr = fmt.Errorf("saving orphan state: %w", err)
		}
	}

	return results, lastErr
}
