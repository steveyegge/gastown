package daemon

import (
	"context"
	"fmt"
	"log"

	"github.com/steveyegge/gastown/internal/bus"
	"github.com/steveyegge/gastown/internal/orchestrator"
	"github.com/steveyegge/gastown/internal/tmux"
)

// OrchestratorPatrol manages the step orchestrator within the daemon heartbeat.
// It watches registered polecat sessions for STEP_COMPLETE markers and drives
// formula workflows forward without Claude involvement.
type OrchestratorPatrol struct {
	orchestrator *orchestrator.Orchestrator
	bus          bus.Bus
	logger       *log.Logger
}

// tmuxSessionReader implements orchestrator.SessionReader using tmux.
type tmuxSessionReader struct {
	t *tmux.Tmux
}

func (r *tmuxSessionReader) CaptureOutput(session string, lines int) (string, error) {
	return r.t.CapturePane(session, lines)
}

// tmuxSessionWriter implements orchestrator.SessionWriter using tmux send-keys.
type tmuxSessionWriter struct {
	t *tmux.Tmux
}

func (w *tmuxSessionWriter) SendStep(session string, instruction string) error {
	return w.t.SendKeys(session, instruction)
}

// NewOrchestratorPatrol creates the orchestrator patrol.
// If b is nil, a local in-process bus is used.
func NewOrchestratorPatrol(b bus.Bus, logger *log.Logger) *OrchestratorPatrol {
	if b == nil {
		b = bus.NewLocalBus()
	}

	t := tmux.NewTmux()
	cfg := orchestrator.DefaultConfig()
	orch := orchestrator.New(
		cfg,
		&tmuxSessionReader{t: t},
		&tmuxSessionWriter{t: t},
		nil, // StepProvider wired when formulas are registered
		nil, // TriageClient wired when Haiku API is configured
		b,
		logger,
	)

	return &OrchestratorPatrol{
		orchestrator: orch,
		bus:          b,
		logger:       logger,
	}
}

// Tick runs one orchestrator poll cycle. Called from daemon heartbeat.
func (p *OrchestratorPatrol) Tick(ctx context.Context) {
	p.orchestrator.Tick(ctx)
}

// Register adds a polecat session to the orchestrator.
func (p *OrchestratorPatrol) Register(polecat, rig, formulaName, startStepID string) {
	p.orchestrator.Register(polecat, rig, formulaName, startStepID)
	p.logger.Printf("orchestrator_patrol: registered %s (rig=%s formula=%s step=%s)",
		polecat, rig, formulaName, startStepID)
}

// Unregister removes a polecat session from the orchestrator.
func (p *OrchestratorPatrol) Unregister(polecat string) {
	p.orchestrator.Unregister(polecat)
}

// Bus returns the event bus for subscriber access.
func (p *OrchestratorPatrol) Bus() bus.Bus {
	return p.bus
}

// OrchestratorPatrolConfig holds configuration for the orchestrator patrol.
type OrchestratorPatrolConfig struct {
	// Enabled controls whether the orchestrator runs during heartbeat.
	Enabled bool `json:"enabled"`

	// RedisURL is the Redis connection URL for the distributed bus.
	// If empty, a local in-process bus is used.
	RedisURL string `json:"redis_url,omitempty"`
}

// ensureOrchestratorRunning is called from the daemon heartbeat.
// The orchestrator patrol is opt-in: disabled by default until
// formula workflows start using STEP_COMPLETE markers.
func (d *Daemon) ensureOrchestratorRunning() {
	if d.orchestratorPatrol == nil {
		d.orchestratorPatrol = NewOrchestratorPatrol(nil, d.logger)
		d.logger.Println("Orchestrator patrol initialized")
	}
	d.orchestratorPatrol.Tick(d.ctx)
}

// tickOrchestrator is a standalone function for use when the daemon doesn't
// directly own the patrol (e.g., CLI-driven orchestrator).
func tickOrchestrator(ctx context.Context, patrol *OrchestratorPatrol) error {
	if patrol == nil {
		return fmt.Errorf("orchestrator patrol not initialized")
	}
	patrol.Tick(ctx)
	return nil
}
