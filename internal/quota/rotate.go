package quota

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

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
	// LimitedSessions are sessions detected as hard rate-limited.
	LimitedSessions []ScanResult

	// NearLimitSessions are sessions approaching their rate limit.
	// Only populated when PlanOpts.IncludeNearLimit is true.
	NearLimitSessions []ScanResult `json:"near_limit_sessions,omitempty"`

	// AvailableAccounts are accounts that can be rotated to.
	AvailableAccounts []string

	// Assignments maps session -> new account handle.
	Assignments map[string]string

	// ConfigDirSwaps maps config_dir -> new account handle.
	// One keychain swap per config dir, not per session.
	// All sessions sharing a config dir get the same assignment.
	ConfigDirSwaps map[string]string

	// SkippedAccounts maps handle -> reason for accounts that were
	// available by quota status but had invalid/expired tokens.
	SkippedAccounts map[string]string `json:"skipped_accounts,omitempty"`
}

// PlanOpts configures the rotation planning behavior.
type PlanOpts struct {
	// FromAccount targets all sessions using this account regardless of
	// rate-limit status (preemptive rotation). Empty string = default behavior.
	FromAccount string

	// IncludeNearLimit includes sessions approaching their rate limit
	// (not just hard-limited sessions) as rotation candidates.
	IncludeNearLimit bool
}

// PlanRotation scans for limited sessions and plans account assignments.
// The opts parameter controls targeting behavior:
//   - opts.FromAccount: targets all sessions using that account regardless of limit status
//   - opts.IncludeNearLimit: also targets sessions approaching their limit
//
// Returns a plan that can be reviewed before execution.
func PlanRotation(scanner *Scanner, mgr *Manager, acctCfg *config.AccountsConfig, opts PlanOpts) (*RotatePlan, error) {
	// Scan for rate-limited and near-limit sessions
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

	// Auto-clear accounts whose reset time has passed so they
	// become available for rotation.
	mgr.ClearExpired(state)

	// Find target sessions based on opts.
	var limitedSessions []ScanResult
	var nearLimitSessions []ScanResult
	for _, r := range results {
		if opts.FromAccount != "" {
			// Preemptive: target all sessions using the specified account
			if r.AccountHandle == opts.FromAccount {
				limitedSessions = append(limitedSessions, r)
			}
		} else {
			// Reactive: target rate-limited sessions
			if r.RateLimited {
				limitedSessions = append(limitedSessions, r)
			} else if r.NearLimit {
				nearLimitSessions = append(nearLimitSessions, r)
			}
		}
	}

	// Combine limited + near-limit sessions for assignment planning
	targetSessions := limitedSessions
	if opts.IncludeNearLimit {
		targetSessions = append(targetSessions, nearLimitSessions...)
	}

	// Available accounts come from persisted state only — NOT from scan
	// detections. Stale sessions (e.g., parked rigs with old rate-limit
	// messages still in the pane) would otherwise mark their accounts as
	// limited, shrinking the available pool and blocking rotation of
	// sessions that actually need it.
	//
	// The caller persists confirmed rate-limit state after execution.
	available := mgr.AvailableAccounts(state)

	// Validate tokens for available accounts — skip accounts with expired or
	// revoked tokens. This prevents swapping a bad token into the target's
	// keychain entry, which would leave the session non-functional.
	skipped := make(map[string]string)
	var validAvailable []string
	for _, handle := range available {
		if handle == opts.FromAccount {
			continue // rotating away from this account, not a candidate
		}
		acct, ok := acctCfg.Accounts[handle]
		if !ok {
			continue
		}
		configDir := util.ExpandHome(acct.ConfigDir)
		if err := ValidateKeychainToken(configDir); err != nil {
			skipped[handle] = err.Error()
			continue
		}
		validAvailable = append(validAvailable, handle)
	}
	available = validAvailable

	// Collect unique config dirs from target sessions.
	// Multiple sessions can share the same config dir (via the same account).
	// We only need one keychain swap per config dir.
	// Sessions with unknown accounts are included if they have a CLAUDE_CONFIG_DIR.
	type configDirInfo struct {
		configDir     string // resolved config dir path
		accountHandle string // the limited account using this config dir (may be empty)
	}
	uniqueConfigDirs := make(map[string]*configDirInfo) // configDir -> info
	for _, r := range targetSessions {
		var configDir string
		if r.AccountHandle != "" {
			acct, ok := acctCfg.Accounts[r.AccountHandle]
			if !ok {
				continue
			}
			configDir = util.ExpandHome(acct.ConfigDir)
		} else if r.ConfigDir != "" {
			// Unknown account but we have the config dir from tmux
			configDir = r.ConfigDir
		} else {
			continue // No account and no config dir — can't rotate
		}
		if _, exists := uniqueConfigDirs[configDir]; !exists {
			uniqueConfigDirs[configDir] = &configDirInfo{
				configDir:     configDir,
				accountHandle: r.AccountHandle,
			}
		}
	}

	// Assign available accounts to unique config dirs (round-robin, skip same-account).
	// Sort config dirs for deterministic assignment order.
	configDirSwaps := make(map[string]string) // configDir -> new account handle
	availIdx := 0
	for _, configDir := range slices.Sorted(maps.Keys(uniqueConfigDirs)) {
		info := uniqueConfigDirs[configDir]
		if availIdx >= len(available) {
			break
		}
		candidate := available[availIdx]
		if candidate == info.accountHandle {
			availIdx++
			if availIdx >= len(available) {
				break
			}
			candidate = available[availIdx] // re-read after skip
		}
		configDirSwaps[configDir] = candidate
		availIdx++
	}

	// Expand config dir assignments to session-level assignments.
	assignments := make(map[string]string)
	for _, r := range targetSessions {
		var configDir string
		if r.AccountHandle != "" {
			acct, ok := acctCfg.Accounts[r.AccountHandle]
			if !ok {
				continue
			}
			configDir = util.ExpandHome(acct.ConfigDir)
		} else if r.ConfigDir != "" {
			configDir = r.ConfigDir
		} else {
			continue
		}
		if newAccount, ok := configDirSwaps[configDir]; ok {
			assignments[r.Session] = newAccount
		}
	}

	return &RotatePlan{
		LimitedSessions:   limitedSessions,
		NearLimitSessions: nearLimitSessions,
		AvailableAccounts: available,
		Assignments:       assignments,
		ConfigDirSwaps:    configDirSwaps,
		SkippedAccounts:   skipped,
	}, nil
}

// BalanceOpts configures the balance planning behavior.
type BalanceOpts struct {
	// MaxSessions caps sessions per account. Key is account handle.
	MaxSessions map[string]int

	// SharePct sets target percentage per account. Key is account handle.
	// Mutually exclusive with MaxSessions.
	SharePct map[string]int

	// ToAccount consolidates all sessions to a single account.
	ToAccount string

	// Force includes busy sessions (not just idle/limited).
	Force bool
}

// BalancePlan describes what the balancer will do.
type BalancePlan struct {
	// Assignments maps session -> new account handle for sessions that need to move.
	Assignments map[string]string

	// CurrentDistribution maps account handle -> list of session names.
	CurrentDistribution map[string][]string `json:"current_distribution"`

	// TargetCounts maps account handle -> target session count.
	TargetCounts map[string]int `json:"target_counts"`
}

// PlanBalance scans all sessions and plans redistribution across accounts.
// Unlike PlanRotation, this is proactive — it moves sessions regardless of
// rate-limit status to achieve the desired distribution.
func PlanBalance(scanner *Scanner, tmuxClient TmuxIdleChecker, acctCfg *config.AccountsConfig, opts BalanceOpts) (*BalancePlan, error) {
	results, err := scanner.ScanAll()
	if err != nil {
		return nil, fmt.Errorf("scanning sessions: %w", err)
	}

	// Build current distribution: account -> sessions
	currentDist := make(map[string][]string)
	sessionAccount := make(map[string]string)   // session -> current account
	sessionLimited := make(map[string]bool)     // session -> is rate-limited
	for _, r := range results {
		if r.AccountHandle == "" {
			continue // can't balance sessions with unknown accounts
		}
		currentDist[r.AccountHandle] = append(currentDist[r.AccountHandle], r.Session)
		sessionAccount[r.Session] = r.AccountHandle
		if r.RateLimited {
			sessionLimited[r.Session] = true
		}
	}

	totalSessions := len(sessionAccount)
	if totalSessions == 0 {
		return &BalancePlan{
			Assignments:         make(map[string]string),
			CurrentDistribution: currentDist,
			TargetCounts:        make(map[string]int),
		}, nil
	}

	// Compute target counts per account (sorted for deterministic plans)
	accounts := slices.Sorted(maps.Keys(acctCfg.Accounts))

	targetCounts := make(map[string]int)

	switch {
	case opts.ToAccount != "":
		// Consolidate: all sessions to one account
		for _, handle := range accounts {
			if handle == opts.ToAccount {
				targetCounts[handle] = totalSessions
			} else {
				targetCounts[handle] = 0
			}
		}

	case len(opts.MaxSessions) > 0:
		// Cap specified accounts, distribute remainder evenly
		capped := 0
		uncappedAccounts := 0
		for _, handle := range accounts {
			if max, ok := opts.MaxSessions[handle]; ok {
				targetCounts[handle] = max
				capped += max
			} else {
				uncappedAccounts++
			}
		}
		remainder := totalSessions - capped
		if remainder < 0 {
			remainder = 0
		}
		if uncappedAccounts > 0 {
			perUncapped := remainder / uncappedAccounts
			extra := remainder % uncappedAccounts
			i := 0
			for _, handle := range accounts {
				if _, ok := opts.MaxSessions[handle]; !ok {
					targetCounts[handle] = perUncapped
					if i < extra {
						targetCounts[handle]++
					}
					i++
				}
			}
		}

	case len(opts.SharePct) > 0:
		// Percentage-based distribution
		for _, handle := range accounts {
			if pct, ok := opts.SharePct[handle]; ok {
				targetCounts[handle] = (totalSessions * pct + 50) / 100 // round
			}
		}
		// Distribute remaining percentage evenly to unspecified accounts
		specified := 0
		unspecified := 0
		for _, handle := range accounts {
			if _, ok := opts.SharePct[handle]; ok {
				specified += targetCounts[handle]
			} else {
				unspecified++
			}
		}
		remainder := totalSessions - specified
		if remainder < 0 {
			remainder = 0
		}
		if unspecified > 0 {
			perUnspec := remainder / unspecified
			extra := remainder % unspecified
			i := 0
			for _, handle := range accounts {
				if _, ok := opts.SharePct[handle]; !ok {
					targetCounts[handle] = perUnspec
					if i < extra {
						targetCounts[handle]++
					}
					i++
				}
			}
		}

	default:
		// Even split
		perAccount := totalSessions / len(accounts)
		extra := totalSessions % len(accounts)
		for i, handle := range accounts {
			targetCounts[handle] = perAccount
			if i < extra {
				targetCounts[handle]++
			}
		}
	}

	// Validate tokens for target accounts — skip accounts with expired or
	// revoked tokens to avoid swapping bad credentials into sessions.
	validAccounts := make(map[string]bool)
	for _, handle := range accounts {
		acct := acctCfg.Accounts[handle]
		configDir := util.ExpandHome(acct.ConfigDir)
		if err := ValidateKeychainToken(configDir); err == nil {
			validAccounts[handle] = true
		}
	}

	// Identify overloaded (above target) and underloaded (below target) accounts
	type moveCandidate struct {
		session string
		tier    int // 1=limited, 2=idle, 3=busy
	}

	assignments := make(map[string]string)

	// Collect sessions to move from overloaded accounts, prioritized by tier
	var candidates []moveCandidate
	for _, handle := range accounts {
		current := len(currentDist[handle])
		target := targetCounts[handle]
		excess := current - target
		if excess <= 0 {
			continue
		}

		// Prioritize: limited first, then idle, then busy
		var limited, idle, busy []string
		for _, session := range currentDist[handle] {
			if sessionLimited[session] {
				limited = append(limited, session)
			} else if tmuxClient != nil && tmuxClient.IsIdle(session) {
				idle = append(idle, session)
			} else {
				busy = append(busy, session)
			}
		}

		picked := 0
		for _, s := range limited {
			if picked >= excess {
				break
			}
			candidates = append(candidates, moveCandidate{session: s, tier: 1})
			picked++
		}
		for _, s := range idle {
			if picked >= excess {
				break
			}
			candidates = append(candidates, moveCandidate{session: s, tier: 2})
			picked++
		}
		if opts.Force {
			for _, s := range busy {
				if picked >= excess {
					break
				}
				candidates = append(candidates, moveCandidate{session: s, tier: 3})
				picked++
			}
		}
	}

	// Assign candidates to underloaded accounts
	for _, c := range candidates {
		// Find the most underloaded account
		bestHandle := ""
		bestDeficit := 0
		for _, handle := range accounts {
			current := len(currentDist[handle])
			// Count already-assigned incoming sessions
			incoming := 0
			for _, target := range assignments {
				if target == handle {
					incoming++
				}
			}
			// Count already-assigned outgoing sessions
			outgoing := 0
			for session := range assignments {
				if sessionAccount[session] == handle {
					outgoing++
				}
			}
			effective := current + incoming - outgoing
			deficit := targetCounts[handle] - effective
			if deficit > bestDeficit && handle != sessionAccount[c.session] && validAccounts[handle] {
				bestDeficit = deficit
				bestHandle = handle
			}
		}
		if bestHandle != "" {
			assignments[c.session] = bestHandle
		}
	}

	return &BalancePlan{
		Assignments:         assignments,
		CurrentDistribution: currentDist,
		TargetCounts:        targetCounts,
	}, nil
}

// FilterAssignmentsByGlob removes entries from assignments whose session name
// does not match any of the given glob patterns. Uses filepath.Match semantics.
// Note: filepath.Match does not support ** (recursive glob) — only * (single segment).
func FilterAssignmentsByGlob(assignments map[string]string, patterns []string) {
	for session := range assignments {
		matched := false
		for _, pattern := range patterns {
			if ok, _ := filepath.Match(pattern, session); ok {
				matched = true
				break
			}
		}
		if !matched {
			delete(assignments, session)
		}
	}
}
