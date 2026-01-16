package polecat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNamePool_Allocate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	// First allocation should be first themed name (furiosa)
	name, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error: %v", err)
	}
	if name != "furiosa" {
		t.Errorf("expected furiosa, got %s", name)
	}

	// Second allocation should be nux
	name, err = pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error: %v", err)
	}
	if name != "nux" {
		t.Errorf("expected nux, got %s", name)
	}
}

func TestNamePool_Release(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	// Allocate first two
	name1, _ := pool.Allocate()
	name2, _ := pool.Allocate()

	if name1 != "furiosa" || name2 != "nux" {
		t.Fatalf("unexpected allocations: %s, %s", name1, name2)
	}

	// Release first one
	pool.Release("furiosa")

	// Next allocation should reuse furiosa
	name, _ := pool.Allocate()
	if name != "furiosa" {
		t.Errorf("expected furiosa to be reused, got %s", name)
	}
}

func TestNamePool_PrefersOrder(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	// Allocate first 5
	for i := 0; i < 5; i++ {
		pool.Allocate()
	}

	// Release slit and furiosa
	pool.Release("slit")
	pool.Release("furiosa")

	// Next allocation should be furiosa (first in theme order)
	name, _ := pool.Allocate()
	if name != "furiosa" {
		t.Errorf("expected furiosa (first in order), got %s", name)
	}

	// Next should be slit
	name, _ = pool.Allocate()
	if name != "slit" {
		t.Errorf("expected slit, got %s", name)
	}
}

func TestNamePool_Overflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "gastown", "mad-max", nil, 5)

	// Exhaust the small pool
	for i := 0; i < 5; i++ {
		pool.Allocate()
	}

	// Next allocation should be overflow format
	name, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error: %v", err)
	}
	expected := "gastown-6"
	if name != expected {
		t.Errorf("expected overflow name %s, got %s", expected, name)
	}

	// Next overflow
	name, _ = pool.Allocate()
	if name != "gastown-7" {
		t.Errorf("expected gastown-7, got %s", name)
	}
}

func TestNamePool_OverflowNotReusable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "gastown", "mad-max", nil, 3)

	// Exhaust the pool
	for i := 0; i < 3; i++ {
		pool.Allocate()
	}

	// Get overflow name
	overflow1, _ := pool.Allocate()
	if overflow1 != "gastown-4" {
		t.Fatalf("expected gastown-4, got %s", overflow1)
	}

	// Release it - should not be reused
	pool.Release(overflow1)

	// Next allocation should be gastown-5, not gastown-4
	name, _ := pool.Allocate()
	if name != "gastown-5" {
		t.Errorf("expected gastown-5 (overflow increments), got %s", name)
	}
}

func TestNamePool_SaveLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Use config to set MaxSize from the start (affects OverflowNext initialization)
	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, 3)

	// Exhaust the pool to trigger overflow, which increments OverflowNext
	pool.Allocate() // furiosa
	pool.Allocate() // nux
	pool.Allocate() // slit
	overflowName, _ := pool.Allocate() // testrig-4 (overflow)

	if overflowName != "testrig-4" {
		t.Errorf("expected testrig-4 for first overflow, got %s", overflowName)
	}

	// Save state
	if err := pool.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Create new pool and load
	pool2 := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, 3)
	if err := pool2.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// ZFC: InUse is NOT persisted - it's transient state derived from filesystem.
	// After Load(), InUse should be empty (0 active).
	if pool2.ActiveCount() != 0 {
		t.Errorf("expected 0 active after Load (ZFC: InUse is transient), got %d", pool2.ActiveCount())
	}

	// Simulate real usage: after polecat directories are created, Reconcile is called.
	// This clears reservations for names that now have directories (their reservation
	// is no longer needed - the directory itself marks them as in-use).
	// In this test, we simulate that furiosa, nux, slit directories were created.
	pool2.Reconcile([]string{"furiosa", "nux", "slit"})

	// After Reconcile, the 3 names are now InUse (from directories) and their
	// reservations are cleared. So we need to release them to test OverflowNext.
	pool2.Release("furiosa")
	pool2.Release("nux")
	pool2.Release("slit")

	// OverflowNext SHOULD persist - it's the one piece of state that can't be derived.
	// Next overflow should be testrig-5, not testrig-4.
	pool2.Allocate() // furiosa (released, so available again)
	pool2.Allocate() // nux
	pool2.Allocate() // slit
	overflowName2, _ := pool2.Allocate() // Should be testrig-5

	if overflowName2 != "testrig-5" {
		t.Errorf("expected testrig-5 (OverflowNext persisted), got %s", overflowName2)
	}
}

func TestNamePool_Reconcile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	// Simulate existing polecats from filesystem
	existing := []string{"slit", "valkyrie", "some-other-name"}

	pool.Reconcile(existing)

	if pool.ActiveCount() != 2 {
		t.Errorf("expected 2 active after reconcile, got %d", pool.ActiveCount())
	}

	// Should allocate furiosa first (not slit or valkyrie)
	name, _ := pool.Allocate()
	if name != "furiosa" {
		t.Errorf("expected furiosa, got %s", name)
	}
}

func TestNamePool_IsPoolName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	tests := []struct {
		name     string
		expected bool
	}{
		{"furiosa", true},
		{"nux", true},
		{"max", true},
		{"gastown-51", false}, // overflow format
		{"random-name", false},
		{"polecat-01", false}, // old format
	}

	for _, tc := range tests {
		result := pool.IsPoolName(tc.name)
		if result != tc.expected {
			t.Errorf("IsPoolName(%q) = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}

func TestNamePool_ActiveNames(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	pool.Allocate() // furiosa
	pool.Allocate() // nux
	pool.Allocate() // slit
	pool.Release("nux")

	names := pool.ActiveNames()
	if len(names) != 2 {
		t.Errorf("expected 2 active names, got %d", len(names))
	}
	// Names are sorted
	if names[0] != "furiosa" || names[1] != "slit" {
		t.Errorf("expected [furiosa, slit], got %v", names)
	}
}

func TestNamePool_MarkInUse(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	// Mark some slots as in use
	pool.MarkInUse("dementus")
	pool.MarkInUse("valkyrie")

	// Allocate should skip those
	name, _ := pool.Allocate()
	if name != "furiosa" {
		t.Errorf("expected furiosa, got %s", name)
	}

	// Verify count
	if pool.ActiveCount() != 3 { // furiosa, dementus, valkyrie
		t.Errorf("expected 3 active, got %d", pool.ActiveCount())
	}
}

func TestNamePool_StateFilePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")
	pool.Allocate()
	if err := pool.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify file was created in expected location
	expectedPath := filepath.Join(tmpDir, ".runtime", "namepool-state.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("state file not found at expected path: %v", err)
	}
}

func TestNamePool_Themes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Test minerals theme
	pool := NewNamePoolWithConfig(tmpDir, "testrig", "minerals", nil, 50)

	name, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error: %v", err)
	}
	if name != "obsidian" {
		t.Errorf("expected obsidian (first mineral), got %s", name)
	}

	// Test theme switching
	if err := pool.SetTheme("wasteland"); err != nil {
		t.Fatalf("SetTheme error: %v", err)
	}

	// obsidian should be released (not in wasteland theme)
	name, _ = pool.Allocate()
	if name != "rust" {
		t.Errorf("expected rust (first wasteland name), got %s", name)
	}
}

func TestNamePool_CustomNames(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	custom := []string{"alpha", "beta", "gamma", "delta"}
	pool := NewNamePoolWithConfig(tmpDir, "testrig", "", custom, 4)

	name, _ := pool.Allocate()
	if name != "alpha" {
		t.Errorf("expected alpha, got %s", name)
	}

	name, _ = pool.Allocate()
	if name != "beta" {
		t.Errorf("expected beta, got %s", name)
	}
}

func TestListThemes(t *testing.T) {
	themes := ListThemes()
	if len(themes) != 3 {
		t.Errorf("expected 3 themes, got %d", len(themes))
	}

	// Check that all expected themes are present
	expected := map[string]bool{"mad-max": true, "minerals": true, "wasteland": true}
	for _, theme := range themes {
		if !expected[theme] {
			t.Errorf("unexpected theme: %s", theme)
		}
	}
}

func TestGetThemeNames(t *testing.T) {
	names, err := GetThemeNames("mad-max")
	if err != nil {
		t.Fatalf("GetThemeNames error: %v", err)
	}
	if len(names) != 50 {
		t.Errorf("expected 50 mad-max names, got %d", len(names))
	}
	if names[0] != "furiosa" {
		t.Errorf("expected first name to be furiosa, got %s", names[0])
	}

	// Test invalid theme
	_, err = GetThemeNames("invalid-theme")
	if err == nil {
		t.Error("expected error for invalid theme")
	}
}

func TestNamePool_Reset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePool(tmpDir, "testrig")

	// Allocate several names
	for i := 0; i < 10; i++ {
		pool.Allocate()
	}

	if pool.ActiveCount() != 10 {
		t.Errorf("expected 10 active, got %d", pool.ActiveCount())
	}

	// Reset
	pool.Reset()

	if pool.ActiveCount() != 0 {
		t.Errorf("expected 0 active after reset, got %d", pool.ActiveCount())
	}

	// Should allocate furiosa again
	name, _ := pool.Allocate()
	if name != "furiosa" {
		t.Errorf("expected furiosa after reset, got %s", name)
	}
}
