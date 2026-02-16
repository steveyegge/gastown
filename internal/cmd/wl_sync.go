package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wasteland"
	"github.com/steveyegge/gastown/internal/workspace"
)

var wlSyncDryRun bool

var wlSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull upstream changes into local wl-commons fork",
	Args:  cobra.NoArgs,
	RunE:  runWLSync,
	Long: `Sync your local wl-commons fork with the upstream hop/wl-commons.

If you have a local fork of wl-commons (created by gt wl join), this pulls
the latest changes from upstream.

EXAMPLES:
  gt wl sync                # Pull upstream changes
  gt wl sync --dry-run      # Show what would change`,
}

func init() {
	wlSyncCmd.Flags().BoolVar(&wlSyncDryRun, "dry-run", false, "Show what would change without pulling")

	wlCmd.AddCommand(wlSyncCmd)
}

func runWLSync(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		return fmt.Errorf("dolt not found in PATH — install from https://docs.dolthub.com/introduction/installation")
	}

	// Try loading wasteland config first (set by gt wl join)
	forkDir := ""
	if cfg, err := wasteland.LoadConfig(townRoot); err == nil {
		forkDir = cfg.LocalDir
	}

	// Fall back to standard locations
	if forkDir == "" {
		forkDir = findWLCommonsFork(townRoot)
	}

	if forkDir == "" {
		return fmt.Errorf("no local wl-commons fork found\n\nJoin a wasteland first: gt wl join <org/db>")
	}

	fmt.Printf("Local fork: %s\n", style.Dim.Render(forkDir))

	if wlSyncDryRun {
		fmt.Printf("\n%s Dry run — checking upstream for changes...\n", style.Bold.Render("~"))

		fetchCmd := exec.Command(doltPath, "fetch", "upstream")
		fetchCmd.Dir = forkDir
		fetchCmd.Stderr = os.Stderr
		if err := fetchCmd.Run(); err != nil {
			return fmt.Errorf("fetching upstream: %w", err)
		}

		diffCmd := exec.Command(doltPath, "diff", "--stat", "HEAD", "upstream/main")
		diffCmd.Dir = forkDir
		diffCmd.Stdout = os.Stdout
		diffCmd.Stderr = os.Stderr
		if err := diffCmd.Run(); err != nil {
			fmt.Printf("%s Already up to date.\n", style.Bold.Render("✓"))
		}
		return nil
	}

	fmt.Printf("\nPulling from upstream...\n")

	pullCmd := exec.Command(doltPath, "pull", "upstream", "main")
	pullCmd.Dir = forkDir
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("pulling from upstream: %w", err)
	}

	fmt.Printf("\n%s Synced with upstream\n", style.Bold.Render("✓"))

	// Show summary
	summaryQuery := `SELECT
		(SELECT COUNT(*) FROM wanted WHERE status = 'open') AS open_wanted,
		(SELECT COUNT(*) FROM wanted) AS total_wanted,
		(SELECT COUNT(*) FROM completions) AS total_completions,
		(SELECT COUNT(*) FROM stamps) AS total_stamps`

	summaryCmd := exec.Command(doltPath, "sql", "-q", summaryQuery, "-r", "csv")
	summaryCmd.Dir = forkDir
	out, err := summaryCmd.Output()
	if err == nil {
		rows := wlParseCSV(string(out))
		if len(rows) >= 2 && len(rows[1]) >= 4 {
			r := rows[1]
			fmt.Printf("\n  Open wanted:       %s\n", r[0])
			fmt.Printf("  Total wanted:      %s\n", r[1])
			fmt.Printf("  Total completions: %s\n", r[2])
			fmt.Printf("  Total stamps:      %s\n", r[3])
		}
	}

	return nil
}

func findWLCommonsFork(townRoot string) string {
	candidates := []string{
		filepath.Join(townRoot, "wl-commons"),
		filepath.Join(townRoot, "..", "wl-commons"),
		filepath.Join(os.Getenv("HOME"), "wl-commons"),
	}

	for _, dir := range candidates {
		doltDir := filepath.Join(dir, ".dolt")
		if info, err := os.Stat(doltDir); err == nil && info.IsDir() {
			return dir
		}
	}

	return ""
}
