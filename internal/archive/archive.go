// Package archive provides smart tmux log archiving for agent sessions.
//
// The archive package captures tmux pane output at regular intervals and
// stores it in a journal format that supports efficient replay and deduplication.
// This enables the Mayor to review session history without re-reading large
// scrollback buffers.
package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Archiver captures and archives tmux pane output for a session.
type Archiver struct {
	config  Config
	session string
	role    string

	mu       sync.Mutex
	journal  *Journal
	stopCh   chan struct{}
	doneCh   chan struct{}
	running  bool
}

// New creates a new Archiver for the given session.
func New(session, role string, opts ...Option) *Archiver {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Archiver{
		config:  cfg,
		session: session,
		role:    role,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start begins the archival process in a background goroutine.
// It periodically captures tmux pane output and writes to the journal.
func (a *Archiver) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return fmt.Errorf("archiver already running")
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(a.config.StoragePath, 0755); err != nil {
		return fmt.Errorf("creating storage directory: %w", err)
	}

	// Open or create journal file
	journalPath := a.journalPath()
	journal, err := OpenJournal(journalPath)
	if err != nil {
		return fmt.Errorf("opening journal: %w", err)
	}
	a.journal = journal
	a.running = true

	go a.run()
	return nil
}

// Stop halts the archival process and closes the journal.
func (a *Archiver) Stop() error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()

	close(a.stopCh)
	<-a.doneCh

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.journal != nil {
		if err := a.journal.Close(); err != nil {
			return fmt.Errorf("closing journal: %w", err)
		}
		a.journal = nil
	}
	a.running = false
	return nil
}

// Session returns the session name being archived.
func (a *Archiver) Session() string {
	return a.session
}

// Role returns the role of the session being archived.
func (a *Archiver) Role() string {
	return a.role
}

// JournalPath returns the path to the journal file.
func (a *Archiver) JournalPath() string {
	return a.journalPath()
}

// run is the main archival loop.
func (a *Archiver) run() {
	defer close(a.doneCh)

	ticker := time.NewTicker(a.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			if err := a.capture(); err != nil {
				// Log error but continue - archival is best-effort
				fmt.Fprintf(os.Stderr, "archive capture error: %v\n", err)
			}
		}
	}
}

// capture performs a single capture cycle.
func (a *Archiver) capture() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.journal == nil {
		return fmt.Errorf("journal not open")
	}

	// TODO: Implement tmux capture and diff algorithm
	// This will be implemented in gt-svy (diff algorithm) and gt-cor (tmux capture)
	return nil
}

// journalPath returns the full path to the journal file.
func (a *Archiver) journalPath() string {
	filename := fmt.Sprintf("%s.journal", a.session)
	return filepath.Join(a.config.StoragePath, filename)
}
