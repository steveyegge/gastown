package crew

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	colorPrimary   = lipgloss.Color("39")  // blue
	colorSuccess   = lipgloss.Color("76")  // green
	colorWarning   = lipgloss.Color("214") // orange
	colorError     = lipgloss.Color("196") // red
	colorMuted     = lipgloss.Color("242") // gray
	colorWhite     = lipgloss.Color("15")
	colorHighlight = lipgloss.Color("236") // dark gray for backgrounds
)

// Styles for the crew add wizard
var (
	// Title and headers
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginBottom(1)

	// Step indicator
	stepActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	stepInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	stepCompleteStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)

	// Input styles
	inputLabelStyle = lipgloss.NewStyle().
			Foreground(colorWhite).
			Bold(true).
			MarginBottom(1)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	inputErrorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			MarginTop(1)

	inputHintStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true).
			MarginTop(1)

	// Radio button / checkbox styles
	radioSelectedStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)

	radioUnselectedStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	checkboxCheckedStyle = lipgloss.NewStyle().
				Foreground(colorSuccess).
				Bold(true)

	checkboxUncheckedStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	// Progress styles
	progressStepStyle = lipgloss.NewStyle().
				Foreground(colorWhite)

	progressDoneStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)

	progressPendingStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	progressSpinnerStyle = lipgloss.NewStyle().
				Foreground(colorPrimary)

	// Button styles
	buttonActiveStyle = lipgloss.NewStyle().
				Background(colorPrimary).
				Foreground(colorWhite).
				Bold(true).
				Padding(0, 2)

	buttonInactiveStyle = lipgloss.NewStyle().
				Background(colorHighlight).
				Foreground(colorMuted).
				Padding(0, 2)

	// Status and messages
	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	// Help
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	// Borders
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(1, 2)
)
