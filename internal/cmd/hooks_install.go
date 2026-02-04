package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	installRole    string
	installAllRigs bool
	installDryRun  bool
)

var hooksInstallCmd = &cobra.Command{
	Use:   "install <hook-name>",
	Short: "Install a hook from the registry",
	Long: `Install a hook from the registry to worktrees.

By default, installs to the current worktree. Use --role to install
to all worktrees of a specific role in the current rig.

Examples:
  gt hooks install pr-workflow-guard              # Install to current worktree
  gt hooks install pr-workflow-guard --role crew  # Install to all crew in current rig
  gt hooks install session-prime --role crew --all-rigs  # Install to all crew everywhere
  gt hooks install pr-workflow-guard --dry-run    # Preview what would be installed`,
	Args: cobra.ExactArgs(1),
	RunE: runHooksInstall,
}

func init() {
	hooksCmd.AddCommand(hooksInstallCmd)
	hooksInstallCmd.Flags().StringVar(&installRole, "role", "", "Install to all worktrees of this role (crew, polecat, witness, refinery)")
	hooksInstallCmd.Flags().BoolVar(&installAllRigs, "all-rigs", false, "Install across all rigs (requires --role)")
	hooksInstallCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "Preview changes without writing files")
}

func runHooksInstall(cmd *cobra.Command, args []string) error {
	hookName := args[0]

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load registry
	registry, err := LoadRegistry(townRoot)
	if err != nil {
		return err
	}

	// Find the hook
	hookDef, ok := registry.Hooks[hookName]
	if !ok {
		return fmt.Errorf("hook %q not found in registry", hookName)
	}

	if !hookDef.Enabled {
		fmt.Printf("%s Hook %q is disabled in registry. Use --force to install anyway.\n",
			style.Warning.Render("Warning:"), hookName)
	}

	// Determine target worktrees
	targets, err := determineTargets(townRoot, installRole, installAllRigs, hookDef.Roles)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		// No role specified, install to current worktree
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		targets = []string{cwd}
	}

	// Install to each target
	installed := 0
	for _, target := range targets {
		if err := installHookTo(target, hookDef, installDryRun); err != nil {
			fmt.Printf("%s Failed to install to %s: %v\n", style.Error.Render("Error:"), target, err)
			continue
		}
		installed++
	}

	if installDryRun {
		fmt.Printf("\n%s Would install %q to %d worktree(s)\n", style.Dim.Render("Dry run:"), hookName, installed)
	} else {
		fmt.Printf("\n%s Installed %q to %d worktree(s)\n", style.Success.Render("Done:"), hookName, installed)
	}

	return nil
}

// determineTargets finds all worktree paths matching the role criteria.
func determineTargets(townRoot, role string, allRigs bool, allowedRoles []string) ([]string, error) {
	if role == "" {
		return nil, nil // Will use current directory
	}

	// Check if role is allowed for this hook
	roleAllowed := false
	for _, r := range allowedRoles {
		if r == role {
			roleAllowed = true
			break
		}
	}
	if !roleAllowed {
		return nil, fmt.Errorf("hook is not applicable to role %q (allowed: %s)", role, strings.Join(allowedRoles, ", "))
	}

	var targets []string

	// Find rigs to scan
	var rigs []string
	if allRigs {
		entries, err := os.ReadDir(townRoot)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "mayor" && e.Name() != "deacon" && e.Name() != "hooks" {
				rigs = append(rigs, e.Name())
			}
		}
	} else {
		// Find current rig from cwd
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		relPath, err := filepath.Rel(townRoot, cwd)
		if err != nil {
			return nil, err
		}
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) > 0 {
			rigs = []string{parts[0]}
		}
	}

	// Find worktrees for the role in each rig
	for _, rig := range rigs {
		rigPath := filepath.Join(townRoot, rig)

		switch role {
		case "crew":
			crewDir := filepath.Join(rigPath, "crew")
			if entries, err := os.ReadDir(crewDir); err == nil {
				for _, e := range entries {
					if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
						targets = append(targets, filepath.Join(crewDir, e.Name()))
					}
				}
			}
		case "polecat":
			polecatsDir := filepath.Join(rigPath, "polecats")
			if entries, err := os.ReadDir(polecatsDir); err == nil {
				for _, e := range entries {
					if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
						targets = append(targets, filepath.Join(polecatsDir, e.Name()))
					}
				}
			}
		case "witness":
			witnessPath := filepath.Join(rigPath, "witness")
			if _, err := os.Stat(witnessPath); err == nil {
				targets = append(targets, witnessPath)
			}
		case "refinery":
			refineryPath := filepath.Join(rigPath, "refinery")
			if _, err := os.Stat(refineryPath); err == nil {
				targets = append(targets, refineryPath)
			}
		}
	}

	return targets, nil
}

// installHookTo installs a hook to a specific worktree.
func installHookTo(worktreePath string, hookDef HookDefinition, dryRun bool) error {
	settingsPath := filepath.Join(worktreePath, ".claude", "settings.json")

	// Load existing settings or create new
	var settings ClaudeSettings
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing existing settings: %w", err)
		}
	}

	// Initialize maps if needed
	if settings.Hooks == nil {
		settings.Hooks = make(map[string][]ClaudeHookMatcher)
	}
	if settings.EnabledPlugins == nil {
		settings.EnabledPlugins = make(map[string]bool)
	}

	// Build the hook entries
	for _, matcher := range hookDef.Matchers {
		hookEntry := ClaudeHookMatcher{
			Matcher: matcher,
			Hooks: []ClaudeHook{
				{Type: "command", Command: hookDef.Command},
			},
		}

		// Check if this exact matcher already exists
		exists := false
		for _, existing := range settings.Hooks[hookDef.Event] {
			if existing.Matcher == matcher {
				exists = true
				break
			}
		}

		if !exists {
			settings.Hooks[hookDef.Event] = append(settings.Hooks[hookDef.Event], hookEntry)
		}
	}

	// Ensure beads plugin is disabled (standard for Gas Town)
	settings.EnabledPlugins["beads@beads-marketplace"] = false

	// Pretty print relative path
	relPath := worktreePath
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, worktreePath); err == nil && !strings.HasPrefix(rel, "..") {
			relPath = "~/" + rel
		}
	}

	if dryRun {
		fmt.Printf("  %s %s\n", style.Dim.Render("Would install to:"), relPath)
		return nil
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	// Write settings
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0600); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}

	fmt.Printf("  %s %s\n", style.Success.Render("Installed to:"), relPath)
	return nil
}
