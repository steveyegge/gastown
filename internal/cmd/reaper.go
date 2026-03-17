package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/reaper"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	reaperDB       string
	reaperHost     string
	reaperPort     int
	reaperMaxAge   string
	reaperPurgeAge string
	reaperMailAge  string
	reaperStaleAge string
	reaperDryRun   bool
	reaperJSON     bool
)

var reaperCmd = &cobra.Command{
	Use:     "reaper",
	GroupID: GroupServices,
	Short:   "Wisp and issue cleanup operations (Dog-callable helpers)",
	Long: `Execute wisp reaper operations against Dolt databases.

These subcommands are the callable helper functions for the mol-dog-reaper
formula. They execute SQL operations but leave eligibility decisions to the
Dog agent or daemon orchestrator.

When run by a Dog:
  gt reaper scan --db=gastown          # Discover candidates
  gt reaper reap --db=gastown          # Close stale wisps
  gt reaper purge --db=gastown         # Delete old closed wisps + mail
  gt reaper auto-close --db=gastown    # Close stale issues`,
	RunE: requireSubcommand,
}

var reaperDatabasesCmd = &cobra.Command{
	Use:   "databases",
	Short: "List databases available for reaping",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbs := reaper.DiscoverDatabases(reaperHost, reaperPort)
		if reaperJSON {
			fmt.Println(reaper.FormatJSON(dbs))
		} else {
			for _, db := range dbs {
				fmt.Println(db)
			}
		}
		return nil
	},
}

var reaperScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan a database for reaper candidates",
	Long: `Count reap, purge, auto-close, and mail candidates in a database.

Returns counts and anomaly detection results without modifying any data.
The Dog uses this to understand the state before deciding what to reap.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if reaperDB == "" {
			return fmt.Errorf("--db is required")
		}

		maxAge, err := time.ParseDuration(reaperMaxAge)
		if err != nil {
			return fmt.Errorf("invalid --max-age: %w", err)
		}
		purgeAge, err := time.ParseDuration(reaperPurgeAge)
		if err != nil {
			return fmt.Errorf("invalid --purge-age: %w", err)
		}
		mailAge, err := time.ParseDuration(reaperMailAge)
		if err != nil {
			return fmt.Errorf("invalid --mail-age: %w", err)
		}
		staleAge, err := time.ParseDuration(reaperStaleAge)
		if err != nil {
			return fmt.Errorf("invalid --stale-age: %w", err)
		}

		db, err := reaper.OpenDB(reaperHost, reaperPort, reaperDB, 10*time.Second, 10*time.Second)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", reaperDB, err)
		}
		defer db.Close()

		if ok, err := reaper.HasReaperSchema(db); err != nil {
			return fmt.Errorf("check reaper schema on %s: %w", reaperDB, err)
		} else if !ok {
			return fmt.Errorf("database %s missing wisps/issues tables (beads schema not initialized on this server)", reaperDB)
		}

		result, err := reaper.Scan(db, reaperDB, maxAge, purgeAge, mailAge, staleAge)
		if err != nil {
			return fmt.Errorf("scan %s: %w", reaperDB, err)
		}

		if reaperJSON {
			fmt.Println(reaper.FormatJSON(result))
		} else {
			fmt.Printf("Database: %s\n", result.Database)
			fmt.Printf("  Reap candidates:  %d\n", result.ReapCandidates)
			fmt.Printf("  Purge candidates: %d\n", result.PurgeCandidates)
			fmt.Printf("  Mail candidates:  %d\n", result.MailCandidates)
			fmt.Printf("  Stale candidates: %d\n", result.StaleCandidates)
			fmt.Printf("  Open wisps:       %d\n", result.OpenWisps)
			for _, a := range result.Anomalies {
				fmt.Printf("  %s %s\n", style.Warning.Render("ANOMALY:"), a.Message)
			}
		}
		return nil
	},
}

var reaperReapCmd = &cobra.Command{
	Use:   "reap",
	Short: "Close stale wisps past max-age",
	Long: `Close wisps that are past the max-age threshold and whose parent
molecule is already closed (or missing/orphaned).

Returns the count of reaped wisps. Use --dry-run to preview.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if reaperDB == "" {
			return fmt.Errorf("--db is required")
		}

		maxAge, err := time.ParseDuration(reaperMaxAge)
		if err != nil {
			return fmt.Errorf("invalid --max-age: %w", err)
		}

		db, err := reaper.OpenDB(reaperHost, reaperPort, reaperDB, 10*time.Second, 10*time.Second)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", reaperDB, err)
		}
		defer db.Close()

		if ok, err := reaper.HasReaperSchema(db); err != nil {
			return fmt.Errorf("check reaper schema on %s: %w", reaperDB, err)
		} else if !ok {
			return fmt.Errorf("database %s missing wisps/issues tables (beads schema not initialized on this server)", reaperDB)
		}

		result, err := reaper.Reap(db, reaperDB, maxAge, reaperDryRun)
		if err != nil {
			return fmt.Errorf("reap %s: %w", reaperDB, err)
		}

		if reaperJSON {
			fmt.Println(reaper.FormatJSON(result))
		} else {
			prefix := ""
			if result.DryRun {
				prefix = "[DRY RUN] would "
			}
			fmt.Printf("%s: %sreaped %d wisps, %d open remain\n",
				result.Database, prefix, result.Reaped, result.OpenRemain)
		}
		return nil
	},
}

var reaperPurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete old closed wisps and mail",
	Long: `Delete closed wisps past the purge-age threshold and closed mail
past the mail-age threshold. Irreversible operation.

Returns counts of purged rows. Use --dry-run to preview.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if reaperDB == "" {
			return fmt.Errorf("--db is required")
		}

		purgeAge, err := time.ParseDuration(reaperPurgeAge)
		if err != nil {
			return fmt.Errorf("invalid --purge-age: %w", err)
		}
		mailAge, err := time.ParseDuration(reaperMailAge)
		if err != nil {
			return fmt.Errorf("invalid --mail-age: %w", err)
		}

		db, err := reaper.OpenDB(reaperHost, reaperPort, reaperDB, 30*time.Second, 30*time.Second)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", reaperDB, err)
		}
		defer db.Close()

		if ok, err := reaper.HasReaperSchema(db); err != nil {
			return fmt.Errorf("check reaper schema on %s: %w", reaperDB, err)
		} else if !ok {
			return fmt.Errorf("database %s missing wisps/issues tables (beads schema not initialized on this server)", reaperDB)
		}

		result, err := reaper.Purge(db, reaperDB, purgeAge, mailAge, reaperDryRun)
		if err != nil {
			return fmt.Errorf("purge %s: %w", reaperDB, err)
		}

		if reaperJSON {
			fmt.Println(reaper.FormatJSON(result))
		} else {
			prefix := ""
			if result.DryRun {
				prefix = "[DRY RUN] would "
			}
			fmt.Printf("%s: %spurged %d wisps, %d mail\n",
				result.Database, prefix, result.WispsPurged, result.MailPurged)
			for _, a := range result.Anomalies {
				fmt.Printf("  %s %s\n", style.Warning.Render("ANOMALY:"), a.Message)
			}
		}
		return nil
	},
}

var reaperAutoCloseCmd = &cobra.Command{
	Use:   "auto-close",
	Short: "Close stale issues past stale-age",
	Long: `Close issues open with no updates past the stale-age threshold.
Excludes P0/P1 priority, epics, and issues with active dependencies.

Returns the count of closed issues. Use --dry-run to preview.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if reaperDB == "" {
			return fmt.Errorf("--db is required")
		}

		staleAge, err := time.ParseDuration(reaperStaleAge)
		if err != nil {
			return fmt.Errorf("invalid --stale-age: %w", err)
		}

		db, err := reaper.OpenDB(reaperHost, reaperPort, reaperDB, 10*time.Second, 10*time.Second)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", reaperDB, err)
		}
		defer db.Close()

		result, err := reaper.AutoClose(db, reaperDB, staleAge, reaperDryRun)
		if err != nil {
			return fmt.Errorf("auto-close %s: %w", reaperDB, err)
		}

		if reaperJSON {
			fmt.Println(reaper.FormatJSON(result))
		} else {
			prefix := ""
			if result.DryRun {
				prefix = "[DRY RUN] would "
			}
			fmt.Printf("%s: %sauto-closed %d stale issues\n",
				result.Database, prefix, result.Closed)
		}
		return nil
	},
}

var reaperRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run full reaper cycle across all databases",
	Long: `Execute a full reaper cycle: scan → reap → purge → auto-close → report.

This is the inline fallback for when Dog dispatch is unavailable.
Normally the daemon dispatches a Dog to execute the mol-dog-reaper formula.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		databases := reaper.DiscoverDatabases(reaperHost, reaperPort)
		if reaperDB != "" {
			databases = strings.Split(reaperDB, ",")
		}

		maxAge, err := time.ParseDuration(reaperMaxAge)
		if err != nil {
			return fmt.Errorf("invalid --max-age: %w", err)
		}
		purgeAge, err := time.ParseDuration(reaperPurgeAge)
		if err != nil {
			return fmt.Errorf("invalid --purge-age: %w", err)
		}
		mailAge, err := time.ParseDuration(reaperMailAge)
		if err != nil {
			return fmt.Errorf("invalid --mail-age: %w", err)
		}
		staleAge, err := time.ParseDuration(reaperStaleAge)
		if err != nil {
			return fmt.Errorf("invalid --stale-age: %w", err)
		}

		var totalReaped, totalPurged, totalMailPurged, totalClosed, totalOpen int

		for _, dbName := range databases {
			if err := reaper.ValidateDBName(dbName); err != nil {
				fmt.Printf("skip invalid db: %s\n", dbName)
				continue
			}

			db, err := reaper.OpenDB(reaperHost, reaperPort, dbName, 30*time.Second, 30*time.Second)
			if err != nil {
				fmt.Printf("%s: connect error: %v\n", dbName, err)
				continue
			}

			if ok, err := reaper.HasReaperSchema(db); err != nil {
				fmt.Printf("%s: schema check error: %v\n", dbName, err)
				db.Close()
				continue
			} else if !ok {
				fmt.Printf("%s: skipped (no reaper schema)\n", dbName)
				db.Close()
				continue
			}

			// Scan
			scanResult, err := reaper.Scan(db, dbName, maxAge, purgeAge, mailAge, staleAge)
			if err != nil {
				fmt.Printf("%s: scan error: %v\n", dbName, err)
				db.Close()
				continue
			}
			for _, a := range scanResult.Anomalies {
				fmt.Printf("%s: %s %s\n", dbName, style.Warning.Render("ANOMALY:"), a.Message)
			}

			// Reap
			reapResult, err := reaper.Reap(db, dbName, maxAge, reaperDryRun)
			if err != nil {
				fmt.Printf("%s: reap error: %v\n", dbName, err)
			} else {
				totalReaped += reapResult.Reaped
				totalOpen += reapResult.OpenRemain
			}

			// Purge
			purgeResult, err := reaper.Purge(db, dbName, purgeAge, mailAge, reaperDryRun)
			if err != nil {
				fmt.Printf("%s: purge error: %v\n", dbName, err)
			} else {
				totalPurged += purgeResult.WispsPurged
				totalMailPurged += purgeResult.MailPurged
			}

			// Auto-close
			closeResult, err := reaper.AutoClose(db, dbName, staleAge, reaperDryRun)
			if err != nil {
				fmt.Printf("%s: auto-close error: %v\n", dbName, err)
			} else {
				totalClosed += closeResult.Closed
			}

			db.Close()
		}

		// Report
		prefix := ""
		if reaperDryRun {
			prefix = "[DRY RUN] "
		}
		fmt.Printf("\n%sReaper cycle complete:\n", prefix)
		fmt.Printf("  Databases: %d\n", len(databases))
		fmt.Printf("  Reaped:    %d\n", totalReaped)
		fmt.Printf("  Purged:    %d wisps, %d mail\n", totalPurged, totalMailPurged)
		fmt.Printf("  Closed:    %d stale issues\n", totalClosed)
		fmt.Printf("  Open:      %d wisps remain\n", totalOpen)

		return nil
	},
}

func init() {
	// Shared flags
	// GH#2601: Default host/port from env vars for non-localhost setups.
	defaultHost := "127.0.0.1"
	if h := os.Getenv("GT_DOLT_HOST"); h != "" {
		defaultHost = h
	} else if h := os.Getenv("BEADS_DOLT_SERVER_HOST"); h != "" {
		defaultHost = h
	}
	defaultPort := 3307
	if p := os.Getenv("GT_DOLT_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			defaultPort = v
		}
	} else if p := os.Getenv("BEADS_DOLT_SERVER_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			defaultPort = v
		}
	}

	for _, cmd := range []*cobra.Command{reaperScanCmd, reaperReapCmd, reaperPurgeCmd, reaperAutoCloseCmd, reaperRunCmd} {
		cmd.Flags().StringVar(&reaperDB, "db", "", "Database name (required for single-db commands)")
		cmd.Flags().StringVar(&reaperHost, "host", defaultHost, "Dolt server host (env: GT_DOLT_HOST)")
		cmd.Flags().IntVar(&reaperPort, "port", defaultPort, "Dolt server port (env: GT_DOLT_PORT)")
		cmd.Flags().BoolVar(&reaperDryRun, "dry-run", false, "Report what would happen without acting")
	}

	// JSON output flag for single-db commands
	for _, cmd := range []*cobra.Command{reaperScanCmd, reaperReapCmd, reaperPurgeCmd, reaperAutoCloseCmd, reaperDatabasesCmd} {
		cmd.Flags().BoolVar(&reaperJSON, "json", false, "Output as JSON")
	}

	// Threshold flags
	for _, cmd := range []*cobra.Command{reaperScanCmd, reaperReapCmd, reaperRunCmd} {
		cmd.Flags().StringVar(&reaperMaxAge, "max-age", "24h", "Max wisp age before reaping")
	}
	for _, cmd := range []*cobra.Command{reaperScanCmd, reaperPurgeCmd, reaperRunCmd} {
		cmd.Flags().StringVar(&reaperPurgeAge, "purge-age", "168h", "Max closed wisp age before purging (7d)")
		cmd.Flags().StringVar(&reaperMailAge, "mail-age", "168h", "Max closed mail age before purging (7d)")
	}
	for _, cmd := range []*cobra.Command{reaperScanCmd, reaperAutoCloseCmd, reaperRunCmd} {
		cmd.Flags().StringVar(&reaperStaleAge, "stale-age", "720h", "Max issue staleness before auto-close (30d)")
	}

	reaperCmd.AddCommand(reaperDatabasesCmd)
	reaperCmd.AddCommand(reaperScanCmd)
	reaperCmd.AddCommand(reaperReapCmd)
	reaperCmd.AddCommand(reaperPurgeCmd)
	reaperCmd.AddCommand(reaperAutoCloseCmd)
	reaperCmd.AddCommand(reaperRunCmd)

	rootCmd.AddCommand(reaperCmd)
}
