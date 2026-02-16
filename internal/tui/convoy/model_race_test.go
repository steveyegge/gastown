package convoy

import (
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestConvoysWriteConcurrentWithView verifies that updating m.convoys
// concurrently with View() does not trigger data races.
func TestConvoysWriteConcurrentWithView(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: simulate fetchConvoysMsg updates
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.mu.Lock()
			m.convoys = []ConvoyItem{
				{ID: "hq-abc", Title: "Test Convoy", Status: "open",
					Issues:   []IssueItem{{ID: "gt-xyz", Title: "Fix bug", Status: "open"}},
					Progress: "0/1", Expanded: true},
			}
			m.mu.Unlock()
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestToggleExpandConcurrentWithView verifies that toggling convoy expansion
// while View() renders does not race.
func TestToggleExpandConcurrentWithView(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.mu.Unlock()

	// Pre-populate convoys
	m.convoys = []ConvoyItem{
		{ID: "hq-abc", Title: "Convoy 1", Status: "open",
			Issues:   []IssueItem{{ID: "gt-1", Title: "Issue 1", Status: "open"}},
			Progress: "0/1", Expanded: false},
		{ID: "hq-def", Title: "Convoy 2", Status: "open",
			Issues:   []IssueItem{{ID: "gt-2", Title: "Issue 2", Status: "open"}},
			Progress: "0/1", Expanded: false},
	}

	var wg sync.WaitGroup

	// Writer goroutine: toggle expansion
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.mu.Lock()
			m.toggleExpandLocked()
			m.mu.Unlock()
		}
	}()

	// Reader goroutine: render
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestCursorToConvoyIndexLocked verifies correct cursor-to-convoy mapping.
func TestCursorToConvoyIndexLocked(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.convoys = []ConvoyItem{
		{ID: "hq-abc", Title: "C1", Status: "open",
			Issues: []IssueItem{
				{ID: "gt-1", Title: "I1", Status: "open"},
				{ID: "gt-2", Title: "I2", Status: "closed"},
			},
			Expanded: true},
		{ID: "hq-def", Title: "C2", Status: "open",
			Issues: []IssueItem{
				{ID: "gt-3", Title: "I3", Status: "open"},
			},
			Expanded: false},
	}

	tests := []struct {
		cursor    int
		wantConv  int
		wantIssue int
	}{
		{0, 0, -1},  // First convoy header
		{1, 0, 0},   // First issue of first convoy
		{2, 0, 1},   // Second issue of first convoy
		{3, 1, -1},  // Second convoy header (collapsed)
		{4, -1, -1}, // Beyond last item
	}

	for _, tc := range tests {
		m.cursor = tc.cursor
		m.mu.RLock()
		ci, ii := m.cursorToConvoyIndexLocked()
		m.mu.RUnlock()

		if ci != tc.wantConv || ii != tc.wantIssue {
			t.Errorf("cursor=%d: got (%d, %d), want (%d, %d)",
				tc.cursor, ci, ii, tc.wantConv, tc.wantIssue)
		}
	}
}

// TestMaxCursorLocked verifies correct max cursor calculation.
func TestMaxCursorLocked(t *testing.T) {
	m := New("/tmp/fake-beads")

	// Empty
	m.mu.RLock()
	if got := m.maxCursorLocked(); got != 0 {
		t.Errorf("empty: maxCursor = %d, want 0", got)
	}
	m.mu.RUnlock()

	// One convoy, collapsed
	m.convoys = []ConvoyItem{
		{ID: "hq-abc", Issues: []IssueItem{{ID: "gt-1"}}, Expanded: false},
	}
	m.mu.RLock()
	if got := m.maxCursorLocked(); got != 0 {
		t.Errorf("1 collapsed: maxCursor = %d, want 0", got)
	}
	m.mu.RUnlock()

	// One convoy, expanded with 2 issues
	m.convoys[0].Expanded = true
	m.convoys[0].Issues = append(m.convoys[0].Issues, IssueItem{ID: "gt-2"})
	m.mu.RLock()
	if got := m.maxCursorLocked(); got != 2 {
		t.Errorf("1 expanded w/2 issues: maxCursor = %d, want 2", got)
	}
	m.mu.RUnlock()
}

// TestViewConcurrentWithWindowResize verifies that View and WindowSizeMsg
// updates can run concurrently without data races on width/height/help.
func TestViewConcurrentWithWindowResize(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.convoys = []ConvoyItem{
		{ID: "hq-abc", Title: "Test", Status: "open",
			Issues: []IssueItem{{ID: "gt-1", Title: "Issue", Status: "open"}},
			Progress: "0/1", Expanded: true},
	}
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: send WindowSizeMsg via Update
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.Update(tea.WindowSizeMsg{Width: 80 + i, Height: 40 + i})
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestViewConcurrentWithCursorNavigation verifies that View and cursor
// key handlers can run concurrently without data races.
func TestViewConcurrentWithCursorNavigation(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.convoys = []ConvoyItem{
		{ID: "hq-abc", Title: "C1", Status: "open",
			Issues: []IssueItem{
				{ID: "gt-1", Title: "I1", Status: "open"},
				{ID: "gt-2", Title: "I2", Status: "open"},
			},
			Progress: "0/2", Expanded: true},
		{ID: "hq-def", Title: "C2", Status: "open",
			Issues: []IssueItem{{ID: "gt-3", Title: "I3", Status: "open"}},
			Progress: "0/1", Expanded: true},
	}
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: navigate up/down and toggle help
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.Update(tea.KeyMsg{Type: tea.KeyDown})
			m.Update(tea.KeyMsg{Type: tea.KeyUp})
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}

// TestViewConcurrentWithFetchConvoys verifies that View and fetchConvoysMsg
// via Update can run concurrently without data races.
func TestViewConcurrentWithFetchConvoys(t *testing.T) {
	m := New("/tmp/fake-beads")
	m.mu.Lock()
	m.width = 80
	m.height = 40
	m.mu.Unlock()

	var wg sync.WaitGroup

	// Writer goroutine: send fetchConvoysMsg via Update
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			m.Update(fetchConvoysMsg{
				convoys: []ConvoyItem{
					{ID: "hq-abc", Title: "Test", Status: "open",
						Issues:   []IssueItem{{ID: "gt-1", Title: "I1", Status: "open"}},
						Progress: "0/1", Expanded: true},
				},
			})
		}
	}()

	// Reader goroutine: call View() concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = m.View()
		}
	}()

	wg.Wait()
}
