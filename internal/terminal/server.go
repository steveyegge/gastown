package terminal

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// ServerConfig configures a terminal server instance.
type ServerConfig struct {
	// Rig is the rig name (e.g., "gastown").
	Rig string

	// Namespace is the K8s namespace (e.g., "gastown-test").
	Namespace string

	// KubeConfig is the path to kubeconfig. Empty means default.
	KubeConfig string

	// PollInterval is the beads discovery polling interval.
	PollInterval time.Duration

	// HealthInterval is the connection health check interval.
	HealthInterval time.Duration

	// ScreenSession is the screen session name inside pods.
	ScreenSession string

	// PodSource provides pod information. If nil, CLIPodSource is used.
	PodSource PodSource
}

// DefaultHealthInterval is the default health check interval.
const DefaultHealthInterval = 5 * time.Second

// Server manages connections to K8s agent pods.
//
// It discovers pods via beads polling, creates local tmux sessions that pipe
// to pod screen sessions via kubectl exec, monitors connection health, and
// reconnects on failure. Existing gt commands (nudge, peek) work unchanged
// because the terminal server creates tmux sessions with the expected names.
type Server struct {
	rig            string
	namespace      string
	kubeconfig     string
	screenSession  string
	healthInterval time.Duration

	inventory   *PodInventory
	connections map[string]*PodConnection // agentID → connection
	mu          sync.RWMutex
	tmux        *tmux.Tmux
}

// NewServer creates a terminal server with the given configuration.
func NewServer(cfg ServerConfig) *Server {
	healthInterval := cfg.HealthInterval
	if healthInterval == 0 {
		healthInterval = DefaultHealthInterval
	}
	screenSession := cfg.ScreenSession
	if screenSession == "" {
		screenSession = DefaultScreenSession
	}

	s := &Server{
		rig:            cfg.Rig,
		namespace:      cfg.Namespace,
		kubeconfig:     cfg.KubeConfig,
		screenSession:  screenSession,
		healthInterval: healthInterval,
		connections:    make(map[string]*PodConnection),
		tmux:           tmux.NewTmux(),
	}

	// Set up pod source
	source := cfg.PodSource
	if source == nil {
		source = &CLIPodSource{Rig: cfg.Rig}
	}

	s.inventory = NewPodInventory(PodInventoryConfig{
		Source:       source,
		PollInterval: cfg.PollInterval,
		OnChange:     s.handlePodEvent,
	})

	return s
}

// Run starts the terminal server main loop. It blocks until the context is
// cancelled, then performs graceful shutdown.
//
// The main loop runs two concurrent goroutines:
//  1. Discovery: polls beads for pod changes, emits PodEvents
//  2. Health monitor: checks connection liveness, triggers reconnection
func (s *Server) Run(ctx context.Context) error {
	slog.Info("terminal server starting",
		"rig", s.rig,
		"namespace", s.namespace,
		"health_interval", s.healthInterval,
	)

	var wg sync.WaitGroup

	// Start discovery goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.inventory.Watch(ctx); err != nil && ctx.Err() == nil {
			slog.Error("discovery loop failed", "err", err)
		}
	}()

	// Start health monitor goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.runHealthMonitor(ctx)
	}()

	// Wait for context cancellation
	<-ctx.Done()

	slog.Info("terminal server shutting down")

	// Graceful shutdown: close all connections
	s.shutdown()

	// Wait for goroutines to finish
	wg.Wait()

	slog.Info("terminal server stopped")
	return nil
}

// handlePodEvent is called by PodInventory when pod state changes.
// It reconciles the connection state to match the desired pod state.
func (s *Server) handlePodEvent(event PodEvent) {
	switch event.Type {
	case PodAdded:
		s.connectPod(event.Pod)
	case PodRemoved:
		s.disconnectPod(event.Pod.AgentID)
	case PodUpdated:
		// Pod changed (e.g., new pod name after restart) — reconnect
		s.disconnectPod(event.Pod.AgentID)
		s.connectPod(event.Pod)
	}
}

// connectPod creates a PodConnection and opens the tmux session.
func (s *Server) connectPod(pod *PodInfo) {
	sessionName := s.sessionNameForAgent(pod.AgentID)

	slog.Info("connecting to pod",
		"agent", pod.AgentID,
		"pod", pod.PodName,
		"session", sessionName,
	)

	pc := NewPodConnection(PodConnectionConfig{
		AgentID:       pod.AgentID,
		PodName:       pod.PodName,
		Namespace:     s.namespace,
		SessionName:   sessionName,
		ScreenSession: s.screenSession,
		KubeConfig:    s.kubeconfig,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := pc.Open(ctx); err != nil {
		slog.Error("failed to connect to pod",
			"agent", pod.AgentID,
			"pod", pod.PodName,
			"err", err,
		)
		return
	}

	s.mu.Lock()
	s.connections[pod.AgentID] = pc
	s.mu.Unlock()

	slog.Info("connected to pod",
		"agent", pod.AgentID,
		"pod", pod.PodName,
		"session", sessionName,
	)
}

// disconnectPod closes and removes a PodConnection.
func (s *Server) disconnectPod(agentID string) {
	s.mu.Lock()
	pc, exists := s.connections[agentID]
	if exists {
		delete(s.connections, agentID)
	}
	s.mu.Unlock()

	if !exists {
		return
	}

	if err := pc.Close(); err != nil {
		slog.Warn("error closing pod connection",
			"agent", agentID,
			"err", err,
		)
	} else {
		slog.Info("disconnected from pod", "agent", agentID)
	}
}

// runHealthMonitor periodically checks all connections and reconnects dead ones.
func (s *Server) runHealthMonitor(ctx context.Context) {
	ticker := time.NewTicker(s.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkHealth(ctx)
		}
	}
}

// checkHealth checks all connections and triggers reconnection for dead ones.
func (s *Server) checkHealth(ctx context.Context) {
	s.mu.RLock()
	// Snapshot connection list under read lock
	conns := make(map[string]*PodConnection, len(s.connections))
	for id, pc := range s.connections {
		conns[id] = pc
	}
	s.mu.RUnlock()

	for agentID, pc := range conns {
		if pc.IsAlive() {
			continue
		}

		slog.Warn("connection dead, attempting reconnect",
			"agent", agentID,
			"reconnect_count", pc.ReconnectCount(),
		)

		if err := pc.Reconnect(ctx); err != nil {
			slog.Error("reconnect failed",
				"agent", agentID,
				"err", err,
			)
			// If max reconnect attempts exceeded, remove the connection.
			// It will be re-created on the next discovery cycle if the pod is still alive.
			if pc.ReconnectCount() >= MaxReconnectAttempts {
				slog.Warn("max reconnect attempts reached, removing connection",
					"agent", agentID,
				)
				s.mu.Lock()
				delete(s.connections, agentID)
				s.mu.Unlock()
			}
		} else {
			slog.Info("reconnected successfully", "agent", agentID)
		}
	}
}

// shutdown closes all connections gracefully.
func (s *Server) shutdown() {
	s.mu.Lock()
	conns := make(map[string]*PodConnection, len(s.connections))
	for id, pc := range s.connections {
		conns[id] = pc
	}
	s.connections = make(map[string]*PodConnection)
	s.mu.Unlock()

	for agentID, pc := range conns {
		if err := pc.Close(); err != nil {
			slog.Warn("error closing connection during shutdown",
				"agent", agentID,
				"err", err,
			)
		}
	}

	slog.Info("all connections closed", "count", len(conns))
}

// sessionNameForAgent derives the tmux session name from an agent ID.
//
// Agent IDs follow patterns like:
//   - "gastown/polecats/alpha" → "gt-gastown-alpha"
//   - "gastown/witness" → "gt-gastown-witness"
//   - "gastown/crew/k8s" → "gt-gastown-crew-k8s"
func (s *Server) sessionNameForAgent(agentID string) string {
	// Use the session naming utilities from the session package.
	// However, to avoid a circular dependency and keep it simple,
	// we parse the agent ID directly.
	return agentIDToSessionName(agentID)
}

// agentIDToSessionName converts an agent ID to a tmux session name.
// This mirrors the naming convention in internal/session/names.go.
func agentIDToSessionName(agentID string) string {
	// Split agent ID: "rig/role/name" or "rig/role"
	parts := splitAgentID(agentID)
	if len(parts) < 2 {
		return "gt-" + agentID
	}

	rig := parts[0]

	switch {
	case len(parts) == 2:
		// "rig/witness", "rig/refinery"
		return fmt.Sprintf("gt-%s-%s", rig, parts[1])
	case parts[1] == "polecats" && len(parts) == 3:
		// "rig/polecats/name" → "gt-rig-name"
		return fmt.Sprintf("gt-%s-%s", rig, parts[2])
	case parts[1] == "crew" && len(parts) == 3:
		// "rig/crew/name" → "gt-rig-crew-name"
		return fmt.Sprintf("gt-%s-crew-%s", rig, parts[2])
	default:
		// Fallback: join with dashes
		return "gt-" + joinDash(parts)
	}
}

// splitAgentID splits an agent ID on "/" separators.
func splitAgentID(agentID string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(agentID); i++ {
		if agentID[i] == '/' {
			if i > start {
				parts = append(parts, agentID[start:i])
			}
			start = i + 1
		}
	}
	if start < len(agentID) {
		parts = append(parts, agentID[start:])
	}
	return parts
}

// joinDash joins strings with "-".
func joinDash(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "-"
		}
		result += p
	}
	return result
}

// Status returns the current server status.
func (s *Server) Status() ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conns := make([]ConnectionStatus, 0, len(s.connections))
	for agentID, pc := range s.connections {
		conns = append(conns, ConnectionStatus{
			AgentID:        agentID,
			PodName:        pc.PodName,
			SessionName:    pc.SessionName,
			Connected:      pc.IsConnected(),
			ReconnectCount: pc.ReconnectCount(),
		})
	}

	return ServerStatus{
		Rig:         s.rig,
		Namespace:   s.namespace,
		Connections: conns,
		PodCount:    s.inventory.Count(),
	}
}

// ServerStatus represents the current state of the terminal server.
type ServerStatus struct {
	Rig         string
	Namespace   string
	Connections []ConnectionStatus
	PodCount    int
}

// ConnectionStatus represents the state of a single pod connection.
type ConnectionStatus struct {
	AgentID        string
	PodName        string
	SessionName    string
	Connected      bool
	ReconnectCount int
}
