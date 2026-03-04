package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/wasteland"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	wlSyncDryRun bool
	wlSyncUpgrade bool
)

var wlSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull upstream changes into local wl-commons fork",
	Args:  cobra.NoArgs,
	RunE:  runWLSync,
	Long: `Sync your local wl-commons fork with the upstream hop/wl-commons.

If you have a local fork of wl-commons (created by gt wl join), this pulls
the latest changes from upstream.

Schema Evolution:
  - MINOR version bumps (new columns): auto-applied
  - MAJOR version bumps (breaking changes): require --upgrade flag

EXAMPLES:
  gt wl sync                # Pull upstream changes
  gt wl sync --dry-run      # Show what would change
  gt wl sync --upgrade      # Apply breaking schema changes`,
}

func init() {
	wlSyncCmd.Flags().BoolVar(&wlSyncDryRun, "dry-run", false, "Show what would change without pulling")
	wlSyncCmd.Flags().BoolVar(&wlSyncUpgrade, "upgrade", false, "Apply breaking schema changes (MAJOR version bumps)")

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

	// Check schema evolution
	if err := checkSchemaEvolution(doltPath, forkDir, wlSyncUpgrade); err != nil {
		return err
	}

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

// schemaVersion represents a semantic version for schema evolution.
type schemaVersion struct {
	Major int
	Minor int
	Patch int
}

// parseSchemaVersion parses a version string like "1.2.3" or "1.2".
func parseSchemaVersion(s string) (schemaVersion, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ".")

	var v schemaVersion
	if len(parts) >= 1 {
		major, err := strconv.Atoi(parts[0])
		if err != nil {
			return v, fmt.Errorf("invalid major version: %s", parts[0])
		}
		v.Major = major
	}
	if len(parts) >= 2 {
		minor, err := strconv.Atoi(parts[1])
		if err != nil {
			return v, fmt.Errorf("invalid minor version: %s", parts[1])
		}
		v.Minor = minor
	}
	if len(parts) >= 3 {
		patch, err := strconv.Atoi(parts[2])
		if err != nil {
			return v, fmt.Errorf("invalid patch version: %s", parts[2])
		}
		v.Patch = patch
	}
	return v, nil
}

func (v schemaVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// getSchemaVersion queries the _meta table for the schema version.
func getSchemaVersion(doltPath, forkDir string) (string, error) {
	query := "SELECT value FROM _meta WHERE `key` = 'schema_version'"
	cmd := exec.Command(doltPath, "sql", "-q", query, "-r", "csv")
	cmd.Dir = forkDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("querying schema version: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "1.0", nil // Default if not found
	}
	return strings.TrimSpace(lines[1]), nil
}

// getUpstreamSchemaVersion queries upstream for the schema version.
func getUpstreamSchemaVersion(doltPath, forkDir string) (string, error) {
	query := "SELECT value FROM _meta WHERE `key` = 'schema_version'"
	cmd := exec.Command(doltPath, "sql", "-q", query, "-r", "csv", "--ref", "upstream/main")
	cmd.Dir = forkDir
	out, err := cmd.Output()
	if err != nil {
		// If upstream doesn't have _meta, assume same version
		return "1.0", nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "1.0", nil
	}
	return strings.TrimSpace(lines[1]), nil
}

// checkSchemaEvolution compares local and upstream schema versions.
// MINOR bumps (new columns) are auto-applied.
// MAJOR bumps require --upgrade flag.
func checkSchemaEvolution(doltPath, forkDir string, allowUpgrade bool) error {
	localVer, err := getSchemaVersion(doltPath, forkDir)
	if err != nil {
		// Non-fatal - continue without schema check
		return nil
	}

	upstreamVer, err := getUpstreamSchemaVersion(doltPath, forkDir)
	if err != nil {
		return nil
	}

	local, err := parseSchemaVersion(localVer)
	if err != nil {
		return nil
	}

	upstream, err := parseSchemaVersion(upstreamVer)
	if err != nil {
		return nil
	}

	// Same version - nothing to do
	if local.Major == upstream.Major && local.Minor == upstream.Minor {
		return nil
	}

	// MAJOR version bump - requires explicit upgrade
	if upstream.Major > local.Major {
		if !allowUpgrade {
			fmt.Printf("\n%s Schema upgrade required: %s → %s\n",
				style.Bold.Render("⚠"),
				localVer, upstreamVer)
			fmt.Printf("  Run: gt wl sync --upgrade\n")
			fmt.Printf("\n  Breaking changes may affect your local data.\n")
			fmt.Printf("  Review the changelog before upgrading.\n")
			return fmt.Errorf("MAJOR schema upgrade required (use --upgrade to proceed)")
		}

		fmt.Printf("\n%s Applying MAJOR schema upgrade: %s → %s\n",
			style.Bold.Render("⚡"), localVer, upstreamVer)

		if err := applySchemaUpgrade(doltPath, forkDir, local, upstream); err != nil {
			return fmt.Errorf("schema upgrade failed: %w", err)
		}

		fmt.Printf("%s Schema upgraded successfully\n", style.Bold.Render("✓"))
		return nil
	}

	// MINOR version bump - auto-apply
	if upstream.Minor > local.Minor {
		fmt.Printf("\n%s Auto-applying schema changes: %s → %s\n",
			style.Bold.Render("~"), localVer, upstreamVer)

		if err := applySchemaUpgrade(doltPath, forkDir, local, upstream); err != nil {
			return fmt.Errorf("schema migration failed: %w", err)
		}

		fmt.Printf("%s Schema updated\n", style.Bold.Render("✓"))
	}

	return nil
}

// applySchemaUpgrade applies schema migrations between versions.
func applySchemaUpgrade(doltPath, forkDir string, from, to schemaVersion) error {
	// Update the local schema version in _meta
	query := fmt.Sprintf("UPDATE _meta SET value = '%s' WHERE `key` = 'schema_version'", to.String())
	cmd := exec.Command(doltPath, "sql", "-q", query)
	cmd.Dir = forkDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("updating schema version: %w", err)
	}

	// Commit the schema update
	commitCmd := exec.Command(doltPath, "add", "-A")
	commitCmd.Dir = forkDir
	_ = commitCmd.Run()

	commitMsg := fmt.Sprintf("Schema evolution: %s → %s", from.String(), to.String())
	commitCmd = exec.Command(doltPath, "commit", "-m", commitMsg, "--allow-empty")
	commitCmd.Dir = forkDir
	_ = commitCmd.Run()

	return nil
}
