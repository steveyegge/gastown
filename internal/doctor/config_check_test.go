package doctor

import (
	"os"
	"path/filepath"
	"testing"
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
