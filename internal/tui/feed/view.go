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

	if m.viewMode == ViewProblems {
		// Problems view: single panel
		problemsPanel := m.renderProblemsPanel()
		sections = append(sections, problemsPanel)
	} else {
		// Activity view: three panels
		// Tree panel (top)
		treePanel := m.renderTreePanel()
		sections = append(sections, treePanel)

		// Convoy panel (middle)
		convoyPanel := m.renderConvoyPanel()
		sections = append(sections, convoyPanel)

		// Feed panel (bottom)
		feedPanel := m.renderFeedPanel()
		sections = append(sections, feedPanel)
	}

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
	var title string
	if m.viewMode == ViewProblems {
		title = TitleStyle.Render("GT Feed") + " " + ProblemsModeStyle.Render("[PROBLEMS]")
	} else {
		title = TitleStyle.Render("GT Feed")
	}

	// Show summary stats on the right
	var stats string
	if m.viewMode == ViewProblems && len(m.problemAgents) > 0 {
		ok, stuck, idle := m.countAgentStates()
		stats = fmt.Sprintf("%d agents  %s %d ok │ %s %d stuck │ %d idle",
			len(m.problemAgents),
			AgentActiveStyle.Render("●"), ok,
			EventFailStyle.Render("●"), stuck,
			idle)
	} else if m.filter != "" {
		stats = FilterStyle.Render(fmt.Sprintf("Filter: %s", m.filter))
	} else {
		stats = FilterStyle.Render("Filter: all")
	}

	// Right-align stats
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(stats) - 4
	if gap < 1 {
		gap = 1
	}

	return HeaderStyle.Render(title + strings.Repeat(" ", gap) + stats)
}

// countAgentStates returns counts of ok, stuck, and idle agents
func (m *Model) countAgentStates() (ok, stuck, idle int) {
	for _, agent := range m.problemAgents {
		switch agent.State {
		case StateWorking:
			ok++
		case StateIdle:
			idle++
		case StateGUPPViolation, StateInputRequired, StateStalled, StateZombie:
			stuck++
		}
	}
	return
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

// renderProblemsPanel renders the problems view panel
func (m *Model) renderProblemsPanel() string {
	style := ProblemsPanelStyle
	if m.focusedPanel == PanelProblems {
		style = FocusedBorderStyle
	}
	return style.Width(m.width - 2).Render(m.problemsViewport.View())
}

// renderProblemsContent renders the problems view content
func (m *Model) renderProblemsContent() string {
	var lines []string

	if len(m.problemAgents) == 0 {
		return AgentIdleStyle.Render("No agents detected. Run gt feed in a GasTown workspace with active tmux sessions.")
	}

	// Count problems
	var problemAgents []*ProblemAgent
	var workingAgents []*ProblemAgent
	var idleAgents []*ProblemAgent

	for _, agent := range m.problemAgents {
		switch {
		case agent.State.NeedsAttention():
			problemAgents = append(problemAgents, agent)
		case agent.State == StateWorking:
			workingAgents = append(workingAgents, agent)
		default:
			idleAgents = append(idleAgents, agent)
		}
	}

	// NEEDS ATTENTION section
	if len(problemAgents) > 0 {
		lines = append(lines, ProblemsHeaderStyle.Render(fmt.Sprintf("NEEDS ATTENTION (%d)", len(problemAgents))))
		lines = append(lines, "")
		for i, agent := range problemAgents {
			isSelected := i == m.selectedProblem
			lines = append(lines, m.renderProblemAgent(agent, isSelected))
		}
		lines = append(lines, "")
	} else {
		lines = append(lines, ProblemsHeaderStyle.Render("NEEDS ATTENTION (0)"))
		lines = append(lines, "  "+AgentActiveStyle.Render("All agents OK!"))
		lines = append(lines, "")
	}

	// WORKING section (collapsed dots by rig)
	if len(workingAgents) > 0 {
		lines = append(lines, WorkingHeaderStyle.Render(fmt.Sprintf("WORKING (%d)", len(workingAgents))))
		// Group by rig
		byRig := make(map[string]int)
		for _, agent := range workingAgents {
			rig := agent.Rig
			if rig == "" {
				rig = "default"
			}
			byRig[rig]++
		}
		for rig, count := range byRig {
			dots := strings.Repeat("●", count)
			if count > 20 {
				dots = strings.Repeat("●", 20) + fmt.Sprintf("+%d", count-20)
			}
			lines = append(lines, fmt.Sprintf("  %s %s (%d)",
				AgentActiveStyle.Render(dots),
				RigStyle.Render(rig),
				count))
		}
		lines = append(lines, "")
	}

	// IDLE section (collapsed)
	if len(idleAgents) > 0 {
		lines = append(lines, IdleHeaderStyle.Render(fmt.Sprintf("IDLE (%d)", len(idleAgents))))
		dots := strings.Repeat("○", len(idleAgents))
		if len(idleAgents) > 20 {
			dots = strings.Repeat("○", 20) + fmt.Sprintf("+%d", len(idleAgents)-20)
		}
		lines = append(lines, "  "+AgentIdleStyle.Render(dots))
	}

	return strings.Join(lines, "\n")
}

// renderProblemAgent renders a single problem agent line
func (m *Model) renderProblemAgent(agent *ProblemAgent, selected bool) string {
	// Format: "▶polecat-12  🔥 GUPP!    45m (violation)  gt-xyz89   myproject"
	prefix := "  "
	if selected {
		prefix = SelectedStyle.Render("▶ ")
	}

	// Name
	name := agent.Name
	if len(name) > 12 {
		name = name[:12]
	}
	namePart := fmt.Sprintf("%-12s", name)

	// State symbol and label
	stateStyle := getStateStyle(agent.State)
	statePart := stateStyle.Render(fmt.Sprintf("%s %-6s", agent.State.Symbol(), agent.State.Label()))

	// Reason or duration
	var reasonPart string
	if agent.State == StateInputRequired {
		reasonPart = fmt.Sprintf("%-8s %s", agent.InputReason.String(), agent.DurationDisplay()+" ago")
	} else {
		reasonPart = fmt.Sprintf("%s no progress", agent.DurationDisplay())
	}
	reasonPart = fmt.Sprintf("%-20s", reasonPart)

	// Bead ID (if known)
	beadPart := ""
	if agent.CurrentBeadID != "" {
		beadPart = ConvoyIDStyle.Render(agent.CurrentBeadID)
	}

	// Rig
	rigPart := ""
	if agent.Rig != "" {
		rigPart = RigStyle.Render(agent.Rig)
	}

	return prefix + namePart + "  " + statePart + "  " + TimestampStyle.Render(reasonPart) + "  " + beadPart + "  " + rigPart
}

// getStateStyle returns the appropriate style for an agent state
func getStateStyle(state AgentState) lipgloss.Style {
	switch state {
	case StateGUPPViolation:
		return GUPPStyle
	case StateInputRequired:
		return InputRequiredStyle
	case StateStalled:
		return StalledStyle
	case StateZombie:
		return ZombieStyle
	default:
		return AgentIdleStyle
	}
}

// renderTree renders the agent tree content
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

// renderFeed renders the event feed content
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

// renderStatusBar renders the bottom status bar
func (m *Model) renderStatusBar() string {
	var left string
	if m.viewMode == ViewProblems {
		// Problems view: show problem count and selected agent
		problemCount := 0
		for _, agent := range m.problemAgents {
			if agent.State.NeedsAttention() {
				problemCount++
			}
		}
		left = fmt.Sprintf("[problems] %d need attention", problemCount)
		if m.selectedProblem >= 0 && m.selectedProblem < len(m.problemAgents) {
			left += fmt.Sprintf(" | selected: %s", m.problemAgents[m.selectedProblem].Name)
		}
	} else {
		// Activity view: show panel and event count
		var panelName string
		switch m.focusedPanel {
		case PanelTree:
			panelName = "tree"
		case PanelConvoy:
			panelName = "convoy"
		case PanelFeed:
			panelName = "feed"
		}
		left = fmt.Sprintf("[%s] %d events", panelName, len(m.events))
	}

	// Short help
	help := m.renderShortHelp()

	// Combine
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(help) - 4
	if gap < 1 {
		gap = 1
	}

	return StatusBarStyle.Width(m.width).Render(left + strings.Repeat(" ", gap) + help)
}

// renderShortHelp renders abbreviated key hints
func (m *Model) renderShortHelp() string {
	if m.viewMode == ViewProblems {
		hints := []string{
			HelpKeyStyle.Render("p") + HelpDescStyle.Render(":activity"),
			HelpKeyStyle.Render("⏎") + HelpDescStyle.Render(":attach"),
			HelpKeyStyle.Render("n") + HelpDescStyle.Render(":nudge"),
			HelpKeyStyle.Render("h") + HelpDescStyle.Render(":handoff"),
			HelpKeyStyle.Render("Tab") + HelpDescStyle.Render(":next"),
			HelpKeyStyle.Render("?") + HelpDescStyle.Render(":help"),
			HelpKeyStyle.Render("q") + HelpDescStyle.Render(":quit"),
		}
		return strings.Join(hints, "  ")
	}
	hints := []string{
		HelpKeyStyle.Render("p") + HelpDescStyle.Render(":problems"),
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
