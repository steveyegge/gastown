// Package beadswatcher watches the beads daemon's SSE event stream for agent
// lifecycle events and emits them on a channel. The controller's main loop
// reads these events and translates them to K8s pod operations.
//
// This implementation connects to the daemon's HTTP /events endpoint via
// Server-Sent Events (SSE) for real-time mutation streaming. No bd binary or
// shell access is required — the controller is a pure Go binary in a
// distroless container.
package beadswatcher

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

	// AgentUpdate means agent bead metadata was changed (e.g., sidecar profile).
	AgentUpdate EventType = "agent_update"
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

// Config holds configuration for the SSEWatcher.
type Config struct {
	// DaemonHTTPURL is the daemon's HTTP base URL (e.g., "http://host:9080").
	DaemonHTTPURL string

	// DaemonToken is the Bearer auth token for the daemon HTTP API.
	DaemonToken string

	// Namespace is the default K8s namespace for pod metadata.
	Namespace string

	// DefaultImage is the default container image for agent pods.
	DefaultImage string

	// DaemonHost is the BD Daemon host for agent pod env vars.
	DaemonHost string

	// DaemonPort is the BD Daemon port for agent pod env vars.
	DaemonPort string
}

// mutationEvent mirrors the daemon's MutationEvent JSON structure.
// We define it here to avoid importing the beads module.
type mutationEvent struct {
	Type      string    `json:"Type"`
	IssueID   string    `json:"IssueID"`
	Title     string    `json:"Title,omitempty"`
	Assignee  string    `json:"Assignee,omitempty"`
	Actor     string    `json:"Actor,omitempty"`
	Timestamp time.Time `json:"Timestamp"`
	OldStatus string    `json:"old_status,omitempty"`
	NewStatus string    `json:"new_status,omitempty"`
	IssueType string    `json:"issue_type,omitempty"`
	Labels    []string  `json:"labels,omitempty"`
}

// SSEWatcher connects to the daemon's HTTP /events SSE endpoint and translates
// mutation events on agent beads into lifecycle Events. It reconnects with
// exponential backoff on stream errors.
type SSEWatcher struct {
	cfg    Config
	events chan Event
	logger *slog.Logger
}

// NewSSEWatcher creates a watcher backed by the daemon's SSE event stream.
func NewSSEWatcher(cfg Config, logger *slog.Logger) *SSEWatcher {
	return &SSEWatcher{
		cfg:    cfg,
		events: make(chan Event, 64),
		logger: logger,
	}
}

// Start begins watching the SSE stream. Blocks until ctx is canceled.
// Reconnects with exponential backoff on stream errors.
func (w *SSEWatcher) Start(ctx context.Context) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			close(w.events)
			return fmt.Errorf("watcher stopped: %w", ctx.Err())
		default:
		}

		err := w.streamSSE(ctx)
		if err != nil {
			if ctx.Err() != nil {
				close(w.events)
				return fmt.Errorf("watcher stopped: %w", ctx.Err())
			}
			w.logger.Warn("SSE stream error, reconnecting",
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
func (w *SSEWatcher) Events() <-chan Event {
	return w.events
}

// streamSSE connects to the daemon's /events endpoint and processes SSE frames.
func (w *SSEWatcher) streamSSE(ctx context.Context) error {
	url := fmt.Sprintf("%s/events", strings.TrimSuffix(w.cfg.DaemonHTTPURL, "/"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if w.cfg.DaemonToken != "" {
		req.Header.Set("Authorization", "Bearer "+w.cfg.DaemonToken)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}

	w.logger.Info("connecting to daemon SSE stream", "url", url)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE endpoint returned status %d", resp.StatusCode)
	}

	w.logger.Info("SSE stream connected")

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var currentData string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()

		if line == "" {
			// Empty line = SSE event boundary
			if currentData != "" {
				w.processSSEData(currentData)
				currentData = ""
			}
			continue
		}

		if strings.HasPrefix(line, "data: ") || strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data: ")
			data = strings.TrimPrefix(data, "data:")
			if currentData != "" {
				currentData += "\n" + data
			} else {
				currentData = data
			}
		}
		// Ignore id:, event:, and comment lines — we only need the data payload.
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("SSE stream read error: %w", err)
	}

	return fmt.Errorf("SSE stream closed by server")
}

// processSSEData parses a mutation event JSON and emits a lifecycle Event if relevant.
func (w *SSEWatcher) processSSEData(data string) {
	var raw mutationEvent
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		w.logger.Debug("skipping malformed SSE data", "error", err)
		return
	}

	// Only process events on agent beads.
	if !isAgentBead(raw) {
		return
	}

	event, ok := w.mapMutation(raw)
	if !ok {
		return
	}

	w.logger.Info("emitting lifecycle event",
		"type", event.Type, "rig", event.Rig,
		"role", event.Role, "agent", event.AgentName,
		"bead", event.BeadID)

	select {
	case w.events <- event:
	default:
		w.logger.Warn("event channel full, dropping event",
			"type", event.Type, "bead", event.BeadID)
	}
}

// isAgentBead checks if a mutation event is for an agent bead.
func isAgentBead(raw mutationEvent) bool {
	if raw.IssueType == "agent" {
		return true
	}
	for _, label := range raw.Labels {
		if label == "gt:agent" {
			return true
		}
	}
	return false
}

// mapMutation maps a daemon MutationEvent to a beadswatcher Event.
func (w *SSEWatcher) mapMutation(raw mutationEvent) (Event, bool) {
	switch raw.Type {
	case "create":
		// New agent bead created → spawn
		return w.buildEvent(AgentSpawn, raw)
	case "status":
		return w.mapStatusChange(raw)
	case "delete":
		return w.buildEvent(AgentKill, raw)
	case "update":
		// Metadata change on agent bead → may need pod update (e.g., sidecar change).
		return w.buildEvent(AgentUpdate, raw)
	default:
		// "comment", "bonded", etc. — not lifecycle events
		return Event{}, false
	}
}

// mapStatusChange maps beads status transitions to lifecycle events.
func (w *SSEWatcher) mapStatusChange(raw mutationEvent) (Event, bool) {
	switch raw.NewStatus {
	case "closed":
		return w.buildEvent(AgentDone, raw)
	case "in_progress":
		// Agent moving to in_progress may mean (re)activation.
		return w.buildEvent(AgentSpawn, raw)
	default:
		return Event{}, false
	}
}

// buildEvent constructs a lifecycle Event from a mutation event.
func (w *SSEWatcher) buildEvent(eventType EventType, raw mutationEvent) (Event, bool) {
	rig, role, name := extractAgentInfo(raw)
	if role == "" || name == "" {
		w.logger.Debug("skipping event with incomplete agent info",
			"mutation_type", raw.Type, "issue", raw.IssueID, "actor", raw.Actor)
		return Event{}, false
	}

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

	return Event{
		Type:      eventType,
		Rig:       rig,
		Role:      role,
		AgentName: name,
		BeadID:    raw.IssueID,
		Metadata:  meta,
	}, true
}

// extractAgentInfo extracts rig, role, and agent name from a mutation event.
//
// Strategy (in priority order):
//  1. Labels: "rig:X", "role:Y", "agent:Z" — most reliable for structured IDs
//  2. Actor field: "gastown/polecats/rictus" or "gastown/mayor/hq"
//  3. IssueID: e.g., "hq-mayor", "gastown-polecats-toast"
func extractAgentInfo(raw mutationEvent) (rig, role, name string) {
	// Try labels first — these are explicit and unambiguous.
	rig, role, name = extractFromLabels(raw.Labels)
	if rig != "" && role != "" && name != "" {
		return rig, normalizeRole(role), name
	}

	// Try actor field: "gastown/polecats/rictus" or "gastown/crew/k8s"
	if raw.Actor != "" {
		parts := strings.Split(raw.Actor, "/")
		if len(parts) >= 3 {
			return parts[0], normalizeRole(parts[1]), parts[2]
		}
	}

	// Fall back to parsing the IssueID.
	// Agent bead IDs follow patterns:
	//   "hq-mayor"           → rig=town, role=mayor, name=hq
	//   "hq-deacon"          → rig=town, role=deacon, name=hq
	//   "{rig}-{role}-{name}" → e.g., "gastown-polecats-toast"
	return parseAgentBeadID(raw.IssueID)
}

// extractFromLabels extracts rig, role, and agent name from bead labels.
func extractFromLabels(labels []string) (rig, role, name string) {
	for _, label := range labels {
		parts := strings.SplitN(label, ":", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "rig":
			rig = parts[1]
		case "role":
			role = parts[1]
		case "agent":
			name = parts[1]
		}
	}
	return rig, role, name
}

// parseAgentBeadID parses an agent bead ID into rig, role, and name.
// Known patterns:
//   - "hq-mayor"              → town, mayor, hq
//   - "hq-deacon"             → town, deacon, hq
//   - "{rig}-witness-{rig}"   → {rig}, witness, {rig}
//   - "{rig}-refinery-{rig}"  → {rig}, refinery, {rig}
//   - "{rig}-polecats-{name}" → {rig}, polecat, {name}
//   - "{rig}-crew-{name}"     → {rig}, crew, {name}
func parseAgentBeadID(id string) (rig, role, name string) {
	// Town-level singletons
	switch {
	case id == "hq-mayor":
		return "town", "mayor", "hq"
	case id == "hq-deacon":
		return "town", "deacon", "hq"
	}

	// Rig-level agents: "{rig}-{role}-{name}"
	parts := strings.SplitN(id, "-", 3)
	if len(parts) == 3 {
		return parts[0], normalizeRole(parts[1]), parts[2]
	}

	// Can't parse — return what we have
	return "", "", id
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
