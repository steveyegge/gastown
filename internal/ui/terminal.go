package ui

import (
	"os"
	"strings"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// ThemeMode represents the CLI color scheme mode.
type ThemeMode string

const (
	// ThemeModeAuto lets the terminal background guide color selection.
	ThemeModeAuto ThemeMode = "auto"
	// ThemeModeDark forces dark mode colors (light text on dark background).
	ThemeModeDark ThemeMode = "dark"
	// ThemeModeLight forces light mode colors (dark text on light background).
	ThemeModeLight ThemeMode = "light"
)

// themeMode is the cached theme mode, set during init.
var themeMode ThemeMode

// hasDarkBackground caches whether we're in dark mode.
var hasDarkBackground bool

// InitTheme initializes the theme mode. Call this early in main.
// configTheme is the value from TownSettings.CLITheme (may be empty).
func InitTheme(configTheme string) {
	themeMode = resolveThemeMode(configTheme)
	hasDarkBackground = detectDarkBackground(themeMode)
}

// GetThemeMode returns the current CLI color scheme mode.
// Priority order:
//  1. GT_THEME environment variable ("dark", "light", "auto")
//  2. Configured value from settings (passed to InitTheme)
//  3. Default: "auto"
func GetThemeMode() ThemeMode {
	return themeMode
}

// HasDarkBackground returns true if we're displaying on a dark background.
// This is used by lipgloss AdaptiveColor to select appropriate colors.
func HasDarkBackground() bool {
	return hasDarkBackground
}

// resolveThemeMode determines the theme mode from env and config.
func resolveThemeMode(configTheme string) ThemeMode {
	// Priority 1: GT_THEME environment variable
	if envTheme := os.Getenv("GT_THEME"); envTheme != "" {
		switch strings.ToLower(envTheme) {
		case "dark":
			return ThemeModeDark
		case "light":
			return ThemeModeLight
		case "auto":
			return ThemeModeAuto
		}
		// Invalid value - fall through to config
	}

	// Priority 2: Config value
	if configTheme != "" {
		switch strings.ToLower(configTheme) {
		case "dark":
			return ThemeModeDark
		case "light":
			return ThemeModeLight
		case "auto":
			return ThemeModeAuto
		}
	}

	// Default: auto
	return ThemeModeAuto
}

// detectDarkBackground determines if we're on a dark background.
func detectDarkBackground(mode ThemeMode) bool {
	switch mode {
	case ThemeModeDark:
		return true
	case ThemeModeLight:
		return false
	default:
		// Auto mode - use termenv detection
		return termenv.HasDarkBackground()
	}
}

// IsTerminal returns true if stdout is connected to a terminal (TTY).
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// ShouldUseColor determines if ANSI color codes should be used.
// Respects NO_COLOR (https://no-color.org/), CLICOLOR, and CLICOLOR_FORCE conventions.
func ShouldUseColor() bool {
	// NO_COLOR takes precedence - any value disables color
	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		return false
	}

	// CLICOLOR=0 disables color
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}

	// CLICOLOR_FORCE enables color even in non-TTY
	if _, exists := os.LookupEnv("CLICOLOR_FORCE"); exists {
		return true
	}

	// default: use color only if stdout is a TTY
	return IsTerminal()
}

// ShouldUseEmoji determines if emoji decorations should be used.
// Disabled in non-TTY mode to keep output machine-readable.
func ShouldUseEmoji() bool {
	// GT_NO_EMOJI disables emoji output
	if _, exists := os.LookupEnv("GT_NO_EMOJI"); exists {
		return false
	}

	// default: use emoji only if stdout is a TTY
	return IsTerminal()
}

// IsAgentMode returns true if the CLI is running in agent-optimized mode.
// This is triggered by:
//   - GT_AGENT_MODE=1 environment variable (explicit)
//   - CLAUDE_CODE environment variable (auto-detect Claude Code)
//
// Agent mode provides ultra-compact output optimized for LLM context windows.
func IsAgentMode() bool {
	if os.Getenv("GT_AGENT_MODE") == "1" {
		return true
	}
	// auto-detect Claude Code environment
	if os.Getenv("CLAUDE_CODE") != "" {
		return true
	}
	return false
}
