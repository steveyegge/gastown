package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	gitInitGitHub  string
	gitInitPublic  bool
)

var gitInitCmd = &cobra.Command{
	Use:     "git-init",
	GroupID: GroupWorkspace,
	Short:   "Initialize git repository for a Gas Town HQ",
	Long: `Initialize or configure git for an existing Gas Town HQ.

This command:
  1. Creates a comprehensive .gitignore for Gas Town
  2. Initializes a git repository if not already present
  3. Optionally creates a GitHub repository (private by default)

The .gitignore excludes:
  - Polecat worktrees and rig clones (recreated with 'gt sling' or 'gt rig add')
  - Runtime state files (state.json, *.lock)
  - OS and editor files

And tracks:
  - CLAUDE.md and role contexts
  - .beads/ configuration and issues
  - Rig configs and hop/ directory

Examples:
  gt git-init                             # Init git with .gitignore
  gt git-init --github=user/repo          # Create private GitHub repo (default)
  gt git-init --github=user/repo --public # Create public GitHub repo`,
	RunE: runGitInit,
}

func init() {
	gitInitCmd.Flags().StringVar(&gitInitGitHub, "github", "", "Create GitHub repo (format: owner/repo, private by default)")
	gitInitCmd.Flags().BoolVar(&gitInitPublic, "public", false, "Make GitHub repo public (repos are private by default)")
	rootCmd.AddCommand(gitInitCmd)
}

// HQGitignore is the standard .gitignore for Gas Town HQs
const HQGitignore = `# Gas Town HQ .gitignore
# Track: Role context, handoff docs, beads config/data, rig configs
# Ignore: Git worktrees (polecats) and clones (mayor/refinery rigs), runtime state

# =============================================================================
# Runtime state files (transient)
# =============================================================================
**/state.json
**/*.lock
**/registry.json

# =============================================================================
# Rig git worktrees (recreate with 'gt sling' or 'gt rig add')
# =============================================================================

# Polecats - worker worktrees
**/polecats/

# Mayor rig clones
**/mayor/rig/

# Refinery working clones
**/refinery/rig/

# Crew workspaces (user-managed)
**/crew/

# =============================================================================
# Runtime state directories (gitignored ephemeral data)
# =============================================================================
**/.runtime/

# =============================================================================
# Rig .beads symlinks (point to ignored mayor/rig/.beads, recreated on setup)
# =============================================================================
# Add rig-specific symlinks here, e.g.:
# gastown/.beads

# =============================================================================
# OS and editor files
# =============================================================================
.DS_Store
*~
*.swp
*.swo
.vscode/
.idea/

# =============================================================================
# Explicitly track (override above patterns)
# =============================================================================
# Note: .beads/ has its own .gitignore that handles SQLite files
# and keeps issues.jsonl, metadata.json, config file as source of truth
`

func runGitInit(cmd *cobra.Command, args []string) error {
	// Find the HQ root
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	hqRoot, err := workspace.Find(cwd)
	if err != nil || hqRoot == "" {
		return fmt.Errorf("not inside a Gas Town HQ (run 'gt install' first)")
	}

	fmt.Printf("%s Initializing git for HQ at %s\n\n",
		style.Bold.Render("🔧"), style.Dim.Render(hqRoot))

	// Create .gitignore
	gitignorePath := filepath.Join(hqRoot, ".gitignore")
	if err := createGitignore(gitignorePath); err != nil {
		return err
	}

	// Initialize git if needed
	gitDir := filepath.Join(hqRoot, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := initGitRepo(hqRoot); err != nil {
			return err
		}
	} else {
		fmt.Printf("   ✓ Git repository already exists\n")
	}

	// Create GitHub repo if requested
	if gitInitGitHub != "" {
		if err := createGitHubRepo(hqRoot, gitInitGitHub, !gitInitPublic); err != nil {
			return err
		}
	}

	fmt.Printf("\n%s Git initialization complete!\n", style.Bold.Render("✓"))

	// Show next steps if no GitHub was created
	if gitInitGitHub == "" {
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  1. Create initial commit: %s\n",
			style.Dim.Render("git add . && git commit -m 'Initial Gas Town HQ'"))
		fmt.Printf("  2. Create remote repo: %s\n",
			style.Dim.Render("gt git-init --github=user/repo"))
	}

	return nil
}

func createGitignore(path string) error {
	// Check if .gitignore already exists
	if _, err := os.Stat(path); err == nil {
		// Read existing content
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading existing .gitignore: %w", err)
		}

		// Check if it already has Gas Town section
		if strings.Contains(string(content), "Gas Town HQ") {
			fmt.Printf("   ✓ .gitignore already configured for Gas Town\n")
			return nil
		}

		// Append to existing
		combined := string(content) + "\n" + HQGitignore
		if err := os.WriteFile(path, []byte(combined), 0644); err != nil {
			return fmt.Errorf("updating .gitignore: %w", err)
		}
		fmt.Printf("   ✓ Updated .gitignore with Gas Town patterns\n")
		return nil
	}

	// Create new .gitignore
	if err := os.WriteFile(path, []byte(HQGitignore), 0644); err != nil {
		return fmt.Errorf("creating .gitignore: %w", err)
	}
	fmt.Printf("   ✓ Created .gitignore\n")
	return nil
}

func initGitRepo(path string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git init failed: %w", err)
	}
	fmt.Printf("   ✓ Initialized git repository\n")
	return nil
}

func createGitHubRepo(hqRoot, repo string, private bool) error {
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) not found. Install it with: brew install gh")
	}

	// Parse owner/repo format
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid GitHub repo format (expected owner/repo): %s", repo)
	}

	visibility := "private"
	if !private {
		visibility = "public"
	}
	fmt.Printf("   → Creating %s GitHub repository %s...\n", visibility, repo)

	// Build gh repo create command
	args := []string{"repo", "create", repo, "--source", hqRoot}
	if private {
		args = append(args, "--private")
	} else {
		args = append(args, "--public")
	}
	args = append(args, "--push")

	cmd := exec.Command("gh", args...)
	cmd.Dir = hqRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh repo create failed: %w", err)
	}
	fmt.Printf("   ✓ Created and pushed to GitHub: %s (%s)\n", repo, visibility)
	if private {
		fmt.Printf("   ℹ To make this repo public: %s\n", style.Dim.Render("gh repo edit "+repo+" --visibility public"))
	}
	return nil
}

// InitGitForHarness is the shared implementation for git initialization.
// It can be called from both 'gt git-init' and 'gt install --git'.
// Note: Function name kept for backwards compatibility.
func InitGitForHarness(hqRoot string, github string, private bool) error {
	// Create .gitignore
	gitignorePath := filepath.Join(hqRoot, ".gitignore")
	if err := createGitignore(gitignorePath); err != nil {
		return err
	}

	// Initialize git if needed
	gitDir := filepath.Join(hqRoot, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		if err := initGitRepo(hqRoot); err != nil {
			return err
		}
	} else {
		fmt.Printf("   ✓ Git repository already exists\n")
	}

	// Create GitHub repo if requested
	if github != "" {
		if err := createGitHubRepo(hqRoot, github, private); err != nil {
			return err
		}
	}

	return nil
}
