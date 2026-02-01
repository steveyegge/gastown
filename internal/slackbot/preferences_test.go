package slackbot

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultUserPrefs(t *testing.T) {
	prefs := DefaultUserPrefs()

	if prefs.DMOptIn != false {
		t.Errorf("expected DMOptIn=false, got %v", prefs.DMOptIn)
	}
	if prefs.NotificationLevel != "high" {
		t.Errorf("expected NotificationLevel='high', got %q", prefs.NotificationLevel)
	}
	if prefs.ThreadNotifications != false {
		t.Errorf("expected ThreadNotifications=false, got %v", prefs.ThreadNotifications)
	}
	if prefs.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestIsValidNotificationLevel(t *testing.T) {
	tests := []struct {
		level string
		valid bool
	}{
		{"all", true},
		{"high", true},
		{"muted", true},
		{"", false},
		{"invalid", false},
		{"ALL", false}, // Case-sensitive
		{"High", false},
	}

	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			got := IsValidNotificationLevel(tc.level)
			if got != tc.valid {
				t.Errorf("IsValidNotificationLevel(%q) = %v, want %v", tc.level, got, tc.valid)
			}
		})
	}
}

func TestNewPreferenceManager(t *testing.T) {
	tmpDir := t.TempDir()

	pm := NewPreferenceManager(tmpDir)

	expectedPath := filepath.Join(tmpDir, "settings", "slack_user_prefs.json")
	if pm.GetFilePath() != expectedPath {
		t.Errorf("expected filePath=%q, got %q", expectedPath, pm.GetFilePath())
	}

	if pm.UserCount() != 0 {
		t.Errorf("expected 0 users, got %d", pm.UserCount())
	}
}

func TestNewPreferenceManager_EmptyTownRoot(t *testing.T) {
	// Temporarily unset GT_ROOT
	oldGTRoot := os.Getenv("GT_ROOT")
	os.Unsetenv("GT_ROOT")
	defer func() {
		if oldGTRoot != "" {
			os.Setenv("GT_ROOT", oldGTRoot)
		}
	}()

	pm := NewPreferenceManager("")

	// Should fall back to ~/gt
	home, _ := os.UserHomeDir()
	expectedPath := filepath.Join(home, "gt", "settings", "slack_user_prefs.json")
	if pm.GetFilePath() != expectedPath {
		t.Errorf("expected filePath=%q, got %q", expectedPath, pm.GetFilePath())
	}
}

func TestNewPreferenceManager_GTRootEnv(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("GT_ROOT", tmpDir)
	defer os.Unsetenv("GT_ROOT")

	pm := NewPreferenceManager("")

	expectedPath := filepath.Join(tmpDir, "settings", "slack_user_prefs.json")
	if pm.GetFilePath() != expectedPath {
		t.Errorf("expected filePath=%q, got %q", expectedPath, pm.GetFilePath())
	}
}

func TestGetUserPreferences_Default(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())

	prefs := pm.GetUserPreferences("U12345")

	// Should return defaults
	if prefs.DMOptIn != false {
		t.Errorf("expected DMOptIn=false, got %v", prefs.DMOptIn)
	}
	if prefs.NotificationLevel != "high" {
		t.Errorf("expected NotificationLevel='high', got %q", prefs.NotificationLevel)
	}
}

func TestSetDMOptIn(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())
	userID := "U12345"

	// Initially should be false (default)
	prefs := pm.GetUserPreferences(userID)
	if prefs.DMOptIn != false {
		t.Errorf("expected initial DMOptIn=false, got %v", prefs.DMOptIn)
	}

	// Set to true
	if err := pm.SetDMOptIn(userID, true); err != nil {
		t.Fatalf("SetDMOptIn failed: %v", err)
	}

	prefs = pm.GetUserPreferences(userID)
	if prefs.DMOptIn != true {
		t.Errorf("expected DMOptIn=true, got %v", prefs.DMOptIn)
	}

	// Set back to false
	if err := pm.SetDMOptIn(userID, false); err != nil {
		t.Fatalf("SetDMOptIn failed: %v", err)
	}

	prefs = pm.GetUserPreferences(userID)
	if prefs.DMOptIn != false {
		t.Errorf("expected DMOptIn=false, got %v", prefs.DMOptIn)
	}
}

func TestSetNotificationLevel(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())
	userID := "U12345"

	// Set to "all"
	if err := pm.SetNotificationLevel(userID, "all"); err != nil {
		t.Fatalf("SetNotificationLevel failed: %v", err)
	}

	prefs := pm.GetUserPreferences(userID)
	if prefs.NotificationLevel != "all" {
		t.Errorf("expected NotificationLevel='all', got %q", prefs.NotificationLevel)
	}

	// Set to "muted"
	if err := pm.SetNotificationLevel(userID, "muted"); err != nil {
		t.Fatalf("SetNotificationLevel failed: %v", err)
	}

	prefs = pm.GetUserPreferences(userID)
	if prefs.NotificationLevel != "muted" {
		t.Errorf("expected NotificationLevel='muted', got %q", prefs.NotificationLevel)
	}
}

func TestSetNotificationLevel_Invalid(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())
	userID := "U12345"

	err := pm.SetNotificationLevel(userID, "invalid")
	if err == nil {
		t.Error("expected error for invalid notification level")
	}

	err = pm.SetNotificationLevel(userID, "")
	if err == nil {
		t.Error("expected error for empty notification level")
	}
}

func TestSetThreadNotifications(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())
	userID := "U12345"

	// Set to true
	if err := pm.SetThreadNotifications(userID, true); err != nil {
		t.Fatalf("SetThreadNotifications failed: %v", err)
	}

	prefs := pm.GetUserPreferences(userID)
	if prefs.ThreadNotifications != true {
		t.Errorf("expected ThreadNotifications=true, got %v", prefs.ThreadNotifications)
	}

	// Set to false
	if err := pm.SetThreadNotifications(userID, false); err != nil {
		t.Fatalf("SetThreadNotifications failed: %v", err)
	}

	prefs = pm.GetUserPreferences(userID)
	if prefs.ThreadNotifications != false {
		t.Errorf("expected ThreadNotifications=false, got %v", prefs.ThreadNotifications)
	}
}

func TestIsEligibleForDM(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())

	tests := []struct {
		name     string
		setup    func()
		userID   string
		eligible bool
	}{
		{
			name:     "user without preferences (default)",
			setup:    func() {},
			userID:   "U_NEW",
			eligible: false, // Default is opt-out
		},
		{
			name: "opted in, high level",
			setup: func() {
				pm.SetDMOptIn("U_OPTIN", true)
				pm.SetNotificationLevel("U_OPTIN", "high")
			},
			userID:   "U_OPTIN",
			eligible: true,
		},
		{
			name: "opted in, all level",
			setup: func() {
				pm.SetDMOptIn("U_ALL", true)
				pm.SetNotificationLevel("U_ALL", "all")
			},
			userID:   "U_ALL",
			eligible: true,
		},
		{
			name: "opted in but muted",
			setup: func() {
				pm.SetDMOptIn("U_MUTED", true)
				pm.SetNotificationLevel("U_MUTED", "muted")
			},
			userID:   "U_MUTED",
			eligible: false, // Muted overrides opt-in
		},
		{
			name: "not opted in",
			setup: func() {
				pm.SetDMOptIn("U_NOOPTIN", false)
				pm.SetNotificationLevel("U_NOOPTIN", "all")
			},
			userID:   "U_NOOPTIN",
			eligible: false, // Must be opted in
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			got := pm.IsEligibleForDM(tc.userID)
			if got != tc.eligible {
				t.Errorf("IsEligibleForDM(%q) = %v, want %v", tc.userID, got, tc.eligible)
			}
		})
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewPreferenceManager(tmpDir)

	// Set some preferences
	pm.SetDMOptIn("U1", true)
	pm.SetNotificationLevel("U1", "all")
	pm.SetThreadNotifications("U1", true)

	pm.SetDMOptIn("U2", false)
	pm.SetNotificationLevel("U2", "muted")

	// Save
	if err := pm.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(pm.GetFilePath()); os.IsNotExist(err) {
		t.Error("preferences file was not created")
	}

	// Create new manager and load
	pm2 := NewPreferenceManager(tmpDir)

	// Verify loaded preferences
	prefs1 := pm2.GetUserPreferences("U1")
	if prefs1.DMOptIn != true {
		t.Errorf("U1 DMOptIn: expected true, got %v", prefs1.DMOptIn)
	}
	if prefs1.NotificationLevel != "all" {
		t.Errorf("U1 NotificationLevel: expected 'all', got %q", prefs1.NotificationLevel)
	}
	if prefs1.ThreadNotifications != true {
		t.Errorf("U1 ThreadNotifications: expected true, got %v", prefs1.ThreadNotifications)
	}

	prefs2 := pm2.GetUserPreferences("U2")
	if prefs2.DMOptIn != false {
		t.Errorf("U2 DMOptIn: expected false, got %v", prefs2.DMOptIn)
	}
	if prefs2.NotificationLevel != "muted" {
		t.Errorf("U2 NotificationLevel: expected 'muted', got %q", prefs2.NotificationLevel)
	}
}

func TestLoad_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewPreferenceManager(tmpDir)

	// Load should succeed (fresh installation)
	err := pm.Load()
	if err != nil {
		t.Errorf("Load failed for non-existent file: %v", err)
	}

	if pm.UserCount() != 0 {
		t.Errorf("expected 0 users after loading non-existent file, got %d", pm.UserCount())
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	settingsDir := filepath.Join(tmpDir, "settings")
	os.MkdirAll(settingsDir, 0755)

	// Write invalid JSON
	prefsPath := filepath.Join(settingsDir, "slack_user_prefs.json")
	os.WriteFile(prefsPath, []byte("invalid json{"), 0644)

	pm := &PreferenceManager{
		prefs:    make(map[string]UserPrefs),
		filePath: prefsPath,
	}

	err := pm.Load()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestUserCount(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())

	if pm.UserCount() != 0 {
		t.Errorf("expected 0 users initially, got %d", pm.UserCount())
	}

	pm.SetDMOptIn("U1", true)
	if pm.UserCount() != 1 {
		t.Errorf("expected 1 user, got %d", pm.UserCount())
	}

	pm.SetDMOptIn("U2", true)
	if pm.UserCount() != 2 {
		t.Errorf("expected 2 users, got %d", pm.UserCount())
	}

	// Setting on existing user shouldn't increase count
	pm.SetNotificationLevel("U1", "all")
	if pm.UserCount() != 2 {
		t.Errorf("expected 2 users after update, got %d", pm.UserCount())
	}
}

func TestListUsers(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())

	users := pm.ListUsers()
	if len(users) != 0 {
		t.Errorf("expected 0 users initially, got %d", len(users))
	}

	pm.SetDMOptIn("U1", true)
	pm.SetDMOptIn("U2", true)
	pm.SetDMOptIn("U3", true)

	users = pm.ListUsers()
	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}

	// Check all users are present (order not guaranteed)
	userSet := make(map[string]bool)
	for _, u := range users {
		userSet[u] = true
	}
	for _, expected := range []string{"U1", "U2", "U3"} {
		if !userSet[expected] {
			t.Errorf("expected user %s in list", expected)
		}
	}
}

func TestClearUserPreferences(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())

	pm.SetDMOptIn("U1", true)
	pm.SetNotificationLevel("U1", "all")

	if pm.UserCount() != 1 {
		t.Errorf("expected 1 user before clear, got %d", pm.UserCount())
	}

	pm.ClearUserPreferences("U1")

	if pm.UserCount() != 0 {
		t.Errorf("expected 0 users after clear, got %d", pm.UserCount())
	}

	// Should return defaults after clearing
	prefs := pm.GetUserPreferences("U1")
	if prefs.DMOptIn != false {
		t.Errorf("expected default DMOptIn=false after clear, got %v", prefs.DMOptIn)
	}
}

func TestClearUserPreferences_NonExistent(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())

	// Should not panic
	pm.ClearUserPreferences("NONEXISTENT")

	if pm.UserCount() != 0 {
		t.Errorf("expected 0 users, got %d", pm.UserCount())
	}
}

func TestUpdatedAt(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())
	userID := "U12345"

	beforeSet := time.Now()
	pm.SetDMOptIn(userID, true)
	afterSet := time.Now()

	prefs := pm.GetUserPreferences(userID)

	if prefs.UpdatedAt.Before(beforeSet) || prefs.UpdatedAt.After(afterSet) {
		t.Errorf("UpdatedAt=%v not in expected range [%v, %v]", prefs.UpdatedAt, beforeSet, afterSet)
	}

	// Update again and check timestamp changes
	time.Sleep(10 * time.Millisecond)
	pm.SetNotificationLevel(userID, "all")

	prefs2 := pm.GetUserPreferences(userID)
	if !prefs2.UpdatedAt.After(prefs.UpdatedAt) {
		t.Errorf("UpdatedAt should have been updated: old=%v, new=%v", prefs.UpdatedAt, prefs2.UpdatedAt)
	}
}

func TestConcurrentAccess(t *testing.T) {
	pm := NewPreferenceManager(t.TempDir())
	done := make(chan bool)

	// Multiple goroutines setting preferences
	for i := 0; i < 10; i++ {
		go func(id int) {
			userID := "USER"
			for j := 0; j < 100; j++ {
				pm.SetDMOptIn(userID, j%2 == 0)
				pm.SetNotificationLevel(userID, ValidNotificationLevels[j%3])
				pm.GetUserPreferences(userID)
				pm.IsEligibleForDM(userID)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not have crashed
	if pm.UserCount() != 1 {
		t.Errorf("expected 1 user after concurrent access, got %d", pm.UserCount())
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewPreferenceManager(tmpDir)

	// Settings directory doesn't exist yet
	settingsDir := filepath.Join(tmpDir, "settings")
	if _, err := os.Stat(settingsDir); !os.IsNotExist(err) {
		t.Fatal("settings directory should not exist yet")
	}

	pm.SetDMOptIn("U1", true)

	// Save should create the directory
	if err := pm.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(settingsDir); os.IsNotExist(err) {
		t.Error("Save should have created settings directory")
	}
}
