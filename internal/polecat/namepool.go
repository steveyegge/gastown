package polecat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/steveyegge/gastown/internal/util"
)

const (
	// DefaultPoolSize is the number of name slots in the pool.
	// NOTE: This is a pool of NAMES, not polecats. Polecats are spawned fresh
	// for each task and nuked when done - there is no idle pool of polecats.
	// Only the name slots are reused when a polecat is nuked and a new one spawned.
	DefaultPoolSize = 50

	// DefaultTheme is the default theme for new rigs.
	DefaultTheme = "mad-max"
)

// Built-in themes with themed polecat names.
var BuiltinThemes = map[string][]string{
	"mad-max": {
		"furiosa", "nux", "slit", "rictus", "dementus",
		"capable", "toast", "dag", "cheedo", "valkyrie",
		"keeper", "morsov", "ace", "warboy", "imperator",
		"organic", "coma", "splendid", "angharad", "max",
		"immortan", "bullet", "toecutter", "goose", "nightrider",
		"glory", "scrotus", "chumbucket", "corpus", "dinki",
		"prime", "vuvalini", "rockryder", "wretched", "buzzard",
		"gastown", "bullet-farmer", "citadel", "wasteland", "fury",
		"road-warrior", "interceptor", "blackfinger", "wraith", "witness",
		"chrome", "shiny", "mediocre", "guzzoline", "aqua-cola",
	},
	"minerals": {
		"obsidian", "quartz", "jasper", "onyx", "opal",
		"topaz", "garnet", "ruby", "amber", "jade",
		"pearl", "flint", "granite", "basalt", "marble",
		"shale", "slate", "pyrite", "mica", "agate",
		"malachite", "turquoise", "lapis", "emerald", "sapphire",
		"diamond", "amethyst", "citrine", "zircon", "peridot",
		"coral", "jet", "moonstone", "sunstone", "bloodstone",
		"rhodonite", "sodalite", "hematite", "magnetite", "calcite",
		"fluorite", "selenite", "kyanite", "labradorite", "amazonite",
		"chalcedony", "carnelian", "aventurine", "chrysoprase", "heliodor",
	},
	"wasteland": {
		"rust", "chrome", "nitro", "guzzle", "witness",
		"shiny", "fury", "thunder", "dust", "scavenger",
		"radrat", "ghoul", "mutant", "raider", "vault",
		"pipboy", "nuka", "brahmin", "deathclaw", "mirelurk",
		"synth", "institute", "enclave", "brotherhood", "minuteman",
		"railroad", "atom", "crater", "foundation", "refuge",
		"settler", "wanderer", "courier", "lone", "chosen",
		"tribal", "khan", "legion", "ncr", "ranger",
		"overseer", "sentinel", "paladin", "scribe", "initiate",
		"elder", "lancer", "knight", "squire", "proctor",
	},
}

// NamePool manages a bounded pool of reusable polecat NAME SLOTS.
// IMPORTANT: This pools NAMES, not polecats. Polecats are spawned fresh for each
// task and nuked when done - there is no idle pool of polecat instances waiting
// for work. When a polecat is nuked, its name slot becomes available for the next
// freshly-spawned polecat.
//
// Names are drawn from a themed pool (mad-max by default).
// When the pool is exhausted, overflow names use rigname-N format.
type NamePool struct {
	mu sync.RWMutex

	// RigName is the rig this pool belongs to.
	RigName string `json:"rig_name"`

	// Theme is the current theme name (e.g., "mad-max", "minerals").
	Theme string `json:"theme"`

	// CustomNames allows overriding the built-in theme names.
	CustomNames []string `json:"custom_names,omitempty"`

	// InUse tracks which pool names are currently in use.
	// Key is the name itself, value is true if in use.
	// ZFC: This is transient state derived from filesystem via Reconcile().
	// Never persist - always discover from existing polecat directories.
	InUse map[string]bool `json:"-"`

	// Reserved tracks names that have been allocated but not yet instantiated.
	// This prevents race conditions when multiple processes allocate names
	// before the polecat directories are created.
	// Persisted to disk and loaded on startup.
	Reserved map[string]bool `json:"reserved,omitempty"`

	// OverflowNext is the next overflow sequence number.
	// Starts at MaxSize+1 and increments.
	OverflowNext int `json:"overflow_next"`

	// MaxSize is the maximum number of themed names before overflow.
	MaxSize int `json:"max_size"`

	// stateFile is the path to persist pool state.
	stateFile string
}

// NewNamePool creates a new name pool for a rig.
func NewNamePool(rigPath, rigName string) *NamePool {
	return &NamePool{
		RigName:      rigName,
		Theme:        DefaultTheme,
		InUse:        make(map[string]bool),
		Reserved:     make(map[string]bool),
		OverflowNext: DefaultPoolSize + 1,
		MaxSize:      DefaultPoolSize,
		stateFile:    filepath.Join(rigPath, ".runtime", "namepool-state.json"),
	}
}

// NewNamePoolWithConfig creates a name pool with specific configuration.
func NewNamePoolWithConfig(rigPath, rigName, theme string, customNames []string, maxSize int) *NamePool {
	if theme == "" {
		theme = DefaultTheme
	}
	if maxSize <= 0 {
		maxSize = DefaultPoolSize
	}

	return &NamePool{
		RigName:      rigName,
		Theme:        theme,
		CustomNames:  customNames,
		InUse:        make(map[string]bool),
		Reserved:     make(map[string]bool),
		OverflowNext: maxSize + 1,
		MaxSize:      maxSize,
		stateFile:    filepath.Join(rigPath, ".runtime", "namepool-state.json"),
	}
}

// getNames returns the list of names to use for the pool.
func (p *NamePool) getNames() []string {
	// Custom names take precedence
	if len(p.CustomNames) > 0 {
		return p.CustomNames
	}

	// Look up built-in theme
	if names, ok := BuiltinThemes[p.Theme]; ok {
		return names
	}

	// Fall back to default theme
	return BuiltinThemes[DefaultTheme]
}

// Load loads the pool state from disk.
func (p *NamePool) Load() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(p.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Initialize with empty state
			p.InUse = make(map[string]bool)
			p.Reserved = make(map[string]bool)
			p.OverflowNext = p.MaxSize + 1
			return nil
		}
		return err
	}

	var loaded NamePool
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	// Note: Theme and CustomNames are NOT loaded from state file.
	// They are configuration (from settings/config.json), not runtime state.
	// The state file only persists OverflowNext, MaxSize, and Reserved.
	//
	// ZFC: InUse is NEVER loaded from disk - it's transient state derived
	// from filesystem via Reconcile(). Always start with empty map.
	p.InUse = make(map[string]bool)

	// Reserved IS loaded from disk - it tracks names allocated but not yet
	// instantiated (polecat directory not yet created). This prevents race
	// conditions when multiple processes allocate names concurrently.
	if loaded.Reserved != nil {
		p.Reserved = loaded.Reserved
	} else {
		p.Reserved = make(map[string]bool)
	}

	p.OverflowNext = loaded.OverflowNext
	if p.OverflowNext < p.MaxSize+1 {
		p.OverflowNext = p.MaxSize + 1
	}
	if loaded.MaxSize > 0 {
		p.MaxSize = loaded.MaxSize
	}

	return nil
}

// Save persists the pool state to disk using atomic write.
func (p *NamePool) Save() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	dir := filepath.Dir(p.stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return util.AtomicWriteJSON(p.stateFile, p)
}

// Allocate returns a name from the pool.
// It prefers names in order from the theme list, and falls back to overflow names
// when the pool is exhausted.
//
// The allocated name is marked as both InUse (transient) and Reserved (persisted).
// Reserved prevents race conditions when multiple processes allocate names before
// the polecat directories are created. Call ClearReservation after the directory
// is created, or Release when the polecat is removed.
func (p *NamePool) Allocate() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	names := p.getNames()

	// Try to find first available name from the theme
	// A name is available if it's not InUse AND not Reserved
	for i := 0; i < len(names) && i < p.MaxSize; i++ {
		name := names[i]
		if !p.InUse[name] && !p.Reserved[name] {
			p.InUse[name] = true
			p.Reserved[name] = true
			return name, nil
		}
	}

	// Pool exhausted, use overflow naming
	name := p.formatOverflowName(p.OverflowNext)
	p.OverflowNext++
	return name, nil
}

// Release returns a name slot to the available pool.
// Called when a polecat is nuked - the name becomes available for new polecats.
// NOTE: This releases the NAME, not the polecat. The polecat is gone (nuked).
// For overflow names, this is a no-op (they are not reusable).
// Also clears any reservation for the name.
func (p *NamePool) Release(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if it's a themed name
	if p.isThemedName(name) {
		delete(p.InUse, name)
		delete(p.Reserved, name)
	}
	// Overflow names are not reusable, so we don't track them
}

// ClearReservation removes a name from the Reserved set.
// Call this after the polecat directory is successfully created.
// The name remains in InUse (derived from the directory via Reconcile).
func (p *NamePool) ClearReservation(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.Reserved, name)
}

// isThemedName checks if a name is in the theme pool.
func (p *NamePool) isThemedName(name string) bool {
	names := p.getNames()
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// IsPoolName returns true if the name is a pool name (themed or numbered).
func (p *NamePool) IsPoolName(name string) bool {
	return p.isThemedName(name)
}

// ActiveCount returns the number of names currently in use from the pool.
func (p *NamePool) ActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.InUse)
}

// ActiveNames returns a sorted list of names currently in use from the pool.
func (p *NamePool) ActiveNames() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var names []string
	for name := range p.InUse {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MarkInUse marks a name as in use (for reconciling with existing polecats).
func (p *NamePool) MarkInUse(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isThemedName(name) {
		p.InUse[name] = true
	}
}

// Reconcile updates the pool state based on existing polecat directories.
// This should be called on startup to sync pool state with reality.
// Also cleans up stale reservations for names that now have directories.
func (p *NamePool) Reconcile(existingPolecats []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Build set of existing polecats for lookup
	existingSet := make(map[string]bool)
	for _, name := range existingPolecats {
		existingSet[name] = true
	}

	// Clear InUse and rebuild from directories
	p.InUse = make(map[string]bool)

	// Mark all existing polecats as in use
	for _, name := range existingPolecats {
		if p.isThemedName(name) {
			p.InUse[name] = true
		}
	}

	// Clean up stale reservations: if a reserved name now has a directory,
	// the reservation is no longer needed (polecat was successfully created)
	for name := range p.Reserved {
		if existingSet[name] {
			delete(p.Reserved, name)
		}
	}
}

// formatOverflowName formats an overflow sequence number as a name.
func (p *NamePool) formatOverflowName(seq int) string {
	return fmt.Sprintf("%s-%d", p.RigName, seq)
}

// GetTheme returns the current theme name.
func (p *NamePool) GetTheme() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Theme
}

// SetTheme sets the theme and resets the pool.
// Existing in-use names are preserved if they exist in the new theme.
func (p *NamePool) SetTheme(theme string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := BuiltinThemes[theme]; !ok {
		return fmt.Errorf("unknown theme: %s (available: mad-max, minerals, wasteland)", theme)
	}

	// Preserve names that exist in both themes
	newNames := BuiltinThemes[theme]
	newNameSet := make(map[string]bool)
	for _, n := range newNames {
		newNameSet[n] = true
	}

	newInUse := make(map[string]bool)
	for name := range p.InUse {
		if newNameSet[name] {
			newInUse[name] = true
		}
	}

	newReserved := make(map[string]bool)
	for name := range p.Reserved {
		if newNameSet[name] {
			newReserved[name] = true
		}
	}

	p.Theme = theme
	p.InUse = newInUse
	p.Reserved = newReserved
	p.CustomNames = nil
	return nil
}

// ListThemes returns the list of available built-in themes.
func ListThemes() []string {
	themes := make([]string, 0, len(BuiltinThemes))
	for theme := range BuiltinThemes {
		themes = append(themes, theme)
	}
	sort.Strings(themes)
	return themes
}

// GetThemeNames returns the names in a specific theme.
func GetThemeNames(theme string) ([]string, error) {
	if names, ok := BuiltinThemes[theme]; ok {
		return names, nil
	}
	return nil, fmt.Errorf("unknown theme: %s", theme)
}

// AddCustomName adds a custom name to the pool.
func (p *NamePool) AddCustomName(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if already in custom names
	for _, n := range p.CustomNames {
		if n == name {
			return
		}
	}
	p.CustomNames = append(p.CustomNames, name)
}

// Reset clears the pool state, releasing all names.
func (p *NamePool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.InUse = make(map[string]bool)
	p.Reserved = make(map[string]bool)
	p.OverflowNext = p.MaxSize + 1
}
