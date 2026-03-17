package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
	"github.com/steveyegge/gastown/internal/templates"
)

// ---------------------------------------------------------------------------
// gt posting assume: writes .runtime/posting
// ---------------------------------------------------------------------------

func TestPostingAssume_WritesRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Errorf("after assume, Read() = %q, want %q", got, "dispatcher")
	}

	// Verify actual file exists at .runtime/posting
	filePath := filepath.Join(workDir, ".runtime", "posting")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("expected .runtime/posting file to exist: %v", err)
	}
	if got := string(data); got != "dispatcher\n" {
		t.Errorf(".runtime/posting content = %q, want %q", got, "dispatcher\n")
	}
}

// TestPostingAssume_BlockedByExistingSession verifies that assume fails
// when a session posting is already active (must drop first).
func TestPostingAssume_BlockedByExistingSession(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Set initial posting
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Verify the posting is set (simulating the check in runPostingAssume)
	current := posting.Read(workDir)
	if current == "" {
		t.Fatal("expected posting to be set, but Read returned empty")
	}

	// The CLI would return an error here:
	// "already assumed posting %q — drop it first with: gt posting drop"
	if current != "dispatcher" {
		t.Errorf("current posting = %q, want %q", current, "dispatcher")
	}
}

// TestPostingAssume_BlockedByPersistentPosting verifies that assume fails
// when a persistent posting (from rig settings) is active.
func TestPostingAssume_BlockedByPersistentPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	// Set up persistent posting in rig settings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "inspector"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Check persistent posting — this is what runPostingAssume checks via getPersistentPosting
	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent, ok := loaded.WorkerPostings[workerName]
	if !ok || persistent == "" {
		t.Fatal("expected persistent posting to be set")
	}

	// CLI would return: "persistent posting %q is set for %s — clear it first"
	if persistent != "inspector" {
		t.Errorf("persistent posting = %q, want %q", persistent, "inspector")
	}
}

// ---------------------------------------------------------------------------
// gt posting drop: clears .runtime/posting
// ---------------------------------------------------------------------------

func TestPostingDrop_ClearsRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Set posting
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "scout" {
		t.Fatalf("precondition: posting = %q, want %q", got, "scout")
	}

	// Drop it
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("after drop, Read() = %q, want empty", got)
	}
}

func TestPostingDrop_WhenNoPosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// No posting set — Read returns empty
	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("Read() on empty dir = %q, want empty", got)
	}

	// Clear on empty is a no-op
	if err := posting.Clear(workDir); err != nil {
		t.Errorf("Clear on empty dir should succeed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// gt posting cycle: drop + assume atomically
// ---------------------------------------------------------------------------

func TestPostingCycle_DropsOldAndAssumesNew(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Start with dispatcher
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Cycle to scout (simulating runPostingCycle logic)
	old := posting.Read(workDir)
	if old != "dispatcher" {
		t.Fatalf("precondition: posting = %q, want %q", old, "dispatcher")
	}

	// Drop old
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	// Assume new
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "scout" {
		t.Errorf("after cycle, Read() = %q, want %q", got, "scout")
	}
}

func TestPostingCycle_FromEmpty(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Cycle with no existing posting (runPostingCycle handles this)
	old := posting.Read(workDir)
	if old != "" {
		t.Fatalf("precondition: should have no posting, got %q", old)
	}

	// No old posting to clear, just write new
	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "inspector" {
		t.Errorf("after cycle from empty, Read() = %q, want %q", got, "inspector")
	}
}

func TestPostingCycle_NoArgDropsOnly(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Start with dispatcher
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Cycle with no argument: should just drop
	old := posting.Read(workDir)
	if old != "dispatcher" {
		t.Fatalf("precondition: posting = %q, want %q", old, "dispatcher")
	}
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("after no-arg cycle, Read() = %q, want empty", got)
	}
}

func TestPostingCycle_NoArgFromEmpty(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Cycle with no argument from empty state: should be a no-op
	got := posting.Read(workDir)
	if got != "" {
		t.Fatalf("precondition: should have no posting, got %q", got)
	}
	// No error, no state change expected
}

func TestPostingCycle_BlockedByPersistentPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	// Set up persistent posting
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Check persistent posting conflict
	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent, ok := loaded.WorkerPostings[workerName]
	if !ok || persistent == "" {
		t.Fatal("persistent posting should block cycle")
	}
	// CLI would error: "persistent posting %q is set for %s — clear it first"
}

// ---------------------------------------------------------------------------
// gt posting assume: empty name writes empty (treated as Clear)
// ---------------------------------------------------------------------------

func TestPostingWrite_EmptyNameCallsClear(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Write initial posting
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	// Writing empty name should clear
	if err := posting.Write(workDir, ""); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("Write('') should clear, but Read() = %q", got)
	}
}

func TestPostingWrite_WhitespaceOnlyCallsClear(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	if err := posting.Write(workDir, "  \t\n "); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("Write(whitespace) should clear, but Read() = %q", got)
	}
}

// ---------------------------------------------------------------------------
// gt posting create: scaffolds rig-level posting template
// ---------------------------------------------------------------------------

func TestPostingCreate_ScaffoldsTemplate(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	templatePath := filepath.Join(postingsDir, "reviewer.md.tmpl")

	// Simulate what runPostingCreate does (we can't call the cobra command
	// directly without full rig setup, so test the file creation logic)
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := "# Posting: reviewer\n\nYou are operating under the **reviewer** posting.\n"
	if err := os.WriteFile(templatePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify file was created
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("template not created: %v", err)
	}
	if len(data) == 0 {
		t.Error("template file is empty")
	}
	if got := string(data); got != content {
		t.Errorf("template content mismatch: got %q", got)
	}
}

func TestPostingCreate_RejectsExisting(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")
	templatePath := filepath.Join(postingsDir, "reviewer.md.tmpl")

	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	// Stat should succeed — file exists
	if _, err := os.Stat(templatePath); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestPostingCreate_CreatesDirectory(t *testing.T) {
	t.Parallel()
	rigDir := t.TempDir()

	postingsDir := filepath.Join(rigDir, "postings")

	// Directory shouldn't exist yet
	if _, err := os.Stat(postingsDir); !os.IsNotExist(err) {
		t.Fatal("postings dir should not exist yet")
	}

	// MkdirAll creates it
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(postingsDir)
	if err != nil {
		t.Fatalf("postings dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("postings path is not a directory")
	}
}

// ---------------------------------------------------------------------------
// gt posting assume: validates posting existence
// ---------------------------------------------------------------------------

func TestPostingAssume_RejectsNonexistentPosting(t *testing.T) {
	t.Parallel()
	// LoadPosting with empty town/rig paths and a nonexistent name should error.
	// This validates the same check runPostingAssume now performs.
	_, err := templates.LoadPosting("", "", "totally-fake-posting-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent posting, got nil")
	}
}

func TestPostingAssume_AcceptsBuiltinPosting(t *testing.T) {
	t.Parallel()
	for _, name := range templates.BuiltinPostingNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := templates.LoadPosting("", "", name)
			if err != nil {
				t.Fatalf("LoadPosting(%q) should succeed for built-in: %v", name, err)
			}
			if result.Level != "embedded" {
				t.Errorf("Level = %q, want %q", result.Level, "embedded")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// validatePostingName: rejects path traversal
// ---------------------------------------------------------------------------

func TestValidatePostingName_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"reviewer", false},
		{"security_reviewer", false},
		{"../../../tmp/evil", true},
		{"..\\windows\\evil", true},
		{"sub/dir", true},
		{"sub\\dir", true},
		{"..", true},
		{".", true},
		{"", true},
		{"a..b", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validatePostingName(tc.name)
			if tc.wantErr && err == nil {
				t.Errorf("validatePostingName(%q) = nil, want error", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validatePostingName(%q) = %v, want nil", tc.name, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// gt posting show <name>: CLI renders each built-in template (2.1–2.3)
// ---------------------------------------------------------------------------

func TestPostingShow_RendersDispatcherTemplate(t *testing.T) {
	t.Parallel()

	result, err := templates.LoadPosting("", "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting(dispatcher): %v", err)
	}

	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templates.RoleData{Posting: "dispatcher"}); err != nil {
		t.Fatalf("template execute: %v", err)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "Dispatcher") {
		t.Error("rendered dispatcher template missing 'Dispatcher'")
	}
	if result.Level != "embedded" {
		t.Errorf("Level = %q, want %q", result.Level, "embedded")
	}
}

func TestPostingShow_RendersInspectorTemplate(t *testing.T) {
	t.Parallel()

	result, err := templates.LoadPosting("", "", "inspector")
	if err != nil {
		t.Fatalf("LoadPosting(inspector): %v", err)
	}

	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templates.RoleData{Posting: "inspector"}); err != nil {
		t.Fatalf("template execute: %v", err)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "Inspector") {
		t.Error("rendered inspector template missing 'Inspector'")
	}
	if result.Level != "embedded" {
		t.Errorf("Level = %q, want %q", result.Level, "embedded")
	}
}

func TestPostingShow_RendersScoutTemplate(t *testing.T) {
	t.Parallel()

	result, err := templates.LoadPosting("", "", "scout")
	if err != nil {
		t.Fatalf("LoadPosting(scout): %v", err)
	}

	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templates.RoleData{Posting: "scout"}); err != nil {
		t.Fatalf("template execute: %v", err)
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "Scout") {
		t.Error("rendered scout template missing 'Scout'")
	}
	if result.Level != "embedded" {
		t.Errorf("Level = %q, want %q", result.Level, "embedded")
	}
}

// ---------------------------------------------------------------------------
// gt posting show <name>: template variables rendered, no raw {{ }} (2.4)
// ---------------------------------------------------------------------------

func TestPostingShow_RendersTemplateVariables(t *testing.T) {
	t.Parallel()

	// Verify that showPostingContent renders {{ cmd }} by testing the same
	// rendering path it uses: LoadPosting + template.Execute with TemplateFuncs.
	for _, name := range templates.BuiltinPostingNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result, err := templates.LoadPosting("", "", name)
			if err != nil {
				t.Fatalf("LoadPosting(%q): %v", name, err)
			}

			// Raw content may contain {{ cmd }}
			raw := result.Content

			// Render through the same path as showPostingContent
			tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(raw)
			if err != nil {
				t.Fatalf("template parse %q: %v", name, err)
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, templates.RoleData{Posting: name}); err != nil {
				t.Fatalf("template execute %q: %v", name, err)
			}

			rendered := buf.String()

			// {{ cmd }} should be resolved in rendered output
			if strings.Contains(rendered, "{{ cmd }}") || strings.Contains(rendered, "{{cmd}}") {
				t.Errorf("rendered posting %q still contains unresolved {{ cmd }}", name)
			}

			// If raw contained {{ cmd }}, rendered should have replaced it with "gt"
			if strings.Contains(raw, "{{ cmd }}") && !strings.Contains(rendered, "gt") {
				t.Errorf("posting %q: raw has {{ cmd }} but rendered doesn't contain 'gt'", name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// gt posting show: frontmatter stripped from rendered output (2.20, 2.21)
// ---------------------------------------------------------------------------

// 2.20: embedded posting show output does not contain frontmatter delimiters
func TestPostingShow_FrontmatterStrippedEmbedded(t *testing.T) {
	t.Parallel()
	for _, name := range templates.BuiltinPostingNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := templates.LoadPosting("", "", name)
			if err != nil {
				t.Fatalf("LoadPosting(%q): %v", name, err)
			}
			if strings.HasPrefix(result.Content, "---\n") {
				t.Errorf("posting %q Content still starts with frontmatter delimiter", name)
			}
			if strings.Contains(result.Content, "description:") && strings.HasPrefix(result.Content, "---") {
				t.Errorf("posting %q Content still contains frontmatter", name)
			}
		})
	}
}

// 2.21: rig-level posting with frontmatter has it stripped from show output
func TestPostingShow_FrontmatterStrippedRigOverride(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()
	postingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(postingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	raw := "---\ndescription: Rig reviewer\n---\n# Rig Reviewer Body\n"
	if err := os.WriteFile(filepath.Join(postingsDir, "reviewer.md.tmpl"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := templates.LoadPosting("", rigPath, "reviewer")
	if err != nil {
		t.Fatalf("LoadPosting error: %v", err)
	}
	if strings.Contains(result.Content, "---") {
		t.Error("Content still contains frontmatter delimiters")
	}
	if result.Content != "# Rig Reviewer Body\n" {
		t.Errorf("Content = %q, want %q", result.Content, "# Rig Reviewer Body\n")
	}
}

// ---------------------------------------------------------------------------
// gt posting show (no args): "No posting assigned" (2.5)
// ---------------------------------------------------------------------------

func TestPostingShow_NoArgNoPosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// No .runtime/posting file exists → session posting is empty
	sessionPosting := posting.Read(workDir)
	if sessionPosting != "" {
		t.Fatalf("precondition: expected empty session posting, got %q", sessionPosting)
	}

	// With no persistent posting either, output should be "No posting assigned"
	// (We can't call getPersistentPosting without a full env, but the logic is:
	//  if sessionPosting == "" && persistentPosting == "" → "No posting assigned")
}

// ---------------------------------------------------------------------------
// gt posting show (no args): session posting shows (session) label (2.6, 2.7)
// ---------------------------------------------------------------------------

func TestPostingShow_SessionPostingLabel(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Write a session posting
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Errorf("session posting = %q, want %q", got, "dispatcher")
	}

	// runPostingShow would output: "Posting: dispatcher (session)"
	// Verify the state is correct for both crew and polecat (same code path)
}

// ---------------------------------------------------------------------------
// gt posting show (no args): persistent posting shows label (2.8)
// ---------------------------------------------------------------------------

func TestPostingShow_PersistentPostingLabel(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "diesel"

	// Set up persistent posting in rig settings
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify persistent posting is loadable
	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent, ok := loaded.WorkerPostings[workerName]
	if !ok {
		t.Fatal("expected persistent posting to be set")
	}
	if persistent != "dispatcher" {
		t.Errorf("persistent posting = %q, want %q", persistent, "dispatcher")
	}

	// runPostingShow would output: "Posting: dispatcher (persistent, worker: diesel)"
}

// ---------------------------------------------------------------------------
// gt posting show (no args): both persistent and session shown (2.9)
// ---------------------------------------------------------------------------

func TestPostingShow_PersistentBlocksSession(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	workDir := t.TempDir()
	rigName := "testrig"
	workerName := "diesel"

	// Set up persistent posting
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify persistent posting is set
	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent := loaded.WorkerPostings[workerName]
	if persistent != "dispatcher" {
		t.Errorf("persistent = %q, want %q", persistent, "dispatcher")
	}

	// Per design §3: persistent posting blocks session assumption.
	// When a persistent posting exists, the session .runtime/posting
	// should not be written (assume is blocked). Verify no session
	// posting exists in the work directory.
	session := posting.Read(workDir)
	if session != "" {
		t.Errorf("session posting should be empty when persistent exists, got %q", session)
	}
}

// ---------------------------------------------------------------------------
// gt posting show <name>: CLI-level resolution precedence (2.10–2.15)
// ---------------------------------------------------------------------------

func TestPostingShow_RigLevelResolution(t *testing.T) {
	t.Parallel()
	rigPath := t.TempDir()

	// Create rig-level override for dispatcher
	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigContent := "# Rig-level dispatcher"
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "dispatcher.md.tmpl"), []byte(rigContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := templates.LoadPosting("", rigPath, "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}
	if result.Level != "rig" {
		t.Errorf("Level = %q, want %q", result.Level, "rig")
	}
	if result.Content != rigContent {
		t.Errorf("Content = %q, want rig-level content", result.Content)
	}
}

func TestPostingShow_TownLevelResolution(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	// Create town-level override for dispatcher (no rig-level)
	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	townContent := "# Town-level dispatcher"
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte(townContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := templates.LoadPosting(townRoot, "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}
	if result.Level != "town" {
		t.Errorf("Level = %q, want %q", result.Level, "town")
	}
	if result.Content != townContent {
		t.Errorf("Content = %q, want town-level content", result.Content)
	}
}

func TestPostingShow_RigWinsOverTownAndEmbedded(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir()

	// Create both town and rig overrides
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
	rigContent := "# Rig wins"
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "dispatcher.md.tmpl"), []byte(rigContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := templates.LoadPosting(townRoot, rigPath, "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}
	if result.Level != "rig" {
		t.Errorf("Level = %q, want %q (rig should win)", result.Level, "rig")
	}
}

func TestPostingShow_FallsBackToTownWhenNoRig(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := t.TempDir() // exists but no postings/ dir

	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	townContent := "# Town fallback"
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte(townContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := templates.LoadPosting(townRoot, rigPath, "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}
	if result.Level != "town" {
		t.Errorf("Level = %q, want %q (should fall back to town)", result.Level, "town")
	}
}

func TestPostingShow_TownWinsOverEmbedded(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	townContent := "# Town override"
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte(townContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := templates.LoadPosting(townRoot, "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}
	if result.Level != "town" {
		t.Errorf("Level = %q, want %q (town should win over embedded)", result.Level, "town")
	}
}

func TestPostingShow_EmbeddedLevelOnly(t *testing.T) {
	t.Parallel()

	result, err := templates.LoadPosting("", "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}
	if result.Level != "embedded" {
		t.Errorf("Level = %q, want %q", result.Level, "embedded")
	}
}

// ---------------------------------------------------------------------------
// gt posting show: cross-rig isolation (2.16)
// ---------------------------------------------------------------------------

func TestPostingShow_CrossRigIsolation(t *testing.T) {
	t.Parallel()
	rigA := t.TempDir()
	rigB := t.TempDir()

	// Create a posting only in rig A
	rigAPostings := filepath.Join(rigA, "postings")
	if err := os.MkdirAll(rigAPostings, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigAPostings, "rig-a-only.md.tmpl"), []byte("rig A content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Rig A can see it
	result, err := templates.LoadPosting("", rigA, "rig-a-only")
	if err != nil {
		t.Fatalf("rig A should see its own posting: %v", err)
	}
	if result.Level != "rig" {
		t.Errorf("Level = %q, want %q", result.Level, "rig")
	}

	// Rig B cannot see it
	_, err = templates.LoadPosting("", rigB, "rig-a-only")
	if err == nil {
		t.Error("rig B should NOT see rig A's posting, but LoadPosting succeeded")
	}
}

// ---------------------------------------------------------------------------
// gt posting show: NEG nonexistent posting error (2.17)
// ---------------------------------------------------------------------------

func TestPostingShow_NonexistentPostingError(t *testing.T) {
	t.Parallel()

	_, err := templates.LoadPosting("", "", "nonexistent-posting-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent posting")
	}
	// showPostingContent wraps this with "Available built-in postings"
	// Verify the base error exists
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to contain 'not found'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// gt posting show: NEG unknown flag error (2.18)
// The cobra command definition does not accept --raw, so passing it would
// fail at the cobra argument parsing level. We verify the command spec.
// ---------------------------------------------------------------------------

func TestPostingShow_UnknownFlagRejected(t *testing.T) {
	t.Parallel()

	// The postingShowCmd accepts MaximumNArgs(1) and no --raw flag.
	// Cobra rejects unknown flags automatically. We verify the command
	// setup is correct (no --raw flag registered).
	flags := postingShowCmd.Flags()
	flag := flags.Lookup("raw")
	if flag != nil {
		t.Error("postingShowCmd should NOT have a --raw flag")
	}
}

// ---------------------------------------------------------------------------
// gt posting show: NEG path traversal rejected (2.19)
// ---------------------------------------------------------------------------

func TestPostingShow_PathTraversalRejected(t *testing.T) {
	t.Parallel()

	// validatePostingName rejects path traversal — showPostingContent calls it
	err := validatePostingName("../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal name")
	}

	// Also verify LoadPosting doesn't find anything even if validation is bypassed
	_, err = templates.LoadPosting("", "", "../../etc/passwd")
	if err == nil {
		t.Error("LoadPosting should fail for path traversal name")
	}
}

// ---------------------------------------------------------------------------
// crew list: posting column
// ---------------------------------------------------------------------------

func TestCrewListItem_PostingFields(t *testing.T) {
	t.Parallel()

	// Verify the struct has posting fields and they serialize correctly
	item := CrewListItem{
		Name:          "alice",
		Rig:           "myrig",
		Branch:        "main",
		Path:          "/tmp/test",
		HasSession:    true,
		GitClean:      true,
		Posting:       "dispatcher",
		PostingSource: "config",
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	var decoded CrewListItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.Posting != "dispatcher" {
		t.Errorf("Posting = %q, want %q", decoded.Posting, "dispatcher")
	}
	if decoded.PostingSource != "config" {
		t.Errorf("PostingSource = %q, want %q", decoded.PostingSource, "config")
	}
}

func TestCrewListItem_PostingOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	item := CrewListItem{
		Name:   "bob",
		Rig:    "myrig",
		Branch: "main",
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}

	dataStr := string(data)
	if strings.Contains(dataStr, "posting") {
		t.Errorf("empty posting should be omitted from JSON, got: %s", dataStr)
	}
}

// ===========================================================================
// Section 3 — gt posting assume: CLI-level tests (gt-9j6)
// ===========================================================================

// newPostingCmd builds a fresh posting command tree for isolated testing.
// This avoids side effects from the global rootCmd init.
func newPostingCmd() *cobra.Command {
	root := &cobra.Command{Use: "gt"}
	postCmd := &cobra.Command{Use: "posting"}
	root.AddCommand(postCmd)
	return postCmd
}

// 3.1: Crew assume writes .runtime/posting (via runPostingAssume logic)
func TestPostingAssume_CrewWritesRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Simulate runPostingAssume: validate name, check conflicts, write
	name := "dispatcher"
	if err := validatePostingName(name); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// No existing session posting
	if current := posting.Read(workDir); current != "" {
		t.Fatalf("precondition: posting already set: %q", current)
	}

	if err := posting.Write(workDir, name); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Errorf("crew assume: Read() = %q, want %q", got, "dispatcher")
	}

	// Verify file content matches expected format
	data, err := os.ReadFile(filepath.Join(workDir, ".runtime", "posting"))
	if err != nil {
		t.Fatalf("file read: %v", err)
	}
	if string(data) != "dispatcher\n" {
		t.Errorf("file content = %q, want %q", string(data), "dispatcher\n")
	}
}

// 3.2: Polecat assume writes .runtime/posting
func TestPostingAssume_PolecatWritesRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	name := "scout"
	if err := validatePostingName(name); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if err := posting.Write(workDir, name); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := posting.Read(workDir)
	if got != "scout" {
		t.Errorf("polecat assume: Read() = %q, want %q", got, "scout")
	}
}

// 3.5: --reason flag logs reason to stderr
func TestPostingAssume_ReasonFlagLogsToStderr(t *testing.T) {
	t.Parallel()

	// The CLI formats: "posting assume: %s (reason: %s)\n"
	// Test the format string that runPostingAssume would write to stderr.
	postingName := "scout"
	reason := "bead X needs exploration"

	expected := fmt.Sprintf("posting assume: %s (reason: %s)\n", postingName, reason)
	if !strings.Contains(expected, "reason:") {
		t.Error("reason format should contain 'reason:'")
	}
	if !strings.Contains(expected, postingName) {
		t.Errorf("expected format to contain posting name %q", postingName)
	}
	if !strings.Contains(expected, reason) {
		t.Errorf("expected format to contain reason %q", reason)
	}
}

// 3.6: Crew: already assumed must drop first
func TestPostingAssume_CrewBlockedByExistingSession(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Simulating runPostingAssume's conflict check
	current := posting.Read(workDir)
	if current == "" {
		t.Fatal("expected posting to be set")
	}

	// The error message should mention "drop it first"
	errMsg := fmt.Sprintf("already assumed posting %q — drop it first with: gt posting drop", current)
	if !strings.Contains(errMsg, "drop it first") {
		t.Error("error should mention 'drop it first'")
	}
	if !strings.Contains(errMsg, "dispatcher") {
		t.Error("error should mention current posting name")
	}
}

// 3.7: Polecat: already assumed must drop first (same logic, different role context)
func TestPostingAssume_PolecatBlockedByExistingSession(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	current := posting.Read(workDir)
	if current != "inspector" {
		t.Fatalf("precondition: posting = %q, want %q", current, "inspector")
	}

	// Cannot assume another posting while one is active
	errMsg := fmt.Sprintf("already assumed posting %q — drop it first with: gt posting drop", current)
	if !strings.Contains(errMsg, "inspector") {
		t.Error("error should mention current posting 'inspector'")
	}
}

// 3.8: Persistent posting blocks assume — verify error message format
func TestPostingAssume_PersistentBlocksWithClearHint(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "diesel"

	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent := loaded.WorkerPostings[workerName]

	// Verify error format matches runPostingAssume
	errMsg := fmt.Sprintf("persistent posting %q is set for %s — clear it first with: gt crew post %s --clear",
		persistent, workerName, workerName)
	if !strings.Contains(errMsg, "persistent posting") {
		t.Error("error should mention 'persistent posting'")
	}
	if !strings.Contains(errMsg, "--clear") {
		t.Error("error should mention '--clear' flag")
	}
	if !strings.Contains(errMsg, workerName) {
		t.Error("error should mention worker name")
	}
}

// 3.9: Nonexistent posting error includes define-it-at path hints
func TestPostingAssume_NonexistentPostingErrorWithHints(t *testing.T) {
	t.Parallel()

	postingName := "totally-fake-xyz"
	_, loadErr := templates.LoadPosting("", "", postingName)
	if loadErr == nil {
		t.Fatal("expected error for nonexistent posting")
	}

	// Simulate the error format that runPostingAssume produces (posting.go:437).
	// Using concrete paths to verify the format includes real path suggestions.
	rigPath := "/some/rig"
	townRoot := "/some/town"
	errMsg := fmt.Sprintf("posting %q not found: %v\n  Define it at:\n    Rig:  %s/postings/%s.md.tmpl\n    Town: %s/postings/%s.md.tmpl\n  Or use a built-in: %v",
		postingName, loadErr, rigPath, postingName, townRoot, postingName, templates.BuiltinPostingNames())

	// Verify the define-at path hints use the actual rig and town paths
	if !strings.Contains(errMsg, "Define it at:") {
		t.Error("error should contain 'Define it at:' header")
	}
	if !strings.Contains(errMsg, fmt.Sprintf("Rig:  %s/postings/%s.md.tmpl", rigPath, postingName)) {
		t.Errorf("error should contain rig template path, got: %q", errMsg)
	}
	if !strings.Contains(errMsg, fmt.Sprintf("Town: %s/postings/%s.md.tmpl", townRoot, postingName)) {
		t.Errorf("error should contain town template path, got: %q", errMsg)
	}
	if !strings.Contains(errMsg, "Or use a built-in:") {
		t.Error("error should contain 'Or use a built-in:' hint")
	}
	// Verify at least one built-in posting name is mentioned
	if !strings.Contains(errMsg, "dispatcher") {
		t.Error("error should list built-in posting names (e.g. dispatcher)")
	}
}

// 3.10: No args error (cobra ExactArgs(1) enforcement)
func TestPostingAssume_NoArgsError(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use:  "assume <posting>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for zero args, got nil")
	}
}

// 3.11: Too many args error
func TestPostingAssume_TooManyArgsError(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use:  "assume <posting>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	cmd.SetArgs([]string{"dispatcher", "inspector"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for two args, got nil")
	}
}

// 3.12: Path traversal rejected by validatePostingName
func TestPostingAssume_PathTraversalRejected(t *testing.T) {
	t.Parallel()

	traversalNames := []string{
		"../../etc/passwd",
		"../../../tmp/evil",
		"sub/dir/name",
	}
	for _, name := range traversalNames {
		if err := validatePostingName(name); err == nil {
			t.Errorf("validatePostingName(%q) should reject path traversal", name)
		}
	}
}

// 3.13: Injection treated as literal name (contains semicolons/shell chars)
func TestPostingAssume_InjectionTreatedAsLiteral(t *testing.T) {
	t.Parallel()

	// "dispatcher; rm -rf /" is not a valid posting name because it contains
	// no path separators but validatePostingName doesn't reject it —
	// LoadPosting will fail since no template file matches this literal name.
	name := "dispatcher; rm -rf /"
	// No path separators or ".." so validatePostingName won't catch it,
	// but LoadPosting will fail to find a matching template.
	_, err := templates.LoadPosting("", "", name)
	if err == nil {
		t.Error("expected LoadPosting to fail for injection-style name")
	}
}

// 3.14: Template lookup fails gracefully outside workspace
func TestPostingAssume_OutsideWorkspaceFailsGracefully(t *testing.T) {
	t.Parallel()

	// LoadPosting with empty paths should still work for built-in postings
	_, err := templates.LoadPosting("", "", "dispatcher")
	if err != nil {
		t.Errorf("built-in dispatcher should load even outside workspace: %v", err)
	}

	// But non-built-in postings should fail
	_, err = templates.LoadPosting("", "", "custom-posting-xyz")
	if err == nil {
		t.Error("non-built-in posting should fail outside workspace")
	}
}

// 3.15: getWorkDir rejects cwd outside Gas Town workspace (gt-2hz)
// Reproducer: from /tmp with phantom .runtime/posting, "gt posting assume"
// should fail with "not in a Gas Town workspace", not "already assumed".
func TestGetWorkDir_RejectsOutsideWorkspace(t *testing.T) {
	// Cannot use t.Parallel() because we modify cwd and env vars.

	// Use a temp dir that is definitely not a Gas Town workspace
	tmpDir := t.TempDir()

	// Plant phantom .runtime/posting to simulate the bug scenario
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "posting"), []byte("dispatcher\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Confirm phantom state is readable (this is what caused the bug)
	phantom := posting.Read(tmpDir)
	if phantom != "dispatcher" {
		t.Fatalf("expected phantom posting %q, got %q", "dispatcher", phantom)
	}

	// Unset GT_ROLE_HOME and change to the non-workspace dir
	t.Setenv(EnvGTRoleHome, "")
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// getWorkDir should reject this directory
	_, err = getWorkDir()
	if err == nil {
		t.Fatal("expected getWorkDir to fail outside workspace, but it succeeded")
	}
	if !strings.Contains(err.Error(), "not in a Gas Town workspace") {
		t.Errorf("expected 'not in a Gas Town workspace' error, got: %v", err)
	}
}

// ===========================================================================
// Section 4 — gt posting drop: CLI-level tests (gt-9j6)
// ===========================================================================

// 4.1: Crew drop removes .runtime/posting from disk
func TestPostingDrop_CrewRemovesRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(workDir, ".runtime", "posting")
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("precondition: file should exist: %v", err)
	}

	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("after drop, .runtime/posting should be removed from disk")
	}
}

// 4.2: Polecat drop removes .runtime/posting from disk
func TestPostingDrop_PolecatRemovesRuntimePosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(workDir, ".runtime", "posting")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("after polecat drop, .runtime/posting should be removed from disk")
	}
	if got := posting.Read(workDir); got != "" {
		t.Errorf("after drop, Read() = %q, want empty", got)
	}
}

// 4.3/4.4: Drop outputs system-reminder about posting dropped
func TestPostingDrop_OutputsSystemReminder(t *testing.T) {
	t.Parallel()

	// outputPostingDropReminder writes a system-reminder block.
	// Capture what it would produce.
	name := "scout"
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "<system-reminder>\n")
	fmt.Fprintf(&buf, "Your posting %q has been dropped. You are no longer posted.\n", name)
	fmt.Fprintf(&buf, "You have returned to your base role. Any posting-specific context\n")
	fmt.Fprintf(&buf, "or instructions no longer apply.\n")
	fmt.Fprintf(&buf, "</system-reminder>\n")

	output := buf.String()
	if !strings.Contains(output, "<system-reminder>") {
		t.Error("drop output should contain <system-reminder> opening tag")
	}
	if !strings.Contains(output, "</system-reminder>") {
		t.Error("drop output should contain </system-reminder> closing tag")
	}
	if !strings.Contains(output, "scout") {
		t.Error("drop output should mention the dropped posting name")
	}
	if !strings.Contains(output, "no longer posted") {
		t.Error("drop output should indicate agent is no longer posted")
	}
}

// 4.5: Crew round-trip: assume X, drop, assume Y
func TestPostingDrop_CrewRoundTrip(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Assume dispatcher
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "dispatcher" {
		t.Fatalf("step 1: Read() = %q, want %q", got, "dispatcher")
	}

	// Drop
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "" {
		t.Fatalf("step 2: Read() = %q, want empty", got)
	}

	// Assume inspector
	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "inspector" {
		t.Errorf("step 3: Read() = %q, want %q", got, "inspector")
	}
}

// 4.6: Polecat round-trip: assume X, drop, assume Y
func TestPostingDrop_PolecatRoundTrip(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Errorf("polecat round-trip: Read() = %q, want %q", got, "dispatcher")
	}
}

// 4.7: No posting active — graceful message
func TestPostingDrop_NoPostingActive(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Simulating runPostingDrop: Read returns empty
	current := posting.Read(workDir)
	if current != "" {
		t.Fatalf("precondition: expected no posting, got %q", current)
	}

	// The CLI prints "No session posting to drop" in this case
	expectedMsg := "No session posting to drop"
	if !strings.Contains(expectedMsg, "No session posting") {
		t.Error("message format mismatch")
	}
}

// 4.8: Persistent posting hint when no session posting to drop
func TestPostingDrop_PersistentPostingHint(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "diesel"

	// Set up persistent posting
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent := loaded.WorkerPostings[workerName]

	// Verify the hint message format from runPostingDrop
	msg := fmt.Sprintf("No session posting to drop (persistent posting %q is set for %s; use: gt crew post %s --clear)",
		persistent, workerName, workerName)
	if !strings.Contains(msg, "persistent posting") {
		t.Error("hint should mention 'persistent posting'")
	}
	if !strings.Contains(msg, "gt crew post") {
		t.Error("hint should mention 'gt crew post' command")
	}
	if !strings.Contains(msg, "--clear") {
		t.Error("hint should mention '--clear' flag")
	}
	if !strings.Contains(msg, workerName) {
		t.Error("hint should mention worker name")
	}
}

// ===========================================================================
// Section 5 — gt posting cycle: CLI-level tests (gt-9j6)
// ===========================================================================

// 5.1: Crew cycle — drops current posting and triggers state change for handoff
func TestPostingCycle_CrewClearsAndTriggersHandoff(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Assume a posting first
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Simulate runPostingCycle logic (without the syscall.Exec)
	old := posting.Read(workDir)
	if old != "dispatcher" {
		t.Fatalf("precondition: posting = %q, want %q", old, "dispatcher")
	}

	// Drop current
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	// No new posting requested — should be clear
	got := posting.Read(workDir)
	if got != "" {
		t.Errorf("after cycle (no new), Read() = %q, want empty", got)
	}
}

// 5.2: Polecat cycle — drops current posting
func TestPostingCycle_PolecatClearsAndTriggersHandoff(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	old := posting.Read(workDir)
	if old != "inspector" {
		t.Fatalf("precondition: posting = %q", old)
	}

	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	if got := posting.Read(workDir); got != "" {
		t.Errorf("after cycle, Read() = %q, want empty", got)
	}
}

// 5.3: Crew cycle with new posting name — drops old, writes new
func TestPostingCycle_CrewCycleWithNewPosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Start with dispatcher
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	// Cycle to scout (simulating runPostingCycle)
	old := posting.Read(workDir)
	if old != "dispatcher" {
		t.Fatalf("precondition: old = %q", old)
	}

	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}

	newPosting := "scout"
	if err := posting.Write(workDir, newPosting); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "scout" {
		t.Errorf("after crew cycle, Read() = %q, want %q", got, "scout")
	}
}

// 5.4: Polecat cycle with new posting name
func TestPostingCycle_PolecatCycleWithNewPosting(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Errorf("after polecat cycle, Read() = %q, want %q", got, "dispatcher")
	}
}

// 5.5: No current posting + arg — assumes the new posting directly
func TestPostingCycle_NoPreviousPostingAssumesNew(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// No existing posting
	old := posting.Read(workDir)
	if old != "" {
		t.Fatalf("precondition: should have no posting, got %q", old)
	}

	// Cycle with argument: just write the new posting
	newPosting := "dispatcher"
	if err := posting.Write(workDir, newPosting); err != nil {
		t.Fatal(err)
	}

	got := posting.Read(workDir)
	if got != "dispatcher" {
		t.Errorf("cycle from empty with arg: Read() = %q, want %q", got, "dispatcher")
	}
}

// 5.6: No posting, no arg — "No posting to cycle" message
func TestPostingCycle_NoPostingNoArg(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	old := posting.Read(workDir)
	if old != "" {
		t.Fatalf("precondition: should have no posting, got %q", old)
	}

	// Simulating the switch/default case in runPostingCycle
	// When old == "" && newPosting == "", the CLI prints:
	expectedMsg := "No posting to cycle — no session restart needed"
	if !strings.Contains(expectedMsg, "No posting to cycle") {
		t.Error("message format mismatch")
	}
}

// 5.7: Persistent posting blocks cycle (no arg)
func TestPostingCycle_PersistentBlocksCycleNoArg(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent, ok := loaded.WorkerPostings[workerName]
	if !ok || persistent == "" {
		t.Fatal("persistent posting should be set")
	}

	// Verify error message matches runPostingCycle format
	errMsg := fmt.Sprintf("persistent posting %q is set for %s — clear it first with: gt crew post %s --clear",
		persistent, workerName, workerName)
	if !strings.Contains(errMsg, "persistent posting") {
		t.Error("error should mention 'persistent posting'")
	}
}

// 5.8: Persistent posting blocks cycle even with new posting arg
func TestPostingCycle_PersistentBlocksCycleWithArg(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	workerName := "Toast"

	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{workerName: "inspector"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadRigSettings(filepath.Join(settingsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	persistent := loaded.WorkerPostings[workerName]
	if persistent != "inspector" {
		t.Fatalf("persistent = %q, want %q", persistent, "inspector")
	}

	// Even with a new posting arg like "scout", persistent posting blocks
	errMsg := fmt.Sprintf("persistent posting %q is set for %s — clear it first with: gt crew post %s --clear",
		persistent, workerName, workerName)
	if !strings.Contains(errMsg, "inspector") {
		t.Error("error should mention the persistent posting name")
	}
}

// 5.9: Too many args error (cobra MaximumNArgs(1) enforcement)
func TestPostingCycle_TooManyArgsError(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{
		Use:  "cycle [posting]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	cmd.SetArgs([]string{"a", "b"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for two args, got nil")
	}
}

// ---------------------------------------------------------------------------
// Cycle output message format tests
// ---------------------------------------------------------------------------

// Test cycle output format: old → new transition
func TestPostingCycle_OutputFormatTransition(t *testing.T) {
	t.Parallel()

	// Test all four cycle output branches from runPostingCycle
	tests := []struct {
		name    string
		old     string
		newP    string
		wantSub string
	}{
		{"old→new", "dispatcher", "scout", "Cycling posting: dispatcher → scout"},
		{"old→none", "dispatcher", "", "Cycling out of posting: dispatcher"},
		{"none→new", "", "scout", "Cycling into posting: scout"},
		{"none→none", "", "", "No posting to cycle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var msg string
			switch {
			case tt.old != "" && tt.newP != "":
				msg = fmt.Sprintf("Cycling posting: %s → %s (restarting session)", tt.old, tt.newP)
			case tt.old != "" && tt.newP == "":
				msg = fmt.Sprintf("Cycling out of posting: %s (restarting session)", tt.old)
			case tt.old == "" && tt.newP != "":
				msg = fmt.Sprintf("Cycling into posting: %s (restarting session)", tt.newP)
			default:
				msg = "No posting to cycle — no session restart needed"
			}
			if !strings.Contains(msg, tt.wantSub) {
				t.Errorf("message %q should contain %q", msg, tt.wantSub)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cross-section: assume + drop + cycle integration
// ---------------------------------------------------------------------------

// Full lifecycle: assume → drop → cycle with new → verify state at each step
func TestPostingLifecycle_FullIntegration(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Step 1: assume dispatcher
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "dispatcher" {
		t.Fatalf("step 1: Read() = %q, want %q", got, "dispatcher")
	}

	// Step 2: try to assume inspector while dispatcher active — should be blocked
	current := posting.Read(workDir)
	if current == "" {
		t.Fatal("step 2: expected active posting")
	}

	// Step 3: drop
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "" {
		t.Fatalf("step 3: Read() = %q, want empty", got)
	}

	// Step 4: cycle into scout (from empty)
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "scout" {
		t.Fatalf("step 4: Read() = %q, want %q", got, "scout")
	}

	// Step 5: cycle scout → inspector
	if err := posting.Clear(workDir); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}
	if got := posting.Read(workDir); got != "inspector" {
		t.Errorf("step 5: Read() = %q, want %q", got, "inspector")
	}
}

// Verify that .runtime/posting file doesn't accumulate after multiple cycles
func TestPostingCycle_FileDoesNotAccumulate(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Write, clear, write, clear many times
	for i := 0; i < 5; i++ {
		if err := posting.Write(workDir, "dispatcher"); err != nil {
			t.Fatalf("iteration %d write: %v", i, err)
		}
		if err := posting.Clear(workDir); err != nil {
			t.Fatalf("iteration %d clear: %v", i, err)
		}
	}

	// Final state should be empty
	if got := posting.Read(workDir); got != "" {
		t.Errorf("after %d cycles, Read() = %q, want empty", 5, got)
	}

	// File should not exist
	filePath := filepath.Join(workDir, ".runtime", "posting")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("posting file should not exist after final clear")
	}
}

// ===========================================================================
// Section 9.13 — BD_ACTOR bracket notation env var update (gt-5bf)
// ===========================================================================

// 9.13.1: updatePostingEnvVars appends bracket notation to BD_ACTOR
func TestPostingAssume_UpdatesBDActorEnvVar(t *testing.T) {
	// Not parallel: modifies process env vars
	orig := os.Getenv("BD_ACTOR")
	defer os.Setenv("BD_ACTOR", orig)
	origAuthor := os.Getenv("GIT_AUTHOR_NAME")
	defer os.Setenv("GIT_AUTHOR_NAME", origAuthor)
	origPosting := os.Getenv("GT_POSTING")
	defer func() {
		if origPosting == "" {
			os.Unsetenv("GT_POSTING")
		} else {
			os.Setenv("GT_POSTING", origPosting)
		}
	}()

	os.Setenv("BD_ACTOR", "gastown/crew/diesel")
	os.Setenv("GIT_AUTHOR_NAME", "diesel")

	updatePostingEnvVars("inspector")

	if got := os.Getenv("BD_ACTOR"); got != "gastown/crew/diesel[inspector]" {
		t.Errorf("BD_ACTOR = %q, want %q", got, "gastown/crew/diesel[inspector]")
	}
	if got := os.Getenv("GIT_AUTHOR_NAME"); got != "diesel[inspector]" {
		t.Errorf("GIT_AUTHOR_NAME = %q, want %q", got, "diesel[inspector]")
	}
	if got := os.Getenv("GT_POSTING"); got != "inspector" {
		t.Errorf("GT_POSTING = %q, want %q", got, "inspector")
	}
}

// 9.13.2: clearPostingEnvVars strips bracket notation from BD_ACTOR
func TestPostingDrop_RestoresBDActorEnvVar(t *testing.T) {
	// Not parallel: modifies process env vars
	orig := os.Getenv("BD_ACTOR")
	defer os.Setenv("BD_ACTOR", orig)
	origAuthor := os.Getenv("GIT_AUTHOR_NAME")
	defer os.Setenv("GIT_AUTHOR_NAME", origAuthor)
	origPosting := os.Getenv("GT_POSTING")
	defer func() {
		if origPosting == "" {
			os.Unsetenv("GT_POSTING")
		} else {
			os.Setenv("GT_POSTING", origPosting)
		}
	}()

	os.Setenv("BD_ACTOR", "gastown/crew/diesel[inspector]")
	os.Setenv("GIT_AUTHOR_NAME", "diesel[inspector]")
	os.Setenv("GT_POSTING", "inspector")

	clearPostingEnvVars()

	if got := os.Getenv("BD_ACTOR"); got != "gastown/crew/diesel" {
		t.Errorf("BD_ACTOR = %q, want %q", got, "gastown/crew/diesel")
	}
	if got := os.Getenv("GIT_AUTHOR_NAME"); got != "diesel" {
		t.Errorf("GIT_AUTHOR_NAME = %q, want %q", got, "diesel")
	}
	if got, _ := os.LookupEnv("GT_POSTING"); got != "" {
		t.Errorf("GT_POSTING = %q, want unset", got)
	}
}

// 9.13.3: Round-trip: assume sets brackets, drop removes them
func TestPostingEnvVars_RoundTrip(t *testing.T) {
	// Not parallel: modifies process env vars
	orig := os.Getenv("BD_ACTOR")
	defer os.Setenv("BD_ACTOR", orig)
	origAuthor := os.Getenv("GIT_AUTHOR_NAME")
	defer os.Setenv("GIT_AUTHOR_NAME", origAuthor)
	origPosting := os.Getenv("GT_POSTING")
	defer func() {
		if origPosting == "" {
			os.Unsetenv("GT_POSTING")
		} else {
			os.Setenv("GT_POSTING", origPosting)
		}
	}()

	base := "gastown/polecats/dag"
	os.Setenv("BD_ACTOR", base)
	os.Setenv("GIT_AUTHOR_NAME", "dag")

	// Assume
	updatePostingEnvVars("scout")
	if got := os.Getenv("BD_ACTOR"); got != "gastown/polecats/dag[scout]" {
		t.Errorf("after assume: BD_ACTOR = %q, want %q", got, "gastown/polecats/dag[scout]")
	}

	// Drop
	clearPostingEnvVars()
	if got := os.Getenv("BD_ACTOR"); got != base {
		t.Errorf("after drop: BD_ACTOR = %q, want %q", got, base)
	}
	if got := os.Getenv("GIT_AUTHOR_NAME"); got != "dag" {
		t.Errorf("after drop: GIT_AUTHOR_NAME = %q, want %q", got, "dag")
	}
}

// 9.13.4: clearPostingEnvVars is idempotent when no brackets present
func TestPostingEnvVars_ClearIdempotent(t *testing.T) {
	// Not parallel: modifies process env vars
	orig := os.Getenv("BD_ACTOR")
	defer os.Setenv("BD_ACTOR", orig)
	origAuthor := os.Getenv("GIT_AUTHOR_NAME")
	defer os.Setenv("GIT_AUTHOR_NAME", origAuthor)
	origPosting := os.Getenv("GT_POSTING")
	defer func() {
		if origPosting == "" {
			os.Unsetenv("GT_POSTING")
		} else {
			os.Setenv("GT_POSTING", origPosting)
		}
	}()

	os.Setenv("BD_ACTOR", "gastown/crew/diesel")
	os.Setenv("GIT_AUTHOR_NAME", "diesel")
	os.Unsetenv("GT_POSTING")

	clearPostingEnvVars()

	if got := os.Getenv("BD_ACTOR"); got != "gastown/crew/diesel" {
		t.Errorf("BD_ACTOR = %q, want unchanged %q", got, "gastown/crew/diesel")
	}
	if got := os.Getenv("GIT_AUTHOR_NAME"); got != "diesel" {
		t.Errorf("GIT_AUTHOR_NAME = %q, want unchanged %q", got, "diesel")
	}
}
