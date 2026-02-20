// Package quota manages Claude Code account quota rotation for Gas Town.
//
// When sessions hit rate limits, the overseer can scan for blocked sessions
// and rotate them to available accounts. State is persisted to mayor/quota.json
// with crash-safe atomic writes and file-level locking.
package quota

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/util"
)

// Manager handles quota state persistence with file locking.
type Manager struct {
	townRoot string
}

// NewManager creates a new quota manager for the given town root.
func NewManager(townRoot string) *Manager {
	return &Manager{townRoot: townRoot}
}

// statePath returns the path to quota.json.
func (m *Manager) statePath() string {
	return constants.MayorQuotaPath(m.townRoot)
}

// lockPath returns the path to the flock file for quota state.
func (m *Manager) lockPath() string {
	return filepath.Join(m.townRoot, constants.DirMayor, constants.DirRuntime, "quota.lock")
}

// lock acquires an exclusive file lock for quota state operations.
// Caller must defer unlock().
func (m *Manager) lock() (func(), error) {
	lockDir := filepath.Dir(m.lockPath())
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("creating quota lock dir: %w", err)
	}
	fl := flock.New(m.lockPath())
	if err := fl.Lock(); err != nil {
		return nil, fmt.Errorf("acquiring quota lock: %w", err)
	}
	return func() { _ = fl.Unlock() }, nil
}

// Load reads the quota state from disk. Returns an empty state if the file
// doesn't exist yet (first run).
func (m *Manager) Load() (*config.QuotaState, error) {
	data, err := os.ReadFile(m.statePath())
	if os.IsNotExist(err) {
		return &config.QuotaState{
			Version:  config.CurrentQuotaVersion,
			Accounts: make(map[string]config.AccountQuotaState),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading quota state: %w", err)
	}

	var state config.QuotaState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing quota state: %w", err)
	}
	if state.Accounts == nil {
		state.Accounts = make(map[string]config.AccountQuotaState)
	}
	return &state, nil
}

// Save writes the quota state to disk atomically with file locking.
func (m *Manager) Save(state *config.QuotaState) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()

	state.Version = config.CurrentQuotaVersion
	return util.EnsureDirAndWriteJSON(m.statePath(), state)
}

// WithLock acquires the quota file lock, runs fn, then releases the lock.
// Use this to hold the lock across multiple Load/SaveUnlocked calls,
// eliminating TOCTOU races in multi-step operations like rotation.
func (m *Manager) WithLock(fn func() error) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

// SaveUnlocked writes the quota state to disk without acquiring the lock.
// The caller MUST already hold the lock via WithLock. Using this outside
// of WithLock will corrupt state under concurrent access.
func (m *Manager) SaveUnlocked(state *config.QuotaState) error {
	state.Version = config.CurrentQuotaVersion
	return util.EnsureDirAndWriteJSON(m.statePath(), state)
}

// MarkLimited marks an account as rate-limited with an optional reset time.
func (m *Manager) MarkLimited(handle string, resetsAt string) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()

	state, err := m.Load()
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	state.Accounts[handle] = config.AccountQuotaState{
		Status:    config.QuotaStatusLimited,
		LimitedAt: now,
		ResetsAt:  resetsAt,
		LastUsed:  state.Accounts[handle].LastUsed,
	}

	return util.EnsureDirAndWriteJSON(m.statePath(), state)
}

// MarkAvailable marks an account as available (not rate-limited).
func (m *Manager) MarkAvailable(handle string) error {
	unlock, err := m.lock()
	if err != nil {
		return err
	}
	defer unlock()

	state, err := m.Load()
	if err != nil {
		return err
	}

	existing := state.Accounts[handle]
	state.Accounts[handle] = config.AccountQuotaState{
		Status:   config.QuotaStatusAvailable,
		LastUsed: existing.LastUsed,
	}

	return util.EnsureDirAndWriteJSON(m.statePath(), state)
}

// AvailableAccounts returns account handles that are not rate-limited,
// sorted by least-recently-used first.
func (m *Manager) AvailableAccounts(state *config.QuotaState) []string {
	var available []string
	for handle, acctState := range state.Accounts {
		if acctState.Status == config.QuotaStatusAvailable || acctState.Status == "" {
			available = append(available, handle)
		}
	}
	// Sort by LastUsed ascending (least recently used first)
	sortByLastUsed(available, state)
	return available
}

// LimitedAccounts returns account handles that are currently rate-limited.
func (m *Manager) LimitedAccounts(state *config.QuotaState) []string {
	var limited []string
	for handle, acctState := range state.Accounts {
		if acctState.Status == config.QuotaStatusLimited {
			limited = append(limited, handle)
		}
	}
	return limited
}

// sortByLastUsed sorts handles by their LastUsed timestamp ascending.
func sortByLastUsed(handles []string, state *config.QuotaState) {
	// Simple insertion sort — handles list is small (3-5 accounts)
	for i := 1; i < len(handles); i++ {
		key := handles[i]
		j := i - 1
		for j >= 0 && state.Accounts[handles[j]].LastUsed > state.Accounts[key].LastUsed {
			handles[j+1] = handles[j]
			j--
		}
		handles[j+1] = key
	}
}

// EnsureAccountsTracked adds any registered accounts that are missing from
// quota state. Called during scan to keep state in sync with accounts.json.
func (m *Manager) EnsureAccountsTracked(state *config.QuotaState, accounts map[string]config.Account) {
	for handle := range accounts {
		if _, exists := state.Accounts[handle]; !exists {
			state.Accounts[handle] = config.AccountQuotaState{
				Status: config.QuotaStatusAvailable,
			}
		}
	}
}
