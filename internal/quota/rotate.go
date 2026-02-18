package quota

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/util"
)

// RotateResult holds the result of rotating a single session.
type RotateResult struct {
	Session        string `json:"session"`                  // tmux session name
	OldAccount     string `json:"old_account,omitempty"`    // previous account handle
	NewAccount     string `json:"new_account,omitempty"`    // new account handle
	Rotated        bool   `json:"rotated"`                  // whether rotation occurred
	ResumedSession string `json:"resumed_session,omitempty"` // session ID that was resumed (empty if fresh start)
	KeychainSwap   bool   `json:"keychain_swap,omitempty"`   // whether keychain was swapped
	Error          string `json:"error,omitempty"`          // error message if rotation failed
}

// RotatePlan describes what the rotator will do.
type RotatePlan struct {
	// LimitedSessions are sessions detected as rate-limited.
	LimitedSessions []ScanResult

	// AvailableAccounts are accounts that can be rotated to.
	AvailableAccounts []string

	// Assignments maps session -> new account handle.
	Assignments map[string]string

	// ConfigDirSwaps maps config_dir -> new account handle.
	// One keychain swap per config dir, not per session.
	// All sessions sharing a config dir get the same assignment.
	ConfigDirSwaps map[string]string
}

// PlanRotation scans for limited sessions and plans account assignments.
// Returns a plan that can be reviewed before execution.
func PlanRotation(scanner *Scanner, mgr *Manager, acctCfg *config.AccountsConfig) (*RotatePlan, error) {
	// Scan for rate-limited sessions
	results, err := scanner.ScanAll()
	if err != nil {
		return nil, fmt.Errorf("scanning sessions: %w", err)
	}

	// Load quota state
	state, err := mgr.Load()
	if err != nil {
		return nil, fmt.Errorf("loading quota state: %w", err)
	}
	mgr.EnsureAccountsTracked(state, acctCfg.Accounts)

	// Find limited sessions
	var limitedSessions []ScanResult
	for _, r := range results {
		if r.RateLimited {
			limitedSessions = append(limitedSessions, r)
		}
	}

	// Update state: mark detected limited accounts
	for _, r := range limitedSessions {
		if r.AccountHandle != "" {
			state.Accounts[r.AccountHandle] = config.AccountQuotaState{
				Status:    config.QuotaStatusLimited,
				LimitedAt: state.Accounts[r.AccountHandle].LimitedAt,
				ResetsAt:  r.ResetsAt,
				LastUsed:  state.Accounts[r.AccountHandle].LastUsed,
			}
		}
	}

	// Get available accounts
	available := mgr.AvailableAccounts(state)

	// Collect unique config dirs from limited sessions.
	// Multiple sessions can share the same config dir (via the same account).
	// We only need one keychain swap per config dir.
	type configDirInfo struct {
		configDir     string // resolved config dir path
		accountHandle string // the limited account using this config dir
	}
	uniqueConfigDirs := make(map[string]*configDirInfo) // configDir -> info
	for _, r := range limitedSessions {
		if r.AccountHandle == "" {
			continue
		}
		acct, ok := acctCfg.Accounts[r.AccountHandle]
		if !ok {
			continue
		}
		configDir := util.ExpandHome(acct.ConfigDir)
		if _, exists := uniqueConfigDirs[configDir]; !exists {
			uniqueConfigDirs[configDir] = &configDirInfo{
				configDir:     configDir,
				accountHandle: r.AccountHandle,
			}
		}
	}

	// Assign available accounts to unique config dirs (round-robin, skip same-account).
	configDirSwaps := make(map[string]string) // configDir -> new account handle
	availIdx := 0
	for configDir, info := range uniqueConfigDirs {
		if availIdx >= len(available) {
			break
		}
		candidate := available[availIdx]
		if candidate == info.accountHandle {
			availIdx++
			if availIdx >= len(available) {
				break
			}
			candidate = available[availIdx]
		}
		configDirSwaps[configDir] = candidate
		availIdx++
	}

	// Expand config dir assignments to session-level assignments.
	assignments := make(map[string]string)
	for _, r := range limitedSessions {
		if r.AccountHandle == "" {
			continue
		}
		acct, ok := acctCfg.Accounts[r.AccountHandle]
		if !ok {
			continue
		}
		configDir := util.ExpandHome(acct.ConfigDir)
		if newAccount, ok := configDirSwaps[configDir]; ok {
			assignments[r.Session] = newAccount
		}
	}

	return &RotatePlan{
		LimitedSessions:   limitedSessions,
		AvailableAccounts: available,
		Assignments:       assignments,
		ConfigDirSwaps:    configDirSwaps,
	}, nil
}
