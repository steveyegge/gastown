package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// PodConnection manages a local tmux session that bridges to a K8s pod's
// screen session via kubectl exec.
//
// Each K8s agent gets a tmux session named gt-<rig>-<name>. The session's
// pane runs "kubectl exec -it <pod> -n <ns> -- screen -x <screen-session>",
// which attaches to the screen session running Claude Code inside the pod.
//
// gt nudge and gt peek work unchanged because they address tmux sessions
// by name, and the terminal server creates sessions with the expected names.
type PodConnection struct {
	// AgentID is the agent identifier (e.g., "gastown/polecats/alpha").
	AgentID string

	// PodName is the K8s pod name (e.g., "gt-gastown-polecat-alpha").
	PodName string

	// Namespace is the K8s namespace (e.g., "gastown-test").
	Namespace string

	// SessionName is the local tmux session name (e.g., "gt-gastown-alpha").
	SessionName string

	// ScreenSession is the screen session name inside the pod (default "agent").
	ScreenSession string

	// KubeConfig is the path to kubeconfig. Empty means default.
	KubeConfig string

	// tmux wraps local tmux operations.
	tmux *tmux.Tmux

	// connected tracks whether the kubectl exec pipe is established.
	connected bool

	// lastConnected records when the connection was last established.
	lastConnected time.Time

	// reconnectCount tracks consecutive reconnection attempts.
	reconnectCount int

	// mu protects connection state.
	mu sync.Mutex
}

// PodConnectionConfig configures a PodConnection.
type PodConnectionConfig struct {
	AgentID       string
	PodName       string
	Namespace     string
	SessionName   string
	ScreenSession string
	KubeConfig    string
}

// DefaultScreenSession is the default screen session name inside pods.
const DefaultScreenSession = "agent"

// MaxReconnectAttempts is the maximum consecutive reconnection attempts
// before giving up until the next discovery cycle.
const MaxReconnectAttempts = 5

// NewPodConnection creates a PodConnection for a K8s agent pod.
func NewPodConnection(cfg PodConnectionConfig) *PodConnection {
	screenSession := cfg.ScreenSession
	if screenSession == "" {
		screenSession = DefaultScreenSession
	}
	return &PodConnection{
		AgentID:       cfg.AgentID,
		PodName:       cfg.PodName,
		Namespace:     cfg.Namespace,
		SessionName:   cfg.SessionName,
		ScreenSession: screenSession,
		KubeConfig:    cfg.KubeConfig,
		tmux:          tmux.NewTmux(),
	}
}

// Open creates the local tmux session and runs kubectl exec to attach to the
// pod's screen session.
//
// The flow is:
//  1. Create a tmux session named SessionName
//  2. Set remain-on-exit so we can detect when kubectl exec drops
//  3. Run: kubectl exec -it <pod> -n <ns> -- screen -x <screen-session>
//  4. The local tmux pane now mirrors the pod's screen session
func (pc *PodConnection) Open(ctx context.Context) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Kill any stale session with this name
	if has, _ := pc.tmux.HasSession(pc.SessionName); has {
		_ = pc.tmux.KillSessionWithProcesses(pc.SessionName)
	}

	// Build the kubectl exec command
	kubectlCmd := pc.kubectlExecCommand()

	// Create tmux session with the kubectl exec command
	if err := pc.tmux.NewSessionWithCommand(pc.SessionName, "/tmp", kubectlCmd); err != nil {
		return fmt.Errorf("creating tmux session %s: %w", pc.SessionName, err)
	}

	// Set remain-on-exit so we can detect disconnections
	if err := pc.tmux.SetRemainOnExit(pc.SessionName, true); err != nil {
		slog.Warn("failed to set remain-on-exit", "session", pc.SessionName, "err", err)
	}

	pc.connected = true
	pc.lastConnected = time.Now()
	pc.reconnectCount = 0

	slog.Info("pod connection opened",
		"agent", pc.AgentID,
		"pod", pc.PodName,
		"session", pc.SessionName,
	)

	return nil
}

// Close kills the local tmux session, severing the kubectl exec pipe.
func (pc *PodConnection) Close() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.connected = false

	if has, _ := pc.tmux.HasSession(pc.SessionName); !has {
		return nil // Already gone
	}

	if err := pc.tmux.KillSessionWithProcesses(pc.SessionName); err != nil {
		return fmt.Errorf("killing tmux session %s: %w", pc.SessionName, err)
	}

	slog.Info("pod connection closed",
		"agent", pc.AgentID,
		"session", pc.SessionName,
	)

	return nil
}

// IsAlive checks if the kubectl exec connection is still active.
// It does this by checking if the tmux pane is dead (which means kubectl exec exited).
func (pc *PodConnection) IsAlive() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if !pc.connected {
		return false
	}

	has, err := pc.tmux.HasSession(pc.SessionName)
	if err != nil || !has {
		pc.connected = false
		return false
	}

	dead, err := pc.tmux.IsPaneDead(pc.SessionName)
	if err != nil {
		return false
	}

	if dead {
		pc.connected = false
		return false
	}

	return true
}

// Reconnect re-establishes the kubectl exec connection after a drop.
// Screen inside the pod preserves agent state, so reconnection is seamless.
func (pc *PodConnection) Reconnect(ctx context.Context) error {
	pc.mu.Lock()
	reconnectCount := pc.reconnectCount
	pc.mu.Unlock()

	if reconnectCount >= MaxReconnectAttempts {
		return fmt.Errorf("max reconnect attempts (%d) exceeded for %s", MaxReconnectAttempts, pc.SessionName)
	}

	// Apply exponential backoff: 0s, 2s, 4s, 8s, 16s
	if reconnectCount > 0 {
		delay := time.Duration(1<<uint(reconnectCount-1)) * 2 * time.Second
		slog.Info("reconnect backoff",
			"agent", pc.AgentID,
			"attempt", reconnectCount+1,
			"delay", delay,
		)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	pc.mu.Lock()
	pc.reconnectCount++
	pc.mu.Unlock()

	// Kill old session and reconnect
	_ = pc.Close()
	return pc.Open(ctx)
}

// SendKeys sends keystrokes to the local tmux session, which flows through
// the kubectl exec pipe into the pod's screen session.
func (pc *PodConnection) SendKeys(text string) error {
	return pc.tmux.SendKeys(pc.SessionName, text)
}

// Capture reads the last N lines from the local tmux pane, which mirrors
// the pod's screen session output.
func (pc *PodConnection) Capture(lines int) (string, error) {
	return pc.tmux.CapturePane(pc.SessionName, lines)
}

// IsConnected returns the current connection state.
func (pc *PodConnection) IsConnected() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.connected
}

// ReconnectCount returns the number of consecutive reconnection attempts.
func (pc *PodConnection) ReconnectCount() int {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.reconnectCount
}

// kubectlExecCommand builds the kubectl exec command string for attaching
// to the pod's screen session.
func (pc *PodConnection) kubectlExecCommand() string {
	args := "kubectl exec -it"
	if pc.Namespace != "" {
		args += " -n " + pc.Namespace
	}
	args += " " + pc.PodName
	if pc.KubeConfig != "" {
		args = "kubectl --kubeconfig " + pc.KubeConfig + " exec -it"
		if pc.Namespace != "" {
			args += " -n " + pc.Namespace
		}
		args += " " + pc.PodName
	}
	args += " -- screen -x " + pc.ScreenSession
	return args
}
