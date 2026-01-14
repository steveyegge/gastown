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
// It composes CooldownStore for cooldown management.
type DefaultSelector struct {
	mu       sync.RWMutex
	store    *CooldownStore
	policies map[string]*RolePolicy
}

// NewSelector creates a new profile selector with optional initial policies.
func NewSelector(policies map[string]*RolePolicy) *DefaultSelector {
	s := &DefaultSelector{
		store:    NewCooldownStore(),
		policies: make(map[string]*RolePolicy),
	}
	// Copy provided policies
	for role, policy := range policies {
		s.policies[role] = policy
	}
	return s
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
	s.store.MarkCooldown(currentProfile, time.Now().Add(cooldownDuration))

	// Find next available profile in the chain
	for _, profile := range policy.FallbackChain {
		if profile == currentProfile {
			continue // Skip current profile
		}
		if s.store.IsAvailable(profile) {
			return profile, nil
		}
	}

	return "", ErrAllProfilesCooling
}

// MarkCooldown marks a profile as cooling down until the specified time.
func (s *DefaultSelector) MarkCooldown(profile string, until time.Time) {
	s.store.MarkCooldown(profile, until)
}

// IsAvailable checks if a profile is available (not cooling down).
func (s *DefaultSelector) IsAvailable(profile string) bool {
	return s.store.IsAvailable(profile)
}

// ClearCooldown removes a profile from cooldown (for testing).
func (s *DefaultSelector) ClearCooldown(profile string) {
	s.store.ClearCooldown(profile)
}
