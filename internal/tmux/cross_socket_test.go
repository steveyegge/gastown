package tmux

import (
	"fmt"
	"os"
	"testing"
)

var crossSocket = fmt.Sprintf("gt-test-cross-%d", os.Getpid())

func newCrossTestSocket(t *testing.T) *Tmux {
	t.Helper()
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	tm := NewTmuxWithSocket(crossSocket)
	t.Cleanup(func() {
		_ = tm.KillServer()
	})
	return tm
}

func TestCrossSocketIsolation(t *testing.T) {
	defaultTm := newTestTmux(t)
	crossTm := newCrossTestSocket(t)

	sessionA := fmt.Sprintf("gt-test-iso-a-%d", os.Getpid())
	sessionB := fmt.Sprintf("gt-test-iso-b-%d", os.Getpid())

	if err := defaultTm.NewSessionWithCommand(sessionA, ".", "sleep 300"); err != nil {
		t.Fatalf("create session A on default socket: %v", err)
	}
	t.Cleanup(func() { _ = defaultTm.KillSession(sessionA) })

	if err := crossTm.NewSessionWithCommand(sessionB, ".", "sleep 300"); err != nil {
		t.Fatalf("create session B on cross socket: %v", err)
	}
	t.Cleanup(func() { _ = crossTm.KillSession(sessionB) })

	crossSessions, err := crossTm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions on cross socket: %v", err)
	}
	for _, s := range crossSessions {
		if s == sessionA {
			t.Errorf("cross socket should NOT see session %q from default socket", sessionA)
		}
	}

	defaultSessions, err := defaultTm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions on default socket: %v", err)
	}
	for _, s := range defaultSessions {
		if s == sessionB {
			t.Errorf("default socket should NOT see session %q from cross socket", sessionB)
		}
	}

	hasA, err := defaultTm.HasSession(sessionA)
	if err != nil {
		t.Fatalf("HasSession(A) on default: %v", err)
	}
	if !hasA {
		t.Error("default socket should see its own session A")
	}

	hasB, err := crossTm.HasSession(sessionB)
	if err != nil {
		t.Fatalf("HasSession(B) on cross: %v", err)
	}
	if !hasB {
		t.Error("cross socket should see its own session B")
	}
}

func TestCrossSocketKill(t *testing.T) {
	defaultTm := newTestTmux(t)
	crossTm := newCrossTestSocket(t)

	defaultSession := fmt.Sprintf("gt-test-survive-%d", os.Getpid())
	crossSession := fmt.Sprintf("gt-test-doomed-%d", os.Getpid())

	if err := defaultTm.NewSessionWithCommand(defaultSession, ".", "sleep 300"); err != nil {
		t.Fatalf("create session on default socket: %v", err)
	}
	t.Cleanup(func() { _ = defaultTm.KillSession(defaultSession) })

	if err := crossTm.NewSessionWithCommand(crossSession, ".", "sleep 300"); err != nil {
		t.Fatalf("create session on cross socket: %v", err)
	}

	if err := crossTm.KillServer(); err != nil {
		t.Fatalf("KillServer on cross socket: %v", err)
	}

	has, err := defaultTm.HasSession(defaultSession)
	if err != nil {
		t.Fatalf("HasSession on default socket after cross KillServer: %v", err)
	}
	if !has {
		t.Error("default socket session should survive KillServer on cross socket")
	}
}

func TestSessionsOnMultipleSockets(t *testing.T) {
	defaultTm := newTestTmux(t)
	crossTm := newCrossTestSocket(t)

	defaultSessions := []string{
		fmt.Sprintf("gt-test-multi-d1-%d", os.Getpid()),
		fmt.Sprintf("gt-test-multi-d2-%d", os.Getpid()),
	}
	crossSessions := []string{
		fmt.Sprintf("gt-test-multi-c1-%d", os.Getpid()),
		fmt.Sprintf("gt-test-multi-c2-%d", os.Getpid()),
	}

	for _, name := range defaultSessions {
		if err := defaultTm.NewSessionWithCommand(name, ".", "sleep 300"); err != nil {
			t.Fatalf("create %q on default socket: %v", name, err)
		}
		t.Cleanup(func() { _ = defaultTm.KillSession(name) })
	}

	for _, name := range crossSessions {
		if err := crossTm.NewSessionWithCommand(name, ".", "sleep 300"); err != nil {
			t.Fatalf("create %q on cross socket: %v", name, err)
		}
		t.Cleanup(func() { _ = crossTm.KillSession(name) })
	}

	gotDefault, err := defaultTm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions on default: %v", err)
	}
	gotCross, err := crossTm.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions on cross: %v", err)
	}

	defaultSet := make(map[string]bool)
	for _, s := range gotDefault {
		defaultSet[s] = true
	}
	crossSet := make(map[string]bool)
	for _, s := range gotCross {
		crossSet[s] = true
	}

	for _, name := range defaultSessions {
		if !defaultSet[name] {
			t.Errorf("default socket missing its own session %q", name)
		}
		if crossSet[name] {
			t.Errorf("cross socket should NOT contain default session %q", name)
		}
	}

	for _, name := range crossSessions {
		if !crossSet[name] {
			t.Errorf("cross socket missing its own session %q", name)
		}
		if defaultSet[name] {
			t.Errorf("default socket should NOT contain cross session %q", name)
		}
	}

	crossCount := 0
	for _, s := range gotCross {
		for _, name := range crossSessions {
			if s == name {
				crossCount++
			}
		}
	}
	if crossCount != len(crossSessions) {
		t.Errorf("cross socket has %d of %d expected sessions", crossCount, len(crossSessions))
	}
}
