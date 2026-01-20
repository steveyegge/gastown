//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/testutil"
)

func TestTownFixtureCreation(t *testing.T) {
	testutil.RequireGT(t)

	agents := []string{"opencode", "claude", "codex"}

	for _, agent := range agents {
		t.Run(agent, func(t *testing.T) {
			fixture := testutil.NewTownFixture(t, agent)

			requiredPaths := []string{
				"mayor",
				"deacon",
				"settings",
				"mayor/town.json",
				"settings/config.json",
			}

			for _, p := range requiredPaths {
				full := fixture.Path(p)
				if _, err := os.Stat(full); err != nil {
					t.Errorf("Required path should exist: %s", p)
				}
			}

			settingsPath := fixture.Path("settings/config.json")
			content, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatalf("Failed to read settings: %v", err)
			}
			if len(content) < 10 {
				t.Error("Settings file should have content")
			}

			t.Logf("Town fixture created for %s", agent)
		})
	}
}

func TestRuntimeSettings(t *testing.T) {
	testutil.RequireGT(t)

	tests := []struct {
		agent      string
		configDir  string
		configFile string
	}{
		{"opencode", ".opencode/plugin", "gastown.js"},
		{"claude", ".claude", "settings.json"},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			testutil.RequireBinary(t, tt.agent)

			fixture := testutil.NewTownFixture(t, tt.agent)

			configPath := filepath.Join(fixture.Root, "mayor", tt.configDir, tt.configFile)
			if _, err := os.Stat(configPath); err != nil {
				t.Errorf("Runtime config should exist at %s: %v", configPath, err)
			}

			t.Logf("Runtime settings verified for %s", tt.agent)
		})
	}
}
