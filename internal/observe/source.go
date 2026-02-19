package observe

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	feed "github.com/steveyegge/gastown/internal/tui/feed"
)

// maxScannerTokenSize is the maximum line length the scanner will accept.
// Matches the 1MB buffer used by GtEventsSource.loadRecentEvents.
const maxScannerTokenSize = 1024 * 1024

// Source tails a file and emits feed events for each new line.
// It implements the feed.EventSource interface.
type Source struct {
	sourceID  string
	cfg       *config.ObservabilitySourceConfig
	redactor  *Redactor
	file      *os.File
	events    chan feed.Event
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewSource creates a new observability Source that tails the configured file.
func NewSource(sourceID string, cfg *config.ObservabilitySourceConfig) (*Source, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	if cfg.Path == "" {
		return nil, fmt.Errorf("empty path for source %q", sourceID)
	}

	f, err := os.Open(cfg.Path) //nolint:gosec // G304: path is user-configured
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", cfg.Path, err)
	}

	// Seek to end — we only want new lines.
	if _, err := f.Seek(0, 2); err != nil {
		f.Close()
		return nil, fmt.Errorf("seeking to end of %s: %w", cfg.Path, err)
	}

	policy := cfg.RedactionPolicy
	if policy == "" {
		policy = RedactStandard
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Source{
		sourceID: sourceID,
		cfg:      cfg,
		redactor: NewRedactor(policy),
		file:     f,
		events:   make(chan feed.Event, 200),
		cancel:   cancel,
	}

	s.wg.Add(1)
	go s.tail(ctx)

	// Emit source_up event (best-effort).
	kind := cfg.SourceKind
	if kind == "" {
		kind = "log"
	}
	_ = events.LogAudit(events.TypeObserveSourceUp, "observe/"+sourceID,
		events.ObserveSourcePayload(sourceID, kind, cfg.Path))

	return s, nil
}

// Events returns the event channel.
func (s *Source) Events() <-chan feed.Event {
	return s.events
}

// Close stops the tail goroutine, waits for it to exit, then closes the file.
// Safe to call multiple times.
func (s *Source) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.cancel()
		s.wg.Wait() // wait for tail goroutine to exit before closing file

		kind := s.cfg.SourceKind
		if kind == "" {
			kind = "log"
		}
		_ = events.LogAudit(events.TypeObserveSourceDown, "observe/"+s.sourceID,
			events.ObserveSourcePayload(s.sourceID, kind, s.cfg.Path))

		err = s.file.Close()
	})
	return err
}

// tail polls the file for new lines using a 100ms ticker, following the
// GtEventsSource pattern from internal/tui/feed/events.go.
func (s *Source) tail(ctx context.Context) {
	defer s.wg.Done()
	defer close(s.events)

	scanner := bufio.NewScanner(s.file)
	scanner.Buffer(make([]byte, 0, maxScannerTokenSize), maxScannerTokenSize)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for scanner.Scan() {
				line := scanner.Text()
				if strings.TrimSpace(line) == "" {
					continue
				}
				event := s.parseLine(line)
				if event == nil {
					continue
				}
				select {
				case s.events <- *event:
				case <-ctx.Done():
					return
				}
			}
			// If the scanner encountered an error (e.g., token too long),
			// create a fresh scanner so tailing can continue.
			if scanner.Err() != nil {
				scanner = bufio.NewScanner(s.file)
				scanner.Buffer(make([]byte, 0, maxScannerTokenSize), maxScannerTokenSize)
			}
		}
	}
}

// severityRank maps severity strings to numeric ranks for threshold comparison.
var severityRank = map[string]int{
	"debug": 0,
	"info":  1,
	"warn":  2,
	"error": 3,
}

// parseLine converts a raw log line into a feed Event.
func (s *Source) parseLine(line string) *feed.Event {
	severity := inferSeverity(line)
	if !s.passesSeverityFilter(severity) {
		return nil
	}

	redacted := s.redactor.Redact(line)

	serviceID := s.cfg.ServiceID
	actor := "observe/" + s.sourceID
	if serviceID != "" {
		actor = serviceID + "/" + s.sourceID
	}

	return &feed.Event{
		Time:    time.Now(),
		Type:    events.TypeObserveLog,
		Actor:   actor,
		Message: redacted,
		Rig:     serviceID,
		Role:    "observe",
		Raw:     redacted, // use redacted content to prevent PII leaks via Raw field
	}
}

// passesSeverityFilter returns true if the detected severity meets the
// configured threshold. Unknown severities (empty string) always pass.
func (s *Source) passesSeverityFilter(severity string) bool {
	if s.cfg.RoutingRules == nil || s.cfg.RoutingRules.SeverityThreshold == "" {
		return true
	}
	threshold, ok := severityRank[strings.ToLower(s.cfg.RoutingRules.SeverityThreshold)]
	if !ok {
		return true
	}
	if severity == "" {
		return true // unknown severity always passes
	}
	rank, ok := severityRank[strings.ToLower(severity)]
	if !ok {
		return true // unrecognized severity passes
	}
	return rank >= threshold
}

// inferSeverity tries to detect a severity level from a log line.
// Returns an empty string if no severity can be inferred, which causes
// the line to pass through any severity filter.
func inferSeverity(line string) string {
	upper := strings.ToUpper(line)
	switch {
	case strings.Contains(upper, "ERROR") || strings.Contains(upper, "ERR "):
		return "error"
	case strings.Contains(upper, "WARN") || strings.Contains(upper, "WARNING"):
		return "warn"
	case strings.Contains(upper, "DEBUG"):
		return "debug"
	case strings.Contains(upper, "INFO"):
		return "info"
	default:
		return "" // unknown — let it pass through severity filters
	}
}
