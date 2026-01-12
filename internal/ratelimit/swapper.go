package ratelimit

import (
	"context"
	"fmt"
)

// Swapper handles graceful session replacement during rate limit events.
type Swapper interface {
	// Swap terminates the old session and starts a new one with a different profile.
	Swap(ctx context.Context, req SwapRequest) (*SwapResult, error)
}

// DefaultSwapper implements session swapping using SessionOps.
type DefaultSwapper struct {
	ops SessionOps
}

// NewSwapper creates a new session swapper with the given session operations.
func NewSwapper(ops SessionOps) *DefaultSwapper {
	return &DefaultSwapper{
		ops: ops,
	}
}

// Swap performs the session swap operation.
func (s *DefaultSwapper) Swap(ctx context.Context, req SwapRequest) (*SwapResult, error) {
	result := &SwapResult{}

	// Step 1: Capture current hook state if not provided
	if req.HookedWork == "" {
		hookedWork, err := s.ops.GetHookedWork(req.RigName, req.PolecatName)
		if err == nil && hookedWork != "" {
			req.HookedWork = hookedWork
		}
	}

	// Step 2: Stop the old session
	running, _ := s.ops.IsRunning(req.RigName, req.PolecatName)
	if running {
		if err := s.ops.Stop(req.RigName, req.PolecatName, false); err != nil {
			result.Error = fmt.Errorf("stopping old session: %w", err)
			return result, result.Error
		}
	}

	// Step 3: Start new session with new profile
	sessionID, err := s.ops.Start(req.RigName, req.PolecatName, req.NewProfile)
	if err != nil {
		result.Error = fmt.Errorf("starting new session: %w", err)
		return result, result.Error
	}

	// Step 4: Re-hook work if we had work hooked
	if req.HookedWork != "" {
		if err := s.ops.HookWork(req.RigName, req.PolecatName, req.HookedWork); err != nil {
			// Log but don't fail - work might already be hooked
		}
	}

	// Step 5: Nudge the new session to resume work
	nudgeMsg := fmt.Sprintf("Resuming after rate limit swap (was: %s, now: %s, reason: %s)",
		req.OldProfile, req.NewProfile, req.Reason)
	if err := s.ops.Nudge(req.RigName, req.PolecatName, nudgeMsg); err != nil {
		// Non-fatal - session might pick up work automatically
	}

	result.Success = true
	result.NewSessionID = sessionID

	return result, nil
}
