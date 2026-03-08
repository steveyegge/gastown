package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
)

func TestRunGascityRoleValidate(t *testing.T) {
	config.ResetRegistryForTesting()
	t.Cleanup(config.ResetRegistryForTesting)

	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	townCfg := &config.TownConfig{
		Type:       "town",
		Version:    config.CurrentTownVersion,
		Name:       "test-town",
		PublicName: "Test Town",
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := config.SaveTownConfig(filepath.Join(townRoot, "mayor", "town.json"), townCfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "settings"), 0755); err != nil {
		t.Fatal(err)
	}

	// Seed a workspace-local registry so the validator can resolve a custom provider.
	registry := config.AgentRegistry{
		Version: config.CurrentAgentRegistryVersion,
		Agents: map[string]*config.AgentPresetInfo{
			"reviewbot": {
				Name:             "reviewbot",
				Command:          "reviewbot",
				Args:             []string{"--ci"},
				ProcessNames:     []string{"reviewbot"},
				ResumeFlag:       "--resume",
				ResumeStyle:      "flag",
				SupportsHooks:    true,
				ReadyDelayMs:     2500,
				InstructionsFile: "AGENTS.md",
			},
		},
	}
	data, err := json.Marshal(registry)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "settings", "agents.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(townRoot, "reviewer.toml")
	if err := os.WriteFile(specPath, []byte(`
version = 1
role = "reviewer"
scope = "rig"
provider = "reviewbot"

[session]
pattern = "{prefix}-reviewer-{name}"
work_dir = "{town}/{rig}/crew/{name}"
`), 0644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(townRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	if err := runGascityRoleValidate(cmd, []string{specPath}); err != nil {
		t.Fatalf("runGascityRoleValidate() error = %v", err)
	}

	if !strings.Contains(output.String(), "Valid Gas City role spec") {
		t.Fatalf("output = %q, want validation success message", output.String())
	}
	if !strings.Contains(output.String(), "Provider: reviewbot") {
		t.Fatalf("output = %q, want custom provider details", output.String())
	}
}

func TestRunGascityRoleValidateJSON(t *testing.T) {
	config.ResetRegistryForTesting()
	t.Cleanup(config.ResetRegistryForTesting)

	specPath := filepath.Join(t.TempDir(), "role.toml")
	if err := os.WriteFile(specPath, []byte(`
version = 1
role = "reviewer"
scope = "rig"
provider = "codex"

[session]
pattern = "{prefix}-reviewer-{name}"
work_dir = "{town}/{rig}/crew/{name}"
`), 0644); err != nil {
		t.Fatal(err)
	}

	prevJSON := gascityValidateJSON
	gascityValidateJSON = true
	t.Cleanup(func() { gascityValidateJSON = prevJSON })

	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	if err := runGascityRoleValidate(cmd, []string{specPath}); err != nil {
		t.Fatalf("runGascityRoleValidate() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output.String())
	}
	if decoded["provider"] != "codex" {
		t.Fatalf("provider = %v, want codex", decoded["provider"])
	}
}
