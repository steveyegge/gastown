package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/constants"
)

func TestSessionHookCheck_UsesSessionStartScript(t *testing.T) {
	check := NewSessionHookCheck()

	tests := []struct {
		name     string
		content  string
		hookType string
		want     bool
	}{
		{
			name:     "bare gt prime fails",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime"}]}]}}`,
			hookType: "SessionStart",
			want:     false,
		},
		{
			name:     "gt prime --hook passes",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hook"}]}]}}`,
			hookType: "SessionStart",
			want:     true,
		},
		{
			name:     "session-start.sh passes",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "bash ~/.claude/hooks/session-start.sh"}]}]}}`,
			hookType: "SessionStart",
			want:     true,
		},
		{
			name:     "no SessionStart hook passes",
			content:  `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "gt handoff"}]}]}}`,
			hookType: "SessionStart",
			want:     true,
		},
		{
			name:     "PreCompact with --hook passes",
			content:  `{"hooks": {"PreCompact": [{"hooks": [{"type": "command", "command": "gt prime --hook"}]}]}}`,
			hookType: "PreCompact",
			want:     true,
		},
		{
			name:     "PreCompact bare gt prime fails",
			content:  `{"hooks": {"PreCompact": [{"hooks": [{"type": "command", "command": "gt prime"}]}]}}`,
			hookType: "PreCompact",
			want:     false,
		},
		{
			name:     "gt prime --hook with extra flags passes",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hook --verbose"}]}]}}`,
			hookType: "SessionStart",
			want:     true,
		},
		{
			name:     "gt prime with --hook not first still passes",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --verbose --hook"}]}]}}`,
			hookType: "SessionStart",
			want:     true,
		},
		{
			name:     "gt prime with other flags but no --hook fails",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --verbose"}]}]}}`,
			hookType: "SessionStart",
			want:     false,
		},
		{
			name:     "both session-start.sh and gt prime passes (session-start.sh wins)",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "bash session-start.sh && gt prime"}]}]}}`,
			hookType: "SessionStart",
			want:     true,
		},
		{
			name:     "gt prime --hookup is NOT valid (false positive check)",
			content:  `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hookup"}]}]}}`,
			hookType: "SessionStart",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := check.usesSessionStartScript(tt.content, tt.hookType)
			if got != tt.want {
				t.Errorf("usesSessionStartScript() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionHookCheck_Run(t *testing.T) {
	t.Run("bare gt prime warns", func(t *testing.T) {
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			t.Fatal(err)
		}

		settings := `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime"}]}]}}`
		if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewSessionHookCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v", result.Status)
		}
	})

	t.Run("gt prime --hook passes", func(t *testing.T) {
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			t.Fatal(err)
		}

		settings := `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hook"}]}]}}`
		if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewSessionHookCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK, got %v: %v", result.Status, result.Details)
		}
	})

	t.Run("rig-level settings with --hook passes", func(t *testing.T) {
		tmpDir := t.TempDir()

		rigDir := filepath.Join(tmpDir, "myrig")
		if err := os.MkdirAll(filepath.Join(rigDir, "crew"), 0755); err != nil {
			t.Fatal(err)
		}
		claudeDir := filepath.Join(rigDir, ".claude")
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			t.Fatal(err)
		}

		settings := `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hook"}]}]}}`
		if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewSessionHookCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK for rig-level settings, got %v: %v", result.Status, result.Details)
		}
	})

	t.Run("rig-level bare gt prime warns", func(t *testing.T) {
		tmpDir := t.TempDir()

		rigDir := filepath.Join(tmpDir, "myrig")
		if err := os.MkdirAll(filepath.Join(rigDir, "polecats"), 0755); err != nil {
			t.Fatal(err)
		}
		claudeDir := filepath.Join(rigDir, ".claude")
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			t.Fatal(err)
		}

		settings := `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime"}]}]}}`
		if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewSessionHookCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning for rig-level bare gt prime, got %v", result.Status)
		}
	})

	t.Run("mixed valid and invalid hooks warns", func(t *testing.T) {
		tmpDir := t.TempDir()
		claudeDir := filepath.Join(tmpDir, ".claude")
		if err := os.MkdirAll(claudeDir, 0755); err != nil {
			t.Fatal(err)
		}

		settings := `{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "gt prime --hook"}]}], "PreCompact": [{"hooks": [{"type": "command", "command": "gt prime"}]}]}}`
		if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewSessionHookCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning when PreCompact is invalid, got %v", result.Status)
		}
		if len(result.Details) != 1 {
			t.Errorf("expected 1 issue (PreCompact), got %d: %v", len(result.Details), result.Details)
		}
	})

	t.Run("no settings files returns OK", func(t *testing.T) {
		tmpDir := t.TempDir()

		check := NewSessionHookCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK when no settings files, got %v", result.Status)
		}
	})
}

func TestParseConfigOutput(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
	}{
		{
			name:  "simple value",
			input: "agent,role,rig,convoy,slot\n",
			want:  "agent,role,rig,convoy,slot",
		},
		{
			name:  "value with trailing newlines",
			input: "agent,role,rig,convoy,slot\n\n",
			want:  "agent,role,rig,convoy,slot",
		},
		{
			name:  "Note prefix filtered",
			input: "Note: No git repository initialized - running without background sync\nagent,role,rig,convoy,slot\n",
			want:  "agent,role,rig,convoy,slot",
		},
		{
			name:  "multiple Note prefixes filtered",
			input: "Note: First note\nNote: Second note\nagent,role,rig,convoy,slot\n",
			want:  "agent,role,rig,convoy,slot",
		},
		{
			name:  "empty output",
			input: "",
			want:  "",
		},
		{
			name:  "only whitespace",
			input: "  \n  \n",
			want:  "",
		},
		{
			name:  "Note with different casing is not filtered",
			input: "note: lowercase should not match\n",
			want:  "note: lowercase should not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseConfigOutput([]byte(tt.input))
			if got != tt.want {
				t.Errorf("parseConfigOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCustomTypesCheck_ParsesOutputWithNotePrefix(t *testing.T) {
	// This test verifies that CustomTypesCheck correctly parses bd output
	// that contains "Note:" informational messages before the actual config value.
	// Without proper filtering, the check would see "Note: ..." as the config value
	// and incorrectly report all custom types as missing.

	// Test the parsing logic directly - this simulates bd outputting:
	// "Note: No git repository initialized - running without background sync"
	// followed by the actual config value
	output := "Note: No git repository initialized - running without background sync\n" + constants.BeadsCustomTypes + "\n"
	parsed := parseConfigOutput([]byte(output))

	if parsed != constants.BeadsCustomTypes {
		t.Errorf("parseConfigOutput failed to filter Note: prefix\ngot: %q\nwant: %q", parsed, constants.BeadsCustomTypes)
	}

	// Verify that all required types are found in the parsed output
	configuredSet := make(map[string]bool)
	for _, typ := range strings.Split(parsed, ",") {
		configuredSet[strings.TrimSpace(typ)] = true
	}

	var missing []string
	for _, required := range constants.BeadsCustomTypesList() {
		if !configuredSet[required] {
			missing = append(missing, required)
		}
	}

	if len(missing) > 0 {
		t.Errorf("After parsing, missing types: %v", missing)
	}
}
