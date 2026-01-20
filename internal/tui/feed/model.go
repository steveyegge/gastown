package feed

import (
	"os/exec"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/steveyegge/gastown/internal/beads"
)

// Panel represents which panel has focus
type Panel int

const (
	PanelTree Panel = iota
	PanelConvoy
	PanelFeed
	PanelProblems // Problems panel in problems view
)

// ViewMode represents which view is active
type ViewMode int

const (
	ViewActivity ViewMode = iota // Default activity stream view
	ViewProblems                 // Problem-first view
)

// Event represents an activity event
type Event struct {
	Time    time.Time
	Type    string // create, update, complete, fail, delete
	Actor   string // who did it (e.g., "gastown/crew/joe")
	Target  string // what was affected (e.g., "gt-xyz")
	Message string // human-readable description
	Rig     string // which rig
	Role    string // actor's role
	Raw     string // raw line for fallback display
}

// Agent represents an agent in the tree
type Agent struct {
	ID         string
	Name       string
	Role       string // mayor, witness, refinery, crew, polecat
	Rig        string
	Status     string // running, idle, working, dead
	LastEvent  *Event
	LastUpdate time.Time
	Expanded   bool
}

// Rig represents a rig with its agents
type Rig struct {
	Name     string
	Agents   map[string]*Agent // keyed by role/name
	Expanded bool
}

// Model is the main bubbletea model for the feed TUI
type Model struct {
	// Dimensions
	width  int
	height int

	// Panels
	focusedPanel   Panel
	treeViewport   viewport.Model
	convoyViewport viewport.Model
	feedViewport   viewport.Model

	// Data
	rigs        map[string]*Rig
	events      []Event
	convoyState *ConvoyState
	townRoot    string

	// UI state
	keys     KeyMap
	help     help.Model
	showHelp bool
	filter   string

	// View mode
	viewMode ViewMode

	// Problems view state
	problemAgents     []*ProblemAgent
	selectedProblem   int
	problemsViewport  viewport.Model
	stuckDetector     *StuckDetector
	lastProblemsCheck time.Time

	// Event source
	eventChan <-chan Event
	done      chan struct{}
	closeOnce sync.Once
}

// NewModel creates a new feed TUI model
func NewModel() *Model {
	h := help.New()
	h.ShowAll = false

	return &Model{
		focusedPanel:     PanelTree,
		treeViewport:     viewport.New(0, 0),
		convoyViewport:   viewport.New(0, 0),
		feedViewport:     viewport.New(0, 0),
		problemsViewport: viewport.New(0, 0),
		rigs:             make(map[string]*Rig),
		events:           make([]Event, 0, 1000),
		problemAgents:    make([]*ProblemAgent, 0),
		keys:             DefaultKeyMap(),
		help:             h,
		done:             make(chan struct{}),
		viewMode:         ViewActivity,
		stuckDetector:    NewStuckDetector(),
	}
}

// NewModelWithProblemsView creates a new feed TUI model starting in problems view
func NewModelWithProblemsView() *Model {
	m := NewModel()
	m.viewMode = ViewProblems
	m.focusedPanel = PanelProblems
	return m
}

// SetTownRoot sets the town root for convoy fetching
func (m *Model) SetTownRoot(townRoot string) {
	m.townRoot = townRoot
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.listenForEvents(),
		m.fetchConvoys(),
		tea.SetWindowTitle("GT Feed"),
	}
	// If starting in problems view, fetch problems immediately
	if m.viewMode == ViewProblems {
		cmds = append(cmds, m.fetchProblems())
	}
	return tea.Batch(cmds...)
}

// eventMsg is sent when a new event arrives
type eventMsg Event

// convoyUpdateMsg is sent when convoy data is refreshed
type convoyUpdateMsg struct {
	state *ConvoyState
}

// problemsUpdateMsg is sent when problems data is refreshed
type problemsUpdateMsg struct {
	agents []*ProblemAgent
}

// tickMsg is sent periodically to refresh the view
type tickMsg time.Time

// listenForEvents returns a command that listens for events
func (m *Model) listenForEvents() tea.Cmd {
	if m.eventChan == nil {
		return nil
	}
	// Capture channels to avoid race with Model mutations
	eventChan := m.eventChan
	done := m.done
	return func() tea.Msg {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return nil
			}
			return eventMsg(event)
		case <-done:
			return nil
		}
	}
}

// tick returns a command for periodic refresh
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// fetchConvoys returns a command that fetches convoy data
func (m *Model) fetchConvoys() tea.Cmd {
	if m.townRoot == "" {
		return nil
	}
	townRoot := m.townRoot
	return func() tea.Msg {
		state, _ := FetchConvoys(townRoot)
		return convoyUpdateMsg{state: state}
	}
}

// convoyRefreshTick returns a command that schedules the next convoy refresh
func (m *Model) convoyRefreshTick() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg {
		return convoyUpdateMsg{} // Empty state triggers a refresh
	})
}

// fetchProblems returns a command that fetches problem agent data
func (m *Model) fetchProblems() tea.Cmd {
	detector := m.stuckDetector
	return func() tea.Msg {
		sessions, err := detector.FindGasTownSessions()
		if err != nil {
			return problemsUpdateMsg{agents: nil}
		}

		var problems []*ProblemAgent
		for _, sessionID := range sessions {
			agent := detector.AnalyzeSession(sessionID)
			problems = append(problems, agent)
		}

		// Sort by priority (problems first)
		sortProblemAgents(problems)

		return problemsUpdateMsg{agents: problems}
	}
}

// problemsRefreshTick returns a command that schedules the next problems refresh
func (m *Model) problemsRefreshTick() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return problemsUpdateMsg{} // Empty triggers refresh
	})
}

// sortProblemAgents sorts agents by state priority (problems first)
func sortProblemAgents(agents []*ProblemAgent) {
	// Sort by state priority, then by idle time (longest first)
	for i := 0; i < len(agents); i++ {
		for j := i + 1; j < len(agents); j++ {
			// Compare by state priority first
			if agents[i].State.Priority() > agents[j].State.Priority() {
				agents[i], agents[j] = agents[j], agents[i]
			} else if agents[i].State.Priority() == agents[j].State.Priority() {
				// Same state - sort by idle time (longer first)
				if agents[i].IdleMinutes < agents[j].IdleMinutes {
					agents[i], agents[j] = agents[j], agents[i]
				}
			}
		}
	}
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateViewportSizes()

	case eventMsg:
		m.addEvent(Event(msg))
		cmds = append(cmds, m.listenForEvents())

	case convoyUpdateMsg:
		if msg.state != nil {
			// Fresh data arrived - update state and schedule next tick
			m.convoyState = msg.state
			m.updateViewContent()
			cmds = append(cmds, m.convoyRefreshTick())
		} else {
			// Tick fired - fetch new data
			cmds = append(cmds, m.fetchConvoys())
		}

	case problemsUpdateMsg:
		if msg.agents != nil {
			// Fresh data arrived - update state and schedule next tick
			m.problemAgents = msg.agents
			m.lastProblemsCheck = time.Now()
			// Keep selection in bounds
			if m.selectedProblem >= len(m.problemAgents) {
				m.selectedProblem = len(m.problemAgents) - 1
			}
			if m.selectedProblem < 0 {
				m.selectedProblem = 0
			}
			m.updateViewContent()
			if m.viewMode == ViewProblems {
				cmds = append(cmds, m.problemsRefreshTick())
			}
		} else {
			// Tick fired - fetch new data if in problems view
			if m.viewMode == ViewProblems {
				cmds = append(cmds, m.fetchProblems())
			}
		}

	case tickMsg:
		cmds = append(cmds, tick())
	}

	// Update viewports
	var cmd tea.Cmd
	switch m.focusedPanel {
	case PanelTree:
		m.treeViewport, cmd = m.treeViewport.Update(msg)
	case PanelConvoy:
		m.convoyViewport, cmd = m.convoyViewport.Update(msg)
	case PanelFeed:
		m.feedViewport, cmd = m.feedViewport.Update(msg)
	case PanelProblems:
		m.problemsViewport, cmd = m.problemsViewport.Update(msg)
	}
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleKey processes key presses
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.closeOnce.Do(func() { close(m.done) })
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.help.ShowAll = m.showHelp
		return m, nil

	case key.Matches(msg, m.keys.ToggleProblems):
		return m.toggleProblemsView()

	case key.Matches(msg, m.keys.Tab):
		return m.handleTabKey()

	case key.Matches(msg, m.keys.FocusTree):
		if m.viewMode == ViewActivity {
			m.focusedPanel = PanelTree
		}
		return m, nil

	case key.Matches(msg, m.keys.FocusFeed):
		if m.viewMode == ViewActivity {
			m.focusedPanel = PanelFeed
		}
		return m, nil

	case key.Matches(msg, m.keys.FocusConvoy):
		if m.viewMode == ViewActivity {
			m.focusedPanel = PanelConvoy
		}
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		m.updateViewContent()
		if m.viewMode == ViewProblems {
			return m, m.fetchProblems()
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		if m.viewMode == ViewProblems {
			return m.attachToSelected()
		}

	case key.Matches(msg, m.keys.Nudge):
		if m.viewMode == ViewProblems {
			return m.nudgeSelected()
		}

	case key.Matches(msg, m.keys.Handoff):
		if m.viewMode == ViewProblems {
			return m.handoffSelected()
		}

	case key.Matches(msg, m.keys.Restart):
		if m.viewMode == ViewProblems {
			return m.restartSelected()
		}

	case key.Matches(msg, m.keys.Up):
		if m.viewMode == ViewProblems {
			return m.selectPrevProblem()
		}

	case key.Matches(msg, m.keys.Down):
		if m.viewMode == ViewProblems {
			return m.selectNextProblem()
		}
	}

	// Pass to focused viewport
	var cmd tea.Cmd
	switch m.focusedPanel {
	case PanelTree:
		m.treeViewport, cmd = m.treeViewport.Update(msg)
	case PanelConvoy:
		m.convoyViewport, cmd = m.convoyViewport.Update(msg)
	case PanelFeed:
		m.feedViewport, cmd = m.feedViewport.Update(msg)
	case PanelProblems:
		m.problemsViewport, cmd = m.problemsViewport.Update(msg)
	}
	return m, cmd
}

// toggleProblemsView switches between activity and problems view
func (m *Model) toggleProblemsView() (tea.Model, tea.Cmd) {
	if m.viewMode == ViewProblems {
		m.viewMode = ViewActivity
		m.focusedPanel = PanelTree
		m.updateViewportSizes()
		return m, nil
	}
	m.viewMode = ViewProblems
	m.focusedPanel = PanelProblems
	m.updateViewportSizes()
	// Fetch problems if we haven't recently
	if time.Since(m.lastProblemsCheck) > 5*time.Second {
		return m, m.fetchProblems()
	}
	return m, nil
}

// handleTabKey handles Tab key for panel/problem cycling
func (m *Model) handleTabKey() (tea.Model, tea.Cmd) {
	if m.viewMode == ViewProblems {
		// In problems view, Tab cycles through problem agents
		return m.selectNextProblem()
	}
	// In activity view, Tab cycles panels
	switch m.focusedPanel {
	case PanelTree:
		m.focusedPanel = PanelConvoy
	case PanelConvoy:
		m.focusedPanel = PanelFeed
	case PanelFeed:
		m.focusedPanel = PanelTree
	}
	return m, nil
}

// selectNextProblem moves selection to next problem agent
func (m *Model) selectNextProblem() (tea.Model, tea.Cmd) {
	if len(m.problemAgents) == 0 {
		return m, nil
	}
	// Count problem agents
	problemCount := 0
	for _, agent := range m.problemAgents {
		if agent.State.NeedsAttention() {
			problemCount++
		}
	}
	if problemCount == 0 {
		return m, nil
	}
	m.selectedProblem++
	if m.selectedProblem >= problemCount {
		m.selectedProblem = 0
	}
	m.updateViewContent()
	return m, nil
}

// selectPrevProblem moves selection to previous problem agent
func (m *Model) selectPrevProblem() (tea.Model, tea.Cmd) {
	if len(m.problemAgents) == 0 {
		return m, nil
	}
	// Count problem agents
	problemCount := 0
	for _, agent := range m.problemAgents {
		if agent.State.NeedsAttention() {
			problemCount++
		}
	}
	if problemCount == 0 {
		return m, nil
	}
	m.selectedProblem--
	if m.selectedProblem < 0 {
		m.selectedProblem = problemCount - 1
	}
	m.updateViewContent()
	return m, nil
}

// getSelectedProblemAgent returns the currently selected problem agent
func (m *Model) getSelectedProblemAgent() *ProblemAgent {
	if m.selectedProblem < 0 || len(m.problemAgents) == 0 {
		return nil
	}
	// Find the nth problem agent
	idx := 0
	for _, agent := range m.problemAgents {
		if agent.State.NeedsAttention() {
			if idx == m.selectedProblem {
				return agent
			}
			idx++
		}
	}
	return nil
}

// attachToSelected attaches to the selected agent's tmux session
func (m *Model) attachToSelected() (tea.Model, tea.Cmd) {
	agent := m.getSelectedProblemAgent()
	if agent == nil {
		return m, nil
	}
	// Exit TUI and attach to tmux session
	m.closeOnce.Do(func() { close(m.done) })
	c := exec.Command("tmux", "attach-session", "-t", agent.SessionID)
	return m, tea.Sequence(
		tea.ExitAltScreen,
		tea.ExecProcess(c, nil),
	)
}

// nudgeSelected sends a nudge to the selected agent
func (m *Model) nudgeSelected() (tea.Model, tea.Cmd) {
	agent := m.getSelectedProblemAgent()
	if agent == nil {
		return m, nil
	}
	// Run gt nudge in background
	c := exec.Command("gt", "nudge", agent.Name, "continue")
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		// Refresh problems after nudge
		return problemsUpdateMsg{}
	})
}

// handoffSelected sends a handoff request to the selected agent
func (m *Model) handoffSelected() (tea.Model, tea.Cmd) {
	agent := m.getSelectedProblemAgent()
	if agent == nil {
		return m, nil
	}
	// Run gt nudge with handoff message
	c := exec.Command("gt", "nudge", agent.Name, "handoff")
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return problemsUpdateMsg{}
	})
}

// restartSelected restarts the selected agent
func (m *Model) restartSelected() (tea.Model, tea.Cmd) {
	agent := m.getSelectedProblemAgent()
	if agent == nil {
		return m, nil
	}
	// Run gt polecat restart
	c := exec.Command("gt", "polecat", "restart", agent.Name)
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return problemsUpdateMsg{}
	})
}

// updateViewportSizes recalculates viewport dimensions
func (m *Model) updateViewportSizes() {
	// Reserve space: header (1) + borders (6 for 3 panels) + status bar (1) + help (1-2)
	headerHeight := 1
	statusHeight := 1
	helpHeight := 1
	if m.showHelp {
		helpHeight = 3
	}
	borderHeight := 6 // top and bottom borders for 3 panels

	availableHeight := m.height - headerHeight - statusHeight - helpHeight - borderHeight
	if availableHeight < 6 {
		availableHeight = 6
	}

	contentWidth := m.width - 4 // borders and padding
	if contentWidth < 20 {
		contentWidth = 20
	}

	if m.viewMode == ViewProblems {
		// Problems view: single large panel
		m.problemsViewport.Width = contentWidth
		m.problemsViewport.Height = availableHeight
	} else {
		// Activity view: 30% tree, 25% convoy, 45% feed
		treeHeight := availableHeight * 30 / 100
		convoyHeight := availableHeight * 25 / 100
		feedHeight := availableHeight - treeHeight - convoyHeight

		// Ensure minimum heights
		if treeHeight < 3 {
			treeHeight = 3
		}
		if convoyHeight < 3 {
			convoyHeight = 3
		}
		if feedHeight < 3 {
			feedHeight = 3
		}

		m.treeViewport.Width = contentWidth
		m.treeViewport.Height = treeHeight
		m.convoyViewport.Width = contentWidth
		m.convoyViewport.Height = convoyHeight
		m.feedViewport.Width = contentWidth
		m.feedViewport.Height = feedHeight
	}

	m.updateViewContent()
}

// updateViewContent refreshes the content of all viewports
func (m *Model) updateViewContent() {
	if m.viewMode == ViewProblems {
		m.problemsViewport.SetContent(m.renderProblemsContent())
	} else {
		m.treeViewport.SetContent(m.renderTree())
		m.convoyViewport.SetContent(m.renderConvoys())
		m.feedViewport.SetContent(m.renderFeed())
	}
}

// addEvent adds an event and updates the agent tree
func (m *Model) addEvent(e Event) {
	// Update agent tree first (always do this for status tracking)
	if e.Rig != "" {
		rig, ok := m.rigs[e.Rig]
		if !ok {
			rig = &Rig{
				Name:     e.Rig,
				Agents:   make(map[string]*Agent),
				Expanded: true,
			}
			m.rigs[e.Rig] = rig
		}

		if e.Actor != "" {
			agent, ok := rig.Agents[e.Actor]
			if !ok {
				agent = &Agent{
					ID:   e.Actor,
					Name: e.Actor,
					Role: e.Role,
					Rig:  e.Rig,
				}
				rig.Agents[e.Actor] = agent
			}
			agent.LastEvent = &e
			agent.LastUpdate = e.Time
		}
	}

	// Filter out events with empty bead IDs (malformed mutations)
	if e.Type == "update" && e.Target == "" {
		return
	}

	// Filter out noisy agent session updates from the event feed.
	// Agent session molecules (like gt-gastown-crew-joe) update frequently
	// for status tracking. These updates are visible in the agent tree,
	// so we don't need to clutter the event feed with them.
	// We still show create/complete/fail/delete events for agent sessions.
	if e.Type == "update" && beads.IsAgentSessionBead(e.Target) {
		// Skip adding to event feed, but still refresh the view
		// (agent tree was updated above)
		m.updateViewContent()
		return
	}

	// Deduplicate rapid updates to the same bead within 2 seconds.
	// This prevents spam when multiple deps/labels are added to one issue.
	if e.Type == "update" && e.Target != "" && len(m.events) > 0 {
		lastEvent := m.events[len(m.events)-1]
		if lastEvent.Type == "update" && lastEvent.Target == e.Target {
			// Same bead updated within 2 seconds - skip duplicate
			if e.Time.Sub(lastEvent.Time) < 2*time.Second {
				return
			}
		}
	}

	// Add to event feed
	m.events = append(m.events, e)

	// Keep max 1000 events
	if len(m.events) > 1000 {
		m.events = m.events[len(m.events)-1000:]
	}

	m.updateViewContent()
}

// SetEventChannel sets the channel to receive events from
func (m *Model) SetEventChannel(ch <-chan Event) {
	m.eventChan = ch
}

// View renders the TUI
func (m *Model) View() string {
	return m.render()
}
