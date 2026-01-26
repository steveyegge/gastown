package decision

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	colorHighUrgency   = lipgloss.Color("196") // bright red
	colorMediumUrgency = lipgloss.Color("214") // orange
	colorLowUrgency    = lipgloss.Color("76")  // green
	colorSelected      = lipgloss.Color("39")  // blue
	colorMuted         = lipgloss.Color("242") // gray
	colorWhite         = lipgloss.Color("15")
	colorBlack         = lipgloss.Color("0")
)

// Styles for the decision TUI
var (
	// Title styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12")).
			MarginBottom(1)

	// List item styles
	selectedItemStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(colorWhite).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(colorWhite)

	// Urgency indicators
	highUrgencyStyle = lipgloss.NewStyle().
				Foreground(colorHighUrgency).
				Bold(true)

	mediumUrgencyStyle = lipgloss.NewStyle().
				Foreground(colorMediumUrgency)

	lowUrgencyStyle = lipgloss.NewStyle().
			Foreground(colorLowUrgency)

	// Detail pane styles
	detailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorSelected).
				MarginBottom(1)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(colorWhite)

	// Option styles
	optionNumberStyle = lipgloss.NewStyle().
				Foreground(colorSelected).
				Bold(true)

	optionLabelStyle = lipgloss.NewStyle().
				Foreground(colorWhite)

	optionDescStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	selectedOptionStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Foreground(colorWhite)

	// Input styles
	inputLabelStyle = lipgloss.NewStyle().
			Foreground(colorSelected).
			Bold(true)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorSelected).
			Padding(0, 1)

	// Help and status
	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	statusStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorHighUrgency)

	successStyle = lipgloss.NewStyle().
			Foreground(colorLowUrgency)

	// Border styles
	listBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted)

	detailBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorSelected)
)

// urgencyIcon returns the icon for a given urgency level
func urgencyIcon(urgency string) string {
	switch urgency {
	case "high":
		return highUrgencyStyle.Render("●")
	case "medium":
		return mediumUrgencyStyle.Render("●")
	case "low":
		return lowUrgencyStyle.Render("●")
	default:
		return mediumUrgencyStyle.Render("●")
	}
}

// urgencyLabel returns a styled urgency label
func urgencyLabel(urgency string) string {
	switch urgency {
	case "high":
		return highUrgencyStyle.Render("[HIGH]")
	case "medium":
		return mediumUrgencyStyle.Render("[MED]")
	case "low":
		return lowUrgencyStyle.Render("[LOW]")
	default:
		return mediumUrgencyStyle.Render("[MED]")
	}
}
