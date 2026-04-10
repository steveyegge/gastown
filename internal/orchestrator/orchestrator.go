package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/steveyegge/gastown/internal/bus"
)

// SessionReader reads output from a polecat tmux session.
type SessionReader interface {
	// CaptureOutput returns the last N lines of a polecat's tmux pane.
	CaptureOutput(session string, lines int) (string, error)
}

// SessionWriter sends commands to a polecat tmux session.
type SessionWriter interface {
	// SendStep sends a formula step instruction to the polecat.
	SendStep(session string, instruction string) error
}

// StepProvider resolves the next step in a formula workflow.
type StepProvider interface {
	// NextStep returns the next step ID and instruction after the given step.
	// Returns ("", "", nil) when there are no more steps.
	NextStep(formulaName, currentStepID string) (stepID string, instruction string, err error)
}

// Config holds orchestrator configuration.
type Config struct {
	// PollInterval is how often to check tmux output for markers.
	PollInterval time.Duration

	// CaptureLines is how many tmux lines to capture per check.
	CaptureLines int

	// MaxRetries is how many times to retry a failed step.
	MaxRetries int

	// PromptMark is the shell prompt marker to find the start of output.
	PromptMark string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval: 5 * time.Second,
		CaptureLines: 200,
		MaxRetries:   3,
		PromptMark:   "❯",
	}
}

// Session tracks the orchestration state of a single polecat.
type Session struct {
	// Polecat is the polecat name (tmux session).
	Polecat string

	// Rig is the rig this polecat belongs to.
	Rig string

	// FormulaName is the formula being executed.
	FormulaName string

	// CurrentStepID is the step the polecat is currently executing.
	CurrentStepID string

	// Attempt is the current retry attempt for this step.
	Attempt int

	// LastOutput is the most recently captured output (for dedup).
	LastOutput string

	// LastCheck is when we last polled this session.
	LastCheck time.Time
}

// Orchestrator watches polecat sessions and drives formula workflows.
type Orchestrator struct {
	config   Config
	matcher  *Matcher
	router   *Router
	reader   SessionReader
	writer   SessionWriter
	steps    StepProvider
	triage   TriageClient
	bus      bus.Bus
	logger   *log.Logger
	sessions map[string]*Session // keyed by polecat name
}

// New creates an Orchestrator.
func New(cfg Config, reader SessionReader, writer SessionWriter, steps StepProvider, triage TriageClient, b bus.Bus, logger *log.Logger) *Orchestrator {
	return &Orchestrator{
		config:   cfg,
		matcher:  NewMatcher(),
		router:   &Router{MaxRetries: cfg.MaxRetries},
		reader:   reader,
		writer:   writer,
		steps:    steps,
		triage:   triage,
		bus:      b,
		logger:   logger,
		sessions: make(map[string]*Session),
	}
}

// Register adds a polecat session to be orchestrated.
func (o *Orchestrator) Register(polecat, rig, formulaName, startStepID string) {
	o.sessions[polecat] = &Session{
		Polecat:       polecat,
		Rig:           rig,
		FormulaName:   formulaName,
		CurrentStepID: startStepID,
		Attempt:       1,
	}
}

// Unregister removes a polecat session from orchestration.
func (o *Orchestrator) Unregister(polecat string) {
	delete(o.sessions, polecat)
}

// Tick runs one poll cycle across all registered sessions.
// Called from the daemon heartbeat.
func (o *Orchestrator) Tick(ctx context.Context) {
	for name, sess := range o.sessions {
		if err := o.checkSession(ctx, name, sess); err != nil {
			o.logger.Printf("orchestrator: error checking %s: %v", name, err)
		}
	}
}

func (o *Orchestrator) checkSession(ctx context.Context, name string, sess *Session) error {
	output, err := o.reader.CaptureOutput(name, o.config.CaptureLines)
	if err != nil {
		return fmt.Errorf("capturing output: %w", err)
	}

	// Dedup: skip if output hasn't changed.
	if output == sess.LastOutput {
		return nil
	}
	sess.LastOutput = output
	sess.LastCheck = time.Now()

	// Detect STEP_COMPLETE marker.
	marker := DetectStepComplete(output)
	if marker == nil {
		return nil // No marker yet — polecat still working.
	}

	// Extract the relevant output body.
	body := marker.Body

	// Pattern match the outcome.
	match := o.matcher.Match(body)

	// Check if there's a next step.
	nextID, nextInstruction, err := o.steps.NextStep(sess.FormulaName, marker.StepID)
	if err != nil {
		return fmt.Errorf("resolving next step: %w", err)
	}
	hasNext := nextID != ""

	// Route.
	decision := o.router.RouteWithAttempt(match, hasNext, sess.Attempt)

	o.logger.Printf("orchestrator: %s step=%s outcome=%s action=%s",
		name, marker.StepID, match.Outcome, decision.Action)

	return o.execute(ctx, sess, decision, match, marker.StepID, nextID, nextInstruction, body)
}

func (o *Orchestrator) execute(ctx context.Context, sess *Session, decision Decision, match MatchResult, stepID, nextID, nextInstruction, body string) error {
	switch decision.Action {
	case ActionAdvance:
		sess.CurrentStepID = nextID
		sess.Attempt = 1
		o.publishEvent(sess, stepID, bus.StepAdvanced, "")
		return o.writer.SendStep(sess.Polecat, nextInstruction)

	case ActionComplete:
		o.publishEvent(sess, stepID, bus.StepCompleted, "")
		o.Unregister(sess.Polecat)
		return nil

	case ActionRetry:
		sess.Attempt++
		o.publishEvent(sess, stepID, bus.StepRetried, string(match.Outcome))
		retryMsg := fmt.Sprintf("The previous attempt failed (%s). Please retry step %q.", match.Outcome, stepID)
		return o.writer.SendStep(sess.Polecat, retryMsg)

	case ActionTriage:
		return o.handleTriage(ctx, sess, stepID, nextID, nextInstruction, body)

	case ActionEscalate:
		o.publishEvent(sess, stepID, bus.StepEscalated, decision.Reason)
		o.logger.Printf("orchestrator: ESCALATE %s step=%s reason=%s", sess.Polecat, stepID, decision.Reason)
		return nil

	default:
		return fmt.Errorf("unknown action: %s", decision.Action)
	}
}

func (o *Orchestrator) handleTriage(ctx context.Context, sess *Session, stepID, nextID, nextInstruction, body string) error {
	if o.triage == nil {
		o.publishEvent(sess, stepID, bus.StepEscalated, "no triage client configured")
		return nil
	}

	result, err := o.triage.Triage(ctx, body, stepID)
	if err != nil {
		o.logger.Printf("orchestrator: triage error for %s: %v", sess.Polecat, err)
		o.publishEvent(sess, stepID, bus.StepEscalated, "triage error: "+err.Error())
		return nil
	}

	o.publishEvent(sess, stepID, bus.StepTriaged, string(result.Verdict)+": "+result.Reason)

	action := result.ToAction()
	hasNext := nextID != ""

	switch action {
	case ActionAdvance:
		if hasNext {
			sess.CurrentStepID = nextID
			sess.Attempt = 1
			return o.writer.SendStep(sess.Polecat, nextInstruction)
		}
		o.publishEvent(sess, stepID, bus.StepCompleted, "triage: success, no more steps")
		o.Unregister(sess.Polecat)
		return nil

	case ActionRetry:
		sess.Attempt++
		retryMsg := fmt.Sprintf("Triage determined failure (%s). Please retry step %q.", result.Reason, stepID)
		return o.writer.SendStep(sess.Polecat, retryMsg)

	default: // Escalate
		o.publishEvent(sess, stepID, bus.StepEscalated, "Haiku unsure: "+result.Reason)
		return nil
	}
}

func (o *Orchestrator) publishEvent(sess *Session, stepID string, eventType bus.StepEventType, detail string) {
	if o.bus == nil {
		return
	}
	ev := bus.NewStepEvent(sess.Rig, sess.Polecat, stepID, eventType)
	ev.Detail = detail
	ev.Formula = sess.FormulaName
	if err := o.bus.Publish(ev); err != nil {
		o.logger.Printf("orchestrator: bus publish error: %v", err)
	}
}
