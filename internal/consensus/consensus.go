// Package consensus implements fan-out prompt dispatch to multiple Claude Code
// sessions with parallel response collection. It sends the same prompt to
// several tmux sessions and captures their responses for comparison.
package consensus

import (
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
	WaitForIdle(session string, timeout time.Duration) error
	CapturePaneAll(session string) (string, error)
	WakePane(target string)
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
//  1. Pre-flight: filter to idle sessions
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

	// 1. Pre-flight: check which sessions are idle.
	type target struct {
		session     string
		preSnapshot string
	}
	var targets []target
	for _, sess := range req.Sessions {
		if !r.tmux.IsIdle(sess) {
			result.Sessions = append(result.Sessions, SessionResult{
				Session: sess,
				Status:  StatusNotIdle,
			})
			continue
		}

		// 2. Snapshot pane state before sending.
		pre, err := r.tmux.CapturePaneAll(sess)
		if err != nil {
			result.Sessions = append(result.Sessions, SessionResult{
				Session: sess,
				Status:  StatusError,
				Error:   err.Error(),
			})
			continue
		}

		targets = append(targets, target{session: sess, preSnapshot: pre})
	}

	// 3. Sequential send to all targets (SendKeys is fast).
	var sendable []target
	for _, t := range targets {
		if err := r.tmux.SendKeys(t.session, req.Prompt); err != nil {
			result.Sessions = append(result.Sessions, SessionResult{
				Session: t.session,
				Status:  StatusError,
				Error:   err.Error(),
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
			collected[idx] = collectOne(r.tmux, tgt.session, tgt.preSnapshot, req.Timeout)
		}(i, t)
	}
	wg.Wait()

	result.Sessions = append(result.Sessions, collected...)
	result.Duration = time.Since(start)
	return result
}
