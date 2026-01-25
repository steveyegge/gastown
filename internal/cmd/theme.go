package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	themeListFlag     bool
	themeApplyFlag    bool
	themeApplyAllFlag bool
)

// Valid CLI theme modes
var validCLIThemes = []string{"auto", "dark", "light"}

var themeCmd = &cobra.Command{
	Use:     "theme [name]",
	GroupID: GroupConfig,
	Short:   "View or set tmux theme for the current rig",
	Long: `Manage tmux status bar themes for Gas Town sessions.

Without arguments, shows the current theme assignment.
With a name argument, sets the theme for this rig.

Examples:
  gt theme              # Show current theme
  gt theme --list       # List available themes
  gt theme forest       # Set theme to 'forest'
  gt theme apply        # Apply theme to all running sessions in this rig`,
	RunE: runTheme,
}

var themeApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply theme to running sessions",
	Long: `Apply theme to running Gas Town sessions.

By default, only applies to sessions in the current rig.
Use --all to apply to sessions across all rigs.`,
	RunE: runThemeApply,
}

var themeCLICmd = &cobra.Command{
	Use:   "cli [mode]",
	Short: "View or set CLI color scheme (dark/light/auto)",
	Long: `Manage CLI output color scheme for Gas Town commands.

Without arguments, shows the current CLI theme mode and detection.
With a mode argument, sets the CLI theme preference.

Modes:
  auto   - Automatically detect terminal background (default)
  dark   - Force dark mode colors (light text for dark backgrounds)
  light  - Force light mode colors (dark text for light backgrounds)

The setting is stored in town settings (settings/config.json) and can
be overridden per-session via the GT_THEME environment variable.

Examples:
  gt theme cli              # Show current CLI theme
  gt theme cli dark         # Set CLI theme to dark mode
  gt theme cli auto         # Reset to auto-detection
  GT_THEME=light gt status  # Override for a single command`,
	RunE: runThemeCLI,
}

func init() {
	rootCmd.AddCommand(themeCmd)
	themeCmd.AddCommand(themeApplyCmd)
	themeCmd.AddCommand(themeCLICmd)
	themeCmd.Flags().BoolVarP(&themeListFlag, "list", "l", false, "List available themes")
	themeApplyCmd.Flags().BoolVarP(&themeApplyAllFlag, "all", "a", false, "Apply to all rigs, not just current")
}

func runTheme(cmd *cobra.Command, args []string) error {
	// List mode
	if themeListFlag {
		fmt.Println("Available themes:")
		for _, name := range tmux.ListThemeNames() {
			theme := tmux.GetThemeByName(name)
			fmt.Printf("  %-10s  %s\n", name, theme.Style())
		}
		// Also show Mayor theme
		mayor := tmux.MayorTheme()
		fmt.Printf("  %-10s  %s (Mayor only)\n", mayor.Name, mayor.Style())
		return nil
	}

	// Determine current rig
	rigName := detectCurrentRig()
	if rigName == "" {
		rigName = "unknown"
	}

	// Show current theme assignment
	if len(args) == 0 {
		theme := getThemeForRig(rigName)
		fmt.Printf("Rig: %s\n", rigName)
		fmt.Printf("Theme: %s (%s)\n", theme.Name, theme.Style())
		// Show if it's configured vs default
		if configured := loadRigTheme(rigName); configured != "" {
			fmt.Printf("(configured in settings/config.json)\n")
		} else {
			fmt.Printf("(default, based on rig name hash)\n")
		}
		return nil
	}

	// Set theme
	themeName := args[0]
	theme := tmux.GetThemeByName(themeName)
	if theme == nil {
		return fmt.Errorf("unknown theme: %s (use --list to see available themes)", themeName)
	}

	// Save to rig config
	if err := saveRigTheme(rigName, themeName); err != nil {
		return fmt.Errorf("saving theme config: %w", err)
	}

	fmt.Printf("Theme '%s' saved for rig '%s'\n", themeName, rigName)
	fmt.Println("Run 'gt theme apply' to apply to running sessions")

	return nil
}

func runThemeApply(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	// Get all sessions
	sessions, err := t.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	// Determine current rig
	rigName := detectCurrentRig()

	// Get session names for comparison
	mayorSession := session.MayorSessionName()
	deaconSession := session.DeaconSessionName()

	// Apply to matching sessions
	applied := 0
	for _, sess := range sessions {
		if !strings.HasPrefix(sess, "gt-") {
			continue
		}

		// Determine theme and identity for this session
		var theme tmux.Theme
		var rig, worker, role string

		if sess == mayorSession {
			theme = tmux.MayorTheme()
			worker = "Mayor"
			role = "coordinator"
		} else if sess == deaconSession {
			theme = tmux.DeaconTheme()
			worker = "Deacon"
			role = "health-check"
		} else if strings.HasSuffix(sess, "-witness") && strings.HasPrefix(sess, "gt-") {
			// Witness sessions: gt-<rig>-witness
			rig = strings.TrimPrefix(strings.TrimSuffix(sess, "-witness"), "gt-")
			theme = getThemeForRole(rig, "witness")
			worker = "witness"
			role = "witness"
		} else {
			// Parse session name: gt-<rig>-<worker> or gt-<rig>-crew-<name>
			parts := strings.SplitN(sess, "-", 3)
			if len(parts) < 3 {
				continue
			}
			rig = parts[1]

			// Skip if not matching current rig (unless --all flag)
			if !themeApplyAllFlag && rigName != "" && rig != rigName {
				continue
			}

			workerPart := parts[2]
			if strings.HasPrefix(workerPart, "crew-") {
				worker = strings.TrimPrefix(workerPart, "crew-")
				role = "crew"
			} else if workerPart == "refinery" {
				worker = "refinery"
				role = "refinery"
			} else {
				worker = workerPart
				role = "polecat"
			}

			// Use role-based theme resolution
			theme = getThemeForRole(rig, role)
		}

		// Apply theme and status format
		if err := t.ApplyTheme(sess, theme); err != nil {
			fmt.Printf("  %s: failed (%v)\n", sess, err)
			continue
		}
		if err := t.SetStatusFormat(sess, rig, worker, role); err != nil {
			fmt.Printf("  %s: failed to set format (%v)\n", sess, err)
			continue
		}
		if err := t.SetDynamicStatus(sess); err != nil {
			fmt.Printf("  %s: failed to set dynamic status (%v)\n", sess, err)
			continue
		}

		fmt.Printf("  %s: applied %s theme\n", sess, theme.Name)
		applied++
	}

	if applied == 0 {
		fmt.Println("No matching sessions found")
	} else {
		fmt.Printf("\nApplied theme to %d session(s)\n", applied)
	}

	return nil
}

// detectCurrentRig determines the rig from environment or cwd.
func detectCurrentRig() string {
	// Try environment first (GT_RIG is set in tmux sessions)
	if rig := os.Getenv("GT_RIG"); rig != "" {
		return rig
	}

	// Try to extract from tmux session name
	if session := detectCurrentSession(); session != "" {
		// Extract rig from session name: gt-<rig>-...
		parts := strings.SplitN(session, "-", 3)
		if len(parts) >= 2 && parts[0] == "gt" && parts[1] != "mayor" && parts[1] != "deacon" {
			return parts[1]
		}
	}

	// Try to detect from actual cwd path
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Find town root to extract rig name
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return ""
	}

	// Get path relative to town root
	rel, err := filepath.Rel(townRoot, cwd)
	if err != nil {
		return ""
	}

	// Extract first path component (rig name)
	// Patterns: <rig>/..., mayor/..., deacon/...
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) > 0 && parts[0] != "." && parts[0] != "mayor" && parts[0] != "deacon" {
		return parts[0]
	}

	return ""
}

// getThemeForRig returns the theme for a rig, checking config first.
func getThemeForRig(rigName string) tmux.Theme {
	// Try to load configured theme
	if themeName := loadRigTheme(rigName); themeName != "" {
		if theme := tmux.GetThemeByName(themeName); theme != nil {
			return *theme
		}
	}
	// Fall back to hash-based assignment
	return tmux.AssignTheme(rigName)
}

// getThemeForRole returns the theme for a specific role in a rig.
// Resolution order:
// 1. Per-rig role override (rig/settings/config.json)
// 2. Global role default (mayor/config.json)
// 3. Built-in role defaults (witness=rust, refinery=plum)
// 4. Rig theme (config or hash-based)
func getThemeForRole(rigName, role string) tmux.Theme {
	townRoot, _ := workspace.FindFromCwd()

	// 1. Check per-rig role override
	if townRoot != "" {
		settingsPath := filepath.Join(townRoot, rigName, "settings", "config.json")
		if settings, err := config.LoadRigSettings(settingsPath); err == nil {
			if settings.Theme != nil && settings.Theme.RoleThemes != nil {
				if themeName, ok := settings.Theme.RoleThemes[role]; ok {
					if theme := tmux.GetThemeByName(themeName); theme != nil {
						return *theme
					}
				}
			}
		}
	}

	// 2. Check global role default (mayor config)
	if townRoot != "" {
		mayorConfigPath := filepath.Join(townRoot, "mayor", "config.json")
		if mayorCfg, err := config.LoadMayorConfig(mayorConfigPath); err == nil {
			if mayorCfg.Theme != nil && mayorCfg.Theme.RoleDefaults != nil {
				if themeName, ok := mayorCfg.Theme.RoleDefaults[role]; ok {
					if theme := tmux.GetThemeByName(themeName); theme != nil {
						return *theme
					}
				}
			}
		}
	}

	// 3. Check built-in role defaults
	builtins := config.BuiltinRoleThemes()
	if themeName, ok := builtins[role]; ok {
		if theme := tmux.GetThemeByName(themeName); theme != nil {
			return *theme
		}
	}

	// 4. Fall back to rig theme
	return getThemeForRig(rigName)
}

// loadRigTheme loads the theme name from rig settings.
func loadRigTheme(rigName string) string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return ""
	}

	settingsPath := filepath.Join(townRoot, rigName, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return ""
	}

	if settings.Theme != nil && settings.Theme.Name != "" {
		return settings.Theme.Name
	}
	return ""
}

// saveRigTheme saves the theme name to rig settings.
func saveRigTheme(rigName, themeName string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	settingsPath := filepath.Join(townRoot, rigName, "settings", "config.json")

	// Load existing settings or create new
	var settings *config.RigSettings
	settings, err = config.LoadRigSettings(settingsPath)
	if err != nil {
		// Create new settings if not found
		if os.IsNotExist(err) || strings.Contains(err.Error(), "not found") {
			settings = config.NewRigSettings()
		} else {
			return fmt.Errorf("loading settings: %w", err)
		}
	}

	// Set theme
	settings.Theme = &config.ThemeConfig{
		Name: themeName,
	}

	// Save
	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}

	return nil
}

func runThemeCLI(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	settingsPath := config.TownSettingsPath(townRoot)

	// Show current theme
	if len(args) == 0 {
		settings, err := config.LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			return fmt.Errorf("loading settings: %w", err)
		}

		// Determine effective mode
		configValue := settings.CLITheme
		if configValue == "" {
			configValue = "auto"
		}

		// Check for env override
		envValue := os.Getenv("GT_THEME")
		effectiveMode := configValue
		if envValue != "" {
			effectiveMode = strings.ToLower(envValue)
		}

		fmt.Printf("CLI Theme:\n")
		fmt.Printf("  Configured: %s\n", configValue)
		if envValue != "" {
			fmt.Printf("  Override:   %s (via GT_THEME)\n", envValue)
		}
		fmt.Printf("  Effective:  %s\n", effectiveMode)

		// Show detection result for auto mode
		if effectiveMode == "auto" {
			detected := "light"
			if detectTerminalBackground() {
				detected = "dark"
			}
			fmt.Printf("  Detected:   %s background\n", detected)
		}

		return nil
	}

	// Set CLI theme
	mode := strings.ToLower(args[0])
	if !isValidCLITheme(mode) {
		return fmt.Errorf("invalid CLI theme '%s' (valid: auto, dark, light)", mode)
	}

	// Load existing settings
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}

	// Update CLITheme
	settings.CLITheme = mode

	// Save
	if err := config.SaveTownSettings(settingsPath, settings); err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}

	fmt.Printf("CLI theme set to '%s'\n", mode)
	if mode == "auto" {
		fmt.Println("Colors will adapt to your terminal's background.")
	} else {
		fmt.Printf("Colors optimized for %s backgrounds.\n", mode)
	}

	return nil
}

// isValidCLITheme checks if a CLI theme mode is valid.
func isValidCLITheme(mode string) bool {
	for _, valid := range validCLIThemes {
		if mode == valid {
			return true
		}
	}
	return false
}

// detectTerminalBackground returns true if terminal has dark background.
func detectTerminalBackground() bool {
	// Use termenv for detection
	return termenv.HasDarkBackground()
}
