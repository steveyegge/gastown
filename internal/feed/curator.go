// Package feed provides the feed daemon that curates raw events into a user-facing feed.
//
// The curator:
// 1. Tails ~/gt/.events.jsonl (raw events)
// 2. Filters by visibility tag (drops audit-only events)
// 3. Deduplicates repeated updates (5 molecule updates → "agent active")
// 4. Aggregates related events (3 issues closed → "batch complete")
// 5. Writes curated events to ~/gt/.feed.jsonl
package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

// FeedFile is the name of the curated feed file.
const FeedFile = ".feed.jsonl"

// FeedEvent is the structure of events written to the feed.
type FeedEvent struct {
	Timestamp string                 `json:"ts"`
	Source    string                 `json:"source"`
	Type      string                 `json:"type"`
	Actor     string                 `json:"actor"`
	Summary   string                 `json:"summary"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	Count     int                    `json:"count,omitempty"` // For aggregated events
}

// Curator manages the feed curation process.
type Curator struct {
	townRoot string
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// Deduplication state
	mu          sync.Mutex
	recentDone  map[string]time.Time     // actor → last done time (dedupe repeated done events)
	recentSling map[string][]slingRecord // actor → recent slings (aggregate)
	recentMail  map[string]int           // actor → mail count in window (aggregate)
}

type slingRecord struct {
	target string
	ts     time.Time
}

// Deduplication/aggregation settings
const (
	// Dedupe window for repeated done events from same actor
	doneDedupeWindow = 10 * time.Second

	// Aggregation window for sling events
	slingAggregateWindow = 30 * time.Second

	// Mail aggregation window
	mailAggregateWindow = 30 * time.Second

	// Minimum events to trigger aggregation
	minAggregateCount = 3
)

// NewCurator creates a new feed curator.
func NewCurator(townRoot string) *Curator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Curator{
		townRoot:    townRoot,
		ctx:         ctx,
		cancel:      cancel,
		recentDone:  make(map[string]time.Time),
		recentSling: make(map[string][]slingRecord),
		recentMail:  make(map[string]int),
	}
}

// Start begins the curator goroutine.
func (c *Curator) Start() error {
	eventsPath := filepath.Join(c.townRoot, events.EventsFile)

	// Open events file, creating if needed
	file, err := os.OpenFile(eventsPath, os.O_RDONLY|os.O_CREATE, 0644) //nolint:gosec // G302: events file is non-sensitive operational data
	if err != nil {
		return fmt.Errorf("opening events file: %w", err)
	}

	// Seek to end to only process new events
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close() //nolint:gosec // G104: best effort cleanup on error
		return fmt.Errorf("seeking to end: %w", err)
	}

	c.wg.Add(1)
	go c.run(file)

	return nil
}

// Stop gracefully stops the curator.
func (c *Curator) Stop() {
	c.cancel()
	c.wg.Wait()
}

// run is the main curator loop.
func (c *Curator) run(file *os.File) {
	defer c.wg.Done()
	defer file.Close()

	reader := bufio.NewReader(file)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// Cleanup ticker for stale aggregation state
	cleanupTicker := time.NewTicker(time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return

		case <-cleanupTicker.C:
			c.cleanupStaleState()

		case <-ticker.C:
			// Read available lines
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break // No more data available
				}
				c.processLine(line)
			}
		}
	}
}

// processLine processes a single line from the events file.
func (c *Curator) processLine(line string) {
	if line == "" || line == "\n" {
		return
	}

	var rawEvent events.Event
	if err := json.Unmarshal([]byte(line), &rawEvent); err != nil {
		return // Skip malformed lines
	}

	// Filter by visibility - only process feed-visible events
	if rawEvent.Visibility != events.VisibilityFeed && rawEvent.Visibility != events.VisibilityBoth {
		return
	}

	// Apply deduplication and aggregation
	if c.shouldDedupe(&rawEvent) {
		return
	}

	// Write to feed
	c.writeFeedEvent(&rawEvent)
}

// shouldDedupe checks if an event should be deduplicated.
// Returns true if the event should be dropped.
func (c *Curator) shouldDedupe(event *events.Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	switch event.Type {
	case events.TypeDone:
		// Dedupe repeated done events from same actor within window
		if lastDone, ok := c.recentDone[event.Actor]; ok {
			if now.Sub(lastDone) < doneDedupeWindow {
				return true // Skip duplicate
			}
		}
		c.recentDone[event.Actor] = now
		return false

	case events.TypeSling:
		// Track for potential aggregation (but don't dedupe single slings)
		target, _ := event.Payload["target"].(string)
		c.recentSling[event.Actor] = append(c.recentSling[event.Actor], slingRecord{
			target: target,
			ts:     now,
		})
		// Prune old records
		c.recentSling[event.Actor] = c.pruneRecords(c.recentSling[event.Actor], slingAggregateWindow)
		return false

	case events.TypeMail:
		// Track mail count for potential aggregation
		c.recentMail[event.Actor]++
		// Reset after window (rough approximation)
		go func(actor string) {
			time.Sleep(mailAggregateWindow)
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.recentMail[actor] > 0 {
				c.recentMail[actor]--
			}
		}(event.Actor)
		return false
	}

	return false
}

// pruneRecords removes records older than the window.
func (c *Curator) pruneRecords(records []slingRecord, window time.Duration) []slingRecord {
	now := time.Now()
	result := make([]slingRecord, 0, len(records))
	for _, r := range records {
		if now.Sub(r.ts) < window {
			result = append(result, r)
		}
	}
	return result
}

// cleanupStaleState removes old entries from tracking maps.
func (c *Curator) cleanupStaleState() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	staleThreshold := 5 * time.Minute

	// Clean stale done records
	for actor, ts := range c.recentDone {
		if now.Sub(ts) > staleThreshold {
			delete(c.recentDone, actor)
		}
	}

	// Clean stale sling records
	for actor, records := range c.recentSling {
		c.recentSling[actor] = c.pruneRecords(records, staleThreshold)
		if len(c.recentSling[actor]) == 0 {
			delete(c.recentSling, actor)
		}
	}

	// Reset mail counts
	c.recentMail = make(map[string]int)
}

// writeFeedEvent writes a curated event to the feed file.
func (c *Curator) writeFeedEvent(event *events.Event) {
	feedEvent := FeedEvent{
		Timestamp: event.Timestamp,
		Source:    event.Source,
		Type:      event.Type,
		Actor:     event.Actor,
		Summary:   c.generateSummary(event),
		Payload:   event.Payload,
	}

	// Check for aggregation opportunity
	c.mu.Lock()
	if event.Type == events.TypeSling {
		if records := c.recentSling[event.Actor]; len(records) >= minAggregateCount {
			feedEvent.Count = len(records)
			feedEvent.Summary = fmt.Sprintf("%s dispatching work to %d agents", event.Actor, len(records))
		}
	}
	c.mu.Unlock()

	data, err := json.Marshal(feedEvent)
	if err != nil {
		return
	}
	data = append(data, '\n')

	feedPath := filepath.Join(c.townRoot, FeedFile)
	f, err := os.OpenFile(feedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // G302: feed file is non-sensitive operational data
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(data)
}

// generateSummary creates a human-readable summary of an event.
func (c *Curator) generateSummary(event *events.Event) string {
	switch event.Type {
	case events.TypeSling:
		if target, ok := event.Payload["target"].(string); ok {
			if bead, ok := event.Payload["bead"].(string); ok {
				return fmt.Sprintf("%s assigned %s to %s", event.Actor, bead, target)
			}
		}
		return fmt.Sprintf("%s dispatched work", event.Actor)

	case events.TypeDone:
		if bead, ok := event.Payload["bead"].(string); ok {
			return fmt.Sprintf("%s completed work on %s", event.Actor, bead)
		}
		return fmt.Sprintf("%s signaled done", event.Actor)

	case events.TypeHandoff:
		return fmt.Sprintf("%s handed off to fresh session", event.Actor)

	case events.TypeMail:
		if to, ok := event.Payload["to"].(string); ok {
			if subj, ok := event.Payload["subject"].(string); ok {
				return fmt.Sprintf("%s → %s: %s", event.Actor, to, subj)
			}
		}
		return fmt.Sprintf("%s sent mail", event.Actor)

	case events.TypePatrolStarted:
		if rig, ok := event.Payload["rig"].(string); ok {
			return fmt.Sprintf("%s patrol started for %s", event.Actor, rig)
		}
		return fmt.Sprintf("%s started patrol", event.Actor)

	case events.TypePatrolComplete:
		if msg, ok := event.Payload["message"].(string); ok {
			return msg
		}
		return fmt.Sprintf("%s completed patrol", event.Actor)

	case events.TypeMerged:
		if worker, ok := event.Payload["worker"].(string); ok {
			return fmt.Sprintf("Merged work from %s", worker)
		}
		return "Work merged"

	case events.TypeMergeFailed:
		if reason, ok := event.Payload["reason"].(string); ok {
			return fmt.Sprintf("Merge failed: %s", reason)
		}
		return "Merge failed"

	default:
		return fmt.Sprintf("%s: %s", event.Actor, event.Type)
	}
}
