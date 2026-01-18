package agent_test

import (
	"time"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/session"
)

// =============================================================================
// Test Stubs for Error Injection
//
// These stubs wrap pure doubles and allow injecting errors for testing
// error paths. This keeps the doubles as pure drop-in replacements.
// =============================================================================

// --- sessionsStub ---

// sessionsStub wraps a Sessions implementation and allows injecting errors.
// Use this for testing error paths that can't be triggered with session.Double.
type sessionsStub struct {
	session.Sessions

	// Inject errors for specific operations
	StartErr           error
	StopErr            error
	SendControlErr     error
	CaptureErr         error
	CaptureAllErr      error
	GetStartCommandErr error
	ListErr            error
}

func newSessionsStub(wrapped session.Sessions) *sessionsStub {
	return &sessionsStub{Sessions: wrapped}
}

func (s *sessionsStub) Start(name, workDir, command string) (session.SessionID, error) {
	if s.StartErr != nil {
		return "", s.StartErr
	}
	return s.Sessions.Start(name, workDir, command)
}

func (s *sessionsStub) Stop(id session.SessionID) error {
	if s.StopErr != nil {
		return s.StopErr
	}
	return s.Sessions.Stop(id)
}

func (s *sessionsStub) SendControl(id session.SessionID, key string) error {
	if s.SendControlErr != nil {
		return s.SendControlErr
	}
	return s.Sessions.SendControl(id, key)
}

// Delegate all other methods to the wrapped Sessions
func (s *sessionsStub) Exists(id session.SessionID) (bool, error) {
	return s.Sessions.Exists(id)
}

func (s *sessionsStub) Capture(id session.SessionID, lines int) (string, error) {
	if s.CaptureErr != nil {
		return "", s.CaptureErr
	}
	return s.Sessions.Capture(id, lines)
}

func (s *sessionsStub) Send(id session.SessionID, text string) error {
	return s.Sessions.Send(id, text)
}

func (s *sessionsStub) IsRunning(id session.SessionID, processNames ...string) bool {
	return s.Sessions.IsRunning(id, processNames...)
}

func (s *sessionsStub) WaitFor(id session.SessionID, timeout time.Duration, processNames ...string) error {
	return s.Sessions.WaitFor(id, timeout, processNames...)
}

func (s *sessionsStub) List() ([]session.SessionID, error) {
	if s.ListErr != nil {
		return nil, s.ListErr
	}
	return s.Sessions.List()
}

func (s *sessionsStub) CaptureAll(id session.SessionID) (string, error) {
	if s.CaptureAllErr != nil {
		return "", s.CaptureAllErr
	}
	return s.Sessions.CaptureAll(id)
}

func (s *sessionsStub) GetStartCommand(id session.SessionID) (string, error) {
	if s.GetStartCommandErr != nil {
		return "", s.GetStartCommandErr
	}
	return s.Sessions.GetStartCommand(id)
}

func (s *sessionsStub) GetInfo(id session.SessionID) (*session.Info, error) {
	return s.Sessions.GetInfo(id)
}

// --- agentsStub (alias for exported AgentsStub) ---

// newAgentsStub creates a new stub using the exported agent.AgentsStub.
// This keeps internal test code concise while using the shared implementation.
func newAgentsStub(wrapped agent.Agents) *agent.AgentsStub {
	return agent.NewAgentsStub(wrapped)
}
