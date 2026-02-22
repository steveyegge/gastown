package config

import (
	"strings"
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

func TestAgentEnv_Dog(t *testing.T) {
	t.Parallel()
	env := AgentEnv(AgentEnvConfig{
		Role:      "dog",
		AgentName: "alpha",
		TownRoot:  "/town",
	})

	assertEnv(t, env, "GT_ROLE", "dog")
	assertEnv(t, env, "BD_ACTOR", "dog/alpha")
	assertEnv(t, env, "GIT_AUTHOR_NAME", "alpha")
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

// TestAgentEnv_WithoutAgentOverride_RequiresFallback documents that callers
// must set GT_AGENT from RuntimeConfig.ResolvedAgent when AgentEnvConfig.Agent
// is empty. AgentEnv intentionally omits GT_AGENT without an explicit override,
// but tmux session table consumers (IsAgentAlive, GT_AGENT validation) need it.
// Regression test for PR #1776 which removed the session_manager.go fallback.
func TestAgentEnv_WithoutAgentOverride_RequiresFallback(t *testing.T) {
	t.Parallel()

	// Simulate the default polecat dispatch path (no --agent flag).
	// This is what lifecycle.go calls when gt scheduler run / gt sling dispatches.
	env := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "/town",
		Agent:     "", // no explicit override — the common case
	})

	// GT_AGENT must NOT be in the map — this confirms callers need a fallback.
	// session_manager.go must compensate by writing runtimeConfig.ResolvedAgent
	// to the tmux session table via SetEnvironment.
	if _, ok := env["GT_AGENT"]; ok {
		t.Error("AgentEnv should NOT set GT_AGENT when Agent is empty; " +
			"callers must fall back to runtimeConfig.ResolvedAgent")
	}

	// With an explicit override, GT_AGENT IS set.
	envWithOverride := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "myrig",
		AgentName: "Toast",
		TownRoot:  "/town",
		Agent:     "codex",
	})
	assertEnv(t, envWithOverride, "GT_AGENT", "codex")
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
// Agent is empty, for all roles. This is the default behavior.
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

func TestSanitizeOTELAttrValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "simple string unchanged",
			input:  "hello world",
			maxLen: 50,
			want:   "hello world",
		},
		{
			name:   "first line only",
			input:  "first line\nsecond line\nthird line",
			maxLen: 100,
			want:   "first line",
		},
		{
			name:   "commas replaced with pipe",
			input:  "a,b,c",
			maxLen: 50,
			want:   "a|b|c",
		},
		{
			name:   "truncated to maxLen",
			input:  "abcdefghij",
			maxLen: 5,
			want:   "abcde",
		},
		{
			name:   "beacon first line",
			input:  "[GAS TOWN] polecat rust (rig: gastown) <- witness • 2025-12-30T15:42 • assigned:gt-abc12\n\nRun `gt prime --hook`",
			maxLen: 120,
			want:   "[GAS TOWN] polecat rust (rig: gastown) <- witness • 2025-12-30T15:42 • assigned:gt-abc12",
		},
		{
			name:   "trims leading/trailing space",
			input:  "  hello  ",
			maxLen: 50,
			want:   "hello",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 50,
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOTELAttrValue(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("sanitizeOTELAttrValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgentEnv_OTELPromptAndTown(t *testing.T) {
	t.Setenv("GT_OTEL_METRICS_URL", "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv("GT_OTEL_LOGS_URL", "http://localhost:9428/insert/opentelemetry/v1/logs")

	beacon := "[GAS TOWN] polecat rust (rig: gastown) <- witness • 2025-12-30T15:42 • assigned:gt-abc12\n\nRun `gt prime --hook`"
	env := AgentEnv(AgentEnvConfig{
		Role:      "polecat",
		Rig:       "gastown",
		AgentName: "rust",
		TownRoot:  "/home/user/mytown",
		Prompt:    beacon,
	})

	attrs := env["OTEL_RESOURCE_ATTRIBUTES"]
	if attrs == "" {
		t.Fatal("expected OTEL_RESOURCE_ATTRIBUTES to be set")
	}

	// gt.town should be basename of TownRoot
	if !containsAttr(attrs, "gt.town=mytown") {
		t.Errorf("OTEL_RESOURCE_ATTRIBUTES missing gt.town=mytown, got: %s", attrs)
	}

	// gt.prompt should be the first line of the beacon (no newlines, commas replaced)
	wantPromptPrefix := "gt.prompt=[GAS TOWN] polecat rust"
	if !contains(attrs, wantPromptPrefix) {
		t.Errorf("OTEL_RESOURCE_ATTRIBUTES missing %q, got: %s", wantPromptPrefix, attrs)
	}

	// No newlines in the value
	if contains(attrs, "\n") {
		t.Errorf("OTEL_RESOURCE_ATTRIBUTES must not contain newlines, got: %s", attrs)
	}
}

func TestAgentEnv_OTELNoPromptNoTown(t *testing.T) {
	t.Setenv("GT_OTEL_METRICS_URL", "http://localhost:8428/opentelemetry/api/v1/push")
	t.Setenv("GT_OTEL_LOGS_URL", "http://localhost:9428/insert/opentelemetry/v1/logs")

	env := AgentEnv(AgentEnvConfig{
		Role: "mayor",
		// No Prompt, no TownRoot
	})

	attrs := env["OTEL_RESOURCE_ATTRIBUTES"]
	if contains(attrs, "gt.prompt") {
		t.Errorf("OTEL_RESOURCE_ATTRIBUTES should not have gt.prompt when Prompt is empty, got: %s", attrs)
	}
	if contains(attrs, "gt.town") {
		t.Errorf("OTEL_RESOURCE_ATTRIBUTES should not have gt.town when TownRoot is empty, got: %s", attrs)
	}
}

func containsAttr(attrs, attr string) bool {
	for _, part := range splitAttrs(attrs) {
		if part == attr {
			return true
		}
	}
	return false
}

func splitAttrs(attrs string) []string {
	var parts []string
	for _, p := range strings.Split(attrs, ",") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}
