package ui

import (
	"os"

	"golang.org/x/term"
)

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
//   - CLAUDECODE environment variable (auto-detect Claude Code)
//
// Agent mode provides ultra-compact output optimized for LLM context windows.
func IsAgentMode() bool {
	if os.Getenv("GT_AGENT_MODE") == "1" {
		return true
	}
	// auto-detect Claude Code environment
	if os.Getenv("CLAUDECODE") != "" {
		return true
	}
	return false
}
