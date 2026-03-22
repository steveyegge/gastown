//go:build !windows

package telegram

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"syscall"
	"time"
)

// categoryMap maps user-facing category names to event types.
var categoryMap = map[string][]string{
	"stuck_agents":   {"mass_death", "session_death"},
	"escalations":    {"escalation_sent"},
	"merge_failures": {"merge_failed"},
}

// FeedLine represents a single event line from the .feed.jsonl file.
type FeedLine struct {
	Timestamp string                 `json:"ts"`
	Source    string                 `json:"source,omitempty"`
	Type      string                 `json:"type"`
	Actor     string                 `json:"actor"`
	Summary   string                 `json:"summary,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	Count     int                    `json:"count,omitempty"`
}

// CategoryFilter allows events that match any of the configured category names.
type CategoryFilter struct {
	allowedTypes map[string]bool
}

// NewCategoryFilter creates a CategoryFilter from a list of category names.
// Unknown category names are silently ignored.
func NewCategoryFilter(categories []string) *CategoryFilter {
	allowed := make(map[string]bool)
	for _, cat := range categories {
		if types, ok := categoryMap[cat]; ok {
			for _, t := range types {
				allowed[t] = true
			}
		}
	}
	return &CategoryFilter{allowedTypes: allowed}
}

// Matches returns true if the event type is allowed by this filter.
func (f *CategoryFilter) Matches(eventType string) bool {
	return f.allowedTypes[eventType]
}

// ParseFeedLine parses a single JSON line into a FeedLine.
// Returns an error if the line is empty or not valid JSON.
func ParseFeedLine(line string) (*FeedLine, error) {
	if line == "" {
		return nil, fmt.Errorf("empty line")
	}
	var fl FeedLine
	if err := json.Unmarshal([]byte(line), &fl); err != nil {
		return nil, err
	}
	return &fl, nil
}

// FormatNotification formats an event into a human-readable Telegram message.
func FormatNotification(eventType, actor string, payload map[string]interface{}) string {
	switch eventType {
	case "mass_death":
		count := 0
		window := ""
		if payload != nil {
			if v, ok := payload["count"]; ok {
				switch n := v.(type) {
				case float64:
					count = int(n)
				case int:
					count = n
				}
			}
			if v, ok := payload["window"]; ok {
				window, _ = v.(string)
			}
		}
		return fmt.Sprintf("[mass_death] %d agent(s) died in %s", count, window)
	case "session_death":
		session := ""
		reason := ""
		if payload != nil {
			if v, ok := payload["session"]; ok {
				session, _ = v.(string)
			}
			if v, ok := payload["reason"]; ok {
				reason, _ = v.(string)
			}
		}
		return fmt.Sprintf("[session_death] %s died (%s)", session, reason)
	case "escalation_sent":
		message := ""
		if payload != nil {
			if v, ok := payload["message"]; ok {
				message, _ = v.(string)
			}
		}
		return fmt.Sprintf("[escalation] %s: %s", actor, message)
	case "merge_failed":
		branch := ""
		if payload != nil {
			if v, ok := payload["branch"]; ok {
				branch, _ = v.(string)
			}
		}
		return fmt.Sprintf("[merge_failed] %s on %s", actor, branch)
	default:
		return fmt.Sprintf("[%s] %s", eventType, actor)
	}
}

// InodeChanged returns true if the file represented by b has a different inode
// than a, which indicates a file rotation (rename + recreate).
// It also returns true if either FileInfo is nil.
func InodeChanged(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return true
	}
	sa, ok := a.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	sb, ok := b.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	return sa.Ino != sb.Ino
}

// OutboundNotifier tails a feed file and sends matching events to Telegram.
type OutboundNotifier struct {
	feedPath   string
	filter     *CategoryFilter
	bot        BotSender
	msgMap     *MessageMap
	pollInterval time.Duration
}

// NewOutboundNotifier creates an OutboundNotifier that watches feedPath and
// sends events matching categories to Telegram via bot.
func NewOutboundNotifier(feedPath string, categories []string, bot BotSender, msgMap *MessageMap) *OutboundNotifier {
	return &OutboundNotifier{
		feedPath:     feedPath,
		filter:       NewCategoryFilter(categories),
		bot:          bot,
		msgMap:       msgMap,
		pollInterval: 2 * time.Second,
	}
}

// Run blocks, tailing the feed file and dispatching matching events to Telegram.
// It handles log rotation by detecting inode changes.
// It returns when ctx is cancelled.
func (o *OutboundNotifier) Run(ctx context.Context) {
	var (
		f        *os.File
		reader   *bufio.Reader
		lastInfo os.FileInfo
		offset   int64
	)

	openFile := func() {
		if f != nil {
			f.Close() //nolint:errcheck
		}
		var err error
		f, err = os.Open(o.feedPath) //nolint:gosec
		if err != nil {
			f = nil
			reader = nil
			lastInfo = nil
			return
		}
		// Seek to end so we only process new lines.
		offset, _ = f.Seek(0, io.SeekEnd)
		_ = offset
		info, _ := f.Stat()
		lastInfo = info
		reader = bufio.NewReader(f)
	}

	openFile()

	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if f != nil {
				f.Close() //nolint:errcheck
			}
			return
		case <-ticker.C:
			// Check for rotation.
			if curInfo, err := os.Stat(o.feedPath); err == nil {
				if InodeChanged(lastInfo, curInfo) {
					openFile()
				}
			} else if f == nil {
				// File not yet available, try again.
				openFile()
				continue
			}

			if reader == nil {
				continue
			}

			// Read all available lines.
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					// Trim trailing newline.
					trimmed := line
					for len(trimmed) > 0 && (trimmed[len(trimmed)-1] == '\n' || trimmed[len(trimmed)-1] == '\r') {
						trimmed = trimmed[:len(trimmed)-1]
					}
					o.processLine(trimmed)
				}
				if err != nil {
					break
				}
			}
		}
	}
}

// processLine parses and dispatches a single feed line if it matches the filter.
func (o *OutboundNotifier) processLine(line string) {
	fl, err := ParseFeedLine(line)
	if err != nil {
		return
	}
	if !o.filter.Matches(fl.Type) {
		return
	}
	text := FormatNotification(fl.Type, fl.Actor, fl.Payload)
	if _, err := o.bot.SendMessage(text, nil); err != nil {
		log.Printf("outbound: SendMessage: %v", err)
	}
}
