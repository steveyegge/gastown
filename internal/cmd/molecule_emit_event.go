package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/workspace"
)

// emitEventSeq is an atomic counter to ensure unique event filenames even when
// time.Now().UnixNano() has low resolution (e.g., Windows ~100ns granularity).
var emitEventSeq atomic.Uint64

var (
	emitEventChannel string
	emitEventType    string
	emitEventPayload []string
)

var moleculeEmitEventCmd = &cobra.Command{
	Use:   "emit-event",
	Short: "Emit a file-based event on a named channel",
	Long: `Emit an event file to ~/gt/events/<channel>/ for subscribers to pick up.

This is the Go counterpart to emit-event.sh. Events are JSON files consumed
by await-event subscribers (e.g., the refinery watching for MERGE_READY events).

EVENT FORMAT:
Creates a JSON file at ~/gt/events/<channel>/<timestamp>.event:
  {"type": "...", "channel": "...", "timestamp": "...", "payload": {...}}

EXAMPLES:
  # Emit a MERGE_READY event for the refinery
  gt mol step emit-event --channel refinery --type MERGE_READY \
    --payload polecat=nux --payload branch=polecat/nux/gt-iw7m

  # Emit a PATROL_WAKE event
  gt mol step emit-event --channel refinery --type PATROL_WAKE \
    --payload source=witness --payload queue_depth=3

  # Emit an MQ_SUBMIT event
  gt mol step emit-event --channel refinery --type MQ_SUBMIT \
    --payload branch=feat/new-feature --payload mr_id=bd-42`,
	RunE: runMoleculeEmitEvent,
}

// EmitEventResult is returned when an event is emitted.
type EmitEventResult struct {
	Path    string `json:"path"`
	Channel string `json:"channel"`
	Type    string `json:"type"`
}

func init() {
	moleculeEmitEventCmd.Flags().StringVar(&emitEventChannel, "channel", "",
		"Event channel name (required, e.g., 'refinery')")
	moleculeEmitEventCmd.Flags().StringVar(&emitEventType, "type", "",
		"Event type (required, e.g., 'MERGE_READY')")
	moleculeEmitEventCmd.Flags().StringArrayVar(&emitEventPayload, "payload", nil,
		"Payload key=value pairs (repeatable)")
	moleculeEmitEventCmd.Flags().BoolVar(&moleculeJSON, "json", false,
		"Output as JSON")
	_ = moleculeEmitEventCmd.MarkFlagRequired("channel")
	_ = moleculeEmitEventCmd.MarkFlagRequired("type")

	moleculeStepCmd.AddCommand(moleculeEmitEventCmd)
}

func runMoleculeEmitEvent(cmd *cobra.Command, args []string) error {
	path, err := EmitEvent(emitEventChannel, emitEventType, emitEventPayload)
	if err != nil {
		return err
	}

	if moleculeJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(EmitEventResult{
			Path:    path,
			Channel: emitEventChannel,
			Type:    emitEventType,
		})
	}

	fmt.Println(path)
	return nil
}

// EmitEvent creates an event file in the channel directory.
// This is the programmatic API used by both the CLI command and internal callers
// (e.g., nudgeRefinery). Returns the path to the created event file.
func EmitEvent(channel, eventType string, payloadPairs []string) (string, error) {
	// Validate channel name before any filesystem operations (defense-in-depth)
	if !validChannelName.MatchString(channel) {
		return "", fmt.Errorf("invalid channel name %q: must match [a-zA-Z0-9_-]", channel)
	}

	// Resolve event directory
	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		home, _ := os.UserHomeDir()
		townRoot = filepath.Join(home, "gt")
	}
	eventDir := filepath.Join(townRoot, "events", channel)
	if err := os.MkdirAll(eventDir, 0755); err != nil {
		return "", fmt.Errorf("creating event directory: %w", err)
	}

	return emitEventImpl(eventDir, channel, eventType, payloadPairs)
}

// EmitEventToTown creates an event file using an explicit town root.
// Used by internal callers that already know the town root (e.g., nudgeRefinery).
func EmitEventToTown(townRoot, channel, eventType string, payloadPairs []string) (string, error) {
	// Validate channel name before any filesystem operations (defense-in-depth)
	if !validChannelName.MatchString(channel) {
		return "", fmt.Errorf("invalid channel name %q: must match [a-zA-Z0-9_-]", channel)
	}

	eventDir := filepath.Join(townRoot, "events", channel)
	if err := os.MkdirAll(eventDir, 0755); err != nil {
		return "", fmt.Errorf("creating event directory: %w", err)
	}
	return emitEventImpl(eventDir, channel, eventType, payloadPairs)
}

// emitEventImpl writes an event file to the given directory.
func emitEventImpl(eventDir, channel, eventType string, payloadPairs []string) (string, error) {
	// Validate channel name (prevent path traversal)
	if !validChannelName.MatchString(channel) {
		return "", fmt.Errorf("invalid channel name %q: must match [a-zA-Z0-9_-]", channel)
	}

	// Build payload from key=value pairs
	payload := make(map[string]string)
	for _, pair := range payloadPairs {
		key, val, found := strings.Cut(pair, "=")
		if found {
			payload[key] = val
		}
	}

	// Build event JSON
	now := time.Now()
	event := map[string]interface{}{
		"type":      eventType,
		"channel":   channel,
		"timestamp": now.Format(time.RFC3339),
		"payload":   payload,
	}

	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling event: %w", err)
	}

	// Write event file with nanosecond timestamp + sequence + PID for uniqueness
	seq := emitEventSeq.Add(1)
	eventFile := filepath.Join(eventDir, fmt.Sprintf("%d-%d-%d.event", now.UnixNano(), seq, os.Getpid()))
	if err := os.WriteFile(eventFile, data, 0644); err != nil {
		return "", fmt.Errorf("writing event file: %w", err)
	}

	return eventFile, nil
}
