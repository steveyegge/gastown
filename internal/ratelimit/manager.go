package ratelimit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager handles rate-limit aware instance management.
// It tracks cooldowns, selects fallback profiles, and coordinates swaps.
type Manager struct {
	townRoot string
	config   *RateLimitConfig
	mu       sync.RWMutex

	// cooldowns tracks active cooldowns by profile name.
	cooldowns map[string]*CooldownState

	// activeProfiles tracks the current profile for each agent.
	activeProfiles map[string]string // agent -> profile name
}

// NewManager creates a new rate limit manager for a town.
func NewManager(townRoot string) *Manager {
	m := &Manager{
		townRoot:       townRoot,
		cooldowns:      make(map[string]*CooldownState),
		activeProfiles: make(map[string]string),
	}

	// Load config if it exists
	if cfg, err := LoadConfig(townRoot); err == nil {
		m.config = cfg
	} else {
		m.config = NewRateLimitConfig()
	}

	// Load cooldown state if it exists
	_ = m.loadCooldownState()

	return m
}

// GetConfig returns the current rate limit configuration.
func (m *Manager) GetConfig() *RateLimitConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetProfile returns a profile by name.
func (m *Manager) GetProfile(name string) *InstanceProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Profiles[name]
}

// GetActiveProfile returns the current active profile for an agent.
// Returns the first profile in the fallback chain if none is set.
func (m *Manager) GetActiveProfile(agent string, role string) *InstanceProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check for explicitly set active profile
	if profileName, ok := m.activeProfiles[agent]; ok {
		if profile := m.config.Profiles[profileName]; profile != nil {
			return profile
		}
	}

	// Fall back to first profile in the role's fallback chain
	if policy := m.config.Roles[role]; policy != nil && len(policy.FallbackChain) > 0 {
		profileName := policy.FallbackChain[0]
		return m.config.Profiles[profileName]
	}

	return nil
}

// SetActiveProfile sets the active profile for an agent.
func (m *Manager) SetActiveProfile(agent, profileName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.config.Profiles[profileName]; !ok {
		return fmt.Errorf("profile %q not found", profileName)
	}

	m.activeProfiles[agent] = profileName
	return nil
}

// IsProfileInCooldown checks if a profile is currently in cooldown.
func (m *Manager) IsProfileInCooldown(profileName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.cooldowns[profileName]
	if !ok {
		return false
	}
	return !state.IsExpired()
}

// GetCooldownState returns the cooldown state for a profile, or nil if not in cooldown.
func (m *Manager) GetCooldownState(profileName string) *CooldownState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.cooldowns[profileName]
	if !ok || state.IsExpired() {
		return nil
	}
	return state
}

// StartCooldown starts a cooldown period for a profile.
func (m *Manager) StartCooldown(profileName, reason string, minutes int) *CooldownState {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	state := &CooldownState{
		ProfileName: profileName,
		StartedAt:   now,
		ExpiresAt:   now.Add(time.Duration(minutes) * time.Minute),
		Reason:      reason,
	}
	m.cooldowns[profileName] = state

	// Persist cooldown state
	_ = m.saveCooldownState()

	return state
}

// ClearCooldown removes a cooldown for a profile.
func (m *Manager) ClearCooldown(profileName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.cooldowns, profileName)
	_ = m.saveCooldownState()
}

// SelectFallbackProfile selects the next available profile from the fallback chain.
// Respects stickiness preferences and skips profiles in cooldown.
func (m *Manager) SelectFallbackProfile(role, currentProfile string) (*InstanceProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	policy := m.config.Roles[role]
	if policy == nil {
		return nil, fmt.Errorf("no policy defined for role %q", role)
	}

	if len(policy.FallbackChain) == 0 {
		return nil, fmt.Errorf("no fallback chain defined for role %q", role)
	}

	// Handle stickiness: if OnlyFailoverIfAllCooling is true, first try
	// all profiles from the preferred provider.
	if policy.Stickiness != nil && policy.Stickiness.OnlyFailoverIfAllCooling {
		preferred := policy.Stickiness.PreferProvider
		allPreferredCooling := true

		// First pass: check if any preferred provider profiles are available
		for _, name := range policy.FallbackChain {
			profile := m.config.Profiles[name]
			if profile == nil {
				continue
			}
			if profile.Provider != preferred {
				continue
			}
			if name == currentProfile {
				continue // Skip the one that just got rate limited
			}

			// Check if in cooldown
			state, ok := m.cooldowns[name]
			if !ok || state.IsExpired() {
				allPreferredCooling = false
				return profile, nil // Found an available preferred profile
			}
		}

		// If not all preferred are cooling, don't fail over yet
		if !allPreferredCooling {
			// This shouldn't happen since we return above, but be safe
			return nil, fmt.Errorf("preferred provider profiles available but not selected")
		}

		// Fall through to try non-preferred providers
	}

	// Standard fallback: try profiles in order, skip cooldowns
	for _, name := range policy.FallbackChain {
		if name == currentProfile {
			continue // Skip the one that just got rate limited
		}

		// Check if in cooldown
		state, ok := m.cooldowns[name]
		if ok && !state.IsExpired() {
			continue
		}

		profile := m.config.Profiles[name]
		if profile != nil {
			return profile, nil
		}
	}

	return nil, fmt.Errorf("all fallback profiles for role %q are in cooldown", role)
}

// GetTransitionRule finds a matching transition rule for a swap.
func (m *Manager) GetTransitionRule(role, fromProfile, toProfile, trigger string) *TransitionRule {
	m.mu.RLock()
	defer m.mu.RUnlock()

	policy := m.config.Roles[role]
	if policy == nil {
		return nil
	}

	for _, rule := range policy.TransitionRules {
		// Check from profile match
		if rule.FromProfile != "" && rule.FromProfile != "*" && rule.FromProfile != fromProfile {
			continue
		}

		// Check to profile match
		if rule.ToProfile != "" && rule.ToProfile != "*" && rule.ToProfile != toProfile {
			continue
		}

		// Check trigger match
		if rule.OnTrigger != trigger {
			continue
		}

		return &rule
	}

	return nil
}

// HandleRateLimit processes a rate limit event and coordinates the swap.
func (m *Manager) HandleRateLimit(event *RateLimitEvent) (*SwapResult, error) {
	result := &SwapResult{
		FromProfile: event.CurrentProfile,
		Event:       event,
	}

	// Get the role's policy
	policy := m.config.Roles[event.Role]
	if policy == nil {
		result.Error = fmt.Sprintf("no policy for role %s", event.Role)
		return result, fmt.Errorf(result.Error)
	}

	// Start cooldown for the rate-limited profile
	cooldownMinutes := policy.CooldownMinutes
	if cooldownMinutes == 0 {
		cooldownMinutes = m.config.GlobalCooldownMinutes
	}
	if cooldownMinutes == 0 {
		cooldownMinutes = 30 // Default
	}

	cooldown := m.StartCooldown(event.CurrentProfile, "rate_limit", cooldownMinutes)
	event.CooldownUntil = cooldown.ExpiresAt

	// Select fallback profile
	fallback, err := m.SelectFallbackProfile(event.Role, event.CurrentProfile)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.ToProfile = fallback.Name
	event.SwappedTo = fallback.Name

	// Check for transition rules
	rule := m.GetTransitionRule(event.Role, event.CurrentProfile, fallback.Name, "rate_limit")
	if rule != nil {
		result.TransitionPrelude = rule.InjectHookPrelude
		event.TransitionRuleApplied = rule.InjectHookPrelude
	}

	// Update active profile
	m.mu.Lock()
	m.activeProfiles[event.Agent] = fallback.Name
	m.mu.Unlock()

	// Persist the event
	if err := m.persistEvent(event); err != nil {
		// Non-fatal: log but continue
	}

	result.Success = true
	return result, nil
}

// persistEvent saves a rate limit event to the events log.
func (m *Manager) persistEvent(event *RateLimitEvent) error {
	eventsDir := filepath.Join(m.townRoot, "daemon", "ratelimit-events")
	if err := os.MkdirAll(eventsDir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("event-%s.json", event.Timestamp.Format("20060102-150405"))
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(eventsDir, filename), data, 0644)
}

// cooldownStateFile returns the path to the cooldown state file.
func (m *Manager) cooldownStateFile() string {
	return filepath.Join(m.townRoot, "daemon", "cooldowns.json")
}

// loadCooldownState loads persisted cooldown state.
func (m *Manager) loadCooldownState() error {
	data, err := os.ReadFile(m.cooldownStateFile())
	if err != nil {
		return err
	}

	var states []*CooldownState
	if err := json.Unmarshal(data, &states); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Only load non-expired cooldowns
	for _, state := range states {
		if !state.IsExpired() {
			m.cooldowns[state.ProfileName] = state
		}
	}

	return nil
}

// saveCooldownState persists the current cooldown state.
// Called with lock held.
func (m *Manager) saveCooldownState() error {
	var states []*CooldownState
	for _, state := range m.cooldowns {
		if !state.IsExpired() {
			states = append(states, state)
		}
	}

	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(m.cooldownStateFile())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(m.cooldownStateFile(), data, 0644)
}

// LoadConfig loads the rate limit configuration from the town.
func LoadConfig(townRoot string) (*RateLimitConfig, error) {
	// Try settings/ratelimit.json first
	settingsPath := filepath.Join(townRoot, "settings", "ratelimit.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var cfg RateLimitConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", settingsPath, err)
		}
		return &cfg, nil
	}

	// Fall back to mayor/ratelimit.json
	mayorPath := filepath.Join(townRoot, "mayor", "ratelimit.json")
	if data, err := os.ReadFile(mayorPath); err == nil {
		var cfg RateLimitConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", mayorPath, err)
		}
		return &cfg, nil
	}

	return nil, fmt.Errorf("no ratelimit.json found")
}

// SaveConfig saves the rate limit configuration.
func SaveConfig(townRoot string, cfg *RateLimitConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	settingsDir := filepath.Join(townRoot, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(settingsDir, "ratelimit.json"), data, 0644)
}

// PruneExpiredCooldowns removes expired cooldowns from the state.
func (m *Manager) PruneExpiredCooldowns() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	pruned := 0
	for name, state := range m.cooldowns {
		if state.IsExpired() {
			delete(m.cooldowns, name)
			pruned++
		}
	}

	if pruned > 0 {
		_ = m.saveCooldownState()
	}

	return pruned
}

// GetCooldownSummary returns a summary of all active cooldowns.
func (m *Manager) GetCooldownSummary() []*CooldownState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var summary []*CooldownState
	for _, state := range m.cooldowns {
		if !state.IsExpired() {
			summary = append(summary, state)
		}
	}
	return summary
}
