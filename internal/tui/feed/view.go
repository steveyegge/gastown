package feed

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// render produces the full TUI output
func (m *Model) render() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var sections []string

	// Header
	sections = append(sections, m.renderHeader())

	// Tree panel (top)
	treePanel := m.renderTreePanel()
	sections = append(sections, treePanel)

	// Convoy panel (middle)
	convoyPanel := m.renderConvoyPanel()
	sections = append(sections, convoyPanel)

	// Feed panel (bottom)
	feedPanel := m.renderFeedPanel()
	sections = append(sections, feedPanel)

	// Status bar
	sections = append(sections, m.renderStatusBar())

	// Help (if shown)
	if m.showHelp {
		sections = append(sections, m.help.View(m.keys))
	}

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderHeader renders the top header bar
func (m *Model) renderHeader() string {
	title := TitleStyle.Render("GT Feed")

	filter := ""
	if m.filter != "" {
		filter = FilterStyle.Render(fmt.Sprintf("Filter: %s", m.filter))
	} else {
		filter = FilterStyle.Render("Filter: all")
	}

	// Right-align filter
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(filter) - 4
	if gap < 1 {
		gap = 1
	}

	return HeaderStyle.Render(title + strings.Repeat(" ", gap) + filter)
}

// renderTreePanel renders the agent tree panel with border
func (m *Model) renderTreePanel() string {
	style := TreePanelStyle
	if m.focusedPanel == PanelTree {
		style = FocusedBorderStyle
	}
	return style.Width(m.width - 2).Render(m.treeViewport.View())
}

// renderFeedPanel renders the event feed panel with border
func (m *Model) renderFeedPanel() string {
	style := StreamPanelStyle
	if m.focusedPanel == PanelFeed {
		style = FocusedBorderStyle
	}
	return style.Width(m.width - 2).Render(m.feedViewport.View())
}

// renderTree renders the agent tree content.
// Caller must hold m.mu.
func (m *Model) renderTree() string {
	if len(m.rigs) == 0 {
		return AgentIdleStyle.Render("No agents active")
	}

	var lines []string

	// Sort rigs by name
	rigNames := make([]string, 0, len(m.rigs))
	for name := range m.rigs {
		rigNames = append(rigNames, name)
	}
	sort.Strings(rigNames)

	for _, rigName := range rigNames {
		rig := m.rigs[rigName]

		// Rig header
		rigLine := RigStyle.Render(rigName + "/")
		lines = append(lines, rigLine)

		// Group agents by role
		byRole := m.groupAgentsByRole(rig.Agents)

		// Render each role group
		roleOrder := []string{"mayor", "witness", "refinery", "deacon", "crew", "polecat"}
		for _, role := range roleOrder {
			agents, ok := byRole[role]
			if !ok || len(agents) == 0 {
				continue
			}

			icon := RoleIcons[role]
			if icon == "" {
				icon = "•"
			}

			// For crew and polecats, show as expandable group
			if role == "crew" || role == "polecat" {
				lines = append(lines, m.renderAgentGroup(icon, role, agents))
			} else {
				// Single agents (mayor, witness, refinery)
				for _, agent := range agents {
					lines = append(lines, m.renderAgent(icon, agent, 2))
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

// groupAgentsByRole groups agents by their role
func (m *Model) groupAgentsByRole(agents map[string]*Agent) map[string][]*Agent {
	result := make(map[string][]*Agent)
	for _, agent := range agents {
		role := agent.Role
		if role == "" {
			role = "unknown"
		}
		result[role] = append(result[role], agent)
	}

	// Sort each group by name
	for role := range result {
		sort.Slice(result[role], func(i, j int) bool {
			return result[role][i].Name < result[role][j].Name
		})
	}

	return result
}

// renderAgentGroup renders a group of agents (crew or polecats)
func (m *Model) renderAgentGroup(icon, role string, agents []*Agent) string {
	var lines []string

	// Group header
	plural := role
	if role == "polecat" {
		plural = "polecats"
	}
	header := fmt.Sprintf("  %s %s/", icon, plural)
	lines = append(lines, RoleStyle.Render(header))

	// Individual agents
	for _, agent := range agents {
		lines = append(lines, m.renderAgent("", agent, 5))
	}

	return strings.Join(lines, "\n")
}

// renderAgent renders a single agent line
func (m *Model) renderAgent(icon string, agent *Agent, indent int) string {
	prefix := strings.Repeat(" ", indent)
	if icon != "" && indent >= 2 {
		prefix = strings.Repeat(" ", indent-2) + icon + " "
	} else if icon != "" {
		prefix = icon + " "
	}

	// Name with status indicator
	name := agent.Name
	// Extract just the short name if it's a full path
	if parts := strings.Split(name, "/"); len(parts) > 0 {
		name = parts[len(parts)-1]
	}

	nameStyle := AgentIdleStyle
	statusIndicator := ""
	if agent.Status == "running" || agent.Status == "working" {
		nameStyle = AgentActiveStyle
		statusIndicator = " →"
	}

	// Last activity
	activity := ""
	if agent.LastEvent != nil {
		age := formatAge(time.Since(agent.LastEvent.Time))
		msg := agent.LastEvent.Message
		if len(msg) > 40 {
			msg = msg[:37] + "..."
		}
		activity = fmt.Sprintf(" [%s] %s", age, msg)
	}

	line := prefix + nameStyle.Render(name+statusIndicator) + TimestampStyle.Render(activity)
	return line
}

// renderFeed renders the event feed content.
// Caller must hold m.mu.
func (m *Model) renderFeed() string {
	if len(m.events) == 0 {
		return AgentIdleStyle.Render("No events yet")
	}

	var lines []string

	// Show most recent events first (reversed)
	start := 0
	if len(m.events) > 100 {
		start = len(m.events) - 100
	}

	for i := len(m.events) - 1; i >= start; i-- {
		event := m.events[i]
		lines = append(lines, m.renderEvent(event))
	}

	return strings.Join(lines, "\n")
}

// renderEvent renders a single event line
func (m *Model) renderEvent(e Event) string {
	// Timestamp - compact HH:MM format, no brackets
	ts := TimestampStyle.Render(e.Time.Format("15:04"))

	// Symbol based on event type
	symbol := EventSymbols[e.Type]
	if symbol == "" {
		symbol = "•"
	}

	// Style based on event type
	var symbolStyle lipgloss.Style
	switch e.Type {
	case "create":
		symbolStyle = EventCreateStyle
	case "update":
		symbolStyle = EventUpdateStyle
	case "complete", "patrol_complete", "merged", "done":
		symbolStyle = EventCompleteStyle
	case "fail", "merge_failed":
		symbolStyle = EventFailStyle
	case "delete":
		symbolStyle = EventDeleteStyle
	case "merge_started":
		symbolStyle = EventMergeStartedStyle
	case "merge_skipped":
		symbolStyle = EventMergeSkippedStyle
	case "patrol_started", "polecat_checked":
		symbolStyle = EventUpdateStyle
	case "polecat_nudged", "escalation_sent", "nudge":
		symbolStyle = EventFailStyle // Use red/warning style for nudges and escalations
	case "sling", "hook", "spawn", "boot":
		symbolStyle = EventCreateStyle
	case "handoff", "mail":
		symbolStyle = EventUpdateStyle
	default:
		symbolStyle = EventUpdateStyle
	}

	styledSymbol := symbolStyle.Render(symbol)

	// Actor (short form)
	actor := ""
	if e.Actor != "" {
		parts := strings.Split(e.Actor, "/")
		if len(parts) > 0 {
			actor = parts[len(parts)-1]
		}
		if icon := RoleIcons[e.Role]; icon != "" {
			actor = icon + " " + actor
		}
		actor = RoleStyle.Render(actor) + ": "
	}

	// Message
	msg := e.Message
	if msg == "" && e.Raw != "" {
		msg = e.Raw
	}

	return fmt.Sprintf("%s %s %s%s", ts, styledSymbol, actor, msg)
}

// renderStatusBar renders the bottom status bar.
func (m *Model) renderStatusBar() string {
	// Panel indicator
	var panelName string
	switch m.focusedPanel {
	case PanelTree:
		panelName = "tree"
	case PanelConvoy:
		panelName = "convoy"
	case PanelFeed:
		panelName = "feed"
	}
	panel := fmt.Sprintf("[%s]", panelName)

	// Event count
	count := fmt.Sprintf("%d events", len(m.events))

	// Short help
	help := m.renderShortHelp()

	// Combine
	left := panel + " " + count
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(help) - 4
	if gap < 1 {
		gap = 1
	}

	return StatusBarStyle.Width(m.width).Render(left + strings.Repeat(" ", gap) + help)
}

// renderShortHelp renders abbreviated key hints
func (m *Model) renderShortHelp() string {
	hints := []string{
		HelpKeyStyle.Render("j/k") + HelpDescStyle.Render(":scroll"),
		HelpKeyStyle.Render("tab") + HelpDescStyle.Render(":switch"),
		HelpKeyStyle.Render("/") + HelpDescStyle.Render(":search"),
		HelpKeyStyle.Render("q") + HelpDescStyle.Render(":quit"),
		HelpKeyStyle.Render("?") + HelpDescStyle.Render(":help"),
	}
	return strings.Join(hints, "  ")
}

// formatAge formats a duration as a short age string
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
