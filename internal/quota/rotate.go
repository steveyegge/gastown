package quota

import (
	"fmt"

	"github.com/steveyegge/gastown/internal/config"
)

// RotateResult holds the result of rotating a single session.
type RotateResult struct {
	Session        string `json:"session"`                  // tmux session name
	OldAccount     string `json:"old_account,omitempty"`    // previous account handle
	NewAccount     string `json:"new_account,omitempty"`    // new account handle
	Rotated        bool   `json:"rotated"`                  // whether rotation occurred
	ResumedSession string `json:"resumed_session,omitempty"` // session ID that was resumed (empty if fresh start)
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

	// Plan assignments: assign all limited sessions to the best available account.
	// Strategy: pick the first available account (LRU) that isn't already the
	// session's current account. All sessions rotate to the same account so the
	// operator can drain one account at a time, then move on.
	assignments := make(map[string]string)
	if len(available) > 0 {
		for _, r := range limitedSessions {
			// Find the first available account that differs from current
			for _, candidate := range available {
				if candidate != r.AccountHandle {
					assignments[r.Session] = candidate
					break
				}
			}
		}
	}

	return &RotatePlan{
		LimitedSessions:   limitedSessions,
		AvailableAccounts: available,
		Assignments:       assignments,
	}, nil
}
