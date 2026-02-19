package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputPersonaContext_CrewWithPersona(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create persona file
	personaDir := filepath.Join(rigPath, ".personas")
	if err := os.MkdirAll(personaDir, 0755); err != nil {
		t.Fatal(err)
	}
	personaContent := "# Alice\n\nYou are Alice, a frontend expert.\n"
	if err := os.WriteFile(
		filepath.Join(personaDir, "alice.md"),
		[]byte(personaContent), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// Create crew state dir with empty state
	crewDir := filepath.Join(rigPath, "crew", "alice")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		Role:     RoleCrew,
		Rig:      "test-rig",
		Polecat:  "alice",
		TownRoot: townRoot,
		WorkDir:  crewDir,
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputPersonaContext(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "## Persona") {
		t.Error("expected '## Persona' heading in output")
	}
	if !strings.Contains(output, "You are Alice, a frontend expert.") {
		t.Errorf("expected persona content in output, got: %s", output)
	}
}

func TestOutputPersonaContext_NonCrewSkipped(t *testing.T) {
	ctx := RoleContext{
		Role:     RolePolecat,
		Rig:      "test-rig",
		Polecat:  "alpha",
		TownRoot: t.TempDir(),
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputPersonaContext(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if buf.Len() > 0 {
		t.Errorf(
			"expected no output for non-crew role, got: %s",
			buf.String(),
		)
	}
}

func TestOutputPersonaContext_NoPersonaFile(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")
	crewDir := filepath.Join(rigPath, "crew", "bob")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		Role:     RoleCrew,
		Rig:      "test-rig",
		Polecat:  "bob",
		TownRoot: townRoot,
		WorkDir:  crewDir,
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputPersonaContext(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if buf.Len() > 0 {
		t.Errorf(
			"expected no output when no persona file, got: %s",
			buf.String(),
		)
	}
}

func TestOutputPersonaContext_ExplicitAssignment(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create persona file with different name
	personaDir := filepath.Join(rigPath, ".personas")
	if err := os.MkdirAll(personaDir, 0755); err != nil {
		t.Fatal(err)
	}
	personaContent := "# Rust Expert\n\nYou love Rust.\n"
	if err := os.WriteFile(
		filepath.Join(personaDir, "rust-expert.md"),
		[]byte(personaContent), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// Create crew state with explicit persona assignment
	crewDir := filepath.Join(rigPath, "crew", "alice")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatal(err)
	}
	stateJSON := `{"name":"alice","rig":"test-rig","persona":"rust-expert"}`
	if err := os.WriteFile(
		filepath.Join(crewDir, "state.json"),
		[]byte(stateJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		Role:     RoleCrew,
		Rig:      "test-rig",
		Polecat:  "alice",
		TownRoot: townRoot,
		WorkDir:  crewDir,
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputPersonaContext(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "You love Rust.") {
		t.Errorf("expected explicit persona content, got: %s", output)
	}
}

func TestExtractPersonaBrief(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			"heading then text",
			"# Toast\n\nYou are Toast.\n",
			"You are Toast.",
		},
		{"only heading", "# Toast\n", ""},
		{"empty", "", ""},
		{
			"no heading",
			"You are direct.\nYou skip pleasantries.\n",
			"You are direct.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPersonaBrief(tt.content)
			if got != tt.want {
				t.Errorf(
					"extractPersonaBrief() = %q, want %q",
					got, tt.want,
				)
			}
		})
	}
}

func TestResolvePersonaName_StablePath(t *testing.T) {
	townRoot := t.TempDir()
	rig := "test-rig"
	polecat := "alice"

	// Place state.json at the correct location (TownRoot/Rig/crew/Polecat)
	crewDir := filepath.Join(townRoot, rig, "crew", polecat)
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatal(err)
	}
	stateJSON := `{"name":"alice","rig":"test-rig","persona":"rust-expert"}`
	if err := os.WriteFile(
		filepath.Join(crewDir, "state.json"),
		[]byte(stateJSON), 0644,
	); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		Role:     RoleCrew,
		Rig:      rig,
		Polecat:  polecat,
		TownRoot: townRoot,
		// WorkDir intentionally set to a subdirectory to prove path stability
		WorkDir: filepath.Join(crewDir, "subdir"),
	}

	got := resolvePersonaName(ctx)
	if got != "rust-expert" {
		t.Errorf("resolvePersonaName() = %q, want %q", got, "rust-expert")
	}
}
