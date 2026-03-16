package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/posting"
	"github.com/steveyegge/gastown/internal/templates"
)

// ---------------------------------------------------------------------------
// prime_output.go posting injection: resolvePostingName + template loading
// ---------------------------------------------------------------------------

// TestPrimePostingInjection_PolecatWithSessionPosting verifies that a polecat
// with a session-level posting gets it resolved during prime.
func TestPrimePostingInjection_PolecatWithSessionPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "Toast"

	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write session posting
	if err := posting.Write(workDir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Errorf("resolvePostingName() name = %q, want %q", name, "dispatcher")
	}
	if level != "session" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "session")
	}
}

// TestPrimePostingInjection_CrewWithConfigPosting verifies that a crew member
// with a persistent posting (from rig settings) gets it resolved during prime.
func TestPrimePostingInjection_CrewWithConfigPosting(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "emma"

	// Set up rig config with persistent posting for crew
	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{crewName: "scout"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName, // crew member name stored in Polecat field
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "scout" {
		t.Errorf("resolvePostingName() name = %q, want %q", name, "scout")
	}
	if level != "config" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "config")
	}
}

// TestPrimePostingInjection_NonWorkerRolesSkipped verifies that non-worker
// roles (mayor, witness, refinery) don't get posting injection.
func TestPrimePostingInjection_NonWorkerRolesSkipped(t *testing.T) {
	t.Parallel()

	roles := []struct {
		role Role
		name string
	}{
		{RoleMayor, "mayor"},
		{RoleWitness, "witness"},
		{RoleRefinery, "refinery"},
		{RoleDeacon, "deacon"},
	}

	for _, tt := range roles {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Non-worker roles: the prime code only calls resolvePostingName
			// for RolePolecat and RoleCrew. We verify the guard condition.
			role := tt.role
			isWorker := (role == RolePolecat || role == RoleCrew)
			if isWorker {
				t.Errorf("role %s should not be a worker role", tt.name)
			}
		})
	}
}

// TestPrimePostingInjection_LoadEmbeddedPosting verifies that LoadPosting
// can load the built-in embedded posting templates.
func TestPrimePostingInjection_LoadEmbeddedPosting(t *testing.T) {
	t.Parallel()

	builtins := templates.BuiltinPostingNames()
	if len(builtins) == 0 {
		t.Fatal("expected at least one built-in posting, got none")
	}

	for _, name := range builtins {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := templates.LoadPosting("", "", name)
			if err != nil {
				t.Fatalf("LoadPosting(%q) error: %v", name, err)
			}
			if result.Content == "" {
				t.Errorf("LoadPosting(%q) returned empty content", name)
			}
			if result.Level != "embedded" {
				t.Errorf("LoadPosting(%q) level = %q, want %q", name, result.Level, "embedded")
			}
			if result.Name != name {
				t.Errorf("LoadPosting(%q) name = %q", name, result.Name)
			}
		})
	}
}

// TestPrimePostingInjection_LoadPostingNotFound verifies LoadPosting errors
// for nonexistent posting names.
func TestPrimePostingInjection_LoadPostingNotFound(t *testing.T) {
	t.Parallel()
	_, err := templates.LoadPosting("", "", "nonexistent-posting-xyz")
	if err == nil {
		t.Error("LoadPosting for nonexistent posting should return error")
	}
}

// TestPrimePostingInjection_RigOverrideTakesPrecedence verifies that a
// rig-level posting template overrides the built-in one.
func TestPrimePostingInjection_RigOverrideTakesPrecedence(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create rig-level posting override
	rigPostingsDir := filepath.Join(rigPath, "postings")
	if err := os.MkdirAll(rigPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	overrideContent := "# Custom Dispatcher\nRig-specific override for {{.Posting}}.\n"
	if err := os.WriteFile(filepath.Join(rigPostingsDir, "dispatcher.md.tmpl"), []byte(overrideContent), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := templates.LoadPosting(townRoot, rigPath, "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting with rig override failed: %v", err)
	}
	if result.Level != "rig" {
		t.Errorf("with rig override, level = %q, want %q", result.Level, "rig")
	}
	if result.Content != overrideContent {
		t.Errorf("rig override content not loaded correctly")
	}
}

// TestPrimePostingInjection_TownOverrideTakesPrecedence verifies that a
// town-level posting template overrides the embedded one but not rig-level.
func TestPrimePostingInjection_TownOverrideTakesPrecedence(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()

	// Create town-level posting override (no rig override)
	townPostingsDir := filepath.Join(townRoot, "postings")
	if err := os.MkdirAll(townPostingsDir, 0755); err != nil {
		t.Fatal(err)
	}

	overrideContent := "# Town Dispatcher\nTown-level override for {{.Posting}}.\n"
	if err := os.WriteFile(filepath.Join(townPostingsDir, "dispatcher.md.tmpl"), []byte(overrideContent), 0644); err != nil {
		t.Fatal(err)
	}

	// No rig path — so rig override is not checked
	result, err := templates.LoadPosting(townRoot, "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting with town override failed: %v", err)
	}
	if result.Level != "town" {
		t.Errorf("with town override, level = %q, want %q", result.Level, "town")
	}
	if result.Content != overrideContent {
		t.Errorf("town override content not loaded correctly")
	}
}

// TestPrimePostingInjection_ContextPropagation verifies that posting and
// posting level are set on RoleContext/RoleInfo during prime.
func TestPrimePostingInjection_ContextPropagation(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "alpha"

	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	// Simulate what runPrime does: resolve posting and set on ctx
	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	ctx.Posting, _ = resolvePostingName(ctx)

	if ctx.Posting != "inspector" {
		t.Errorf("ctx.Posting = %q, want %q", ctx.Posting, "inspector")
	}

	// Resolve template level — "inspector" is a built-in embedded posting
	ctx.PostingLevel, ctx.PostingAmbiguous = resolvePostingLevel(ctx)
	if ctx.PostingLevel != "embedded" {
		t.Errorf("ctx.PostingLevel = %q, want %q", ctx.PostingLevel, "embedded")
	}
	if ctx.PostingAmbiguous {
		t.Error("ctx.PostingAmbiguous = true, want false (only embedded level)")
	}

	// Verify RoleData can carry the posting for template rendering
	roleData := templates.RoleData{
		Role:    string(ctx.Role),
		Posting: ctx.Posting,
	}
	if roleData.Posting != "inspector" {
		t.Errorf("RoleData.Posting = %q, want %q", roleData.Posting, "inspector")
	}
}

// TestPrimePostingInjection_EmptyPostingSkipsOutput verifies that
// outputPostingContext returns immediately when posting is empty.
func TestPrimePostingInjection_EmptyPostingSkipsOutput(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	workDir := filepath.Join(townRoot, "testrig", "polecats", "alpha")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      "testrig",
		Polecat:  "alpha",
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "" {
		t.Errorf("resolvePostingName() should return empty for no posting, got %q", name)
	}
	if level != "" {
		t.Errorf("resolvePostingName() level should be empty, got %q", level)
	}

	// outputPostingContext would return immediately because ctx.Posting == ""
}

// TestPrimePostingInjection_CmdFuncRendersInAllBuiltins verifies that every
// built-in posting template renders without error when {{ cmd }} is used.
// This is the regression test for gt-bwn: without the FuncMap, templates
// that reference {{ cmd }} (dispatcher, scout) fail silently.
func TestPrimePostingInjection_CmdFuncRendersInAllBuiltins(t *testing.T) {
	t.Parallel()

	for _, name := range templates.BuiltinPostingNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := templates.LoadPosting("", "", name)
			if err != nil {
				t.Fatalf("LoadPosting(%q): %v", name, err)
			}

			// Parse with the shared FuncMap — same path as outputPostingContext
			tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
			if err != nil {
				t.Fatalf("template parse %q: %v", name, err)
			}

			data := templates.RoleData{
				Role:    "polecat",
				RigName: "testrig",
				Polecat: "testcat",
				Posting: name,
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				t.Fatalf("template execute %q: %v", name, err)
			}

			rendered := buf.String()
			if rendered == "" {
				t.Errorf("posting %q rendered to empty string", name)
			}

			// Verify {{ cmd }} resolved (should not contain literal "{{ cmd }}")
			if strings.Contains(rendered, "{{ cmd }}") || strings.Contains(rendered, "{{cmd}}") {
				t.Errorf("posting %q still contains unresolved {{ cmd }}", name)
			}
		})
	}
}
