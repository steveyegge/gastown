package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:     "init",
	GroupID: GroupWorkspace,
	Short:   "Initialize current directory as a Gas Town rig",
	Long: `Initialize the current directory for use as a Gas Town rig.

This creates the standard agent directories (polecats/, witness/, refinery/,
mayor/) and updates .git/info/exclude to ignore them.

The current directory must be a git repository. Use --force to reinitialize
an existing rig structure.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Reinitialize existing structure")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Check if it's a git repository
	g := git.NewGit(cwd)
	if _, err := g.CurrentBranch(); err != nil {
		return fmt.Errorf("not a git repository (run 'git init' first)")
	}

	// Check if already initialized
	polecatsDir := filepath.Join(cwd, "polecats")
	if _, err := os.Stat(polecatsDir); err == nil && !initForce {
		return fmt.Errorf("rig already initialized (use --force to reinitialize)")
	}

	fmt.Printf("%s Initializing Gas Town rig in %s\n\n",
		style.Bold.Render("⚙️"), style.Dim.Render(cwd))

	// Create agent directories
	created := 0
	for _, dir := range rig.AgentDirs {
		dirPath := filepath.Join(cwd, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}

		// Create .gitkeep to ensure directory is tracked if needed (non-fatal)
		gitkeep := filepath.Join(dirPath, ".gitkeep")
		if _, err := os.Stat(gitkeep); os.IsNotExist(err) {
			_ = os.WriteFile(gitkeep, []byte(""), 0644)
		}

		fmt.Printf("   ✓ Created %s/\n", dir)
		created++
	}

	// Update .git/info/exclude
	if err := updateGitExclude(cwd); err != nil {
		fmt.Printf("   %s Could not update .git/info/exclude: %v\n",
			style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("   ✓ Updated .git/info/exclude\n")
	}

	// Register custom beads types for Gas Town (agent, role, rig, convoy, slot).
	// This is best-effort: if beads isn't installed or DB doesn't exist, we skip.
	// The doctor check will catch missing types later.
	if err := registerCustomTypes(cwd); err != nil {
		fmt.Printf("   %s Could not register custom types: %v\n",
			style.Dim.Render("⚠"), err)
	} else {
		fmt.Printf("   ✓ Registered custom beads types\n")
	}

	fmt.Printf("\n%s Rig initialized with %d directories.\n",
		style.Bold.Render("✓"), created)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Add this rig to a town: %s\n",
		style.Dim.Render("gt rig add <name> <git-url>"))
	fmt.Printf("  2. Create a polecat: %s\n",
		style.Dim.Render("gt polecat add <name>"))

	return nil
}

func updateGitExclude(repoPath string) error {
	excludePath := filepath.Join(repoPath, ".git", "info", "exclude")

	// Ensure directory exists
	excludeDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(excludeDir, 0755); err != nil {
		return fmt.Errorf("creating .git/info: %w", err)
	}

	// Read existing content
	content, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if already has Gas Town section
	if strings.Contains(string(content), "Gas Town") {
		return nil // Already configured
	}

	// Append agent dirs
	additions := "\n# Gas Town agent directories\n"
	for _, dir := range rig.AgentDirs {
		// Get first component (e.g., "polecats" from "polecats")
		// or "refinery" from "refinery/rig"
		base := filepath.Dir(dir)
		if base == "." {
			base = dir
		}
		additions += base + "/\n"
	}

	// Write back
	return os.WriteFile(excludePath, append(content, []byte(additions)...), 0644)
}

// registerCustomTypes registers Gas Town custom issue types with beads.
// This is best-effort: returns nil if beads isn't available or DB doesn't exist.
// Handles gracefully: beads not installed, no .beads directory, or config errors.
func registerCustomTypes(workDir string) error {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		return nil // beads not installed, skip silently
	}

	// Check if .beads directory exists
	beadsDir := filepath.Join(workDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return nil // no beads DB yet, skip silently
	}

	// Try to set custom types
	cmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check for common expected errors
		outStr := string(output)
		if strings.Contains(outStr, "not initialized") ||
			strings.Contains(outStr, "no such file") {
			return nil // DB not initialized, skip silently
		}
		return fmt.Errorf("%s", strings.TrimSpace(outStr))
	}
	return nil
}
