package terminal

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/steveyegge/gastown/internal/tmux"
)

// localTmux is the subset of tmux.Tmux methods used by TmuxBackend.
type localTmux interface {
	HasSession(name string) (bool, error)
	CapturePane(session string, lines int) (string, error)
	CapturePaneAll(session string) (string, error)
	CapturePaneLines(session string, lines int) ([]string, error)
	NudgeSession(session, message string) error
	SendKeysRaw(session, keys string) error
	IsPaneDead(session string) (bool, error)
	SetPaneDiedHook(session, agentID string) error
	KillSessionWithProcesses(session string) error
}

// TmuxBackend wraps a local tmux instance to implement Backend.
// This is the default backend for locally-running agents.
type TmuxBackend struct {
	tmux localTmux
}

// NewTmuxBackend creates a Backend backed by local tmux.
func NewTmuxBackend(t *tmux.Tmux) *TmuxBackend {
	return &TmuxBackend{tmux: t}
}

func (b *TmuxBackend) HasSession(session string) (bool, error) {
	return b.tmux.HasSession(session)
}

func (b *TmuxBackend) CapturePane(session string, lines int) (string, error) {
	return b.tmux.CapturePane(session, lines)
}

func (b *TmuxBackend) CapturePaneAll(session string) (string, error) {
	return b.tmux.CapturePaneAll(session)
}

func (b *TmuxBackend) CapturePaneLines(session string, lines int) ([]string, error) {
	return b.tmux.CapturePaneLines(session, lines)
}

func (b *TmuxBackend) NudgeSession(session string, message string) error {
	return b.tmux.NudgeSession(session, message)
}

func (b *TmuxBackend) SendKeys(session string, keys string) error {
	return b.tmux.SendKeysRaw(session, keys)
}

func (b *TmuxBackend) IsPaneDead(session string) (bool, error) {
	return b.tmux.IsPaneDead(session)
}

func (b *TmuxBackend) SetPaneDiedHook(session, agentID string) error {
	return b.tmux.SetPaneDiedHook(session, agentID)
}

// --- Coop-first stubs (return ErrNotSupported) ---

func (b *TmuxBackend) KillSession(session string) error { return b.tmux.KillSessionWithProcesses(session) }
func (b *TmuxBackend) IsAgentRunning(_ string) (bool, error)         { return false, ErrNotSupported }
func (b *TmuxBackend) GetAgentState(_ string) (string, error)        { return "", ErrNotSupported }
func (b *TmuxBackend) SetEnvironment(_, _, _ string) error           { return ErrNotSupported }
func (b *TmuxBackend) GetEnvironment(_, _ string) (string, error)    { return "", ErrNotSupported }
func (b *TmuxBackend) GetPaneWorkDir(_ string) (string, error)       { return "", ErrNotSupported }
func (b *TmuxBackend) SendInput(_ string, _ string, _ bool) error    { return ErrNotSupported }
func (b *TmuxBackend) RespawnPane(_ string) error                    { return ErrNotSupported }
func (b *TmuxBackend) SwitchSession(_ string, _ SwitchConfig) error { return ErrNotSupported }

// AttachSession attaches to a tmux session interactively.
// If already inside tmux, uses switch-client. Otherwise uses attach-session.
func (b *TmuxBackend) AttachSession(session string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	var cmd *exec.Cmd
	if os.Getenv("TMUX") != "" {
		cmd = exec.Command(tmuxPath, "switch-client", "-t", session)
	} else {
		cmd = exec.Command(tmuxPath, "attach-session", "-t", session)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
