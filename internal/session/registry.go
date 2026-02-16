// Package session provides polecat session lifecycle management.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PrefixRegistry maps beads prefixes to rig names and vice versa.
// Used to resolve session names that use rig-specific prefixes.
type PrefixRegistry struct {
	mu          sync.RWMutex
	prefixToRig map[string]string // "gt" → "gastown"
	rigToPrefix map[string]string // "gastown" → "gt"
}

// NewPrefixRegistry creates an empty prefix registry.
func NewPrefixRegistry() *PrefixRegistry {
	return &PrefixRegistry{
		prefixToRig: make(map[string]string),
		rigToPrefix: make(map[string]string),
	}
}

// Register adds a prefix↔rig mapping.
func (r *PrefixRegistry) Register(prefix, rigName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixToRig[prefix] = rigName
	r.rigToPrefix[rigName] = prefix
}

// RigForPrefix returns the rig name for a given prefix.
// Returns the prefix itself if no mapping is found.
func (r *PrefixRegistry) RigForPrefix(prefix string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rig, ok := r.prefixToRig[prefix]; ok {
		return rig
	}
	return prefix
}

// PrefixForRig returns the beads prefix for a given rig name.
// Returns DefaultPrefix if no mapping is found.
func (r *PrefixRegistry) PrefixForRig(rigName string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if prefix, ok := r.rigToPrefix[rigName]; ok {
		return prefix
	}
	return DefaultPrefix
}

// Prefixes returns all registered prefixes, sorted longest-first for matching.
func (r *PrefixRegistry) Prefixes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prefixes := make([]string, 0, len(r.prefixToRig))
	for p := range r.prefixToRig {
		prefixes = append(prefixes, p)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return prefixes
}

// defaultRegistry is the package-level registry used by convenience functions.
var defaultRegistry = NewPrefixRegistry()

// DefaultRegistry returns the package-level prefix registry.
func DefaultRegistry() *PrefixRegistry {
	return defaultRegistry
}

// SetDefaultRegistry replaces the package-level prefix registry.
func SetDefaultRegistry(r *PrefixRegistry) {
	defaultRegistry = r
}

// InitRegistry populates the default registry from the town's rigs.json.
// Should be called early in the process lifecycle.
// Safe to call multiple times; later calls replace earlier data.
func InitRegistry(townRoot string) error {
	r, err := BuildPrefixRegistryFromTown(townRoot)
	if err != nil {
		return err
	}
	SetDefaultRegistry(r)
	return nil
}

// PrefixFor returns the beads prefix for a rig, using the default registry.
// Returns DefaultPrefix if the rig is unknown.
func PrefixFor(rigName string) string {
	return defaultRegistry.PrefixForRig(rigName)
}

// BuildPrefixRegistryFromTown reads rigs.json from a town root directory
// and returns a populated PrefixRegistry.
func BuildPrefixRegistryFromTown(townRoot string) (*PrefixRegistry, error) {
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	return BuildPrefixRegistryFromFile(rigsPath)
}

// rigsJSON is the minimal structure for reading rigs.json prefix data.
type rigsJSON struct {
	Rigs map[string]rigEntry `json:"rigs"`
}

type rigEntry struct {
	Beads *beadsEntry `json:"beads,omitempty"`
}

type beadsEntry struct {
	Prefix string `json:"prefix"`
}

// BuildPrefixRegistryFromFile reads a rigs.json file and returns a PrefixRegistry.
func BuildPrefixRegistryFromFile(path string) (*PrefixRegistry, error) {
	r := NewPrefixRegistry()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, err
	}

	var rigs rigsJSON
	if err := json.Unmarshal(data, &rigs); err != nil {
		return nil, err
	}

	for rigName, entry := range rigs.Rigs {
		if entry.Beads != nil && entry.Beads.Prefix != "" {
			r.Register(entry.Beads.Prefix, rigName)
		}
	}

	return r, nil
}

// HasPrefix returns true if the session name starts with a registered prefix followed by a dash.
func (r *PrefixRegistry) HasPrefix(sess string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for p := range r.prefixToRig {
		if strings.HasPrefix(sess, p+"-") {
			return true
		}
	}
	return false
}

// IsKnownSession returns true if the session name belongs to Gas Town.
// Checks for HQ prefix and registered rig prefixes from the default registry.
func IsKnownSession(sess string) bool {
	if strings.HasPrefix(sess, HQPrefix) {
		return true
	}
	return defaultRegistry.HasPrefix(sess)
}

// matchPrefix finds the prefix in a session name suffix using the registry.
// Returns the prefix and the remaining string after the prefix dash.
// Tries longest prefix match first.
// Falls back to splitting on the first dash if no registry match.
func (r *PrefixRegistry) matchPrefix(session string) (prefix, rest string, matched bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try known prefixes, longest first
	for _, p := range r.sortedPrefixes() {
		candidate := p + "-"
		if strings.HasPrefix(session, candidate) {
			return p, session[len(candidate):], true
		}
	}

	// Fallback: split on first dash
	idx := strings.Index(session, "-")
	if idx > 0 && idx < len(session)-1 {
		return session[:idx], session[idx+1:], false
	}

	return "", "", false
}

// sortedPrefixes returns prefixes sorted longest-first (must hold read lock).
func (r *PrefixRegistry) sortedPrefixes() []string {
	prefixes := make([]string, 0, len(r.prefixToRig))
	for p := range r.prefixToRig {
		prefixes = append(prefixes, p)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return prefixes
}
