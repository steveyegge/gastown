package decision

import (
	"fmt"
	"strings"
	"time"
)

// renderView renders the entire view
func (m *Model) renderView() string {
	var b strings.Builder

	// Check terminal size
	if m.width < 40 || m.height < 10 {
		return "Terminal too small. Please resize."
	}

	// Title
	b.WriteString(titleStyle.Render("🎯 Decision Watch"))
	b.WriteString("\n")

	// Error message
	if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Input mode
	if m.inputMode != ModeNormal {
		return m.renderInputMode(&b)
	}

	// Empty state
	if len(m.decisions) == 0 {
		b.WriteString("\n")
		b.WriteString(statusStyle.Render("No pending decisions."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Watching for new decisions... (press q to quit)"))
		b.WriteString("\n")
		return b.String()
	}

	// Calculate layout
	listHeight := m.height / 3
	if listHeight < 5 {
		listHeight = 5
	}

	// Render decision list
	b.WriteString(m.renderDecisionList(listHeight))
	b.WriteString("\n")

	// Render detail pane for selected decision
	if m.selected < len(m.decisions) {
		b.WriteString(m.renderDetailPane())
	}

	// Status line
	if m.status != "" {
		b.WriteString("\n")
		b.WriteString(statusStyle.Render(m.status))
	}

	// Help
	if m.showHelp {
		b.WriteString("\n\n")
		b.WriteString(m.help.View(m.keys))
	} else {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("j/k: navigate  1-4: select option  r: rationale  enter: confirm  ?: help  q: quit"))
	}

	return b.String()
}

// renderDecisionList renders the list of pending decisions
func (m *Model) renderDecisionList(maxHeight int) string {
	var b strings.Builder

	header := fmt.Sprintf("─── Pending Decisions (%d) ", len(m.decisions))
	header += strings.Repeat("─", max(0, m.width-len(header)-2))
	b.WriteString(helpStyle.Render(header))
	b.WriteString("\n")

	for i, d := range m.decisions {
		if i >= maxHeight-2 {
			b.WriteString(helpStyle.Render(fmt.Sprintf("  ... and %d more", len(m.decisions)-i)))
			break
		}

		isSelected := i == m.selected
		line := m.renderDecisionLine(d, isSelected)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// renderDecisionLine renders a single decision list item
func (m *Model) renderDecisionLine(d DecisionItem, selected bool) string {
	// Urgency indicator
	urgency := urgencyLabel(d.Urgency)

	// Time ago
	timeAgo := formatTimeAgo(d.RequestedAt)

	// Truncate prompt if needed
	prompt := d.Prompt
	maxPromptLen := m.width - 40
	if maxPromptLen < 20 {
		maxPromptLen = 20
	}
	if len(prompt) > maxPromptLen {
		prompt = prompt[:maxPromptLen-3] + "..."
	}

	// Build line
	line := fmt.Sprintf(" %s %s - %s", urgency, d.ID, prompt)

	// Add requester and time
	meta := fmt.Sprintf("  %s · %s", detailLabelStyle.Render(d.RequestedBy), detailLabelStyle.Render(timeAgo))

	if selected {
		return selectedItemStyle.Render(line) + "\n" + meta
	}
	return normalItemStyle.Render(line) + "\n" + meta
}

// renderDetailPane renders the detail view for the selected decision
func (m *Model) renderDetailPane() string {
	if m.selected >= len(m.decisions) {
		return ""
	}

	d := m.decisions[m.selected]
	var b strings.Builder

	// Header
	header := "─── Decision Details "
	header += strings.Repeat("─", max(0, m.width-len(header)-2))
	b.WriteString(helpStyle.Render(header))
	b.WriteString("\n\n")

	// Question
	b.WriteString(detailTitleStyle.Render(d.Prompt))
	b.WriteString("\n\n")

	// Context if available
	if d.Context != "" {
		b.WriteString(detailLabelStyle.Render("Context:"))
		b.WriteString("\n")
		b.WriteString(wrapText(detailValueStyle.Render(d.Context), m.width-4))
		b.WriteString("\n\n")
	}

	// Analysis if available
	if d.Analysis != "" {
		b.WriteString(detailLabelStyle.Render("Analysis:"))
		b.WriteString("\n")
		b.WriteString(wrapText(detailValueStyle.Render(d.Analysis), m.width-4))
		b.WriteString("\n\n")
	}

	// Tradeoffs if available
	if d.Tradeoffs != "" {
		b.WriteString(detailLabelStyle.Render("Tradeoffs:"))
		b.WriteString("\n")
		b.WriteString(wrapText(detailValueStyle.Render(d.Tradeoffs), m.width-4))
		b.WriteString("\n\n")
	}

	// Options
	b.WriteString(detailLabelStyle.Render("Options:"))
	b.WriteString("\n")

	for i, opt := range d.Options {
		optNum := i + 1
		isSelected := optNum == m.selectedOption

		// Option number
		numStr := optionNumberStyle.Render(fmt.Sprintf("[%d]", optNum))

		// Option label with recommended marker
		label := opt.Label
		if opt.Short != "" && opt.Short != opt.ID {
			label = opt.Short + ": " + label
		}
		if opt.Recommended {
			label += " ★"
		}

		// Option description
		desc := ""
		if opt.Description != "" {
			desc = " - " + optionDescStyle.Render(opt.Description)
		}

		line := fmt.Sprintf("  %s %s%s", numStr, label, desc)

		if isSelected {
			b.WriteString(selectedOptionStyle.Render(line))
			b.WriteString(" ←")
		} else {
			b.WriteString(optionLabelStyle.Render(line))
		}
		b.WriteString("\n")

		// Pros if available
		if len(opt.Pros) > 0 {
			for _, pro := range opt.Pros {
				b.WriteString(successStyle.Render(fmt.Sprintf("      + %s", pro)))
				b.WriteString("\n")
			}
		}

		// Cons if available
		if len(opt.Cons) > 0 {
			for _, con := range opt.Cons {
				b.WriteString(errorStyle.Render(fmt.Sprintf("      - %s", con)))
				b.WriteString("\n")
			}
		}
	}

	// Recommendation rationale if available
	if d.RecommendationRationale != "" {
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Recommendation:"))
		b.WriteString("\n")
		b.WriteString(wrapText(detailValueStyle.Render(d.RecommendationRationale), m.width-4))
		b.WriteString("\n")
	}

	// Show selected option instructions
	if m.selectedOption > 0 {
		b.WriteString("\n")
		b.WriteString(successStyle.Render(fmt.Sprintf("Option %d selected. ", m.selectedOption)))
		b.WriteString(helpStyle.Render("Press Enter to confirm, r for rationale"))
	}

	// Show rationale if set
	if m.rationale != "" {
		b.WriteString("\n")
		b.WriteString(detailLabelStyle.Render("Rationale: "))
		b.WriteString(detailValueStyle.Render(m.rationale))
	}

	return b.String()
}

// wrapText wraps text to a given width, preserving newlines
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}

		// Simple word wrapping
		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) <= width {
				currentLine += " " + word
			} else {
				result.WriteString(currentLine)
				result.WriteString("\n")
				currentLine = word
			}
		}
		result.WriteString(currentLine)
	}

	return result.String()
}

// renderInputMode renders the input mode view
func (m *Model) renderInputMode(b *strings.Builder) string {
	b.WriteString("\n")

	switch m.inputMode {
	case ModeRationale:
		b.WriteString(inputLabelStyle.Render("Enter rationale:"))
		b.WriteString("\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Enter: confirm  Esc: cancel"))

	case ModeText:
		b.WriteString(inputLabelStyle.Render("Enter custom response (triggers iteration):"))
		b.WriteString("\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Enter: submit  Esc: cancel"))
	}

	return b.String()
}

// formatTimeAgo formats a time as "X ago"
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}

	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
