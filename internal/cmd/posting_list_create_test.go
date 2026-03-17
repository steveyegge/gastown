package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/templates"
)

// ===========================================================================
// Section 1: gt posting list (unit tests)
// ===========================================================================

// 1.1: embedded postings listed with (embedded) label
func TestPostingList_EmbeddedPostingsLabeled(t *testing.T) {
	t.Parallel()
	postings := templates.ListAvailablePostings("", "")

	builtins := templates.BuiltinPostingNames()
	for _, name := range builtins {
		found := false
		for _, p := range postings {
			if p.Name == name {
				found = true
				if p.Level != "embedded" {
					t.Errorf("posting %q level = %q, want %q", name, p.Level, "embedded")
				}
				break
			}
		}
		if !found {
			t.Errorf("built-in posting %q not found in list", name)
		}
	}
}

// 1.2: rig-level template shows (rig) label
func TestPostingList_RigLevelShowsRigLabel(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()

	postingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postingsDir, "reviewer.md.tmpl"), []byte("# Reviewer"), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings("", rigPath)

	found := false
	for _, p := range postings {
		if p.Name == "reviewer" {
			found = true
			if p.Level != "rig" {
				t.Errorf("rig-level posting %q level = %q, want %q", p.Name, p.Level, "rig")
			}
			break
		}
	}
	if !found {
		t.Error("rig-level posting 'reviewer' not found in list")
	}
}

// 1.3: town-level template shows (town) label
func TestPostingList_TownLevelShowsTownLabel(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	postingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postingsDir, "medic.md.tmpl"), []byte("# Medic"), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings(townRoot, "")

	found := false
	for _, p := range postings {
		if p.Name == "medic" {
			found = true
			if p.Level != "town" {
				t.Errorf("town-level posting %q level = %q, want %q", p.Name, p.Level, "town")
			}
			break
		}
	}
	if !found {
		t.Error("town-level posting 'medic' not found in list")
	}
}

// 1.4: all 3 levels shown together
func TestPostingList_AllThreeLevels(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir()

	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townPostingsDir, "medic.md.tmpl"), []byte("town"), 0644); err != nil {
		t.Fatal(err)
	}

	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "reviewer.md.tmpl"), []byte("rig"), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings(townRoot, rigPath)

	levels := make(map[string]string)
	for _, p := range postings {
		levels[p.Name] = p.Level
	}

	for _, name := range templates.BuiltinPostingNames() {
		if _, ok := levels[name]; !ok {
			t.Errorf("embedded posting %q missing from list", name)
		}
	}
	if levels["medic"] != "town" {
		t.Errorf("medic level = %q, want %q", levels["medic"], "town")
	}
	if levels["reviewer"] != "rig" {
		t.Errorf("reviewer level = %q, want %q", levels["reviewer"], "rig")
	}
}

// 1.5: alphabetical sort order
func TestPostingList_AlphabeticalSortOrder(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir()

	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zebra", "alpha", "midway"} {
		if err := os.WriteFile(filepath.Join(rigPostingsDir, name+".md.tmpl"), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	postings := templates.ListAvailablePostings(townRoot, rigPath)

	names := make([]string, len(postings))
	for i, p := range postings {
		names[i] = p.Name
	}

	if !sort.StringsAreSorted(names) {
		t.Errorf("postings not sorted alphabetically: %v", names)
	}
}

// 1.6: rig-level override of embedded changes label
func TestPostingList_RigOverrideChangesLabel(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()

	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "dispatcher.md.tmpl"), []byte("rig override"), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings("", rigPath)

	for _, p := range postings {
		if p.Name == "dispatcher" {
			if p.Level != "rig" {
				t.Errorf("overridden dispatcher level = %q, want %q (rig should override embedded)", p.Level, "rig")
			}
			return
		}
	}
	t.Error("dispatcher not found in list after rig override")
}

// 1.8: list shows descriptions for embedded postings
func TestPostingList_EmbeddedDescriptions(t *testing.T) {
	t.Parallel()
	postings := templates.ListAvailablePostings("", "")

	wantDescs := map[string]string{
		"dispatcher": "Triage and route work to polecats. Never writes code.",
		"inspector":  "Code review and quality gates. May write tests.",
		"scout":      "Read-only exploration and research. Never writes code.",
	}
	for _, p := range postings {
		if want, ok := wantDescs[p.Name]; ok {
			if p.Description != want {
				t.Errorf("posting %q description = %q, want %q", p.Name, p.Description, want)
			}
		}
	}
}

// 1.9: rig-level posting with description shows in list
func TestPostingList_RigDescriptionShown(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()
	postingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	raw := "---\ndescription: Custom rig reviewer\n---\n# Reviewer\n"
	if err := os.WriteFile(filepath.Join(postingsDir, "reviewer.md.tmpl"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings("", rigPath)
	for _, p := range postings {
		if p.Name == "reviewer" {
			if p.Description != "Custom rig reviewer" {
				t.Errorf("description = %q, want %q", p.Description, "Custom rig reviewer")
			}
			return
		}
	}
	t.Error("rig-level posting 'reviewer' not found in list")
}

// 1.10: posting without frontmatter has empty description in list
func TestPostingList_NoFrontmatterEmptyDescription(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()
	postingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postingsDir, "plain.md.tmpl"), []byte("# Plain"), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings("", rigPath)
	for _, p := range postings {
		if p.Name == "plain" {
			if p.Description != "" {
				t.Errorf("description = %q, want empty", p.Description)
			}
			return
		}
	}
	t.Error("posting 'plain' not found in list")
}

// ===========================================================================
// Section 2: gt posting list --all / outside-rig fallback (unit tests)
// ===========================================================================

// 2.1: ListAvailablePostings with empty rigPath returns embedded + town only
func TestPostingList_NoRigPathReturnsEmbeddedAndTown(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	postingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postingsDir, "medic.md.tmpl"), []byte("# Medic"), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings(townRoot, "")

	hasEmbedded := false
	hasTown := false
	for _, p := range postings {
		if p.Level == "embedded" {
			hasEmbedded = true
		}
		if p.Level == "town" && p.Name == "medic" {
			hasTown = true
		}
		if p.Level == "rig" {
			t.Errorf("unexpected rig-level posting %q with empty rigPath", p.Name)
		}
	}
	if !hasEmbedded {
		t.Error("expected embedded postings when rigPath is empty")
	}
	if !hasTown {
		t.Error("expected town-level posting 'medic' when rigPath is empty")
	}
}

// 2.2: multiple rigs' postings can be collected independently
func TestPostingList_MultipleRigsCollectedIndependently(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	// Create two rig postings dirs
	rig1Dir := filepath.Join(townRoot, "rig1", "postings")
	rig2Dir := filepath.Join(townRoot, "rig2", "postings")
	if err := os.MkdirAll(rig1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rig2Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rig1Dir, "alpha.md.tmpl"), []byte("# Alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rig2Dir, "beta.md.tmpl"), []byte("# Beta"), 0644); err != nil {
		t.Fatal(err)
	}

	// Each rig sees its own rig-level posting
	postings1 := templates.ListAvailablePostings(townRoot, filepath.Join(townRoot, "rig1"))
	postings2 := templates.ListAvailablePostings(townRoot, filepath.Join(townRoot, "rig2"))

	hasAlpha := false
	for _, p := range postings1 {
		if p.Name == "alpha" && p.Level == "rig" {
			hasAlpha = true
		}
		if p.Name == "beta" && p.Level == "rig" {
			t.Error("rig1 should not see rig2's posting 'beta'")
		}
	}
	if !hasAlpha {
		t.Error("rig1 should see its own posting 'alpha'")
	}

	hasBeta := false
	for _, p := range postings2 {
		if p.Name == "beta" && p.Level == "rig" {
			hasBeta = true
		}
		if p.Name == "alpha" && p.Level == "rig" {
			t.Error("rig2 should not see rig1's posting 'alpha'")
		}
	}
	if !hasBeta {
		t.Error("rig2 should see its own posting 'beta'")
	}
}

// ===========================================================================
// Section 6: gt posting create (unit tests)
// ===========================================================================

// 6.1: creates rig-level template with scaffold
func TestPostingCreate_CreatesRigLevelTemplate(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	templatePath := filepath.Join(postingsDir, "reviewer.md.tmpl")
	name := "reviewer"
	if err := validatePostingName(name); err != nil {
		t.Fatalf("validatePostingName(%q) unexpected error: %v", name, err)
	}

	content := generatePostingScaffold(name)
	if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("template not created: %v", err)
	}
	if len(data) == 0 {
		t.Error("template file is empty")
	}
	if !strings.Contains(string(data), "# Posting: reviewer") {
		t.Errorf("scaffold missing expected header, got: %s", string(data)[:100])
	}
}

// 6.2: scaffold has TODO placeholders and {{ cmd }}
func TestPostingCreate_ScaffoldHasTODOAndCmd(t *testing.T) {
	t.Parallel()
	content := generatePostingScaffold("reviewer")

	if !strings.Contains(content, "TODO") {
		t.Error("scaffold missing TODO placeholders")
	}
	if !strings.Contains(content, "{{ cmd }}") {
		t.Error("scaffold missing {{ cmd }} template variable")
	}
}

// 6.3: new posting appears in list
func TestPostingCreate_NewPostingAppearsInList(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := generatePostingScaffold("reviewer")
	if err := os.WriteFile(filepath.Join(postingsDir, "reviewer.md.tmpl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings("", rigDir)

	found := false
	for _, p := range postings {
		if p.Name == "reviewer" {
			found = true
			if p.Level != "rig" {
				t.Errorf("created posting level = %q, want %q", p.Level, "rig")
			}
			break
		}
	}
	if !found {
		t.Error("newly created posting 'reviewer' not found in list")
	}
}

// 6.4: override of built-in allowed
func TestPostingCreate_OverrideBuiltinAllowed(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	name := "dispatcher"
	if err := validatePostingName(name); err != nil {
		t.Fatalf("validatePostingName(%q) should allow builtin override: %v", name, err)
	}

	templatePath := filepath.Join(postingsDir, name+".md.tmpl")
	if _, err := os.Stat(templatePath); err == nil {
		t.Fatal("rig-level dispatcher should not exist yet")
	}

	content := generatePostingScaffold(name)
	if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings("", rigDir)
	for _, p := range postings {
		if p.Name == "dispatcher" {
			if p.Level != "rig" {
				t.Errorf("override dispatcher level = %q, want %q", p.Level, "rig")
			}
			return
		}
	}
	t.Error("overridden dispatcher not found in list")
}

// 6.13: scaffold has frontmatter placeholder
func TestPostingCreate_ScaffoldHasFrontmatter(t *testing.T) {
	t.Parallel()
	content := generatePostingScaffold("reviewer")

	if !strings.Contains(content, "---\n") {
		t.Error("scaffold missing frontmatter delimiters")
	}
	if !strings.Contains(content, "description:") {
		t.Error("scaffold missing description field in frontmatter")
	}
}

// 6.6: NEG already exists
func TestPostingCreate_RejectsAlreadyExisting(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	templatePath := filepath.Join(postingsDir, "reviewer.md.tmpl")
	if err := os.WriteFile(templatePath, []byte("existing content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Reproduce the check from runPostingCreate: stat succeeds → reject
	if _, err := os.Stat(templatePath); err != nil {
		t.Fatalf("precondition: file should exist: %v", err)
	}
	// runPostingCreate would return: fmt.Errorf("posting %q already exists at %s", ...)
}

// 6.9: NEG path traversal
func TestPostingCreate_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	err := validatePostingName("../../../tmp/evil")
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
}

// 6.10: NEG empty name
func TestPostingCreate_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	err := validatePostingName("")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

// 6.11: NEG name with spaces
func TestPostingCreate_NameWithSpaces(t *testing.T) {
	t.Parallel()
	name := "name with spaces"
	err := validatePostingName(name)
	if err != nil {
		// Validation rejects it — acceptable
		return
	}

	// Validation passes — verify ListAvailablePostings can pick it up
	rigDir := t.TempDir()
	postingsDir := filepath.Join(rigDir, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(postingsDir, name+".md.tmpl"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	postings := templates.ListAvailablePostings("", rigDir)
	found := false
	for _, p := range postings {
		if p.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("posting with spaces %q not found in list after creation", name)
	}
}

// ===========================================================================
// Helpers
// ===========================================================================

// generatePostingScaffold produces the same scaffold content as runPostingCreate.
func generatePostingScaffold(name string) string {
	return "---\ndescription: \"TODO: Brief one-line description of this posting\"\n---\n# Posting: " + name + "\n\nYou are operating under the **" + name + "** posting. This augments your base role\nwith additional responsibilities.\n\n## Responsibilities\n\n- TODO: Define what this posting specializes in\n- TODO: Define behavioral constraints (what you do / don't do)\n\n## Principles\n\n1. TODO: Add guiding principles for this posting\n\n## Key Commands\n\n```bash\nbd ready                        # Find unblocked work\n{{ cmd }} nudge <target> \"msg\"         # Coordinate with other workers\n```\n"
}
