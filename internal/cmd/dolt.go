package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var doltCmd = &cobra.Command{
	Use:     "dolt",
	GroupID: GroupServices,
	Short:   "Manage the Dolt SQL server",
	RunE:    requireSubcommand,
	Long: `Manage the Dolt SQL server for Gas Town beads.

The Dolt server provides multi-client access to all rig databases,
avoiding the single-writer limitation of embedded Dolt mode.

Server configuration:
  - Port: 3307 (avoids conflict with MySQL on 3306)
  - User: root (default Dolt user, no password for localhost)
  - Data directory: .dolt-data/ (contains all rig databases)

Each rig (hq, gastown, beads) has its own database subdirectory.`,
}

var doltInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize and repair Dolt workspace configuration",
	Long: `Verify and repair the Dolt workspace configuration.

This command scans all rig metadata.json files for Dolt server configuration
and ensures the referenced databases actually exist. It fixes the broken state
where metadata.json says backend=dolt but the database is missing from .dolt-data/.

For each broken workspace, it will:
  1. Check if local .beads/dolt/ data exists and migrate it
  2. Otherwise, create a fresh database in .dolt-data/

This is safe to run multiple times (idempotent). It will not modify workspaces
that are already healthy.`,
	RunE: runDoltInit,
}

var doltStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Dolt server",
	Long: `Start the Dolt SQL server in the background.

The server will run until stopped with 'gt dolt stop'.`,
	RunE: runDoltStart,
}

var doltStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Dolt server",
	Long:  `Stop the running Dolt SQL server.`,
	RunE:  runDoltStop,
}

var doltStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Dolt server status",
	Long:  `Show the current status of the Dolt SQL server.`,
	RunE:  runDoltStatus,
}

var doltLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View Dolt server logs",
	Long:  `View the Dolt server log file.`,
	RunE:  runDoltLogs,
}

var doltSQLCmd = &cobra.Command{
	Use:   "sql",
	Short: "Open Dolt SQL shell",
	Long: `Open an interactive SQL shell to the Dolt database.

Works in both embedded mode (no server) and server mode.
For multi-client access, start the server first with 'gt dolt start'.`,
	RunE: runDoltSQL,
}

var doltInitRigCmd = &cobra.Command{
	Use:   "init-rig <name>",
	Short: "Initialize a new rig database",
	Long: `Initialize a new rig database in the Dolt data directory.

Each rig (e.g., gastown, beads) gets its own database that will be
served by the Dolt server. The rig name becomes the database name
when connecting via MySQL protocol.

Example:
  gt dolt init-rig gastown
  gt dolt init-rig beads`,
	Args: cobra.ExactArgs(1),
	RunE: runDoltInitRig,
}

var doltListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available rig databases",
	Long:  `List all rig databases in the Dolt data directory.`,
	RunE:  runDoltList,
}

var doltMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate existing dolt databases to centralized data directory",
	Long: `Migrate existing dolt databases from .beads/dolt/ locations to the
centralized .dolt-data/ directory structure.

This command will:
1. Detect existing dolt databases in .beads/dolt/ directories
2. Move them to .dolt-data/<rigname>/
3. Remove the old empty directories

Use --dry-run to preview what would be moved (source/target paths and sizes)
without making any changes.

After migration, start the server with 'gt dolt start'.`,
	RunE: runDoltMigrate,
}

var doltFixMetadataCmd = &cobra.Command{
	Use:   "fix-metadata",
	Short: "Update metadata.json in all rig .beads directories",
	Long: `Ensure all rig .beads/metadata.json files have correct Dolt server configuration.

This fixes the split-brain problem where bd falls back to local embedded databases
instead of connecting to the centralized Dolt server. It updates metadata.json with:
  - backend: "dolt"
  - dolt_mode: "server"
  - dolt_database: "<rigname>"

Safe to run multiple times (idempotent). Preserves any existing fields in metadata.json.`,
	RunE: runDoltFixMetadata,
}

var doltRecoverCmd = &cobra.Command{
	Use:   "recover",
	Short: "Detect and recover from Dolt read-only state",
	Long: `Detect if the Dolt server is in read-only mode and attempt recovery.

When the Dolt server enters read-only mode (e.g., from concurrent write
contention on the storage manifest), all write operations fail. This command:

  1. Probes the server to detect read-only state
  2. Stops the server if read-only
  3. Restarts the server
  4. Verifies recovery with a write probe

If the server is already writable, this is a no-op.

The daemon performs this check automatically every 30 seconds. Use this command
for immediate recovery without waiting for the daemon's health check loop.`,
	RunE: runDoltRecover,
}

var doltSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Push Dolt databases to DoltHub remotes",
	Long: `Push all local Dolt databases to their configured DoltHub remotes.

This command automates the tedious process of pushing each database individually:
  1. Stops the Dolt server (required for CLI push)
  2. Optionally purges closed ephemeral beads (--gc)
  3. Iterates databases in .dolt-data/
  4. For each database with a configured remote, runs dolt push
  5. Reports success/failure per database
  6. Restarts the Dolt server

Use --db to sync a single database, --dry-run to preview, or --force for force-push.
Use --gc to purge closed ephemeral beads (wisps, convoys) before pushing.

Examples:
  gt dolt sync                # Push all databases with remotes
  gt dolt sync --dry-run      # Preview what would be pushed
  gt dolt sync --db gastown   # Push only the gastown database
  gt dolt sync --force        # Force-push all databases
  gt dolt sync --gc           # Purge closed ephemeral beads, then push
  gt dolt sync --gc --dry-run # Preview purge + push without changes`,
	RunE: runDoltSync,
}

var doltCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove orphaned databases from .dolt-data/",
	Long: `Detect and remove orphaned databases from the .dolt-data/ directory.

An orphaned database is one that exists in .dolt-data/ but is not referenced
by any rig's metadata.json. These are typically left over from partial setups,
renamed databases, or failed migrations.

Use --dry-run to preview what would be removed without making changes.

Examples:
  gt dolt cleanup             # Remove all orphaned databases
  gt dolt cleanup --dry-run   # Preview what would be removed`,
	RunE: runDoltCleanup,
}

var doltRollbackCmd = &cobra.Command{
	Use:   "rollback [backup-dir]",
	Short: "Restore .beads directories from a migration backup",
	Long: `Roll back a migration by restoring .beads directories from a backup.

If no backup directory is specified, the most recent migration-backup-TIMESTAMP/
directory is used automatically.

This command will:
1. Stop the Dolt server if running
2. Find the specified (or most recent) backup
3. Restore all .beads directories from the backup
4. Reset metadata.json files to their pre-migration state
5. Validate the restored state with bd list

The backup directory is expected to be in the format created by the migration
formula's backup step (migration-backup-YYYYMMDD-HHMMSS/).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDoltRollback,
}

var (
	doltLogLines     int
	doltLogFollow    bool
	doltMigrateDry   bool
	doltCleanupDry   bool
	doltRollbackDry  bool
	doltRollbackList bool
	doltSyncDry      bool
	doltSyncForce    bool
	doltSyncDB       string
	doltSyncGC       bool
)

func init() {
	doltCmd.AddCommand(doltInitCmd)
	doltCmd.AddCommand(doltStartCmd)
	doltCmd.AddCommand(doltStopCmd)
	doltCmd.AddCommand(doltStatusCmd)
	doltCmd.AddCommand(doltLogsCmd)
	doltCmd.AddCommand(doltSQLCmd)
	doltCmd.AddCommand(doltInitRigCmd)
	doltCmd.AddCommand(doltListCmd)
	doltCmd.AddCommand(doltMigrateCmd)
	doltCmd.AddCommand(doltFixMetadataCmd)
	doltCmd.AddCommand(doltRecoverCmd)
	doltCmd.AddCommand(doltCleanupCmd)
	doltCmd.AddCommand(doltRollbackCmd)
	doltCmd.AddCommand(doltSyncCmd)

	doltCleanupCmd.Flags().BoolVar(&doltCleanupDry, "dry-run", false, "Preview what would be removed without making changes")

	doltLogsCmd.Flags().IntVarP(&doltLogLines, "lines", "n", 50, "Number of lines to show")
	doltLogsCmd.Flags().BoolVarP(&doltLogFollow, "follow", "f", false, "Follow log output")

	doltMigrateCmd.Flags().BoolVar(&doltMigrateDry, "dry-run", false, "Preview what would be migrated without making changes")

	doltRollbackCmd.Flags().BoolVar(&doltRollbackDry, "dry-run", false, "Show what would be restored without making changes")
	doltRollbackCmd.Flags().BoolVar(&doltRollbackList, "list", false, "List available backups and exit")

	doltSyncCmd.Flags().BoolVar(&doltSyncDry, "dry-run", false, "Preview what would be pushed without pushing")
	doltSyncCmd.Flags().BoolVar(&doltSyncForce, "force", false, "Force-push to remotes")
	doltSyncCmd.Flags().StringVar(&doltSyncDB, "db", "", "Sync a single database instead of all")
	doltSyncCmd.Flags().BoolVar(&doltSyncGC, "gc", false, "Purge closed ephemeral beads before push (requires bd purge)")

	rootCmd.AddCommand(doltCmd)
}

func runDoltStart(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)
	if config.IsRemote() {
		return fmt.Errorf("Dolt server is remote (%s) — start/stop managed externally", config.HostPort())
	}

	// Check for databases before starting — user-facing guard for manual starts.
	// Internal callers (install, migrate) may legitimately start with an empty
	// data dir and create databases afterward via bd init.
	databases, _ := doltserver.ListDatabases(townRoot)
	if len(databases) == 0 {
		return fmt.Errorf("no databases found in %s\nInitialize with: gt dolt init-rig <name>", config.DataDir)
	}

	if err := doltserver.Start(townRoot); err != nil {
		return err
	}

	// Get state for display
	state, _ := doltserver.LoadState(townRoot)

	fmt.Printf("%s Dolt server started (PID %d, port %d)\n",
		style.Bold.Render("✓"), state.PID, config.Port)
	fmt.Printf("  Data dir: %s\n", state.DataDir)
	fmt.Printf("  Databases: %s\n", style.Dim.Render(strings.Join(state.Databases, ", ")))
	fmt.Printf("  Connection: %s\n", style.Dim.Render(doltserver.GetConnectionString(townRoot)))

	// Verify all filesystem databases are actually served by the SQL server.
	// Use retry since Start() only waits 500ms — DBs may still be loading.
	served, missing, verifyErr := doltserver.VerifyDatabasesWithRetry(townRoot, 5)
	if verifyErr != nil {
		fmt.Printf("  %s Could not verify databases: %v\n", style.Dim.Render("⚠"), verifyErr)
	} else if len(missing) > 0 {
		fmt.Printf("\n%s Some databases exist on disk but are NOT served:\n", style.Bold.Render("⚠"))
		for _, db := range missing {
			fmt.Printf("  - %s\n", db)
		}
		fmt.Printf("\n  Served: %v\n", served)
		fmt.Printf("  This usually means the database has a stale manifest.\n")
		fmt.Printf("  Try: %s\n", style.Dim.Render("cd ~/gt/.dolt-data/<db> && dolt fsck --repair"))
	} else {
		fmt.Printf("  %s All %d databases verified\n", style.Bold.Render("✓"), len(served))
	}

	return nil
}

func runDoltStop(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)
	if config.IsRemote() {
		return fmt.Errorf("Dolt server is remote (%s) — start/stop managed externally", config.HostPort())
	}

	_, pid, _ := doltserver.IsRunning(townRoot)

	if err := doltserver.Stop(townRoot); err != nil {
		return err
	}

	fmt.Printf("%s Dolt server stopped (was PID %d)\n", style.Bold.Render("✓"), pid)
	return nil
}

func runDoltStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	running, pid, err := doltserver.IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking server status: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)

	if config.IsRemote() {
		if running {
			fmt.Printf("%s Dolt server is %s (remote: %s)\n",
				style.Bold.Render("●"),
				style.Bold.Render("reachable"),
				config.HostPort())
		} else {
			fmt.Printf("%s Dolt server is %s (remote: %s)\n",
				style.Dim.Render("○"),
				"not reachable",
				config.HostPort())
		}
		fmt.Printf("  Connection: %s\n", doltserver.GetConnectionString(townRoot))
		if running {
			metrics := doltserver.GetHealthMetrics(townRoot)
			fmt.Printf("\n  %s\n", style.Bold.Render("Resource Metrics:"))
			fmt.Printf("    Query latency: %v\n", metrics.QueryLatency.Round(time.Millisecond))
			fmt.Printf("    Connections:   %d / %d (%.0f%%)\n",
				metrics.Connections, metrics.MaxConnections, metrics.ConnectionPct)
			if metrics.ReadOnly {
				fmt.Printf("\n  %s %s\n",
					style.Bold.Render("!!!"),
					style.Bold.Render("SERVER IS READ-ONLY — contact the remote server admin"))
			}
		}
		return nil
	}

	if running {
		fmt.Printf("%s Dolt server is %s (PID %d)\n",
			style.Bold.Render("●"),
			style.Bold.Render("running"),
			pid)

		// Load state for more details
		state, err := doltserver.LoadState(townRoot)
		if err == nil && !state.StartedAt.IsZero() {
			fmt.Printf("  Started: %s\n", state.StartedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("  Port: %d\n", state.Port)
			fmt.Printf("  Data dir: %s\n", state.DataDir)
			if len(state.Databases) > 0 {
				fmt.Printf("  Databases:\n")
				for _, db := range state.Databases {
					fmt.Printf("    - %s\n", db)
				}
			}
			fmt.Printf("  Connection: %s\n", doltserver.GetConnectionString(townRoot))
		}

		// Resource metrics
		metrics := doltserver.GetHealthMetrics(townRoot)
		fmt.Printf("\n  %s\n", style.Bold.Render("Resource Metrics:"))
		fmt.Printf("    Query latency: %v\n", metrics.QueryLatency.Round(time.Millisecond))
		fmt.Printf("    Connections:   %d / %d (%.0f%%)\n",
			metrics.Connections, metrics.MaxConnections, metrics.ConnectionPct)
		fmt.Printf("    Disk usage:    %s\n", metrics.DiskUsageHuman)
		if metrics.ReadOnly {
			fmt.Printf("\n  %s %s\n",
				style.Bold.Render("!!!"),
				style.Bold.Render("SERVER IS READ-ONLY — run 'gt dolt recover' to restart"))
		}

		// Verify all filesystem databases are actually served.
		_, missing, verifyErr := doltserver.VerifyDatabases(townRoot)
		if verifyErr != nil {
			fmt.Printf("\n  %s Database verification failed: %v\n", style.Bold.Render("!"), verifyErr)
		} else if len(missing) > 0 {
			fmt.Printf("\n  %s %s\n", style.Bold.Render("!!!"),
				style.Bold.Render("MISSING DATABASES — exist on disk but not served:"))
			for _, db := range missing {
				fmt.Printf("    - %s\n", db)
			}
			fmt.Printf("  Try: cd ~/gt/.dolt-data/<db> && dolt fsck --repair\n")
		}

		// Check for orphaned databases
		orphans, orphanErr := doltserver.FindOrphanedDatabases(townRoot)
		if orphanErr == nil && len(orphans) > 0 {
			fmt.Printf("\n  %s %d orphaned database(s) (not referenced by any rig):\n",
				style.Bold.Render("!"), len(orphans))
			for _, o := range orphans {
				fmt.Printf("    - %s (%s)\n", o.Name, formatBytes(o.SizeBytes))
			}
			fmt.Printf("  Clean up with: %s\n", style.Dim.Render("gt dolt cleanup"))
		}

		if len(metrics.Warnings) > 0 {
			fmt.Printf("\n  %s\n", style.Bold.Render("Warnings:"))
			for _, w := range metrics.Warnings {
				fmt.Printf("    %s %s\n", style.Bold.Render("!"), w)
			}
		}
	} else {
		fmt.Printf("%s Dolt server is %s\n",
			style.Dim.Render("○"),
			"not running")

		// List available databases
		databases, _ := doltserver.ListDatabases(townRoot)
		if len(databases) == 0 {
			fmt.Printf("\n%s No rig databases found in %s\n",
				style.Bold.Render("!"),
				config.DataDir)
			fmt.Printf("  Initialize with: %s\n", style.Dim.Render("gt dolt init-rig <name>"))
		} else {
			fmt.Printf("\nAvailable databases in %s:\n", config.DataDir)
			for _, db := range databases {
				fmt.Printf("  - %s\n", db)
			}
			fmt.Printf("\nStart with: %s\n", style.Dim.Render("gt dolt start"))
		}
	}

	return nil
}

func runDoltLogs(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)

	if _, err := os.Stat(config.LogFile); os.IsNotExist(err) {
		return fmt.Errorf("no log file found at %s", config.LogFile)
	}

	if doltLogFollow {
		// Use tail -f for following
		tailCmd := exec.Command("tail", "-f", config.LogFile)
		tailCmd.Stdout = os.Stdout
		tailCmd.Stderr = os.Stderr
		return tailCmd.Run()
	}

	// Use tail -n for last N lines
	tailCmd := exec.Command("tail", "-n", strconv.Itoa(doltLogLines), config.LogFile)
	tailCmd.Stdout = os.Stdout
	tailCmd.Stderr = os.Stderr
	return tailCmd.Run()
}

func runDoltSQL(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)

	// Check if server is running - if so, connect via Dolt SQL client
	running, _, _ := doltserver.IsRunning(townRoot)
	if running {
		// Connect to running server using dolt sql client
		// Using --no-tls since server doesn't have TLS configured
		host := config.Host
		if host == "" {
			host = "127.0.0.1"
		}
		sqlArgs := []string{
			"--host", host,
			"--port", strconv.Itoa(config.Port),
			"--user", config.User,
			"--no-tls",
			"sql",
		}
		sqlCmd := exec.Command("dolt", sqlArgs...)
		if config.Password != "" {
			sqlCmd.Env = append(os.Environ(), "DOLT_CLI_PASSWORD="+config.Password)
		}
		sqlCmd.Stdin = os.Stdin
		sqlCmd.Stdout = os.Stdout
		sqlCmd.Stderr = os.Stderr
		return sqlCmd.Run()
	}

	// Server not running - list databases and pick first one for embedded mode
	databases, err := doltserver.ListDatabases(townRoot)
	if err != nil {
		return fmt.Errorf("listing databases: %w", err)
	}

	if len(databases) == 0 {
		return fmt.Errorf("no databases found in %s\nInitialize with: gt dolt init-rig <name>", config.DataDir)
	}

	// Use first database for embedded SQL shell
	dbDir := doltserver.RigDatabaseDir(townRoot, databases[0])
	fmt.Printf("Using database: %s (start server with 'gt dolt start' for multi-database access)\n\n", databases[0])

	sqlCmd := exec.Command("dolt", "sql")
	sqlCmd.Dir = dbDir
	sqlCmd.Stdin = os.Stdin
	sqlCmd.Stdout = os.Stdout
	sqlCmd.Stderr = os.Stderr

	return sqlCmd.Run()
}

func runDoltInitRig(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	rigName := args[0]

	serverWasRunning, created, err := doltserver.InitRig(townRoot, rigName)
	if err != nil {
		return err
	}

	config := doltserver.DefaultConfig(townRoot)
	rigDir := doltserver.RigDatabaseDir(townRoot, rigName)

	if !created {
		fmt.Printf("%s Rig database %q already exists (no-op)\n", style.Bold.Render("✓"), rigName)
		fmt.Printf("  Location: %s\n", rigDir)
		return nil
	}

	fmt.Printf("%s Initialized rig database %q\n", style.Bold.Render("✓"), rigName)
	fmt.Printf("  Location: %s\n", rigDir)
	fmt.Printf("  Data dir: %s\n", config.DataDir)

	if serverWasRunning {
		fmt.Printf("  Server: %s\n", style.Bold.Render("database registered with running server"))
	} else {
		fmt.Printf("\nStart server with: %s\n", style.Dim.Render("gt dolt start"))
	}

	return nil
}

func runDoltInit(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find workspaces with broken Dolt configuration
	broken := doltserver.FindBrokenWorkspaces(townRoot)

	// Check for orphaned databases regardless of broken workspaces
	orphans, orphanErr := doltserver.FindOrphanedDatabases(townRoot)

	if len(broken) == 0 {
		// Also check if there are any databases at all
		databases, _ := doltserver.ListDatabases(townRoot)
		if len(databases) == 0 {
			fmt.Println("No Dolt databases found and no workspaces configured for Dolt.")
			fmt.Printf("\nInitialize a rig database with: %s\n", style.Dim.Render("gt dolt init-rig <name>"))
		} else {
			fmt.Printf("%s All workspaces healthy (%d database(s) verified)\n",
				style.Bold.Render("✓"), len(databases))
		}

		// Report orphans even when workspaces are healthy
		if orphanErr == nil && len(orphans) > 0 {
			fmt.Printf("\n%s %d orphaned database(s) in .dolt-data/ (not referenced by any rig):\n",
				style.Bold.Render("!"), len(orphans))
			for _, o := range orphans {
				fmt.Printf("  - %s (%s)\n", o.Name, formatBytes(o.SizeBytes))
			}
			fmt.Printf("\nClean up with: %s\n", style.Dim.Render("gt dolt cleanup"))
		}

		return nil
	}

	fmt.Printf("Found %d workspace(s) with broken Dolt configuration:\n\n", len(broken))

	repaired := 0
	for _, ws := range broken {
		fmt.Printf("  %s %s: metadata.json → database %q (missing from .dolt-data/)\n",
			style.Bold.Render("!"), ws.RigName, ws.ConfiguredDB)
		if ws.HasLocalData {
			fmt.Printf("    Local data found at %s\n", style.Dim.Render(ws.LocalDataPath))
		}

		action, err := doltserver.RepairWorkspace(townRoot, ws)
		if err != nil {
			fmt.Printf("    %s Repair failed: %v\n", style.Bold.Render("✗"), err)
			continue
		}

		fmt.Printf("    %s Repaired: %s\n", style.Bold.Render("✓"), action)
		repaired++
	}

	if repaired > 0 {
		fmt.Printf("\n%s Repaired %d/%d workspace(s)\n", style.Bold.Render("✓"), repaired, len(broken))
	}

	// Report orphans after repairs
	if orphanErr == nil && len(orphans) > 0 {
		fmt.Printf("\n%s %d orphaned database(s) in .dolt-data/ (not referenced by any rig):\n",
			style.Bold.Render("!"), len(orphans))
		for _, o := range orphans {
			fmt.Printf("  - %s (%s)\n", o.Name, formatBytes(o.SizeBytes))
		}
		fmt.Printf("\nClean up with: %s\n", style.Dim.Render("gt dolt cleanup"))
	}

	return nil
}

func runDoltCleanup(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	orphans, err := doltserver.FindOrphanedDatabases(townRoot)
	if err != nil {
		return fmt.Errorf("finding orphaned databases: %w", err)
	}

	if len(orphans) == 0 {
		fmt.Printf("%s No orphaned databases found in .dolt-data/\n", style.Bold.Render("✓"))
		return nil
	}

	fmt.Printf("Found %d orphaned database(s) in .dolt-data/:\n\n", len(orphans))
	for _, o := range orphans {
		fmt.Printf("  %s %s (%s)\n", style.Bold.Render("!"), o.Name, formatBytes(o.SizeBytes))
		fmt.Printf("    %s\n", style.Dim.Render(o.Path))
	}

	if doltCleanupDry {
		fmt.Println("\nDry run: no changes made.")
		return nil
	}

	fmt.Println()
	removed := 0
	for _, o := range orphans {
		if err := doltserver.RemoveDatabase(townRoot, o.Name); err != nil {
			fmt.Printf("  %s Failed to remove %s: %v\n", style.Bold.Render("✗"), o.Name, err)
			continue
		}
		fmt.Printf("  %s Removed %s\n", style.Bold.Render("✓"), o.Name)
		removed++
	}

	fmt.Printf("\n%s Removed %d/%d orphaned database(s)\n",
		style.Bold.Render("✓"), removed, len(orphans))

	return nil
}

func runDoltList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)
	databases, err := doltserver.ListDatabases(townRoot)
	if err != nil {
		return fmt.Errorf("listing databases: %w", err)
	}

	if len(databases) == 0 {
		fmt.Printf("No rig databases found in %s\n", config.DataDir)
		fmt.Printf("\nInitialize with: %s\n", style.Dim.Render("gt dolt init-rig <name>"))
		return nil
	}

	fmt.Printf("Rig databases in %s:\n\n", config.DataDir)
	for _, db := range databases {
		dbDir := doltserver.RigDatabaseDir(townRoot, db)
		fmt.Printf("  %s\n    %s\n", style.Bold.Render(db), style.Dim.Render(dbDir))
	}

	return nil
}

func runDoltMigrate(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)
	if config.IsRemote() {
		return fmt.Errorf("Dolt server is remote (%s) — migration requires local server access", config.HostPort())
	}

	// Check if daemon is running - must stop first to avoid race conditions.
	// The daemon spawns many bd processes via gt status heartbeats. If these
	// run concurrently with migration, race conditions occur between old
	// old and new backends.
	daemonRunning, _, _ := daemon.IsRunning(townRoot)
	if daemonRunning {
		return fmt.Errorf("Gas Town daemon is running. Stop it first with: gt daemon stop\n\nThe daemon spawns bd processes that can race with migration.\nStop the daemon, run migration, then restart it.")
	}

	// Check if Dolt server is running - must stop first
	running, _, _ := doltserver.IsRunning(townRoot)
	if running {
		return fmt.Errorf("Dolt server is running. Stop it first with: gt dolt stop")
	}

	// Find databases to migrate
	migrations := doltserver.FindMigratableDatabases(townRoot)
	if len(migrations) == 0 {
		fmt.Println("No databases found to migrate.")
		return nil
	}

	fmt.Printf("Found %d database(s) to migrate:\n\n", len(migrations))
	for _, m := range migrations {
		sizeStr := dirSizeHuman(m.SourcePath)
		fmt.Printf("  %s (%s)\n", m.SourcePath, sizeStr)
		fmt.Printf("    → %s\n\n", m.TargetPath)
	}

	if doltMigrateDry {
		fmt.Println("Dry run: no changes made.")
		return nil
	}

	// Perform migrations
	for _, m := range migrations {
		fmt.Printf("Migrating %s...\n", m.RigName)
		if err := doltserver.MigrateRigFromBeads(townRoot, m.RigName, m.SourcePath); err != nil {
			return fmt.Errorf("migrating %s: %w", m.RigName, err)
		}
		fmt.Printf("  %s Migrated to %s\n", style.Bold.Render("✓"), m.TargetPath)
	}

	// Update metadata.json for all migrated rigs
	updated, metaErrs := doltserver.EnsureAllMetadata(townRoot)
	if len(updated) > 0 {
		fmt.Printf("\nUpdated metadata.json for: %s\n", strings.Join(updated, ", "))
	}
	for _, err := range metaErrs {
		fmt.Printf("  %s metadata.json update failed: %v\n", style.Dim.Render("⚠"), err)
	}

	fmt.Printf("\n%s Migration complete.\n", style.Bold.Render("✓"))

	// Auto-start the Dolt server to prevent split-brain risk.
	// If bd commands are run before the server starts, they may silently create
	// isolated local databases instead of connecting to the centralized server.
	fmt.Printf("\nStarting Dolt server to prevent split-brain risk...\n")
	if err := doltserver.Start(townRoot); err != nil {
		fmt.Printf("\n%s Could not auto-start Dolt server: %v\n", style.Bold.Render("⚠"), err)
		fmt.Printf("\n%s WARNING: Do NOT run bd commands until the server is started!\n", style.Bold.Render("⚠"))
		fmt.Printf("  Running bd before 'gt dolt start' risks split-brain: bd may create an\n")
		fmt.Printf("  isolated local database instead of connecting to the centralized server.\n")
		fmt.Printf("\n  Start manually with: %s\n", style.Dim.Render("gt dolt start"))
	} else {
		state, _ := doltserver.LoadState(townRoot)
		fmt.Printf("%s Dolt server started (PID %d)\n", style.Bold.Render("✓"), state.PID)

		// Verify the server is actually serving all databases that exist on disk.
		// Dolt silently skips databases with stale manifests after migration,
		// so filesystem discovery and SQL discovery can diverge.
		// Use retry since the server may still be loading databases after Start().
		served, missing, verifyErr := doltserver.VerifyDatabasesWithRetry(townRoot, 5)
		if verifyErr != nil {
			fmt.Printf("  %s Could not verify databases: %v\n", style.Dim.Render("⚠"), verifyErr)
			fmt.Printf("  Migration may be incomplete. Verify manually with: %s\n", style.Dim.Render("gt dolt status"))
			return fmt.Errorf("database verification failed after migration: %w", verifyErr)
		} else if len(missing) > 0 {
			fmt.Printf("\n%s Some databases exist on disk but are NOT served by Dolt:\n", style.Bold.Render("⚠"))
			for _, db := range missing {
				fmt.Printf("  - %s\n", db)
			}
			fmt.Printf("\n  Served databases: %v\n", served)
			fmt.Printf("\n  This usually means the database has a stale manifest from migration.\n")
			fmt.Printf("  To fix, try:\n")
			fmt.Printf("    1. Stop the server:  %s\n", style.Dim.Render("gt dolt stop"))
			fmt.Printf("    2. Repair the DB:    %s\n", style.Dim.Render("cd ~/gt/.dolt-data/<db> && dolt fsck --repair"))
			fmt.Printf("    3. Restart:           %s\n", style.Dim.Render("gt dolt start"))
			return fmt.Errorf("migration incomplete: %d database(s) exist on disk but are not served: %v", len(missing), missing)
		} else {
			fmt.Printf("  %s All %d databases verified as served\n", style.Bold.Render("✓"), len(served))
		}
	}

	return nil
}

// dirSizeHuman returns a human-readable size string for a directory tree.
func dirSizeHuman(path string) string {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return formatBytes(total)
}

func runDoltFixMetadata(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	updated, errs := doltserver.EnsureAllMetadata(townRoot)

	if len(updated) > 0 {
		fmt.Printf("%s Updated metadata.json for %d rig(s):\n", style.Bold.Render("✓"), len(updated))
		for _, name := range updated {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(errs) > 0 {
		fmt.Println()
		for _, err := range errs {
			fmt.Printf("  %s %v\n", style.Dim.Render("⚠"), err)
		}
	}

	if len(updated) == 0 && len(errs) == 0 {
		fmt.Println("No rig databases found. Nothing to update.")
	}

	return nil
}

func runDoltRecover(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)
	if config.IsRemote() {
		return fmt.Errorf("Dolt server is remote (%s) — recovery requires local server access", config.HostPort())
	}

	running, _, _ := doltserver.IsRunning(townRoot)
	if !running {
		return fmt.Errorf("Dolt server is not running — start with 'gt dolt start'")
	}

	readOnly, err := doltserver.CheckReadOnly(townRoot)
	if err != nil {
		return fmt.Errorf("read-only probe failed: %w", err)
	}

	if !readOnly {
		fmt.Printf("%s Dolt server is writable (no recovery needed)\n", style.Bold.Render("✓"))
		return nil
	}

	if err := doltserver.RecoverReadOnly(townRoot); err != nil {
		return fmt.Errorf("recovery failed: %w", err)
	}

	fmt.Printf("%s Dolt server recovered from read-only state\n", style.Bold.Render("✓"))
	return nil
}

func runDoltRollback(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)
	if config.IsRemote() {
		return fmt.Errorf("Dolt server is remote (%s) — rollback requires local server access", config.HostPort())
	}

	// Find available backups
	backups, err := doltserver.FindBackups(townRoot)
	if err != nil {
		return fmt.Errorf("finding backups: %w", err)
	}

	if len(backups) == 0 {
		return fmt.Errorf("no migration backups found in %s\nExpected directories matching: migration-backup-YYYYMMDD-HHMMSS/", townRoot)
	}

	// List mode: show available backups and exit
	if doltRollbackList {
		fmt.Printf("Available migration backups in %s:\n\n", townRoot)
		for i, b := range backups {
			label := ""
			if i == 0 {
				label = " (most recent)"
			}
			fmt.Printf("  %s%s\n", b.Timestamp, label)
			fmt.Printf("    %s\n", style.Dim.Render(b.Path))
			if b.Metadata != nil {
				if createdAt, ok := b.Metadata["created_at"]; ok {
					fmt.Printf("    Created: %v\n", createdAt)
				}
			}
		}
		return nil
	}

	// Determine which backup to use
	var backupPath string
	if len(args) > 0 {
		// User specified a backup directory
		backupPath = args[0]
		// Check if it's a relative path or timestamp
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			// Try as a timestamp suffix
			candidate := fmt.Sprintf("migration-backup-%s", args[0])
			candidatePath := fmt.Sprintf("%s/%s", townRoot, candidate)
			if _, err := os.Stat(candidatePath); err == nil {
				backupPath = candidatePath
			} else {
				return fmt.Errorf("backup not found: %s\nUse --list to see available backups", args[0])
			}
		}
	} else {
		// Use the most recent backup
		backupPath = backups[0].Path
	}

	fmt.Printf("Backup: %s\n", backupPath)

	// Dry-run mode: show what would be restored
	if doltRollbackDry {
		fmt.Printf("\n%s Dry run - no changes will be made\n\n", style.Bold.Render("!"))
		printBackupContents(backupPath, townRoot)
		return nil
	}

	// Stop Dolt server if running
	running, _, _ := doltserver.IsRunning(townRoot)
	if running {
		fmt.Println("Stopping Dolt server...")
		if err := doltserver.Stop(townRoot); err != nil {
			return fmt.Errorf("stopping Dolt server: %w", err)
		}
		fmt.Printf("%s Dolt server stopped\n", style.Bold.Render("✓"))
	}

	// Perform the rollback
	fmt.Println("\nRestoring from backup...")
	result, err := doltserver.RestoreFromBackup(townRoot, backupPath)
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	// Report results
	fmt.Println()
	if result.RestoredTown {
		fmt.Printf("  %s Restored town-level .beads\n", style.Bold.Render("✓"))
	}
	for _, rig := range result.RestoredRigs {
		fmt.Printf("  %s Restored %s/.beads\n", style.Bold.Render("✓"), rig)
	}
	for _, rig := range result.SkippedRigs {
		fmt.Printf("  %s Skipped %s (restore failed)\n", style.Dim.Render("⚠"), rig)
	}

	if len(result.MetadataReset) > 0 {
		fmt.Printf("\n  Metadata reset for: %s\n", strings.Join(result.MetadataReset, ", "))
	}

	// Validate restored state
	fmt.Println("\nValidating restored state...")
	validateCmd := exec.Command("bd", "list", "--limit", "5")
	validateCmd.Dir = townRoot
	output, validateErr := validateCmd.CombinedOutput()
	if validateErr != nil {
		fmt.Printf("  %s bd list returned an error: %v\n",
			style.Dim.Render("⚠"), validateErr)
		if len(output) > 0 {
			fmt.Printf("  %s\n", string(output))
		}
	} else {
		fmt.Printf("  %s bd list succeeded\n", style.Bold.Render("✓"))
		if len(output) > 0 {
			// Show first few lines of output
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				fmt.Printf("  %s\n", style.Dim.Render(line))
			}
		}
	}

	fmt.Printf("\n%s Rollback complete from %s\n", style.Bold.Render("✓"), backupPath)

	return nil
}

// printBackupContents shows what's in a backup directory for dry-run output.
func printBackupContents(backupPath, townRoot string) {
	// Check town-level backup
	townBackup := fmt.Sprintf("%s/town-beads", backupPath)
	if _, err := os.Stat(townBackup); err == nil {
		dst := fmt.Sprintf("%s/.beads", townRoot)
		fmt.Printf("  Would restore: %s\n", style.Dim.Render(dst))
		fmt.Printf("    From: %s\n", style.Dim.Render(townBackup))
	}

	// Check formula-style rig backups
	entries, err := os.ReadDir(backupPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "town-beads" || name == "rigs" {
			continue
		}
		if strings.HasSuffix(name, "-beads") {
			rigName := strings.TrimSuffix(name, "-beads")
			dst := fmt.Sprintf("%s/%s/.beads", townRoot, rigName)
			src := fmt.Sprintf("%s/%s", backupPath, name)
			fmt.Printf("  Would restore: %s\n", style.Dim.Render(dst))
			fmt.Printf("    From: %s\n", style.Dim.Render(src))
		}
	}

	// Check test-backup-style rig backups
	rigsDir := fmt.Sprintf("%s/rigs", backupPath)
	if rigEntries, err := os.ReadDir(rigsDir); err == nil {
		for _, entry := range rigEntries {
			if !entry.IsDir() {
				continue
			}
			rigName := entry.Name()
			beadsDir := fmt.Sprintf("%s/%s/.beads", rigsDir, rigName)
			if _, err := os.Stat(beadsDir); err != nil {
				continue
			}
			dst := fmt.Sprintf("%s/%s/.beads", townRoot, rigName)
			fmt.Printf("  Would restore: %s\n", style.Dim.Render(dst))
			fmt.Printf("    From: %s\n", style.Dim.Render(beadsDir))
		}
	}
}

func runDoltSync(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	config := doltserver.DefaultConfig(townRoot)
	if config.IsRemote() {
		return fmt.Errorf("Dolt server is remote (%s) — sync requires local server access", config.HostPort())
	}

	// Validate --db flag if set
	if doltSyncDB != "" && !doltserver.DatabaseExists(townRoot, doltSyncDB) {
		return fmt.Errorf("database %q not found in .dolt-data/\nRun 'gt dolt list' to see available databases", doltSyncDB)
	}

	// GC phase: purge closed ephemeral beads BEFORE stopping the server.
	// bd purge needs SQL access via the running Dolt server.
	purgeResults := make(map[string]struct {
		purged int
		err    error
	})
	if doltSyncGC {
		databases, listErr := doltserver.ListDatabases(townRoot)
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: --gc: could not list databases: %v\n", listErr)
		} else {
			for _, db := range databases {
				if doltSyncDB != "" && db != doltSyncDB {
					continue
				}
				purged, purgeErr := doltserver.PurgeClosedEphemerals(townRoot, db, doltSyncDry)
				purgeResults[db] = struct {
					purged int
					err    error
				}{purged, purgeErr}
			}
		}
	}

	// Check server state
	wasRunning, pid, _ := doltserver.IsRunning(townRoot)

	if wasRunning {
		fmt.Printf("Stopping Dolt server (PID %d)...\n", pid)
		if err := doltserver.Stop(townRoot); err != nil {
			return fmt.Errorf("stopping Dolt server: %w", err)
		}
		fmt.Printf("%s Dolt server stopped\n", style.Bold.Render("✓"))

		// Guarantee restart even if push fails
		defer func() {
			fmt.Printf("\nRestarting Dolt server...\n")
			if startErr := doltserver.Start(townRoot); startErr != nil {
				fmt.Printf("%s Failed to restart Dolt server: %v\n", style.Bold.Render("✗"), startErr)
				fmt.Printf("  Start manually with: %s\n", style.Dim.Render("gt dolt start"))
				return
			}
			// Start() now verifies the server is accepting connections,
			// so if we get here it's genuinely ready.
			fmt.Printf("%s Dolt server restarted (accepting connections)\n", style.Bold.Render("✓"))
		}()
	}

	opts := doltserver.SyncOptions{
		Force:  doltSyncForce,
		DryRun: doltSyncDry,
		Filter: doltSyncDB,
	}

	results := doltserver.SyncDatabases(townRoot, opts)

	if len(results) == 0 {
		fmt.Println("No databases to sync.")
		return nil
	}

	fmt.Printf("\nSyncing %d database(s)...\n", len(results))

	var pushed, skipped, failed, totalPurged int
	for _, r := range results {
		fmt.Println()
		// Show purge results if --gc was used
		if doltSyncGC {
			if pr, ok := purgeResults[r.Database]; ok {
				if pr.err != nil {
					fmt.Printf("  %s %s gc: %v\n", style.Bold.Render("!"), r.Database, pr.err)
				} else if pr.purged > 0 {
					fmt.Printf("  %s %s gc: purged %d closed ephemeral bead(s)\n", style.Bold.Render("✓"), r.Database, pr.purged)
					totalPurged += pr.purged
				}
			}
		}
		switch {
		case r.Pushed:
			fmt.Printf("  %s %s → origin main\n", style.Bold.Render("✓"), r.Database)
			fmt.Printf("    %s\n", style.Dim.Render(r.Remote))
			pushed++
		case r.DryRun:
			fmt.Printf("  %s %s → origin main (dry run)\n", style.Bold.Render("~"), r.Database)
			fmt.Printf("    %s\n", style.Dim.Render(r.Remote))
			pushed++ // count as would-push for summary
		case r.Skipped:
			fmt.Printf("  %s %s — no remote configured\n", style.Dim.Render("○"), r.Database)
			skipped++
		case r.Error != nil:
			fmt.Printf("  %s %s → origin main\n", style.Bold.Render("✗"), r.Database)
			fmt.Printf("    error: %v\n", r.Error)
			failed++
		}
	}

	summary := fmt.Sprintf("Summary: %d pushed, %d skipped, %d failed", pushed, skipped, failed)
	if doltSyncGC && totalPurged > 0 {
		summary += fmt.Sprintf(", %d purged", totalPurged)
	}
	fmt.Printf("\n%s\n", summary)

	if failed > 0 {
		return fmt.Errorf("%d database(s) failed to sync", failed)
	}
	return nil
}


