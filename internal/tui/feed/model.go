package feed

import (
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
)

// Layout constants for panel height distribution and event history.
const (
	treePanelPercent   = 30
	convoyPanelPercent = 25
	maxEventHistory    = 1000
)

// Event represents an activity event
type Event struct {
	Time     time.Time
	Type     string // create, update, complete, fail, delete
	Actor    string // who did it (e.g., "gastown/crew/joe")
	Target   string // what was affected (e.g., "gt-xyz")
	Message  string // human-readable description
	Rig      string // which rig
	Role     string // actor's role
	Raw      string // raw line for fallback display
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

	// Event source
	eventChan <-chan Event
	done      chan struct{}
	closeOnce sync.Once

	// mu protects all fields read by View() from concurrent access:
	// events, rigs, convoyState, eventChan, townRoot, width, height,
	// focusedPanel, showHelp, help, filter, and the three viewports.
	// Write lock is held during Update/handleKey mutations; read lock
	// is held during View/render.
	mu sync.RWMutex
}

// NewModel creates a new feed TUI model
func NewModel() *Model {
	h := help.New()
	h.ShowAll = false

	return &Model{
		focusedPanel:   PanelTree,
		treeViewport:   viewport.New(0, 0),
		convoyViewport: viewport.New(0, 0),
		feedViewport:   viewport.New(0, 0),
		rigs:           make(map[string]*Rig),
		events:         make([]Event, 0, maxEventHistory),
		keys:           DefaultKeyMap(),
		help:           h,
		done:           make(chan struct{}),
	}
}

// SetTownRoot sets the town root for convoy fetching.
// Safe to call concurrently with the Bubble Tea event loop.
func (m *Model) SetTownRoot(townRoot string) {
	m.mu.Lock()
	m.townRoot = townRoot
	m.mu.Unlock()
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.listenForEvents(),
		m.fetchConvoys(),
		tea.SetWindowTitle("GT Feed"),
	)
}

// eventMsg is sent when a new event arrives
type eventMsg Event

// convoyUpdateMsg is sent when convoy data is refreshed
type convoyUpdateMsg struct {
	state *ConvoyState
}

// tickMsg is sent periodically to refresh the view
type tickMsg time.Time

// listenForEvents returns a command that listens for events.
// Captures channels under the read lock to avoid racing with SetEventChannel.
func (m *Model) listenForEvents() tea.Cmd {
	m.mu.RLock()
	eventChan := m.eventChan
	done := m.done
	m.mu.RUnlock()

	if eventChan == nil {
		return nil
	}
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

// fetchConvoys returns a command that fetches convoy data.
// Captures townRoot under the read lock to avoid racing with SetTownRoot.
func (m *Model) fetchConvoys() tea.Cmd {
	m.mu.RLock()
	townRoot := m.townRoot
	m.mu.RUnlock()

	if townRoot == "" {
		return nil
	}
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

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.mu.Lock()
		m.width = msg.Width
		m.height = msg.Height
		m.mu.Unlock()
		m.updateViewportSizes()

	case eventMsg:
		m.addEvent(Event(msg))
		cmds = append(cmds, m.listenForEvents())

	case convoyUpdateMsg:
		if msg.state != nil {
			// Fresh data arrived - update state and schedule next tick
			m.mu.Lock()
			m.convoyState = msg.state
			m.updateViewContentLocked()
			m.mu.Unlock()
			cmds = append(cmds, m.convoyRefreshTick())
		} else {
			// Tick fired - fetch new data
			cmds = append(cmds, m.fetchConvoys())
		}

	case tickMsg:
		cmds = append(cmds, tick())
	}

	// Update viewports (under lock to protect from concurrent View)
	m.mu.Lock()
	var cmd tea.Cmd
	switch m.focusedPanel {
	case PanelTree:
		m.treeViewport, cmd = m.treeViewport.Update(msg)
	case PanelConvoy:
		m.convoyViewport, cmd = m.convoyViewport.Update(msg)
	case PanelFeed:
		m.feedViewport, cmd = m.feedViewport.Update(msg)
	}
	m.mu.Unlock()
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
		m.mu.Lock()
		m.showHelp = !m.showHelp
		m.help.ShowAll = m.showHelp
		m.mu.Unlock()
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		// Cycle: Tree -> Convoy -> Feed -> Tree
		m.mu.Lock()
		switch m.focusedPanel {
		case PanelTree:
			m.focusedPanel = PanelConvoy
		case PanelConvoy:
			m.focusedPanel = PanelFeed
		case PanelFeed:
			m.focusedPanel = PanelTree
		}
		m.mu.Unlock()
		return m, nil

	case key.Matches(msg, m.keys.FocusTree):
		m.mu.Lock()
		m.focusedPanel = PanelTree
		m.mu.Unlock()
		return m, nil

	case key.Matches(msg, m.keys.FocusFeed):
		m.mu.Lock()
		m.focusedPanel = PanelFeed
		m.mu.Unlock()
		return m, nil

	case key.Matches(msg, m.keys.FocusConvoy):
		m.mu.Lock()
		m.focusedPanel = PanelConvoy
		m.mu.Unlock()
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		m.updateViewContent()
		return m, nil
	}

	// Pass to focused viewport (under lock to protect from concurrent View)
	m.mu.Lock()
	var cmd tea.Cmd
	switch m.focusedPanel {
	case PanelTree:
		m.treeViewport, cmd = m.treeViewport.Update(msg)
	case PanelConvoy:
		m.convoyViewport, cmd = m.convoyViewport.Update(msg)
	case PanelFeed:
		m.feedViewport, cmd = m.feedViewport.Update(msg)
	}
	m.mu.Unlock()
	return m, cmd
}

// updateViewportSizes recalculates viewport dimensions.
// Acquires the write lock for the entire operation so that reads of
// width/height/showHelp and writes to viewports are atomic with View().
func (m *Model) updateViewportSizes() {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	// Split: tree/convoy/feed by configured percentages
	treeHeight := availableHeight * treePanelPercent / 100
	convoyHeight := availableHeight * convoyPanelPercent / 100
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

	contentWidth := m.width - 4 // borders and padding
	if contentWidth < 20 {
		contentWidth = 20
	}

	m.treeViewport.Width = contentWidth
	m.treeViewport.Height = treeHeight
	m.convoyViewport.Width = contentWidth
	m.convoyViewport.Height = convoyHeight
	m.feedViewport.Width = contentWidth
	m.feedViewport.Height = feedHeight
	m.updateViewContentLocked()
}

// updateViewContent refreshes the content of all viewports.
// Acquires the write lock to protect viewport and data access.
func (m *Model) updateViewContent() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateViewContentLocked()
}

// updateViewContentLocked refreshes viewport content.
// Caller must hold m.mu.
func (m *Model) updateViewContentLocked() {
	m.treeViewport.SetContent(m.renderTree())
	m.convoyViewport.SetContent(m.renderConvoys())
	m.feedViewport.SetContent(m.renderFeed())
}

// addEvent adds an event and updates the agent tree.
// Acquires mu for the entire operation including view updates.
func (m *Model) addEvent(e Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.addEventLocked(e) {
		m.updateViewContentLocked()
	}
}

// addEventLocked performs the actual event mutation under the write lock.
// Returns true if the caller should call updateViewContent afterward.
// Caller must hold m.mu write lock.
func (m *Model) addEventLocked(e Event) bool {
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
		return false
	}

	// Filter out noisy agent session updates from the event feed.
	// Agent session molecules (like gt-gastown-crew-joe) update frequently
	// for status tracking. These updates are visible in the agent tree,
	// so we don't need to clutter the event feed with them.
	// We still show create/complete/fail/delete events for agent sessions.
	if e.Type == "update" && beads.IsAgentSessionBead(e.Target) {
		// Skip adding to event feed, but still refresh the view
		// (agent tree was updated above)
		return true
	}

	// Deduplicate rapid updates to the same bead within 2 seconds.
	// This prevents spam when multiple deps/labels are added to one issue.
	if e.Type == "update" && e.Target != "" && len(m.events) > 0 {
		lastEvent := m.events[len(m.events)-1]
		if lastEvent.Type == "update" && lastEvent.Target == e.Target {
			// Same bead updated within 2 seconds - skip duplicate
			if e.Time.Sub(lastEvent.Time) < 2*time.Second {
				return false
			}
		}
	}

	// Add to event feed
	m.events = append(m.events, e)

	// Keep max events within history limit
	if len(m.events) > maxEventHistory {
		m.events = m.events[len(m.events)-maxEventHistory:]
	}

	return true
}

// SetEventChannel sets the channel to receive events from.
// Safe to call concurrently with the Bubble Tea event loop.
func (m *Model) SetEventChannel(ch <-chan Event) {
	m.mu.Lock()
	m.eventChan = ch
	m.mu.Unlock()
}

// View renders the TUI.
// Acquires the read lock to safely access model state from the render path.
func (m *Model) View() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.render()
}
