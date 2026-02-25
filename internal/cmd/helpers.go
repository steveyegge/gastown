package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
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

// parseRigSlashName parses "rig/name" format into separate rig and name parts.
// Returns (rig, name, true) if the format matches, or ("", original, false) if not.
// Examples:
//   - "beads/emma" -> ("beads", "emma", true)
//   - "emma" -> ("", "emma", false)
//   - "beads/crew/emma" -> ("beads", "crew/emma", true) - only first slash splits
func parseRigSlashName(input string) (rigName, name string, ok bool) {
	// Only split on first slash to handle edge cases
	idx := strings.Index(input, "/")
	if idx == -1 {
		return "", input, false
	}
	return input[:idx], input[idx+1:], true
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
	cmd := tmux.BuildCommand("display-message", "-p", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	currentSession := strings.TrimSpace(string(out))
	return currentSession == targetSession
}

// isInSameTmuxSocket checks if we're inside a tmux session on the same socket
// as the current town. Used to decide between switch-client and attach-session.
func isInSameTmuxSocket() bool {
	return tmux.IsInSameSocket()
}

// attachToTmuxSession attaches to a tmux session.
// If already inside tmux, uses switch-client instead of attach-session.
// Uses syscall.Exec to replace the Go process with tmux for direct terminal
// control, and passes -u for UTF-8 support regardless of locale settings.
// See: https://github.com/steveyegge/gastown/issues/1219
func attachToTmuxSession(sessionID string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	// Base args with UTF-8 and socket support
	baseArgs := []string{"tmux", "-u"}
	if socket := tmux.GetDefaultSocket(); socket != "" {
		baseArgs = append(baseArgs, "-L", socket)
	}

	var args []string
	if isInSameTmuxSocket() {
		// Same tmux socket: switch to the target session
		args = append(baseArgs, "switch-client", "-t", sessionID)
	} else {
		// Outside tmux or different socket: attach to the session
		args = append(baseArgs, "attach-session", "-t", sessionID)
	}

	// Replace the Go process with tmux for direct terminal control
	return syscall.Exec(tmuxPath, args, os.Environ())
}

// isShellCommand checks if the command is a shell (meaning the runtime has exited).
func isShellCommand(cmd string) bool {
	shells := constants.SupportedShells
	for _, shell := range shells {
		if cmd == shell {
			return true
		}
	}
	return false
}

// execAgent execs the configured agent, replacing the current process.
// Used when we're already in the target session and just need to start the agent.
// If prompt is provided, it's passed as the initial prompt.
func execAgent(cfg *config.RuntimeConfig, prompt string) error {
	if cfg == nil {
		cfg = config.DefaultRuntimeConfig()
	}

	agentPath, err := exec.LookPath(cfg.Command)
	if err != nil {
		return fmt.Errorf("%s not found: %w", cfg.Command, err)
	}

	// exec replaces current process with agent
	// args[0] must be the command name (convention for exec)
	args := append([]string{cfg.Command}, cfg.Args...)
	if prompt != "" {
		args = append(args, prompt)
	}
	return syscall.Exec(agentPath, args, os.Environ())
}

// execRuntime execs the runtime CLI, replacing the current process.
// Used when we're already in the target session and just need to start the runtime.
// If prompt is provided, it's passed according to the runtime's prompt mode.
func execRuntime(prompt, rigPath, configDir string) error {
	townRoot := filepath.Dir(rigPath)
	runtimeConfig := config.ResolveRoleAgentConfig("crew", townRoot, rigPath)
	args := runtimeConfig.BuildArgsWithPrompt(prompt)
	if len(args) == 0 {
		return fmt.Errorf("runtime command not configured")
	}

	binPath, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("runtime command not found: %w", err)
	}

	env := os.Environ()
	if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && configDir != "" {
		env = append(env, fmt.Sprintf("%s=%s", runtimeConfig.Session.ConfigDirEnv, configDir))
	}

	return syscall.Exec(binPath, args, env)
}

// ensureDefaultBranch checks if a git directory is on the default branch.
// ensureDefaultBranch checks out the configured default branch and pulls latest.
// Returns an error if the checkout or pull fails.
func ensureDefaultBranch(dir, roleName, rigPath string) error {
	g := git.NewGit(dir)

	branch, err := g.CurrentBranch()
	if err != nil {
		// Not a git repo or other error, skip check
		return fmt.Errorf("could not determine current branch: %w", err)
	}

	// Get configured default branch for this rig
	defaultBranch := "main" // fallback
	if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	if branch == defaultBranch {
		// Already on default branch — still pull to ensure up-to-date
		if err := g.Pull("origin", defaultBranch); err != nil {
			return fmt.Errorf("pull failed on %s: %w", defaultBranch, err)
		}
		fmt.Printf("  %s Already on %s, pulled latest\n", style.Success.Render("✓"), defaultBranch)
		return nil
	}

	// Not on default branch — switch to it
	fmt.Printf("  %s is on branch '%s', switching to %s...\n", roleName, branch, defaultBranch)
	if err := g.Checkout(defaultBranch); err != nil {
		return fmt.Errorf("could not switch to %s: %w", defaultBranch, err)
	}

	// Pull latest
	if err := g.Pull("origin", defaultBranch); err != nil {
		return fmt.Errorf("pull failed on %s: %w", defaultBranch, err)
	}
	fmt.Printf("  %s Switched to %s and pulled latest\n", style.Success.Render("✓"), defaultBranch)

	return nil
}

// warnIfNotDefaultBranch prints a warning if the workspace is not on the
// configured default branch. Used when --reset is not set to alert users
// before an agent wastes its context window on a branch that can't push.
func warnIfNotDefaultBranch(dir, roleName, rigPath string) {
	g := git.NewGit(dir)

	branch, err := g.CurrentBranch()
	if err != nil {
		return
	}

	defaultBranch := "main"
	if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	if branch == defaultBranch {
		return
	}

	fmt.Printf("\n%s %s is on branch '%s', not '%s'.\n",
		style.Warning.Render("⚠"),
		roleName,
		branch,
		defaultBranch)
	fmt.Printf("  Use --reset to switch to %s, or continue at your own risk.\n\n", defaultBranch)
}
