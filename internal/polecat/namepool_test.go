package polecat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamePool_Allocate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

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

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

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

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

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

	// Next allocation should be overflow format (just number, not rig-prefixed)
	name, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error: %v", err)
	}
	expected := "6"
	if name != expected {
		t.Errorf("expected overflow name %s, got %s", expected, name)
	}

	// Next overflow
	name, _ = pool.Allocate()
	if name != "7" {
		t.Errorf("expected 7, got %s", name)
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

	// Get overflow name (just number, not rig-prefixed)
	overflow1, _ := pool.Allocate()
	if overflow1 != "4" {
		t.Fatalf("expected 4, got %s", overflow1)
	}

	// Release it - should not be reused
	pool.Release(overflow1)

	// Next allocation should be 5, not 4 (overflow increments)
	name, _ := pool.Allocate()
	if name != "5" {
		t.Errorf("expected 5 (overflow increments), got %s", name)
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
	overflowName, _ := pool.Allocate() // 4 (overflow - just number, not rig-prefixed)

	if overflowName != "4" {
		t.Errorf("expected 4 for first overflow, got %s", overflowName)
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

	// OverflowNext SHOULD persist - it's the one piece of state that can't be derived.
	// Next overflow should be 5, not 4 (OverflowNext persisted).
	pool2.Allocate() // furiosa (InUse empty, so starts from beginning)
	pool2.Allocate() // nux
	pool2.Allocate() // slit
	overflowName2, _ := pool2.Allocate() // Should be 5

	if overflowName2 != "5" {
		t.Errorf("expected 5 (OverflowNext persisted), got %s", overflowName2)
	}
}

func TestNamePool_Reconcile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

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

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

	tests := []struct {
		name     string
		expected bool
	}{
		{"furiosa", true},
		{"nux", true},
		{"max", true},
		{"51", false}, // overflow format (just number)
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

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

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

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

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

	pool := NewNamePoolWithConfig(tmpDir, "testrig", "mad-max", nil, DefaultPoolSize)

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

func TestThemeForRig(t *testing.T) {
	// Different rigs should get different themes (with high probability)
	themes := make(map[string]bool)
	for _, rigName := range []string{"gastown", "beads", "myproject", "webapp"} {
		themes[ThemeForRig(rigName)] = true
	}
	// Should have at least 2 different themes across 4 rigs
	if len(themes) < 2 {
		t.Errorf("expected variety in themes, got only %d unique theme(s)", len(themes))
	}
}

func TestThemeForRigDeterministic(t *testing.T) {
	// Same rig name should always get same theme
	theme1 := ThemeForRig("myrig")
	theme2 := ThemeForRig("myrig")
	if theme1 != theme2 {
		t.Errorf("theme not deterministic: got %q and %q", theme1, theme2)
	}
}

func TestNamePool_ReservedNamesExcluded(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Test all themes to ensure reserved names are excluded
	for themeName := range BuiltinThemes {
		pool := NewNamePoolWithConfig(tmpDir, "testrig", themeName, nil, 100)

		// Allocate all available names (up to 100)
		allocated := make(map[string]bool)
		for i := 0; i < 100; i++ {
			name, err := pool.Allocate()
			if err != nil {
				t.Fatalf("Allocate error: %v", err)
			}
			allocated[name] = true
		}

		// Verify no reserved names were allocated
		for reserved := range ReservedInfraAgentNames {
			if allocated[reserved] {
				t.Errorf("theme %q allocated reserved name %q", themeName, reserved)
			}
		}

		pool.Reset()
	}
}

func TestNamePool_ReservedNamesInCustomNames(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Custom names that include reserved names should have them filtered out
	custom := []string{"alpha", "witness", "beta", "mayor", "gamma"}
	pool := NewNamePoolWithConfig(tmpDir, "testrig", "", custom, 10)

	// Allocate all names
	allocated := make(map[string]bool)
	for i := 0; i < 5; i++ {
		name, _ := pool.Allocate()
		allocated[name] = true
	}

	// Should only get alpha, beta, gamma (3 non-reserved names)
	// Then overflow names for the remaining allocations
	if allocated["witness"] {
		t.Error("allocated reserved name 'witness' from custom names")
	}
	if allocated["mayor"] {
		t.Error("allocated reserved name 'mayor' from custom names")
	}
	if !allocated["alpha"] || !allocated["beta"] || !allocated["gamma"] {
		t.Errorf("expected alpha, beta, gamma to be allocated, got %v", allocated)
	}
}

// --- Custom theme tests ---

func TestParseThemeFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-theme-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	content := `# Tolkien characters
aragorn
legolas
gimli
GANDALF
frodo

# duplicates should be removed
aragorn
samwise
`
	path := filepath.Join(tmpDir, "tolkien.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := ParseThemeFile(path)
	if err != nil {
		t.Fatalf("ParseThemeFile error: %v", err)
	}

	expected := []string{"aragorn", "legolas", "gimli", "gandalf", "frodo", "samwise"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("name[%d] = %q, expected %q", i, name, expected[i])
		}
	}
}

func TestParseThemeFile_MinLength(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-theme-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// All names <=3 chars should be rejected
	content := "ab\nabc\nab1\nvalid-name\n"
	path := filepath.Join(tmpDir, "short.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := ParseThemeFile(path)
	if err != nil {
		t.Fatalf("ParseThemeFile error: %v", err)
	}
	if len(names) != 1 || names[0] != "valid-name" {
		t.Errorf("expected [valid-name], got %v", names)
	}
}

func TestParseThemeFile_ReservedNames(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-theme-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	content := "witness\nrefinery\nalpha\nbeta-name\n"
	path := filepath.Join(tmpDir, "reserved.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := ParseThemeFile(path)
	if err != nil {
		t.Fatalf("ParseThemeFile error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names (alpha, beta-name), got %v", names)
	}
	if names[0] != "alpha" || names[1] != "beta-name" {
		t.Errorf("expected [alpha, beta-name], got %v", names)
	}
}

func TestParseThemeFile_InvalidFormat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-theme-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Names with spaces, special chars should be filtered
	content := "valid-name\nhas space\nfoo/bar\ngood-name\n123-start\n"
	path := filepath.Join(tmpDir, "invalid.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := ParseThemeFile(path)
	if err != nil {
		t.Fatalf("ParseThemeFile error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 valid names (valid-name, good-name), got %v", names)
	}
	if names[0] != "valid-name" || names[1] != "good-name" {
		t.Errorf("expected [valid-name, good-name], got %v", names)
	}
}

func TestParseThemeFile_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-theme-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	content := "# just a comment\n\n"
	path := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = ParseThemeFile(path)
	if err == nil {
		t.Error("expected error for empty theme file")
	}
}

func TestResolveThemeNames_Builtin(t *testing.T) {
	names, err := ResolveThemeNames("/nonexistent", "mad-max")
	if err != nil {
		t.Fatalf("ResolveThemeNames error: %v", err)
	}
	if len(names) != 50 {
		t.Errorf("expected 50 names, got %d", len(names))
	}
}

func TestResolveThemeNames_Custom(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-resolve-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create theme file
	themesDir := filepath.Join(tmpDir, "settings", "themes")
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "alpha\nbeta-name\ngamma-ray\ndelta-force\n"
	if err := os.WriteFile(filepath.Join(themesDir, "test-theme.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := ResolveThemeNames(tmpDir, "test-theme")
	if err != nil {
		t.Fatalf("ResolveThemeNames error: %v", err)
	}
	if len(names) != 4 {
		t.Errorf("expected 4 names, got %d: %v", len(names), names)
	}
}

func TestResolveThemeNames_NotFound(t *testing.T) {
	_, err := ResolveThemeNames("/nonexistent", "no-such-theme")
	if err == nil {
		t.Error("expected error for missing theme")
	}
}

func TestListAllThemes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-list-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a custom theme
	themesDir := filepath.Join(tmpDir, "settings", "themes")
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "custom-one.txt"), []byte("alpha\nbeta-name\ngamma-ray\n"), 0644); err != nil {
		t.Fatal(err)
	}

	themes := ListAllThemes(tmpDir)

	// Should have 3 built-in + 1 custom = 4
	if len(themes) != 4 {
		t.Fatalf("expected 4 themes, got %d: %v", len(themes), themes)
	}

	// Find custom theme
	found := false
	for _, ti := range themes {
		if ti.Name == "custom-one" {
			found = true
			if !ti.IsCustom {
				t.Error("expected custom-one to be marked as custom")
			}
			if ti.Count != 3 {
				t.Errorf("expected 3 names in custom-one, got %d", ti.Count)
			}
		}
	}
	if !found {
		t.Error("custom-one theme not found in ListAllThemes")
	}
}

func TestIsBuiltinTheme(t *testing.T) {
	if !IsBuiltinTheme("mad-max") {
		t.Error("mad-max should be built-in")
	}
	if !IsBuiltinTheme("minerals") {
		t.Error("minerals should be built-in")
	}
	if IsBuiltinTheme("tolkien") {
		t.Error("tolkien should not be built-in")
	}
}

func TestValidatePoolName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"furiosa", false},
		{"beta-name", false},
		{"alpha123", false},
		{"abc", true},  // too short
		{"ab", true},   // too short
		{"", true},     // empty
		{"UPPER", true}, // uppercase
		{"has space", true},
		{"witness", true},  // reserved
		{"refinery", true}, // reserved
		{"valid-name", false},
	}
	for _, tc := range tests {
		err := ValidatePoolName(tc.name)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidatePoolName(%q) error=%v, wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

func TestSetTheme_Custom(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-settheme-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create custom theme file
	themesDir := filepath.Join(tmpDir, "settings", "themes")
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "tolkien.txt"), []byte("aragorn\nlegolas\ngimli-son\ngandalf\n"), 0644); err != nil {
		t.Fatal(err)
	}

	pool := NewNamePool(tmpDir, "testrig")
	pool.SetTownRoot(tmpDir)

	if err := pool.SetTheme("tolkien"); err != nil {
		t.Fatalf("SetTheme error: %v", err)
	}
	if pool.GetTheme() != "tolkien" {
		t.Errorf("expected theme tolkien, got %s", pool.GetTheme())
	}
}

func TestCustomTheme_Allocation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-alloc-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create custom theme file
	themesDir := filepath.Join(tmpDir, "settings", "themes")
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		t.Fatal(err)
	}
	names := "aragorn\nlegolas\ngimli-son\ngandalf\nfrodo-baggins\nsamwise\n"
	if err := os.WriteFile(filepath.Join(themesDir, "tolkien.txt"), []byte(names), 0644); err != nil {
		t.Fatal(err)
	}

	// Create pool with custom theme
	pool := NewNamePoolWithConfig(tmpDir, "testrig", "tolkien", nil, 10)
	pool.SetTownRoot(tmpDir)

	// Allocate should return custom theme names
	name, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error: %v", err)
	}
	if name != "aragorn" {
		t.Errorf("expected aragorn, got %s", name)
	}

	name, _ = pool.Allocate()
	if name != "legolas" {
		t.Errorf("expected legolas, got %s", name)
	}
}

func TestSaveCustomTheme(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-save-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	names := []string{"aragorn", "legolas", "gimli-son"}
	if err := SaveCustomTheme(tmpDir, "tolkien", names); err != nil {
		t.Fatalf("SaveCustomTheme error: %v", err)
	}

	// Re-read and verify
	loaded, err := ParseThemeFile(filepath.Join(tmpDir, "settings", "themes", "tolkien.txt"))
	if err != nil {
		t.Fatalf("ParseThemeFile error: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 names, got %d", len(loaded))
	}
	for i, name := range names {
		if loaded[i] != name {
			t.Errorf("loaded[%d] = %q, expected %q", i, loaded[i], name)
		}
	}
}

func TestSaveCustomTheme_AppendName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-append-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create initial theme
	initial := []string{"aragorn", "legolas", "gimli-son"}
	if err := SaveCustomTheme(tmpDir, "tolkien", initial); err != nil {
		t.Fatalf("SaveCustomTheme error: %v", err)
	}

	// Read back, append, save again (simulates what `gt namepool add` does)
	existing, err := ResolveThemeNames(tmpDir, "tolkien")
	if err != nil {
		t.Fatalf("ResolveThemeNames error: %v", err)
	}
	updated := append(existing, "gandalf")
	if err := SaveCustomTheme(tmpDir, "tolkien", updated); err != nil {
		t.Fatalf("SaveCustomTheme (append) error: %v", err)
	}

	// Verify the theme now has 4 names
	final, err := ResolveThemeNames(tmpDir, "tolkien")
	if err != nil {
		t.Fatalf("ResolveThemeNames error: %v", err)
	}
	if len(final) != 4 {
		t.Fatalf("expected 4 names after append, got %d: %v", len(final), final)
	}
	if final[3] != "gandalf" {
		t.Errorf("expected gandalf at index 3, got %s", final[3])
	}

	// Verify pool allocation uses the updated theme
	pool := NewNamePoolWithConfig(tmpDir, "testrig", "tolkien", nil, 10)
	pool.SetTownRoot(tmpDir)
	for i, expected := range []string{"aragorn", "legolas", "gimli-son", "gandalf"} {
		name, err := pool.Allocate()
		if err != nil {
			t.Fatalf("Allocate error: %v", err)
		}
		if name != expected {
			t.Errorf("allocation %d: expected %s, got %s", i, expected, name)
		}
	}
}

func TestSaveCustomTheme_BuiltinConflict(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-save-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	err = SaveCustomTheme(tmpDir, "mad-max", []string{"test-name"})
	if err == nil {
		t.Error("expected error when creating theme that conflicts with built-in")
	}
}

func TestDeleteCustomTheme(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-delete-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create then delete
	names := []string{"aragorn", "legolas", "gimli-son"}
	if err := SaveCustomTheme(tmpDir, "tolkien", names); err != nil {
		t.Fatalf("SaveCustomTheme error: %v", err)
	}

	if err := DeleteCustomTheme(tmpDir, "tolkien"); err != nil {
		t.Fatalf("DeleteCustomTheme error: %v", err)
	}

	// Verify gone
	path := filepath.Join(tmpDir, "settings", "themes", "tolkien.txt")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected theme file to be deleted")
	}
}

func TestDeleteCustomTheme_BuiltinRefused(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-delete-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	err = DeleteCustomTheme(tmpDir, "mad-max")
	if err == nil {
		t.Error("expected error when deleting built-in theme")
	}
}

func TestFindRigsUsingTheme(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-findrig-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create mayor/rigs.json with two rigs
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"rig-alpha":{},"rig-beta":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// rig-alpha uses "tolkien" theme
	alphaSettings := filepath.Join(tmpDir, "rig-alpha", "settings")
	if err := os.MkdirAll(alphaSettings, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alphaSettings, "config.json"), []byte(`{"namepool":{"style":"tolkien"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// rig-beta uses "mad-max" theme
	betaSettings := filepath.Join(tmpDir, "rig-beta", "settings")
	if err := os.MkdirAll(betaSettings, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(betaSettings, "config.json"), []byte(`{"namepool":{"style":"mad-max"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Should find rig-alpha using tolkien
	using := FindRigsUsingTheme(tmpDir, "tolkien")
	if len(using) != 1 || using[0] != "rig-alpha" {
		t.Errorf("expected [rig-alpha], got %v", using)
	}

	// Should find rig-beta using mad-max
	using = FindRigsUsingTheme(tmpDir, "mad-max")
	if len(using) != 1 || using[0] != "rig-beta" {
		t.Errorf("expected [rig-beta], got %v", using)
	}

	// Should find nothing for unused theme
	using = FindRigsUsingTheme(tmpDir, "unused-theme")
	if len(using) != 0 {
		t.Errorf("expected empty, got %v", using)
	}
}

func TestDeleteCustomTheme_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-delete-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	err = DeleteCustomTheme(tmpDir, "nonexistent")
	if err == nil {
		t.Error("expected error when deleting nonexistent theme")
	}
}

func TestDeleteCustomTheme_PathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-delete-traversal-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// ValidatePoolName should reject path traversal attempts
	err = ValidatePoolName("../../etc/passwd")
	if err == nil {
		t.Error("expected ValidatePoolName to reject path traversal")
	}

	err = ValidatePoolName("../secret")
	if err == nil {
		t.Error("expected ValidatePoolName to reject path with dots")
	}
}

func TestAppendToCustomTheme(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-append-atomic-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create initial theme
	initial := []string{"aragorn", "legolas", "gimli-son"}
	if err := SaveCustomTheme(tmpDir, "tolkien", initial); err != nil {
		t.Fatalf("SaveCustomTheme error: %v", err)
	}

	// Append a new name
	alreadyExists, err := AppendToCustomTheme(tmpDir, "tolkien", "gandalf")
	if err != nil {
		t.Fatalf("AppendToCustomTheme error: %v", err)
	}
	if alreadyExists {
		t.Error("expected alreadyExists=false for new name")
	}

	// Append a duplicate
	alreadyExists, err = AppendToCustomTheme(tmpDir, "tolkien", "aragorn")
	if err != nil {
		t.Fatalf("AppendToCustomTheme error: %v", err)
	}
	if !alreadyExists {
		t.Error("expected alreadyExists=true for duplicate name")
	}

	// Verify final state
	names, err := ResolveThemeNames(tmpDir, "tolkien")
	if err != nil {
		t.Fatalf("ResolveThemeNames error: %v", err)
	}
	if len(names) != 4 {
		t.Fatalf("expected 4 names, got %d: %v", len(names), names)
	}
	if names[3] != "gandalf" {
		t.Errorf("expected gandalf at index 3, got %s", names[3])
	}
}

func TestParseThemeFile_MaxSize(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-maxsize-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Generate a file with MaxThemeNames+1 valid names
	var sb strings.Builder
	for i := 0; i <= MaxThemeNames; i++ {
		fmt.Fprintf(&sb, "name-%06d\n", i)
	}
	path := filepath.Join(tmpDir, "huge.txt")
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = ParseThemeFile(path)
	if err == nil {
		t.Errorf("expected error when theme file exceeds %d names", MaxThemeNames)
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected 'exceeds maximum' error, got: %v", err)
	}
}

func TestGetNames_FallbackOnDeletedThemeFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "namepool-fallback-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a custom theme file
	themesDir := filepath.Join(tmpDir, "settings", "themes")
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		t.Fatal(err)
	}
	themePath := filepath.Join(themesDir, "ephemeral.txt")
	if err := os.WriteFile(themePath, []byte("alpha-one\nbeta-two\ngamma-three\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create pool pointing at custom theme
	pool := NewNamePoolWithConfig(tmpDir, "testrig", "ephemeral", nil, 10)
	pool.SetTownRoot(tmpDir)

	// Verify custom theme works
	name, err := pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error: %v", err)
	}
	if name != "alpha-one" {
		t.Errorf("expected alpha-one, got %s", name)
	}
	pool.Release(name)

	// Delete the theme file out from under the pool
	if err := os.Remove(themePath); err != nil {
		t.Fatal(err)
	}

	// Pool should silently fall back to default theme
	pool.Reset()
	name, err = pool.Allocate()
	if err != nil {
		t.Fatalf("Allocate error after fallback: %v", err)
	}

	// Should get the first name from the default theme (mad-max), which is "furiosa"
	if name != "furiosa" {
		t.Errorf("expected fallback to default theme (furiosa), got %s", name)
	}
}
