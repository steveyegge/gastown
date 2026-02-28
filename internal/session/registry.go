// Package session provides polecat session lifecycle management.
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"regexp"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/tmux"
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

// AllRigs returns a copy of the rig-name → prefix mapping for all registered rigs.
// Callers can iterate it to find known rig names embedded in session strings.
func (r *PrefixRegistry) AllRigs() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.rigToPrefix))
	for rig, prefix := range r.rigToPrefix {
		out[rig] = prefix
	}
	return out
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
// Access is protected by defaultRegistryMu for concurrent test safety.
var (
	defaultRegistry   = NewPrefixRegistry()
	defaultRegistryMu sync.RWMutex
)

// DefaultRegistry returns the package-level prefix registry.
func DefaultRegistry() *PrefixRegistry {
	defaultRegistryMu.RLock()
	defer defaultRegistryMu.RUnlock()
	return defaultRegistry
}

// SetDefaultRegistry replaces the package-level prefix registry.
func SetDefaultRegistry(r *PrefixRegistry) {
	defaultRegistryMu.Lock()
	defaultRegistry = r
	defaultRegistryMu.Unlock()
}

// InitRegistry populates the default registry from the town's rigs.json and
// loads the agent registry from settings/agents.json.
// Both registries are loaded independently — a failure in one does not
// prevent the other from loading.
// Should be called early in the process lifecycle.
// Safe to call multiple times; later calls replace earlier data.
func InitRegistry(townRoot string) error {
	var errs []error

	// Use the default tmux socket so all sessions are visible via prefix+s
	// from any terminal. Multi-town isolation (which would need per-town
	// sockets) already requires containers/VMs due to singleton mayor/deacon
	// session names, so a dedicated socket provides no real benefit while
	// causing cross-socket bugs and split session visibility.
	tmux.SetDefaultSocket("default")

	r, err := BuildPrefixRegistryFromTown(townRoot)
	if err != nil {
		errs = append(errs, fmt.Errorf("prefix registry: %w", err))
	} else {
		SetDefaultRegistry(r)
	}

	// Load agent registry so all entry points (CLI, daemon, witness) respect
	// user-configured overrides like custom process_names.
	if err := config.LoadAgentRegistry(config.DefaultAgentRegistryPath(townRoot)); err != nil {
		errs = append(errs, fmt.Errorf("agent registry: %w", err))
	}

	return errors.Join(errs...)
}

// sanitizeRe matches non-alphanumeric, non-hyphen characters.
var sanitizeRe = regexp.MustCompile(`[^a-z0-9-]+`)

// sanitizeTownName cleans a town name to be a valid tmux socket name.
// Lowercases, replaces non-alphanumeric characters with hyphens, trims hyphens.
func sanitizeTownName(name string) string {
	name = strings.ToLower(name)
	name = sanitizeRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "default"
	}
	return name
}

// PrefixFor returns the beads prefix for a rig, using the default registry.
// Returns DefaultPrefix if the rig is unknown.
func PrefixFor(rigName string) string {
	return DefaultRegistry().PrefixForRig(rigName)
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

// LegacyPrefixes are prefixes accepted as valid even when the registry is empty.
// gt = default rig, bd = beads, hq = town-level HQ services, gthq = gastown HQ.
var LegacyPrefixes = []string{"gt", "bd", "hq", "gthq"}

// HasKnownPrefix returns true if s starts with a registered or legacy prefix
// followed by "-". Use this instead of hand-rolling prefix checks so that
// all call-sites agree on what constitutes a valid prefix.
func HasKnownPrefix(s string) bool {
	if DefaultRegistry().HasPrefix(s) {
		return true
	}
	for _, p := range LegacyPrefixes {
		if strings.HasPrefix(s, p+"-") {
			return true
		}
	}
	return false
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
	return DefaultRegistry().HasPrefix(sess)
}

// matchPrefix finds the prefix in a session name suffix using the registry.
// Returns the prefix and the remaining string after the prefix dash.
// Tries longest prefix match first.
// Only matches sessions with registered prefixes - does NOT fall back to
// splitting on dashes, as that would incorrectly match non-gastown sessions
// (e.g., "gs-1923" or "dotfiles-main" would be parsed as gastown sessions).
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
