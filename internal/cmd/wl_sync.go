package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wasteland"
	"github.com/steveyegge/gastown/internal/workspace"
)

var wlSyncDryRun bool
var wlSyncUpgrade bool

var wlSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull upstream changes into local wl-commons fork",
	Args:  cobra.NoArgs,
	RunE:  runWLSync,
	Long: `Sync your local wl-commons fork with the upstream hop/wl-commons.

If you have a local fork of wl-commons (created by gt wl join), this pulls
the latest changes from upstream.

Schema evolution is handled automatically based on semantic versioning:
  - MINOR version bump (e.g. 1.0 → 1.1): auto-applied (new columns, tables)
  - MAJOR version bump (e.g. 1.0 → 2.0): requires --upgrade flag

EXAMPLES:
  gt wl sync                # Pull upstream changes
  gt wl sync --dry-run      # Show what would change
  gt wl sync --upgrade      # Proceed through a MAJOR schema version bump`,
}

func init() {
	wlSyncCmd.Flags().BoolVar(&wlSyncDryRun, "dry-run", false, "Show what would change without pulling")
	wlSyncCmd.Flags().BoolVar(&wlSyncUpgrade, "upgrade", false, "Allow MAJOR schema version upgrades")

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

		if err := checkSchemaEvolution(doltPath, forkDir, wlSyncUpgrade); err != nil {
			return err
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

	fmt.Printf("\nFetching from upstream...\n")

	fetchCmd := exec.Command(doltPath, "fetch", "upstream")
	fetchCmd.Dir = forkDir
	fetchCmd.Stderr = os.Stderr
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("fetching upstream: %w", err)
	}

	if err := checkSchemaEvolution(doltPath, forkDir, wlSyncUpgrade); err != nil {
		return err
	}

	fmt.Printf("Merging upstream changes...\n")

	pullCmd := exec.Command(doltPath, "merge", "upstream/main")
	pullCmd.Dir = forkDir
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("merging upstream: %w", err)
	}

	fmt.Printf("\n%s Synced with upstream\n", style.Bold.Render("✓"))

	// Auto-sweep expired claims after sync
	sweepSyncExpiredClaims(forkDir)

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

// sweepSyncExpiredClaims releases expired claims as part of sync.
// Errors are non-fatal — we report them but don't fail the sync.
func sweepSyncExpiredClaims(forkDir string) {
	cutoff := doltserver.ClaimTimeoutCutoff(DefaultClaimTimeout)

	countQuery := fmt.Sprintf(`SELECT COUNT(*) as cnt FROM wanted WHERE status='claimed' AND claimed_at IS NOT NULL AND claimed_at < '%s';`, cutoff)
	countCmd := exec.Command("dolt", "sql", "-q", countQuery, "-r", "csv")
	countCmd.Dir = forkDir
	countOut, err := countCmd.CombinedOutput()
	if err != nil {
		return // silently skip — claimed_at column may not exist yet
	}
	rows := wlParseCSV(string(countOut))
	if len(rows) < 2 || len(rows[1]) == 0 || rows[1][0] == "0" {
		return
	}
	count := rows[1][0]

	script := fmt.Sprintf(`UPDATE wanted SET status='open', claimed_by=NULL, claimed_at=NULL, updated_at=NOW()
  WHERE status='claimed' AND claimed_at IS NOT NULL AND claimed_at < '%s';
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'wl sweep: release %s expired claims');`, cutoff, doltserver.EscapeSQL(count))

	cmd := exec.Command("dolt", "sql", "-q", script)
	cmd.Dir = forkDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := strings.ToLower(string(out))
		if strings.Contains(s, "nothing to commit") {
			return
		}
		// Non-fatal
		return
	}

	fmt.Printf("  Swept %s expired claim(s)\n", count)
}
