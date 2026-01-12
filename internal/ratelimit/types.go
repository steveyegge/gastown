package ratelimit

import "time"

// ExitCodeRateLimit is the exit code Claude Code uses for rate limits.
const ExitCodeRateLimit = 2

// RateLimitEvent represents a detected rate limit occurrence.
type RateLimitEvent struct {
	AgentID      string    `json:"agent_id"`
	Profile      string    `json:"profile"`
	Timestamp    time.Time `json:"timestamp"`
	ExitCode     int       `json:"exit_code"`
	ErrorSnippet string    `json:"error_snippet,omitempty"`
	Provider     string    `json:"provider,omitempty"`
}

// RolePolicy defines the fallback behavior for a role.
type RolePolicy struct {
	FallbackChain   []string `json:"fallback_chain"`   // Profile names in priority order
	CooldownMinutes int      `json:"cooldown_minutes"` // How long to wait after rate limit
	Stickiness      string   `json:"stickiness"`       // Preferred provider (optional)
}

// SwapRequest contains the parameters for a session swap.
type SwapRequest struct {
	RigName     string `json:"rig_name"`
	PolecatName string `json:"polecat_name"`
	OldProfile  string `json:"old_profile"`
	NewProfile  string `json:"new_profile"`
	HookedWork  string `json:"hooked_work"` // Bead ID
	Reason      string `json:"reason"`      // "rate_limit", "stuck", "manual"
}

// SwapResult contains the result of a session swap.
type SwapResult struct {
	Success      bool   `json:"success"`
	NewSessionID string `json:"new_session_id,omitempty"`
	Error        error  `json:"-"`
}
