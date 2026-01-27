package decision

import (
	"fmt"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/ui"
)

// renderView renders the entire view
func (m *Model) renderView() string {
	var b strings.Builder

	// Check terminal size
	if m.width < 40 || m.height < 10 {
		return "Terminal too small. Please resize."
	}

	// Crew wizard mode - show wizard instead of normal view
	if m.creatingCrew && m.crewWizard != nil {
		return m.crewWizard.View()
	}

	// Peek mode - show terminal content instead of normal view
	if m.peeking {
		return m.renderPeekMode()
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
		b.WriteString(helpStyle.Render("j/k: navigate  1-4: select  r: rationale  p: peek  c: crew  enter: confirm  ?: help  q: quit"))
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

	// Question (wrap to terminal width)
	wrappedPrompt := ui.WrapText(d.Prompt, m.width-4)
	b.WriteString(detailTitleStyle.Render(wrappedPrompt))
	b.WriteString("\n\n")

	// Context if available (wrap to terminal width)
	if d.Context != "" {
		b.WriteString(detailLabelStyle.Render("Context:"))
		b.WriteString("\n")
		wrappedContext := ui.WrapText(d.Context, m.width-4)
		b.WriteString(detailValueStyle.Render(wrappedContext))
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

		// Option label
		label := opt.Label
		if opt.Short != "" && opt.Short != opt.ID {
			label = opt.Short + ": " + label
		}

		// Build option line (number + label)
		optLine := fmt.Sprintf("  %s %s", numStr, label)

		if isSelected {
			b.WriteString(selectedOptionStyle.Render(optLine))
			b.WriteString(" ←")
		} else {
			b.WriteString(optionLabelStyle.Render(optLine))
		}
		b.WriteString("\n")

		// Option description on separate line, wrapped and indented
		if opt.Description != "" {
			wrappedDesc := ui.WrapText(opt.Description, m.width-10)
			// Indent each line of the wrapped description
			for _, descLine := range strings.Split(wrappedDesc, "\n") {
				b.WriteString("       ")
				b.WriteString(optionDescStyle.Render(descLine))
				b.WriteString("\n")
			}
		}
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
		b.WriteString(inputLabelStyle.Render("Custom text response (not yet implemented):"))
		b.WriteString("\n\n")
		b.WriteString(m.textInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Enter: (will show message)  Esc: cancel"))
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

// renderPeekMode renders the terminal peek overlay
func (m *Model) renderPeekMode() string {
	var b strings.Builder

	// Safety check for uninitialized dimensions
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// Header
	header := fmt.Sprintf("─── Terminal Peek: %s ", m.peekSessionName)
	header += strings.Repeat("─", max(0, m.width-len(header)-2))
	b.WriteString(titleStyle.Render(header))
	b.WriteString("\n\n")

	// Content - use viewport for scrolling
	// Handle empty content gracefully
	if m.peekContent == "" {
		b.WriteString(helpStyle.Render("(No terminal content captured)"))
		b.WriteString("\n\n")
		b.WriteString(strings.Repeat("─", max(0, m.width-2)))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("Press any key to close"))
		return b.String()
	}

	// Ensure viewport has valid dimensions before rendering
	if m.peekViewport.Width == 0 || m.peekViewport.Height == 0 {
		m.peekViewport.Width = m.width - 4
		m.peekViewport.Height = m.height - 6
	}

	b.WriteString(m.peekViewport.View())
	b.WriteString("\n")

	// Footer with scroll position
	scrollPercent := m.peekViewport.ScrollPercent() * 100
	scrollInfo := fmt.Sprintf(" %.0f%% ", scrollPercent)
	footerLeft := strings.Repeat("─", max(0, (m.width-len(scrollInfo))/2))
	footerRight := strings.Repeat("─", max(0, m.width-len(footerLeft)-len(scrollInfo)-2))
	b.WriteString(helpStyle.Render(footerLeft + scrollInfo + footerRight))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓/j/k: scroll  pgup/pgdn: page  any other key: close"))

	return b.String()
}
