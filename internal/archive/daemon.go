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

	// Rate limiting state for adaptive diff strategy
	lastMyersTime   time.Time // Last time Myers diff was run
	myersFailures   int       // Consecutive Myers failures (for backoff)
	kmpFailures     int       // Consecutive KMP failures (triggers Myers)
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
//
// Uses an adaptive strategy cascade:
// 1. Fast path: KMP overlap detection O(H) - handles 90%+ of cases
// 2. Medium path: Myers diff O(H·D) - rate limited for complex changes
// 3. Slow path: Full redraw - just replace active block
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
		d.kmpFailures = 0
		return nil
	}

	// === FAST PATH: KMP scroll detection O(H) ===
	scrolled, newLines := DetectScrollKMP(normalizedPrev, normalizedNext, d.config.ScrollThreshold)
	if scrolled {
		d.kmpFailures = 0
		return d.handleScroll(normalizedPrev, normalizedNext, newLines)
	}

	// KMP failed - track for adaptive behavior
	d.kmpFailures++

	// === MEDIUM PATH: Myers diff O(H·D) - rate limited ===
	if d.canRunMyers() {
		prevHashes := HashLines(normalizedPrev)
		nextHashes := HashLines(normalizedNext)

		edits := MyersDiff(prevHashes, nextHashes)
		if edits != nil {
			// Myers succeeded
			d.lastMyersTime = time.Now()
			d.myersFailures = 0
			return d.applyMyersEdits(normalizedPrev, normalizedNext, edits)
		}

		// Myers exceeded threshold - count as failure
		d.myersFailures++
	}

	// === SLOW PATH: Full redraw ===
	return d.handleFullRedraw(normalizedNext)
}

// canRunMyers checks if we're allowed to run Myers diff based on rate limiting.
func (d *Daemon) canRunMyers() bool {
	// Always allow if we haven't run recently
	elapsed := time.Since(d.lastMyersTime)
	baseLimit := d.config.MyersRateLimit

	// Apply exponential backoff on consecutive failures
	if d.myersFailures > 0 {
		backoff := baseLimit * time.Duration(1<<min(d.myersFailures, 4)) // Cap at 16x
		if elapsed < backoff {
			return false
		}
	}

	// Allow if enough time has passed
	if elapsed >= baseLimit {
		return true
	}

	// Allow if KMP is failing repeatedly (need to try something)
	if d.kmpFailures >= 3 {
		return true
	}

	return false
}

// handleScroll processes a detected scroll event.
func (d *Daemon) handleScroll(prev, next, newLines []string) error {
	if len(newLines) > 0 {
		// The lines that scrolled off need to be committed
		scrolledOff := len(prev) - (len(next) - len(newLines))
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

// applyMyersEdits applies a Myers diff result to the archive state.
func (d *Daemon) applyMyersEdits(prev, next []string, edits []DiffEdit) error {
	for _, edit := range edits {
		switch edit.Type {
		case EditInsert:
			// Insert new lines
			if edit.NextI >= 0 && edit.NextI+edit.Count <= len(next) {
				insertedLines := next[edit.NextI : edit.NextI+edit.Count]
				if err := d.state.AppendLines(insertedLines); err != nil {
					return fmt.Errorf("applying insert: %w", err)
				}
			}
		case EditDelete:
			// Delete (commit) old lines at the top
			if edit.PrevI == 0 {
				if err := d.state.CommitLines(edit.Count); err != nil {
					return fmt.Errorf("applying delete: %w", err)
				}
			}
		case EditEqual:
			// No action needed for equal regions
		}
	}
	return nil
}

// handleFullRedraw replaces the entire active block with new content.
// This is the fallback when diff algorithms fail or are too expensive.
func (d *Daemon) handleFullRedraw(next []string) error {
	// Commit everything in the active block
	active := d.state.Active()
	if len(active) > 0 {
		if err := d.state.CommitLines(len(active)); err != nil {
			return fmt.Errorf("committing for redraw: %w", err)
		}
	}

	// Append the new screen content
	return d.state.AppendLines(next)
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
