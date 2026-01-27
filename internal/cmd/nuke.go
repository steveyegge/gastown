// Package cmd provides CLI commands for the gt tool.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
)

var (
	nukeForce  bool
	nukeDryRun bool
)

var nukeCmd = &cobra.Command{
	Use:     "nuke <path>",
	GroupID: GroupServices,
	Short:   "Completely destroy a Gas Town workspace",
	Long: `Nuclear option to completely destroy a Gas Town workspace.

This command is BRUTAL and IRREVERSIBLE. It will:
1. Kill ALL tmux sessions with gt-* or hq-* prefixes (SIGKILL, no grace)
2. Kill ALL Claude/node processes that might be related
3. rm -rf the entire directory

There is NO graceful shutdown. NO saving state. NO confirmation unless you
forget --force. This is the "burn it all down" option.

Examples:
  gt nuke ./my-town --force       # Destroy ./my-town completely
  gt nuke /home/user/gt --force   # Destroy with absolute path
  gt nuke . --force               # Destroy current directory (if it's a town)
  gt nuke ./my-town --dry-run     # See what would be destroyed

WARNING: This will delete ALL files in the target directory, not just
Gas Town files. Make sure you're pointing at the right directory!`,
	Args: cobra.ExactArgs(1),
	RunE: runNuke,
}

func init() {
	nukeCmd.Flags().BoolVarP(&nukeForce, "force", "f", false,
		"Skip confirmation (REQUIRED for actual destruction)")
	nukeCmd.Flags().BoolVar(&nukeDryRun, "dry-run", false,
		"Show what would be destroyed without doing it")

	rootCmd.AddCommand(nukeCmd)
}

func runNuke(cmd *cobra.Command, args []string) error {
	targetPath := args[0]

	// Resolve to absolute path
	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Safety check: make sure path exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", absPath)
		}
		return fmt.Errorf("checking path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	// Safety check: don't nuke home directory or root
	homeDir, _ := os.UserHomeDir()
	if absPath == "/" || absPath == homeDir {
		return fmt.Errorf("refusing to nuke %s - that would be catastrophic", absPath)
	}

	// Collect what we're going to destroy
	t := tmux.NewTmux()
	sessions, _ := t.ListSessions() // Ignore error - no sessions is fine

	var gtSessions []string
	for _, sess := range sessions {
		if strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-") {
			gtSessions = append(gtSessions, sess)
		}
	}

	// Show what will be destroyed
	fmt.Printf("%s NUCLEAR DESTRUCTION TARGET: %s\n\n", style.Bold.Render("☢️"), absPath)

	if len(gtSessions) > 0 {
		fmt.Println("Tmux sessions to kill:")
		for _, sess := range gtSessions {
			fmt.Printf("  %s %s\n", style.Bold.Render("💀"), sess)
		}
		fmt.Println()
	} else {
		fmt.Printf("  %s No gt-*/hq-* tmux sessions found\n\n", style.Dim.Render("○"))
	}

	fmt.Printf("Directory to delete:\n")
	fmt.Printf("  %s %s (and ALL contents)\n\n", style.Bold.Render("🗑️"), absPath)

	// Dry run stops here
	if nukeDryRun {
		fmt.Printf("%s Dry run - nothing was destroyed\n", style.Info.Render("ℹ"))
		return nil
	}

	// Require --force for actual destruction
	if !nukeForce {
		fmt.Printf("%s This is a DESTRUCTIVE operation!\n", style.Warning.Render("⚠️"))
		fmt.Printf("  Add --force to actually destroy everything.\n")
		fmt.Printf("  Add --dry-run to preview what would be destroyed.\n")
		return fmt.Errorf("--force required for nuclear destruction")
	}

	fmt.Printf("%s NUKING EVERYTHING...\n\n", style.Bold.Render("☢️"))

	// Phase 1: Kill all gt/hq tmux sessions with extreme prejudice
	if len(gtSessions) > 0 {
		fmt.Println("Phase 1: Killing tmux sessions...")
		for _, sess := range gtSessions {
			// Get pane PID first for process group kill
			pid, _ := t.GetPanePID(sess)
			if pid != "" {
				// Kill the entire process group with SIGKILL (no grace)
				pgid := getProcessGroupIDForNuke(pid)
				if pgid != "" && pgid != "0" && pgid != "1" {
					_ = exec.Command("kill", "-9", "--", "-"+pgid).Run()
				}
				// Also kill the specific PID
				_ = exec.Command("kill", "-9", pid).Run()
			}
			// Kill the tmux session
			if err := t.KillSession(sess); err != nil {
				fmt.Printf("  %s %s (may already be dead)\n", style.Dim.Render("○"), sess)
			} else {
				fmt.Printf("  %s %s\n", style.Success.Render("✓"), sess)
			}
		}
		fmt.Println()
	}

	// Phase 2: Find and kill any orphaned Claude/node processes in the target directory
	fmt.Println("Phase 2: Hunting orphaned processes...")
	killed := killProcessesInDirectory(absPath)
	if killed > 0 {
		fmt.Printf("  %s Killed %d orphaned process(es)\n", style.Success.Render("✓"), killed)
	} else {
		fmt.Printf("  %s No orphaned processes found\n", style.Dim.Render("○"))
	}
	fmt.Println()

	// Phase 3: Nuclear option - rm -rf the entire directory
	fmt.Println("Phase 3: Deleting directory...")
	if err := os.RemoveAll(absPath); err != nil {
		return fmt.Errorf("failed to delete %s: %w", absPath, err)
	}
	fmt.Printf("  %s Deleted %s\n", style.Success.Render("✓"), absPath)
	fmt.Println()

	fmt.Printf("%s Nuclear destruction complete. Nothing remains.\n", style.Bold.Render("☢️"))
	return nil
}

// getProcessGroupIDForNuke returns the process group ID for a PID.
func getProcessGroupIDForNuke(pid string) string {
	out, err := exec.Command("ps", "-o", "pgid=", "-p", pid).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// killProcessesInDirectory finds and kills processes with CWD in the target directory.
// This catches any Claude/node processes that escaped session termination.
func killProcessesInDirectory(targetDir string) int {
	killed := 0

	// Use lsof to find processes with files open in the target directory
	// This is more reliable than checking CWD for catching all related processes
	out, err := exec.Command("lsof", "+D", targetDir, "-t").Output()
	if err != nil {
		// lsof might not find anything, which is fine
		// Also try the /proc method as fallback
		return killProcessesViaProc(targetDir)
	}

	pids := strings.Fields(strings.TrimSpace(string(out)))
	for _, pid := range pids {
		// Skip our own process
		if pid == fmt.Sprintf("%d", os.Getpid()) {
			continue
		}
		// Kill with SIGKILL - no mercy
		if err := exec.Command("kill", "-9", pid).Run(); err == nil {
			killed++
		}
	}

	return killed
}

// killProcessesViaProc uses /proc to find processes with CWD in target directory.
// Fallback for when lsof isn't available or fails.
func killProcessesViaProc(targetDir string) int {
	killed := 0

	// Read all PIDs from /proc
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	myPid := fmt.Sprintf("%d", os.Getpid())

	for _, entry := range entries {
		// Skip non-numeric entries (not PIDs)
		pid := entry.Name()
		if !isNumeric(pid) || pid == myPid {
			continue
		}

		// Check if process CWD is in our target directory
		cwdLink := filepath.Join("/proc", pid, "cwd")
		cwd, err := os.Readlink(cwdLink)
		if err != nil {
			continue
		}

		if strings.HasPrefix(cwd, targetDir) || cwd == targetDir {
			// Kill it
			if err := exec.Command("kill", "-9", pid).Run(); err == nil {
				killed++
			}
		}
	}

	return killed
}

// isNumeric checks if a string is all digits.
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
