// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// StartupNudgeConfig configures a startup nudge message.
type StartupNudgeConfig struct {
	// Recipient is the address of the agent being nudged.
	// Examples: "gastown/crew/gus", "deacon", "gastown/witness"
	Recipient string

	// Sender is the agent initiating the nudge.
	// Examples: "mayor", "deacon", "self" (for handoff)
	Sender string

	// Topic describes why the session was started.
	// Examples: "cold-start", "handoff", "assigned", or a mol-id
	Topic string

	// MolID is an optional molecule ID being worked.
	// If provided, appended to topic as "topic:mol-id"
	MolID string
}

// StartupNudge sends a formatted startup message to a Claude Code session.
// The message becomes the session title in Claude Code's /resume picker,
// enabling workers to find predecessor sessions.
//
// Format: [GAS TOWN] <recipient> <- <sender> • <timestamp> • <topic[:mol-id]>
//
// Examples:
//   - [GAS TOWN] gastown/crew/gus <- deacon • 2025-12-30T15:42 • assigned:gt-abc12
//   - [GAS TOWN] deacon <- mayor • 2025-12-30T08:00 • cold-start
//   - [GAS TOWN] gastown/witness <- self • 2025-12-30T14:00 • handoff
//
// The message content doesn't trigger GUPP - CLAUDE.md and hooks handle that.
// The metadata makes sessions identifiable in /resume.
func StartupNudge(t *tmux.Tmux, session string, cfg StartupNudgeConfig) error {
	message := FormatStartupNudge(cfg)
	return t.NudgeSession(session, message)
}

// FormatStartupNudge builds the formatted startup nudge message.
// Separated from StartupNudge for testing and reuse.
func FormatStartupNudge(cfg StartupNudgeConfig) string {
	// Use local time in compact format
	timestamp := time.Now().Format("2006-01-02T15:04")

	// Build topic string - append mol-id if provided
	topic := cfg.Topic
	if cfg.MolID != "" && cfg.Topic != "" {
		topic = fmt.Sprintf("%s:%s", cfg.Topic, cfg.MolID)
	} else if cfg.MolID != "" {
		topic = cfg.MolID
	} else if topic == "" {
		topic = "ready"
	}

	// Build the beacon: [GAS TOWN] recipient <- sender • timestamp • topic
	beacon := fmt.Sprintf("[GAS TOWN] %s <- %s • %s • %s",
		cfg.Recipient, cfg.Sender, timestamp, topic)

	// For handoff, add explicit instructions so the agent knows what to do
	// even if hooks haven't loaded CLAUDE.md yet
	if cfg.Topic == "handoff" {
		beacon += "\n\nCheck your hook and mail, then act on the hook if present:\n" +
			"1. `gt hook` - shows hooked work (if any)\n" +
			"2. `gt mail inbox` - check for messages\n" +
			"3. If work is hooked → execute it immediately\n" +
			"4. If nothing hooked → wait for instructions"
	}

	return beacon
}
