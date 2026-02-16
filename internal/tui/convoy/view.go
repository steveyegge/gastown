package convoy

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// Styles for the convoy TUI
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15"))

	convoyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	issueOpenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")) // yellow

	issueClosedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")) // green

	progressStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // gray

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9")) // red
)

// renderView renders the entire view.
// Caller must hold m.mu.
func (m *Model) renderView() string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Convoys"))
	b.WriteString("\n\n")

	// Error message
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Empty state
	if len(m.convoys) == 0 && m.err == nil {
		b.WriteString("No convoys found.\n")
		b.WriteString("Create a convoy with: gt convoy create <name> [issues...]\n")
	}

	// Render convoys
	pos := 0
	for ci, c := range m.convoys {
		isSelected := pos == m.cursor

		// Convoy row
		expandIcon := "▶"
		if c.Expanded {
			expandIcon = "▼"
		}

		statusIcon := statusToIcon(c.Status)
		line := fmt.Sprintf("%s %d. %s %s: %s %s",
			expandIcon,
			ci+1,
			statusIcon,
			c.ID,
			c.Title,
			progressStyle.Render(fmt.Sprintf("(%s)", c.Progress)),
		)

		if isSelected {
			b.WriteString(selectedStyle.Render(line))
		} else {
			b.WriteString(convoyStyle.Render(line))
		}
		b.WriteString("\n")
		pos++

		// Render issues if expanded
		if c.Expanded {
			for ii, issue := range c.Issues {
				isIssueSelected := pos == m.cursor

				// Tree connector
				connector := "├─"
				if ii == len(c.Issues)-1 {
					connector = "└─"
				}

				issueIcon := "○"
				style := issueOpenStyle
				if issue.Status == "closed" {
					issueIcon = "✓"
					style = issueClosedStyle
				}

				issueLine := fmt.Sprintf("  %s %s %s: %s",
					connector,
					issueIcon,
					issue.ID,
					truncate(issue.Title, 50),
				)

				if isIssueSelected {
					b.WriteString(selectedStyle.Render(issueLine))
				} else {
					b.WriteString(style.Render(issueLine))
				}
				b.WriteString("\n")
				pos++
			}
		}
	}

	// Help footer
	b.WriteString("\n")
	if m.showHelp {
		b.WriteString(m.help.View(m.keys))
	} else {
		b.WriteString(helpStyle.Render("j/k:navigate  enter:expand  1-9:jump  q:quit  ?:help"))
	}

	return b.String()
}

// statusToIcon converts a status string to an icon.
func statusToIcon(status string) string {
	switch status {
	case "open":
		return "🚚"
	case "closed":
		return "✓"
	case "in_progress":
		return "→"
	default:
		return "●"
	}
}

// truncate shortens a string to the given rune length, preserving UTF-8.
func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if maxLen <= 3 {
		return "..."
	}
	return string(runes[:maxLen-3]) + "..."
}
