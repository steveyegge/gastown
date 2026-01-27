package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// skipIfNoSymlinkSupport skips the test if symlinks are not supported.
// On Windows, symlinks require elevated privileges or Developer Mode.
func skipIfNoSymlinkSupport(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests require elevated privileges on Windows")
	}
}

// setupSeanceTestEnv creates a test environment with multiple accounts and sessions.
func setupSeanceTestEnv(t *testing.T) (townRoot, fakeHome string, cleanup func()) {
	t.Helper()

	// Create fake home directory
	fakeHome = t.TempDir()

	// Create town root
	townRoot = t.TempDir()

	// Create mayor directory structure
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	// Create two account config directories
	account1Dir := filepath.Join(fakeHome, "claude-config-account1")
	account2Dir := filepath.Join(fakeHome, "claude-config-account2")
	if err := os.MkdirAll(account1Dir, 0755); err != nil {
		t.Fatalf("mkdir account1: %v", err)
	}
	if err := os.MkdirAll(account2Dir, 0755); err != nil {
		t.Fatalf("mkdir account2: %v", err)
	}

	// Create accounts.json pointing to both accounts
	accountsCfg := &config.AccountsConfig{
		Version: 1,
		Default: "account1",
		Accounts: map[string]config.Account{
			"account1": {Email: "test1@example.com", ConfigDir: account1Dir},
			"account2": {Email: "test2@example.com", ConfigDir: account2Dir},
		},
	}
	accountsPath := filepath.Join(mayorDir, "accounts.json")
	if err := config.SaveAccountsConfig(accountsPath, accountsCfg); err != nil {
		t.Fatalf("save accounts.json: %v", err)
	}

	// Create ~/.claude symlink pointing to account1 (current account)
	claudeDir := filepath.Join(fakeHome, ".claude")
	if err := os.Symlink(account1Dir, claudeDir); err != nil {
		t.Fatalf("symlink .claude: %v", err)
	}

	// Set up HOME env var
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", fakeHome)

	cleanup = func() {
		os.Setenv("HOME", oldHome)
	}

	return townRoot, fakeHome, cleanup
}

// createTestSession creates a mock session file and index entry.
func createTestSession(t *testing.T, configDir, projectName, sessionID string) {
	t.Helper()

	projectDir := filepath.Join(configDir, "projects", projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	// Create session file
	sessionFile := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(sessionFile, []byte(`{"type":"test"}`), 0600); err != nil {
		t.Fatalf("write session file: %v", err)
	}

	// Create or update sessions-index.json
	indexPath := filepath.Join(projectDir, "sessions-index.json")
	var index sessionsIndex
	if data, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(data, &index)
	} else {
		index.Version = 1
	}

	// Add session entry
	entry := map[string]interface{}{
		"sessionId":    sessionID,
		"name":         "Test Session",
		"lastAccessed": "2026-01-22T00:00:00Z",
	}
	entryJSON, _ := json.Marshal(entry)
	index.Entries = append(index.Entries, entryJSON)

	indexData, _ := json.MarshalIndent(index, "", "  ")
	if err := os.WriteFile(indexPath, indexData, 0600); err != nil {
		t.Fatalf("write sessions-index.json: %v", err)
	}
}

func TestFindSessionLocation(t *testing.T) {
	skipIfNoSymlinkSupport(t)

	t.Run("finds session in account1", func(t *testing.T) {
		townRoot, fakeHome, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		account1Dir := filepath.Join(fakeHome, "claude-config-account1")
		createTestSession(t, account1Dir, "test-project", "session-abc123")

		loc := findSessionLocation(townRoot, "session-abc123")
		if loc == nil {
			t.Fatal("expected to find session, got nil")
		}
		if loc.configDir != account1Dir {
			t.Errorf("expected configDir %s, got %s", account1Dir, loc.configDir)
		}
		if loc.projectDir != "test-project" {
			t.Errorf("expected projectDir test-project, got %s", loc.projectDir)
		}
	})

	t.Run("finds session in account2", func(t *testing.T) {
		townRoot, fakeHome, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		account2Dir := filepath.Join(fakeHome, "claude-config-account2")
		createTestSession(t, account2Dir, "other-project", "session-xyz789")

		loc := findSessionLocation(townRoot, "session-xyz789")
		if loc == nil {
			t.Fatal("expected to find session, got nil")
		}
		if loc.configDir != account2Dir {
			t.Errorf("expected configDir %s, got %s", account2Dir, loc.configDir)
		}
		if loc.projectDir != "other-project" {
			t.Errorf("expected projectDir other-project, got %s", loc.projectDir)
		}
	})

	t.Run("returns nil for nonexistent session", func(t *testing.T) {
		townRoot, _, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		loc := findSessionLocation(townRoot, "session-notfound")
		if loc != nil {
			t.Errorf("expected nil for nonexistent session, got %+v", loc)
		}
	})

	t.Run("returns nil for empty townRoot", func(t *testing.T) {
		loc := findSessionLocation("", "session-abc")
		if loc != nil {
			t.Errorf("expected nil for empty townRoot, got %+v", loc)
		}
	})
}

func TestSymlinkSessionToCurrentAccount(t *testing.T) {
	skipIfNoSymlinkSupport(t)

	t.Run("creates symlink for session in other account", func(t *testing.T) {
		townRoot, fakeHome, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		// Create session in account2 (not the current account)
		account2Dir := filepath.Join(fakeHome, "claude-config-account2")
		createTestSession(t, account2Dir, "cross-project", "session-cross123")

		// Call symlinkSessionToCurrentAccount
		cleanupFn, err := symlinkSessionToCurrentAccount(townRoot, "session-cross123")
		if err != nil {
			t.Fatalf("symlinkSessionToCurrentAccount failed: %v", err)
		}
		if cleanupFn == nil {
			t.Fatal("expected cleanup function, got nil")
		}

		// Verify symlink was created in current account (account1)
		account1Dir := filepath.Join(fakeHome, "claude-config-account1")
		symlinkPath := filepath.Join(account1Dir, "projects", "cross-project", "session-cross123.jsonl")

		info, err := os.Lstat(symlinkPath)
		if err != nil {
			t.Fatalf("symlink not found: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("expected symlink, got regular file")
		}

		// Verify sessions-index.json was updated
		indexPath := filepath.Join(account1Dir, "projects", "cross-project", "sessions-index.json")
		data, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatalf("reading index: %v", err)
		}

		var index sessionsIndex
		if err := json.Unmarshal(data, &index); err != nil {
			t.Fatalf("parsing index: %v", err)
		}

		found := false
		for _, entry := range index.Entries {
			var e sessionsIndexEntry
			if json.Unmarshal(entry, &e) == nil && e.SessionID == "session-cross123" {
				found = true
				break
			}
		}
		if !found {
			t.Error("session not found in target index")
		}

		// Test cleanup
		cleanupFn()

		// Verify symlink was removed
		if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
			t.Error("symlink should have been removed after cleanup")
		}

		// Verify session was removed from index
		data, _ = os.ReadFile(indexPath)
		_ = json.Unmarshal(data, &index)
		for _, entry := range index.Entries {
			var e sessionsIndexEntry
			if json.Unmarshal(entry, &e) == nil && e.SessionID == "session-cross123" {
				t.Error("session should have been removed from index after cleanup")
			}
		}
	})

	t.Run("returns nil cleanup for session in current account", func(t *testing.T) {
		townRoot, fakeHome, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		// Create session in account1 (the current account)
		account1Dir := filepath.Join(fakeHome, "claude-config-account1")
		createTestSession(t, account1Dir, "local-project", "session-local456")

		cleanupFn, err := symlinkSessionToCurrentAccount(townRoot, "session-local456")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cleanupFn != nil {
			t.Error("expected nil cleanup for session in current account")
		}
	})

	t.Run("returns error for nonexistent session", func(t *testing.T) {
		townRoot, _, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		_, err := symlinkSessionToCurrentAccount(townRoot, "session-notfound")
		if err == nil {
			t.Error("expected error for nonexistent session")
		}
	})
}

func TestCleanupOrphanedSessionSymlinks(t *testing.T) {
	skipIfNoSymlinkSupport(t)

	t.Run("removes orphaned symlinks", func(t *testing.T) {
		_, fakeHome, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		account1Dir := filepath.Join(fakeHome, "claude-config-account1")
		projectDir := filepath.Join(account1Dir, "projects", "orphan-project")
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatalf("mkdir project: %v", err)
		}

		// Create an orphaned symlink (target doesn't exist)
		orphanSymlink := filepath.Join(projectDir, "orphan-session.jsonl")
		nonexistentTarget := filepath.Join(fakeHome, "nonexistent", "session.jsonl")
		if err := os.Symlink(nonexistentTarget, orphanSymlink); err != nil {
			t.Fatalf("create orphan symlink: %v", err)
		}

		// Create a sessions-index.json with the orphaned entry
		index := sessionsIndex{
			Version: 1,
			Entries: []json.RawMessage{
				json.RawMessage(`{"sessionId":"orphan-session","name":"Orphan"}`),
			},
		}
		indexPath := filepath.Join(projectDir, "sessions-index.json")
		data, _ := json.MarshalIndent(index, "", "  ")
		if err := os.WriteFile(indexPath, data, 0600); err != nil {
			t.Fatalf("write index: %v", err)
		}

		// Run cleanup
		cleanupOrphanedSessionSymlinks()

		// Verify orphan symlink was removed
		if _, err := os.Lstat(orphanSymlink); !os.IsNotExist(err) {
			t.Error("orphaned symlink should have been removed")
		}

		// Verify entry was removed from index
		data, _ = os.ReadFile(indexPath)
		var updatedIndex sessionsIndex
		_ = json.Unmarshal(data, &updatedIndex)
		if len(updatedIndex.Entries) != 0 {
			t.Errorf("expected 0 entries after cleanup, got %d", len(updatedIndex.Entries))
		}
	})

	t.Run("preserves valid symlinks", func(t *testing.T) {
		_, fakeHome, cleanup := setupSeanceTestEnv(t)
		defer cleanup()

		account1Dir := filepath.Join(fakeHome, "claude-config-account1")
		account2Dir := filepath.Join(fakeHome, "claude-config-account2")

		// Create a real session in account2
		createTestSession(t, account2Dir, "valid-project", "valid-session")

		// Create project dir in account1
		projectDir := filepath.Join(account1Dir, "projects", "valid-project")
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatalf("mkdir project: %v", err)
		}

		// Create a valid symlink pointing to the real session
		validSymlink := filepath.Join(projectDir, "valid-session.jsonl")
		realTarget := filepath.Join(account2Dir, "projects", "valid-project", "valid-session.jsonl")
		if err := os.Symlink(realTarget, validSymlink); err != nil {
			t.Fatalf("create valid symlink: %v", err)
		}

		// Create index with valid entry
		index := sessionsIndex{
			Version: 1,
			Entries: []json.RawMessage{
				json.RawMessage(`{"sessionId":"valid-session","name":"Valid"}`),
			},
		}
		indexPath := filepath.Join(projectDir, "sessions-index.json")
		data, _ := json.MarshalIndent(index, "", "  ")
		if err := os.WriteFile(indexPath, data, 0600); err != nil {
			t.Fatalf("write index: %v", err)
		}

		// Run cleanup
		cleanupOrphanedSessionSymlinks()

		// Verify valid symlink was preserved
		if _, err := os.Lstat(validSymlink); err != nil {
			t.Error("valid symlink should have been preserved")
		}

		// Verify entry was preserved in index
		data, _ = os.ReadFile(indexPath)
		var updatedIndex sessionsIndex
		_ = json.Unmarshal(data, &updatedIndex)
		if len(updatedIndex.Entries) != 1 {
			t.Errorf("expected 1 entry preserved, got %d", len(updatedIndex.Entries))
		}
	})
}
