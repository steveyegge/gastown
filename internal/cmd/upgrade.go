package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/migrate"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/upgrade"
	"github.com/steveyegge/gastown/internal/workspace"
)

// ErrIncompatibleUpgrade is returned when an upgrade is blocked due to compatibility issues.
var ErrIncompatibleUpgrade = errors.New("upgrade blocked due to compatibility issues")

var (
	upgradeCheck  bool // --check: only check for updates
	upgradeForce  bool // --force: upgrade even if latest or incompatible
	upgradeDryRun bool // --dry-run: show what would happen
)

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	GroupID: GroupDiag,
	Short:   "Upgrade gt to the latest version",
	Long: `Upgrade gt to the latest version from GitHub releases.

This command:
1. Checks for the latest release on GitHub
2. Verifies compatibility with your workspace
3. Downloads the appropriate binary for your platform
4. Verifies the download checksum (mandatory for security)
5. Atomically replaces the current binary

If gt was installed via Homebrew, you'll be directed to use 'brew upgrade' instead.

Note: Checksum verification cannot be skipped for security reasons.

Examples:
  gt upgrade              # Upgrade to latest version
  gt upgrade --check      # Only check for updates, don't install
  gt upgrade --dry-run    # Show what would happen without making changes
  gt upgrade --force      # Upgrade even if already latest or incompatible`,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheck, "check", false, "Only check for updates, don't install")
	upgradeCmd.Flags().BoolVarP(&upgradeForce, "force", "f", false, "Upgrade even if already latest or incompatible")
	upgradeCmd.Flags().BoolVarP(&upgradeDryRun, "dry-run", "n", false, "Show what would happen without making changes")
	rootCmd.AddCommand(upgradeCmd)
}

// upgradeContext holds state shared across upgrade sub-functions.
type upgradeContext struct {
	ctx         context.Context
	currentPath string
	release     *upgrade.ReleaseInfo
	asset       *upgrade.Asset
	townRoot    string
	checksums   map[string]string
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	uctx := &upgradeContext{ctx: cmd.Context()}

	// Step 1: Check for updates and determine if upgrade is needed
	shouldContinue, err := checkForUpdates(uctx)
	if err != nil {
		return err
	}
	if !shouldContinue {
		return nil
	}

	// Step 2: Check workspace compatibility (if in a workspace)
	if err := checkWorkspaceCompatibility(uctx); err != nil {
		return err
	}

	// Step 3: Handle dry-run mode
	if upgradeDryRun {
		printDryRunInfo(uctx)
		return nil
	}

	// Step 4: Download and verify the binary
	archivePath, err := downloadAndVerify(uctx)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(archivePath) }()

	// Step 5: Install the binary
	if err := installBinary(uctx, archivePath); err != nil {
		return err
	}

	fmt.Printf("%s Upgraded to %s\n", style.SuccessPrefix, uctx.release.TagName)
	return nil
}

// checkForUpdates fetches release info and determines if an upgrade is needed.
// Returns true if the upgrade should continue, false if we should stop (already up to date).
func checkForUpdates(uctx *upgradeContext) (bool, error) {
	fmt.Println("Checking for updates...")

	// Get current binary path
	currentPath, err := upgrade.GetCurrentBinaryPath()
	if err != nil {
		return false, fmt.Errorf("locating current binary: %w", err)
	}
	uctx.currentPath = currentPath

	// Check for Homebrew installation
	if upgrade.IsHomebrew(currentPath) {
		fmt.Println()
		fmt.Println("gt appears to be installed via Homebrew.")
		fmt.Printf("Run: %s\n", style.Bold.Render("brew upgrade gt"))
		return false, nil
	}

	// Fetch latest release info
	release, err := upgrade.FetchLatestReleaseWithContext(uctx.ctx)
	if err != nil {
		return false, fmt.Errorf("checking for updates: %w", err)
	}
	uctx.release = release

	// Parse versions for comparison
	currentVer, err := upgrade.ParseVersion(Version)
	if err != nil {
		return false, fmt.Errorf("parsing current version: %w", err)
	}

	latestVer, err := upgrade.ParseVersion(release.TagName)
	if err != nil {
		return false, fmt.Errorf("parsing latest version: %w", err)
	}

	// Check if update is needed
	isUpToDate := !currentVer.LessThan(latestVer)
	if isUpToDate && !upgradeForce {
		fmt.Printf("%s Already at latest version (%s)\n", style.SuccessPrefix, Version)
		return false, nil
	}

	// Show update info
	if isUpToDate {
		fmt.Printf("Already at latest version (%s), --force specified\n", Version)
	} else {
		fmt.Printf("Update available: %s -> %s\n", Version, release.TagName)
	}

	// If just checking, stop here
	if upgradeCheck {
		return false, nil
	}

	// Find the appropriate asset
	asset, err := upgrade.SelectAsset(release)
	if err != nil {
		return false, fmt.Errorf("finding download: %w", err)
	}
	uctx.asset = asset

	return true, nil
}

// checkWorkspaceCompatibility verifies migration status and compatibility if in a workspace.
func checkWorkspaceCompatibility(uctx *upgradeContext) error {
	townRoot, _ := workspace.FindFromCwd()
	uctx.townRoot = townRoot

	if townRoot == "" {
		return nil // Not in a workspace, nothing to check
	}

	// Check if workspace migration is needed first
	migrationCheck, err := migrate.NeedsMigration(townRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s Could not check migration status: %v\n", style.WarningPrefix, err)
	}
	if migrationCheck != nil && migrationCheck.NeedsMigration && !upgradeForce {
		fmt.Println()
		fmt.Printf("%s Workspace migration required before upgrade\n", style.WarningPrefix)
		fmt.Printf("  Current layout: %s\n", migrationCheck.LayoutType)
		fmt.Printf("  Target version: %s\n", migrationCheck.TargetVersion)
		fmt.Println()
		fmt.Println("Run 'gt migrate' to migrate your workspace first,")
		fmt.Println("or use 'gt upgrade --force' to skip this check.")
		return ErrIncompatibleUpgrade
	}

	// Fetch compatibility info
	compatInfo, err := upgrade.FetchCompatibilityInfoWithContext(uctx.ctx, uctx.release)
	if err != nil {
		// Non-fatal: log and continue
		fmt.Fprintf(os.Stderr, "%s Could not fetch compatibility info: %v\n", style.WarningPrefix, err)
	}

	// Check compatibility
	result := upgrade.CheckCompatibility(Version, uctx.release, compatInfo)
	if !result.Compatible && !upgradeForce {
		fmt.Println()
		fmt.Println(upgrade.FormatCompatWarning(result))
		return ErrIncompatibleUpgrade
	}

	if !result.Compatible {
		fmt.Printf("%s Incompatible upgrade (--force specified), proceeding anyway\n", style.WarningPrefix)
	}

	return nil
}

// printDryRunInfo outputs what would happen without making changes.
func printDryRunInfo(uctx *upgradeContext) {
	fmt.Println()
	fmt.Println("Dry run - would perform the following:")
	fmt.Printf("  Would upgrade: %s -> %s\n", Version, uctx.release.TagName)
	fmt.Printf("  Would download: %s (%s)\n", uctx.asset.Name, upgrade.FormatSize(uctx.asset.Size))
	fmt.Printf("  Would replace: %s\n", uctx.currentPath)
}

// downloadAndVerify downloads the release asset and verifies its checksum.
// Returns the path to the downloaded archive.
func downloadAndVerify(uctx *upgradeContext) (string, error) {
	// Fetch checksums for verification
	// Note: Checksum verification is mandatory for security - cannot be skipped with --force
	fmt.Println("Fetching checksums...")
	checksums, checksumErr := upgrade.FetchChecksumWithContext(uctx.ctx, uctx.release)
	if checksumErr != nil {
		// Checksum verification is mandatory - cannot proceed without it
		fmt.Fprintf(os.Stderr, "%s Could not fetch checksums: %v\n", style.WarningPrefix, checksumErr)
		return "", fmt.Errorf("checksum verification unavailable (required for security)")
	}
	uctx.checksums = checksums

	// Download the asset
	fmt.Printf("Downloading %s...\n", uctx.release.TagName)
	archivePath, err := upgrade.DownloadWithContext(uctx.ctx, uctx.asset, func(downloaded, total int64) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			fmt.Printf("\r  %s / %s (%.0f%%)", upgrade.FormatSize(downloaded), upgrade.FormatSize(total), pct)
		}
	})
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}
	fmt.Println() // newline after progress

	// Verify checksum - mandatory for security
	expectedHash, ok := checksums[uctx.asset.Name]
	if !ok {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("no checksum found for %s in checksums file (required for security)", uctx.asset.Name)
	}
	fmt.Println("Verifying checksum...")
	if err := upgrade.VerifyChecksum(archivePath, expectedHash); err != nil {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("checksum verification failed: %w", err)
	}
	fmt.Println("  Checksum verified")

	return archivePath, nil
}

// installBinary extracts the archive and replaces the current binary.
func installBinary(uctx *upgradeContext, archivePath string) error {
	// Extract the binary
	fmt.Println("Extracting...")
	binaryPath, err := upgrade.Extract(archivePath)
	if err != nil {
		return fmt.Errorf("extracting: %w", err)
	}
	// Clean up extract directory (binaryPath is the file, parent is the temp dir)
	defer func() { _ = os.RemoveAll(filepath.Dir(binaryPath)) }()

	// Replace the current binary
	fmt.Println("Installing...")
	if err := upgrade.Replace(uctx.currentPath, binaryPath); err != nil {
		return fmt.Errorf("installing: %w", err)
	}

	// Update workspace version if in a workspace
	if uctx.townRoot != "" {
		if err := upgrade.SetWorkspaceVersion(uctx.townRoot, uctx.release.TagName); err != nil {
			// Non-fatal: log and continue
			fmt.Fprintf(os.Stderr, "%s Could not update workspace version: %v\n", style.WarningPrefix, err)
		}
	}

	return nil
}
