package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

// Swapper handles graceful session replacement during rate limit events.
type Swapper interface {
	// Swap terminates the old session and starts a new one with a different profile.
	Swap(ctx context.Context, req SwapRequest) (*SwapResult, error)
}

// DefaultSwapper implements session swapping using gt commands.
type DefaultSwapper struct {
	workDir string
	tmux    *tmux.Tmux
}

// NewSwapper creates a new session swapper.
func NewSwapper(workDir string) *DefaultSwapper {
	return &DefaultSwapper{
		workDir: workDir,
		tmux:    tmux.NewTmux(),
	}
}

// Swap performs the session swap operation.
func (s *DefaultSwapper) Swap(ctx context.Context, req SwapRequest) (*SwapResult, error) {
	result := &SwapResult{}

	// Step 1: Capture current hook state (should already be known from req.HookedWork)
	// This is a verification step to ensure we don't lose work
	if req.HookedWork == "" {
		// Try to get current hook
		hookOutput, err := util.ExecWithOutput(s.workDir, "gt", "hook", "show",
			"--rig", req.RigName, "--polecat", req.PolecatName)
		if err == nil && hookOutput != "" {
			req.HookedWork = hookOutput
		}
	}

	// Step 2: Gracefully terminate old session
	sessionName := fmt.Sprintf("gt-%s-%s", req.RigName, req.PolecatName)
	if running, _ := s.tmux.HasSession(sessionName); running {
		// Send Ctrl-C for graceful shutdown
		_ = s.tmux.SendKeysRaw(sessionName, "C-c")
		time.Sleep(200 * time.Millisecond)

		// Kill the session
		if err := s.tmux.KillSession(sessionName); err != nil {
			// Log but continue - session might already be dead
		}
	}

	// Step 3: Update polecat config with new profile
	// This is done via gt polecat profile set or by spawning with --profile
	// For now, we spawn with the profile flag

	// Step 4: Spawn new session with new profile
	err := util.ExecRun(s.workDir, "gt", "polecat", "spawn",
		req.PolecatName,
		"--rig", req.RigName,
		"--profile", req.NewProfile,
	)
	if err != nil {
		result.Error = fmt.Errorf("spawning new session: %w", err)
		return result, result.Error
	}

	// Step 5: Re-hook work if we had work hooked
	if req.HookedWork != "" {
		err := util.ExecRun(s.workDir, "gt", "hook", "attach",
			req.HookedWork,
			"--rig", req.RigName,
			"--polecat", req.PolecatName,
		)
		if err != nil {
			// Log but don't fail - work might already be hooked
		}
	}

	// Step 6: Nudge the new session to resume work
	nudgeMsg := fmt.Sprintf("Resuming after rate limit swap (was: %s, now: %s, reason: %s)",
		req.OldProfile, req.NewProfile, req.Reason)
	err = util.ExecRun(s.workDir, "gt", "nudge",
		"--rig", req.RigName,
		"--polecat", req.PolecatName,
		"-m", nudgeMsg,
	)
	if err != nil {
		// Non-fatal - session might pick up work automatically
	}

	result.Success = true
	result.NewSessionID = sessionName

	return result, nil
}
