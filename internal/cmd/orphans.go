package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var orphansCmd = &cobra.Command{
	Use:     "orphans",
	GroupID: GroupWork,
	Short:   "Find lost polecat work",
	Long: `Find orphaned commits that were never merged to main.

Polecat work can get lost when:
- Session killed before merge
- Refinery fails to process
- Network issues during push

This command uses 'git fsck --unreachable' to find dangling commits,
filters to recent ones, and shows details to help recovery.

Examples:
  gt orphans              # Last 7 days (default)
  gt orphans --days=14    # Last 2 weeks
  gt orphans --all        # Show all orphans (no date filter)`,
	RunE: runOrphans,
}

var (
	orphansDays int
	orphansAll  bool

	// Kill commits command flags
	orphansKillDryRun bool
	orphansKillDays   int
	orphansKillAll    bool
	orphansKillForce  bool

	// Process orphan flags
	orphansProcsForce bool
)

// Commit orphan kill command
var orphansKillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Remove orphaned commits permanently",
	Long: `Remove orphaned commits by running git garbage collection.

This command finds orphaned commits and then runs 'git gc --prune=now'
to permanently delete unreachable objects from the repository.

WARNING: This operation is irreversible. Once commits are pruned,
they cannot be recovered.

The command will:
1. Find orphaned commits (same as 'gt orphans')
2. Show what will be removed
3. Ask for confirmation (unless --force)
4. Run git gc --prune=now

Examples:
  gt orphans kill              # Kill orphans from last 7 days (default)
  gt orphans kill --days=14    # Kill orphans from last 2 weeks
  gt orphans kill --all        # Kill all orphans
  gt orphans kill --dry-run    # Preview without deleting
  gt orphans kill --force      # Skip confirmation prompt`,
	RunE: runOrphansKill,
}

// Process orphan commands
var orphansProcsCmd = &cobra.Command{
	Use:   "procs",
	Short: "Manage orphaned Claude processes",
	Long: `Find and kill Claude processes that have become orphaned (PPID=1).

These are processes that survived session termination and are now
parented to init/launchd. They consume resources and should be killed.

Examples:
  gt orphans procs        # List orphaned Claude processes
  gt orphans procs list   # Same as above
  gt orphans procs kill   # Kill orphaned processes`,
	RunE: runOrphansListProcesses, // Default to list
}

var orphansProcsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List orphaned Claude processes",
	Long: `List Claude processes that have become orphaned (PPID=1).

These are processes that survived session termination and are now
parented to init/launchd. They consume resources and should be killed.

Excludes:
- tmux server processes
- Claude.app desktop application processes

Examples:
  gt orphans procs list      # Show all orphan Claude processes`,
	RunE: runOrphansListProcesses,
}

var orphansProcsKillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Kill orphaned Claude processes",
	Long: `Kill Claude processes that have become orphaned (PPID=1).

Without flags, prompts for confirmation before killing.
Use -f/--force to kill without confirmation.

Examples:
  gt orphans procs kill      # Kill with confirmation
  gt orphans procs kill -f   # Force kill without confirmation`,
	RunE: runOrphansKillProcesses,
}

func init() {
	orphansCmd.Flags().IntVar(&orphansDays, "days", 7, "Show orphans from last N days")
	orphansCmd.Flags().BoolVar(&orphansAll, "all", false, "Show all orphans (no date filter)")

	// Kill commits command flags
	orphansKillCmd.Flags().BoolVar(&orphansKillDryRun, "dry-run", false, "Preview without deleting")
	orphansKillCmd.Flags().IntVar(&orphansKillDays, "days", 7, "Kill orphans from last N days")
	orphansKillCmd.Flags().BoolVar(&orphansKillAll, "all", false, "Kill all orphans (no date filter)")
	orphansKillCmd.Flags().BoolVar(&orphansKillForce, "force", false, "Skip confirmation prompt")

	// Process orphan kill command flags
	orphansProcsKillCmd.Flags().BoolVarP(&orphansProcsForce, "force", "f", false, "Kill without confirmation")

	// Wire up subcommands
	orphansProcsCmd.AddCommand(orphansProcsListCmd)
	orphansProcsCmd.AddCommand(orphansProcsKillCmd)

	orphansCmd.AddCommand(orphansKillCmd)
	orphansCmd.AddCommand(orphansProcsCmd)

	rootCmd.AddCommand(orphansCmd)
}

// OrphanCommit represents an unreachable commit
type OrphanCommit struct {
	SHA     string
	Date    time.Time
	Author  string
	Subject string
}

func runOrphans(cmd *cobra.Command, args []string) error {
	// Find workspace to determine rig root
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find current rig
	rigName, r, err := findCurrentRig(townRoot)
	if err != nil {
		return fmt.Errorf("determining rig: %w", err)
	}

	// We need to run from the mayor's clone (main git repo for the rig)
	mayorPath := r.Path + "/mayor/rig"

	fmt.Printf("Scanning for orphaned commits in %s...\n\n", rigName)

	// Run git fsck
	orphans, err := findOrphanCommits(mayorPath)
	if err != nil {
		return fmt.Errorf("finding orphans: %w", err)
	}

	if len(orphans) == 0 {
		fmt.Printf("%s No orphaned commits found\n", style.Bold.Render("✓"))
		return nil
	}

	// Filter by date unless --all
	cutoff := time.Now().AddDate(0, 0, -orphansDays)
	var filtered []OrphanCommit

	for _, o := range orphans {
		if orphansAll || o.Date.After(cutoff) {
			filtered = append(filtered, o)
		}
	}

	if len(filtered) == 0 {
		fmt.Printf("%s No orphaned commits in the last %d days\n", style.Bold.Render("✓"), orphansDays)
		fmt.Printf("%s Use --days=N or --all to see older orphans\n", style.Dim.Render("Hint:"))
		return nil
	}

	// Display results
	fmt.Printf("%s Found %d orphaned commit(s):\n\n", style.Warning.Render("⚠"), len(filtered))

	for _, o := range filtered {
		age := formatAge(o.Date)
		fmt.Printf("  %s %s\n", style.Bold.Render(o.SHA[:8]), o.Subject)
		fmt.Printf("    %s by %s\n\n", style.Dim.Render(age), o.Author)
	}

	// Recovery hints
	fmt.Printf("%s\n", style.Dim.Render("To recover a commit:"))
	fmt.Printf("%s\n", style.Dim.Render("  git cherry-pick <sha>     # Apply to current branch"))
	fmt.Printf("%s\n", style.Dim.Render("  git show <sha>            # View full commit"))
	fmt.Printf("%s\n", style.Dim.Render("  git branch rescue <sha>   # Create branch from commit"))

	return nil
}

// findOrphanCommits runs git fsck and parses orphaned commits
func findOrphanCommits(repoPath string) ([]OrphanCommit, error) {
	// Run git fsck to find unreachable objects
	fsckCmd := exec.Command("git", "fsck", "--unreachable", "--no-reflogs")
	fsckCmd.Dir = repoPath

	var fsckOut, fsckErr bytes.Buffer
	fsckCmd.Stdout = &fsckOut
	fsckCmd.Stderr = &fsckErr

	if err := fsckCmd.Run(); err != nil {
		// git fsck returns non-zero if there are issues, but we still get output
		// Only fail if we got no output at all
		if fsckOut.Len() == 0 {
			// Include stderr in error message for debugging
			errMsg := strings.TrimSpace(fsckErr.String())
			if errMsg != "" {
				return nil, fmt.Errorf("git fsck failed: %w (%s)", err, errMsg)
			}
			return nil, fmt.Errorf("git fsck failed: %w", err)
		}
	}

	// Parse commit SHAs from output
	var commitSHAs []string
	scanner := bufio.NewScanner(&fsckOut)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "unreachable commit <sha>"
		if strings.HasPrefix(line, "unreachable commit ") {
			sha := strings.TrimPrefix(line, "unreachable commit ")
			commitSHAs = append(commitSHAs, sha)
		}
	}

	if len(commitSHAs) == 0 {
		return nil, nil
	}

	// Get details for each commit
	var orphans []OrphanCommit
	for _, sha := range commitSHAs {
		commit, err := getCommitDetails(repoPath, sha)
		if err != nil {
			continue // Skip commits we can't parse
		}

		// Skip stash-like and routine sync commits
		if isNoiseCommit(commit.Subject) {
			continue
		}

		orphans = append(orphans, commit)
	}

	return orphans, nil
}

// getCommitDetails retrieves commit metadata
func getCommitDetails(repoPath, sha string) (OrphanCommit, error) {
	// Format: timestamp|author|subject
	cmd := exec.Command("git", "log", "-1", "--format=%at|%an|%s", sha)
	cmd.Dir = repoPath

	out, err := cmd.Output()
	if err != nil {
		return OrphanCommit{}, err
	}

	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 3)
	if len(parts) < 3 {
		return OrphanCommit{}, fmt.Errorf("unexpected format")
	}

	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return OrphanCommit{}, err
	}

	return OrphanCommit{
		SHA:     sha,
		Date:    time.Unix(timestamp, 0),
		Author:  parts[1],
		Subject: parts[2],
	}, nil
}

// isNoiseCommit returns true for stash-related or routine sync commits
func isNoiseCommit(subject string) bool {
	// Git stash creates commits with these prefixes
	noisePrefixes := []string{
		"WIP on ",
		"index on ",
		"On ",              // "On branch: message"
		"stash@{",          // Direct stash reference
		"untracked files ", // Stash with untracked
		"bd sync:",         // Beads sync commits (routine)
		"bd sync: ",        // Beads sync commits (routine)
	}

	for _, prefix := range noisePrefixes {
		if strings.HasPrefix(subject, prefix) {
			return true
		}
	}

	return false
}

// formatAge returns a human-readable age string
func formatAge(t time.Time) string {
	d := time.Since(t)

	if d < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

// runOrphansKill removes orphaned commits by running git gc
func runOrphansKill(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName, r, err := findCurrentRig(townRoot)
	if err != nil {
		return fmt.Errorf("determining rig: %w", err)
	}

	mayorPath := r.Path + "/mayor/rig"
	fmt.Printf("Scanning for orphaned commits in %s...\n\n", rigName)

	orphans, err := findOrphanCommits(mayorPath)
	if err != nil {
		return fmt.Errorf("finding orphans: %w", err)
	}

	if len(orphans) == 0 {
		fmt.Printf("%s No orphaned commits found\n", style.Bold.Render("✓"))
		return nil
	}

	cutoff := time.Now().AddDate(0, 0, -orphansKillDays)
	var filtered []OrphanCommit
	for _, o := range orphans {
		if orphansKillAll || o.Date.After(cutoff) {
			filtered = append(filtered, o)
		}
	}

	if len(filtered) == 0 {
		fmt.Printf("%s No orphaned commits in the last %d days\n", style.Bold.Render("✓"), orphansKillDays)
		fmt.Printf("%s Use --days=N or --all to target older orphans\n", style.Dim.Render("Hint:"))
		return nil
	}

	fmt.Printf("%s Found %d orphaned commit(s) to remove:\n\n", style.Warning.Render("⚠"), len(filtered))
	for _, o := range filtered {
		fmt.Printf("  %s %s\n", style.Bold.Render(o.SHA[:8]), o.Subject)
		fmt.Printf("    %s by %s\n\n", style.Dim.Render(formatAge(o.Date)), o.Author)
	}

	if orphansKillDryRun {
		fmt.Printf("%s Dry run - no changes made\n", style.Dim.Render("ℹ"))
		return nil
	}

	if !orphansKillForce {
		fmt.Printf("%s\n", style.Warning.Render("WARNING: This operation is irreversible!"))
		fmt.Printf("Remove %d orphaned commit(s)? [y/N] ", len(filtered))
		var response string
		_, _ = fmt.Scanln(&response)
		if strings.ToLower(strings.TrimSpace(response)) != "y" {
			fmt.Printf("%s Canceled\n", style.Dim.Render("ℹ"))
			return nil
		}
	}

	fmt.Printf("\nRunning git gc --prune=now...\n")
	gcCmd := exec.Command("git", "gc", "--prune=now")
	gcCmd.Dir = mayorPath
	gcCmd.Stdout = os.Stdout
	gcCmd.Stderr = os.Stderr
	if err := gcCmd.Run(); err != nil {
		return fmt.Errorf("git gc failed: %w", err)
	}

	fmt.Printf("\n%s Removed %d orphaned commit(s)\n", style.Bold.Render("✓"), len(filtered))
	return nil
}

// OrphanProcess represents a Claude process that has become orphaned (PPID=1)
type OrphanProcess struct {
	PID  int
	Args string
}

// findOrphanProcesses finds Claude processes with PPID=1 (orphaned)
func findOrphanProcesses() ([]OrphanProcess, error) {
	// Run ps to get all processes with PID, PPID, and args
	cmd := exec.Command("ps", "-eo", "pid,ppid,args")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running ps: %w", err)
	}

	var orphans []OrphanProcess
	scanner := bufio.NewScanner(bytes.NewReader(out))

	// Skip header line
	if scanner.Scan() {
		// First line is header, skip it
	}

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		// Only interested in orphans (PPID=1)
		if ppid != 1 {
			continue
		}

		// Reconstruct the args (rest of the fields)
		args := strings.Join(fields[2:], " ")

		// Check if it's a claude-related process
		if !isClaudeProcess(args) {
			continue
		}

		// Exclude processes we don't want to kill
		if isExcludedProcess(args) {
			continue
		}

		orphans = append(orphans, OrphanProcess{
			PID:  pid,
			Args: args,
		})
	}

	return orphans, nil
}

// isClaudeProcess checks if the process is claude-related
func isClaudeProcess(args string) bool {
	argsLower := strings.ToLower(args)
	return strings.Contains(argsLower, "claude")
}

// isExcludedProcess checks if the process should be excluded from orphan list
func isExcludedProcess(args string) bool {
	// Exclude any tmux process (server, new-session, etc.)
	// These may contain "claude" in args but are tmux processes, not actual Claude processes
	if strings.HasPrefix(args, "tmux ") || strings.HasPrefix(args, "/usr/bin/tmux") {
		return true
	}

	// Exclude Claude.app desktop application processes
	if strings.Contains(args, "Claude.app") || strings.Contains(args, "/Applications/Claude") {
		return true
	}

	// Exclude Claude Helper processes (part of Claude.app)
	if strings.Contains(args, "Claude Helper") {
		return true
	}

	return false
}

// runOrphansListProcesses lists orphaned Claude processes
func runOrphansListProcesses(cmd *cobra.Command, args []string) error {
	orphans, err := findOrphanProcesses()
	if err != nil {
		return fmt.Errorf("finding orphan processes: %w", err)
	}

	if len(orphans) == 0 {
		fmt.Printf("%s No orphaned Claude processes found\n", style.Bold.Render("✓"))
		return nil
	}

	fmt.Printf("%s Found %d orphaned Claude process(es):\n\n", style.Warning.Render("⚠"), len(orphans))

	for _, o := range orphans {
		// Truncate args for display
		displayArgs := o.Args
		if len(displayArgs) > 80 {
			displayArgs = displayArgs[:77] + "..."
		}
		fmt.Printf("  %s %s\n", style.Bold.Render(fmt.Sprintf("PID %d", o.PID)), displayArgs)
	}

	fmt.Printf("\n%s\n", style.Dim.Render("Use 'gt orphans procs kill' to terminate these processes"))

	return nil
}

// runOrphansKillProcesses kills orphaned Claude processes
func runOrphansKillProcesses(cmd *cobra.Command, args []string) error {
	orphans, err := findOrphanProcesses()
	if err != nil {
		return fmt.Errorf("finding orphan processes: %w", err)
	}

	if len(orphans) == 0 {
		fmt.Printf("%s No orphaned Claude processes found\n", style.Bold.Render("✓"))
		return nil
	}

	// Show what we're about to kill
	fmt.Printf("%s Found %d orphaned Claude process(es):\n\n", style.Warning.Render("⚠"), len(orphans))
	for _, o := range orphans {
		displayArgs := o.Args
		if len(displayArgs) > 80 {
			displayArgs = displayArgs[:77] + "..."
		}
		fmt.Printf("  %s %s\n", style.Bold.Render(fmt.Sprintf("PID %d", o.PID)), displayArgs)
	}
	fmt.Println()

	// Confirm unless --force
	if !orphansProcsForce {
		fmt.Printf("Kill these %d process(es)? [y/N] ", len(orphans))
		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted")
			return nil
		}
	}

	// Kill the processes
	var killed, failed int
	for _, o := range orphans {
		proc, err := os.FindProcess(o.PID)
		if err != nil {
			fmt.Printf("  %s PID %d: %v\n", style.Error.Render("✗"), o.PID, err)
			failed++
			continue
		}

		// Send SIGTERM first for graceful shutdown
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			// Process may have already exited
			if err == os.ErrProcessDone {
				fmt.Printf("  %s PID %d: already terminated\n", style.Dim.Render("○"), o.PID)
				continue
			}
			fmt.Printf("  %s PID %d: %v\n", style.Error.Render("✗"), o.PID, err)
			failed++
			continue
		}

		fmt.Printf("  %s PID %d killed\n", style.Bold.Render("✓"), o.PID)
		killed++
	}

	fmt.Printf("\n%s %d killed", style.Bold.Render("Summary:"), killed)
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
	}
	fmt.Println()

	return nil
}
