// Test Kiro-Only Operation
//
// This integration test proves that Gas Town works with Kiro as the sole agent,
// with zero dependency on Claude Code. It exercises the config, loader, session
// identity, and kiro settings layers end-to-end.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/kiro"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupKiroOnlyTown creates a minimal town+rig layout with kiro as the sole agent.
// The town default_agent is "kiro" and no Claude references appear in any config.
func setupKiroOnlyTown(t *testing.T, townRoot, rigName string) string {
	t.Helper()

	rigPath := filepath.Join(townRoot, rigName)

	dirs := []string{
		filepath.Join(townRoot, "mayor"),
		filepath.Join(townRoot, "settings"),
		filepath.Join(rigPath, "settings"),
		filepath.Join(rigPath, "polecats"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	// town identity
	writeKiroJSON(t, filepath.Join(townRoot, "mayor", "town.json"), map[string]interface{}{
		"type":       "town",
		"version":    2,
		"name":       "kiro-town",
		"created_at": time.Now().Format(time.RFC3339),
	})

	// town settings -- kiro only
	writeKiroJSON(t, filepath.Join(townRoot, "settings", "config.json"), map[string]interface{}{
		"type":          "town-settings",
		"version":       1,
		"default_agent": "kiro",
	})

	// rig settings -- kiro only
	writeKiroJSON(t, filepath.Join(rigPath, "settings", "config.json"), map[string]interface{}{
		"type":    "rig-settings",
		"version": 1,
		"agent":   "kiro",
	})

	// rigs registry
	writeKiroJSON(t, filepath.Join(townRoot, "mayor", "rigs.json"), map[string]interface{}{
		"version": 1,
		"rigs": map[string]interface{}{
			rigName: map[string]interface{}{
				"git_url":  "https://github.com/test/kiro-rig.git",
				"added_at": time.Now().Format(time.RFC3339),
			},
		},
	})

	return rigPath
}

// writeKiroJSON marshals data as indented JSON and writes it to path.
func writeKiroJSON(t *testing.T, path string, data interface{}) {
	t.Helper()
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// assertNoClaudeRefs scans a string for the word "claude" (case-insensitive)
// and fails if found. Used to prove Kiro-only operation.
func assertNoClaudeRefs(t *testing.T, label, s string) {
	t.Helper()
	if strings.Contains(strings.ToLower(s), "claude") {
		t.Errorf("%s contains Claude reference: %s", label, s)
	}
}

// Session identity helpers -- inlined here to avoid importing internal/session
// (which would create a cycle: config test -> session -> tmux -> config).

// mailAddress returns the mail-style address for a role.
func mailAddress(role, rig, name string) string {
	switch role {
	case "mayor":
		return "mayor"
	case "deacon":
		return "deacon"
	case "witness":
		return fmt.Sprintf("%s/witness", rig)
	case "refinery":
		return fmt.Sprintf("%s/refinery", rig)
	case "crew":
		return fmt.Sprintf("%s/crew/%s", rig, name)
	case "polecat":
		return fmt.Sprintf("%s/polecats/%s", rig, name)
	default:
		return ""
	}
}

// tmuxSessionName returns the tmux session name for a role.
func tmuxSessionName(role, rig, name string) string {
	switch role {
	case "mayor":
		return "hq-mayor"
	case "deacon":
		return "hq-deacon"
	case "witness":
		return fmt.Sprintf("gt-%s-witness", rig)
	case "refinery":
		return fmt.Sprintf("gt-%s-refinery", rig)
	case "crew":
		return fmt.Sprintf("gt-%s-crew-%s", rig, name)
	case "polecat":
		return fmt.Sprintf("gt-%s-%s", rig, name)
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// 1. TestKiroOnlyRigConfiguration
// ---------------------------------------------------------------------------

// TestKiroOnlyRigConfiguration creates a rig config that uses "kiro" as the agent,
// verifies it loads and resolves correctly with no Claude references.
func TestKiroOnlyRigConfiguration(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	rigName := "myrig"
	rigPath := setupKiroOnlyTown(t, townRoot, rigName)

	// Verify town settings load with kiro default
	ts, err := LoadOrCreateTownSettings(TownSettingsPath(townRoot))
	if err != nil {
		t.Fatalf("LoadOrCreateTownSettings: %v", err)
	}
	if ts.DefaultAgent != "kiro" {
		t.Errorf("DefaultAgent = %q, want kiro", ts.DefaultAgent)
	}

	// Verify rig settings load with kiro agent
	rs, err := LoadRigSettings(RigSettingsPath(rigPath))
	if err != nil {
		t.Fatalf("LoadRigSettings: %v", err)
	}
	if rs.Agent != "kiro" {
		t.Errorf("RigSettings.Agent = %q, want kiro", rs.Agent)
	}

	// Resolve agent and verify it is kiro
	rc := ResolveAgentConfig(townRoot, rigPath)
	if rc == nil {
		t.Fatal("ResolveAgentConfig returned nil")
	}
	if rc.Command != "kiro" {
		t.Errorf("resolved Command = %q, want kiro", rc.Command)
	}
	if len(rc.Args) != 1 || rc.Args[0] != "--trust-all-tools" {
		t.Errorf("resolved Args = %v, want [--trust-all-tools]", rc.Args)
	}

	// Serialize and check no Claude references leak
	b, _ := json.Marshal(rc)
	assertNoClaudeRefs(t, "RuntimeConfig JSON", string(b))
}

// ---------------------------------------------------------------------------
// 2. TestKiroRigRuntimeConfig
// ---------------------------------------------------------------------------

// TestKiroRigRuntimeConfig generates a RuntimeConfig for a Kiro rig, verifying
// Hooks.Provider is "kiro", the agent command is "kiro", and args are correct.
func TestKiroRigRuntimeConfig(t *testing.T) {
	t.Parallel()

	// Use normalizeRuntimeConfig to get full kiro defaults (internal API).
	rc := normalizeRuntimeConfig(&RuntimeConfig{Provider: "kiro"})

	// Provider
	if rc.Provider != "kiro" {
		t.Errorf("Provider = %q, want kiro", rc.Provider)
	}

	// Command & Args
	if rc.Command != "kiro" {
		t.Errorf("Command = %q, want kiro", rc.Command)
	}
	if len(rc.Args) != 1 || rc.Args[0] != "--trust-all-tools" {
		t.Errorf("Args = %v, want [--trust-all-tools]", rc.Args)
	}

	// PromptMode
	if rc.PromptMode != "none" {
		t.Errorf("PromptMode = %q, want none", rc.PromptMode)
	}

	// Hooks
	if rc.Hooks == nil {
		t.Fatal("Hooks is nil")
	}
	if rc.Hooks.Provider != "kiro" {
		t.Errorf("Hooks.Provider = %q, want kiro", rc.Hooks.Provider)
	}
	if rc.Hooks.Dir != ".kiro" {
		t.Errorf("Hooks.Dir = %q, want .kiro", rc.Hooks.Dir)
	}
	if rc.Hooks.SettingsFile != "settings.json" {
		t.Errorf("Hooks.SettingsFile = %q, want settings.json", rc.Hooks.SettingsFile)
	}

	// Session env vars (kiro does not use Claude session env)
	if rc.Session == nil {
		t.Fatal("Session is nil")
	}
	if rc.Session.SessionIDEnv != "" {
		t.Errorf("Session.SessionIDEnv = %q, want empty", rc.Session.SessionIDEnv)
	}
	if rc.Session.ConfigDirEnv != "" {
		t.Errorf("Session.ConfigDirEnv = %q, want empty", rc.Session.ConfigDirEnv)
	}

	// Tmux heuristics
	if rc.Tmux == nil {
		t.Fatal("Tmux is nil")
	}
	if len(rc.Tmux.ProcessNames) != 2 || rc.Tmux.ProcessNames[0] != "kiro" || rc.Tmux.ProcessNames[1] != "node" {
		t.Errorf("Tmux.ProcessNames = %v, want [kiro node]", rc.Tmux.ProcessNames)
	}
	if rc.Tmux.ReadyDelayMs != 8000 {
		t.Errorf("Tmux.ReadyDelayMs = %d, want 8000", rc.Tmux.ReadyDelayMs)
	}

	// Instructions
	if rc.Instructions == nil {
		t.Fatal("Instructions is nil")
	}
	if rc.Instructions.File != "AGENTS.md" {
		t.Errorf("Instructions.File = %q, want AGENTS.md", rc.Instructions.File)
	}
}

// ---------------------------------------------------------------------------
// 3. TestKiroSessionIdentitiesAllRoles
// ---------------------------------------------------------------------------

// TestKiroSessionIdentitiesAllRoles creates session identities for every role
// using Kiro and verifies that mail addresses and tmux session names resolve
// correctly with no Claude references.
func TestKiroSessionIdentitiesAllRoles(t *testing.T) {
	t.Parallel()

	const rigName = "kirorig"

	tests := []struct {
		name        string
		role        string
		rig         string
		agentName   string
		wantAddress string
		wantSession string
	}{
		{
			name:        "mayor",
			role:        "mayor",
			wantAddress: "mayor",
			wantSession: "hq-mayor",
		},
		{
			name:        "deacon",
			role:        "deacon",
			wantAddress: "deacon",
			wantSession: "hq-deacon",
		},
		{
			name:        "witness",
			role:        "witness",
			rig:         rigName,
			wantAddress: rigName + "/witness",
			wantSession: "gt-" + rigName + "-witness",
		},
		{
			name:        "refinery",
			role:        "refinery",
			rig:         rigName,
			wantAddress: rigName + "/refinery",
			wantSession: "gt-" + rigName + "-refinery",
		},
		{
			name:        "crew",
			role:        "crew",
			rig:         rigName,
			agentName:   "max",
			wantAddress: rigName + "/crew/max",
			wantSession: "gt-" + rigName + "-crew-max",
		},
		{
			name:        "polecat",
			role:        "polecat",
			rig:         rigName,
			agentName:   "spark",
			wantAddress: rigName + "/polecats/spark",
			wantSession: "gt-" + rigName + "-spark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := mailAddress(tt.role, tt.rig, tt.agentName)
			sess := tmuxSessionName(tt.role, tt.rig, tt.agentName)

			if addr != tt.wantAddress {
				t.Errorf("mailAddress() = %q, want %q", addr, tt.wantAddress)
			}
			if sess != tt.wantSession {
				t.Errorf("tmuxSessionName() = %q, want %q", sess, tt.wantSession)
			}

			// Also verify AgentEnv produces the right env vars
			env := AgentEnv(AgentEnvConfig{
				Role:      tt.role,
				Rig:       tt.rig,
				AgentName: tt.agentName,
			})
			if env["GT_ROLE"] != tt.wantAddress {
				t.Errorf("GT_ROLE = %q, want %q", env["GT_ROLE"], tt.wantAddress)
			}

			// No Claude references
			assertNoClaudeRefs(t, "address", addr)
			assertNoClaudeRefs(t, "session", sess)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. TestKiroSettingsGenerationAllRoles
// ---------------------------------------------------------------------------

// TestKiroSettingsGenerationAllRoles generates Kiro settings for each role type,
// verifying correct hooks, no Claude references, and gt commands present.
func TestKiroSettingsGenerationAllRoles(t *testing.T) {
	t.Parallel()

	type roleCase struct {
		role            string
		wantRoleType    kiro.RoleType
		wantMailInStart bool   // autonomous roles inject mail in SessionStart
		wantGTCommand   string // a gt command that must appear somewhere
	}

	cases := []roleCase{
		{"mayor", kiro.Interactive, false, "gt prime --hook"},
		{"deacon", kiro.Autonomous, true, "gt mail check --inject"},
		{"witness", kiro.Autonomous, true, "gt mail check --inject"},
		{"refinery", kiro.Autonomous, true, "gt mail check --inject"},
		{"crew", kiro.Interactive, false, "gt prime --hook"},
		{"polecat", kiro.Autonomous, true, "gt mail check --inject"},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			// Verify role type classification
			if got := kiro.RoleTypeFor(tc.role); got != tc.wantRoleType {
				t.Errorf("RoleTypeFor(%q) = %q, want %q", tc.role, got, tc.wantRoleType)
			}

			// Generate settings file
			dir := t.TempDir()
			if err := kiro.EnsureSettingsForRoleAt(dir, tc.role, ".kiro", "settings.json"); err != nil {
				t.Fatalf("EnsureSettingsForRoleAt: %v", err)
			}

			data, err := os.ReadFile(filepath.Join(dir, ".kiro", "settings.json"))
			if err != nil {
				t.Fatalf("read settings: %v", err)
			}
			content := string(data)

			// Must parse as valid JSON
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("settings is not valid JSON: %v", err)
			}

			// Must contain hooks key
			if _, ok := raw["hooks"]; !ok {
				t.Error("settings missing 'hooks' key")
			}

			// gt commands present
			if !strings.Contains(content, tc.wantGTCommand) {
				t.Errorf("settings missing gt command %q", tc.wantGTCommand)
			}

			// Autonomous roles must have mail check in SessionStart
			if tc.wantMailInStart {
				hooks, ok := raw["hooks"].(map[string]interface{})
				if !ok {
					t.Fatal("hooks is not an object")
				}
				sessionStart, ok := hooks["SessionStart"]
				if !ok {
					t.Fatal("missing SessionStart hook")
				}
				ssJSON, _ := json.Marshal(sessionStart)
				if !strings.Contains(string(ssJSON), "gt mail check --inject") {
					t.Error("SessionStart hook missing 'gt mail check --inject' for autonomous role")
				}
			}

			// gt costs record in Stop hook
			if !strings.Contains(content, "gt costs record") {
				t.Error("settings missing 'gt costs record' in Stop hook")
			}

			// No Claude references anywhere in the output
			assertNoClaudeRefs(t, "kiro settings for "+tc.role, content)
		})
	}
}

// ---------------------------------------------------------------------------
// 5. TestKiroAgentResolutionWithoutClaude
// ---------------------------------------------------------------------------

// TestKiroAgentResolutionWithoutClaude verifies that when ONLY kiro is in PATH
// (not claude), the agent resolution still works and picks kiro.
func TestKiroAgentResolutionWithoutClaude(t *testing.T) {
	// This test manipulates PATH so must not run in parallel.

	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	rigName := "konly"
	rigPath := setupKiroOnlyTown(t, townRoot, rigName)

	// Build a minimal PATH that contains only a kiro stub (no claude).
	binDir := filepath.Join(tmpDir, "kiro-bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("mkdir kiro-bin: %v", err)
	}
	stub := []byte("#!/bin/sh\nexit 0\n")
	kiroStub := filepath.Join(binDir, "kiro")
	if err := os.WriteFile(kiroStub, stub, 0755); err != nil {
		t.Fatalf("write kiro stub: %v", err)
	}

	// Also create a gt stub so ValidateAgentConfig can find it
	gtStub := filepath.Join(binDir, "gt")
	if err := os.WriteFile(gtStub, stub, 0755); err != nil {
		t.Fatalf("write gt stub: %v", err)
	}

	// Save and restore original PATH
	origPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir); err != nil {
		t.Fatalf("setenv PATH: %v", err)
	}
	t.Cleanup(func() {
		os.Setenv("PATH", origPath) //nolint:errcheck
	})

	// Reset registry so it re-evaluates with the new PATH
	ResetRegistryForTesting()
	t.Cleanup(ResetRegistryForTesting)

	// Resolve agent -- should pick kiro since it is the town default
	rc := ResolveAgentConfig(townRoot, rigPath)
	if rc == nil {
		t.Fatal("ResolveAgentConfig returned nil")
	}
	if rc.Command != "kiro" {
		t.Errorf("Command = %q, want kiro (Claude is not in PATH)", rc.Command)
	}

	// Also test with the preset directly
	preset := GetAgentPreset(AgentKiro)
	if preset == nil {
		t.Fatal("kiro preset missing")
	}
	if preset.Command != "kiro" {
		t.Errorf("preset Command = %q, want kiro", preset.Command)
	}

	// Verify no Claude references in the resolved config
	b, _ := json.Marshal(rc)
	assertNoClaudeRefs(t, "resolved config without Claude in PATH", string(b))
}

// ---------------------------------------------------------------------------
// 6. TestKiroCustomAgentOverride
// ---------------------------------------------------------------------------

// TestKiroCustomAgentOverride tests that a custom agent config that overrides
// the kiro preset works correctly (e.g. a "kiro-fast" agent with extra flags).
func TestKiroCustomAgentOverride(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	rigName := "overrig"
	rigPath := filepath.Join(townRoot, rigName)

	// Create directory structure
	for _, dir := range []string{
		filepath.Join(townRoot, "mayor"),
		filepath.Join(townRoot, "settings"),
		filepath.Join(rigPath, "settings"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	// Town identity
	writeKiroJSON(t, filepath.Join(townRoot, "mayor", "town.json"), map[string]interface{}{
		"type":       "town",
		"version":    2,
		"name":       "kiro-override-town",
		"created_at": time.Now().Format(time.RFC3339),
	})

	// Town settings with a custom "kiro-fast" agent defined
	writeKiroJSON(t, filepath.Join(townRoot, "settings", "config.json"), map[string]interface{}{
		"type":          "town-settings",
		"version":       1,
		"default_agent": "kiro",
		"agents": map[string]interface{}{
			"kiro-fast": map[string]interface{}{
				"provider": "kiro",
				"command":  "/opt/kiro/bin/kiro",
				"args":     []string{"--trust-all-tools", "--turbo"},
			},
		},
	})

	// Rig settings with agent pointing to the custom override
	writeKiroJSON(t, filepath.Join(rigPath, "settings", "config.json"), map[string]interface{}{
		"type":    "rig-settings",
		"version": 1,
		"agent":   "kiro-fast",
	})

	// rigs.json
	writeKiroJSON(t, filepath.Join(townRoot, "mayor", "rigs.json"), map[string]interface{}{
		"version": 1,
		"rigs": map[string]interface{}{
			rigName: map[string]interface{}{
				"git_url":  "https://github.com/test/kiro-override.git",
				"added_at": time.Now().Format(time.RFC3339),
			},
		},
	})

	// Resolve -- should pick "kiro-fast" from town custom agents
	rc := ResolveAgentConfig(townRoot, rigPath)
	if rc == nil {
		t.Fatal("ResolveAgentConfig returned nil")
	}
	if rc.Command != "/opt/kiro/bin/kiro" {
		t.Errorf("Command = %q, want /opt/kiro/bin/kiro", rc.Command)
	}

	// Verify --turbo is in args
	found := false
	for _, a := range rc.Args {
		if a == "--turbo" {
			found = true
		}
	}
	if !found {
		t.Errorf("Args %v missing --turbo", rc.Args)
	}

	// Verify --trust-all-tools is preserved
	hasTrust := false
	for _, a := range rc.Args {
		if a == "--trust-all-tools" {
			hasTrust = true
		}
	}
	if !hasTrust {
		t.Errorf("Args %v missing --trust-all-tools", rc.Args)
	}

	// ResolveAgentConfigWithOverride for an explicit override
	rcOverride, name, err := ResolveAgentConfigWithOverride(townRoot, rigPath, "kiro-fast")
	if err != nil {
		t.Fatalf("ResolveAgentConfigWithOverride: %v", err)
	}
	if name != "kiro-fast" {
		t.Errorf("agent name = %q, want kiro-fast", name)
	}
	if rcOverride.Command != "/opt/kiro/bin/kiro" {
		t.Errorf("override Command = %q, want /opt/kiro/bin/kiro", rcOverride.Command)
	}

	// Also test that the built-in kiro preset is still accessible
	rcBuiltin, builtinName, err := ResolveAgentConfigWithOverride(townRoot, rigPath, "kiro")
	if err != nil {
		t.Fatalf("ResolveAgentConfigWithOverride(kiro): %v", err)
	}
	if builtinName != "kiro" {
		t.Errorf("built-in name = %q, want kiro", builtinName)
	}
	if rcBuiltin.Command != "kiro" {
		t.Errorf("built-in Command = %q, want kiro", rcBuiltin.Command)
	}

	// Verify custom agent roundtrip via startup command
	cmd := BuildPolecatStartupCommand(rigName, "speedy", rigPath, "")
	if !strings.Contains(cmd, "/opt/kiro/bin/kiro") {
		t.Errorf("startup command missing custom kiro path: %s", cmd)
	}
	if !strings.Contains(cmd, "--turbo") {
		t.Errorf("startup command missing --turbo: %s", cmd)
	}

	// Env vars present
	if !strings.Contains(cmd, "GT_ROLE="+rigName+"/polecats/speedy") {
		t.Errorf("startup command missing GT_ROLE=%s/polecats/speedy: %s", rigName, cmd)
	}
	if !strings.Contains(cmd, "GT_POLECAT=speedy") {
		t.Errorf("startup command missing GT_POLECAT=speedy: %s", cmd)
	}

	// No Claude references
	assertNoClaudeRefs(t, "custom kiro startup command", cmd)
}

// ---------------------------------------------------------------------------
// Supplementary: environment variable generation for Kiro roles
// ---------------------------------------------------------------------------

// TestKiroAgentEnvAllRoles verifies that AgentEnv generates correct environment
// variables for every role when using Kiro (no Claude-specific env vars leak).
func TestKiroAgentEnvAllRoles(t *testing.T) {
	t.Parallel()

	const rig = "krig"

	cases := []struct {
		role      string
		agentName string
		wantKeys  []string // expected env var keys
		denyKeys  []string // must NOT appear
	}{
		{
			role:     "mayor",
			wantKeys: []string{"GT_ROLE", "BD_ACTOR", "GIT_AUTHOR_NAME"},
			denyKeys: []string{"GT_RIG", "GT_POLECAT", "GT_CREW"},
		},
		{
			role:     "deacon",
			wantKeys: []string{"GT_ROLE", "BD_ACTOR"},
			denyKeys: []string{"GT_RIG"},
		},
		{
			role:     "witness",
			wantKeys: []string{"GT_ROLE", "GT_RIG", "BD_ACTOR"},
			denyKeys: []string{"GT_POLECAT", "GT_CREW"},
		},
		{
			role:     "refinery",
			wantKeys: []string{"GT_ROLE", "GT_RIG", "BD_ACTOR"},
			denyKeys: []string{"GT_POLECAT", "GT_CREW"},
		},
		{
			role:      "crew",
			agentName: "max",
			wantKeys:  []string{"GT_ROLE", "GT_RIG", "GT_CREW", "BD_ACTOR", "BEADS_AGENT_NAME"},
			denyKeys:  []string{"GT_POLECAT"},
		},
		{
			role:      "polecat",
			agentName: "spark",
			wantKeys:  []string{"GT_ROLE", "GT_RIG", "GT_POLECAT", "BD_ACTOR", "BEADS_AGENT_NAME"},
			denyKeys:  []string{"GT_CREW"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			env := AgentEnv(AgentEnvConfig{
				Role:      tc.role,
				Rig:       rig,
				AgentName: tc.agentName,
			})

			// Required keys
			for _, k := range tc.wantKeys {
				if _, ok := env[k]; !ok {
					t.Errorf("missing env var %s for role %s", k, tc.role)
				}
			}

			// Denied keys
			for _, k := range tc.denyKeys {
				if _, ok := env[k]; ok {
					t.Errorf("unexpected env var %s for role %s", k, tc.role)
				}
			}

			// No CLAUDE_CONFIG_DIR unless explicitly set
			if _, ok := env["CLAUDE_CONFIG_DIR"]; ok {
				t.Errorf("CLAUDE_CONFIG_DIR should not appear for Kiro role %s", tc.role)
			}

			// Role value (compound format: rig/role or rig/polecats/name etc.)
			wantRole := mailAddress(tc.role, rig, tc.agentName)
			if env["GT_ROLE"] != wantRole {
				t.Errorf("GT_ROLE = %q, want %q", env["GT_ROLE"], wantRole)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Supplementary: full startup command with Kiro provider
// ---------------------------------------------------------------------------

// TestKiroStartupCommandNoClaudeArtifacts builds full startup commands for
// all rig-level roles via Kiro and checks that zero Claude-specific artifacts
// (CLAUDE_SESSION_ID, CLAUDE_CONFIG_DIR, --dangerously-skip-permissions) appear.
func TestKiroStartupCommandNoClaudeArtifacts(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	townRoot := filepath.Join(tmpDir, "town")
	rigName := "cmdrig"
	rigPath := setupKiroOnlyTown(t, townRoot, rigName)

	claudeArtifacts := []string{
		"CLAUDE_SESSION_ID",
		"CLAUDE_CONFIG_DIR",
		"--dangerously-skip-permissions",
	}

	roles := []struct {
		build func() string
		label string
	}{
		{
			label: "witness",
			build: func() string {
				return BuildAgentStartupCommand("witness", rigName, townRoot, rigPath, "")
			},
		},
		{
			label: "refinery",
			build: func() string {
				return BuildAgentStartupCommand("refinery", rigName, townRoot, rigPath, "")
			},
		},
		{
			label: "polecat",
			build: func() string {
				return BuildPolecatStartupCommand(rigName, "rusty", rigPath, "")
			},
		},
		{
			label: "crew",
			build: func() string {
				return BuildCrewStartupCommand(rigName, "max", rigPath, "")
			},
		},
	}

	for _, r := range roles {
		t.Run(r.label, func(t *testing.T) {
			cmd := r.build()

			// Must contain kiro
			if !strings.Contains(cmd, "kiro") {
				t.Errorf("%s command missing 'kiro': %s", r.label, cmd)
			}

			// Must contain --trust-all-tools
			if !strings.Contains(cmd, "--trust-all-tools") {
				t.Errorf("%s command missing --trust-all-tools: %s", r.label, cmd)
			}

			// Must not contain any Claude artifacts
			for _, artifact := range claudeArtifacts {
				if strings.Contains(cmd, artifact) {
					t.Errorf("%s command contains Claude artifact %q: %s", r.label, artifact, cmd)
				}
			}
		})
	}
}
