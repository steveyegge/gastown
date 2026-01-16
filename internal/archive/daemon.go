package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Daemon runs the archiver as a background service.
// It periodically captures tmux pane output and processes it through
// the diff algorithms to maintain an efficient transcript.
type Daemon struct {
	config   Config
	state    *ArchiveState
	session  string
	capturer *Capturer
	journal  *Journal

	// prevScreen holds the previous capture for comparison
	prevScreen []string
}

// NewDaemon creates a new archiver daemon for the given session.
func NewDaemon(session string, opts ...Option) *Daemon {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Daemon{
		config:   cfg,
		session:  session,
		capturer: NewCapturer(),
	}
}

// Run starts the daemon's main loop, capturing and processing tmux output.
// It blocks until the context is cancelled, then performs graceful shutdown.
func (d *Daemon) Run(ctx context.Context) error {
	// Initialize storage and journal
	if err := d.initialize(); err != nil {
		return fmt.Errorf("initializing daemon: %w", err)
	}
	defer d.shutdown()

	ticker := time.NewTicker(d.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := d.tick(); err != nil {
				// Log error but continue - archival is best-effort
				fmt.Fprintf(os.Stderr, "archive daemon tick error: %v\n", err)
			}
		}
	}
}

// initialize sets up the journal and state for the daemon.
func (d *Daemon) initialize() error {
	// Ensure storage directory exists
	if err := os.MkdirAll(d.config.StoragePath, 0755); err != nil {
		return fmt.Errorf("creating storage directory: %w", err)
	}

	// Open journal file
	journalPath := d.journalPath()
	journal, err := OpenJournal(journalPath)
	if err != nil {
		return fmt.Errorf("opening journal: %w", err)
	}
	d.journal = journal

	// Create archive state
	d.state = NewArchiveState(journal, d.config.Height)

	return nil
}

// shutdown performs graceful cleanup when the daemon stops.
func (d *Daemon) shutdown() error {
	// Commit any remaining active content
	if d.state != nil {
		active := d.state.Active()
		if len(active) > 0 {
			if err := d.state.CommitLines(len(active)); err != nil {
				fmt.Fprintf(os.Stderr, "archive daemon shutdown commit error: %v\n", err)
			}
		}
	}

	// Close journal
	if d.journal != nil {
		if err := d.journal.Close(); err != nil {
			return fmt.Errorf("closing journal: %w", err)
		}
	}

	return nil
}

// tick performs a single capture and processing cycle.
func (d *Daemon) tick() error {
	// Capture current tmux pane content
	nextScreen, err := d.capturer.CaptureTmuxPane(d.session, d.config.Width, d.config.Height)
	if err != nil {
		return fmt.Errorf("capturing pane: %w", err)
	}

	// Process the capture
	if err := d.processCapture(nextScreen); err != nil {
		return fmt.Errorf("processing capture: %w", err)
	}

	// Update previous screen for next comparison
	d.prevScreen = nextScreen
	return nil
}

// processCapture analyzes the difference between previous and current captures
// and updates the archive state accordingly.
func (d *Daemon) processCapture(nextScreen []string) error {
	// Normalize screens for comparison
	normalizedNext := Normalize(nextScreen)

	// First capture - initialize active block
	if d.prevScreen == nil {
		return d.state.AppendLines(normalizedNext)
	}

	normalizedPrev := Normalize(d.prevScreen)

	// No change - nothing to do
	if slicesEqual(normalizedPrev, normalizedNext) {
		return nil
	}

	// Try scroll detection first (most common case)
	scrolled, newLines := DetectScrollKMP(normalizedPrev, normalizedNext, d.config.ScrollThreshold)
	if scrolled {
		// Scroll detected - commit old lines that scrolled off, append new ones
		if len(newLines) > 0 {
			// The lines that scrolled off need to be committed
			scrolledOff := len(normalizedPrev) - (len(normalizedNext) - len(newLines))
			if scrolledOff > 0 {
				if err := d.state.CommitLines(scrolledOff); err != nil {
					return fmt.Errorf("committing scrolled lines: %w", err)
				}
			}
			// Append the new lines
			return d.state.AppendLines(newLines)
		}
		return nil
	}

	// No scroll detected - try anchor-based diff for full redraw handling
	regions := DiffWithAnchors(normalizedPrev, normalizedNext)

	// Process diff regions
	for _, region := range regions {
		switch region.Type {
		case Inserted:
			// New lines inserted
			if region.NextStart < len(normalizedNext) {
				insertedLines := normalizedNext[region.NextStart:region.NextEnd]
				if err := d.state.AppendLines(insertedLines); err != nil {
					return fmt.Errorf("appending inserted lines: %w", err)
				}
			}
		case Modified:
			// Lines changed in place - replace in active block
			if region.NextStart < len(normalizedNext) {
				modifiedLines := normalizedNext[region.NextStart:region.NextEnd]
				// Map to active block position
				activeStart := region.PrevStart
				activeCount := region.PrevEnd - region.PrevStart
				if err := d.state.ReplaceLines(activeStart, activeCount, modifiedLines); err != nil {
					return fmt.Errorf("replacing modified lines: %w", err)
				}
			}
		case Deleted:
			// Lines removed - commit them if they're at the top
			if region.PrevStart == 0 {
				deleteCount := region.PrevEnd - region.PrevStart
				if err := d.state.CommitLines(deleteCount); err != nil {
					return fmt.Errorf("committing deleted lines: %w", err)
				}
			}
		case Unchanged:
			// No action needed for unchanged regions
		}
	}

	return nil
}

// journalPath returns the full path to the journal file for this session.
func (d *Daemon) journalPath() string {
	filename := fmt.Sprintf("%s.journal", d.session)
	return filepath.Join(d.config.StoragePath, filename)
}

// Session returns the tmux session name being archived.
func (d *Daemon) Session() string {
	return d.session
}

// State returns the archive state (for testing/inspection).
func (d *Daemon) State() *ArchiveState {
	return d.state
}
