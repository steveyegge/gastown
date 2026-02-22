package daemon

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/dog"
	"github.com/steveyegge/gastown/internal/tmux"
)

// testHandlerDaemon creates a minimal Daemon with a logger for handler tests.
func testHandlerDaemon(t *testing.T, townRoot string) *Daemon {
	t.Helper()
	return &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(os.Stderr, "test: ", log.LstdFlags),
	}
}

// testSetupDogState creates a dog directory with a .dog.json state file.
func testSetupDogState(t *testing.T, townRoot, name string, state dog.State, lastActive time.Time) {
	t.Helper()

	kennelDir := filepath.Join(townRoot, "deacon", "dogs", name)
	if err := os.MkdirAll(kennelDir, 0755); err != nil {
		t.Fatalf("Failed to create kennel dir for %s: %v", name, err)
	}

	ds := &dog.DogState{
		Name:       name,
		State:      state,
		LastActive: lastActive,
		Worktrees:  map[string]string{},
		CreatedAt:  lastActive,
		UpdatedAt:  lastActive,
	}

	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal dog state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kennelDir, ".dog.json"), data, 0644); err != nil {
		t.Fatalf("Failed to write dog state: %v", err)
	}
}

// testDogExists checks if a dog directory exists in the kennel.
func testDogExists(townRoot, name string) bool {
	_, err := os.Stat(filepath.Join(townRoot, "deacon", "dogs", name, ".dog.json"))
	return err == nil
}

func TestReapIdleDogs_SkipsWorkingDogs(t *testing.T) {
	townRoot := t.TempDir()
	d := testHandlerDaemon(t, townRoot)

	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	mgr := dog.NewManager(townRoot, rigsConfig)
	tm := tmux.NewTmux()
	sm := dog.NewSessionManager(tm, townRoot, mgr)

	// Create a working dog with old LastActive — should NOT be reaped.
	testSetupDogState(t, townRoot, "worker", dog.StateWorking, time.Now().Add(-5*time.Hour))

	d.reapIdleDogs(mgr, sm)

	if !testDogExists(townRoot, "worker") {
		t.Error("working dog should not be removed by reapIdleDogs")
	}
}

func TestReapIdleDogs_SkipsRecentlyActiveDogs(t *testing.T) {
	townRoot := t.TempDir()
	d := testHandlerDaemon(t, townRoot)

	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	mgr := dog.NewManager(townRoot, rigsConfig)
	tm := tmux.NewTmux()
	sm := dog.NewSessionManager(tm, townRoot, mgr)

	// Create idle dogs that were active recently — should NOT be reaped.
	for i := 0; i < 6; i++ {
		name := "recent-" + string(rune('a'+i))
		testSetupDogState(t, townRoot, name, dog.StateIdle, time.Now().Add(-30*time.Minute))
	}

	d.reapIdleDogs(mgr, sm)

	// All dogs should still exist.
	dogs, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(dogs) != 6 {
		t.Errorf("expected 6 dogs after reap, got %d", len(dogs))
	}
}

func TestReapIdleDogs_RemovesLongIdleDogsWhenPoolOversized(t *testing.T) {
	townRoot := t.TempDir()
	d := testHandlerDaemon(t, townRoot)

	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	mgr := dog.NewManager(townRoot, rigsConfig)
	tm := tmux.NewTmux()
	sm := dog.NewSessionManager(tm, townRoot, mgr)

	// Create 6 idle dogs: 4 recent, 2 long-idle.
	// Pool is 6 > maxDogPoolSize(4), so long-idle dogs should be removed.
	for i := 0; i < 4; i++ {
		name := "recent-" + string(rune('a'+i))
		testSetupDogState(t, townRoot, name, dog.StateIdle, time.Now().Add(-10*time.Minute))
	}
	testSetupDogState(t, townRoot, "old-1", dog.StateIdle, time.Now().Add(-5*time.Hour))
	testSetupDogState(t, townRoot, "old-2", dog.StateIdle, time.Now().Add(-6*time.Hour))

	d.reapIdleDogs(mgr, sm)

	// Long-idle dogs should be removed, recent ones kept.
	dogs, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}

	if len(dogs) > maxDogPoolSize {
		t.Errorf("expected pool trimmed to at most %d, got %d", maxDogPoolSize, len(dogs))
	}

	// Verify the old dogs were removed.
	if testDogExists(townRoot, "old-1") {
		t.Error("old-1 should have been removed")
	}
	if testDogExists(townRoot, "old-2") {
		t.Error("old-2 should have been removed")
	}
}

func TestReapIdleDogs_DoesNotRemoveWhenPoolAtMaxSize(t *testing.T) {
	townRoot := t.TempDir()
	d := testHandlerDaemon(t, townRoot)

	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	mgr := dog.NewManager(townRoot, rigsConfig)
	tm := tmux.NewTmux()
	sm := dog.NewSessionManager(tm, townRoot, mgr)

	// Create exactly maxDogPoolSize idle dogs, all long-idle.
	// Pool is NOT oversized, so none should be removed.
	for i := 0; i < maxDogPoolSize; i++ {
		name := "idle-" + string(rune('a'+i))
		testSetupDogState(t, townRoot, name, dog.StateIdle, time.Now().Add(-5*time.Hour))
	}

	d.reapIdleDogs(mgr, sm)

	dogs, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(dogs) != maxDogPoolSize {
		t.Errorf("expected %d dogs (pool not oversized), got %d", maxDogPoolSize, len(dogs))
	}
}

func TestReapIdleDogs_StopsRemovingAtMaxPoolSize(t *testing.T) {
	townRoot := t.TempDir()
	d := testHandlerDaemon(t, townRoot)

	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	mgr := dog.NewManager(townRoot, rigsConfig)
	tm := tmux.NewTmux()
	sm := dog.NewSessionManager(tm, townRoot, mgr)

	// Create 7 idle dogs, all long-idle.
	// Should remove 3 to get down to maxDogPoolSize(4).
	for i := 0; i < 7; i++ {
		name := "dog-" + string(rune('a'+i))
		testSetupDogState(t, townRoot, name, dog.StateIdle, time.Now().Add(-5*time.Hour))
	}

	d.reapIdleDogs(mgr, sm)

	dogs, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(dogs) > maxDogPoolSize {
		t.Errorf("expected pool trimmed to %d, got %d", maxDogPoolSize, len(dogs))
	}
}

func TestReapIdleDogs_MixedStates(t *testing.T) {
	townRoot := t.TempDir()
	d := testHandlerDaemon(t, townRoot)

	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	mgr := dog.NewManager(townRoot, rigsConfig)
	tm := tmux.NewTmux()
	sm := dog.NewSessionManager(tm, townRoot, mgr)

	// 2 working + 3 recent idle + 2 long-idle = 7 total.
	// Pool is oversized (7 > 4). Only long-idle IDLE dogs should be removed.
	// Working dogs are never touched.
	testSetupDogState(t, townRoot, "worker-a", dog.StateWorking, time.Now().Add(-5*time.Hour))
	testSetupDogState(t, townRoot, "worker-b", dog.StateWorking, time.Now().Add(-5*time.Hour))
	testSetupDogState(t, townRoot, "recent-a", dog.StateIdle, time.Now().Add(-10*time.Minute))
	testSetupDogState(t, townRoot, "recent-b", dog.StateIdle, time.Now().Add(-10*time.Minute))
	testSetupDogState(t, townRoot, "recent-c", dog.StateIdle, time.Now().Add(-10*time.Minute))
	testSetupDogState(t, townRoot, "old-a", dog.StateIdle, time.Now().Add(-5*time.Hour))
	testSetupDogState(t, townRoot, "old-b", dog.StateIdle, time.Now().Add(-6*time.Hour))

	d.reapIdleDogs(mgr, sm)

	// Working dogs must survive.
	if !testDogExists(townRoot, "worker-a") {
		t.Error("worker-a should not be removed")
	}
	if !testDogExists(townRoot, "worker-b") {
		t.Error("worker-b should not be removed")
	}

	// Long-idle dogs should be removed (pool was 7 > 4).
	if testDogExists(townRoot, "old-a") {
		t.Error("old-a should have been removed")
	}
	if testDogExists(townRoot, "old-b") {
		t.Error("old-b should have been removed")
	}

	// Recent idle dogs should survive.
	if !testDogExists(townRoot, "recent-a") {
		t.Error("recent-a should not be removed")
	}
}

func TestReapIdleDogs_EmptyKennel(t *testing.T) {
	townRoot := t.TempDir()
	d := testHandlerDaemon(t, townRoot)

	rigsConfig := &config.RigsConfig{Version: 1, Rigs: map[string]config.RigEntry{}}
	mgr := dog.NewManager(townRoot, rigsConfig)
	tm := tmux.NewTmux()
	sm := dog.NewSessionManager(tm, townRoot, mgr)

	// Should not panic or error with empty kennel.
	d.reapIdleDogs(mgr, sm)
}

func TestReapIdleDogs_Constants(t *testing.T) {
	if dogIdleSessionTimeout != 1*time.Hour {
		t.Errorf("dogIdleSessionTimeout = %v, want 1h", dogIdleSessionTimeout)
	}
	if dogIdleRemoveTimeout != 4*time.Hour {
		t.Errorf("dogIdleRemoveTimeout = %v, want 4h", dogIdleRemoveTimeout)
	}
	if maxDogPoolSize != 4 {
		t.Errorf("maxDogPoolSize = %d, want 4", maxDogPoolSize)
	}
}
