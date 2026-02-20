package quota

import (
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestPlanRotation_NoLimitedSessions(t *testing.T) {
	setupTestRegistry(t)

	tmux := &mockTmux{
		sessions: []string{"gt-crew-bear", "gt-witness"},
		paneContent: map[string]string{
			"gt-crew-bear": "working normally...",
			"gt-witness":   "watching...",
		},
	}

	accounts := &config.AccountsConfig{
		Accounts: map[string]config.Account{
			"work":     {ConfigDir: "/home/user/.claude-accounts/work"},
			"personal": {ConfigDir: "/home/user/.claude-accounts/personal"},
		},
	}

	scanner, err := NewScanner(tmux, nil, accounts)
	if err != nil {
		t.Fatal(err)
	}

	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	plan, err := PlanRotation(scanner, mgr, accounts)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.LimitedSessions) != 0 {
		t.Errorf("expected 0 limited sessions, got %d", len(plan.LimitedSessions))
	}
	if len(plan.Assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(plan.Assignments))
	}
}

func TestPlanRotation_AssignsAvailableAccount(t *testing.T) {
	setupTestRegistry(t)

	tmux := &mockTmux{
		sessions: []string{"gt-crew-bear", "gt-witness"},
		paneContent: map[string]string{
			"gt-crew-bear": "You've hit your limit · resets 7pm (America/Los_Angeles)",
			"gt-witness":   "watching...",
		},
		envVars: map[string]map[string]string{
			"gt-crew-bear": {"CLAUDE_CONFIG_DIR": "/home/user/.claude-accounts/work"},
			"gt-witness":   {"CLAUDE_CONFIG_DIR": "/home/user/.claude-accounts/personal"},
		},
	}

	accounts := &config.AccountsConfig{
		Accounts: map[string]config.Account{
			"work":     {ConfigDir: "/home/user/.claude-accounts/work"},
			"personal": {ConfigDir: "/home/user/.claude-accounts/personal"},
		},
	}

	scanner, err := NewScanner(tmux, nil, accounts)
	if err != nil {
		t.Fatal(err)
	}

	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// Pre-seed quota state with both accounts available
	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"work":     {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T02:00:00Z"},
			"personal": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T01:00:00Z"},
		},
	}
	if err := mgr.Save(state); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanRotation(scanner, mgr, accounts)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.LimitedSessions) != 1 {
		t.Fatalf("expected 1 limited session, got %d", len(plan.LimitedSessions))
	}
	if plan.LimitedSessions[0].Session != "gt-crew-bear" {
		t.Errorf("expected limited session gt-crew-bear, got %s", plan.LimitedSessions[0].Session)
	}

	if len(plan.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(plan.Assignments))
	}

	newAccount, ok := plan.Assignments["gt-crew-bear"]
	if !ok {
		t.Fatal("expected assignment for gt-crew-bear")
	}
	// Should assign "personal" since "work" is now limited
	if newAccount != "personal" {
		t.Errorf("expected assignment to 'personal', got %q", newAccount)
	}
}

func TestPlanRotation_NoAvailableAccounts(t *testing.T) {
	setupTestRegistry(t)

	tmux := &mockTmux{
		sessions: []string{"gt-crew-bear"},
		paneContent: map[string]string{
			"gt-crew-bear": "You've hit your limit",
		},
		envVars: map[string]map[string]string{
			"gt-crew-bear": {"CLAUDE_CONFIG_DIR": "/home/user/.claude-accounts/work"},
		},
	}

	accounts := &config.AccountsConfig{
		Accounts: map[string]config.Account{
			"work": {ConfigDir: "/home/user/.claude-accounts/work"},
		},
	}

	scanner, err := NewScanner(tmux, nil, accounts)
	if err != nil {
		t.Fatal(err)
	}

	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// Only one account and it's limited
	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"work": {Status: config.QuotaStatusAvailable},
		},
	}
	if err := mgr.Save(state); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanRotation(scanner, mgr, accounts)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.LimitedSessions) != 1 {
		t.Fatalf("expected 1 limited session, got %d", len(plan.LimitedSessions))
	}
	// No assignments because there's no other account to rotate to
	if len(plan.Assignments) != 0 {
		t.Errorf("expected 0 assignments (no available accounts), got %d", len(plan.Assignments))
	}
}

func TestPlanRotation_SkipsSameAccount(t *testing.T) {
	setupTestRegistry(t)

	tmux := &mockTmux{
		sessions: []string{"gt-crew-bear"},
		paneContent: map[string]string{
			"gt-crew-bear": "You've hit your limit",
		},
		envVars: map[string]map[string]string{
			"gt-crew-bear": {"CLAUDE_CONFIG_DIR": "/home/user/.claude-accounts/alpha"},
		},
	}

	accounts := &config.AccountsConfig{
		Accounts: map[string]config.Account{
			"alpha": {ConfigDir: "/home/user/.claude-accounts/alpha"},
			"beta":  {ConfigDir: "/home/user/.claude-accounts/beta"},
			"gamma": {ConfigDir: "/home/user/.claude-accounts/gamma"},
		},
	}

	scanner, err := NewScanner(tmux, nil, accounts)
	if err != nil {
		t.Fatal(err)
	}

	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// alpha is LRU (oldest) but is the session's current account
	// Should skip alpha and assign beta (next LRU)
	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"alpha": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T01:00:00Z"},
			"beta":  {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T02:00:00Z"},
			"gamma": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T03:00:00Z"},
		},
	}
	if err := mgr.Save(state); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanRotation(scanner, mgr, accounts)
	if err != nil {
		t.Fatal(err)
	}

	newAccount, ok := plan.Assignments["gt-crew-bear"]
	if !ok {
		t.Fatal("expected assignment for gt-crew-bear")
	}
	// Should skip alpha (same account), assign beta
	if newAccount != "beta" {
		t.Errorf("expected assignment to 'beta' (skipping same account), got %q", newAccount)
	}
}

func TestPlanRotation_MultipleLimitedSessions(t *testing.T) {
	setupTestRegistry(t)

	tmux := &mockTmux{
		sessions: []string{"hq-mayor", "gt-crew-bear", "gt-crew-wolf"},
		paneContent: map[string]string{
			"hq-mayor":     "You've hit your limit · resets 7pm",
			"gt-crew-bear": "You've hit your limit · resets 7pm",
			"gt-crew-wolf": "working fine...",
		},
		envVars: map[string]map[string]string{
			"hq-mayor":     {"CLAUDE_CONFIG_DIR": "/home/user/.claude-accounts/alpha"},
			"gt-crew-bear": {"CLAUDE_CONFIG_DIR": "/home/user/.claude-accounts/alpha"},
			"gt-crew-wolf": {"CLAUDE_CONFIG_DIR": "/home/user/.claude-accounts/beta"},
		},
	}

	accounts := &config.AccountsConfig{
		Accounts: map[string]config.Account{
			"alpha": {ConfigDir: "/home/user/.claude-accounts/alpha"},
			"beta":  {ConfigDir: "/home/user/.claude-accounts/beta"},
			"gamma": {ConfigDir: "/home/user/.claude-accounts/gamma"},
		},
	}

	scanner, err := NewScanner(tmux, nil, accounts)
	if err != nil {
		t.Fatal(err)
	}

	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"alpha": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T01:00:00Z"},
			"beta":  {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T02:00:00Z"},
			"gamma": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T03:00:00Z"},
		},
	}
	if err := mgr.Save(state); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanRotation(scanner, mgr, accounts)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.LimitedSessions) != 2 {
		t.Fatalf("expected 2 limited sessions, got %d", len(plan.LimitedSessions))
	}

	// Both limited sessions should be assigned to the same account (beta, LRU available)
	if len(plan.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(plan.Assignments))
	}
	for session, acct := range plan.Assignments {
		if acct != "beta" {
			t.Errorf("expected session %s assigned to 'beta', got %q", session, acct)
		}
	}
}
