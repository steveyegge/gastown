package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// inferRigFromCwd tries to determine the rig from the current directory.
func inferRigFromCwd(townRoot string) (string, error) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}

	// Check if cwd is within a rig
	rel, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return "", fmt.Errorf("not in workspace")
	}

	// Normalize and split path - first component is the rig name
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")

	if len(parts) > 0 && parts[0] != "" && parts[0] != "." {
		return parts[0], nil
	}

	return "", fmt.Errorf("could not infer rig from current directory")
}

// getCrewManager returns a crew manager for the specified or inferred rig.
func getCrewManager(rigName string) (*crew.Manager, *rig.Rig, error) {
	// Handle optional rig inference from cwd
	if rigName == "" {
		townRoot, err := workspace.FindFromCwdOrError()
		if err != nil {
			return nil, nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
		}
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return nil, nil, fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	_, r, err := getRig(rigName)
	if err != nil {
		return nil, nil, err
	}

	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	return crewMgr, r, nil
}

// crewSessionName generates the tmux session name for a crew worker.
func crewSessionName(rigName, crewName string) string {
	return fmt.Sprintf("gt-%s-crew-%s", rigName, crewName)
}

// parseRigSlashName parses "rig/name" format into separate rig and name parts.
// Returns (rig, name, true) if the format matches, or ("", original, false) if not.
// Examples:
//   - "beads/emma" -> ("beads", "emma", true)
//   - "emma" -> ("", "emma", false)
//   - "beads/crew/emma" -> ("beads", "crew/emma", true) - only first slash splits
func parseRigSlashName(input string) (rig, name string, ok bool) {
	// Only split on first slash to handle edge cases
	idx := strings.Index(input, "/")
	if idx == -1 {
		return "", input, false
	}
	return input[:idx], input[idx+1:], true
}

// crewDetection holds the result of detecting crew workspace from cwd.
type crewDetection struct {
	rigName  string
	crewName string
}

// detectCrewFromCwd attempts to detect the crew workspace from the current directory.
// It looks for the pattern <town>/<rig>/crew/<name>/ in the current path.
func detectCrewFromCwd() (*crewDetection, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting cwd: %w", err)
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil, fmt.Errorf("not in Gas Town workspace: %w", err)
	}
	if townRoot == "" {
		return nil, fmt.Errorf("not in Gas Town workspace")
	}

	// Get relative path from town root
	relPath, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return nil, fmt.Errorf("getting relative path: %w", err)
	}

	// Normalize and split path
	relPath = filepath.ToSlash(relPath)
	parts := strings.Split(relPath, "/")

	// Look for pattern: <rig>/crew/<name>/...
	// Minimum: rig, crew, name = 3 parts
	if len(parts) < 3 {
		return nil, fmt.Errorf("not in a crew workspace (path too short)")
	}

	rigName := parts[0]
	if parts[1] != "crew" {
		return nil, fmt.Errorf("not in a crew workspace (not in crew/ directory)")
	}
	crewName := parts[2]

	return &crewDetection{
		rigName:  rigName,
		crewName: crewName,
	}, nil
}

// isShellCommand checks if the command is a shell (meaning Claude has exited).
func isShellCommand(cmd string) bool {
	shells := constants.SupportedShells
	for _, shell := range shells {
		if cmd == shell {
			return true
		}
	}
	return false
}

// execClaude execs claude, replacing the current process.
// Used when we're already in the target session and just need to start Claude.
// If prompt is provided, it's passed as the initial prompt to Claude.
func execClaude(prompt string) error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found: %w", err)
	}

	// exec replaces current process with claude
	args := []string{"claude", "--dangerously-skip-permissions"}
	if prompt != "" {
		args = append(args, prompt)
	}
	return syscall.Exec(claudePath, args, os.Environ())
}

// isInTmuxSession checks if we're currently inside the target tmux session.
func isInTmuxSession(targetSession string) bool {
	// TMUX env var format: /tmp/tmux-501/default,12345,0
	// We need to get the current session name via tmux display-message
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return false // Not in tmux at all
	}

	// Get current session name
	cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	currentSession := strings.TrimSpace(string(out))
	return currentSession == targetSession
}

// attachToTmuxSession attaches to a tmux session.
// Should only be called from outside tmux.
func attachToTmuxSession(sessionID string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	cmd := exec.Command(tmuxPath, "attach-session", "-t", sessionID)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ensureMainBranch checks if a git directory is on main branch.
// If not, warns the user and offers to switch.
// Returns true if on main (or switched to main), false if user declined.
func ensureMainBranch(dir, roleName string) bool { //nolint:unparam // bool return kept for future callers to check
	g := git.NewGit(dir)

	branch, err := g.CurrentBranch()
	if err != nil {
		// Not a git repo or other error, skip check
		return true
	}

	if branch == "main" || branch == "master" {
		return true
	}

	// Warn about wrong branch
	fmt.Printf("\n%s %s is on branch '%s', not main\n",
		style.Warning.Render("⚠"),
		roleName,
		branch)
	fmt.Println("  Persistent roles should work on main to avoid orphaned work.")
	fmt.Println()

	// Auto-switch to main
	fmt.Printf("  Switching to main...\n")
	if err := g.Checkout("main"); err != nil {
		fmt.Printf("  %s Could not switch to main: %v\n", style.Error.Render("✗"), err)
		fmt.Println("  Please manually run: git checkout main && git pull")
		return false
	}

	// Pull latest
	if err := g.Pull("origin", "main"); err != nil {
		fmt.Printf("  %s Pull failed (continuing anyway): %v\n", style.Warning.Render("⚠"), err)
	} else {
		fmt.Printf("  %s Switched to main and pulled latest\n", style.Success.Render("✓"))
	}

	return true
}

// parseCrewSessionName extracts rig and crew name from a tmux session name.
// Format: gt-<rig>-crew-<name>
// Returns empty strings and false if the format doesn't match.
func parseCrewSessionName(sessionName string) (rigName, crewName string, ok bool) {
	// Must start with "gt-" and contain "-crew-"
	if !strings.HasPrefix(sessionName, "gt-") {
		return "", "", false
	}

	// Remove "gt-" prefix
	rest := sessionName[3:]

	// Find "-crew-" separator
	idx := strings.Index(rest, "-crew-")
	if idx == -1 {
		return "", "", false
	}

	rigName = rest[:idx]
	crewName = rest[idx+6:] // len("-crew-") = 6

	if rigName == "" || crewName == "" {
		return "", "", false
	}

	return rigName, crewName, true
}

// findRigCrewSessions returns all crew sessions for a given rig, sorted alphabetically.
// Uses tmux list-sessions to find sessions matching gt-<rig>-crew-* pattern.
func findRigCrewSessions(rigName string) ([]string, error) { //nolint:unparam // error return kept for future use
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// No tmux server or no sessions
		return nil, nil
	}

	prefix := fmt.Sprintf("gt-%s-crew-", rigName)
	var sessions []string

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, prefix) {
			sessions = append(sessions, line)
		}
	}

	// Sessions are already sorted by tmux, but sort explicitly for consistency
	// (alphabetical by session name means alphabetical by crew name)
	return sessions, nil
}
