package feed

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PrintGtEvents reads .events.jsonl and prints recent events to stdout.
func PrintGtEvents(townRoot string, limit int) error {
	eventsPath := filepath.Join(townRoot, ".events.jsonl")
	file, err := os.Open(eventsPath)
	if err != nil {
		return fmt.Errorf("no events file found at %s: %w", eventsPath, err)
	}
	defer file.Close()

	var events []Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if event := parseGtEventLine(line); event != nil {
			events = append(events, *event)
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
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	// Reverse to show oldest first (chronological)
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	if len(events) == 0 {
		fmt.Println("No events found in .events.jsonl")
		return nil
	}

	for _, event := range events {
		symbol := typeSymbol(event.Type)
		ts := event.Time.Format("15:04:05")
		actor := event.Actor
		if actor == "" {
			actor = "system"
		}
		fmt.Printf("[%s] %s %-25s %s\n", ts, symbol, actor, event.Message)
	}

	return nil
}

// printGtEventLine formats a single GtEvent from raw JSON for plain output.
func printGtEventLine(line string) string {
	var ge GtEvent
	if err := json.Unmarshal([]byte(line), &ge); err != nil {
		return ""
	}

	t, err := time.Parse(time.RFC3339, ge.Timestamp)
	if err != nil {
		t = time.Now()
	}

	symbol := typeSymbol(ge.Type)
	message := buildEventMessage(ge.Type, ge.Payload)

	return fmt.Sprintf("[%s] %s %-25s %s", t.Format("15:04:05"), symbol, ge.Actor, message)
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
