// ABOUTME: Command to clean gastown artifacts from a git repo.
// ABOUTME: Removes role beads, rig identity beads, routes, and gastown config.

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	cleanDryRun bool
	cleanForce  bool
	cleanAll    bool
)

// gastown role bead suffixes to clean from .beads/
var gastownRoleSuffixes = []string{
	"-mayor-role",
	"-deacon-role",
	"-dog-role",
	"-witness-role",
	"-refinery-role",
	"-polecat-role",
	"-crew-role",
}

// gastown agent bead patterns to clean
var gastownAgentPatterns = []string{
	"-mayor",
	"-deacon",
	"-dog-",
	"-witness",
	"-refinery",
	"-polecat-",
	"-rig-",
}

var cleanCmd = &cobra.Command{
	Use:     "clean [path]",
	GroupID: GroupWorkspace,
	Short:   "Remove gastown artifacts from a git repo",
	Long: `Remove gastown-specific artifacts from a git repo's .beads/ directory.

This command cleans up gastown role beads and agent beads while preserving
regular beads issues. Use this when you want to remove gastown from a repo
that other developers use with plain beads.

By default, removes:
  - Role beads (*-mayor-role, *-witness-role, *-refinery-role, etc.)
  - Agent beads (*-mayor, *-witness, *-refinery, *-polecat-*, etc.)
  - Rig identity beads (*-rig-*)

With --all, also removes:
  - routes.jsonl entries for this repo
  - Gastown directories (mayor/, refinery/, witness/, polecats/, crew/)
  - config.json and .repo.git/

Does NOT remove:
  - Regular beads issues (bugs, features, tasks)
  - .beads/ directory itself
  - Git history

Examples:
  gt clean                    # Clean current directory
  gt clean /path/to/repo      # Clean specific repo
  gt clean --dry-run          # Show what would be removed
  gt clean --all              # Also remove gastown directories
  gt clean --force            # Skip confirmation`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

func init() {
	cleanCmd.Flags().BoolVar(&cleanDryRun, "dry-run", false, "Show what would be removed without making changes")
	cleanCmd.Flags().BoolVarP(&cleanForce, "force", "f", false, "Skip confirmation prompt")
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "Also remove gastown directories (mayor/, refinery/, etc.)")
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	// Determine target path
	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Check .beads/ exists
	beadsDir := filepath.Join(absPath, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return fmt.Errorf("no .beads/ directory found at %s", absPath)
	}

	// Find artifacts to clean
	artifacts, err := findGastownArtifacts(absPath)
	if err != nil {
		return fmt.Errorf("scanning for artifacts: %w", err)
	}

	if len(artifacts.RoleBeads) == 0 && len(artifacts.AgentBeads) == 0 &&
		len(artifacts.Directories) == 0 && len(artifacts.Files) == 0 {
		fmt.Println("No gastown artifacts found.")
		return nil
	}

	// Display what will be removed
	fmt.Printf("Gastown artifacts found in %s:\n\n", absPath)

	if len(artifacts.RoleBeads) > 0 {
		fmt.Printf("%s (%d)\n", style.Bold.Render("Role beads"), len(artifacts.RoleBeads))
		for _, id := range artifacts.RoleBeads {
			fmt.Printf("  • %s\n", id)
		}
		fmt.Println()
	}

	if len(artifacts.AgentBeads) > 0 {
		fmt.Printf("%s (%d)\n", style.Bold.Render("Agent beads"), len(artifacts.AgentBeads))
		for _, id := range artifacts.AgentBeads {
			fmt.Printf("  • %s\n", id)
		}
		fmt.Println()
	}

	if cleanAll {
		if len(artifacts.Directories) > 0 {
			fmt.Printf("%s (%d)\n", style.Bold.Render("Directories"), len(artifacts.Directories))
			for _, dir := range artifacts.Directories {
				fmt.Printf("  • %s/\n", dir)
			}
			fmt.Println()
		}

		if len(artifacts.Files) > 0 {
			fmt.Printf("%s (%d)\n", style.Bold.Render("Files"), len(artifacts.Files))
			for _, f := range artifacts.Files {
				fmt.Printf("  • %s\n", f)
			}
			fmt.Println()
		}
	}

	if cleanDryRun {
		fmt.Printf("%s No changes made.\n", style.Dim.Render("(dry-run)"))
		return nil
	}

	// Confirm unless --force
	if !cleanForce {
		fmt.Print("Remove these artifacts? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Perform cleanup
	fmt.Println()
	fmt.Println("Cleaning...")

	var errors []string

	// Remove beads
	bd := beads.New(absPath)

	// Remove role beads (close with reason)
	for _, id := range artifacts.RoleBeads {
		if err := bd.CloseWithReason("Cleaned by gt clean", id); err != nil {
			errors = append(errors, fmt.Sprintf("role bead %s: %v", id, err))
			continue
		}
		fmt.Printf("  %s Removed role bead: %s\n", style.Success.Render("✓"), id)
	}

	// Remove agent beads (use DeleteAgentBead)
	for _, id := range artifacts.AgentBeads {
		if err := bd.DeleteAgentBead(id); err != nil {
			// Fall back to close if delete fails
			if err := bd.CloseWithReason("Cleaned by gt clean", id); err != nil {
				errors = append(errors, fmt.Sprintf("agent bead %s: %v", id, err))
				continue
			}
		}
		fmt.Printf("  %s Removed agent bead: %s\n", style.Success.Render("✓"), id)
	}

	// Remove directories (if --all)
	if cleanAll {
		for _, dir := range artifacts.Directories {
			dirPath := filepath.Join(absPath, dir)
			if err := os.RemoveAll(dirPath); err != nil {
				errors = append(errors, fmt.Sprintf("directory %s: %v", dir, err))
				continue
			}
			fmt.Printf("  %s Removed directory: %s/\n", style.Success.Render("✓"), dir)
		}

		for _, f := range artifacts.Files {
			filePath := filepath.Join(absPath, f)
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("file %s: %v", f, err))
				continue
			}
			fmt.Printf("  %s Removed file: %s\n", style.Success.Render("✓"), f)
		}
	}

	fmt.Println()

	if len(errors) > 0 {
		fmt.Printf("%s Some artifacts could not be removed:\n", style.Warning.Render("⚠"))
		for _, e := range errors {
			fmt.Printf("  • %s\n", e)
		}
		return fmt.Errorf("cleanup incomplete")
	}

	fmt.Printf("%s Gastown artifacts cleaned from %s\n", style.Success.Render("✓"), absPath)

	if !cleanAll && (len(artifacts.Directories) > 0 || len(artifacts.Files) > 0) {
		fmt.Printf("\n%s Use --all to also remove gastown directories.\n", style.Dim.Render("Tip:"))
	}

	return nil
}

// gastownArtifacts holds discovered gastown artifacts in a repo
type gastownArtifacts struct {
	RoleBeads   []string // Role definition beads (*-role)
	AgentBeads  []string // Agent instance beads
	Directories []string // Gastown directories (mayor/, refinery/, etc.)
	Files       []string // Gastown files (config.json, etc.)
}

// findGastownArtifacts scans a repo for gastown artifacts
func findGastownArtifacts(repoPath string) (*gastownArtifacts, error) {
	artifacts := &gastownArtifacts{}

	// Scan .beads/ for role and agent beads
	bd := beads.New(repoPath)
	issues, err := bd.List(beads.ListOptions{
		Status:   "all",
		Priority: -1,
	})
	if err == nil {
		for _, issue := range issues {
			// Check for role beads
			for _, suffix := range gastownRoleSuffixes {
				if strings.HasSuffix(issue.ID, suffix) {
					artifacts.RoleBeads = append(artifacts.RoleBeads, issue.ID)
					break
				}
			}

			// Check for agent beads
			for _, pattern := range gastownAgentPatterns {
				if strings.Contains(issue.ID, pattern) {
					// Avoid double-counting role beads
					isRole := false
					for _, suffix := range gastownRoleSuffixes {
						if strings.HasSuffix(issue.ID, suffix) {
							isRole = true
							break
						}
					}
					if !isRole {
						artifacts.AgentBeads = append(artifacts.AgentBeads, issue.ID)
					}
					break
				}
			}
		}
	}

	// Check for gastown directories
	gastownDirs := []string{
		"mayor",
		"refinery",
		"witness",
		"polecats",
		"crew",
		"plugins",
		".repo.git",
	}
	for _, dir := range gastownDirs {
		dirPath := filepath.Join(repoPath, dir)
		if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
			artifacts.Directories = append(artifacts.Directories, dir)
		}
	}

	// Check for gastown files
	gastownFiles := []string{
		"config.json",
		"routes.jsonl",
	}
	for _, f := range gastownFiles {
		filePath := filepath.Join(repoPath, f)
		if _, err := os.Stat(filePath); err == nil {
			artifacts.Files = append(artifacts.Files, f)
		}
	}

	return artifacts, nil
}
