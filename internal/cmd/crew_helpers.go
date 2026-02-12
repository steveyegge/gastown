package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/rpcclient"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/terminal"
	"github.com/steveyegge/gastown/internal/tmux"
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

// newDaemonRPCClient creates an RPC client from the BD_DAEMON_HOST and
// BD_DAEMON_TOKEN environment variables. Returns nil if not in daemon mode.
// This detects K8s pod mode where BD_DAEMON_HOST is set by the controller.
func newDaemonRPCClient() *rpcclient.Client {
	if !beads.IsDaemonMode() {
		return nil
	}
	host := os.Getenv("BD_DAEMON_HOST")
	token := os.Getenv("BD_DAEMON_TOKEN")
	townName := os.Getenv("GT_TOWN")
	var opts []rpcclient.Option
	if token != "" {
		opts = append(opts, rpcclient.WithAPIKey(token))
	}
	if townName != "" {
		opts = append(opts, rpcclient.WithTownName(townName))
	}
	return rpcclient.NewClient(host, opts...)
}

// newConnectedDaemonClient creates an RPC client from either BD_DAEMON_HOST
// env var (K8s pods) or the global daemon config written by "gt connect"
// (local users connected to a remote daemon). Returns nil if no remote daemon
// is configured.
func newConnectedDaemonClient() *rpcclient.Client {
	// Check env var first (K8s pod mode).
	if client := newDaemonRPCClient(); client != nil {
		return client
	}

	// Check global daemon config (gt connect mode).
	cfg, err := readGlobalDaemonConfigFull()
	if err != nil || cfg.DaemonHost == "" {
		return nil
	}
	var opts []rpcclient.Option
	if cfg.DaemonToken != "" {
		opts = append(opts, rpcclient.WithAPIKey(cfg.DaemonToken))
	}
	if cfg.TownName != "" {
		opts = append(opts, rpcclient.WithTownName(cfg.TownName))
	}
	return rpcclient.NewClient(cfg.DaemonHost, opts...)
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
		return nil, fmt.Errorf("not inside a crew workspace - specify the crew name or cd into a crew directory (e.g., gastown/crew/max)")
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
	runtimeConfig := config.LoadRuntimeConfig(rigPath)
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

// isInTmuxSession checks if we're currently inside the target tmux session.
func isInTmuxSession(targetSession string) bool {
	// TMUX env var format: /tmp/tmux-501/default,12345,0
	// We need to get the current session name via tmux display-message
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		return false // Not in tmux at all
	}

	// Get current session name
	currentSession, err := tmux.NewTmux().GetCurrentSessionName()
	if err != nil {
		return false
	}

	return currentSession == targetSession
}

// attachToTmuxSession attaches to a tmux session.
// If already inside tmux, uses switch-client instead of attach-session.
func attachToTmuxSession(sessionID string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	// Check if we're already inside a tmux session
	var cmd *exec.Cmd
	if os.Getenv("TMUX") != "" {
		// Inside tmux: switch to the target session
		cmd = exec.Command(tmuxPath, "switch-client", "-t", sessionID)
	} else {
		// Outside tmux: attach to the session
		cmd = exec.Command(tmuxPath, "attach-session", "-t", sessionID)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// attachToCoopPod attaches to a K8s pod running coop via port-forward.
// It starts kubectl port-forward in the background, waits for coop to respond,
// then execs into `coop attach` which takes over the terminal.
func attachToCoopPod(podName, namespace string) error {
	return attachToCoopPodWithBrowser(podName, namespace, false)
}

// attachToCoopPodWithBrowser attaches to a K8s pod via port-forward.
// If browser is true, opens the web terminal in a browser instead of using coop attach.
func attachToCoopPodWithBrowser(podName, namespace string, browser bool) error {
	// Create and open the coop connection (port-forward).
	conn := terminal.NewCoopPodConnection(terminal.CoopPodConnectionConfig{
		PodName:   podName,
		Namespace: namespace,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := conn.Open(ctx); err != nil {
		return fmt.Errorf("connecting to pod: %w", err)
	}
	defer conn.Close()

	localURL := conn.LocalURL()
	fmt.Printf("  Port-forward: localhost:%d → %s:8080\n", conn.LocalPort(), podName)

	if browser {
		return openBrowserAndBlock(localURL)
	}

	coopPath, err := findCoopBinary()
	if err != nil {
		return err
	}

	fmt.Printf("  Detach: Ctrl+]\n\n")

	// Exec into coop attach — replaces this process.
	return syscall.Exec(coopPath, []string{"coop", "attach", localURL}, os.Environ())
}

// openBrowserAndBlock opens the given URL in a browser and blocks until interrupted.
func openBrowserAndBlock(localURL string) error {
	opener := "xdg-open"
	if _, err := exec.LookPath("open"); err == nil {
		opener = "open"
	}
	fmt.Printf("  Opening %s\n", localURL)
	openCmd := exec.Command(opener, localURL)
	if err := openCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  Failed to open browser: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Open manually: %s\n", localURL)
	}
	fmt.Fprintf(os.Stderr, "  Press Ctrl+C to stop port-forward\n")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	return nil
}

// findCoopBinary locates the coop binary on the system.
func findCoopBinary() (string, error) {
	// Check PATH first.
	if p, err := exec.LookPath("coop"); err == nil {
		return p, nil
	}

	// Check common build locations.
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, rel := range []string{
			"coop/target/release/coop",
			"coop/target/debug/coop",
		} {
			p := filepath.Join(home, rel)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("coop binary not found. Install from ~/coop with: cargo build --release")
}

// ensureDefaultBranch checks if a git directory is on the default branch.
// If not, warns the user and offers to switch.
// Returns true if on default branch (or switched to it), false if user declined.
// The rigPath parameter is used to look up the configured default branch.
func ensureDefaultBranch(dir, roleName, rigPath string) bool { //nolint:unparam // bool return kept for future callers to check
	g := git.NewGit(dir)

	branch, err := g.CurrentBranch()
	if err != nil {
		// Not a git repo or other error, skip check
		return true
	}

	// Get configured default branch for this rig
	defaultBranch := "main" // fallback
	if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	if branch == defaultBranch || branch == "master" {
		return true
	}

	// Warn about wrong branch
	fmt.Printf("\n%s %s is on branch '%s', not %s\n",
		style.Warning.Render("⚠"),
		roleName,
		branch,
		defaultBranch)
	fmt.Printf("  Persistent roles should work on %s to avoid orphaned work.\n", defaultBranch)
	fmt.Println()

	// Auto-switch to default branch
	fmt.Printf("  Switching to %s...\n", defaultBranch)
	if err := g.Checkout(defaultBranch); err != nil {
		fmt.Printf("  %s Could not switch to %s: %v\n", style.Error.Render("✗"), defaultBranch, err)
		fmt.Printf("  Please manually run: git checkout %s && git pull\n", defaultBranch)
		return false
	}

	// Pull latest
	if err := g.Pull("origin", defaultBranch); err != nil {
		fmt.Printf("  %s Pull failed (continuing anyway): %v\n", style.Warning.Render("⚠"), err)
	} else {
		fmt.Printf("  %s Switched to %s and pulled latest\n", style.Success.Render("✓"), defaultBranch)
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
// Uses SessionRegistry to find sessions matching gt-<rig>-crew-* pattern.
func findRigCrewSessions(rigName string) ([]string, error) { //nolint:unparam // error return kept for future use
	townRoot, _ := workspace.FindFromCwd()
	allSessions := discoverSessionNames(townRoot)

	prefix := fmt.Sprintf("gt-%s-crew-", rigName)
	var sessions []string

	for _, line := range allSessions {
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
