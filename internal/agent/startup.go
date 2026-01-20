// Package agent provides startup nudge formatting utilities.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// FormatStartupNudge builds a formatted startup nudge message.
// The message becomes the session title in Claude Code's /resume picker,
// enabling workers to find predecessor sessions.
//
// Format: [GAS TOWN] <recipient> <- <sender> • <timestamp> • <topic[:mol-id]>
//
// Examples:
//   - [GAS TOWN] gastown/crew/gus <- deacon • 2025-12-30T15:42 • assigned:gt-abc12
//   - [GAS TOWN] deacon <- mayor • 2025-12-30T08:00 • cold-start
//   - [GAS TOWN] gastown/witness <- self • 2025-12-30T14:00 • handoff
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

	// For assigned, work is already on the hook - just tell them to run it
	// This prevents the "helpful assistant" exploration pattern (see PRIMING.md)
	if cfg.Topic == "assigned" {
		beacon += "\n\nWork is on your hook. Run `gt hook` now and begin immediately."
	}

	// For start/restart/refresh, add fallback instructions in case SessionStart hook fails
	// to inject context via gt prime. This prevents the "No recent activity" state
	// where agents sit idle because they received only metadata, no instructions.
	// See: gt-uoc64 (crew workers starting without proper context injection)
	if cfg.Topic == "start" || cfg.Topic == "restart" || cfg.Topic == "refresh" {
		beacon += "\n\nRun `gt prime` now for full context, then check your hook and mail."
	}

	return beacon
}

// PropulsionNudge returns the basic GUPP nudge message.
func PropulsionNudge() string {
	return "Run `gt hook` to check your hook and begin work."
}

// PropulsionNudgeForRole generates a role-specific GUPP nudge.
// Different roles have different startup flows:
//   - polecat/crew: Check hook for slung work
//   - witness/refinery: Start patrol cycle
//   - deacon: Start heartbeat patrol
//   - mayor: Check mail for coordination work
//
// The workDir parameter is used to locate .runtime/session_id for including
// session ID in the message (for Claude Code /resume picker discovery).
func PropulsionNudgeForRole(role, workDir string) string {
	var msg string
	switch role {
	case "polecat", "crew":
		msg = PropulsionNudge()
	case "witness":
		msg = "Run `gt prime` to check patrol status and begin work."
	case "refinery":
		msg = "Run `gt prime` to check MQ status and begin patrol."
	case "deacon":
		msg = "Run `gt prime` to check patrol status and begin heartbeat cycle."
	case "mayor":
		msg = "Run `gt prime` to check mail and begin coordination."
	default:
		msg = PropulsionNudge()
	}

	// Append session ID if available (for /resume picker visibility)
	if sessionID := readSessionID(workDir); sessionID != "" {
		msg = fmt.Sprintf("%s [session:%s]", msg, sessionID)
	}
	return msg
}

// readSessionID reads the session ID from .runtime/session_id if it exists.
// Returns empty string if the file doesn't exist or can't be read.
func readSessionID(workDir string) string {
	if workDir == "" {
		return ""
	}
	path := filepath.Join(workDir, ".runtime", "session_id")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
