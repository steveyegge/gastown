package beadswatcher

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestStubWatcher_Events(t *testing.T) {
	w := NewStubWatcher(slog.Default())
	ch := w.Events()
	if ch == nil {
		t.Fatal("Events() returned nil channel")
	}
}

func TestStubWatcher_Start(t *testing.T) {
	w := NewStubWatcher(slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := w.Start(ctx)
	if err == nil {
		t.Fatal("Start() should return error when context canceled")
	}
}

func TestEventTypes(t *testing.T) {
	types := []EventType{AgentSpawn, AgentDone, AgentStuck, AgentKill}
	for _, et := range types {
		if et == "" {
			t.Error("event type should not be empty")
		}
	}
}

func TestActivityWatcher_Events(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	ch := w.Events()
	if ch == nil {
		t.Fatal("Events() returned nil channel")
	}
}

func TestActivityWatcher_DefaultConfig(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	if w.cfg.BdBinary != "bd" {
		t.Errorf("BdBinary = %q, want %q", w.cfg.BdBinary, "bd")
	}
	if w.cfg.Namespace != "gastown" {
		t.Errorf("Namespace = %q, want %q", w.cfg.Namespace, "gastown")
	}
}

func TestActivityWatcher_CustomConfig(t *testing.T) {
	w := NewActivityWatcher(Config{
		BdBinary:  "/usr/local/bin/bd",
		Namespace: "custom-ns",
		TownRoot:  "/opt/gastown",
	}, slog.Default())
	if w.cfg.BdBinary != "/usr/local/bin/bd" {
		t.Errorf("BdBinary = %q, want %q", w.cfg.BdBinary, "/usr/local/bin/bd")
	}
	if w.cfg.Namespace != "custom-ns" {
		t.Errorf("Namespace = %q, want %q", w.cfg.Namespace, "custom-ns")
	}
}

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"polecats", "polecat"},
		{"crews", "crew"},
		{"polecat", "polecat"},
		{"crew", "crew"},
		{"witness", "witness"},
		{"refinery", "refinery"},
		{"mayor", "mayor"},
		{"deacon", "deacon"},
	}
	for _, tc := range tests {
		got := normalizeRole(tc.input)
		if got != tc.want {
			t.Errorf("normalizeRole(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestExtractAgentInfo_Actor(t *testing.T) {
	raw := bdActivityEvent{Actor: "gastown/polecats/rictus"}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "polecat" || name != "rictus" {
		t.Errorf("got (%q, %q, %q), want (gastown, polecat, rictus)", rig, role, name)
	}
}

func TestExtractAgentInfo_ActorCrew(t *testing.T) {
	raw := bdActivityEvent{Actor: "gastown/crew/k8s"}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "crew" || name != "k8s" {
		t.Errorf("got (%q, %q, %q), want (gastown, crew, k8s)", rig, role, name)
	}
}

func TestExtractAgentInfo_Payload(t *testing.T) {
	raw := bdActivityEvent{
		Payload: map[string]interface{}{
			"rig":   "beads",
			"role":  "polecat",
			"agent": "worker1",
		},
	}
	rig, role, name := extractAgentInfo(raw)
	if rig != "beads" || role != "polecat" || name != "worker1" {
		t.Errorf("got (%q, %q, %q), want (beads, polecat, worker1)", rig, role, name)
	}
}

func TestExtractAgentInfo_PayloadAgentName(t *testing.T) {
	raw := bdActivityEvent{
		Payload: map[string]interface{}{
			"rig":        "gastown",
			"role":       "crews",
			"agent_name": "mobile",
		},
	}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "crew" || name != "mobile" {
		t.Errorf("got (%q, %q, %q), want (gastown, crew, mobile)", rig, role, name)
	}
}

func TestExtractAgentInfo_PayloadTarget(t *testing.T) {
	raw := bdActivityEvent{
		Payload: map[string]interface{}{
			"target": "gastown/polecats/slit",
		},
	}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "polecat" || name != "slit" {
		t.Errorf("got (%q, %q, %q), want (gastown, polecat, slit)", rig, role, name)
	}
}

func TestExtractAgentInfo_Empty(t *testing.T) {
	raw := bdActivityEvent{}
	rig, role, name := extractAgentInfo(raw)
	if rig != "" || role != "" || name != "" {
		t.Errorf("got (%q, %q, %q), want empty strings", rig, role, name)
	}
}

func TestExtractAgentInfo_ShortActor(t *testing.T) {
	raw := bdActivityEvent{Actor: "gastown/witness"}
	rig, role, name := extractAgentInfo(raw)
	// Only 2 parts, not enough for full agent info
	if rig != "" || role != "" || name != "" {
		t.Errorf("got (%q, %q, %q), want empty strings for short actor", rig, role, name)
	}
}

func TestParseLine_SlingEvent(t *testing.T) {
	w := NewActivityWatcher(Config{Namespace: "test-ns"}, slog.Default())
	line := `{"type":"sling","actor":"gastown/polecats/rictus","issue_id":"gt-abc123","message":"sling work"}`
	event, ok := w.parseLine(line)
	if !ok {
		t.Fatal("parseLine should return true for sling event")
	}
	if event.Type != AgentSpawn {
		t.Errorf("Type = %q, want %q", event.Type, AgentSpawn)
	}
	if event.Rig != "gastown" {
		t.Errorf("Rig = %q, want %q", event.Rig, "gastown")
	}
	if event.Role != "polecat" {
		t.Errorf("Role = %q, want %q", event.Role, "polecat")
	}
	if event.AgentName != "rictus" {
		t.Errorf("AgentName = %q, want %q", event.AgentName, "rictus")
	}
	if event.BeadID != "gt-abc123" {
		t.Errorf("BeadID = %q, want %q", event.BeadID, "gt-abc123")
	}
	if event.Metadata["namespace"] != "test-ns" {
		t.Errorf("namespace = %q, want %q", event.Metadata["namespace"], "test-ns")
	}
}

func TestParseLine_HookEvent(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	line := `{"type":"hook","actor":"gastown/polecats/slit","issue_id":"gt-xyz"}`
	event, ok := w.parseLine(line)
	if !ok {
		t.Fatal("parseLine should return true for hook event")
	}
	if event.Type != AgentSpawn {
		t.Errorf("Type = %q, want %q", event.Type, AgentSpawn)
	}
	if event.AgentName != "slit" {
		t.Errorf("AgentName = %q, want %q", event.AgentName, "slit")
	}
}

func TestParseLine_DoneEvent(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	line := `{"type":"done","actor":"gastown/polecats/rictus","issue_id":"gt-abc123"}`
	event, ok := w.parseLine(line)
	if !ok {
		t.Fatal("parseLine should return true for done event")
	}
	if event.Type != AgentDone {
		t.Errorf("Type = %q, want %q", event.Type, AgentDone)
	}
}

func TestParseLine_KillEvent(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	line := `{"type":"kill","actor":"gastown/polecats/rictus","issue_id":"gt-abc123"}`
	event, ok := w.parseLine(line)
	if !ok {
		t.Fatal("parseLine should return true for kill event")
	}
	if event.Type != AgentKill {
		t.Errorf("Type = %q, want %q", event.Type, AgentKill)
	}
}

func TestParseLine_EscalationEvent(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	line := `{"type":"escalation_sent","actor":"gastown/polecats/rictus","issue_id":"gt-esc1"}`
	event, ok := w.parseLine(line)
	if !ok {
		t.Fatal("parseLine should return true for escalation_sent event")
	}
	if event.Type != AgentStuck {
		t.Errorf("Type = %q, want %q", event.Type, AgentStuck)
	}
}

func TestParseLine_StatusClosed(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	line := `{"type":"status","actor":"gastown/polecats/rictus","issue_id":"gt-abc","old_status":"in_progress","new_status":"closed"}`
	event, ok := w.parseLine(line)
	if !ok {
		t.Fatal("parseLine should return true for status closed event")
	}
	if event.Type != AgentDone {
		t.Errorf("Type = %q, want %q", event.Type, AgentDone)
	}
}

func TestParseLine_StatusInProgress(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	line := `{"type":"status","actor":"gastown/polecats/rictus","issue_id":"gt-abc","old_status":"open","new_status":"in_progress"}`
	event, ok := w.parseLine(line)
	if !ok {
		t.Fatal("parseLine should return true for status in_progress event")
	}
	if event.Type != AgentSpawn {
		t.Errorf("Type = %q, want %q", event.Type, AgentSpawn)
	}
}

func TestParseLine_StatusOpen(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	line := `{"type":"status","actor":"gastown/polecats/rictus","issue_id":"gt-abc","old_status":"closed","new_status":"open"}`
	_, ok := w.parseLine(line)
	if ok {
		t.Error("parseLine should return false for status→open (not a lifecycle event)")
	}
}

func TestParseLine_IrrelevantType(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	irrelevant := []string{
		`{"type":"mail","actor":"gastown/polecats/rictus","issue_id":"gt-abc"}`,
		`{"type":"handoff","actor":"gastown/polecats/rictus","issue_id":"gt-abc"}`,
		`{"type":"patrol_started","actor":"gastown/witness/wit","issue_id":"gt-abc"}`,
		`{"type":"merged","actor":"gastown/refinery/ref","issue_id":"gt-abc"}`,
	}
	for _, line := range irrelevant {
		_, ok := w.parseLine(line)
		if ok {
			t.Errorf("parseLine should return false for irrelevant line: %s", line)
		}
	}
}

func TestParseLine_Empty(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	_, ok := w.parseLine("")
	if ok {
		t.Error("parseLine should return false for empty line")
	}
	_, ok = w.parseLine("   ")
	if ok {
		t.Error("parseLine should return false for whitespace line")
	}
}

func TestParseLine_MalformedJSON(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	_, ok := w.parseLine("not json at all")
	if ok {
		t.Error("parseLine should return false for malformed JSON")
	}
}

func TestParseLine_IncompleteAgentInfo(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	// Event with no actor or payload - can't extract agent info
	_, ok := w.parseLine(`{"type":"sling","issue_id":"gt-abc123"}`)
	if ok {
		t.Error("parseLine should return false when agent info is incomplete")
	}
}

func TestBuildMetadata_Defaults(t *testing.T) {
	w := NewActivityWatcher(Config{
		Namespace:    "test-ns",
		DefaultImage: "ghcr.io/gastown:latest",
		DaemonHost:   "bd-daemon",
		DaemonPort:   "9876",
	}, slog.Default())

	raw := bdActivityEvent{}
	meta := w.buildMetadata(raw)

	if meta["namespace"] != "test-ns" {
		t.Errorf("namespace = %q, want %q", meta["namespace"], "test-ns")
	}
	if meta["image"] != "ghcr.io/gastown:latest" {
		t.Errorf("image = %q, want %q", meta["image"], "ghcr.io/gastown:latest")
	}
	if meta["daemon_host"] != "bd-daemon" {
		t.Errorf("daemon_host = %q, want %q", meta["daemon_host"], "bd-daemon")
	}
	if meta["daemon_port"] != "9876" {
		t.Errorf("daemon_port = %q, want %q", meta["daemon_port"], "9876")
	}
}

func TestBuildMetadata_PayloadOverrides(t *testing.T) {
	w := NewActivityWatcher(Config{
		Namespace:    "default",
		DefaultImage: "default-image",
	}, slog.Default())

	raw := bdActivityEvent{
		Payload: map[string]interface{}{
			"image":     "custom-image:v2",
			"namespace": "custom-ns",
		},
	}
	meta := w.buildMetadata(raw)

	if meta["namespace"] != "custom-ns" {
		t.Errorf("namespace = %q, want %q", meta["namespace"], "custom-ns")
	}
	if meta["image"] != "custom-image:v2" {
		t.Errorf("image = %q, want %q", meta["image"], "custom-image:v2")
	}
}

func TestBuildMetadata_EmptyConfig(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	raw := bdActivityEvent{}
	meta := w.buildMetadata(raw)

	if meta["namespace"] != "gastown" {
		t.Errorf("namespace = %q, want %q", meta["namespace"], "gastown")
	}
	if _, ok := meta["image"]; ok {
		t.Error("image should not be set with empty DefaultImage")
	}
	if _, ok := meta["daemon_host"]; ok {
		t.Error("daemon_host should not be set with empty DaemonHost")
	}
}

func TestMapEvent_AllTypes(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	actor := "gastown/polecats/rictus"

	tests := []struct {
		eventType string
		wantType  EventType
	}{
		{"sling", AgentSpawn},
		{"hook", AgentSpawn},
		{"spawn", AgentSpawn},
		{"done", AgentDone},
		{"unhook", AgentDone},
		{"kill", AgentKill},
		{"session_death", AgentKill},
		{"escalation_sent", AgentStuck},
		{"polecat_nudged", AgentStuck},
	}

	for _, tc := range tests {
		raw := bdActivityEvent{Type: tc.eventType, Actor: actor, IssueID: "test-1"}
		event, ok := w.mapEvent(raw)
		if !ok {
			t.Errorf("mapEvent(%q) returned false, want true", tc.eventType)
			continue
		}
		if event.Type != tc.wantType {
			t.Errorf("mapEvent(%q).Type = %q, want %q", tc.eventType, event.Type, tc.wantType)
		}
	}
}

func TestMapEvent_UnknownType(t *testing.T) {
	w := NewActivityWatcher(Config{}, slog.Default())
	raw := bdActivityEvent{Type: "unknown_event", Actor: "gastown/polecats/rictus"}
	_, ok := w.mapEvent(raw)
	if ok {
		t.Error("mapEvent should return false for unknown event type")
	}
}

func TestActivityWatcher_Start_CancelledContext(t *testing.T) {
	w := NewActivityWatcher(Config{
		BdBinary: "false", // Binary that exits immediately with error
	}, slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := w.Start(ctx)
	if err == nil {
		t.Fatal("Start() should return error when context canceled")
	}
}
