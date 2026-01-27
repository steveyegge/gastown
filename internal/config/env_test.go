package config

import (
	"testing"
)

func TestAgentEnv_Mayor(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:     "mayor",
		TownRoot: "/town",
	})

	assertEnv(t, env, "GT_ROLE", "mayor")
	assertEnv(t, env, "BD_ACTOR", "mayor")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "mayor")
	assertEnv(t, env, "GT_ROOT", "/town")
	assertNotSet(t, env, "GT_RIG")
	assertNotSet(t, env, "BEADS_NO_DAEMON")
}

func TestAgentEnv_Witness(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:     "witness",
		Rig:      "myrig",
		TownRoot: "/town",
	})

	assertEnv(t, env, "GT_ROLE", "myrig/witness") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "BD_ACTOR", "myrig/witness")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "myrig/witness")
	assertEnv(t, env, "GT_ROOT", "/town")
}

func TestAgentEnv_Polecat(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:          "polecat",
		Rig:           "myrig",
		AgentName:     "Toast",
		TownRoot:      "/town",
		BeadsNoDaemon: true,
	})

	assertEnv(t, env, "GT_ROLE", "myrig/polecats/Toast") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "GT_POLECAT", "Toast")
	assertEnv(t, env, "BD_ACTOR", "myrig/polecats/Toast")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "Toast")
	assertEnv(t, env, "BEADS_AGENT_NAME", "myrig/Toast")
	assertEnv(t, env, "BEADS_NO_DAEMON", "1")
	assertEnv(t, env, "BEADS_AUTO_START_DAEMON", "false")
}

func TestAgentEnv_Crew(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:          "crew",
		Rig:           "myrig",
		AgentName:     "emma",
		TownRoot:      "/town",
		BeadsNoDaemon: true,
	})

	assertEnv(t, env, "GT_ROLE", "myrig/crew/emma") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "GT_CREW", "emma")
	assertEnv(t, env, "BD_ACTOR", "myrig/crew/emma")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "emma")
	assertEnv(t, env, "BEADS_AGENT_NAME", "myrig/emma")
	assertEnv(t, env, "BEADS_NO_DAEMON", "1")
	assertEnv(t, env, "BEADS_AUTO_START_DAEMON", "false")
}

func TestAgentEnv_Refinery(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:          "refinery",
		Rig:           "myrig",
		TownRoot:      "/town",
		BeadsNoDaemon: true,
	})

	assertEnv(t, env, "GT_ROLE", "myrig/refinery") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "BD_ACTOR", "myrig/refinery")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "myrig/refinery")
	assertEnv(t, env, "BEADS_NO_DAEMON", "1")
	assertEnv(t, env, "BEADS_AUTO_START_DAEMON", "false")
}

func TestAgentEnv_Deacon(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:     "deacon",
		TownRoot: "/town",
	})

	assertEnv(t, env, "GT_ROLE", "deacon")
	assertEnv(t, env, "BD_ACTOR", "deacon")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "deacon")
	assertEnv(t, env, "GT_ROOT", "/town")
	assertNotSet(t, env, "GT_RIG")
	assertNotSet(t, env, "BEADS_NO_DAEMON")
}

func TestAgentEnv_Boot(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:     "boot",
		TownRoot: "/town",
	})

	assertEnv(t, env, "GT_ROLE", "deacon/boot") // compound format
	assertEnv(t, env, "BD_ACTOR", "deacon-boot")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "boot")
	assertEnv(t, env, "GT_ROOT", "/town")
	assertNotSet(t, env, "GT_RIG")
	assertNotSet(t, env, "BEADS_NO_DAEMON")
}

func TestAgentEnv_WithRuntimeConfigDir(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:             "polecat",
		Rig:              "myrig",
		AgentName:        "Toast",
		TownRoot:         "/town",
		RuntimeConfigDir: "/home/user/.config/claude",
	})

	assertEnv(t, env, "CLAUDE_CONFIG_DIR", "/home/user/.config/claude")
}

func TestAgentEnv_WithoutRuntimeConfigDir(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "/town",
	})

	assertNotSet(t, env, "CLAUDE_CONFIG_DIR")
}

func TestAgentEnvSimple(t *testing.T) {
	t.Parallel()
	env := AgentEnvSimple("polecat", "myrig", "Toast")

	assertEnv(t, env, "GT_ROLE", "myrig/polecats/Toast") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "GT_POLECAT", "Toast")
	// Simple doesn't set TownRoot, so key should be absent
	// (not empty string which would override tmux session environment)
	assertNotSet(t, env, "GT_ROOT")
}

func TestAgentEnv_EmptyTownRootOmitted(t *testing.T) {
	t.Parallel()
	// Regression test: empty TownRoot should NOT create keys in the map.
	// If it was set to empty string, ExportPrefix would generate "export GT_ROOT= ..."
	// which overrides tmux session environment where it's correctly set.
	env := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "", // explicitly empty
	})

	// Key should be absent, not empty string
	assertNotSet(t, env, "GT_ROOT")

	// Other keys should still be set
	assertEnv(t, env, "GT_ROLE", "myrig/polecats/Toast") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
}

func TestShellQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple value no quoting",
			input:    "foobar",
			expected: "foobar",
		},
		{
			name:     "alphanumeric and underscore",
			input:    "FOO_BAR_123",
			expected: "FOO_BAR_123",
		},
		// CRITICAL: These values are used by existing agents and must NOT be quoted
		{
			name:     "path with slashes (GT_ROOT, CLAUDE_CONFIG_DIR)",
			input:    "/home/user/.config/claude",
			expected: "/home/user/.config/claude", // NOT quoted
		},
		{
			name:     "BD_ACTOR with slashes",
			input:    "myrig/polecats/Toast",
			expected: "myrig/polecats/Toast", // NOT quoted
		},
		{
			name:     "value with hyphen",
			input:    "deacon-boot",
			expected: "deacon-boot", // NOT quoted
		},
		{
			name:     "value with dots",
			input:    "user.name",
			expected: "user.name", // NOT quoted
		},
		{
			name:     "value with spaces",
			input:    "hello world",
			expected: "'hello world'",
		},
		{
			name:     "value with double quotes",
			input:    `say "hello"`,
			expected: `'say "hello"'`,
		},
		{
			name:     "JSON object",
			input:    `{"*":"allow"}`,
			expected: `'{"*":"allow"}'`,
		},
		{
			name:     "OPENCODE_PERMISSION value",
			input:    `{"*":"allow"}`,
			expected: `'{"*":"allow"}'`,
		},
		{
			name:     "value with single quote",
			input:    "it's a test",
			expected: `'it'\''s a test'`,
		},
		{
			name:     "value with dollar sign",
			input:    "$HOME",
			expected: "'$HOME'",
		},
		{
			name:     "value with backticks",
			input:    "`whoami`",
			expected: "'`whoami`'",
		},
		{
			name:     "value with asterisk",
			input:    "*.txt",
			expected: "'*.txt'",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShellQuote(tt.input)
			if result != tt.expected {
				t.Errorf("ShellQuote(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExportPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		env      map[string]string
		expected string
	}{
		{
			name:     "empty",
			env:      map[string]string{},
			expected: "",
		},
		{
			name:     "single var",
			env:      map[string]string{"FOO": "bar"},
			expected: "export FOO=bar && ",
		},
		{
			name: "multiple vars sorted",
			env: map[string]string{
				"ZZZ": "last",
				"AAA": "first",
				"MMM": "middle",
			},
			expected: "export AAA=first MMM=middle ZZZ=last && ",
		},
		{
			name: "JSON value is quoted",
			env: map[string]string{
				"OPENCODE_PERMISSION": `{"*":"allow"}`,
			},
			expected: `export OPENCODE_PERMISSION='{"*":"allow"}' && `,
		},
		{
			name: "mixed simple and complex values",
			env: map[string]string{
				"SIMPLE":  "value",
				"COMPLEX": `{"key":"val"}`,
				"GT_ROLE": "polecat",
			},
			expected: `export COMPLEX='{"key":"val"}' GT_ROLE=polecat SIMPLE=value && `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExportPrefix(tt.env)
			if result != tt.expected {
				t.Errorf("ExportPrefix() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBuildStartupCommandWithEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		env      map[string]string
		agentCmd string
		prompt   string
		expected string
	}{
		{
			name:     "no env no prompt",
			env:      map[string]string{},
			agentCmd: "claude",
			prompt:   "",
			expected: "claude",
		},
		{
			name:     "env no prompt",
			env:      map[string]string{"GT_ROLE": "polecat"},
			agentCmd: "claude",
			prompt:   "",
			expected: "export GT_ROLE=polecat && claude",
		},
		{
			name:     "env with prompt",
			env:      map[string]string{"GT_ROLE": "polecat"},
			agentCmd: "claude",
			prompt:   "gt prime",
			expected: `export GT_ROLE=polecat && claude "gt prime"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildStartupCommandWithEnv(tt.env, tt.agentCmd, tt.prompt)
			if result != tt.expected {
				t.Errorf("BuildStartupCommandWithEnv() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMergeEnv(t *testing.T) {
	t.Parallel()
	a := map[string]string{"A": "1", "B": "2"}
	b := map[string]string{"B": "override", "C": "3"}

	result := MergeEnv(a, b)

	assertEnv(t, result, "A", "1")
	assertEnv(t, result, "B", "override")
	assertEnv(t, result, "C", "3")
}

func TestFilterEnv(t *testing.T) {
	t.Parallel()
	env := map[string]string{"A": "1", "B": "2", "C": "3"}

	result := FilterEnv(env, "A", "C")

	assertEnv(t, result, "A", "1")
	assertNotSet(t, result, "B")
	assertEnv(t, result, "C", "3")
}

func TestWithoutEnv(t *testing.T) {
	t.Parallel()
	env := map[string]string{"A": "1", "B": "2", "C": "3"}

	result := WithoutEnv(env, "B")

	assertEnv(t, result, "A", "1")
	assertNotSet(t, result, "B")
	assertEnv(t, result, "C", "3")
}

func TestEnvToSlice(t *testing.T) {
	t.Parallel()
	env := map[string]string{"A": "1", "B": "2"}

	result := EnvToSlice(env)

	if len(result) != 2 {
		t.Errorf("EnvToSlice() returned %d items, want 2", len(result))
	}

	// Check both entries exist (order not guaranteed)
	found := make(map[string]bool)
	for _, s := range result {
		found[s] = true
	}
	if !found["A=1"] || !found["B=2"] {
		t.Errorf("EnvToSlice() = %v, want [A=1, B=2]", result)
	}
}

// Helper functions

func assertEnv(t *testing.T, env map[string]string, key, expected string) {
	t.Helper()
	if got := env[key]; got != expected {
		t.Errorf("env[%q] = %q, want %q", key, got, expected)
	}
}

func assertNotSet(t *testing.T, env map[string]string, key string) {
	t.Helper()
	if _, ok := env[key]; ok {
		t.Errorf("env[%q] should not be set, but is %q", key, env[key])
	}
}
