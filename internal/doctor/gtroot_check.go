package doctor

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/tmux"
)

// GTRootCheck verifies that tmux sessions have GT_ROOT set.
// Sessions without GT_ROOT cannot find town-level formulas.
type GTRootCheck struct {
	BaseCheck
	tmux TmuxEnvGetter // nil means use real tmux
}

// TmuxEnvGetter abstracts tmux environment access for testing.
type TmuxEnvGetter interface {
	ListSessions() ([]string, error)
	GetEnvironment(session, key string) (string, error)
}

// realTmux wraps real tmux operations.
type realTmux struct {
	t *tmux.Tmux
}

func (r *realTmux) ListSessions() ([]string, error) {
	return r.t.ListSessions()
}

func (r *realTmux) GetEnvironment(session, key string) (string, error) {
	return r.t.GetEnvironment(session, key)
}

// NewGTRootCheck creates a new GT_ROOT check.
func NewGTRootCheck() *GTRootCheck {
	return &GTRootCheck{
		BaseCheck: BaseCheck{
			CheckName:        "gt-root-env",
			CheckDescription: "Verify sessions have GT_ROOT set for formula discovery",
		},
	}
}

// NewGTRootCheckWithTmux creates a check with a custom tmux interface (for testing).
func NewGTRootCheckWithTmux(t TmuxEnvGetter) *GTRootCheck {
	c := NewGTRootCheck()
	c.tmux = t
	return c
}

// Run checks GT_ROOT environment variable for all Gas Town sessions.
func (c *GTRootCheck) Run(ctx *CheckContext) *CheckResult {
	t := c.tmux
	if t == nil {
		t = &realTmux{t: tmux.NewTmux()}
	}

	sessions, err := t.ListSessions()
	if err != nil {
		// No tmux server - not an error, Gas Town might just be down
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No tmux sessions running",
		}
	}

	// Filter to Gas Town sessions (gt-* and hq-*)
	var gtSessions []string
	for _, sess := range sessions {
		if strings.HasPrefix(sess, "gt-") || strings.HasPrefix(sess, "hq-") {
			gtSessions = append(gtSessions, sess)
		}
	}

	if len(gtSessions) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "No Gas Town sessions running",
		}
	}

	var missingSessions []string
	var okCount int

	for _, sess := range gtSessions {
		gtRoot, err := t.GetEnvironment(sess, "GT_ROOT")
		if err != nil || gtRoot == "" {
			missingSessions = append(missingSessions, sess)
		} else {
			okCount++
		}
	}

	if len(missingSessions) == 0 {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: fmt.Sprintf("All %d session(s) have GT_ROOT set", okCount),
		}
	}

	details := make([]string, 0, len(missingSessions)+2)
	for _, sess := range missingSessions {
		details = append(details, fmt.Sprintf("Missing GT_ROOT: %s", sess))
	}
	details = append(details, "", "Sessions without GT_ROOT cannot find town-level formulas.")

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: fmt.Sprintf("%d session(s) missing GT_ROOT environment variable", len(missingSessions)),
		Details: details,
		FixHint: "Restart sessions to pick up GT_ROOT: gt shutdown && gt up",
	}
}
