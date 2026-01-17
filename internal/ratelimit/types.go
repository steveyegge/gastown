// Package ratelimit provides intelligent instance swapping for rate-limit aware operation.
// It supports provider/auth profiles, fallback chains, cooldown management, and
// automatic instance swapping when rate limits are detected.
//
// See GitHub Issue #232 for the full specification.
package ratelimit

import (
	"time"
)

// InstanceProfile defines how to launch an agent session with a specific provider/account.
// Multiple profiles enable switching between providers or accounts when rate-limited.
type InstanceProfile struct {
	// Name is the unique identifier for this profile (e.g., "anthropic_opus_acctA").
	Name string `json:"name"`

	// Provider is the LLM provider: "anthropic", "z-ai", "minimax", "openai", etc.
	Provider string `json:"provider"`

	// AuthRef references an account configuration in mayor/accounts.json.
	// For multi-account setups, this maps to different API keys/configs.
	AuthRef string `json:"auth_ref,omitempty"`

	// ModelMain is the primary model to use (e.g., "opus-4.5", "claude-sonnet").
	ModelMain string `json:"model_main,omitempty"`

	// ModelFast is the fast model for quick operations (optional).
	ModelFast string `json:"model_fast,omitempty"`

	// HarnessCommand overrides the default agent startup command.
	// Supports variables: {town_root}, {rig_name}, {role}.
	HarnessCommand string `json:"harness_command,omitempty"`

	// EnvVars are additional environment variables to set.
	EnvVars map[string]string `json:"env_vars,omitempty"`

	// SkillPack is an optional skill pack to preload for this profile.
	SkillPack string `json:"skill_pack,omitempty"`
}

// RolePolicy defines the rate-limit response behavior for a specific role.
// It includes fallback chains, stickiness preferences, and cooldown settings.
type RolePolicy struct {
	// Role is the role this policy applies to (e.g., "deacon", "witness", "polecat").
	Role string `json:"role"`

	// FallbackChain is an ordered list of profile names to try on rate-limit.
	// The first available profile (not in cooldown) will be selected.
	FallbackChain []string `json:"fallback_chain"`

	// Stickiness defines provider preferences (e.g., "prefer anthropic" means
	// only fail over to non-Anthropic profiles when all Anthropic profiles are cooling).
	Stickiness *StickinessConfig `json:"stickiness,omitempty"`

	// CooldownMinutes is how long a profile stays in cooldown after rate-limiting.
	// During cooldown, the profile is skipped in the fallback chain.
	CooldownMinutes int `json:"cooldown_minutes"`

	// TransitionRules define special behaviors when swapping between specific profiles.
	TransitionRules []TransitionRule `json:"transition_rules,omitempty"`
}

// StickinessConfig defines provider preference for a role.
type StickinessConfig struct {
	// PreferProvider is the preferred provider (e.g., "anthropic").
	// The role will try all profiles from this provider before others.
	PreferProvider string `json:"prefer_provider,omitempty"`

	// OnlyFailoverIfAllCooling means only use non-preferred providers
	// when ALL preferred provider profiles are in cooldown.
	OnlyFailoverIfAllCooling bool `json:"only_failover_if_all_cooling,omitempty"`
}

// TransitionRule defines special behavior when swapping between specific profiles.
type TransitionRule struct {
	// FromProfile is the source profile name (or "*" for any).
	FromProfile string `json:"from,omitempty"`

	// ToProfile is the destination profile name (or "*" for any).
	ToProfile string `json:"to,omitempty"`

	// OnTrigger is the event type: "rate_limit", "stuck", "crash".
	OnTrigger string `json:"on"`

	// InjectHookPrelude is the hook prelude to inject before resuming work.
	// This enables workflows like TTC scaling or MCTS when switching to specific profiles.
	InjectHookPrelude string `json:"inject_hook_prelude,omitempty"`
}

// RateLimitConfig is the top-level configuration for rate-limit aware operation.
// Typically stored in settings/ratelimit.json or mayor/ratelimit.json.
type RateLimitConfig struct {
	Type    string `json:"type"`    // "ratelimit-config"
	Version int    `json:"version"` // schema version

	// Profiles maps profile names to their configurations.
	Profiles map[string]*InstanceProfile `json:"profiles"`

	// Roles maps role names to their policies.
	Roles map[string]*RolePolicy `json:"roles"`

	// GlobalCooldownMinutes is the default cooldown if not specified per-role.
	GlobalCooldownMinutes int `json:"global_cooldown_minutes,omitempty"`
}

// CurrentRateLimitConfigVersion is the current schema version.
const CurrentRateLimitConfigVersion = 1

// NewRateLimitConfig creates a new RateLimitConfig with sensible defaults.
func NewRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Type:                  "ratelimit-config",
		Version:               CurrentRateLimitConfigVersion,
		Profiles:              make(map[string]*InstanceProfile),
		Roles:                 make(map[string]*RolePolicy),
		GlobalCooldownMinutes: 30,
	}
}

// RateLimitEvent records a rate-limit occurrence for auditing and analysis.
// Events are persisted in beads for auditability.
type RateLimitEvent struct {
	// ID is a unique identifier for this event.
	ID string `json:"id"`

	// Timestamp is when the rate limit was detected.
	Timestamp time.Time `json:"timestamp"`

	// Agent is the agent that hit the rate limit (e.g., "gastown/witness", "gastown/polecats/Nux").
	Agent string `json:"agent"`

	// Role is the agent's role (e.g., "witness", "polecat", "deacon").
	Role string `json:"role"`

	// Rig is the rig name if applicable.
	Rig string `json:"rig,omitempty"`

	// CurrentProfile is the profile that was rate-limited.
	CurrentProfile string `json:"current_profile"`

	// StatusCode is the HTTP status code (typically 429).
	StatusCode int `json:"status_code,omitempty"`

	// ErrorSnippet is a snippet of the error message for debugging.
	ErrorSnippet string `json:"error_snippet,omitempty"`

	// RetryCount is how many retries were attempted before giving up.
	RetryCount int `json:"retry_count,omitempty"`

	// SwappedTo is the profile that was swapped to (empty if no swap occurred).
	SwappedTo string `json:"swapped_to,omitempty"`

	// CooldownUntil is when the current profile's cooldown expires.
	CooldownUntil time.Time `json:"cooldown_until,omitempty"`

	// TransitionRuleApplied is the transition rule that was applied, if any.
	TransitionRuleApplied string `json:"transition_rule_applied,omitempty"`
}

// CooldownState tracks the cooldown status for a profile.
type CooldownState struct {
	// ProfileName is the profile in cooldown.
	ProfileName string `json:"profile_name"`

	// StartedAt is when the cooldown started.
	StartedAt time.Time `json:"started_at"`

	// ExpiresAt is when the cooldown expires.
	ExpiresAt time.Time `json:"expires_at"`

	// Reason is why the cooldown was triggered.
	Reason string `json:"reason"`
}

// IsExpired returns true if the cooldown has expired.
func (c *CooldownState) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// SwapResult contains the outcome of an instance swap operation.
type SwapResult struct {
	// Success indicates if the swap completed successfully.
	Success bool `json:"success"`

	// FromProfile is the profile that was swapped from.
	FromProfile string `json:"from_profile"`

	// ToProfile is the profile that was swapped to.
	ToProfile string `json:"to_profile"`

	// Event is the rate limit event that triggered the swap.
	Event *RateLimitEvent `json:"event,omitempty"`

	// Error is set if the swap failed.
	Error string `json:"error,omitempty"`

	// TransitionPrelude is the hook prelude to inject, if applicable.
	TransitionPrelude string `json:"transition_prelude,omitempty"`
}
