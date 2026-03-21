package cmd

import (
	"fmt"
	"testing"

	"github.com/steveyegge/gastown/internal/session"
)

type mockLegacyTmux struct {
	sessions []string
	listErr  error
	killed   []string
	killErr  error
}

func (m *mockLegacyTmux) ListSessions() ([]string, error) {
	return m.sessions, m.listErr
}

func (m *mockLegacyTmux) KillSessionWithProcesses(name string) error {
	if m.killErr != nil {
		return m.killErr
	}
	m.killed = append(m.killed, name)
	return nil
}

// setupLegacyHooks wires test hooks and a fresh registry, returning a cleanup func.
func setupLegacyHooks(t *testing.T, currentSocket string, mock *mockLegacyTmux) {
	t.Helper()

	origTmuxHook := legacyTmuxForTest
	origSocketHook := legacySocketForTest
	origRegistry := session.DefaultRegistry()
	t.Cleanup(func() {
		legacyTmuxForTest = origTmuxHook
		legacySocketForTest = origSocketHook
		session.SetDefaultRegistry(origRegistry)
	})

	legacySocketForTest = func() string { return currentSocket }
	legacyTmuxForTest = func(socket string) legacySocketTmux { return mock }

	r := session.NewPrefixRegistry()
	r.Register("ga", "gastown")
	session.SetDefaultRegistry(r)
}

// ---------------------------------------------------------------------------
// cleanupLegacyDefaultSocket
// ---------------------------------------------------------------------------

func TestCleanupLegacyDefaultSocket_SkipsWhenOnDefaultSocket(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "", mock)

	got := cleanupLegacyDefaultSocket()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCleanupLegacyDefaultSocket_SkipsWhenSocketIsDefault(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "default", mock)

	got := cleanupLegacyDefaultSocket()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCleanupLegacyDefaultSocket_CleansGastownSessions(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness", "hq-mayor"},
	}
	setupLegacyHooks(t, "gt-abc123", mock)

	got := cleanupLegacyDefaultSocket()
	if got != 2 {
		t.Errorf("expected 2 cleaned, got %d", got)
	}
	if len(mock.killed) != 2 {
		t.Fatalf("expected 2 killed, got %d: %v", len(mock.killed), mock.killed)
	}
	want := map[string]bool{"ga-witness": true, "hq-mayor": true}
	for _, k := range mock.killed {
		if !want[k] {
			t.Errorf("unexpected kill: %s", k)
		}
	}
}

func TestCleanupLegacyDefaultSocket_IgnoresNonGastownSessions(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"personal-stuff", "ga-witness"},
	}
	setupLegacyHooks(t, "gt-abc123", mock)

	got := cleanupLegacyDefaultSocket()
	if got != 1 {
		t.Errorf("expected 1 cleaned, got %d", got)
	}
	if len(mock.killed) != 1 || mock.killed[0] != "ga-witness" {
		t.Errorf("expected only ga-witness killed, got %v", mock.killed)
	}
}

func TestCleanupLegacyDefaultSocket_NoDefaultServer(t *testing.T) {
	mock := &mockLegacyTmux{
		listErr: fmt.Errorf("no server running"),
	}
	setupLegacyHooks(t, "gt-abc123", mock)

	got := cleanupLegacyDefaultSocket()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// countLegacyDefaultSocketSessions
// ---------------------------------------------------------------------------

func TestCountLegacyDefaultSocket_SkipsWhenOnDefault(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "", mock)

	got := countLegacyDefaultSocketSessions()
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCountLegacyDefaultSocket_CountsGastownOnly(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness", "personal"},
	}
	setupLegacyHooks(t, "gt-abc123", mock)

	got := countLegacyDefaultSocketSessions()
	if got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// cleanupLegacyBaseSocket
// ---------------------------------------------------------------------------

func TestCleanupLegacyBaseSocket_SkipsWhenSameSocket(t *testing.T) {
	mock := &mockLegacyTmux{}
	// LegacySocketName for "/some/path/gt" returns "gt"
	setupLegacyHooks(t, "gt", mock)

	got := cleanupLegacyBaseSocket("/some/path/gt")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCleanupLegacyBaseSocket_CleansOldSessions(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness"},
	}
	setupLegacyHooks(t, "gt-abc123", mock)

	got := cleanupLegacyBaseSocket("/some/path/gt")
	if got != 1 {
		t.Errorf("expected 1 cleaned, got %d", got)
	}
	if len(mock.killed) != 1 || mock.killed[0] != "ga-witness" {
		t.Errorf("expected ga-witness killed, got %v", mock.killed)
	}
}

// ---------------------------------------------------------------------------
// countLegacyBaseSocketSessions
// ---------------------------------------------------------------------------

func TestCountLegacyBaseSocket_SkipsWhenSame(t *testing.T) {
	mock := &mockLegacyTmux{}
	setupLegacyHooks(t, "gt", mock)

	got := countLegacyBaseSocketSessions("/some/path/gt")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCountLegacyBaseSocket_CountsCorrectly(t *testing.T) {
	mock := &mockLegacyTmux{
		sessions: []string{"ga-witness", "hq-deacon", "random-thing"},
	}
	setupLegacyHooks(t, "gt-abc123", mock)

	got := countLegacyBaseSocketSessions("/some/path/gt")
	if got != 2 {
		t.Errorf("expected 2, got %d", got)
	}
}
