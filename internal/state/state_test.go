// ABOUTME: Tests for global state management.
// ABOUTME: Verifies enable/disable toggle and XDG path resolution.

package state

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestStateDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".local", "state", "gastown")

	os.Unsetenv("XDG_STATE_HOME")
	if got := StateDir(); got != expected {
		t.Errorf("StateDir() = %q, want %q", got, expected)
	}

	os.Setenv("XDG_STATE_HOME", "/custom/state")
	defer os.Unsetenv("XDG_STATE_HOME")
	if got := filepath.ToSlash(StateDir()); got != "/custom/state/gastown" {
		t.Errorf("StateDir() with XDG = %q, want /custom/state/gastown", got)
	}
}

func TestConfigDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "gastown")

	os.Unsetenv("XDG_CONFIG_HOME")
	if got := ConfigDir(); got != expected {
		t.Errorf("ConfigDir() = %q, want %q", got, expected)
	}

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	defer os.Unsetenv("XDG_CONFIG_HOME")
	if got := filepath.ToSlash(ConfigDir()); got != "/custom/config/gastown" {
		t.Errorf("ConfigDir() with XDG = %q, want /custom/config/gastown", got)
	}
}

func TestCacheDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".cache", "gastown")

	os.Unsetenv("XDG_CACHE_HOME")
	if got := CacheDir(); got != expected {
		t.Errorf("CacheDir() = %q, want %q", got, expected)
	}

	os.Setenv("XDG_CACHE_HOME", "/custom/cache")
	defer os.Unsetenv("XDG_CACHE_HOME")
	if got := filepath.ToSlash(CacheDir()); got != "/custom/cache/gastown" {
		t.Errorf("CacheDir() with XDG = %q, want /custom/cache/gastown", got)
	}
}

func TestIsEnabled_EnvOverride(t *testing.T) {
	os.Setenv("GASTOWN_DISABLED", "1")
	defer os.Unsetenv("GASTOWN_DISABLED")
	if IsEnabled() {
		t.Error("IsEnabled() should return false when GASTOWN_DISABLED=1")
	}

	os.Unsetenv("GASTOWN_DISABLED")
	os.Setenv("GASTOWN_ENABLED", "1")
	defer os.Unsetenv("GASTOWN_ENABLED")
	if !IsEnabled() {
		t.Error("IsEnabled() should return true when GASTOWN_ENABLED=1")
	}
}

func TestIsEnabled_DisabledOverridesEnabled(t *testing.T) {
	os.Setenv("GASTOWN_DISABLED", "1")
	os.Setenv("GASTOWN_ENABLED", "1")
	defer os.Unsetenv("GASTOWN_DISABLED")
	defer os.Unsetenv("GASTOWN_ENABLED")

	if IsEnabled() {
		t.Error("GASTOWN_DISABLED should take precedence over GASTOWN_ENABLED")
	}
}

func TestEnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("XDG_STATE_HOME", tmpDir)
	defer os.Unsetenv("XDG_STATE_HOME")
	os.Unsetenv("GASTOWN_DISABLED")
	os.Unsetenv("GASTOWN_ENABLED")

	if err := Enable("1.0.0"); err != nil {
		t.Fatalf("Enable() failed: %v", err)
	}

	if !IsEnabled() {
		t.Error("IsEnabled() should return true after Enable()")
	}

	s, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if s.Version != "1.0.0" {
		t.Errorf("State.Version = %q, want %q", s.Version, "1.0.0")
	}
	if s.MachineID == "" {
		t.Error("State.MachineID should not be empty")
	}

	if err := Disable(); err != nil {
		t.Fatalf("Disable() failed: %v", err)
	}

	if IsEnabled() {
		t.Error("IsEnabled() should return false after Disable()")
	}
}

func TestGenerateMachineID(t *testing.T) {
	id1 := generateMachineID()
	id2 := generateMachineID()

	if len(id1) != 8 {
		t.Errorf("generateMachineID() length = %d, want 8", len(id1))
	}
	if id1 == id2 {
		t.Error("generateMachineID() should generate unique IDs")
	}
}

func TestGetMachineID_PersistsOnFirstCall(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("XDG_STATE_HOME", tmpDir)
	defer os.Unsetenv("XDG_STATE_HOME")

	id := GetMachineID()
	if len(id) != 8 {
		t.Fatalf("GetMachineID() length = %d, want 8", len(id))
	}

	// Second call should return the same persisted ID.
	id2 := GetMachineID()
	if id != id2 {
		t.Errorf("GetMachineID() returned different IDs: %q vs %q", id, id2)
	}

	// Verify it was persisted to disk.
	s, err := Load()
	if err != nil {
		t.Fatalf("Load() after GetMachineID: %v", err)
	}
	if s.MachineID != id {
		t.Errorf("persisted MachineID = %q, want %q", s.MachineID, id)
	}
}

func TestConcurrentEnable_SameMachineID(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("XDG_STATE_HOME", tmpDir)
	defer os.Unsetenv("XDG_STATE_HOME")

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := Enable("1.0.0"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("Enable() failed: %v", err)
	}

	// After all goroutines finish, there must be exactly one MachineID on disk.
	s, err := Load()
	if err != nil {
		t.Fatalf("Load() after concurrent Enable: %v", err)
	}
	if s.MachineID == "" {
		t.Fatal("MachineID should not be empty after concurrent Enable calls")
	}
	if len(s.MachineID) != 8 {
		t.Errorf("MachineID length = %d, want 8", len(s.MachineID))
	}
}

func TestConcurrentGetMachineID_Converges(t *testing.T) {
	tmpDir := t.TempDir()
	os.Setenv("XDG_STATE_HOME", tmpDir)
	defer os.Unsetenv("XDG_STATE_HOME")

	const goroutines = 10
	ids := make([]string, goroutines)
	var wg sync.WaitGroup

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			ids[i] = GetMachineID()
		}()
	}
	wg.Wait()

	// All goroutines should have converged on the same ID.
	for i := 1; i < goroutines; i++ {
		if ids[i] != ids[0] {
			t.Errorf("GetMachineID() goroutine %d returned %q, want %q (same as goroutine 0)", i, ids[i], ids[0])
		}
	}
}
