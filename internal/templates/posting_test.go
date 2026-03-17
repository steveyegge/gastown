package templates

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// 3-layer resolution precedence tests for LoadPosting
// ---------------------------------------------------------------------------

func TestLoadPosting_EmbeddedBuiltin(t *testing.T) {
	t.Parallel()
	// With no rig or town overrides, should resolve from embedded postings.
	for _, name := range BuiltinPostingNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := LoadPosting("", "", name)
			if err != nil {
				t.Fatalf("LoadPosting(%q) error: %v", name, err)
			}
			if result.Level != "embedded" {
				t.Errorf("Level = %q, want %q", result.Level, "embedded")
			}
			if result.Name != name {
				t.Errorf("Name = %q, want %q", result.Name, name)
			}
			if result.Content == "" {
				t.Error("Content is empty")
			}
		})
	}
}

func TestLoadPosting_TownOverridesEmbedded(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	// Create a town-level override for "dispatcher"
	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	townContent := "# Town-level dispatcher override"
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte(townContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadPosting(townRoot, "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting() error: %v", err)
	}
	if result.Level != "town" {
		t.Errorf("Level = %q, want %q", result.Level, "town")
	}
	if result.Content != townContent {
		t.Errorf("Content = %q, want %q", result.Content, townContent)
	}
}

func TestLoadPosting_RigOverridesTownAndEmbedded(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir()

	// Create both town-level and rig-level overrides
	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte("town content"), 0644); err != nil {
		t.Fatal(err)
	}

	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigContent := "# Rig-level dispatcher override"
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "dispatcher.md.tmpl"), []byte(rigContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadPosting(townRoot, rigPath, "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting() error: %v", err)
	}
	if result.Level != "rig" {
		t.Errorf("Level = %q, want %q (rig should override town and embedded)", result.Level, "rig")
	}
	if result.Content != rigContent {
		t.Errorf("Content = %q, want %q", result.Content, rigContent)
	}
}

func TestLoadPosting_TownOnlyNoRig(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir() // exists but no postings/ dir

	// Only town-level override
	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	townContent := "# Town-level custom posting"
	if err := os.WriteFile(filepath.Join(townPostingsDir, "custom-role.md.tmpl"), []byte(townContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadPosting(townRoot, rigPath, "custom-role")
	if err != nil {
		t.Fatalf("LoadPosting() error: %v", err)
	}
	if result.Level != "town" {
		t.Errorf("Level = %q, want %q", result.Level, "town")
	}
}

func TestLoadPosting_RigOnlyNoTown(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()

	// Only rig-level override (no matching embedded or town)
	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigContent := "# Rig-only posting"
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "rig-special.md.tmpl"), []byte(rigContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadPosting("", rigPath, "rig-special")
	if err != nil {
		t.Fatalf("LoadPosting() error: %v", err)
	}
	if result.Level != "rig" {
		t.Errorf("Level = %q, want %q", result.Level, "rig")
	}
}

// ---------------------------------------------------------------------------
// Nonexistent posting warning
// ---------------------------------------------------------------------------

func TestLoadPosting_NotFound(t *testing.T) {
	t.Parallel()
	result, err := LoadPosting("", "", "nonexistent-posting-xyz")
	if err == nil {
		t.Fatalf("expected error for nonexistent posting, got result: %+v", result)
	}
	if result != nil {
		t.Errorf("expected nil result for nonexistent posting, got: %+v", result)
	}
}

func TestLoadPosting_NotFoundWithPaths(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir()

	// Both paths exist but contain no matching posting
	result, err := LoadPosting(townRoot, rigPath, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent posting")
	}
	if result != nil {
		t.Errorf("expected nil result, got: %+v", result)
	}
}

func TestLoadPosting_EmptyName(t *testing.T) {
	t.Parallel()
	_, err := LoadPosting("", "", "")
	if err == nil {
		t.Fatal("expected error for empty posting name")
	}
}

// ---------------------------------------------------------------------------
// BuiltinPostingNames
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// PostingLevels ambiguity detection
// ---------------------------------------------------------------------------

func TestPostingLevels_EmbeddedOnly(t *testing.T) {
	t.Parallel()
	levels := PostingLevels("", "", "dispatcher")
	if len(levels) != 1 || levels[0] != "embedded" {
		t.Errorf("PostingLevels() = %v, want [embedded]", levels)
	}
}

func TestPostingLevels_Ambiguous_TownAndEmbedded(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte("town"), 0644); err != nil {
		t.Fatal(err)
	}

	levels := PostingLevels(townRoot, "", "dispatcher")
	if len(levels) != 2 {
		t.Fatalf("PostingLevels() = %v, want 2 levels (town + embedded)", levels)
	}
}

func TestPostingLevels_Ambiguous_AllThree(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir()

	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte("town"), 0644); err != nil {
		t.Fatal(err)
	}

	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "dispatcher.md.tmpl"), []byte("rig"), 0644); err != nil {
		t.Fatal(err)
	}

	levels := PostingLevels(townRoot, rigPath, "dispatcher")
	if len(levels) != 3 {
		t.Fatalf("PostingLevels() = %v, want 3 levels (rig + town + embedded)", levels)
	}
}

func TestPostingLevels_NotFound(t *testing.T) {
	t.Parallel()
	levels := PostingLevels("", "", "nonexistent-xyz")
	if len(levels) != 0 {
		t.Errorf("PostingLevels() = %v, want empty", levels)
	}
}

func TestPostingLevels_EmptyName(t *testing.T) {
	t.Parallel()
	levels := PostingLevels("", "", "")
	if levels != nil {
		t.Errorf("PostingLevels() = %v, want nil", levels)
	}
}

func TestBuiltinPostingNames(t *testing.T) {
	t.Parallel()
	names := BuiltinPostingNames()
	if len(names) == 0 {
		t.Fatal("BuiltinPostingNames() returned empty slice")
	}
	// Verify each builtin can be loaded from embedded FS
	for _, name := range names {
		result, err := LoadPosting("", "", name)
		if err != nil {
			t.Errorf("built-in posting %q cannot be loaded: %v", name, err)
		}
		if result.Level != "embedded" {
			t.Errorf("built-in posting %q level = %q, want %q", name, result.Level, "embedded")
		}
	}
}

// ---------------------------------------------------------------------------
// 15.x: Frontmatter description tests
// ---------------------------------------------------------------------------

// 15.1: LoadPosting returns correct description for dispatcher
func TestLoadPosting_DispatcherDescription(t *testing.T) {
	t.Parallel()
	result, err := LoadPosting("", "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting(dispatcher) error: %v", err)
	}
	want := "Triage and route work to polecats. Never writes code."
	if result.Description != want {
		t.Errorf("Description = %q, want %q", result.Description, want)
	}
}

// 15.2: LoadPosting returns correct description for inspector
func TestLoadPosting_InspectorDescription(t *testing.T) {
	t.Parallel()
	result, err := LoadPosting("", "", "inspector")
	if err != nil {
		t.Fatalf("LoadPosting(inspector) error: %v", err)
	}
	want := "Code review and quality gates. May write tests."
	if result.Description != want {
		t.Errorf("Description = %q, want %q", result.Description, want)
	}
}

// 15.3: LoadPosting returns correct description for scout
func TestLoadPosting_ScoutDescription(t *testing.T) {
	t.Parallel()
	result, err := LoadPosting("", "", "scout")
	if err != nil {
		t.Fatalf("LoadPosting(scout) error: %v", err)
	}
	want := "Read-only exploration and research. Never writes code."
	if result.Description != want {
		t.Errorf("Description = %q, want %q", result.Description, want)
	}
}

// 15.4: No frontmatter → empty description, content unchanged
func TestLoadPosting_NoFrontmatter_EmptyDescription(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()
	postingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rawContent := "# No frontmatter posting\n\nJust content."
	if err := os.WriteFile(filepath.Join(postingsDir, "plain.md.tmpl"), []byte(rawContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadPosting("", rigPath, "plain")
	if err != nil {
		t.Fatalf("LoadPosting(plain) error: %v", err)
	}
	if result.Description != "" {
		t.Errorf("Description = %q, want empty", result.Description)
	}
	if result.Content != rawContent {
		t.Errorf("Content changed; got %q, want %q", result.Content, rawContent)
	}
}

// 15.5: With frontmatter → content stripped of --- block
func TestLoadPosting_FrontmatterStripped(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()
	postingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	raw := "---\ndescription: Test posting\n---\n# Body content\n"
	if err := os.WriteFile(filepath.Join(postingsDir, "fmtest.md.tmpl"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadPosting("", rigPath, "fmtest")
	if err != nil {
		t.Fatalf("LoadPosting(fmtest) error: %v", err)
	}
	if result.Description != "Test posting" {
		t.Errorf("Description = %q, want %q", result.Description, "Test posting")
	}
	if result.Content != "# Body content\n" {
		t.Errorf("Content = %q, want %q", result.Content, "# Body content\n")
	}
}

// 15.7: Rig override shows rig description, not embedded
func TestLoadPosting_RigOverrideDescription(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()
	postingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	raw := "---\ndescription: Custom rig dispatcher\n---\n# Rig dispatcher\n"
	if err := os.WriteFile(filepath.Join(postingsDir, "dispatcher.md.tmpl"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadPosting("", rigPath, "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting error: %v", err)
	}
	if result.Level != "rig" {
		t.Errorf("Level = %q, want rig", result.Level)
	}
	if result.Description != "Custom rig dispatcher" {
		t.Errorf("Description = %q, want %q", result.Description, "Custom rig dispatcher")
	}
}

// ---------------------------------------------------------------------------
// parsePostingFrontmatter unit tests
// ---------------------------------------------------------------------------

func TestParsePostingFrontmatter_Empty(t *testing.T) {
	t.Parallel()
	desc, content := parsePostingFrontmatter("")
	if desc != "" || content != "" {
		t.Errorf("got desc=%q content=%q, want both empty", desc, content)
	}
}

func TestParsePostingFrontmatter_NoDelimiters(t *testing.T) {
	t.Parallel()
	raw := "# Just content\nNo frontmatter here."
	desc, content := parsePostingFrontmatter(raw)
	if desc != "" {
		t.Errorf("desc = %q, want empty", desc)
	}
	if content != raw {
		t.Errorf("content changed: %q", content)
	}
}

func TestParsePostingFrontmatter_ValidYAML(t *testing.T) {
	t.Parallel()
	raw := "---\ndescription: Hello world\n---\nBody here\n"
	desc, content := parsePostingFrontmatter(raw)
	if desc != "Hello world" {
		t.Errorf("desc = %q, want %q", desc, "Hello world")
	}
	if content != "Body here\n" {
		t.Errorf("content = %q, want %q", content, "Body here\n")
	}
}

func TestParsePostingFrontmatter_UnclosedDelimiter(t *testing.T) {
	t.Parallel()
	raw := "---\ndescription: Hello\nNo closing delimiter"
	desc, content := parsePostingFrontmatter(raw)
	if desc != "" {
		t.Errorf("desc = %q, want empty for unclosed", desc)
	}
	if content != raw {
		t.Errorf("content changed for unclosed frontmatter")
	}
}
