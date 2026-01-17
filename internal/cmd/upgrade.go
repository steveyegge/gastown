package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/upgrade"
	"github.com/steveyegge/gastown/internal/workspace"
)

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
4. Atomically replaces the current binary

If gt was installed via Homebrew, you'll be directed to use 'brew upgrade' instead.

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

func runUpgrade(cmd *cobra.Command, args []string) error {
	fmt.Println("Checking for updates...")

	// Get current binary path
	currentPath, err := upgrade.GetCurrentBinaryPath()
	if err != nil {
		return fmt.Errorf("locating current binary: %w", err)
	}

	// Check for Homebrew installation
	if upgrade.IsHomebrew(currentPath) {
		fmt.Println()
		fmt.Println("gt appears to be installed via Homebrew.")
		fmt.Printf("Run: %s\n", style.Bold.Render("brew upgrade gt"))
		return nil
	}

	// Fetch latest release info
	release, err := upgrade.FetchLatestRelease()
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	// Parse versions for comparison
	currentVer, err := upgrade.ParseVersion(Version)
	if err != nil {
		return fmt.Errorf("parsing current version: %w", err)
	}

	latestVer, err := upgrade.ParseVersion(release.TagName)
	if err != nil {
		return fmt.Errorf("parsing latest version: %w", err)
	}

	// Check if update is needed
	isUpToDate := !currentVer.LessThan(latestVer)
	if isUpToDate && !upgradeForce {
		fmt.Printf("%s Already at latest version (%s)\n", style.SuccessPrefix, Version)
		return nil
	}

	// Show update info
	if isUpToDate {
		fmt.Printf("Already at latest version (%s), --force specified\n", Version)
	} else {
		fmt.Printf("Update available: %s -> %s\n", Version, release.TagName)
	}

	// If just checking, stop here
	if upgradeCheck {
		return nil
	}

	// Find the appropriate asset
	asset, err := upgrade.SelectAsset(release)
	if err != nil {
		return fmt.Errorf("finding download: %w", err)
	}

	// Check workspace compatibility (if in a workspace)
	townRoot, _ := workspace.FindFromCwd()
	if townRoot != "" {
		// Fetch compatibility info
		compatInfo, err := upgrade.FetchCompatibilityInfo(release)
		if err != nil {
			// Non-fatal: log and continue
			fmt.Fprintf(os.Stderr, "%s Could not fetch compatibility info: %v\n", style.WarningPrefix, err)
		}

		// Check compatibility
		result := upgrade.CheckCompatibility(townRoot, Version, release, compatInfo)
		if !result.Compatible && !upgradeForce {
			fmt.Println()
			fmt.Println(upgrade.FormatCompatWarning(result))
			return nil
		}

		if !result.Compatible {
			fmt.Printf("%s Incompatible upgrade (--force specified), proceeding anyway\n", style.WarningPrefix)
		}
	}

	// Dry-run mode: show what would happen
	if upgradeDryRun {
		fmt.Println()
		fmt.Println("Dry run - would perform the following:")
		fmt.Printf("  Would upgrade: %s -> %s\n", Version, release.TagName)
		fmt.Printf("  Would download: %s (%s)\n", asset.Name, upgrade.FormatSize(asset.Size))
		fmt.Printf("  Would replace: %s\n", currentPath)
		return nil
	}

	// Download the asset
	fmt.Printf("Downloading %s...\n", release.TagName)
	archivePath, err := upgrade.Download(asset, func(downloaded, total int64) {
		if total > 0 {
			pct := float64(downloaded) / float64(total) * 100
			fmt.Printf("\r  %s / %s (%.0f%%)", upgrade.FormatSize(downloaded), upgrade.FormatSize(total), pct)
		}
	})
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}
	defer os.Remove(archivePath)
	fmt.Println() // newline after progress

	// Extract the binary
	fmt.Println("Extracting...")
	binaryPath, err := upgrade.Extract(archivePath)
	if err != nil {
		return fmt.Errorf("extracting: %w", err)
	}
	defer os.RemoveAll(binaryPath) // Clean up extract dir

	// Replace the current binary
	fmt.Println("Installing...")
	if err := upgrade.Replace(currentPath, binaryPath); err != nil {
		return fmt.Errorf("installing: %w", err)
	}

	// Update workspace version if in a workspace
	if townRoot != "" {
		if err := upgrade.SetWorkspaceVersion(townRoot, release.TagName); err != nil {
			// Non-fatal: log and continue
			fmt.Fprintf(os.Stderr, "%s Could not update workspace version: %v\n", style.WarningPrefix, err)
		}
	}

	fmt.Printf("%s Upgraded to %s\n", style.SuccessPrefix, release.TagName)
	return nil
}
