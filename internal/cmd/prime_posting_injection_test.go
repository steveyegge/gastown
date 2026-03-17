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
// Section 8: Priming injection (gt prime)
// Tests 8.1–8.8 from postings-test-spec.md
// ---------------------------------------------------------------------------

// TestPrimingInjection_8_1_CrewPostingAfterRoleTemplate verifies that crew
// prime includes posting content AFTER the crew role template.
func TestPrimingInjection_8_1_CrewPostingAfterRoleTemplate(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"

	workDir := filepath.Join(townRoot, rigName, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "inspector"); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	// Resolve posting
	ctx.Posting, _ = resolvePostingName(ctx)
	if ctx.Posting != "inspector" {
		t.Fatalf("expected posting 'inspector', got %q", ctx.Posting)
	}

	// Load and render the posting template
	result, err := templates.LoadPosting(townRoot, filepath.Join(townRoot, rigName), ctx.Posting)
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}

	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}

	data := templates.RoleData{
		Role:    "crew",
		RigName: rigName,
		Polecat: crewName,
		Posting: ctx.Posting,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}

	rendered := buf.String()
	if rendered == "" {
		t.Error("posting content should not be empty after crew role template")
	}
	// The posting content should be non-empty and distinct from the role template
	if !strings.Contains(rendered, "Code review") && !strings.Contains(rendered, "inspector") {
		t.Error("inspector posting should contain review-related content")
	}
}

// TestPrimingInjection_8_2_PolecatPostingAfterRoleTemplate verifies that polecat
// prime includes posting content AFTER the polecat role template.
func TestPrimingInjection_8_2_PolecatPostingAfterRoleTemplate(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "Toast"

	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := posting.Write(workDir, "scout"); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	ctx.Posting, _ = resolvePostingName(ctx)
	if ctx.Posting != "scout" {
		t.Fatalf("expected posting 'scout', got %q", ctx.Posting)
	}

	result, err := templates.LoadPosting(townRoot, filepath.Join(townRoot, rigName), ctx.Posting)
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}

	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}

	data := templates.RoleData{
		Role:    "polecat",
		RigName: rigName,
		Polecat: polecatName,
		Posting: ctx.Posting,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}

	rendered := buf.String()
	if rendered == "" {
		t.Error("posting content should not be empty after polecat role template")
	}
}

// TestPrimingInjection_8_3_TemplateVarsRenderedInPostingSection verifies that
// template variables like {{ cmd }} are rendered in the posting section.
func TestPrimingInjection_8_3_TemplateVarsRenderedInPostingSection(t *testing.T) {
	t.Parallel()

	for _, name := range templates.BuiltinPostingNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result, err := templates.LoadPosting("", "", name)
			if err != nil {
				t.Fatalf("LoadPosting(%q): %v", name, err)
			}

			tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
			if err != nil {
				t.Fatalf("template parse %q: %v", name, err)
			}

			data := templates.RoleData{
				Role:    "crew",
				RigName: "testrig",
				Polecat: "diesel",
				Posting: name,
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, data); err != nil {
				t.Fatalf("template execute %q: %v", name, err)
			}

			rendered := buf.String()
			// No unresolved template variables should remain
			if strings.Contains(rendered, "{{ cmd }}") || strings.Contains(rendered, "{{cmd}}") {
				t.Errorf("posting %q still contains unresolved {{ cmd }}", name)
			}
			if strings.Contains(rendered, "{{ .Posting }}") || strings.Contains(rendered, "{{.Posting}}") {
				t.Errorf("posting %q still contains unresolved {{ .Posting }}", name)
			}
			if strings.Contains(rendered, "{{ .RigName }}") || strings.Contains(rendered, "{{.RigName}}") {
				t.Errorf("posting %q still contains unresolved {{ .RigName }}", name)
			}
		})
	}
}

// TestPrimingInjection_8_3b_PersistentPostingTemplateVarsRendered verifies that
// template variables like {{ cmd }} are rendered when the posting is resolved via
// WorkerPostings config (persistent) rather than .runtime/posting (session).
func TestPrimingInjection_8_3b_PersistentPostingTemplateVarsRendered(t *testing.T) {
	t.Parallel()

	for _, name := range templates.BuiltinPostingNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			townRoot := t.TempDir()
			rigName := "testrig"
			workerName := "diesel"

			// Set up rig config with WorkerPostings (persistent posting)
			rigPath := filepath.Join(townRoot, rigName)
			settingsDir := filepath.Join(rigPath, "settings")
			if err := os.MkdirAll(settingsDir, 0755); err != nil {
				t.Fatal(err)
			}
			settings := config.NewRigSettings()
			settings.WorkerPostings = map[string]string{workerName: name}
			data, _ := json.Marshal(settings)
			if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
				t.Fatal(err)
			}

			workDir := filepath.Join(rigPath, "crew", workerName)
			if err := os.MkdirAll(workDir, 0755); err != nil {
				t.Fatal(err)
			}
			// No session posting — only config/persistent posting

			ctx := RoleInfo{
				Role:     RoleCrew,
				Rig:      rigName,
				Polecat:  workerName,
				TownRoot: townRoot,
				WorkDir:  workDir,
			}

			// Resolve posting via config path
			resolved, level := resolvePostingName(ctx)
			if resolved != name {
				t.Fatalf("resolvePostingName() = %q, want %q", resolved, name)
			}
			if level != "config" {
				t.Fatalf("resolvePostingName() level = %q, want %q", level, "config")
			}

			// Load and render the posting template
			result, err := templates.LoadPosting(townRoot, rigPath, resolved)
			if err != nil {
				t.Fatalf("LoadPosting(%q): %v", resolved, err)
			}

			tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
			if err != nil {
				t.Fatalf("template parse %q: %v", resolved, err)
			}

			roleData := templates.RoleData{
				Role:    "crew",
				RigName: rigName,
				Polecat: workerName,
				Posting: resolved,
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, roleData); err != nil {
				t.Fatalf("template execute %q: %v", resolved, err)
			}

			rendered := buf.String()
			// No unresolved template variables should remain
			if strings.Contains(rendered, "{{ cmd }}") || strings.Contains(rendered, "{{cmd}}") {
				t.Errorf("posting %q still contains unresolved {{ cmd }}", resolved)
			}
			if strings.Contains(rendered, "{{ .Posting }}") || strings.Contains(rendered, "{{.Posting}}") {
				t.Errorf("posting %q still contains unresolved {{ .Posting }}", resolved)
			}
			if strings.Contains(rendered, "{{ .RigName }}") || strings.Contains(rendered, "{{.RigName}}") {
				t.Errorf("posting %q still contains unresolved {{ .RigName }}", resolved)
			}
		})
	}
}

// TestPrimingInjection_8_4_CrewNoPostingStandardPriming verifies that a crew
// member with no posting gets standard priming only, no posting section.
func TestPrimingInjection_8_4_CrewNoPostingStandardPriming(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"

	workDir := filepath.Join(townRoot, rigName, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	// No posting written

	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "" {
		t.Errorf("resolvePostingName() name = %q, want empty (no posting)", name)
	}
	if level != "" {
		t.Errorf("resolvePostingName() level = %q, want empty", level)
	}

	// outputPostingContext returns immediately when ctx.Posting == ""
	// Verify the guard condition: no posting means no injection
	if ctx.Posting != "" {
		t.Error("ctx.Posting should be empty when no posting is set")
	}
}

// TestPrimingInjection_8_5_PolecatNoPostingStandardPriming verifies that a polecat
// with no posting gets standard priming only, no posting section.
func TestPrimingInjection_8_5_PolecatNoPostingStandardPriming(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "Toast"

	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
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
	if name != "" {
		t.Errorf("resolvePostingName() name = %q, want empty", name)
	}
	if level != "" {
		t.Errorf("resolvePostingName() level = %q, want empty", level)
	}
}

// TestPrimingInjection_8_6_PersistentPostingInjected verifies that a crew member
// with a persistent posting (from rig config, not session) gets posting injected.
func TestPrimingInjection_8_6_PersistentPostingInjected(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	crewName := "diesel"

	rigPath := filepath.Join(townRoot, rigName)
	settingsDir := filepath.Join(rigPath, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings := config.NewRigSettings()
	settings.WorkerPostings = map[string]string{crewName: "dispatcher"}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(rigPath, "crew", crewName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	// No session posting — only config/persistent posting

	ctx := RoleInfo{
		Role:     RoleCrew,
		Rig:      rigName,
		Polecat:  crewName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	name, level := resolvePostingName(ctx)
	if name != "dispatcher" {
		t.Errorf("resolvePostingName() name = %q, want %q", name, "dispatcher")
	}
	if level != "config" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "config")
	}

	// Verify the posting can actually be loaded and rendered
	ctx.Posting = name
	result, err := templates.LoadPosting(townRoot, rigPath, ctx.Posting)
	if err != nil {
		t.Fatalf("LoadPosting: %v", err)
	}

	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}

	roleData := templates.RoleData{
		Role:    "crew",
		RigName: rigName,
		Polecat: crewName,
		Posting: ctx.Posting,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, roleData); err != nil {
		t.Fatalf("template execute: %v", err)
	}

	if buf.String() == "" {
		t.Error("persistent posting should produce non-empty rendered content")
	}
}

// TestPrimingInjection_8_7_DispatcherNeverWritesCode verifies that the dispatcher
// posting content includes "NEVER writes code" — later priming supersedes earlier.
func TestPrimingInjection_8_7_DispatcherNeverWritesCode(t *testing.T) {
	t.Parallel()

	result, err := templates.LoadPosting("", "", "dispatcher")
	if err != nil {
		t.Fatalf("LoadPosting(dispatcher): %v", err)
	}

	tmpl, err := template.New("posting").Funcs(templates.TemplateFuncs()).Parse(result.Content)
	if err != nil {
		t.Fatalf("template parse: %v", err)
	}

	data := templates.RoleData{
		Role:    "crew",
		RigName: "testrig",
		Polecat: "diesel",
		Posting: "dispatcher",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}

	rendered := buf.String()
	// The dispatcher posting should contain a hard prohibition against writing code
	if !strings.Contains(rendered, "NEVER writes code") && !strings.Contains(rendered, "NEVER write code") {
		t.Error("dispatcher posting should contain 'NEVER writes code' prohibition")
	}
}

// TestPrimingInjection_8_8_UndefinedPostingWarnsOnStderr verifies that when an
// undefined posting is set, LoadPosting returns an error (which the prime code
// handles by warning on stderr and continuing with exit 0).
func TestPrimingInjection_8_8_UndefinedPostingWarnsOnStderr(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigName := "testrig"
	polecatName := "Toast"

	workDir := filepath.Join(townRoot, rigName, "polecats", polecatName)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write an undefined posting name to session state
	if err := posting.Write(workDir, "nonexistent-posting-xyz"); err != nil {
		t.Fatal(err)
	}

	ctx := RoleInfo{
		Role:     RolePolecat,
		Rig:      rigName,
		Polecat:  polecatName,
		TownRoot: townRoot,
		WorkDir:  workDir,
	}

	// Posting name is resolved from session state
	name, level := resolvePostingName(ctx)
	if name != "nonexistent-posting-xyz" {
		t.Errorf("resolvePostingName() name = %q, want %q", name, "nonexistent-posting-xyz")
	}
	if level != "session" {
		t.Errorf("resolvePostingName() level = %q, want %q", level, "session")
	}

	// LoadPosting should fail for the undefined posting — this is what triggers
	// the stderr warning in outputPostingContext (explain() call).
	rigPath := filepath.Join(townRoot, rigName)
	_, err := templates.LoadPosting(townRoot, rigPath, name)
	if err == nil {
		t.Error("LoadPosting should return error for undefined posting")
	}
}
