package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputIdentityContext_CrewWithIdentity(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create identity file
	idDir := filepath.Join(rigPath, "identities")
	if err := os.MkdirAll(idDir, 0755); err != nil {
		t.Fatal(err)
	}
	idContent := "# Alice\n\nYou are Alice, a frontend expert.\n"
	if err := os.WriteFile(
		filepath.Join(idDir, "alice.md"),
		[]byte(idContent), 0644,
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

	outputIdentityContext(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "## Identity") {
		t.Error("expected '## Identity' heading in output")
	}
	if !strings.Contains(output, "You are Alice, a frontend expert.") {
		t.Errorf("expected identity content in output, got: %s", output)
	}
}

func TestOutputIdentityContext_NonCrewSkipped(t *testing.T) {
	ctx := RoleContext{
		Role:     RolePolecat,
		Rig:      "test-rig",
		Polecat:  "alpha",
		TownRoot: t.TempDir(),
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputIdentityContext(ctx)

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

func TestOutputIdentityContext_NoIdentityFile(t *testing.T) {
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

	outputIdentityContext(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if buf.Len() > 0 {
		t.Errorf(
			"expected no output when no identity file, got: %s",
			buf.String(),
		)
	}
}

func TestOutputIdentityContext_ExplicitAssignment(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "test-rig")

	// Create identity file with different name
	idDir := filepath.Join(rigPath, "identities")
	if err := os.MkdirAll(idDir, 0755); err != nil {
		t.Fatal(err)
	}
	idContent := "# Rust Expert\n\nYou love Rust.\n"
	if err := os.WriteFile(
		filepath.Join(idDir, "rust-expert.md"),
		[]byte(idContent), 0644,
	); err != nil {
		t.Fatal(err)
	}

	// Create crew state with explicit identity
	crewDir := filepath.Join(rigPath, "crew", "alice")
	if err := os.MkdirAll(crewDir, 0755); err != nil {
		t.Fatal(err)
	}
	stateJSON := `{"name":"alice","rig":"test-rig","identity":"rust-expert"}`
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

	outputIdentityContext(ctx)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "You love Rust.") {
		t.Errorf("expected explicit identity content, got: %s", output)
	}
}

func TestExtractIdentityBrief(t *testing.T) {
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
			got := extractIdentityBrief(tt.content)
			if got != tt.want {
				t.Errorf(
					"extractIdentityBrief() = %q, want %q",
					got, tt.want,
				)
			}
		})
	}
}
