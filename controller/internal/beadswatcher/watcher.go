// Package beadswatcher watches the beads activity stream for agent lifecycle
// events and emits them on a channel. The controller's main loop reads these
// events and translates them to K8s pod operations.
package beadswatcher

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"
)

// EventType identifies the kind of beads lifecycle event.
type EventType string

const (
	// AgentSpawn means a new agent needs a pod (crew created, polecat hooked).
	AgentSpawn EventType = "agent_spawn"

	// AgentDone means an agent completed its work (gt done, bead closed).
	AgentDone EventType = "agent_done"

	// AgentStuck means an agent is unresponsive (witness escalation).
	AgentStuck EventType = "agent_stuck"

	// AgentKill means an agent should be terminated (lifecycle shutdown).
	AgentKill EventType = "agent_kill"
)

// Event represents a beads lifecycle event that requires a pod operation.
type Event struct {
	Type      EventType
	Rig       string
	Role      string // polecat, crew, witness, refinery, mayor, deacon
	AgentName string
	BeadID    string            // The bead that triggered this event
	Metadata  map[string]string // Additional context from beads
}

// Watcher subscribes to BD Daemon lifecycle events and emits them on a channel.
type Watcher interface {
	// Start begins watching for beads events. Blocks until ctx is canceled.
	Start(ctx context.Context) error

	// Events returns a read-only channel of lifecycle events.
	Events() <-chan Event
}

// Config holds configuration for the ActivityWatcher.
type Config struct {
	// TownRoot is the Gas Town workspace root for bd commands.
	TownRoot string

	// BdBinary is the path to the bd executable (default: "bd").
	BdBinary string

	// Namespace is the default K8s namespace for pod metadata.
	Namespace string

	// DefaultImage is the default container image for agent pods.
	DefaultImage string

	// DaemonHost is the BD Daemon host for agent pod env vars.
	DaemonHost string

	// DaemonPort is the BD Daemon port for agent pod env vars.
	DaemonPort string
}

// bdActivityEvent represents a single NDJSON event from bd activity --follow --json.
type bdActivityEvent struct {
	Timestamp string                 `json:"timestamp"`
	Type      string                 `json:"type"`
	IssueID   string                 `json:"issue_id"`
	Symbol    string                 `json:"symbol"`
	Message   string                 `json:"message"`
	OldStatus string                 `json:"old_status,omitempty"`
	NewStatus string                 `json:"new_status,omitempty"`
	Actor     string                 `json:"actor,omitempty"`
	Source    string                 `json:"source,omitempty"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

// ActivityWatcher watches the beads activity stream (via bd activity --follow --json)
// for agent lifecycle events and emits them as typed Events. It reconnects with
// exponential backoff on stream errors.
type ActivityWatcher struct {
	cfg    Config
	events chan Event
	logger *slog.Logger
}

// NewActivityWatcher creates a watcher backed by the beads activity stream.
func NewActivityWatcher(cfg Config, logger *slog.Logger) *ActivityWatcher {
	if cfg.BdBinary == "" {
		cfg.BdBinary = "bd"
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "gastown"
	}
	return &ActivityWatcher{
		cfg:    cfg,
		events: make(chan Event, 64),
		logger: logger,
	}
}

// Start begins watching the beads activity stream. Blocks until ctx is canceled.
// Reconnects with exponential backoff on stream errors.
func (w *ActivityWatcher) Start(ctx context.Context) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			close(w.events)
			return fmt.Errorf("watcher stopped: %w", ctx.Err())
		default:
		}

		err := w.watchActivity(ctx)
		if err != nil {
			if ctx.Err() != nil {
				close(w.events)
				return fmt.Errorf("watcher stopped: %w", ctx.Err())
			}
			w.logger.Warn("activity stream error, reconnecting",
				"error", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				close(w.events)
				return fmt.Errorf("watcher stopped: %w", ctx.Err())
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			backoff = time.Second
		}
	}
}

// Events returns a read-only channel of lifecycle events.
func (w *ActivityWatcher) Events() <-chan Event {
	return w.events
}

// watchActivity runs bd activity --follow --town --json and processes events.
func (w *ActivityWatcher) watchActivity(ctx context.Context) error {
	args := []string{"activity", "--follow", "--town", "--json"}
	cmd := exec.CommandContext(ctx, w.cfg.BdBinary, args...)
	if w.cfg.TownRoot != "" {
		cmd.Dir = w.cfg.TownRoot
	}
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	w.logger.Info("starting beads activity stream",
		"binary", w.cfg.BdBinary, "dir", w.cfg.TownRoot)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting bd activity: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return nil
		default:
		}

		line := scanner.Text()
		if event, ok := w.parseLine(line); ok {
			w.logger.Info("emitting lifecycle event",
				"type", event.Type, "rig", event.Rig,
				"role", event.Role, "agent", event.AgentName,
				"bead", event.BeadID)
			select {
			case w.events <- event:
			case <-ctx.Done():
				_ = cmd.Process.Kill()
				return nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading bd activity: %w", err)
	}

	return cmd.Wait()
}

// parseLine parses a single NDJSON line and returns a lifecycle Event if relevant.
func (w *ActivityWatcher) parseLine(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Event{}, false
	}

	var raw bdActivityEvent
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		w.logger.Debug("skipping malformed activity line", "error", err)
		return Event{}, false
	}

	return w.mapEvent(raw)
}

// mapEvent maps a raw bd activity event to a beadswatcher Event.
// Returns the event and true if relevant, or false to skip.
func (w *ActivityWatcher) mapEvent(raw bdActivityEvent) (Event, bool) {
	switch raw.Type {
	case "sling", "hook", "spawn":
		return w.mapToEvent(AgentSpawn, raw)
	case "done", "unhook":
		return w.mapToEvent(AgentDone, raw)
	case "kill", "session_death":
		return w.mapToEvent(AgentKill, raw)
	case "escalation_sent", "polecat_nudged":
		return w.mapToEvent(AgentStuck, raw)
	case "status":
		return w.mapStatusChange(raw)
	default:
		return Event{}, false
	}
}

// mapToEvent creates a lifecycle Event of the given type from a raw event.
// Returns false if agent info cannot be extracted.
func (w *ActivityWatcher) mapToEvent(eventType EventType, raw bdActivityEvent) (Event, bool) {
	rig, role, name := extractAgentInfo(raw)
	if rig == "" || role == "" || name == "" {
		w.logger.Debug("skipping event with incomplete agent info",
			"type", raw.Type, "issue", raw.IssueID, "actor", raw.Actor)
		return Event{}, false
	}

	return Event{
		Type:      eventType,
		Rig:       rig,
		Role:      role,
		AgentName: name,
		BeadID:    raw.IssueID,
		Metadata:  w.buildMetadata(raw),
	}, true
}

// mapStatusChange maps beads status changes to lifecycle events.
// A close → AgentDone; open/in_progress with agent info → AgentSpawn.
func (w *ActivityWatcher) mapStatusChange(raw bdActivityEvent) (Event, bool) {
	switch raw.NewStatus {
	case "closed":
		return w.mapToEvent(AgentDone, raw)
	case "in_progress":
		// An issue moving to in_progress might mean an agent is being activated.
		return w.mapToEvent(AgentSpawn, raw)
	default:
		return Event{}, false
	}
}

// extractAgentInfo extracts rig, role, and agent name from an event.
// Tries actor field first ("rig/role/name"), then payload, then message.
func extractAgentInfo(raw bdActivityEvent) (rig, role, name string) {
	// Try actor field: "gastown/polecats/rictus" or "gastown/crew/k8s"
	if raw.Actor != "" {
		parts := strings.Split(raw.Actor, "/")
		if len(parts) >= 3 {
			return parts[0], normalizeRole(parts[1]), parts[2]
		}
	}

	// Try payload fields
	if raw.Payload != nil {
		if r, ok := raw.Payload["rig"].(string); ok {
			rig = r
		}
		if r, ok := raw.Payload["role"].(string); ok {
			role = normalizeRole(r)
		}
		if n, ok := raw.Payload["agent"].(string); ok {
			name = n
		}
		if n, ok := raw.Payload["agent_name"].(string); ok {
			name = n
		}
		// Try target field: "gastown/polecats/rictus"
		if target, ok := raw.Payload["target"].(string); ok && (rig == "" || role == "" || name == "") {
			parts := strings.Split(target, "/")
			if len(parts) >= 3 {
				if rig == "" {
					rig = parts[0]
				}
				if role == "" {
					role = normalizeRole(parts[1])
				}
				if name == "" {
					name = parts[2]
				}
			}
		}
	}

	return rig, role, name
}

// normalizeRole converts plural role names to the singular form used by podmanager.
func normalizeRole(role string) string {
	switch role {
	case "polecats":
		return "polecat"
	case "crews":
		return "crew"
	default:
		return role
	}
}

// buildMetadata creates the metadata map for a pod operation.
func (w *ActivityWatcher) buildMetadata(raw bdActivityEvent) map[string]string {
	meta := map[string]string{
		"namespace": w.cfg.Namespace,
	}
	if w.cfg.DefaultImage != "" {
		meta["image"] = w.cfg.DefaultImage
	}
	if w.cfg.DaemonHost != "" {
		meta["daemon_host"] = w.cfg.DaemonHost
	}
	if w.cfg.DaemonPort != "" {
		meta["daemon_port"] = w.cfg.DaemonPort
	}

	// Override with payload-specific values
	if raw.Payload != nil {
		if img, ok := raw.Payload["image"].(string); ok {
			meta["image"] = img
		}
		if ns, ok := raw.Payload["namespace"].(string); ok {
			meta["namespace"] = ns
		}
	}
	return meta
}

// StubWatcher is a no-op implementation for testing and scaffolding.
type StubWatcher struct {
	events chan Event
	logger *slog.Logger
}

// NewStubWatcher creates a watcher that emits no events.
func NewStubWatcher(logger *slog.Logger) *StubWatcher {
	return &StubWatcher{
		events: make(chan Event),
		logger: logger,
	}
}

// Start blocks until context is canceled. The stub emits no events.
func (w *StubWatcher) Start(ctx context.Context) error {
	w.logger.Info("beads watcher started (stub — no events will be emitted)")
	<-ctx.Done()
	close(w.events)
	return fmt.Errorf("watcher stopped: %w", ctx.Err())
}

// Events returns the event channel.
func (w *StubWatcher) Events() <-chan Event {
	return w.events
}
