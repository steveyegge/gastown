package quota

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
)

// setupTestTown creates a temporary town root with mayor directory.
func setupTestTown(t *testing.T) string {
	t.Helper()
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, constants.DirMayor)
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	return townRoot
}

func TestNewManager(t *testing.T) {
	mgr := NewManager("/tmp/test-town")
	if mgr.townRoot != "/tmp/test-town" {
		t.Errorf("expected townRoot /tmp/test-town, got %s", mgr.townRoot)
	}
}

func TestLoadEmpty(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	state, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if state.Version != config.CurrentQuotaVersion {
		t.Errorf("expected version %d, got %d", config.CurrentQuotaVersion, state.Version)
	}
	if len(state.Accounts) != 0 {
		t.Errorf("expected empty accounts, got %d", len(state.Accounts))
	}
}

func TestSaveAndLoad(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"work": {
				Status:   config.QuotaStatusAvailable,
				LastUsed: "2025-01-01T00:00:00Z",
			},
			"personal": {
				Status:    config.QuotaStatusLimited,
				LimitedAt: "2025-01-01T12:00:00Z",
				ResetsAt:  "2025-01-01T13:00:00Z",
			},
		},
	}

	if err := mgr.Save(state); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() after Save() error: %v", err)
	}

	if loaded.Version != config.CurrentQuotaVersion {
		t.Errorf("expected version %d, got %d", config.CurrentQuotaVersion, loaded.Version)
	}
	if len(loaded.Accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(loaded.Accounts))
	}
	if loaded.Accounts["work"].Status != config.QuotaStatusAvailable {
		t.Errorf("expected work status available, got %s", loaded.Accounts["work"].Status)
	}
	if loaded.Accounts["personal"].Status != config.QuotaStatusLimited {
		t.Errorf("expected personal status limited, got %s", loaded.Accounts["personal"].Status)
	}
}

func TestMarkLimited(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// Save initial state
	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"work": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T00:00:00Z"},
		},
	}
	if err := mgr.Save(state); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Mark as limited
	if err := mgr.MarkLimited("work", "7:00 PM PST"); err != nil {
		t.Fatalf("MarkLimited() error: %v", err)
	}

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	acct := loaded.Accounts["work"]
	if acct.Status != config.QuotaStatusLimited {
		t.Errorf("expected status limited, got %s", acct.Status)
	}
	if acct.LimitedAt == "" {
		t.Error("expected LimitedAt to be set")
	}
	if acct.ResetsAt != "7:00 PM PST" {
		t.Errorf("expected ResetsAt '7:00 PM PST', got %q", acct.ResetsAt)
	}
	// LastUsed should be preserved
	if acct.LastUsed != "2025-01-01T00:00:00Z" {
		t.Errorf("expected LastUsed preserved, got %q", acct.LastUsed)
	}
}

func TestMarkAvailable(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// Save initial state with limited account
	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"work": {
				Status:    config.QuotaStatusLimited,
				LimitedAt: "2025-01-01T12:00:00Z",
				LastUsed:  "2025-01-01T11:00:00Z",
			},
		},
	}
	if err := mgr.Save(state); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	if err := mgr.MarkAvailable("work"); err != nil {
		t.Fatalf("MarkAvailable() error: %v", err)
	}

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	acct := loaded.Accounts["work"]
	if acct.Status != config.QuotaStatusAvailable {
		t.Errorf("expected status available, got %s", acct.Status)
	}
	if acct.LimitedAt != "" {
		t.Errorf("expected LimitedAt cleared, got %q", acct.LimitedAt)
	}
	// LastUsed should be preserved
	if acct.LastUsed != "2025-01-01T11:00:00Z" {
		t.Errorf("expected LastUsed preserved, got %q", acct.LastUsed)
	}
}

func TestAvailableAccounts(t *testing.T) {
	mgr := NewManager("/tmp/unused")
	state := &config.QuotaState{
		Accounts: map[string]config.AccountQuotaState{
			"a": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T03:00:00Z"},
			"b": {Status: config.QuotaStatusLimited},
			"c": {Status: config.QuotaStatusAvailable, LastUsed: "2025-01-01T01:00:00Z"},
			"d": {Status: "", LastUsed: "2025-01-01T02:00:00Z"}, // empty status = available
		},
	}

	available := mgr.AvailableAccounts(state)
	if len(available) != 3 {
		t.Fatalf("expected 3 available, got %d: %v", len(available), available)
	}
	// Should be sorted by LastUsed ascending
	if available[0] != "c" {
		t.Errorf("expected first available 'c' (oldest), got %q", available[0])
	}
	if available[1] != "d" {
		t.Errorf("expected second available 'd', got %q", available[1])
	}
	if available[2] != "a" {
		t.Errorf("expected third available 'a' (newest), got %q", available[2])
	}
}

func TestSortByLastUsed_EmptyStrings(t *testing.T) {
	state := &config.QuotaState{
		Accounts: map[string]config.AccountQuotaState{
			"a": {LastUsed: "2025-01-01T03:00:00Z"},
			"b": {LastUsed: ""},
			"c": {LastUsed: "2025-01-01T01:00:00Z"},
		},
	}
	handles := []string{"a", "b", "c"}
	sortByLastUsed(handles, state)

	// Empty LastUsed sorts first (least recently used = highest priority).
	if handles[0] != "b" {
		t.Errorf("expected 'b' (empty LastUsed) first, got %q", handles[0])
	}
	if handles[1] != "c" {
		t.Errorf("expected 'c' second, got %q", handles[1])
	}
	if handles[2] != "a" {
		t.Errorf("expected 'a' (most recent) last, got %q", handles[2])
	}
}

func TestLimitedAccounts(t *testing.T) {
	mgr := NewManager("/tmp/unused")
	state := &config.QuotaState{
		Accounts: map[string]config.AccountQuotaState{
			"a": {Status: config.QuotaStatusAvailable},
			"b": {Status: config.QuotaStatusLimited},
			"c": {Status: config.QuotaStatusLimited},
			"d": {Status: config.QuotaStatusCooldown},
		},
	}

	limited := mgr.LimitedAccounts(state)
	if len(limited) != 2 {
		t.Fatalf("expected 2 limited, got %d: %v", len(limited), limited)
	}
}

func TestEnsureAccountsTracked(t *testing.T) {
	mgr := NewManager("/tmp/unused")
	state := &config.QuotaState{
		Accounts: map[string]config.AccountQuotaState{
			"existing": {Status: config.QuotaStatusLimited, LimitedAt: "2025-01-01T00:00:00Z"},
		},
	}

	accounts := map[string]config.Account{
		"existing": {Email: "a@test.com"},
		"new":      {Email: "b@test.com"},
	}

	mgr.EnsureAccountsTracked(state, accounts)

	if len(state.Accounts) != 2 {
		t.Errorf("expected 2 accounts, got %d", len(state.Accounts))
	}
	// Existing should be unchanged
	if state.Accounts["existing"].Status != config.QuotaStatusLimited {
		t.Errorf("existing account status changed: %s", state.Accounts["existing"].Status)
	}
	// New should default to available
	if state.Accounts["new"].Status != config.QuotaStatusAvailable {
		t.Errorf("new account status: %s, expected available", state.Accounts["new"].Status)
	}
}

func TestLoadCorruptFile(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// Write corrupt JSON
	path := constants.MayorQuotaPath(townRoot)
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.Load()
	if err == nil {
		t.Error("expected error loading corrupt file")
	}
}

func TestWithLock(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// Save initial state
	state := &config.QuotaState{
		Version: config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{
			"work": {Status: config.QuotaStatusAvailable},
		},
	}
	if err := mgr.Save(state); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Use WithLock to do load + modify + save atomically
	err := mgr.WithLock(func() error {
		s, err := mgr.Load()
		if err != nil {
			return err
		}
		s.Accounts["work"] = config.AccountQuotaState{
			Status:   config.QuotaStatusLimited,
			LastUsed: "2025-01-01T00:00:00Z",
		}
		return mgr.SaveUnlocked(s)
	})
	if err != nil {
		t.Fatalf("WithLock() error: %v", err)
	}

	// Verify the change persisted
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Accounts["work"].Status != config.QuotaStatusLimited {
		t.Errorf("expected status limited, got %s", loaded.Accounts["work"].Status)
	}
}

func TestWithLock_PropagatesError(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	sentinel := fmt.Errorf("test error")
	err := mgr.WithLock(func() error {
		return sentinel
	})
	if err != sentinel {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestSaveUnlocked(t *testing.T) {
	townRoot := setupTestTown(t)
	mgr := NewManager(townRoot)

	// Use WithLock + SaveUnlocked
	err := mgr.WithLock(func() error {
		state := &config.QuotaState{
			Accounts: map[string]config.AccountQuotaState{
				"test": {Status: config.QuotaStatusAvailable},
			},
		}
		return mgr.SaveUnlocked(state)
	})
	if err != nil {
		t.Fatalf("WithLock/SaveUnlocked error: %v", err)
	}

	// Verify
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Version != config.CurrentQuotaVersion {
		t.Errorf("expected version %d, got %d", config.CurrentQuotaVersion, loaded.Version)
	}
	if loaded.Accounts["test"].Status != config.QuotaStatusAvailable {
		t.Errorf("expected status available, got %s", loaded.Accounts["test"].Status)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	townRoot := t.TempDir()
	// Don't create mayor dir — Save should handle it via EnsureDirAndWriteJSON
	mgr := NewManager(townRoot)

	state := &config.QuotaState{
		Version:  config.CurrentQuotaVersion,
		Accounts: map[string]config.AccountQuotaState{},
	}

	if err := mgr.Save(state); err != nil {
		t.Fatalf("Save() should create directories: %v", err)
	}

	// Verify file exists
	data, err := os.ReadFile(constants.MayorQuotaPath(townRoot))
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	var loaded config.QuotaState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("parsing saved file: %v", err)
	}
}
