package witness

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/townlog"
)

// LogEntry is a unified log entry from any source.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`  // "townlog" or "pane:<session>"
	Agent     string    `json:"agent"`   // e.g., "gastown/polecats/ace"
	Type      string    `json:"type"`    // event type or "output"
	Content   string    `json:"content"` // log line or pane output line
}

// LogQuery defines filters for log aggregation.
type LogQuery struct {
	Rig     *rig.Rig
	Polecat string        // filter to specific polecat name (empty = all)
	Since   time.Duration // time window (0 = no limit)
	Type    string        // event type filter (empty = all)
	Grep    string        // text search pattern (empty = no filter)
	Tail    int           // max entries to return (0 = unlimited)
	Live    bool          // include live pane captures
}

// AggregateResult holds the result of log aggregation.
type AggregateResult struct {
	Entries []LogEntry
	Errors  []string // non-fatal errors (e.g., pane capture failures)
}

// AggregateLogs collects and filters logs for a rig from all available sources.
func AggregateLogs(townRoot string, q LogQuery) *AggregateResult {
	result := &AggregateResult{}

	// Source 1: Town log events scoped to this rig
	townEntries, err := aggregateTownLog(townRoot, q)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("town log: %v", err))
	} else {
		result.Entries = append(result.Entries, townEntries...)
	}

	// Source 2: Live pane captures from active sessions
	if q.Live {
		paneEntries, errs := aggregatePaneOutput(q)
		result.Entries = append(result.Entries, paneEntries...)
		result.Errors = append(result.Errors, errs...)
	}

	// Apply grep filter
	if q.Grep != "" {
		result.Entries = grepFilter(result.Entries, q.Grep)
	}

	// Apply tail limit
	if q.Tail > 0 && len(result.Entries) > q.Tail {
		result.Entries = result.Entries[len(result.Entries)-q.Tail:]
	}

	return result
}

// aggregateTownLog reads town.log and filters to rig-scoped entries.
func aggregateTownLog(townRoot string, q LogQuery) ([]LogEntry, error) {
	events, err := townlog.ReadEvents(townRoot)
	if err != nil {
		return nil, err
	}

	rigPrefix := q.Rig.Name + "/"

	filter := townlog.Filter{}
	if q.Since > 0 {
		filter.Since = time.Now().Add(-q.Since)
	}
	if q.Type != "" {
		filter.Type = townlog.EventType(q.Type)
	}

	// Filter to rig agents
	if q.Polecat != "" {
		// Specific polecat: match both "rig/polecat" and "rig/polecats/polecat"
		filter.Agent = q.Rig.Name + "/"
	} else {
		filter.Agent = rigPrefix
	}

	events = townlog.FilterEvents(events, filter)

	// Further filter by specific polecat name if set
	if q.Polecat != "" {
		events = filterByPolecat(events, q.Rig.Name, q.Polecat)
	}

	var entries []LogEntry
	for _, e := range events {
		entries = append(entries, LogEntry{
			Timestamp: e.Timestamp,
			Source:    "townlog",
			Agent:     e.Agent,
			Type:      string(e.Type),
			Content:   formatTownlogContent(e),
		})
	}

	return entries, nil
}

// filterByPolecat filters events to those matching a specific polecat name.
func filterByPolecat(events []townlog.Event, rigName, polecat string) []townlog.Event {
	var filtered []townlog.Event
	for _, e := range events {
		// Match patterns: "rig/polecat", "rig/polecats/polecat"
		if e.Agent == rigName+"/"+polecat ||
			e.Agent == rigName+"/polecats/"+polecat ||
			strings.HasPrefix(e.Agent, rigName+"/"+polecat+"/") ||
			strings.HasPrefix(e.Agent, rigName+"/polecats/"+polecat+"/") {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// formatTownlogContent produces a human-readable content string from a town log event.
func formatTownlogContent(e townlog.Event) string {
	if e.Context != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Type, e.Agent, e.Context)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Agent)
}

// aggregatePaneOutput captures live tmux pane content from all rig sessions.
func aggregatePaneOutput(q LogQuery) ([]LogEntry, []string) {
	var entries []LogEntry
	var errs []string

	t := tmux.NewTmux()
	now := time.Now()

	// Collect session names for this rig
	sessions := rigSessionNames(q)

	for _, sess := range sessions {
		// Check if session exists
		running, _ := t.HasSession(sess.name)
		if !running {
			continue
		}

		output, err := t.CapturePane(sess.name, 100)
		if err != nil {
			errs = append(errs, fmt.Sprintf("pane %s: %v", sess.name, err))
			continue
		}

		lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			entries = append(entries, LogEntry{
				Timestamp: now,
				Source:    "pane:" + sess.name,
				Agent:     sess.agent,
				Type:      "output",
				Content:   line,
			})
		}
	}

	return entries, errs
}

type rigSession struct {
	name  string // tmux session name
	agent string // agent path (e.g., "gastown/polecats/ace")
}

// rigSessionNames returns the tmux session names for a rig's agents.
func rigSessionNames(q LogQuery) []rigSession {
	var sessions []rigSession
	prefix := session.PrefixFor(q.Rig.Name)

	// Witness session
	if q.Polecat == "" {
		sessions = append(sessions, rigSession{
			name:  session.WitnessSessionName(prefix),
			agent: q.Rig.Name + "/witness",
		})
	}

	// Polecat sessions
	polecats := q.Rig.Polecats
	for _, p := range polecats {
		if q.Polecat != "" && p != q.Polecat {
			continue
		}
		sessions = append(sessions, rigSession{
			name:  session.PolecatSessionName(prefix, p),
			agent: q.Rig.Name + "/polecats/" + p,
		})
	}

	return sessions
}

// grepFilter filters entries by a case-insensitive text pattern.
func grepFilter(entries []LogEntry, pattern string) []LogEntry {
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
	if err != nil {
		// Fall back to simple contains
		lower := strings.ToLower(pattern)
		var filtered []LogEntry
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Content), lower) ||
				strings.Contains(strings.ToLower(e.Agent), lower) {
				filtered = append(filtered, e)
			}
		}
		return filtered
	}

	var filtered []LogEntry
	for _, e := range entries {
		if re.MatchString(e.Content) || re.MatchString(e.Agent) {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
