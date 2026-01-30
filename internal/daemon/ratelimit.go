package daemon

import (
	"fmt"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/ratelimit"
)

// rateLimitCheckInterval is how often to check for rate limits.
// Must be balanced: too frequent wastes resources, too infrequent delays recovery.
const rateLimitCheckInterval = 30 * time.Second

// checkRateLimits monitors all active sessions for rate limit indicators.
// When a rate limit is detected, triggers an automatic instance swap if configured.
func (d *Daemon) checkRateLimits() {
	// Load rate limit manager (lazy init per heartbeat to pick up config changes)
	manager := ratelimit.NewManager(d.config.TownRoot)
	config := manager.GetConfig()

	// Skip if no profiles configured
	if len(config.Profiles) == 0 {
		return
	}

	// Check Deacon
	if IsPatrolEnabled(d.patrolConfig, "deacon") {
		if policy := config.Roles["deacon"]; policy != nil {
			d.checkSessionForRateLimit(manager, "gt-deacon", "deacon", "", "deacon")
		}
	}

	// Check Witnesses and Refineries for all rigs
	rigs := d.getKnownRigs()
	for _, rigName := range rigs {
		// Check Witness
		if IsPatrolEnabled(d.patrolConfig, "witness") {
			if policy := config.Roles["witness"]; policy != nil {
				sessionName := fmt.Sprintf("gt-%s-witness", rigName)
				d.checkSessionForRateLimit(manager, sessionName, "witness", rigName, fmt.Sprintf("%s/witness", rigName))
			}
		}

		// Check Refinery
		if IsPatrolEnabled(d.patrolConfig, "refinery") {
			if policy := config.Roles["refinery"]; policy != nil {
				sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
				d.checkSessionForRateLimit(manager, sessionName, "refinery", rigName, fmt.Sprintf("%s/refinery", rigName))
			}
		}

		// Check Polecats
		if policy := config.Roles["polecat"]; policy != nil {
			d.checkRigPolecatsForRateLimit(manager, rigName)
		}
	}

	// Prune expired cooldowns
	pruned := manager.PruneExpiredCooldowns()
	if pruned > 0 {
		d.logger.Printf("Pruned %d expired cooldown(s)", pruned)
	}
}

// checkSessionForRateLimit checks a single session for rate limit indicators.
func (d *Daemon) checkSessionForRateLimit(manager *ratelimit.Manager, sessionName, role, rig, agent string) {
	// Check if session exists
	hasSession, err := d.tmux.HasSession(sessionName)
	if err != nil || !hasSession {
		return
	}

	// Capture recent output
	output, err := d.tmux.CapturePane(sessionName, 50)
	if err != nil {
		return
	}

	// Detect rate limit
	indicator := ratelimit.DetectRateLimit(output)
	if !indicator.Detected {
		return
	}

	d.logger.Printf("RATE LIMIT DETECTED: session=%s provider=%s status=%d",
		sessionName, indicator.Provider, indicator.StatusCode)

	// Get current profile
	currentProfile := manager.GetActiveProfile(agent, role)
	profileName := "default"
	if currentProfile != nil {
		profileName = currentProfile.Name
	}

	// Create rate limit event
	event := &ratelimit.RateLimitEvent{
		ID:             fmt.Sprintf("%s-%d", sessionName, time.Now().UnixNano()),
		Timestamp:      time.Now(),
		Agent:          agent,
		Role:           role,
		Rig:            rig,
		CurrentProfile: profileName,
		StatusCode:     indicator.StatusCode,
		ErrorSnippet:   indicator.Message,
	}

	// Handle the rate limit (select fallback, start cooldown)
	result, err := manager.HandleRateLimit(event)
	if err != nil {
		d.logger.Printf("Failed to handle rate limit for %s: %v", sessionName, err)
		// Emit event for monitoring
		_ = events.LogFeed(events.TypeRateLimit, agent, map[string]interface{}{
			"session":     sessionName,
			"role":        role,
			"error":       err.Error(),
			"profile":     profileName,
			"status_code": indicator.StatusCode,
		})
		return
	}

	d.logger.Printf("Rate limit handled: %s -> %s (prelude=%s)",
		result.FromProfile, result.ToProfile, result.TransitionPrelude)

	// Emit success event
	_ = events.LogFeed(events.TypeInstanceSwap, agent, map[string]interface{}{
		"session":      sessionName,
		"role":         role,
		"from_profile": result.FromProfile,
		"to_profile":   result.ToProfile,
		"trigger":      "rate_limit",
		"prelude":      result.TransitionPrelude,
	})
}

// checkRigPolecatsForRateLimit checks all polecats in a rig for rate limits.
func (d *Daemon) checkRigPolecatsForRateLimit(manager *ratelimit.Manager, rigName string) {
	polecatsDir := fmt.Sprintf("%s/%s/polecats", d.config.TownRoot, rigName)
	polecats, err := listPolecatWorktrees(polecatsDir)
	if err != nil {
		return
	}

	for _, polecatName := range polecats {
		sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)
		agent := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
		d.checkSessionForRateLimit(manager, sessionName, "polecat", rigName, agent)
	}
}

// checkWitnessForRateLimit checks if the Witness itself needs swapping.
// This implements "watch the watcher" - the daemon supervises Witness sessions.
func (d *Daemon) checkWitnessForRateLimit(manager *ratelimit.Manager, rigName string) {
	sessionName := fmt.Sprintf("gt-%s-witness", rigName)
	agent := fmt.Sprintf("%s/witness", rigName)

	// Check if session exists and is responding
	hasSession, err := d.tmux.HasSession(sessionName)
	if err != nil || !hasSession {
		return
	}

	// Capture output and check for rate limits
	output, err := d.tmux.CapturePane(sessionName, 100)
	if err != nil {
		return
	}

	// Detect both rate limits and stuck conditions
	indicator := ratelimit.DetectRateLimit(output)
	if !indicator.Detected {
		// Also check for stuck condition
		// Use a simpler heuristic: look for no recent activity with error patterns
		stuckIndicator := ratelimit.DetectStuck(output, 20) // 20 minutes threshold
		if !stuckIndicator.Detected {
			return
		}
		indicator = stuckIndicator
	}

	d.logger.Printf("WITNESS ISSUE DETECTED: %s - %s", sessionName, indicator.Message)

	// Handle the witness swap
	currentProfile := manager.GetActiveProfile(agent, "witness")
	profileName := "default"
	if currentProfile != nil {
		profileName = currentProfile.Name
	}

	event := &ratelimit.RateLimitEvent{
		ID:             fmt.Sprintf("%s-%d", sessionName, time.Now().UnixNano()),
		Timestamp:      time.Now(),
		Agent:          agent,
		Role:           "witness",
		Rig:            rigName,
		CurrentProfile: profileName,
		StatusCode:     indicator.StatusCode,
		ErrorSnippet:   indicator.Message,
	}

	// Try to handle the rate limit
	_, err = manager.HandleRateLimit(event)
	if err != nil {
		d.logger.Printf("Failed to handle witness rate limit %s: %v", sessionName, err)
		// As a last resort, try a simple restart with the same profile
		d.restartWitnessSession(rigName)
		return
	}

	d.logger.Printf("Witness rate limit handled for %s", sessionName)
}

// restartWitnessSession restarts a witness session without changing profiles.
// This is a fallback when profile swapping fails.
func (d *Daemon) restartWitnessSession(rigName string) {
	d.logger.Printf("Attempting direct witness restart for %s", rigName)

	sessionName := fmt.Sprintf("gt-%s-witness", rigName)

	// Kill existing session
	_ = d.tmux.KillSession(sessionName)

	// ensureWitnessRunning will recreate it on next heartbeat
	d.ensureWitnessRunning(rigName)
}

// getRateLimitStatus returns a summary of current rate limit state.
func (d *Daemon) getRateLimitStatus() *RateLimitStatus {
	manager := ratelimit.NewManager(d.config.TownRoot)
	cooldowns := manager.GetCooldownSummary()

	status := &RateLimitStatus{
		CooldownCount:    len(cooldowns),
		CooldownProfiles: make([]string, 0, len(cooldowns)),
	}

	for _, c := range cooldowns {
		status.CooldownProfiles = append(status.CooldownProfiles,
			fmt.Sprintf("%s (until %s)", c.ProfileName, c.ExpiresAt.Format("15:04")))
	}

	return status
}

// RateLimitStatus provides a summary of rate limit state for status commands.
type RateLimitStatus struct {
	CooldownCount    int      `json:"cooldown_count"`
	CooldownProfiles []string `json:"cooldown_profiles"`
}

// formatRateLimitStatus formats rate limit status for logging.
func formatRateLimitStatus(status *RateLimitStatus) string {
	if status.CooldownCount == 0 {
		return "no profiles in cooldown"
	}
	return fmt.Sprintf("%d profile(s) in cooldown: %s",
		status.CooldownCount, strings.Join(status.CooldownProfiles, ", "))
}
