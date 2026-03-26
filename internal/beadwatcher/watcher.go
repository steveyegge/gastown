// Package beadwatcher polls bead state and notifies the mayor via nudge when
// a bead transitions to blocked, failed, or becomes overdue (open longer than
// a configurable threshold).
//
// Deduplication: the watcher remembers the last-notified state for each bead
// and skips re-notification unless the state has changed since the last nudge.
// This prevents nudge storms for long-lived stuck beads.
package beadwatcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/session"
)

// DefaultOverdueThreshold is how long an open bead must be unchanged before
// it is considered overdue and triggers a mayor nudge.
const DefaultOverdueThreshold = 4 * time.Hour

// defaultSender is the sender label used in nudge messages when none is set.
const defaultSender = "bead-watcher"

// watchedStatus captures the last-notified alertable state for a single bead.
// If a bead clears this state and later re-enters it, the watcher will re-notify.
type watchedStatus struct {
	// notifiedFor is the status string that was last nudged ("blocked", "failed", "overdue").
	notifiedFor string
}

// BeadsLister is the interface used by Watcher to list beads.
// In production this wraps *beads.Beads; in tests a fake is injected.
type BeadsLister interface {
	List(opts beads.ListOptions) ([]*beads.Issue, error)
}

// WatcherConfig holds configuration for the Watcher.
type WatcherConfig struct {
	// PollInterval is how often the watcher checks bead states.
	// Default: 60s (set by callers; zero value falls back to 60s internally).
	PollInterval time.Duration

	// OverdueThreshold is how long an open bead must be unchanged before
	// it is considered overdue.  Default: DefaultOverdueThreshold (4h).
	OverdueThreshold time.Duration

	// Sender is the sender label attached to nudge messages.
	// Default: "bead-watcher".
	Sender string
}

// Watcher polls bead state and sends nudges to the mayor on notable transitions.
type Watcher struct {
	townRoot string
	bl       BeadsLister
	cfg      WatcherConfig

	// seen maps bead ID → last-notified alert category.
	seen map[string]watchedStatus
}

// NewWatcher constructs a Watcher with the given configuration.
// townRoot is the Gas Town root directory (needed for nudge queue paths).
// bl provides the bead listing interface (injectable for tests).
func NewWatcher(townRoot string, bl BeadsLister, cfg WatcherConfig) *Watcher {
	if cfg.OverdueThreshold == 0 {
		cfg.OverdueThreshold = DefaultOverdueThreshold
	}
	if cfg.Sender == "" {
		cfg.Sender = defaultSender
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 60 * time.Second
	}
	return &Watcher{
		townRoot: townRoot,
		bl:       bl,
		cfg:      cfg,
		seen:     make(map[string]watchedStatus),
	}
}

// alertCategory returns the alert category for a bead, or "" if none applies.
//
// Priority order: blocked > failed > overdue.
// Terminal beads (closed/tombstone) are ignored.
func (w *Watcher) alertCategory(issue *beads.Issue) string {
	status := beads.IssueStatus(issue.Status)

	// Ignore terminal states
	if status.IsTerminal() {
		return ""
	}

	switch status {
	case beads.StatusBlocked:
		return "blocked"
	}

	// Check for "failed" label as a proxy for failed status (common convention
	// in Gas Town: failed beads get a "failed" label since there is no built-in
	// failed status in beads).
	for _, lbl := range issue.Labels {
		if lbl == "failed" || lbl == "gt:failed" {
			return "failed"
		}
	}

	// Overdue: open/in-progress bead that has not been updated within the threshold.
	if status == beads.StatusOpen || status == beads.StatusInProgress {
		age := w.beadAge(issue)
		if age > w.cfg.OverdueThreshold {
			return "overdue"
		}
	}

	return ""
}

// beadAge returns how long ago the bead was created, using CreatedAt.
// Falls back to UpdatedAt if CreatedAt is unavailable.
func (w *Watcher) beadAge(issue *beads.Issue) time.Duration {
	ts := issue.CreatedAt
	if ts == "" {
		ts = issue.UpdatedAt
	}
	if ts == "" {
		return 0
	}

	// Try common layouts
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return time.Since(t)
		}
	}
	return 0
}

// tick performs a single poll cycle:
//  1. List all non-terminal beads.
//  2. For each, determine if it is in an alertable state.
//  3. If the state differs from what was last notified, enqueue a mayor nudge.
//  4. Clear the seen entry for any bead that has left its alertable state.
func (w *Watcher) tick() error {
	issues, err := w.bl.List(beads.ListOptions{Status: "all"})
	if err != nil {
		return fmt.Errorf("beadwatcher: listing beads: %w", err)
	}

	// Track which IDs are still present in this cycle.
	active := make(map[string]struct{})

	mayorSession := session.MayorSessionName()

	for _, issue := range issues {
		if issue == nil {
			continue
		}
		cat := w.alertCategory(issue)
		active[issue.ID] = struct{}{}

		prev, wasSeen := w.seen[issue.ID]

		if cat == "" {
			// No alert — clear previous seen entry if bead resolved.
			if wasSeen {
				delete(w.seen, issue.ID)
			}
			continue
		}

		// Skip if same category was already notified (deduplication).
		if wasSeen && prev.notifiedFor == cat {
			continue
		}

		// State changed or first notification — send nudge.
		msg := w.buildMessage(issue, cat)
		if enqErr := nudge.Enqueue(w.townRoot, mayorSession, nudge.QueuedNudge{
			Sender:   w.cfg.Sender,
			Message:  msg,
			Priority: nudge.PriorityUrgent,
			Kind:     "bead-alert",
		}); enqErr != nil {
			// Log but don't abort the whole tick on queue errors.
			fmt.Printf("beadwatcher: enqueue nudge for %s: %v\n", issue.ID, enqErr)
			continue
		}

		w.seen[issue.ID] = watchedStatus{notifiedFor: cat}
	}

	// Prune seen entries for beads that are no longer present (closed/deleted).
	for id := range w.seen {
		if _, ok := active[id]; !ok {
			delete(w.seen, id)
		}
	}

	return nil
}

// buildMessage constructs the nudge message text.
func (w *Watcher) buildMessage(issue *beads.Issue, category string) string {
	var sb strings.Builder
	switch category {
	case "blocked":
		fmt.Fprintf(&sb, "Bead %s (%s) is now blocked.", issue.ID, truncate(issue.Title, 60))
	case "failed":
		fmt.Fprintf(&sb, "Bead %s (%s) has failed.", issue.ID, truncate(issue.Title, 60))
	case "overdue":
		age := w.beadAge(issue)
		fmt.Fprintf(&sb, "Bead %s (%s) is overdue — open for %s (threshold: %s).",
			issue.ID,
			truncate(issue.Title, 60),
			formatDuration(age),
			formatDuration(w.cfg.OverdueThreshold),
		)
	default:
		fmt.Fprintf(&sb, "Bead %s alert: %s.", issue.ID, category)
	}
	return sb.String()
}

// Run starts the watcher loop, blocking until ctx is canceled.
// It ticks immediately on start and then on every PollInterval thereafter.
func (w *Watcher) Run(ctx context.Context) error {
	// First tick immediately.
	if err := w.tick(); err != nil {
		fmt.Printf("beadwatcher: tick error: %v\n", err)
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.tick(); err != nil {
				fmt.Printf("beadwatcher: tick error: %v\n", err)
			}
		}
	}
}

// truncate shortens a string to at most maxLen characters, appending "…" if
// the string was cut.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// formatDuration produces a human-readable duration like "4h30m" or "2h0m".
func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}
