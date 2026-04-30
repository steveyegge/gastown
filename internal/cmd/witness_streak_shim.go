// Package cmd — `gt witness install-streak-shim` subcommand for ka-0g8 #2.
//
// Wires the existing bd-with-streak.sh wrapper (witness-owned at
// ~/gt/occultfusion/witness/scripts/bd-with-streak.sh — counts consecutive
// Dolt remote-push failures into the M4 metric) onto PATH as `bd-with-streak`,
// so witness can opt-in by calling `bd-with-streak push` instead of
// `bd push`.
//
// Munger ratify hq-wisp-2npadh (Director AFK proxy) — Option A (opt-in,
// scope-bound) over Options B/C. Tier B: Atlas self-claim OK. Reversible
// (uninstall sibling). 30d sunset/scope-broaden gate per ratify.
//
// Pure-symlink install. No bd binary modification. No global PATH override.
// Other crews unaffected — they continue to call standard `bd`.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

const (
	streakShimName       = "bd-with-streak"
	streakShimSourceRel  = "occultfusion/witness/scripts/bd-with-streak.sh"
	streakShimDefaultDir = ".local/bin"
)

var (
	streakShimInstallDir string
	streakShimDryRun     bool
)

var witnessInstallStreakShimCmd = &cobra.Command{
	Use:   "install-streak-shim",
	Short: "Install bd-with-streak shim onto PATH for opt-in witness M4 metric collection (ka-0g8 #2)",
	Long: `Install the bd-with-streak shim symlink onto PATH.

Wires the witness-owned bd-with-streak.sh wrapper (counts consecutive Dolt
remote-push failures into the M4 metric) onto PATH as 'bd-with-streak', so
witness can opt-in by calling 'bd-with-streak push' instead of 'bd push'.

Other crews are unaffected — standard 'bd' continues to work for everyone.

Default install location: ~/.local/bin/bd-with-streak (override with --dir).

Per ka-0g8 #2 Munger ratify hq-wisp-2npadh:
  - Option A (this command): opt-in symlink — Tier B, Atlas self-claim OK.
  - Option B (Witness-only PATH prefix): rejected (config drift risk).
  - Option C (global bd override): rejected (Tier C blast radius).

Idempotent: re-running with the symlink already correct is a no-op.
Reversible: ` + "`gt witness uninstall-streak-shim`" + ` removes the symlink.

Examples:
  gt witness install-streak-shim                         # default ~/.local/bin
  gt witness install-streak-shim --dir ~/bin             # custom install dir
  gt witness install-streak-shim --dry-run               # show what would happen`,
	Args: cobra.NoArgs,
	RunE: runWitnessInstallStreakShim,
}

var witnessUninstallStreakShimCmd = &cobra.Command{
	Use:   "uninstall-streak-shim",
	Short: "Remove the bd-with-streak shim symlink (reverses install-streak-shim)",
	Long: `Remove the bd-with-streak symlink from PATH.

Reversibility sibling for install-streak-shim. Removes only the symlink
this command would have created (refuses to delete a non-symlink or a
symlink pointing elsewhere — fail-soft to avoid clobbering user state).

Examples:
  gt witness uninstall-streak-shim
  gt witness uninstall-streak-shim --dir ~/bin
  gt witness uninstall-streak-shim --dry-run`,
	Args: cobra.NoArgs,
	RunE: runWitnessUninstallStreakShim,
}

func init() {
	witnessInstallStreakShimCmd.Flags().StringVar(&streakShimInstallDir, "dir", "",
		"Install dir override (default: ~/.local/bin)")
	witnessInstallStreakShimCmd.Flags().BoolVar(&streakShimDryRun, "dry-run", false,
		"Show what would happen without creating the symlink")

	witnessUninstallStreakShimCmd.Flags().StringVar(&streakShimInstallDir, "dir", "",
		"Install dir override (must match the dir used at install time)")
	witnessUninstallStreakShimCmd.Flags().BoolVar(&streakShimDryRun, "dry-run", false,
		"Show what would happen without removing the symlink")

	witnessCmd.AddCommand(witnessInstallStreakShimCmd)
	witnessCmd.AddCommand(witnessUninstallStreakShimCmd)
}

// streakShimPaths resolves the (source, target) pair for the shim, honoring
// the --dir override and the gas town workspace root for the source script.
func streakShimPaths() (source, target string, err error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return "", "", fmt.Errorf("not in a Gas Town workspace; cd into the town root or a crew directory")
	}
	source = filepath.Join(townRoot, streakShimSourceRel)

	dir := streakShimInstallDir
	if dir == "" {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", "", fmt.Errorf("resolving home dir: %w", herr)
		}
		dir = filepath.Join(home, streakShimDefaultDir)
	}
	target = filepath.Join(dir, streakShimName)
	return source, target, nil
}

func runWitnessInstallStreakShim(_ *cobra.Command, _ []string) error {
	source, target, err := streakShimPaths()
	if err != nil {
		return err
	}

	// Source must exist and be executable. Witness owns the script; we
	// don't try to create it here — that would conflate install with
	// authoring.
	srcInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("source script not found at %s: %w (witness-owned; create it before installing the shim)", source, err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("source path %s is a directory, not a script", source)
	}
	if srcInfo.Mode()&0o111 == 0 {
		return fmt.Errorf("source script %s is not executable; chmod +x the script first", source)
	}

	// Idempotency: if the symlink already points at the right source,
	// no-op. If it points elsewhere or is a regular file, fail-soft so
	// the user can pick the resolution path explicitly (uninstall, then
	// retry, or pick a different --dir).
	switch existing, err := os.Readlink(target); {
	case err == nil && existing == source:
		fmt.Printf("%s shim already installed at %s -> %s (no-op)\n", style.Bold.Render("✓"), target, source)
		return nil
	case err == nil && existing != source:
		return fmt.Errorf("symlink %s already exists pointing at %s (not %s); run `gt witness uninstall-streak-shim --dir %s` first or pick a different --dir", target, existing, source, filepath.Dir(target))
	case err != nil && !os.IsNotExist(err):
		// Path exists but is not a symlink — refuse to clobber.
		if info, statErr := os.Lstat(target); statErr == nil && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("path %s already exists and is not a symlink; refusing to overwrite (move it aside before installing)", target)
		}
	}

	if streakShimDryRun {
		fmt.Printf("dry-run: would create symlink %s -> %s\n", target, source)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating install dir %s: %w", filepath.Dir(target), err)
	}
	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("creating symlink %s -> %s: %w", target, source, err)
	}
	fmt.Printf("%s installed bd-with-streak shim: %s -> %s\n", style.SuccessPrefix, target, source)
	fmt.Printf("  PATH check: %s\n", pathContainsHint(filepath.Dir(target)))
	fmt.Printf("  usage:      bd-with-streak push   # opt-in M4 metric path\n")
	return nil
}

func runWitnessUninstallStreakShim(_ *cobra.Command, _ []string) error {
	source, target, err := streakShimPaths()
	if err != nil {
		return err
	}

	existing, err := os.Readlink(target)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("%s no shim installed at %s (nothing to do)\n", style.Bold.Render("✓"), target)
			return nil
		}
		return fmt.Errorf("reading symlink %s: %w", target, err)
	}
	if existing != source {
		return fmt.Errorf("symlink %s points at %s, not the expected source %s; refusing to remove (it was not installed by this command)", target, existing, source)
	}

	if streakShimDryRun {
		fmt.Printf("dry-run: would remove symlink %s -> %s\n", target, source)
		return nil
	}

	if err := os.Remove(target); err != nil {
		return fmt.Errorf("removing symlink %s: %w", target, err)
	}
	fmt.Printf("%s removed bd-with-streak shim: %s\n", style.SuccessPrefix, target)
	return nil
}

// pathContainsHint returns a one-line user hint about whether PATH contains
// dir, so a freshly-installed shim that won't resolve is flagged early
// rather than discovered later via "command not found".
func pathContainsHint(dir string) string {
	pathEnv := os.Getenv("PATH")
	clean := filepath.Clean(dir)
	for _, entry := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if filepath.Clean(entry) == clean {
			return fmt.Sprintf("%s is on PATH (shim resolves)", clean)
		}
	}
	return fmt.Sprintf("warn: %s is NOT on PATH; add it (e.g., add `export PATH=\"%s:$PATH\"` to your shell rc) or pass --dir to a PATH-resolved location", clean, clean)
}
