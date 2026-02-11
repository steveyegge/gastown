package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// mockAgentLister is a test AgentLister.
type mockAgentLister struct {
	beads map[string]*beads.Issue
	err   error
}

func (m *mockAgentLister) ListAgentBeads() (map[string]*beads.Issue, error) {
	return m.beads, m.err
}

// mockNotesReader returns canned notes per agent ID.
type mockNotesReader struct {
	notes map[string]string
}

func (m *mockNotesReader) GetAgentNotes(agentID string) (string, error) {
	n, ok := m.notes[agentID]
	if !ok {
		return "", fmt.Errorf("no notes for %s", agentID)
	}
	return n, nil
}

// mockTmuxLister returns a canned list of tmux sessions.
type mockTmuxLister struct {
	sessions []string
	err      error
}

func (m *mockTmuxLister) ListSessions() ([]string, error) {
	return m.sessions, m.err
}

func TestDiscoverAll_Empty(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		&mockNotesReader{notes: map[string]string{}},
		&mockTmuxLister{sessions: nil},
	)

	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestDiscoverAll_LocalTmux(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"hq-mayor": {
				ID:          "hq-mayor",
				Title:       "Mayor",
				Description: "Mayor\n\nrole_type: mayor\nrig: null\nagent_state: working",
				Labels:      []string{"gt:agent"},
			},
		}},
		&mockNotesReader{notes: map[string]string{}},
		&mockTmuxLister{sessions: []string{"hq-mayor", "hq-deacon"}},
	)

	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.ID != "hq-mayor" {
		t.Errorf("ID = %q, want hq-mayor", s.ID)
	}
	if s.Role != "mayor" {
		t.Errorf("Role = %q, want mayor", s.Role)
	}
	if s.BackendType != "tmux" {
		t.Errorf("BackendType = %q, want tmux", s.BackendType)
	}
	if !s.Alive {
		t.Error("expected Alive=true for tmux session in list")
	}
	if s.AgentState != "working" {
		t.Errorf("AgentState = %q, want working", s.AgentState)
	}
}

func TestDiscoverAll_CoopFromNotes(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"gt-gastown-crew-k8s": {
				ID:          "gt-gastown-crew-k8s",
				Title:       "k8s crew",
				Description: "k8s crew\n\nrole_type: crew\nrig: gastown\nagent_state: spawning",
				Labels:      []string{"gt:agent", "execution_target:k8s"},
			},
		}},
		&mockNotesReader{notes: map[string]string{
			"gt-gastown-crew-k8s": "backend: coop\ncoop_url: http://10.0.1.5:8080",
		}},
		nil, // no tmux in K8s
	)

	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.BackendType != "coop" {
		t.Errorf("BackendType = %q, want coop", s.BackendType)
	}
	if s.CoopURL != "http://10.0.1.5:8080" {
		t.Errorf("CoopURL = %q, want http://10.0.1.5:8080", s.CoopURL)
	}
	if s.Target != "k8s" {
		t.Errorf("Target = %q, want k8s", s.Target)
	}
	if s.Rig != "gastown" {
		t.Errorf("Rig = %q, want gastown", s.Rig)
	}
}

func TestDiscoverAll_K8sDefaultsToCoop(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"gt-gastown-witness": {
				ID:          "gt-gastown-witness",
				Title:       "Witness",
				Description: "Witness\n\nrole_type: witness\nrig: gastown\nagent_state: working",
				Labels:      []string{"gt:agent", "execution_target:k8s"},
			},
		}},
		&mockNotesReader{notes: map[string]string{}}, // no notes yet
		nil,
	)

	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	// K8s target without coop_url should default to coop backend
	if s.BackendType != "coop" {
		t.Errorf("BackendType = %q, want coop (K8s default)", s.BackendType)
	}
}

func TestDiscoverRig_Filters(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"hq-mayor": {
				ID:          "hq-mayor",
				Title:       "Mayor",
				Description: "Mayor\n\nrole_type: mayor\nrig: null\nagent_state: working",
				Labels:      []string{"gt:agent"},
			},
			"gt-gastown-witness": {
				ID:          "gt-gastown-witness",
				Title:       "Witness",
				Description: "Witness\n\nrole_type: witness\nrig: gastown\nagent_state: working",
				Labels:      []string{"gt:agent"},
			},
			"gt-beads-witness": {
				ID:          "gt-beads-witness",
				Title:       "Witness",
				Description: "Witness\n\nrole_type: witness\nrig: beads\nagent_state: working",
				Labels:      []string{"gt:agent"},
			},
		}},
		&mockNotesReader{notes: map[string]string{}},
		&mockTmuxLister{sessions: nil},
	)

	sessions, err := reg.DiscoverRig(context.Background(), "gastown", DiscoverOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 gastown session, got %d", len(sessions))
	}
	if len(sessions) > 0 && sessions[0].Rig != "gastown" {
		t.Errorf("Rig = %q, want gastown", sessions[0].Rig)
	}
}

func TestLookup_Found(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"hq-mayor": {
				ID:          "hq-mayor",
				Title:       "Mayor",
				Description: "Mayor\n\nrole_type: mayor\nrig: null\nagent_state: working",
				Labels:      []string{"gt:agent"},
			},
		}},
		&mockNotesReader{notes: map[string]string{}},
		nil,
	)

	s, err := reg.Lookup(context.Background(), "hq-mayor", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "hq-mayor" {
		t.Errorf("ID = %q, want hq-mayor", s.ID)
	}
}

func TestLookup_NotFound(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		&mockNotesReader{notes: map[string]string{}},
		nil,
	)

	_, err := reg.Lookup(context.Background(), "missing", false)
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestDiscoverAll_WithLivenessCheck(t *testing.T) {
	// Create a mock coop server that returns healthy
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := int32(1234)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "running",
			"pid":    pid,
			"ready":  true,
		})
	}))
	defer srv.Close()

	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"gt-gastown-crew-k8s": {
				ID:          "gt-gastown-crew-k8s",
				Title:       "k8s crew",
				Description: "k8s crew\n\nrole_type: crew\nrig: gastown\nagent_state: working",
				Labels:      []string{"gt:agent", "execution_target:k8s"},
			},
		}},
		&mockNotesReader{notes: map[string]string{
			"gt-gastown-crew-k8s": "backend: coop\ncoop_url: " + srv.URL,
		}},
		nil,
	)

	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{
		CheckLiveness: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if !sessions[0].Alive {
		t.Error("expected Alive=true after successful health check")
	}
}

func TestDiscoverAll_LivenessCheckUnreachable(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"gt-gastown-crew-k8s": {
				ID:          "gt-gastown-crew-k8s",
				Title:       "k8s crew",
				Description: "k8s crew\n\nrole_type: crew\nrig: gastown\nagent_state: working",
				Labels:      []string{"gt:agent", "execution_target:k8s"},
			},
		}},
		&mockNotesReader{notes: map[string]string{
			"gt-gastown-crew-k8s": "backend: coop\ncoop_url: http://127.0.0.1:1", // unreachable
		}},
		nil,
	)

	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{
		CheckLiveness: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Alive {
		t.Error("expected Alive=false for unreachable coop")
	}
}

func TestDiscoverAll_ListerError(t *testing.T) {
	reg := New(
		&mockAgentLister{err: fmt.Errorf("daemon unreachable")},
		nil,
		nil,
	)

	_, err := reg.DiscoverAll(context.Background(), DiscoverOpts{})
	if err == nil {
		t.Fatal("expected error when lister fails")
	}
}

func TestDiscoverAll_TmuxErrors_Graceful(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"hq-mayor": {
				ID:          "hq-mayor",
				Title:       "Mayor",
				Description: "Mayor\n\nrole_type: mayor\nrig: null\nagent_state: working",
				Labels:      []string{"gt:agent"},
			},
		}},
		&mockNotesReader{notes: map[string]string{}},
		&mockTmuxLister{err: fmt.Errorf("tmux not running")},
	)

	// Should still succeed — tmux errors are non-fatal
	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// Mayor should not be alive since tmux failed
	if sessions[0].Alive {
		t.Error("expected Alive=false when tmux list failed")
	}
}

func TestApplyNotes_BackendDetection(t *testing.T) {
	tests := []struct {
		name        string
		notes       string
		wantBackend string
		wantCoopURL string
	}{
		{"coop explicit", "backend: coop\ncoop_url: http://x:8080", "coop", "http://x:8080"},
		{"coop from url only", "coop_url: http://x:8080", "coop", "http://x:8080"},
		{"ssh backend", "backend: k8s\nssh_host: user@pod", "ssh", ""},
		{"ssh alt", "backend: ssh\nssh_host: user@pod", "ssh", ""},
		{"no backend info", "some_key: some_val", "tmux", ""},
	}

	reg := &SessionRegistry{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{BackendType: "tmux"}
			reg.applyNotes(s, tt.notes)
			if s.BackendType != tt.wantBackend {
				t.Errorf("BackendType = %q, want %q", s.BackendType, tt.wantBackend)
			}
			if s.CoopURL != tt.wantCoopURL {
				t.Errorf("CoopURL = %q, want %q", s.CoopURL, tt.wantCoopURL)
			}
		})
	}
}

func TestParseNameFromID(t *testing.T) {
	tests := []struct {
		id, rig, role string
		want          string
	}{
		{"hq-mayor", "", "mayor", "hq"},
		{"hq-deacon", "", "deacon", "hq"},
		{"hq-boot", "", "boot", "hq"},
		{"gt-gastown-witness", "gastown", "witness", "witness"},
		{"gt-gastown-crew-k8s", "gastown", "crew", "k8s"},
		{"gt-beads-refinery", "beads", "refinery", "refinery"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := parseNameFromID(tt.id, tt.rig, tt.role)
			if got != tt.want {
				t.Errorf("parseNameFromID(%q, %q, %q) = %q, want %q", tt.id, tt.rig, tt.role, got, tt.want)
			}
		})
	}
}

// --- mockBeadWriter for lifecycle tests ---

type mockBeadWriter struct {
	created    map[string]*beads.Issue
	closed     map[string]string
	labels     map[string][]string
	createErr  error
	closeErr   error
	addLabelErr error
}

func newMockWriter() *mockBeadWriter {
	return &mockBeadWriter{
		created: make(map[string]*beads.Issue),
		closed:  make(map[string]string),
		labels:  make(map[string][]string),
	}
}

func (m *mockBeadWriter) CreateOrReopenAgentBead(id, title string, fields *beads.AgentFields) (*beads.Issue, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	issue := &beads.Issue{
		ID:          id,
		Title:       title,
		Description: beads.FormatAgentDescription(title, fields),
		Labels:      []string{"gt:agent"},
	}
	m.created[id] = issue
	return issue, nil
}

func (m *mockBeadWriter) CloseAndClearAgentBead(id, reason string) error {
	if m.closeErr != nil {
		return m.closeErr
	}
	m.closed[id] = reason
	return nil
}

func (m *mockBeadWriter) AddLabel(id, label string) error {
	if m.addLabelErr != nil {
		return m.addLabelErr
	}
	m.labels[id] = append(m.labels[id], label)
	return nil
}

func TestCreateSession_K8s(t *testing.T) {
	writer := newMockWriter()
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		&mockNotesReader{notes: map[string]string{}},
		nil,
	)
	reg.SetWriter(writer)

	s, err := reg.CreateSession(CreateSessionOpts{
		ID:    "gt-gastown-crew-k8s",
		Title: "k8s crew",
		Rig:   "gastown",
		Role:  "crew",
		K8s:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "gt-gastown-crew-k8s" {
		t.Errorf("ID = %q, want gt-gastown-crew-k8s", s.ID)
	}
	if s.Target != "k8s" {
		t.Errorf("Target = %q, want k8s", s.Target)
	}
	if s.BackendType != "coop" {
		t.Errorf("BackendType = %q, want coop", s.BackendType)
	}

	// Verify bead was created
	if _, ok := writer.created["gt-gastown-crew-k8s"]; !ok {
		t.Error("expected bead to be created")
	}

	// Verify K8s label was added
	labels := writer.labels["gt-gastown-crew-k8s"]
	found := false
	for _, l := range labels {
		if l == "execution_target:k8s" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected execution_target:k8s label, got %v", labels)
	}
}

func TestCreateSession_Local(t *testing.T) {
	writer := newMockWriter()
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		&mockNotesReader{notes: map[string]string{}},
		nil,
	)
	reg.SetWriter(writer)

	s, err := reg.CreateSession(CreateSessionOpts{
		ID:    "hq-mayor",
		Title: "Mayor",
		Role:  "mayor",
		K8s:   false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Target != "local" {
		t.Errorf("Target = %q, want local", s.Target)
	}
	if s.BackendType != "tmux" {
		t.Errorf("BackendType = %q, want tmux", s.BackendType)
	}

	// No K8s label
	if len(writer.labels["hq-mayor"]) != 0 {
		t.Errorf("expected no labels for local agent, got %v", writer.labels["hq-mayor"])
	}
}

func TestCreateSession_NoWriter(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		nil,
		nil,
	)

	_, err := reg.CreateSession(CreateSessionOpts{ID: "test", Title: "Test"})
	if err == nil {
		t.Fatal("expected error when no writer configured")
	}
}

func TestCreateSession_EmptyID(t *testing.T) {
	writer := newMockWriter()
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		nil,
		nil,
	)
	reg.SetWriter(writer)

	_, err := reg.CreateSession(CreateSessionOpts{Title: "Test"})
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestDestroySession(t *testing.T) {
	writer := newMockWriter()
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		nil,
		nil,
	)
	reg.SetWriter(writer)

	err := reg.DestroySession("gt-gastown-crew-k8s", "shutdown requested")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reason, ok := writer.closed["gt-gastown-crew-k8s"]
	if !ok {
		t.Fatal("expected bead to be closed")
	}
	if reason != "shutdown requested" {
		t.Errorf("close reason = %q, want %q", reason, "shutdown requested")
	}
}

func TestDestroySession_NoWriter(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{}},
		nil,
		nil,
	)

	err := reg.DestroySession("test", "reason")
	if err == nil {
		t.Fatal("expected error when no writer configured")
	}
}

func TestRestartSession_CoopBackend(t *testing.T) {
	// Create mock coop server that accepts PUT /api/v1/session/switch
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		switch r.URL.Path {
		case "/api/v1/agent/state":
			json.NewEncoder(w).Encode(map[string]string{"state": "working", "agent": "claude"})
		case "/api/v1/session/switch":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"gt-gastown-crew-k8s": {
				ID:          "gt-gastown-crew-k8s",
				Title:       "k8s crew",
				Description: "k8s crew\n\nrole_type: crew\nrig: gastown\nagent_state: working",
				Labels:      []string{"gt:agent", "execution_target:k8s"},
			},
		}},
		&mockNotesReader{notes: map[string]string{
			"gt-gastown-crew-k8s": "backend: coop\ncoop_url: " + srv.URL,
		}},
		nil,
	)

	err := reg.RestartSession(context.Background(), "gt-gastown-crew-k8s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PUT" {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/api/v1/session/switch" {
		t.Errorf("expected /api/v1/session/switch, got %s", gotPath)
	}
}

func TestRestartSession_TmuxBackendFails(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"hq-mayor": {
				ID:          "hq-mayor",
				Title:       "Mayor",
				Description: "Mayor\n\nrole_type: mayor\nrig: null\nagent_state: working",
				Labels:      []string{"gt:agent"},
			},
		}},
		&mockNotesReader{notes: map[string]string{}},
		nil,
	)

	err := reg.RestartSession(context.Background(), "hq-mayor")
	if err == nil {
		t.Fatal("expected error for non-coop session")
	}
}

func TestHookBeadFromJSON(t *testing.T) {
	reg := New(
		&mockAgentLister{beads: map[string]*beads.Issue{
			"hq-mayor": {
				ID:          "hq-mayor",
				Title:       "Mayor",
				Description: "Mayor\n\nrole_type: mayor\nrig: null\nagent_state: working\nhook_bead: null",
				Labels:      []string{"gt:agent"},
				HookBead:    "bd-abc123", // JSON field overrides description
				AgentState:  "hooked",    // JSON field overrides description
			},
		}},
		&mockNotesReader{notes: map[string]string{}},
		nil,
	)

	sessions, err := reg.DiscoverAll(context.Background(), DiscoverOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].HookBead != "bd-abc123" {
		t.Errorf("HookBead = %q, want bd-abc123", sessions[0].HookBead)
	}
	if sessions[0].AgentState != "hooked" {
		t.Errorf("AgentState = %q, want hooked", sessions[0].AgentState)
	}
}
