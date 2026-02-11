package terminal

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// SSHBackend runs tmux commands on a remote host via SSH.
// Used for K8s-hosted polecats where the pod runs sshd + tmux.
type SSHBackend struct {
	// Host is the SSH target (e.g., "gt@pod-name.namespace" or "gt@localhost -p 2222").
	Host string

	// Port is the SSH port (default 22).
	Port int

	// IdentityFile is the path to the SSH private key.
	IdentityFile string

	// ProxyCommand is an optional SSH ProxyCommand for tunneling (e.g., kubectl port-forward).
	ProxyCommand string

	// nudgeLocks serializes nudges to the same session (mirrors local tmux behavior).
	nudgeLocks sync.Map
}

// SSHConfig configures an SSH connection to a remote polecat.
type SSHConfig struct {
	Host         string
	Port         int
	IdentityFile string
	ProxyCommand string
}

// NewSSHBackend creates a Backend that runs tmux commands over SSH.
func NewSSHBackend(cfg SSHConfig) *SSHBackend {
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	return &SSHBackend{
		Host:         cfg.Host,
		Port:         port,
		IdentityFile: cfg.IdentityFile,
		ProxyCommand: cfg.ProxyCommand,
	}
}

// sshArgs builds the base SSH command arguments.
func (b *SSHBackend) sshArgs() []string {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		"-p", fmt.Sprintf("%d", b.Port),
	}
	if b.IdentityFile != "" {
		args = append(args, "-i", b.IdentityFile)
	}
	if b.ProxyCommand != "" {
		args = append(args, "-o", fmt.Sprintf("ProxyCommand=%s", b.ProxyCommand))
	}
	args = append(args, b.Host)
	return args
}

// runRemote executes a command on the remote host via SSH.
func (b *SSHBackend) runRemote(timeout time.Duration, remoteCmd string) (string, error) {
	args := append(b.sshArgs(), remoteCmd)
	cmd := exec.Command("ssh", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			errMsg := strings.TrimSpace(stderr.String())
			if errMsg != "" {
				return "", fmt.Errorf("ssh remote command failed: %s: %w", errMsg, err)
			}
			return "", fmt.Errorf("ssh remote command failed: %w", err)
		}
		return strings.TrimSpace(stdout.String()), nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("ssh command timed out after %v", timeout)
	}
}

func (b *SSHBackend) HasSession(session string) (bool, error) {
	out, err := b.runRemote(10*time.Second, fmt.Sprintf("tmux has-session -t '=%s' 2>/dev/null && echo yes || echo no", session))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "yes", nil
}

func (b *SSHBackend) CapturePane(session string, lines int) (string, error) {
	cmd := fmt.Sprintf("tmux capture-pane -p -t '=%s' -S -%d", session, lines)
	return b.runRemote(10*time.Second, cmd)
}

func (b *SSHBackend) CapturePaneAll(session string) (string, error) {
	cmd := fmt.Sprintf("tmux capture-pane -p -t '=%s' -S -", session)
	return b.runRemote(10*time.Second, cmd)
}

func (b *SSHBackend) CapturePaneLines(session string, lines int) ([]string, error) {
	out, err := b.CapturePane(session, lines)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (b *SSHBackend) NudgeSession(session string, message string) error {
	// Serialize nudges to the same session
	lockI, _ := b.nudgeLocks.LoadOrStore(session, &sync.Mutex{})
	lock := lockI.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// Send text in literal mode
	escaped := strings.ReplaceAll(message, "'", "'\\''")
	cmd := fmt.Sprintf("tmux send-keys -t '=%s' -l '%s'", session, escaped)
	if _, err := b.runRemote(10*time.Second, cmd); err != nil {
		return fmt.Errorf("sending text: %w", err)
	}

	// Wait for paste to settle
	time.Sleep(3 * time.Second)

	// Send Escape (exit vim mode if active)
	escCmd := fmt.Sprintf("tmux send-keys -t '=%s' Escape", session)
	_, _ = b.runRemote(5*time.Second, escCmd)

	// Send Enter to submit
	enterCmd := fmt.Sprintf("tmux send-keys -t '=%s' Enter", session)
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		_, _ = b.runRemote(5*time.Second, enterCmd)
	}

	return nil
}

func (b *SSHBackend) SendKeys(session string, keys string) error {
	escaped := strings.ReplaceAll(keys, "'", "'\\''")
	cmd := fmt.Sprintf("tmux send-keys -t '=%s' '%s'", session, escaped)
	_, err := b.runRemote(10*time.Second, cmd)
	return err
}

func (b *SSHBackend) IsPaneDead(session string) (bool, error) {
	cmd := fmt.Sprintf("tmux display-message -t '=%s' -p '#{pane_dead}'", session)
	out, err := b.runRemote(10*time.Second, cmd)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

func (b *SSHBackend) SetPaneDiedHook(session, agentID string) error {
	escaped := strings.ReplaceAll(agentID, "'", "'\\''")
	sessionEscaped := strings.ReplaceAll(session, "'", "'\\''")
	hookCmd := fmt.Sprintf(`tmux set-hook -t '=%s' pane-died "run-shell \"gt log crash --agent '%s' --session '%s' --exit-code #{pane_dead_status}\""`,
		sessionEscaped, escaped, sessionEscaped)
	_, err := b.runRemote(10*time.Second, hookCmd)
	return err
}

// --- Coop-first stubs (return ErrNotSupported) ---

func (b *SSHBackend) KillSession(_ string) error                    { return ErrNotSupported }
func (b *SSHBackend) IsAgentRunning(_ string) (bool, error)         { return false, ErrNotSupported }
func (b *SSHBackend) GetAgentState(_ string) (string, error)        { return "", ErrNotSupported }
func (b *SSHBackend) SetEnvironment(_, _, _ string) error           { return ErrNotSupported }
func (b *SSHBackend) GetEnvironment(_, _ string) (string, error)    { return "", ErrNotSupported }
func (b *SSHBackend) GetPaneWorkDir(_ string) (string, error)       { return "", ErrNotSupported }
func (b *SSHBackend) SendInput(_ string, _ string, _ bool) error    { return ErrNotSupported }
func (b *SSHBackend) RespawnPane(_ string) error                    { return ErrNotSupported }
func (b *SSHBackend) SwitchSession(_ string, _ SwitchConfig) error  { return ErrNotSupported }
func (b *SSHBackend) AttachSession(_ string) error                  { return ErrNotSupported }
