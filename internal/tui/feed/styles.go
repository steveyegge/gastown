// Package feed provides a TUI for the Gas Town activity feed.
package feed

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/ui"
)

// Color palette using Ayu theme colors from ui package
var (
	colorPrimary   = ui.ColorAccent // Blue
	colorSuccess   = ui.ColorPass   // Green
	colorWarning   = ui.ColorWarn   // Yellow
	colorError     = ui.ColorFail   // Red
	colorDim       = ui.ColorMuted  // Gray
	colorHighlight = lipgloss.AdaptiveColor{Light: "#59c2ff", Dark: "#59c2ff"} // Cyan (Ayu)
	colorAccent    = lipgloss.AdaptiveColor{Light: "#d2a6ff", Dark: "#d2a6ff"} // Purple (Ayu)
)

// Styles for the feed TUI
var (
	// Header styles
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Padding(0, 1)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))

	FilterStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	// Agent tree styles
	TreePanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDim).
			Padding(0, 1)

	RigStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	RoleStyle = lipgloss.NewStyle().
			Foreground(colorAccent)

	AgentNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	AgentActiveStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)

	AgentIdleStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	// Event stream styles
	StreamPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorDim).
				Padding(0, 1)

	TimestampStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	EventCreateStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)

	EventUpdateStyle = lipgloss.NewStyle().
				Foreground(colorPrimary)

	EventCompleteStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	EventFailStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	EventDeleteStyle = lipgloss.NewStyle().
				Foreground(colorWarning)

	// Status bar styles
	StatusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(colorDim).
			Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(colorHighlight).
			Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	// Focus indicator
	FocusedBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	// Role icons - uses centralized emojis from constants package
	RoleIcons = map[string]string{
		constants.RoleMayor:    constants.EmojiMayor,
		constants.RoleWitness:  constants.EmojiWitness,
		constants.RoleRefinery: constants.EmojiRefinery,
		constants.RoleCrew:     constants.EmojiCrew,
		constants.RolePolecat:  constants.EmojiPolecat,
		constants.RoleDeacon:   constants.EmojiDeacon,
	}

	// MQ event styles
	EventMergeStartedStyle = lipgloss.NewStyle().
				Foreground(colorPrimary)

	EventMergedStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	EventMergeFailedStyle = lipgloss.NewStyle().
				Foreground(colorError).
				Bold(true)

	EventMergeSkippedStyle = lipgloss.NewStyle().
				Foreground(colorWarning)

	// Event symbols
	EventSymbols = map[string]string{
		"create":   "+",
		"update":   "→",
		"complete": "✓",
		"fail":     "✗",
		"delete":   "⊘",
		"pin":      "📌",
		// Witness patrol events
		"patrol_started":  constants.EmojiWitness,
		"patrol_complete": "✓",
		"polecat_checked": "·",
		"polecat_nudged":  "⚡",
		"escalation_sent": "⬆",
		// Merge events
		"merge_started": "⚙",
		"merged":        "✓",
		"merge_failed":  "✗",
		"merge_skipped": "⊘",
		// General gt events
		"sling":   "🎯",
		"hook":    "🪝",
		"unhook":  "↩",
		"handoff": "🤝",
		"done":    "✓",
		"mail":    "✉",
		"spawn":   "🚀",
		"kill":    "💀",
		"nudge":   "⚡",
		"boot":    "🔌",
		"halt":    "⏹",
	}
)
