package doctor

import (
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

func TestSocketSplitBrainCheck_Metadata(t *testing.T) {
	check := NewSocketSplitBrainCheck()

	if check.Name() != "socket-split-brain" {
		t.Errorf("Name() = %q, want %q", check.Name(), "socket-split-brain")
	}
	if check.Description() != "Detect sessions duplicated across tmux sockets (causes communication failures)" {
		t.Errorf("Description() = %q", check.Description())
	}
	if check.Category() != CategoryInfrastructure {
		t.Errorf("Category() = %q, want %q", check.Category(), CategoryInfrastructure)
	}
	if !check.CanFix() {
		t.Error("CanFix() should return true")
	}
}

func TestSocketSplitBrainCheck_NoTownSocket(t *testing.T) {
	orig := tmux.GetDefaultSocket()
	defer tmux.SetDefaultSocket(orig)

	tmux.SetDefaultSocket("")

	check := NewSocketSplitBrainCheck()
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK when no town socket configured", result.Status)
	}
}

func TestSocketSplitBrainCheck_DefaultSocket(t *testing.T) {
	orig := tmux.GetDefaultSocket()
	defer tmux.SetDefaultSocket(orig)

	tmux.SetDefaultSocket("default")

	check := NewSocketSplitBrainCheck()
	result := check.Run(&CheckContext{})

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK when socket is 'default'", result.Status)
	}
}

func TestSocketSplitBrainCheck_FixNoDuplicates(t *testing.T) {
	check := NewSocketSplitBrainCheck()
	// No duplicates cached — Fix should be a no-op
	if err := check.Fix(&CheckContext{}); err != nil {
		t.Errorf("Fix() error = %v, want nil", err)
	}
}
