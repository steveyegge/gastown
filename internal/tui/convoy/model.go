package convoy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// convoyIDPattern validates convoy IDs.
var convoyIDPattern = regexp.MustCompile(`^hq-[a-zA-Z0-9-]+$`)

// subprocessTimeout is the timeout for bd subprocess calls.
const subprocessTimeout = 5 * time.Second

// IssueItem represents a tracked issue within a convoy.
type IssueItem struct {
	ID     string
	Title  string
	Status string
}

// ConvoyItem represents a convoy with its tracked issues.
type ConvoyItem struct {
	ID       string
	Title    string
	Status   string
	Issues   []IssueItem
	Progress string // e.g., "2/5"
	Expanded bool
}

// Model is the bubbletea model for the convoy TUI.
type Model struct {
	convoys   []ConvoyItem
	cursor    int    // Current selection index in flattened view
	townBeads string // Path to town beads directory
	err       error

	// UI state
	keys     KeyMap
	help     help.Model
	showHelp bool
	width    int
	height   int
}

// New creates a new convoy TUI model.
func New(townBeads string) Model {
	return Model{
		townBeads: townBeads,
		keys:      DefaultKeyMap(),
		help:      help.New(),
		convoys:   make([]ConvoyItem, 0),
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	return m.fetchConvoys
}

// fetchConvoysMsg is the result of fetching convoys.
type fetchConvoysMsg struct {
	convoys []ConvoyItem
	err     error
}

// fetchConvoys fetches convoy data from beads.
func (m Model) fetchConvoys() tea.Msg {
	convoys, err := loadConvoys(m.townBeads)
	return fetchConvoysMsg{convoys: convoys, err: err}
}

// loadConvoys loads convoy data from the beads directory.
func loadConvoys(townBeads string) ([]ConvoyItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), subprocessTimeout)
	defer cancel()

	// Get list of open convoys
	listArgs := []string{"list", "--type=convoy", "--json"}
	listCmd := exec.CommandContext(ctx, "bd", listArgs...)
	listCmd.Dir = townBeads
	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return nil, fmt.Errorf("listing convoys: %w", err)
	}

	var rawConvoys []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &rawConvoys); err != nil {
		return nil, fmt.Errorf("parsing convoy list: %w", err)
	}

	convoys := make([]ConvoyItem, 0, len(rawConvoys))
	for _, rc := range rawConvoys {
		issues, completed, total := loadTrackedIssues(townBeads, rc.ID)
		convoys = append(convoys, ConvoyItem{
			ID:       rc.ID,
			Title:    rc.Title,
			Status:   rc.Status,
			Issues:   issues,
			Progress: fmt.Sprintf("%d/%d", completed, total),
			Expanded: false,
		})
	}

	return convoys, nil
}

// loadTrackedIssues loads issues tracked by a convoy.
func loadTrackedIssues(townBeads, convoyID string) ([]IssueItem, int, int) {
	// Validate convoy ID for safety
	if !convoyIDPattern.MatchString(convoyID) {
		return nil, 0, 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), subprocessTimeout)
	defer cancel()

	// Query tracked issues using bd dep list (returns full issue details)
	cmd := exec.CommandContext(ctx, "bd", "dep", "list", convoyID, "-t", "tracks", "--json")
	cmd.Dir = townBeads
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, 0, 0
	}

	var tracked []struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &tracked); err != nil {
		return nil, 0, 0
	}

	issues := make([]IssueItem, 0, len(tracked))
	completed := 0
	for _, t := range tracked {
		issues = append(issues, IssueItem{
			ID:     t.ID,
			Title:  t.Title,
			Status: t.Status,
		})
		if t.Status == "closed" {
			completed++
		}
	}

	// Sort by status (open first, then closed)
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Status == issues[j].Status {
			return issues[i].ID < issues[j].ID
		}
		return issues[i].Status != "closed" // open comes first
	})

	return issues, completed, len(issues)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		return m, nil

	case fetchConvoysMsg:
		m.err = msg.err
		m.convoys = msg.convoys
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			max := m.maxCursor()
			if m.cursor < max {
				m.cursor++
			}
			return m, nil

		case key.Matches(msg, m.keys.Top):
			m.cursor = 0
			return m, nil

		case key.Matches(msg, m.keys.Bottom):
			m.cursor = m.maxCursor()
			return m, nil

		case key.Matches(msg, m.keys.Toggle):
			m.toggleExpand()
			return m, nil

		// Number keys for direct convoy access
		case msg.String() >= "1" && msg.String() <= "9":
			n := int(msg.String()[0] - '0')
			if n <= len(m.convoys) {
				m.jumpToConvoy(n - 1)
			}
			return m, nil
		}
	}

	return m, nil
}

// maxCursor returns the maximum valid cursor position.
func (m Model) maxCursor() int {
	count := 0
	for _, c := range m.convoys {
		count++ // convoy itself
		if c.Expanded {
			count += len(c.Issues)
		}
	}
	if count == 0 {
		return 0
	}
	return count - 1
}

// cursorToConvoyIndex returns the convoy index and issue index for the current cursor.
// Returns (convoyIdx, issueIdx) where issueIdx is -1 if on a convoy row.
func (m Model) cursorToConvoyIndex() (int, int) {
	pos := 0
	for ci, c := range m.convoys {
		if pos == m.cursor {
			return ci, -1
		}
		pos++
		if c.Expanded {
			for ii := range c.Issues {
				if pos == m.cursor {
					return ci, ii
				}
				pos++
			}
		}
	}
	return -1, -1
}

// toggleExpand toggles expansion of the convoy at the current cursor.
func (m *Model) toggleExpand() {
	ci, ii := m.cursorToConvoyIndex()
	if ci >= 0 && ii == -1 {
		// On a convoy row, toggle it
		m.convoys[ci].Expanded = !m.convoys[ci].Expanded
	}
}

// jumpToConvoy moves the cursor to a specific convoy by index.
func (m *Model) jumpToConvoy(convoyIdx int) {
	if convoyIdx < 0 || convoyIdx >= len(m.convoys) {
		return
	}
	pos := 0
	for ci, c := range m.convoys {
		if ci == convoyIdx {
			m.cursor = pos
			return
		}
		pos++
		if c.Expanded {
			pos += len(c.Issues)
		}
	}
}

// View renders the model.
func (m Model) View() string {
	return m.renderView()
}
