package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

const (
	// diskLogSizeThreshold is the log file size above which we rotate/truncate.
	diskLogSizeThreshold = 50 * 1024 * 1024 // 50MB

	// diskPruneClaudeProjectsDays prunes projects not accessed in this many days.
	diskPruneClaudeProjectsDays = 30
)

var (
	maintainDiskEmergency           bool
	maintainDiskPruneClaudeProjects bool
)

var maintainDiskCmd = &cobra.Command{
	Use:   "disk",
	Short: "Reclaim disk space (log rotation, Go cache, git gc)",
	Long: `Reclaim disk space without risking data loss.

Actions performed:
  1. Rotate daemon logs exceeding 50MB (copytruncate, gzip compressed)
  2. Clear Go build cache (go clean -cache)
  3. Run git gc --auto on all rig worktrees
  4. Prune stale git worktrees

With --prune-claude-projects: also removes ~/.claude/projects entries
not accessed in 30+ days (large conversation histories).

With --emergency: also clears Go test cache and prunes git objects.

Examples:
  gt maintain disk                       # Safe cleanup
  gt maintain disk --force               # Non-interactive
  gt maintain disk --prune-claude-projects  # Also prune old Claude projects
  gt maintain disk --emergency --force   # Emergency aggressive cleanup`,
	RunE: runMaintainDisk,
}

func init() {
	maintainDiskCmd.Flags().BoolVar(&maintainForce, "force", false, "Non-interactive mode (skip confirmation)")
	maintainDiskCmd.Flags().BoolVar(&maintainDryRun, "dry-run", false, "Preview without making changes")
	maintainDiskCmd.Flags().BoolVar(&maintainDiskEmergency, "emergency", false, "Aggressive cleanup: also clear test cache and prune git objects")
	maintainDiskCmd.Flags().BoolVar(&maintainDiskPruneClaudeProjects, "prune-claude-projects", false, "Remove ~/.claude/projects entries not accessed in 30+ days")
	maintainCmd.AddCommand(maintainDiskCmd)
}

func runMaintainDisk(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// --- Build plan ---
	fmt.Printf("%s Building disk cleanup plan...\n", style.Bold.Render("●"))

	logsToRotate, err := findLogsToRotate(townRoot)
	if err != nil {
		return fmt.Errorf("scanning logs: %w", err)
	}

	goCache, goCacheSize := estimateGoCache()
	worktrees := findRigWorktrees(townRoot)

	claudeProjects := []string(nil)
	claudeProjectsSize := int64(0)
	if maintainDiskPruneClaudeProjects {
		claudeProjects, claudeProjectsSize = findStaleClaudeProjects()
	}

	// --- Display plan ---
	fmt.Printf("\n%s Disk cleanup plan:\n", style.Bold.Render("●"))
	fmt.Printf("  Logs to rotate: %d (>50MB)\n", len(logsToRotate))
	for _, p := range logsToRotate {
		info, _ := os.Stat(p)
		if info != nil {
			fmt.Printf("    %s (%s)\n", p, formatDiskSize(info.Size()))
		}
	}
	fmt.Printf("  Go build cache: %s\n", goCache)
	fmt.Printf("  Worktrees for git gc --auto: %d\n", len(worktrees))
	if maintainDiskEmergency {
		fmt.Printf("  %s Emergency: also clear test cache, prune git objects\n", style.Warning.Render("!"))
	}
	if maintainDiskPruneClaudeProjects {
		fmt.Printf("  Stale Claude projects to prune: %d (%s)\n", len(claudeProjects), formatDiskSize(claudeProjectsSize))
	}

	if maintainDryRun {
		fmt.Printf("\n%s Dry run complete — no changes made\n", style.Dim.Render("ℹ"))
		return nil
	}

	if !maintainForce {
		fmt.Printf("\nProceed? [y/N] ")
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	start := time.Now()
	var freedBytes int64

	// --- Phase 1: Rotate logs ---
	if len(logsToRotate) > 0 {
		fmt.Printf("\n%s Rotating logs...\n", style.Bold.Render("●"))
		result := daemon.RotateLogs(townRoot)
		for _, p := range result.Rotated {
			fmt.Printf("  %s rotated: %s\n", style.Bold.Render("✓"), p)
		}
		for _, e := range result.Errors {
			fmt.Printf("  %s error: %v\n", style.Warning.Render("!"), e)
		}
	}

	// --- Phase 2: Clear Go build cache ---
	fmt.Printf("\n%s Clearing Go build cache (%s)...\n", style.Bold.Render("●"), goCache)
	if err := runGoClean(maintainDiskEmergency); err != nil {
		fmt.Printf("  %s go clean failed: %v\n", style.Warning.Render("!"), err)
	} else {
		fmt.Printf("  %s Go build cache cleared\n", style.Bold.Render("✓"))
		freedBytes += goCacheSize
	}

	// --- Phase 3: git gc --auto on worktrees ---
	if len(worktrees) > 0 {
		fmt.Printf("\n%s Running git gc --auto on %d worktrees...\n", style.Bold.Render("●"), len(worktrees))
		for _, wt := range worktrees {
			if err := runGitGCAuto(wt, maintainDiskEmergency); err != nil {
				fmt.Printf("  %s %s: %v\n", style.Warning.Render("!"), filepath.Base(wt), err)
			} else {
				fmt.Printf("  %s %s\n", style.Bold.Render("✓"), wt)
			}
		}
	}

	// --- Phase 4: Prune stale Claude projects ---
	if maintainDiskPruneClaudeProjects && len(claudeProjects) > 0 {
		fmt.Printf("\n%s Pruning %d stale Claude projects (%s)...\n",
			style.Bold.Render("●"), len(claudeProjects), formatDiskSize(claudeProjectsSize))
		for _, p := range claudeProjects {
			if err := os.RemoveAll(p); err != nil {
				fmt.Printf("  %s %s: %v\n", style.Warning.Render("!"), filepath.Base(p), err)
			} else {
				fmt.Printf("  %s removed: %s\n", style.Bold.Render("✓"), p)
				freedBytes += claudeProjectsSize / int64(len(claudeProjects))
			}
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("\n%s Disk cleanup complete (%v)\n", style.Success.Render("✓"), elapsed.Round(time.Second))
	if freedBytes > 0 {
		fmt.Printf("  Estimated space freed: %s\n", formatDiskSize(freedBytes))
	}

	return nil
}

// findLogsToRotate returns daemon log files larger than diskLogSizeThreshold.
func findLogsToRotate(townRoot string) ([]string, error) {
	var large []string

	// Daemon-level logs
	daemonDir := filepath.Join(townRoot, "daemon")
	for _, name := range []string{"dolt.log", "dolt-server.log", "daemon.log"} {
		p := filepath.Join(daemonDir, name)
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.Size() >= diskLogSizeThreshold {
			large = append(large, p)
		}
	}

	// Rig-level dolt-server.log
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return large, nil
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "daemon" {
			continue
		}
		rigLog := filepath.Join(townRoot, entry.Name(), ".beads", "dolt-server.log")
		info, err := os.Stat(rigLog)
		if err == nil && info.Size() >= diskLogSizeThreshold {
			large = append(large, rigLog)
		}
	}

	return large, nil
}

// estimateGoCache returns a human-readable size estimate and byte count of the Go build cache.
func estimateGoCache() (string, int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "go", "env", "GOCACHE").Output()
	if err != nil {
		return "unknown", 0
	}
	cacheDir := strings.TrimSpace(string(out))
	size := dirSize(cacheDir)
	return formatDiskSize(size), size
}

// runGoClean clears the Go build cache. If emergency, also clears the test cache.
func runGoClean(emergency bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	args := []string{"clean", "-cache"}
	if emergency {
		args = append(args, "-testcache")
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// findRigWorktrees returns all git worktree paths under townRoot.
func findRigWorktrees(townRoot string) []string {
	var worktrees []string

	// Walk polecats/<lang>/<rig> structure.
	entries, _ := os.ReadDir(townRoot)
	for _, rigEntry := range entries {
		if !rigEntry.IsDir() || strings.HasPrefix(rigEntry.Name(), ".") {
			continue
		}
		polecatsDir := filepath.Join(townRoot, rigEntry.Name(), "polecats")
		langEntries, err := os.ReadDir(polecatsDir)
		if err != nil {
			continue
		}
		for _, langEntry := range langEntries {
			if !langEntry.IsDir() {
				continue
			}
			// Each subdir under polecats/<lang>/ is a worktree.
			wtPath := filepath.Join(polecatsDir, langEntry.Name())
			// Check if it's a git worktree (has .git file or dir).
			gitMarker := filepath.Join(wtPath, ".git")
			if _, err := os.Stat(gitMarker); err == nil {
				worktrees = append(worktrees, wtPath)
				continue
			}
			// Check one level deeper: polecats/<lang>/<rig>.
			subEntries, err := os.ReadDir(wtPath)
			if err != nil {
				continue
			}
			for _, subEntry := range subEntries {
				if !subEntry.IsDir() {
					continue
				}
				subPath := filepath.Join(wtPath, subEntry.Name())
				gitMarker := filepath.Join(subPath, ".git")
				if _, err := os.Stat(gitMarker); err == nil {
					worktrees = append(worktrees, subPath)
				}
			}
		}
	}
	return worktrees
}

// runGitGCAuto runs git gc --auto (and optionally --prune=now) on a worktree.
func runGitGCAuto(worktree string, aggressive bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	args := []string{"gc", "--auto"}
	if aggressive {
		args = append(args, "--prune=now")
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = worktree
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// findStaleClaudeProjects returns paths of ~/.claude/projects entries not accessed in 30+ days.
func findStaleClaudeProjects() ([]string, int64) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, 0
	}
	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, 0
	}

	cutoff := time.Now().AddDate(0, 0, -diskPruneClaudeProjectsDays)
	var stale []string
	var totalSize int64

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(projectsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			size := dirSize(path)
			stale = append(stale, path)
			totalSize += size
		}
	}
	return stale, totalSize
}

// dirSize returns the total size of all files in a directory tree.
func dirSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// formatDiskSize formats a byte count as a human-readable string.
func formatDiskSize(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	ub := uint64(b)
	switch {
	case ub >= gb:
		return fmt.Sprintf("%.1f GB", float64(ub)/float64(gb))
	case ub >= mb:
		return fmt.Sprintf("%.1f MB", float64(ub)/float64(mb))
	case ub >= kb:
		return fmt.Sprintf("%.1f KB", float64(ub)/float64(kb))
	default:
		return fmt.Sprintf("%d B", ub)
	}
}
