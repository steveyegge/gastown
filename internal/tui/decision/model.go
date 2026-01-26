package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
		filter:         "all",
		done:           make(chan struct{}),
	}
}

// SetFilter sets the urgency filter
func (m *Model) SetFilter(filter string) {
	m.filter = filter
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

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.detailViewport.Width = msg.Width - 4
		m.detailViewport.Height = msg.Height/2 - 4
		m.textInput.SetWidth(msg.Width - 10)

	case tea.KeyMsg:
		// Handle input mode first
		if m.inputMode != ModeNormal {
			return m.handleInputMode(msg)
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
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

		case key.Matches(msg, m.keys.FilterHigh):
			m.filter = "high"

		case key.Matches(msg, m.keys.FilterAll):
			m.filter = "all"
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
			// Text mode would trigger iteration - for now just cancel
			m.inputMode = ModeNormal
			m.textInput.Blur()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// filterDecisions filters decisions based on current filter
func (m *Model) filterDecisions(decisions []DecisionItem) []DecisionItem {
	if m.filter == "all" {
		return decisions
	}

	var filtered []DecisionItem
	for _, d := range decisions {
		if d.Urgency == m.filter {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// View renders the TUI
func (m *Model) View() string {
	return m.renderView()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
