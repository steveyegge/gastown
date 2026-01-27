// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"time"
)

// BeaconConfig configures a startup beacon message.
// The beacon is injected into the CLI prompt to identify sessions in /resume picker.
type BeaconConfig struct {
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

// FormatStartupBeacon builds the formatted startup beacon message.
// The beacon is injected into the CLI prompt, making sessions identifiable
// in Claude Code's /resume picker for predecessor discovery.
//
// Format: [GAS TOWN] <recipient> <- <sender> • <timestamp> • <topic[:mol-id]>
//
// Examples:
//   - [GAS TOWN] gastown/crew/gus <- deacon • 2025-12-30T15:42 • assigned:gt-abc12
//   - [GAS TOWN] deacon <- daemon • 2025-12-30T08:00 • patrol
//   - [GAS TOWN] gastown/witness <- deacon • 2025-12-30T14:00 • patrol
func FormatStartupBeacon(cfg BeaconConfig) string {
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

	// For handoff, cold-start, and attach, add explicit instructions so the agent knows
	// what to do even if hooks haven't loaded CLAUDE.md yet
	if cfg.Topic == "handoff" || cfg.Topic == "cold-start" || cfg.Topic == "attach" {
		beacon += "\n\nCheck your hook and mail, then act on the hook if present:\n" +
			"1. `gt hook` - shows hooked work (if any)\n" +
			"2. `gt mail inbox` - check for messages\n" +
			"3. If work is hooked → execute it immediately\n" +
			"4. If nothing hooked → wait for instructions"
	}

	// For assigned, work is already on the hook - just tell them to run it
	// This prevents the "helpful assistant" exploration pattern (see PRIMING.md)
	if cfg.Topic == "assigned" {
		beacon += "\n\nWork is on your hook. Run `gt hook` now and begin immediately."
	}

	return beacon
}

// BuildStartupPrompt creates the CLI prompt for agent startup.
//
// GUPP (Gas Town Universal Propulsion Principle) implementation:
//   - Beacon identifies session for /resume predecessor discovery
//   - Instructions tell agent to start working immediately
//   - SessionStart hook runs `gt prime` which injects full context including
//     "AUTONOMOUS WORK MODE" instructions when work is hooked
//
// This replaces the old two-step StartupNudge + PropulsionNudge pattern.
// The beacon is processed in Claude's first turn along with gt prime context,
// so no separate propulsion nudge is needed.
func BuildStartupPrompt(cfg BeaconConfig, instructions string) string {
	return FormatStartupBeacon(cfg) + "\n\n" + instructions
}
