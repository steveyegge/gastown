package ratelimit

import (
	"errors"
	"sync"
	"time"
)

// ErrAllProfilesCooling is returned when all profiles are in cooldown.
var ErrAllProfilesCooling = errors.New("all profiles are cooling down")

// Selector selects the next available profile after a rate limit.
type Selector interface {
	// SelectNext returns the next available profile for the role.
	// Returns ErrAllProfilesCooling if no profiles are available.
	SelectNext(role string, currentProfile string, event *RateLimitEvent) (string, error)

	// MarkCooldown marks a profile as cooling down until the specified time.
	MarkCooldown(profile string, until time.Time)

	// IsAvailable checks if a profile is available (not cooling down).
	IsAvailable(profile string) bool

	// SetPolicy configures a role's fallback policy.
	SetPolicy(role string, policy RolePolicy)
}

// DefaultSelector implements profile selection with cooldown tracking.
type DefaultSelector struct {
	mu        sync.RWMutex
	cooldowns map[string]time.Time // profile -> cooldown expiry
	policies  map[string]*RolePolicy
}

// NewSelector creates a new profile selector.
func NewSelector() *DefaultSelector {
	return &DefaultSelector{
		cooldowns: make(map[string]time.Time),
		policies:  make(map[string]*RolePolicy),
	}
}

// SetPolicy configures a role's fallback policy.
func (s *DefaultSelector) SetPolicy(role string, policy RolePolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.policies[role] = &policy
}

// SelectNext finds the next available profile in the fallback chain.
func (s *DefaultSelector) SelectNext(role string, currentProfile string, event *RateLimitEvent) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Mark current profile as cooling down
	policy := s.policies[role]
	if policy == nil {
		// No policy defined, use default 5 minute cooldown
		policy = &RolePolicy{
			FallbackChain:   []string{currentProfile},
			CooldownMinutes: 5,
		}
	}

	cooldownDuration := time.Duration(policy.CooldownMinutes) * time.Minute
	if cooldownDuration == 0 {
		cooldownDuration = 5 * time.Minute
	}
	s.cooldowns[currentProfile] = time.Now().Add(cooldownDuration)

	// Find next available profile in the chain
	now := time.Now()
	for _, profile := range policy.FallbackChain {
		if profile == currentProfile {
			continue // Skip current profile
		}
		expiry, inCooldown := s.cooldowns[profile]
		if !inCooldown || now.After(expiry) {
			// This profile is available
			return profile, nil
		}
	}

	return "", ErrAllProfilesCooling
}

// MarkCooldown marks a profile as cooling down until the specified time.
func (s *DefaultSelector) MarkCooldown(profile string, until time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldowns[profile] = until
}

// IsAvailable checks if a profile is available (not cooling down).
func (s *DefaultSelector) IsAvailable(profile string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	expiry, exists := s.cooldowns[profile]
	if !exists {
		return true
	}
	return time.Now().After(expiry)
}

// ClearCooldown removes a profile from cooldown (for testing).
func (s *DefaultSelector) ClearCooldown(profile string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cooldowns, profile)
}
