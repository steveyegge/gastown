package ratelimit

import (
	"sync"
	"time"
)

// CooldownStore tracks which profiles are currently cooling down after rate limits.
type CooldownStore struct {
	mu        sync.RWMutex
	cooldowns map[string]time.Time // profile name -> cooldown expiry time
}

// NewCooldownStore creates a new in-memory cooldown store.
func NewCooldownStore() *CooldownStore {
	return &CooldownStore{
		cooldowns: make(map[string]time.Time),
	}
}

// MarkCooldown marks a profile as cooling down until the specified time.
func (s *CooldownStore) MarkCooldown(profile string, until time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cooldowns[profile] = until
}

// ClearCooldown removes the cooldown for a profile, making it immediately available.
func (s *CooldownStore) ClearCooldown(profile string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cooldowns, profile)
}

// IsAvailable checks if a profile is available (not cooling down).
func (s *CooldownStore) IsAvailable(profile string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	until, exists := s.cooldowns[profile]
	if !exists {
		return true
	}
	// If cooldown has expired, profile is available
	return time.Now().After(until)
}

// GetCooldownUntil returns the cooldown expiry time for a profile.
// Returns zero time if no cooldown is set.
func (s *CooldownStore) GetCooldownUntil(profile string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cooldowns[profile]
}
