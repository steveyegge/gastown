package doctor

import (
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

func TestNewCrossSocketZombieCheck(t *testing.T) {
	check := NewCrossSocketZombieCheck()

	if check.Name() != "cross-socket-zombies" {
		t.Errorf("expected name 'cross-socket-zombies', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}

	if check.Description() != "Detect agent sessions on wrong tmux socket" {
		t.Errorf("unexpected description: %q", check.Description())
	}
}

func TestCrossSocketZombieCheck_NoTownSocket(t *testing.T) {
	// When no town socket is configured (single-socket mode), check should pass
	old := tmux.GetDefaultSocket()
	tmux.SetDefaultSocket("")
	defer tmux.SetDefaultSocket(old)

	check := NewCrossSocketZombieCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no town socket, got %v: %s", result.Status, result.Message)
	}
}

func TestCrossSocketZombieCheck_DefaultSocket(t *testing.T) {
	// When town socket IS "default", check should sweep legacy sockets (gt, gas-town).
	// Without a real tmux server on those sockets, the check should still return OK
	// (it handles ListSessions errors gracefully).
	old := tmux.GetDefaultSocket()
	tmux.SetDefaultSocket("default")
	defer tmux.SetDefaultSocket(old)

	check := NewCrossSocketZombieCheck()
	result := check.Run(&CheckContext{TownRoot: t.TempDir()})

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no legacy socket servers running, got %v: %s", result.Status, result.Message)
	}
}

func TestCrossSocketTargets(t *testing.T) {
	old := tmux.GetDefaultSocket()
	defer tmux.SetDefaultSocket(old)

	// Empty socket → no targets
	tmux.SetDefaultSocket("")
	targets := crossSocketTargets()
	if targets != nil {
		t.Errorf("expected nil targets for empty socket, got %v", targets)
	}

	// "default" socket → legacy named sockets
	tmux.SetDefaultSocket("default")
	targets = crossSocketTargets()
	if len(targets) != 2 || targets[0] != "gt" || targets[1] != "gas-town" {
		t.Errorf("expected [gt gas-town] for default socket, got %v", targets)
	}

	// Named socket → ["default"]
	tmux.SetDefaultSocket("mytown")
	targets = crossSocketTargets()
	if len(targets) != 1 || targets[0] != "default" {
		t.Errorf("expected [default] for named socket, got %v", targets)
	}
}
