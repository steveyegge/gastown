package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// Logger defines the interface for structured logging in the rate limit handler.
type Logger interface {
	// Info logs informational messages with structured key-value pairs.
	Info(msg string, keysAndValues ...any)
	// Warn logs warning messages with structured key-value pairs.
	Warn(msg string, keysAndValues ...any)
	// Error logs error messages with structured key-value pairs.
	Error(msg string, keysAndValues ...any)
}

// DefaultLogger is a simple logger implementation using the standard log package.
type DefaultLogger struct{}

// Info logs informational messages.
func (l *DefaultLogger) Info(msg string, keysAndValues ...any) {
	log.Printf("[INFO] %s %v", msg, formatKV(keysAndValues))
}

// Warn logs warning messages.
func (l *DefaultLogger) Warn(msg string, keysAndValues ...any) {
	log.Printf("[WARN] %s %v", msg, formatKV(keysAndValues))
}

// Error logs error messages.
func (l *DefaultLogger) Error(msg string, keysAndValues ...any) {
	log.Printf("[ERROR] %s %v", msg, formatKV(keysAndValues))
}

// formatKV formats key-value pairs for logging.
func formatKV(keysAndValues []any) string {
	if len(keysAndValues) == 0 {
		return ""
	}
	result := ""
	for i := 0; i < len(keysAndValues)-1; i += 2 {
		if i > 0 {
			result += " "
		}
		result += fmt.Sprintf("%v=%v", keysAndValues[i], keysAndValues[i+1])
	}
	return result
}

// Handler orchestrates rate limit detection, profile selection, and session swapping.
// It is the main integration point for the Witness's rate limit handling.
type Handler struct {
	detector   Detector
	selector   Selector
	swapper    Swapper
	controller SessionController
	logger     Logger
}

// HandlerConfig contains configuration for the rate limit handler.
type HandlerConfig struct {
	// DefaultCooldownMinutes is the default cooldown period for profiles.
	DefaultCooldownMinutes int

	// RolePolicies maps roles to their profile fallback policies.
	RolePolicies map[string]RolePolicy

	// Logger is an optional structured logger. If nil, DefaultLogger is used.
	Logger Logger
}

// NewHandler creates a new rate limit handler with the given configuration.
func NewHandler(controller SessionController, cfg HandlerConfig) *Handler {
	detector := NewDetector("", "") // Agent info set per-call via SetAgentInfo

	// Convert RolePolicies to pointer map for NewSelector
	policies := make(map[string]*RolePolicy)
	for role, policy := range cfg.RolePolicies {
		p := policy // Copy to avoid aliasing
		policies[role] = &p
	}
	selector := NewSelector(policies)
	swapper := NewSwapper(controller)

	logger := cfg.Logger
	if logger == nil {
		logger = &DefaultLogger{}
	}

	h := &Handler{
		detector:   detector,
		selector:   selector,
		swapper:    swapper,
		controller: controller,
		logger:     logger,
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
	h.logRateLimitEvent(event)

	// Step 2: Select fallback profile
	newProfile, err := h.selector.SelectNext("polecat", exitInfo.CurrentProfile, event)
	if err != nil {
		if errors.Is(err, ErrAllProfilesCooling) {
			result.AllProfilesCooling = true
			h.alertNoProfilesAvailable(exitInfo, event)
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
func (h *Handler) logRateLimitEvent(event *RateLimitEvent) {
	h.logger.Info("rate limit detected",
		"agent", event.AgentID,
		"profile", event.Profile,
		"provider", event.Provider,
		"exit_code", event.ExitCode,
		"timestamp", event.Timestamp.Format(time.RFC3339),
		"error", event.ErrorSnippet,
	)
}

// alertNoProfilesAvailable emits an alert when all profiles are cooling down.
func (h *Handler) alertNoProfilesAvailable(exitInfo PolecatExitInfo, event *RateLimitEvent) {
	h.logger.Error("all profiles cooling - agent cannot continue",
		"rig", exitInfo.RigName,
		"polecat", exitInfo.PolecatName,
		"last_profile", event.Profile,
		"rate_limit_time", event.Timestamp.Format(time.RFC3339),
		"hooked_work", exitInfo.HookedWork,
	)
	// In a full implementation, this would:
	// 1. Send mail to Witness/Mayor for escalation
	// 2. Create an alert bead for tracking
	// 3. Possibly emit to external monitoring
}
