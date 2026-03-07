package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wasteland"
	"github.com/steveyegge/gastown/internal/workspace"
)

// DefaultClaimTimeout is the default duration after which uncompleted claims
// are released back to open status.
const DefaultClaimTimeout = 72 * time.Hour

var wlSweepTimeout string
var wlSweepDryRun bool

var wlSweepCmd = &cobra.Command{
	Use:   "sweep",
	Short: "Release expired claimed items back to open",
	Long: `Release wanted items that have been claimed but not completed within
a timeout period. Expired claims are set back to 'open' so other rigs
can claim them.

The default timeout is 72 hours (3 days). Use --timeout to override.

Examples:
  gt wl sweep                    # Release claims older than 72h
  gt wl sweep --timeout 24h     # Release claims older than 24h
  gt wl sweep --timeout 168h    # Release claims older than 1 week
  gt wl sweep --dry-run         # Show what would be released`,
	Args: cobra.NoArgs,
	RunE: runWlSweep,
}

func init() {
	wlSweepCmd.Flags().StringVar(&wlSweepTimeout, "timeout", "72h", "Claim timeout duration (e.g., 24h, 168h)")
	wlSweepCmd.Flags().BoolVar(&wlSweepDryRun, "dry-run", false, "Show what would be released without making changes")

	wlCmd.AddCommand(wlSweepCmd)
}

func runWlSweep(cmd *cobra.Command, args []string) error {
	timeout, err := time.ParseDuration(wlSweepTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout %q: %w", wlSweepTimeout, err)
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Try server-backed store first, fall back to local clone.
	if doltserver.DatabaseExists(townRoot, doltserver.WLCommonsDB) {
		return sweepServerStore(townRoot, timeout, wlSweepDryRun)
	}

	wlCfg, err := wasteland.LoadConfig(townRoot)
	if err != nil {
		return fmt.Errorf("loading wasteland config: %w", err)
	}
	if wlCfg.LocalDir == "" {
		return fmt.Errorf("database %q not found\nJoin a wasteland first with: gt wl join <org/db>", doltserver.WLCommonsDB)
	}

	return sweepLocalClone(wlCfg.LocalDir, timeout, wlSweepDryRun)
}

func sweepServerStore(townRoot string, timeout time.Duration, dryRun bool) error {
	store := doltserver.NewWLCommons(townRoot)

	if dryRun {
		items, err := store.QueryExpiredClaims(timeout)
		if err != nil {
			return fmt.Errorf("querying expired claims: %w", err)
		}
		printSweepDryRunItems(items, timeout)
		return nil
	}

	released, err := store.ReleaseExpiredClaims(timeout)
	if err != nil {
		return fmt.Errorf("sweeping expired claims: %w", err)
	}

	if released == 0 {
		fmt.Printf("%s No expired claims found (timeout: %s)\n", style.Bold.Render("✓"), timeout)
	} else {
		fmt.Printf("%s Released %d expired claim(s) (timeout: %s)\n", style.Bold.Render("✓"), released, timeout)
	}
	return nil
}

func sweepLocalClone(localDir string, timeout time.Duration, dryRun bool) error {
	cutoff := time.Now().UTC().Add(-timeout).Format("2006-01-02 15:04:05")

	if dryRun {
		query := fmt.Sprintf(`SELECT id, title, claimed_by, claimed_at FROM wanted WHERE status='claimed' AND claimed_at IS NOT NULL AND claimed_at < '%s';`, cutoff)
		cmd := exec.Command("dolt", "sql", "-q", query, "-r", "csv")
		cmd.Dir = localDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("querying expired claims: %w (%s)", err, strings.TrimSpace(string(out)))
		}
		printSweepDryRunCSV(string(out), timeout)
		return nil
	}

	// Count first
	countQuery := fmt.Sprintf(`SELECT COUNT(*) as cnt FROM wanted WHERE status='claimed' AND claimed_at IS NOT NULL AND claimed_at < '%s';`, cutoff)
	countCmd := exec.Command("dolt", "sql", "-q", countQuery, "-r", "csv")
	countCmd.Dir = localDir
	countOut, err := countCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("counting expired claims: %w", err)
	}
	rows := wlParseCSV(string(countOut))
	if len(rows) < 2 || len(rows[1]) == 0 || rows[1][0] == "0" {
		fmt.Printf("%s No expired claims found (timeout: %s)\n", style.Bold.Render("✓"), timeout)
		return nil
	}
	count := rows[1][0]

	script := fmt.Sprintf(`UPDATE wanted SET status='open', claimed_by=NULL, claimed_at=NULL, updated_at=NOW()
  WHERE status='claimed' AND claimed_at IS NOT NULL AND claimed_at < '%s';
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('-m', 'wl sweep: release %s expired claims');`, cutoff, doltserver.EscapeSQL(count))

	releaseCmd := exec.Command("dolt", "sql", "-q", script)
	releaseCmd.Dir = localDir
	out, err := releaseCmd.CombinedOutput()
	if err != nil {
		s := strings.ToLower(string(out))
		if strings.Contains(s, "nothing to commit") {
			fmt.Printf("%s No expired claims found (timeout: %s)\n", style.Bold.Render("✓"), timeout)
			return nil
		}
		return fmt.Errorf("releasing expired claims: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	fmt.Printf("%s Released %s expired claim(s) (timeout: %s)\n", style.Bold.Render("✓"), count, timeout)
	return nil
}

func printSweepDryRunItems(items []*doltserver.WantedItem, timeout time.Duration) {
	if len(items) == 0 {
		fmt.Printf("%s No expired claims found (timeout: %s)\n", style.Bold.Render("~"), timeout)
		return
	}

	fmt.Printf("Expired claims that would be released (timeout: %s):\n\n", timeout)
	tbl := style.NewTable(
		style.Column{Name: "ID", Width: 14},
		style.Column{Name: "TITLE", Width: 40},
		style.Column{Name: "CLAIMED BY", Width: 20},
		style.Column{Name: "CLAIMED AT", Width: 20},
	)
	for _, item := range items {
		claimedAt := ""
		if item.ClaimedAt != nil {
			claimedAt = item.ClaimedAt.Format("2006-01-02 15:04")
		}
		tbl.AddRow(item.ID, item.Title, item.ClaimedBy, claimedAt)
	}
	fmt.Print(tbl.Render())
	fmt.Printf("\n%d item(s) would be released.\n", len(items))
}

func printSweepDryRunCSV(csvOutput string, timeout time.Duration) {
	rows := wlParseCSV(csvOutput)
	if len(rows) <= 1 {
		fmt.Printf("%s No expired claims found (timeout: %s)\n", style.Bold.Render("~"), timeout)
		return
	}

	fmt.Printf("Expired claims that would be released (timeout: %s):\n\n", timeout)
	tbl := style.NewTable(
		style.Column{Name: "ID", Width: 14},
		style.Column{Name: "TITLE", Width: 40},
		style.Column{Name: "CLAIMED BY", Width: 20},
		style.Column{Name: "CLAIMED AT", Width: 20},
	)
	for _, row := range rows[1:] {
		if len(row) < 4 {
			continue
		}
		tbl.AddRow(row[0], row[1], row[2], row[3])
	}
	fmt.Print(tbl.Render())
	fmt.Printf("\n%d item(s) would be released.\n", len(rows)-1)
}
