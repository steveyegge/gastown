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

func TestSSEWatcher_Events(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	ch := w.Events()
	if ch == nil {
		t.Fatal("Events() returned nil channel")
	}
}

func TestSSEWatcher_ConfigFields(t *testing.T) {
	cfg := Config{
		DaemonHTTPURL: "http://localhost:9080",
		DaemonToken:   "test-token",
		Namespace:     "gastown-uat",
		DefaultImage:  "agent:latest",
		DaemonHost:    "bd-daemon",
		DaemonPort:    "9876",
	}
	w := NewSSEWatcher(cfg, slog.Default())
	if w.cfg.DaemonHTTPURL != "http://localhost:9080" {
		t.Errorf("DaemonHTTPURL = %q, want %q", w.cfg.DaemonHTTPURL, "http://localhost:9080")
	}
	if w.cfg.Namespace != "gastown-uat" {
		t.Errorf("Namespace = %q, want %q", w.cfg.Namespace, "gastown-uat")
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

func TestExtractAgentInfo_Labels(t *testing.T) {
	// Labels should take priority over actor and ID parsing.
	raw := mutationEvent{
		IssueID: "bd-beads-polecat-coral",
		Labels:  []string{"rig:beads", "role:polecat", "agent:coral", "gt:agent"},
	}
	rig, role, name := extractAgentInfo(raw)
	if rig != "beads" || role != "polecat" || name != "coral" {
		t.Errorf("got (%q, %q, %q), want (beads, polecat, coral)", rig, role, name)
	}
}

func TestExtractAgentInfo_LabelsPluralRole(t *testing.T) {
	raw := mutationEvent{
		IssueID: "bd-beads-polecats-coral",
		Labels:  []string{"rig:beads", "role:polecats", "agent:coral"},
	}
	rig, role, name := extractAgentInfo(raw)
	if rig != "beads" || role != "polecat" || name != "coral" {
		t.Errorf("got (%q, %q, %q), want (beads, polecat, coral)", rig, role, name)
	}
}

func TestExtractAgentInfo_LabelsOverrideActor(t *testing.T) {
	// When both labels and actor are present, labels win.
	raw := mutationEvent{
		Actor:  "wrong/polecats/wrong",
		Labels: []string{"rig:beads", "role:polecat", "agent:coral"},
	}
	rig, role, name := extractAgentInfo(raw)
	if rig != "beads" || role != "polecat" || name != "coral" {
		t.Errorf("got (%q, %q, %q), want (beads, polecat, coral)", rig, role, name)
	}
}

func TestExtractAgentInfo_PartialLabels_FallsThrough(t *testing.T) {
	// Incomplete labels should fall through to actor/ID parsing.
	raw := mutationEvent{
		Actor:  "gastown/polecats/rictus",
		Labels: []string{"rig:beads"}, // missing role and agent
	}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "polecat" || name != "rictus" {
		t.Errorf("got (%q, %q, %q), want (gastown, polecat, rictus)", rig, role, name)
	}
}

func TestExtractAgentInfo_Actor(t *testing.T) {
	raw := mutationEvent{Actor: "gastown/polecats/rictus"}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "polecat" || name != "rictus" {
		t.Errorf("got (%q, %q, %q), want (gastown, polecat, rictus)", rig, role, name)
	}
}

func TestExtractAgentInfo_ActorCrew(t *testing.T) {
	raw := mutationEvent{Actor: "gastown/crew/k8s"}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "crew" || name != "k8s" {
		t.Errorf("got (%q, %q, %q), want (gastown, crew, k8s)", rig, role, name)
	}
}

func TestExtractAgentInfo_IssueIDFallback(t *testing.T) {
	raw := mutationEvent{IssueID: "gastown-polecats-furiosa"}
	rig, role, name := extractAgentInfo(raw)
	if rig != "gastown" || role != "polecat" || name != "furiosa" {
		t.Errorf("got (%q, %q, %q), want (gastown, polecat, furiosa)", rig, role, name)
	}
}

func TestExtractAgentInfo_Empty(t *testing.T) {
	raw := mutationEvent{}
	rig, role, name := extractAgentInfo(raw)
	if rig != "" || role != "" || name != "" {
		t.Errorf("got (%q, %q, %q), want empty strings", rig, role, name)
	}
}

func TestExtractAgentInfo_ShortActor(t *testing.T) {
	raw := mutationEvent{Actor: "gastown/witness"}
	rig, role, name := extractAgentInfo(raw)
	// Only 2 parts, not enough for full agent info — falls through to IssueID
	if rig != "" || role != "" || name != "" {
		t.Errorf("got (%q, %q, %q), want empty strings for short actor", rig, role, name)
	}
}

func TestParseAgentBeadID_Mayor(t *testing.T) {
	rig, role, name := parseAgentBeadID("hq-mayor")
	if rig != "town" || role != "mayor" || name != "hq" {
		t.Errorf("got (%q, %q, %q), want (town, mayor, hq)", rig, role, name)
	}
}

func TestParseAgentBeadID_Deacon(t *testing.T) {
	rig, role, name := parseAgentBeadID("hq-deacon")
	if rig != "town" || role != "deacon" || name != "hq" {
		t.Errorf("got (%q, %q, %q), want (town, deacon, hq)", rig, role, name)
	}
}

func TestParseAgentBeadID_ThreeParts(t *testing.T) {
	rig, role, name := parseAgentBeadID("gastown-polecats-rictus")
	if rig != "gastown" || role != "polecat" || name != "rictus" {
		t.Errorf("got (%q, %q, %q), want (gastown, polecat, rictus)", rig, role, name)
	}
}

func TestParseAgentBeadID_TwoParts(t *testing.T) {
	rig, role, name := parseAgentBeadID("unknown-thing")
	// Only 2 parts — can't split into rig/role/name
	if name != "unknown-thing" {
		t.Errorf("expected name=%q for 2-part ID, got (%q, %q, %q)", "unknown-thing", rig, role, name)
	}
}

func TestIsAgentBead_ByType(t *testing.T) {
	raw := mutationEvent{IssueType: "agent"}
	if !isAgentBead(raw) {
		t.Error("should return true for IssueType=agent")
	}
}

func TestIsAgentBead_ByLabel(t *testing.T) {
	raw := mutationEvent{Labels: []string{"some-label", "gt:agent"}}
	if !isAgentBead(raw) {
		t.Error("should return true for gt:agent label")
	}
}

func TestIsAgentBead_NotAgent(t *testing.T) {
	raw := mutationEvent{IssueType: "task", Labels: []string{"some-label"}}
	if isAgentBead(raw) {
		t.Error("should return false for non-agent bead")
	}
}

func TestMapMutation_Create(t *testing.T) {
	w := NewSSEWatcher(Config{Namespace: "test-ns"}, slog.Default())
	raw := mutationEvent{Type: "create", Actor: "gastown/polecats/rictus", IssueID: "gt-abc"}
	event, ok := w.mapMutation(raw)
	if !ok {
		t.Fatal("mapMutation should return true for create")
	}
	if event.Type != AgentSpawn {
		t.Errorf("Type = %q, want %q", event.Type, AgentSpawn)
	}
}

func TestMapMutation_StatusClosed(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	raw := mutationEvent{
		Type:      "status",
		Actor:     "gastown/polecats/rictus",
		IssueID:   "gt-abc",
		NewStatus: "closed",
	}
	event, ok := w.mapMutation(raw)
	if !ok {
		t.Fatal("mapMutation should return true for status→closed")
	}
	if event.Type != AgentDone {
		t.Errorf("Type = %q, want %q", event.Type, AgentDone)
	}
}

func TestMapMutation_StatusInProgress(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	raw := mutationEvent{
		Type:      "status",
		Actor:     "gastown/polecats/rictus",
		IssueID:   "gt-abc",
		NewStatus: "in_progress",
	}
	event, ok := w.mapMutation(raw)
	if !ok {
		t.Fatal("mapMutation should return true for status→in_progress")
	}
	if event.Type != AgentSpawn {
		t.Errorf("Type = %q, want %q", event.Type, AgentSpawn)
	}
}

func TestMapMutation_StatusOpen(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	raw := mutationEvent{
		Type:      "status",
		Actor:     "gastown/polecats/rictus",
		NewStatus: "open",
	}
	_, ok := w.mapMutation(raw)
	if ok {
		t.Error("mapMutation should return false for status→open")
	}
}

func TestMapMutation_Delete(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	raw := mutationEvent{Type: "delete", Actor: "gastown/polecats/rictus", IssueID: "gt-abc"}
	event, ok := w.mapMutation(raw)
	if !ok {
		t.Fatal("mapMutation should return true for delete")
	}
	if event.Type != AgentKill {
		t.Errorf("Type = %q, want %q", event.Type, AgentKill)
	}
}

func TestMapMutation_Update(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	raw := mutationEvent{Type: "update", Actor: "gastown/polecats/rictus"}
	event, ok := w.mapMutation(raw)
	if !ok {
		t.Error("mapMutation should return true for update on agent bead")
	}
	if event.Type != AgentUpdate {
		t.Errorf("event type = %q, want %q", event.Type, AgentUpdate)
	}
}

func TestBuildEvent_Metadata(t *testing.T) {
	w := NewSSEWatcher(Config{
		Namespace:    "test-ns",
		DefaultImage: "ghcr.io/gastown:latest",
		DaemonHost:   "bd-daemon",
		DaemonPort:   "9876",
	}, slog.Default())

	raw := mutationEvent{Actor: "gastown/polecats/rictus", IssueID: "gt-abc"}
	event, ok := w.buildEvent(AgentSpawn, raw)
	if !ok {
		t.Fatal("buildEvent should return true")
	}

	if event.Metadata["namespace"] != "test-ns" {
		t.Errorf("namespace = %q, want %q", event.Metadata["namespace"], "test-ns")
	}
	if event.Metadata["image"] != "ghcr.io/gastown:latest" {
		t.Errorf("image = %q, want %q", event.Metadata["image"], "ghcr.io/gastown:latest")
	}
	if event.Metadata["daemon_host"] != "bd-daemon" {
		t.Errorf("daemon_host = %q, want %q", event.Metadata["daemon_host"], "bd-daemon")
	}
	if event.Metadata["daemon_port"] != "9876" {
		t.Errorf("daemon_port = %q, want %q", event.Metadata["daemon_port"], "9876")
	}
}

func TestBuildEvent_EmptyConfig(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	raw := mutationEvent{Actor: "gastown/polecats/rictus", IssueID: "gt-abc"}
	event, ok := w.buildEvent(AgentSpawn, raw)
	if !ok {
		t.Fatal("buildEvent should return true")
	}
	if _, hasImage := event.Metadata["image"]; hasImage {
		t.Error("image should not be set with empty DefaultImage")
	}
	if _, hasDaemonHost := event.Metadata["daemon_host"]; hasDaemonHost {
		t.Error("daemon_host should not be set with empty DaemonHost")
	}
}

func TestBuildEvent_IncompleteAgentInfo(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())
	// No actor, no parseable IssueID
	raw := mutationEvent{IssueID: "x"}
	_, ok := w.buildEvent(AgentSpawn, raw)
	if ok {
		t.Error("buildEvent should return false when agent info is incomplete")
	}
}

func TestProcessSSEData_AgentBead(t *testing.T) {
	w := NewSSEWatcher(Config{Namespace: "test-ns"}, slog.Default())

	data := `{"Type":"create","IssueID":"gt-abc123","Actor":"gastown/polecats/rictus","issue_type":"agent"}`
	w.processSSEData(data)

	select {
	case event := <-w.events:
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
	default:
		t.Fatal("expected event to be emitted")
	}
}

func TestProcessSSEData_AgentBeadWithLabels(t *testing.T) {
	w := NewSSEWatcher(Config{Namespace: "test-ns"}, slog.Default())

	// This is the exact scenario that was broken: bd-beads-polecat-coral
	// with rig/role/agent labels. Without the fix, it would parse as
	// rig=bd, role=beads, agent=polecat-coral.
	data := `{"Type":"create","IssueID":"bd-beads-polecat-coral","issue_type":"agent","labels":["rig:beads","role:polecat","agent:coral","execution_target:k8s","gt:agent"]}`
	w.processSSEData(data)

	select {
	case event := <-w.events:
		if event.Type != AgentSpawn {
			t.Errorf("Type = %q, want %q", event.Type, AgentSpawn)
		}
		if event.Rig != "beads" {
			t.Errorf("Rig = %q, want %q", event.Rig, "beads")
		}
		if event.Role != "polecat" {
			t.Errorf("Role = %q, want %q", event.Role, "polecat")
		}
		if event.AgentName != "coral" {
			t.Errorf("AgentName = %q, want %q", event.AgentName, "coral")
		}
	default:
		t.Fatal("expected event to be emitted")
	}
}

func TestProcessSSEData_NonAgentBead(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())

	data := `{"Type":"create","IssueID":"task-123","Actor":"user","issue_type":"task"}`
	w.processSSEData(data)

	select {
	case <-w.events:
		t.Fatal("should not emit event for non-agent bead")
	default:
		// good
	}
}

func TestProcessSSEData_MalformedJSON(t *testing.T) {
	w := NewSSEWatcher(Config{}, slog.Default())

	w.processSSEData("not json at all")

	select {
	case <-w.events:
		t.Fatal("should not emit event for malformed JSON")
	default:
		// good
	}
}
