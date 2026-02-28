// Package consensus implements fan-out prompt dispatch to multiple AI agent
// sessions with parallel response collection. It sends the same prompt to
// several tmux sessions and captures their responses for comparison.
//
// Provider-aware: each session's GT_AGENT env var is used to look up the
// agent preset, enabling idle detection for Claude, Gemini, Codex, and
// other providers.
package consensus

import (
	"fmt"
	"sync"
	"time"
)

// ResultStatus describes the outcome of a single session's response.
type ResultStatus string

const (
	StatusOK          ResultStatus = "ok"
	StatusTimeout     ResultStatus = "timeout"
	StatusError       ResultStatus = "error"
	StatusRateLimited ResultStatus = "rate_limited"
	StatusNotIdle     ResultStatus = "not_idle"
)

var (
	errIdleTimeout = fmt.Errorf("idle timeout exceeded")
	errNoDetection = fmt.Errorf("no idle detection method available for provider")
)

// Request specifies what to send and where.
type Request struct {
	Prompt   string
	Sessions []string      // tmux session names
	Timeout  time.Duration // per-session wait timeout
}

// SessionResult holds the outcome for a single session.
type SessionResult struct {
	Session  string        `json:"session"`
	Status   ResultStatus  `json:"status"`
	Response string        `json:"response,omitempty"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
	Provider string        `json:"provider,omitempty"`
}

// Result aggregates all session outcomes.
type Result struct {
	Prompt   string          `json:"prompt"`
	Sessions []SessionResult `json:"sessions"`
	Duration time.Duration   `json:"duration"`
}

// TmuxClient is the interface for tmux operations needed by the consensus runner.
type TmuxClient interface {
	IsIdle(session string) bool
	SendKeys(session, keys string) error
	CapturePaneAll(session string) (string, error)
	CapturePaneLines(session string, n int) ([]string, error)
	WakePane(target string)
	GetEnvironment(session, key string) (string, error)
}

// Runner orchestrates fan-out prompt dispatch and response collection.
type Runner struct {
	tmux TmuxClient
}

// NewRunner creates a Runner with the given tmux client.
func NewRunner(tmux TmuxClient) *Runner {
	return &Runner{tmux: tmux}
}

// Run sends a prompt to all requested sessions and collects responses.
//
// Strategy:
//  1. Pre-flight: resolve provider per session, filter to idle sessions
//  2. Snapshot pane state before sending (for diff)
//  3. Sequential send via SendKeys (fast, avoids interleave)
//  4. Parallel wait via goroutines (the slow part)
//  5. Capture + diff responses
func (r *Runner) Run(req Request) *Result {
	start := time.Now()
	result := &Result{
		Prompt: req.Prompt,
	}

	if len(req.Sessions) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	// 1. Pre-flight: resolve provider and check idle status.
	type target struct {
		session     string
		preSnapshot string
		provider    ProviderInfo
	}
	var targets []target
	for _, sess := range req.Sessions {
		provider := resolveProvider(r.tmux, sess)

		if !isSessionIdle(r.tmux, sess, provider) {
			result.Sessions = append(result.Sessions, SessionResult{
				Session:  sess,
				Status:   StatusNotIdle,
				Provider: provider.Name,
			})
			continue
		}

		// 2. Snapshot pane state before sending.
		pre, err := r.tmux.CapturePaneAll(sess)
		if err != nil {
			result.Sessions = append(result.Sessions, SessionResult{
				Session:  sess,
				Status:   StatusError,
				Error:    err.Error(),
				Provider: provider.Name,
			})
			continue
		}

		targets = append(targets, target{session: sess, preSnapshot: pre, provider: provider})
	}

	// 3. Sequential send to all targets (SendKeys is fast).
	var sendable []target
	for _, t := range targets {
		if err := r.tmux.SendKeys(t.session, req.Prompt); err != nil {
			result.Sessions = append(result.Sessions, SessionResult{
				Session:  t.session,
				Status:   StatusError,
				Error:    err.Error(),
				Provider: t.provider.Name,
			})
			continue
		}
		sendable = append(sendable, t)
	}

	// 4. Parallel wait + capture (the slow part).
	collected := make([]SessionResult, len(sendable))
	var wg sync.WaitGroup
	for i, t := range sendable {
		wg.Add(1)
		go func(idx int, tgt target) {
			defer wg.Done()
			collected[idx] = collectOne(r.tmux, tgt.session, tgt.preSnapshot, tgt.provider, req.Timeout)
		}(i, t)
	}
	wg.Wait()

	result.Sessions = append(result.Sessions, collected...)
	result.Duration = time.Since(start)
	return result
}
