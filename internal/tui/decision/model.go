package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	crewTUI "github.com/steveyegge/gastown/internal/tui/crew"
)

// pollInterval is how often we check for new decisions
const pollInterval = 5 * time.Second

// Option represents a decision option
type Option struct {
	ID          string `json:"id"`
	Short       string `json:"short"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// DecisionItem represents a pending decision
type DecisionItem struct {
	ID          string    `json:"id"`
	Prompt      string    `json:"prompt"`
	Options     []Option  `json:"options"`
	Urgency     string    `json:"urgency"`
	RequestedBy string    `json:"requested_by"`
	RequestedAt time.Time `json:"requested_at"`
	Context     string    `json:"context"`
}

// rawDecisionItem is the actual JSON format from gt decision list --json
type rawDecisionItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"created_at"`
	CreatedBy   string   `json:"created_by"`
	Labels      []string `json:"labels"`
}

// UnmarshalJSON implements custom JSON unmarshaling to handle the actual format
func (d *DecisionItem) UnmarshalJSON(data []byte) error {
	var raw rawDecisionItem
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	d.ID = raw.ID
	d.Prompt = raw.Title
	d.Urgency = extractUrgencyFromLabels(raw.Labels)
	d.Options = parseOptionsFromDescription(raw.Description)
	d.Context = extractContextFromDescription(raw.Description)

	// Parse timestamp
	if raw.CreatedAt != "" {
		t, err := time.Parse(time.RFC3339, raw.CreatedAt)
		if err == nil {
			d.RequestedAt = t
		}
	}

	// Extract requested_by from description or use created_by
	d.RequestedBy = extractRequestedByFromDescription(raw.Description)
	if d.RequestedBy == "" {
		d.RequestedBy = raw.CreatedBy
	}

	return nil
}

// extractUrgencyFromLabels extracts urgency level from labels array
func extractUrgencyFromLabels(labels []string) string {
	for _, label := range labels {
		if strings.HasPrefix(label, "urgency:") {
			return strings.TrimPrefix(label, "urgency:")
		}
	}
	return "medium" // default
}

// parseOptionsFromDescription parses options from markdown description
func parseOptionsFromDescription(desc string) []Option {
	var options []Option

	lines := strings.Split(desc, "\n")
	var currentOption *Option
	var descLines []string

	for _, line := range lines {
		// Look for option headers: "### 1. Label" or "### N. Label"
		if strings.HasPrefix(line, "### ") {
			// Save previous option if exists
			if currentOption != nil {
				currentOption.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
				options = append(options, *currentOption)
				descLines = nil
			}

			// Parse new option header
			header := strings.TrimPrefix(line, "### ")
			// Remove leading number and dot: "1. Label" -> "Label"
			if dotIdx := strings.Index(header, ". "); dotIdx != -1 {
				header = strings.TrimSpace(header[dotIdx+2:])
			}

			currentOption = &Option{
				ID:    fmt.Sprintf("%d", len(options)+1),
				Label: header,
			}
		} else if currentOption != nil {
			// Check for end markers
			if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "## ") {
				// Save current option and stop
				currentOption.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
				options = append(options, *currentOption)
				currentOption = nil
				descLines = nil
			} else if strings.TrimSpace(line) != "" {
				descLines = append(descLines, strings.TrimSpace(line))
			}
		}
	}

	// Don't forget the last option
	if currentOption != nil {
		currentOption.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
		options = append(options, *currentOption)
	}

	return options
}

// extractContextFromDescription extracts context section from description
func extractContextFromDescription(desc string) string {
	// Look for context between "## Context" and the next section
	lines := strings.Split(desc, "\n")
	var contextLines []string
	inContext := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## Context") {
			inContext = true
			continue
		}
		if inContext {
			if strings.HasPrefix(line, "## ") {
				break
			}
			if strings.TrimSpace(line) != "" {
				contextLines = append(contextLines, strings.TrimSpace(line))
			}
		}
	}

	return strings.Join(contextLines, "\n")
}

// extractRequestedByFromDescription extracts requester from markdown footer
func extractRequestedByFromDescription(desc string) string {
	// Look for "_Requested by: xxx_" pattern
	lines := strings.Split(desc, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "_Requested by:") && strings.HasSuffix(line, "_") {
			// Extract between ":" and trailing "_"
			inner := strings.TrimPrefix(line, "_Requested by:")
			inner = strings.TrimSuffix(inner, "_")
			return strings.TrimSpace(inner)
		}
	}
	return ""
}

// InputMode represents the current input mode
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeRationale
	ModeText
)

// Model is the bubbletea model for the decision TUI
type Model struct {
	// Dimensions
	width  int
	height int

	// Data
	decisions      []DecisionItem
	selected       int
	selectedOption int // 0 = none, 1-4 = option number

	// Input
	inputMode InputMode
	textInput textarea.Model
	rationale string

	// UI state
	keys           KeyMap
	help           help.Model
	showHelp       bool
	detailViewport viewport.Model
	filter         string // "high", "all", etc.
	notify         bool   // desktop notifications
	err            error
	status         string

	// Peek state - for viewing agent terminal
	peeking         bool
	peekContent     string
	peekSessionName string
	peekViewport    viewport.Model

	// Crew wizard state
	creatingCrew bool
	crewWizard   *crewTUI.AddModel
	townRoot     string
	currentRig   string

	// Polling
	pollTicker *time.Ticker
	done       chan struct{}
}

// New creates a new decision TUI model
func New() *Model {
	ta := textarea.New()
	ta.Placeholder = "Enter rationale..."
	ta.SetHeight(3)
	ta.SetWidth(60)

	h := help.New()
	h.ShowAll = false

	return &Model{
		keys:           DefaultKeyMap(),
		help:           h,
		textInput:      ta,
		detailViewport: viewport.New(0, 0),
		peekViewport:   viewport.New(0, 0),
		filter:         "all",
		done:           make(chan struct{}),
	}
}

// SetFilter sets the urgency filter
func (m *Model) SetFilter(filter string) {
	m.filter = filter
}

// SetWorkspace sets the workspace info for crew creation
func (m *Model) SetWorkspace(townRoot, currentRig string) {
	m.townRoot = townRoot
	m.currentRig = currentRig
}

// SetNotify enables desktop notifications
func (m *Model) SetNotify(notify bool) {
	m.notify = notify
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchDecisions(),
		m.startPolling(),
		tea.SetWindowTitle("GT Decision Watch"),
	)
}

// fetchDecisionsMsg is sent when decisions are fetched
type fetchDecisionsMsg struct {
	decisions []DecisionItem
	err       error
}

// tickMsg is sent on each poll interval
type tickMsg time.Time

// resolvedMsg is sent when a decision is resolved
type resolvedMsg struct {
	id  string
	err error
}

// dismissedMsg is sent when a decision is dismissed/canceled
type dismissedMsg struct {
	id  string
	err error
}

// peekMsg is sent when terminal content is captured
type peekMsg struct {
	sessionName string
	content     string
	err         error
}

// fetchDecisions fetches pending decisions from bd
func (m *Model) fetchDecisions() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "gt", "decision", "list", "--json")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			// Check if it's just "no decisions"
			if strings.Contains(stderr.String(), "No pending") ||
				strings.Contains(stdout.String(), "[]") {
				return fetchDecisionsMsg{decisions: []DecisionItem{}}
			}
			return fetchDecisionsMsg{err: fmt.Errorf("failed to fetch decisions: %w", err)}
		}

		var decisions []DecisionItem
		if err := json.Unmarshal(stdout.Bytes(), &decisions); err != nil {
			// Try parsing as a different format
			return fetchDecisionsMsg{decisions: []DecisionItem{}}
		}

		return fetchDecisionsMsg{decisions: decisions}
	}
}

// startPolling starts the poll ticker
func (m *Model) startPolling() tea.Cmd {
	return tea.Tick(pollInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// resolveDecision resolves a decision with the given option
func (m *Model) resolveDecision(decisionID string, choice int, rationale string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		args := []string{"decision", "resolve", decisionID, "--choice", fmt.Sprintf("%d", choice)}
		if rationale != "" {
			args = append(args, "--rationale", rationale)
		}

		cmd := exec.CommandContext(ctx, "gt", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return resolvedMsg{id: decisionID, err: fmt.Errorf("%s", stderr.String())}
		}

		return resolvedMsg{id: decisionID}
	}
}

// dismissDecision cancels/dismisses a decision
func (m *Model) dismissDecision(decisionID string, reason string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		args := []string{"decision", "cancel", decisionID}
		if reason != "" {
			args = append(args, "--reason", reason)
		}

		cmd := exec.CommandContext(ctx, "gt", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return dismissedMsg{id: decisionID, err: fmt.Errorf("%s", stderr.String())}
		}

		return dismissedMsg{id: decisionID}
	}
}

// getSessionName converts a RequestedBy path to a tmux session name
// e.g., "gastown/crew/decision_point" -> "gt-gastown-crew-decision_point"
func getSessionName(requestedBy string) (string, error) {
	if requestedBy == "" {
		return "", fmt.Errorf("no requestor specified")
	}

	// Handle special cases
	if requestedBy == "overseer" || requestedBy == "human" {
		return "", fmt.Errorf("cannot peek human session")
	}

	// Parse rig/type/name format
	parts := strings.Split(requestedBy, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid requestor format: %s", requestedBy)
	}

	// Construct session name: gt-<rig>-<type>-<name>
	// e.g., "gastown/crew/decision_point" -> "gt-gastown-crew-decision_point"
	return "gt-" + strings.ReplaceAll(requestedBy, "/", "-"), nil
}

// captureTerminal captures the content of an agent's terminal
func (m *Model) captureTerminal(sessionName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// First check if session exists
		checkCmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", sessionName)
		if err := checkCmd.Run(); err != nil {
			return peekMsg{sessionName: sessionName, err: fmt.Errorf("session '%s' not found", sessionName)}
		}

		// Capture pane content with scrollback
		cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", sessionName, "-p", "-S", "-100")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return peekMsg{sessionName: sessionName, err: fmt.Errorf("capture failed: %s", stderr.String())}
		}

		return peekMsg{sessionName: sessionName, content: stdout.String()}
	}
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.detailViewport.Width = msg.Width - 4
		m.detailViewport.Height = msg.Height/2 - 4
		m.peekViewport.Width = msg.Width - 4
		m.peekViewport.Height = msg.Height - 6
		m.textInput.SetWidth(msg.Width - 10)
		// Forward to crew wizard if active
		if m.crewWizard != nil {
			m.crewWizard.SetSize(msg.Width, msg.Height)
		}

	case tea.KeyMsg:
		// Handle crew wizard mode - delegate all input
		if m.creatingCrew && m.crewWizard != nil {
			updated, cmd := m.crewWizard.Update(msg)
			if wizard, ok := updated.(*crewTUI.AddModel); ok {
				m.crewWizard = wizard
				// Check if wizard completed or was cancelled
				if wizard.IsDone() {
					m.creatingCrew = false
					m.crewWizard = nil
					// Refresh decisions after crew creation
					return m, m.fetchDecisions()
				}
			}
			return m, cmd
		}

		// Handle peek mode - arrow keys scroll, other keys dismiss
		if m.peeking {
			switch {
			case key.Matches(msg, m.keys.Up):
				m.peekViewport.LineUp(1)
				return m, nil
			case key.Matches(msg, m.keys.Down):
				m.peekViewport.LineDown(1)
				return m, nil
			case key.Matches(msg, m.keys.PageUp):
				m.peekViewport.HalfViewUp()
				return m, nil
			case key.Matches(msg, m.keys.PageDown):
				m.peekViewport.HalfViewDown()
				return m, nil
			default:
				// Any other key dismisses peek
				m.peeking = false
				m.peekContent = ""
				m.peekSessionName = ""
				return m, nil
			}
		}

		// Handle input mode first
		if m.inputMode != ModeNormal {
			return m.handleInputMode(msg)
		}

		switch {
		case key.Matches(msg, m.keys.Quit), key.Matches(msg, m.keys.Cancel):
			close(m.done)
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp

		case key.Matches(msg, m.keys.Up):
			if m.selected > 0 {
				m.selected--
				m.selectedOption = 0
			}

		case key.Matches(msg, m.keys.Down):
			if m.selected < len(m.decisions)-1 {
				m.selected++
				m.selectedOption = 0
			}

		case key.Matches(msg, m.keys.Select1):
			m.selectedOption = 1

		case key.Matches(msg, m.keys.Select2):
			m.selectedOption = 2

		case key.Matches(msg, m.keys.Select3):
			m.selectedOption = 3

		case key.Matches(msg, m.keys.Select4):
			m.selectedOption = 4

		case key.Matches(msg, m.keys.Rationale):
			if m.selectedOption > 0 {
				m.inputMode = ModeRationale
				m.textInput.Focus()
				m.textInput.SetValue("")
				m.textInput.Placeholder = "Enter rationale (optional)..."
			}

		case key.Matches(msg, m.keys.Text):
			m.inputMode = ModeText
			m.textInput.Focus()
			m.textInput.SetValue("")
			m.textInput.Placeholder = "Enter custom response..."

		case key.Matches(msg, m.keys.Peek):
			if len(m.decisions) > 0 && m.selected < len(m.decisions) {
				d := m.decisions[m.selected]
				sessionName, err := getSessionName(d.RequestedBy)
				if err != nil {
					m.status = fmt.Sprintf("Cannot peek: %v", err)
				} else {
					m.status = fmt.Sprintf("Peeking at %s...", sessionName)
					cmds = append(cmds, m.captureTerminal(sessionName))
				}
			}

		case key.Matches(msg, m.keys.Confirm):
			if m.selectedOption > 0 && len(m.decisions) > 0 && m.selected < len(m.decisions) {
				d := m.decisions[m.selected]
				if m.selectedOption <= len(d.Options) {
					cmds = append(cmds, m.resolveDecision(d.ID, m.selectedOption, m.rationale))
					m.status = fmt.Sprintf("Resolving %s...", d.ID)
				}
			}

		case key.Matches(msg, m.keys.Refresh):
			cmds = append(cmds, m.fetchDecisions())
			m.status = "Refreshing..."

		case key.Matches(msg, m.keys.Dismiss):
			if len(m.decisions) > 0 && m.selected < len(m.decisions) {
				d := m.decisions[m.selected]
				cmds = append(cmds, m.dismissDecision(d.ID, "Dismissed via TUI"))
				m.status = fmt.Sprintf("Dismissing %s...", d.ID)
			}

		case key.Matches(msg, m.keys.FilterHigh):
			m.filter = "high"

		case key.Matches(msg, m.keys.FilterAll):
			m.filter = "all"

		case key.Matches(msg, m.keys.CreateCrew):
			if m.townRoot != "" {
				m.crewWizard = crewTUI.NewAddModel(m.townRoot, m.currentRig)
				m.crewWizard.SetSize(m.width, m.height) // Pass dimensions to wizard
				m.creatingCrew = true
				return m, m.crewWizard.Init()
			} else {
				m.status = "Cannot create crew: workspace not configured"
			}
		}

	case fetchDecisionsMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.decisions = m.filterDecisions(msg.decisions)
			if m.selected >= len(m.decisions) {
				m.selected = max(0, len(m.decisions)-1)
			}
			m.status = fmt.Sprintf("Updated: %d pending", len(m.decisions))
		}

	case tickMsg:
		cmds = append(cmds, m.fetchDecisions())
		cmds = append(cmds, m.startPolling())

	case resolvedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Resolved: %s", msg.id)
			m.selectedOption = 0
			m.rationale = ""
			cmds = append(cmds, m.fetchDecisions())
		}

	case dismissedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = fmt.Sprintf("Dismiss error: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("Dismissed: %s", msg.id)
			m.selectedOption = 0
			cmds = append(cmds, m.fetchDecisions())
		}

	case peekMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Peek failed: %v", msg.err)
		} else {
			m.peeking = true
			m.peekSessionName = msg.sessionName
			m.peekContent = msg.content
			// Set viewport dimensions for scrolling
			m.peekViewport.Width = m.width - 4
			m.peekViewport.Height = m.height - 6 // Leave room for header/footer
			m.peekViewport.SetContent(msg.content)
			m.peekViewport.GotoBottom()
			m.status = fmt.Sprintf("Peeking: %s (↑/↓ scroll, any other key to close)", msg.sessionName)
		}

	default:
		// Forward unknown messages to crew wizard if active
		// This handles the wizard's internal messages (rigsLoadedMsg, crewCreatedMsg, etc.)
		if m.creatingCrew && m.crewWizard != nil {
			updated, cmd := m.crewWizard.Update(msg)
			if wizard, ok := updated.(*crewTUI.AddModel); ok {
				m.crewWizard = wizard
				if wizard.IsDone() {
					m.creatingCrew = false
					m.crewWizard = nil
					return m, m.fetchDecisions()
				}
			}
			cmds = append(cmds, cmd)
		}
	}

	// Update viewport
	var cmd tea.Cmd
	m.detailViewport, cmd = m.detailViewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleInputMode handles key presses in input mode
func (m *Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.inputMode = ModeNormal
		m.textInput.Blur()
		return m, nil

	case tea.KeyEnter:
		if m.inputMode == ModeRationale {
			m.rationale = m.textInput.Value()
			m.inputMode = ModeNormal
			m.textInput.Blur()

			// Auto-confirm if we have an option selected
			if m.selectedOption > 0 && len(m.decisions) > 0 && m.selected < len(m.decisions) {
				d := m.decisions[m.selected]
				if m.selectedOption <= len(d.Options) {
					return m, m.resolveDecision(d.ID, m.selectedOption, m.rationale)
				}
			}
		} else if m.inputMode == ModeText {
			// Custom text iteration is not yet implemented
			// For now, show a status message explaining this
			m.status = "Custom text iteration not yet implemented. Use number keys (1-4) to select an option, or 'd' to dismiss."
			m.inputMode = ModeNormal
			m.textInput.Blur()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// filterDecisions filters and sorts decisions based on current filter
func (m *Model) filterDecisions(decisions []DecisionItem) []DecisionItem {
	var result []DecisionItem

	if m.filter == "all" {
		result = decisions
	} else {
		for _, d := range decisions {
			if d.Urgency == m.filter {
				result = append(result, d)
			}
		}
	}

	// Sort by urgency (high first) then by time (newest first)
	sort.Slice(result, func(i, j int) bool {
		urgencyOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		ui := urgencyOrder[result[i].Urgency]
		uj := urgencyOrder[result[j].Urgency]
		if ui != uj {
			return ui < uj
		}
		// Same urgency, sort by time (newest first)
		return result[i].RequestedAt.After(result[j].RequestedAt)
	})

	return result
}

// View renders the TUI
func (m *Model) View() string {
	return m.renderView()
}

// Using built-in max() from Go 1.21+
