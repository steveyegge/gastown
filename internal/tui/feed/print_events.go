package feed

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PrintOptions controls filtering and behavior for PrintGtEvents.
type PrintOptions struct {
	Limit    int
	Follow   bool
	Since    string          // duration string like "5m", "1h"
	Mol      string          // molecule/issue ID prefix filter
	Type     string          // event type filter
	Rig      string          // rig name filter (matches event's Rig field)
	Actor    string          // actor substring filter (case-insensitive)
	Contains string          // free-text filter across message/target/actor/raw (case-insensitive)
	Ctx      context.Context // optional: controls follow-mode lifecycle; nil uses signal.NotifyContext
}

// PrintGtEvents reads .events.jsonl and prints events to stdout.
// When opts.Follow is true, it tails the file for new events after printing
// the initial batch, polling every 200ms. Canceled via opts.Ctx or SIGINT.
func PrintGtEvents(townRoot string, opts PrintOptions) error {
	eventsPath := filepath.Join(townRoot, ".events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		return fmt.Errorf("no events file found at %s: %w", eventsPath, err)
	}
	defer file.Close()

	// Parse --since into a cutoff time
	var sinceTime time.Time
	if opts.Since != "" {
		dur, err := time.ParseDuration(opts.Since)
		if err != nil {
			return fmt.Errorf("invalid --since duration %q: %w", opts.Since, err)
		}
		sinceTime = time.Now().Add(-dur)
	}

	var events []Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if event := parseGtEventLine(line); event != nil {
			if matchesFilters(event, sinceTime, opts.Mol, opts.Type, opts.Rig, opts.Actor, opts.Contains) {
				events = append(events, *event)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading events: %w", err)
	}

	// Sort by time descending (most recent first)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.After(events[j].Time)
	})

	// Apply limit
	if opts.Limit > 0 && len(events) > opts.Limit {
		events = events[:opts.Limit]
	}

	// Reverse to show oldest first (chronological)
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	if len(events) == 0 && !opts.Follow {
		fmt.Println("No events found in .events.jsonl")
		return nil
	}

	for _, event := range events {
		printEvent(event)
	}

	if !opts.Follow {
		return nil
	}

	// Tail mode: poll for new lines using a fresh scanner each tick.
	// bufio.Scanner sets an internal 'done' flag after EOF and won't retry,
	// so we must create a new scanner each poll cycle while preserving the
	// file offset (os.File tracks position across scanner instances).
	ctx := opts.Ctx
	if ctx == nil {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s := bufio.NewScanner(file)
			s.Buffer(make([]byte, 1024*1024), 1024*1024)
			for s.Scan() {
				line := s.Text()
				if event := parseGtEventLine(line); event != nil {
					if matchesFilters(event, sinceTime, opts.Mol, opts.Type, opts.Rig, opts.Actor, opts.Contains) {
						printEvent(*event)
					}
				}
			}
		}
	}
}

func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// matchesFilters checks whether an event passes feed CLI filters.
func matchesFilters(event *Event, sinceTime time.Time, mol, eventType, rig, actor, contains string) bool {
	if !sinceTime.IsZero() && event.Time.Before(sinceTime) {
		return false
	}
	if mol != "" && !containsFold(event.Target, mol) && !containsFold(event.Message, mol) {
		return false
	}
	if eventType != "" && !strings.EqualFold(event.Type, eventType) {
		return false
	}
	if rig != "" && !strings.EqualFold(event.Rig, rig) {
		return false
	}
	if actor != "" && !containsFold(event.Actor, actor) {
		return false
	}
	if contains != "" {
		if !containsFold(event.Message, contains) &&
			!containsFold(event.Target, contains) &&
			!containsFold(event.Actor, contains) &&
			!containsFold(event.Raw, contains) {
			return false
		}
	}
	if eventType != "" && event.Type == "" {
		return false
	}
	return true
}

// printEvent formats and prints a single event line.
func printEvent(event Event) {
	symbol := typeSymbol(event.Type)
	ts := event.Time.Format("15:04:05")
	actor := event.Actor
	if actor == "" {
		actor = "system"
	}
	fmt.Printf("[%s] %s %-25s %s\n", ts, symbol, actor, event.Message)
}

func typeSymbol(eventType string) string {
	switch eventType {
	case "patrol_started":
		return "\U0001F989" // owl
	case "patrol_complete":
		return "\U0001F989" // owl
	case "polecat_nudged":
		return "\u26A1" // lightning
	case "sling":
		return "\U0001F3AF" // target
	case "handoff":
		return "\U0001F91D" // handshake
	case "done":
		return "\u2713" // checkmark
	case "merged":
		return "\u2713"
	case "merge_failed":
		return "\u2717" // x
	case "create":
		return "+"
	case "complete":
		return "\u2713"
	case "fail":
		return "\u2717"
	case "delete":
		return "\u2298" // circled minus
	default:
		return "\u2192" // arrow
	}
}
