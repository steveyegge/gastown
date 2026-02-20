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
	assertEnv(t, env, "GIT_CEILING_DIRECTORIES", "/town") // prevents git walking to umbrella
	assertEnv(t, env, "NODE_OPTIONS", "")                  // cleared to prevent debugger inheritance
	assertEnv(t, env, "CLAUDECODE", "")                    // cleared to prevent nested session detection
	assertNotSet(t, env, "GT_RIG")
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
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "/town",
	})

	assertEnv(t, env, "GT_ROLE", "myrig/polecats/Toast") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "GT_POLECAT", "Toast")
	assertEnv(t, env, "BD_ACTOR", "myrig/polecats/Toast")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "Toast")
	assertEnv(t, env, "BEADS_AGENT_NAME", "myrig/Toast")
	assertEnv(t, env, "BD_DOLT_AUTO_COMMIT", "off") // gt-5cc2p: prevent manifest contention
	assertEnv(t, env, "NODE_OPTIONS", "")            // cleared to prevent debugger inheritance
	assertEnv(t, env, "CLAUDECODE", "")              // cleared to prevent nested session detection
}

func TestAgentEnv_Crew(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:      "crew",
		Rig:       "myrig",
		AgentName: "emma",
		TownRoot:  "/town",
	})

	assertEnv(t, env, "GT_ROLE", "myrig/crew/emma") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "GT_CREW", "emma")
	assertEnv(t, env, "BD_ACTOR", "myrig/crew/emma")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "emma")
	assertEnv(t, env, "BEADS_AGENT_NAME", "myrig/emma")
}

func TestAgentEnv_Refinery(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:     "refinery",
		Rig:      "myrig",
		TownRoot: "/town",
	})

	assertEnv(t, env, "GT_ROLE", "myrig/refinery") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
	assertEnv(t, env, "BD_ACTOR", "myrig/refinery")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "myrig/refinery")
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
	assertNotSet(t, env, "GIT_CEILING_DIRECTORIES") // also not set when TownRoot empty

	// Other keys should still be set
	assertEnv(t, env, "GT_ROLE", "myrig/polecats/Toast") // compound format
	assertEnv(t, env, "GT_RIG", "myrig")
}

func TestAgentEnv_WithAgentOverride(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "/town",
		Agent:     "codex",
	})

	assertEnv(t, env, "GT_AGENT", "codex")
}

func TestAgentEnv_WithoutAgentOverride(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "/town",
	})

	assertNotSet(t, env, "GT_AGENT")
}

// TestAgentEnv_WithoutAgentOverride_UsesResolvedAgent verifies that AgentEnv
// uses ResolvedAgent as fallback when Agent (explicit --agent override) is empty.
// This centralizes GT_AGENT resolution in AgentEnv, eliminating the need for
// per-caller fallback logic (previously in session_manager.go, daemon.go).
// Fix for gt-nwgd5: non-polecat roles had GT_AGENT missing from tmux session table.
func TestAgentEnv_WithoutAgentOverride_UsesResolvedAgent(t *testing.T) {
	t.Parallel()

	// Simulate the default polecat dispatch path (no --agent flag)
	// with a resolved agent from config.
	env := AgentEnv(AgentEnvConfig{
		Role:          "polecat",
		Rig:           "myrig",
		AgentName:     "Toast",
		TownRoot:      "/town",
		Agent:         "",       // no explicit override
		ResolvedAgent: "codex",  // resolved from rig/town config
	})

	// GT_AGENT IS set from ResolvedAgent — no caller fallback needed.
	assertEnv(t, env, "GT_AGENT", "codex")

	// Without ResolvedAgent, GT_AGENT is absent (Claude default).
	envNoResolved := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "/town",
		Agent:     "",
	})
	assertNotSet(t, envNoResolved, "GT_AGENT")

	// With an explicit override, GT_AGENT uses the override (not ResolvedAgent).
	envWithOverride := AgentEnv(AgentEnvConfig{
		Role:          "polecat",
		Rig:           "myrig",
		AgentName:     "Toast",
		TownRoot:      "/town",
		Agent:         "gemini",
		ResolvedAgent: "codex",
	})
	assertEnv(t, envWithOverride, "GT_AGENT", "gemini")
}

// TestAgentEnv_AgentOverrideAllRoles verifies that GT_AGENT is emitted for
// every role that supports agent overrides. This mirrors the actual
// AgentEnvConfig constructions in each manager's Start method.
func TestAgentEnv_AgentOverrideAllRoles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  AgentEnvConfig
	}{
		{
			name: "polecat via session_manager",
			cfg: AgentEnvConfig{
				Role:      "polecat",
				Rig:       "rig1",
				AgentName: "Toast",
				TownRoot:  "/town",
				Agent:     "codex",
			},
		},
		{
			name: "witness",
			cfg: AgentEnvConfig{
				Role:     "witness",
				Rig:      "rig1",
				TownRoot: "/town",
				Agent:    "gemini",
			},
		},
		{
			name: "refinery",
			cfg: AgentEnvConfig{
				Role:     "refinery",
				Rig:      "rig1",
				TownRoot: "/town",
				Agent:    "codex",
			},
		},
		{
			name: "deacon",
			cfg: AgentEnvConfig{
				Role:     "deacon",
				TownRoot: "/town",
				Agent:    "gemini",
			},
		},
		{
			name: "crew",
			cfg: AgentEnvConfig{
				Role:             "crew",
				Rig:              "rig1",
				AgentName:        "worker1",
				TownRoot:         "/town",
				RuntimeConfigDir: "/config",
				Agent:            "codex",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := AgentEnv(tc.cfg)
			assertEnv(t, env, "GT_AGENT", tc.cfg.Agent)
		})
	}
}

// TestAgentEnv_NoAgentOverrideOmitsKey verifies GT_AGENT is absent when
// both Agent and ResolvedAgent are empty, for all roles.
func TestAgentEnv_NoAgentOverrideOmitsKey(t *testing.T) {
	t.Parallel()
	roles := []string{"polecat", "witness", "refinery", "deacon", "crew"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			env := AgentEnv(AgentEnvConfig{
				Role:     role,
				TownRoot: "/town",
			})
			assertNotSet(t, env, "GT_AGENT")
		})
	}
}

// TestAgentEnv_ResolvedAgentFallback verifies that GT_AGENT is set from
// ResolvedAgent when Agent (explicit --agent override) is empty.
// This fixes the bug where non-polecat roles (Witness, Refinery, Crew, Deacon)
// had GT_AGENT missing from the tmux session table on the default dispatch path.
func TestAgentEnv_ResolvedAgentFallback(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:          "witness",
		Rig:           "myrig",
		TownRoot:      "/town",
		Agent:         "",       // no explicit override
		ResolvedAgent: "codex",  // resolved from config
	})

	// GT_AGENT should be set from ResolvedAgent fallback
	assertEnv(t, env, "GT_AGENT", "codex")
}

// TestAgentEnv_AgentOverrideTakesPrecedenceOverResolved verifies that an
// explicit Agent override takes precedence over ResolvedAgent.
func TestAgentEnv_AgentOverrideTakesPrecedenceOverResolved(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:          "polecat",
		Rig:           "myrig",
		AgentName:     "Toast",
		TownRoot:      "/town",
		Agent:         "gemini",  // explicit override
		ResolvedAgent: "codex",   // resolved from config (should be ignored)
	})

	// Agent override takes precedence
	assertEnv(t, env, "GT_AGENT", "gemini")
}

// TestAgentEnv_ResolvedAgentFallbackAllRoles verifies that GT_AGENT is set
// from ResolvedAgent for every role that supports agent overrides.
// This is the fix for gt-nwgd5: non-polecat roles had GT_AGENT missing.
func TestAgentEnv_ResolvedAgentFallbackAllRoles(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  AgentEnvConfig
	}{
		{
			name: "polecat",
			cfg: AgentEnvConfig{
				Role:          "polecat",
				Rig:           "rig1",
				AgentName:     "Toast",
				TownRoot:      "/town",
				ResolvedAgent: "codex",
			},
		},
		{
			name: "witness",
			cfg: AgentEnvConfig{
				Role:          "witness",
				Rig:           "rig1",
				TownRoot:      "/town",
				ResolvedAgent: "gemini",
			},
		},
		{
			name: "refinery",
			cfg: AgentEnvConfig{
				Role:          "refinery",
				Rig:           "rig1",
				TownRoot:      "/town",
				ResolvedAgent: "codex",
			},
		},
		{
			name: "deacon",
			cfg: AgentEnvConfig{
				Role:          "deacon",
				TownRoot:      "/town",
				ResolvedAgent: "gemini",
			},
		},
		{
			name: "crew",
			cfg: AgentEnvConfig{
				Role:             "crew",
				Rig:              "rig1",
				AgentName:        "worker1",
				TownRoot:         "/town",
				RuntimeConfigDir: "/config",
				ResolvedAgent:    "codex",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := AgentEnv(tc.cfg)
			assertEnv(t, env, "GT_AGENT", tc.cfg.ResolvedAgent)
		})
	}
}

// TestAgentEnv_EmptyResolvedAgentOmitsKey verifies GT_AGENT is absent when
// both Agent and ResolvedAgent are empty (Claude default — no GT_AGENT needed).
func TestAgentEnv_EmptyResolvedAgentOmitsKey(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:          "witness",
		Rig:           "myrig",
		TownRoot:      "/town",
		Agent:         "",
		ResolvedAgent: "",
	})
	assertNotSet(t, env, "GT_AGENT")
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

func TestSanitizeAgentEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		resolvedEnv map[string]string
		callerEnv   map[string]string
		wantKey     bool   // expect NODE_OPTIONS to be present in resolvedEnv
		wantValue   string // expected value if present
	}{
		{
			name:        "neither map has NODE_OPTIONS — sets empty",
			resolvedEnv: map[string]string{"GT_ROLE": "polecat"},
			callerEnv:   map[string]string{"GT_ROLE": "polecat"},
			wantKey:     true,
			wantValue:   "",
		},
		{
			name:        "caller provides NODE_OPTIONS — preserved",
			resolvedEnv: map[string]string{"NODE_OPTIONS": "--max-old-space-size=4096"},
			callerEnv:   map[string]string{"NODE_OPTIONS": "--max-old-space-size=4096"},
			wantKey:     true,
			wantValue:   "--max-old-space-size=4096",
		},
		{
			name:        "rc.Env provides NODE_OPTIONS in resolvedEnv — preserved",
			resolvedEnv: map[string]string{"NODE_OPTIONS": "--max-old-space-size=8192"},
			callerEnv:   map[string]string{},
			wantKey:     true,
			wantValue:   "--max-old-space-size=8192",
		},
		{
			name:        "empty maps — sets empty",
			resolvedEnv: map[string]string{},
			callerEnv:   map[string]string{},
			wantKey:     true,
			wantValue:   "",
		},
		{
			name:        "same map without NODE_OPTIONS — sets empty (lifecycle.go pattern)",
			resolvedEnv: map[string]string{"GT_ROLE": "polecat", "GT_RIG": "myrig"},
			callerEnv:   nil, // will be set to same map below
			wantKey:     true,
			wantValue:   "",
		},
		{
			name:        "AgentEnv output with empty callerEnv — preserves empty NODE_OPTIONS",
			resolvedEnv: map[string]string{"GT_ROLE": "polecat", "NODE_OPTIONS": ""},
			callerEnv:   map[string]string{},
			wantKey:     true,
			wantValue:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callerEnv := tt.callerEnv
			if callerEnv == nil {
				// same-map pattern: pass resolvedEnv as both args (lifecycle.go pattern)
				callerEnv = tt.resolvedEnv
			}
			SanitizeAgentEnv(tt.resolvedEnv, callerEnv)
			val, ok := tt.resolvedEnv["NODE_OPTIONS"]
			if ok != tt.wantKey {
				t.Errorf("NODE_OPTIONS present=%v, want %v", ok, tt.wantKey)
			}
			if ok && val != tt.wantValue {
				t.Errorf("NODE_OPTIONS=%q, want %q", val, tt.wantValue)
			}
		})
	}
}

func TestSanitizeAgentEnv_ClearsClaudeCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		resolvedEnv map[string]string
		callerEnv   map[string]string
		wantKey     bool   // expect CLAUDECODE to be present in resolvedEnv
		wantValue   string // expected value if present
	}{
		{
			name:        "neither map has CLAUDECODE — sets empty",
			resolvedEnv: map[string]string{"GT_ROLE": "polecat"},
			callerEnv:   map[string]string{"GT_ROLE": "polecat"},
			wantKey:     true,
			wantValue:   "",
		},
		{
			name:        "caller provides CLAUDECODE — preserved",
			resolvedEnv: map[string]string{"CLAUDECODE": "1"},
			callerEnv:   map[string]string{"CLAUDECODE": "1"},
			wantKey:     true,
			wantValue:   "1",
		},
		{
			name:        "inherited CLAUDECODE not in callerEnv — cleared",
			resolvedEnv: map[string]string{"CLAUDECODE": "1"},
			callerEnv:   map[string]string{},
			wantKey:     true,
			wantValue:   "",
		},
		{
			name:        "empty maps — sets empty",
			resolvedEnv: map[string]string{},
			callerEnv:   map[string]string{},
			wantKey:     true,
			wantValue:   "",
		},
		{
			name:        "same map without CLAUDECODE — sets empty (lifecycle.go pattern)",
			resolvedEnv: map[string]string{"GT_ROLE": "polecat", "GT_RIG": "myrig"},
			callerEnv:   nil, // will be set to same map below
			wantKey:     true,
			wantValue:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callerEnv := tt.callerEnv
			if callerEnv == nil {
				callerEnv = tt.resolvedEnv
			}
			SanitizeAgentEnv(tt.resolvedEnv, callerEnv)
			val, ok := tt.resolvedEnv["CLAUDECODE"]
			if ok != tt.wantKey {
				t.Errorf("CLAUDECODE present=%v, want %v", ok, tt.wantKey)
			}
			if ok && val != tt.wantValue {
				t.Errorf("CLAUDECODE=%q, want %q", val, tt.wantValue)
			}
		})
	}
}

func TestAgentEnv_IncludesNodeOptionsClearing(t *testing.T) {
	t.Parallel()
	// Verify AgentEnv always includes NODE_OPTIONS="" regardless of role.
	// This protects tmux SetEnvironment and EnvForExecCommand paths.
	roles := []struct {
		role      string
		rig       string
		agentName string
	}{
		{"mayor", "", ""},
		{"deacon", "", ""},
		{"boot", "", ""},
		{"witness", "myrig", ""},
		{"refinery", "myrig", ""},
		{"polecat", "myrig", "Toast"},
		{"crew", "myrig", "emma"},
	}
	for _, r := range roles {
		t.Run(r.role, func(t *testing.T) {
			env := AgentEnv(AgentEnvConfig{
				Role:      r.role,
				Rig:       r.rig,
				AgentName: r.agentName,
				TownRoot:  "/town",
			})
			assertEnv(t, env, "NODE_OPTIONS", "")
		})
	}
}

func TestAgentEnv_IncludesClaudeCodeClearing(t *testing.T) {
	t.Parallel()
	// Verify AgentEnv always includes CLAUDECODE="" regardless of role.
	// This prevents nested session detection when gt sling is invoked
	// from within a Claude Code session (issue #1666).
	roles := []struct {
		role      string
		rig       string
		agentName string
	}{
		{"mayor", "", ""},
		{"deacon", "", ""},
		{"boot", "", ""},
		{"witness", "myrig", ""},
		{"refinery", "myrig", ""},
		{"polecat", "myrig", "Toast"},
		{"crew", "myrig", "emma"},
	}
	for _, r := range roles {
		t.Run(r.role, func(t *testing.T) {
			env := AgentEnv(AgentEnvConfig{
				Role:      r.role,
				Rig:       r.rig,
				AgentName: r.agentName,
				TownRoot:  "/town",
			})
			assertEnv(t, env, "CLAUDECODE", "")
		})
	}
}

func TestBuildStartupCommandWithEnv_IncludesNodeOptions(t *testing.T) {
	t.Parallel()
	// Integration test: verify BuildStartupCommandWithEnv output includes NODE_OPTIONS=
	// when the env map has it set to empty (as AgentEnv produces).
	env := map[string]string{
		"GT_ROLE":      "polecat",
		"NODE_OPTIONS": "",
	}
	result := BuildStartupCommandWithEnv(env, "claude", "")
	expected := "export GT_ROLE=polecat NODE_OPTIONS= && claude"
	if result != expected {
		t.Errorf("BuildStartupCommandWithEnv() = %q, want %q", result, expected)
	}
}
