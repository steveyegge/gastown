package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// Handler orchestrates rate limit detection, profile selection, and session swapping.
// It is the main integration point for the Witness's rate limit handling.
type Handler struct {
	detector   Detector
	selector   Selector
	swapper    Swapper
	controller SessionController
}

// HandlerConfig contains configuration for the rate limit handler.
type HandlerConfig struct {
	// DefaultCooldownMinutes is the default cooldown period for profiles.
	DefaultCooldownMinutes int

	// RolePolicies maps roles to their profile fallback policies.
	RolePolicies map[string]RolePolicy
}

// NewHandler creates a new rate limit handler with the given configuration.
func NewHandler(controller SessionController, cfg HandlerConfig) *Handler {
	detector := NewDetector()
	selector := NewSelector()
	swapper := NewSwapper(controller)

	// Configure role policies
	for role, policy := range cfg.RolePolicies {
		selector.SetPolicy(role, policy)
	}

	h := &Handler{
		detector:   detector,
		selector:   selector,
		swapper:    swapper,
		controller: controller,
	}
	return h
}

// HandleExitResult contains the result of handling a session exit.
type HandleExitResult struct {
	// WasRateLimit indicates if the exit was due to rate limiting.
	WasRateLimit bool

	// Event contains the rate limit event details (if detected).
	Event *RateLimitEvent

	// SwapAttempted indicates if a profile swap was attempted.
	SwapAttempted bool

	// SwapResult contains the swap outcome (if attempted).
	SwapResult *SwapResult

	// AllProfilesCooling indicates all profiles are in cooldown.
	AllProfilesCooling bool

	// Error contains any error that occurred during handling.
	Error error
}

// HandlePolecatExit processes a polecat session exit and handles rate limits.
// This is the main entry point for the Witness to detect and respond to rate limits.
func (h *Handler) HandlePolecatExit(ctx context.Context, exitInfo PolecatExitInfo) *HandleExitResult {
	result := &HandleExitResult{}

	// Configure detector with agent context
	h.detector.SetAgentInfo(
		fmt.Sprintf("%s/%s", exitInfo.RigName, exitInfo.PolecatName),
		exitInfo.CurrentProfile,
		exitInfo.Provider,
	)

	// Step 1: Detect rate limit
	event, isRateLimit := h.detector.Detect(exitInfo.ExitCode, exitInfo.Stderr)
	if !isRateLimit {
		return result
	}

	result.WasRateLimit = true
	result.Event = event

	// Log the rate limit event
	logRateLimitEvent(event)

	// Step 2: Select fallback profile
	newProfile, err := h.selector.SelectNext("polecat", exitInfo.CurrentProfile, event)
	if err != nil {
		if err == ErrAllProfilesCooling {
			result.AllProfilesCooling = true
			alertNoProfilesAvailable(exitInfo, event)
			return result
		}
		result.Error = fmt.Errorf("selecting fallback profile: %w", err)
		return result
	}

	// Step 3: Perform swap
	result.SwapAttempted = true

	swapReq := SwapRequest{
		RigName:     exitInfo.RigName,
		PolecatName: exitInfo.PolecatName,
		OldProfile:  exitInfo.CurrentProfile,
		NewProfile:  newProfile,
		HookedWork:  exitInfo.HookedWork,
		Reason:      "rate_limit",
	}

	swapResult, err := h.swapper.Swap(ctx, swapReq)
	result.SwapResult = swapResult
	if err != nil {
		result.Error = fmt.Errorf("swapping session: %w", err)
		EmitSwapEvent(swapReq, swapResult)
		return result
	}

	// Emit successful swap event
	EmitSwapEvent(swapReq, swapResult)

	return result
}

// PolecatExitInfo contains information about a polecat session exit.
type PolecatExitInfo struct {
	// RigName is the rig containing the polecat.
	RigName string

	// PolecatName is the name of the polecat that exited.
	PolecatName string

	// ExitCode is the process exit code.
	ExitCode int

	// Stderr is the stderr output from the session.
	Stderr string

	// CurrentProfile is the profile that was in use.
	CurrentProfile string

	// Provider is the API provider (e.g., "anthropic").
	Provider string

	// HookedWork is the bead ID of work that was hooked.
	HookedWork string
}

// GetSelector returns the profile selector for external configuration.
func (h *Handler) GetSelector() Selector {
	return h.selector
}

// SetPolicy configures a role's fallback policy.
func (h *Handler) SetPolicy(role string, policy RolePolicy) {
	h.selector.SetPolicy(role, policy)
}

// logRateLimitEvent logs a rate limit event for observability.
func logRateLimitEvent(event *RateLimitEvent) {
	fmt.Printf("[RATE_LIMIT] Agent: %s, Profile: %s, Provider: %s, ExitCode: %d, Time: %s\n",
		event.AgentID, event.Profile, event.Provider, event.ExitCode,
		event.Timestamp.Format(time.RFC3339))
	if event.ErrorSnippet != "" {
		fmt.Printf("[RATE_LIMIT] Error: %s\n", event.ErrorSnippet)
	}
}

// alertNoProfilesAvailable emits an alert when all profiles are cooling down.
func alertNoProfilesAvailable(exitInfo PolecatExitInfo, event *RateLimitEvent) {
	fmt.Printf("[ALERT] All profiles cooling! Agent: %s/%s cannot continue\n",
		exitInfo.RigName, exitInfo.PolecatName)
	fmt.Printf("[ALERT] Last profile: %s, Rate limit at: %s\n",
		event.Profile, event.Timestamp.Format(time.RFC3339))
	fmt.Printf("[ALERT] Work at risk: %s\n", exitInfo.HookedWork)
	// In a full implementation, this would:
	// 1. Send mail to Witness/Mayor for escalation
	// 2. Create an alert bead for tracking
	// 3. Possibly emit to external monitoring
}
