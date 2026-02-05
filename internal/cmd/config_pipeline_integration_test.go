//go:build integration

// Package cmd provides integration tests for the seed-to-materialize pipeline.
// These tests verify the full round-trip: seed config from templates/filesystem
// into beads DB, then materialize back to filesystem.
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/configbeads"
)

// --- Test helpers ---

// setupPipelineTestTown creates a minimal Gas Town directory with town.json.
func setupPipelineTestTown(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	mayorDir := filepath.Join(dir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	tc := &config.TownConfig{
		Type:       "town",
		Version:    2,
		Name:       "testtown",
		Owner:      "test@example.com",
		PublicName: "Test Town",
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := config.SaveTownConfig(filepath.Join(mayorDir, "town.json"), tc); err != nil {
		t.Fatal(err)
	}

	return dir
}

// setupPipelineBeads creates an isolated beads instance with initialized DB.
// Skips test if bd CLI is unavailable.
// Temporarily clears BD_DAEMON_HOST so that EnsureCustomTypes can run
// config_set against the local SQLite database instead of the remote daemon.
func setupPipelineBeads(t *testing.T, dir string) *beads.Beads {
	t.Helper()

	// BD_DAEMON_HOST blocks local config access - clear it for test isolation.
	if old := os.Getenv("BD_DAEMON_HOST"); old != "" {
		os.Unsetenv("BD_DAEMON_HOST")
		t.Cleanup(func() { os.Setenv("BD_DAEMON_HOST", old) })
	}

	bd := beads.NewIsolated(dir)
	if err := bd.Init("hq-"); err != nil {
		t.Skipf("cannot initialize beads repo (bd not available?): %v", err)
		return nil
	}
	beadsDir := filepath.Join(dir, ".beads")
	if err := beads.EnsureCustomTypes(beadsDir); err != nil {
		t.Logf("warning: could not set custom types: %v", err)
	}
	return bd
}

// --- Seed Hook Beads + Materialize Tests ---

func TestSeedAndMaterializeHooksForCrew(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Step 1: Seed hook beads
	created, skipped, _, err := seedHookBeads(bd)
	if err != nil {
		t.Fatalf("seedHookBeads failed: %v", err)
	}
	if created == 0 {
		t.Fatal("expected at least 1 bead created")
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped on first seed, got %d", skipped)
	}

	// Step 2: Resolve config metadata for crew role
	layers, err := bd.ResolveConfigMetadata(
		beads.ConfigCategoryClaudeHooks, "testtown", "", "crew", "")
	if err != nil {
		t.Fatalf("ResolveConfigMetadata failed: %v", err)
	}
	if len(layers) == 0 {
		t.Fatal("expected at least 1 metadata layer for crew")
	}

	// Step 3: Materialize to filesystem
	workDir := filepath.Join(dir, "crew-work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := claude.MaterializeSettings(workDir, "crew", layers); err != nil {
		t.Fatalf("MaterializeSettings failed: %v", err)
	}

	// Step 4: Verify the materialized settings.json
	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	// Verify hooks exist
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'hooks' key in materialized settings")
	}

	// Crew should have hooks from base + crew overlay
	// The base should have shared hooks (PreToolUse, etc.)
	// The crew overlay should have crew-specific SessionStart/Stop
	if len(hooks) == 0 {
		t.Error("expected non-empty hooks map")
	}

	// Verify that the result has standard hook types
	for _, hookType := range []string{"PreToolUse", "PostToolUse"} {
		if _, exists := hooks[hookType]; !exists {
			t.Errorf("expected hook type %s in materialized settings", hookType)
		}
	}
}

func TestSeedAndMaterializeHooksForPolecat(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed hook beads
	created, _, _, err := seedHookBeads(bd)
	if err != nil {
		t.Fatalf("seedHookBeads failed: %v", err)
	}
	if created == 0 {
		t.Fatal("expected at least 1 bead created")
	}

	// Resolve for polecat role
	layers, err := bd.ResolveConfigMetadata(
		beads.ConfigCategoryClaudeHooks, "testtown", "", "polecat", "")
	if err != nil {
		t.Fatalf("ResolveConfigMetadata failed: %v", err)
	}
	if len(layers) == 0 {
		t.Fatal("expected at least 1 metadata layer for polecat")
	}

	// Materialize to filesystem
	workDir := filepath.Join(dir, "polecat-work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := claude.MaterializeSettings(workDir, "polecat", layers); err != nil {
		t.Fatalf("MaterializeSettings failed: %v", err)
	}

	// Verify materialized settings
	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'hooks' key in materialized settings")
	}

	// Polecat should have hooks from base + polecat overlay
	if len(hooks) == 0 {
		t.Error("expected non-empty hooks map")
	}

	// Verify SessionStart is present (polecat gets its own SessionStart)
	if _, exists := hooks["SessionStart"]; !exists {
		t.Error("expected SessionStart hook for polecat role")
	}
}

func TestSeedAndMaterializeHooksCrewVsPolecat(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed hook beads
	if _, _, _, err := seedHookBeads(bd); err != nil {
		t.Fatalf("seedHookBeads failed: %v", err)
	}

	// Resolve for crew
	crewLayers, err := bd.ResolveConfigMetadata(
		beads.ConfigCategoryClaudeHooks, "testtown", "", "crew", "")
	if err != nil {
		t.Fatalf("ResolveConfigMetadata for crew failed: %v", err)
	}

	// Resolve for polecat
	polecatLayers, err := bd.ResolveConfigMetadata(
		beads.ConfigCategoryClaudeHooks, "testtown", "", "polecat", "")
	if err != nil {
		t.Fatalf("ResolveConfigMetadata for polecat failed: %v", err)
	}

	// Materialize both
	crewDir := filepath.Join(dir, "crew-work")
	polecatDir := filepath.Join(dir, "polecat-work")
	os.MkdirAll(crewDir, 0755)
	os.MkdirAll(polecatDir, 0755)

	if err := claude.MaterializeSettings(crewDir, "crew", crewLayers); err != nil {
		t.Fatalf("MaterializeSettings for crew failed: %v", err)
	}
	if err := claude.MaterializeSettings(polecatDir, "polecat", polecatLayers); err != nil {
		t.Fatalf("MaterializeSettings for polecat failed: %v", err)
	}

	// Read both settings files
	crewData, _ := os.ReadFile(filepath.Join(crewDir, ".claude", "settings.json"))
	polecatData, _ := os.ReadFile(filepath.Join(polecatDir, ".claude", "settings.json"))

	var crewSettings, polecatSettings map[string]interface{}
	json.Unmarshal(crewData, &crewSettings)
	json.Unmarshal(polecatData, &polecatSettings)

	crewHooks, _ := crewSettings["hooks"].(map[string]interface{})
	polecatHooks, _ := polecatSettings["hooks"].(map[string]interface{})

	// SessionStart should differ between crew and polecat
	crewSS, _ := json.Marshal(crewHooks["SessionStart"])
	polecatSS, _ := json.Marshal(polecatHooks["SessionStart"])
	if string(crewSS) == string(polecatSS) {
		t.Error("expected SessionStart to differ between crew and polecat")
	}

	// Shared hooks (PreToolUse, PostToolUse) should be identical
	for _, hookType := range []string{"PreToolUse", "PostToolUse"} {
		crewH, _ := json.Marshal(crewHooks[hookType])
		polecatH, _ := json.Marshal(polecatHooks[hookType])
		if string(crewH) != string(polecatH) {
			t.Errorf("expected %s to be identical between crew and polecat", hookType)
		}
	}
}

// --- Seed MCP Beads + Materialize Tests ---

func TestSeedAndMaterializeMCP(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed MCP beads
	created, skipped, _, err := seedMCPBeads(bd)
	if err != nil {
		t.Fatalf("seedMCPBeads failed: %v", err)
	}
	if created != 1 {
		t.Errorf("expected 1 MCP bead created, got %d", created)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Resolve MCP metadata
	layers, err := bd.ResolveConfigMetadata(
		beads.ConfigCategoryMCP, "testtown", "", "", "")
	if err != nil {
		t.Fatalf("ResolveConfigMetadata failed: %v", err)
	}
	if len(layers) == 0 {
		t.Fatal("expected at least 1 MCP metadata layer")
	}

	// Materialize to filesystem
	workDir := filepath.Join(dir, "mcp-work")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := claude.MaterializeMCPConfig(workDir, layers); err != nil {
		t.Fatalf("MaterializeMCPConfig failed: %v", err)
	}

	// Verify .mcp.json was created
	mcpPath := filepath.Join(workDir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf("reading .mcp.json: %v", err)
	}

	var mcpConfig map[string]interface{}
	if err := json.Unmarshal(data, &mcpConfig); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}

	// Should have mcpServers
	servers, ok := mcpConfig["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'mcpServers' key in materialized MCP config")
	}
	if len(servers) == 0 {
		t.Error("expected at least one MCP server")
	}
}

// --- Seed Idempotency (skip existing) ---

func TestSeedIdempotencySkipExisting(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// First seed: should create beads
	created1, skipped1, _, err := seedHookBeads(bd)
	if err != nil {
		t.Fatalf("first seedHookBeads failed: %v", err)
	}
	if created1 == 0 {
		t.Fatal("expected beads to be created on first seed")
	}
	if skipped1 != 0 {
		t.Errorf("expected 0 skipped on first seed, got %d", skipped1)
	}

	// Second seed: should skip all (idempotent)
	created2, skipped2, _, err := seedHookBeads(bd)
	if err != nil {
		t.Fatalf("second seedHookBeads failed: %v", err)
	}
	if created2 != 0 {
		t.Errorf("expected 0 created on second seed, got %d", created2)
	}
	if skipped2 != created1 {
		t.Errorf("expected %d skipped on second seed, got %d", created1, skipped2)
	}
}

func TestSeedIdempotencySkipExistingMCP(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// First seed
	created1, _, _, err := seedMCPBeads(bd)
	if err != nil {
		t.Fatalf("first seedMCPBeads failed: %v", err)
	}
	if created1 != 1 {
		t.Fatalf("expected 1 created, got %d", created1)
	}

	// Second seed: should skip
	created2, skipped2, _, err := seedMCPBeads(bd)
	if err != nil {
		t.Fatalf("second seedMCPBeads failed: %v", err)
	}
	if created2 != 0 {
		t.Errorf("expected 0 created on second seed, got %d", created2)
	}
	if skipped2 != 1 {
		t.Errorf("expected 1 skipped on second seed, got %d", skipped2)
	}
}

// --- Seed Force (update existing) ---

func TestSeedForceUpdateExisting(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// First seed: create the MCP bead
	created, _, _, err := seedMCPBeads(bd)
	if err != nil {
		t.Fatalf("first seedMCPBeads failed: %v", err)
	}
	if created != 1 {
		t.Fatalf("expected 1 created, got %d", created)
	}

	// Read the original metadata
	_, origFields, err := bd.GetConfigBead(beads.ConfigBeadID("mcp-global"))
	if err != nil {
		t.Fatalf("GetConfigBead failed: %v", err)
	}
	origMetadata := origFields.Metadata

	// Set force flag and re-seed
	oldForce := seedForce
	seedForce = true
	defer func() { seedForce = oldForce }()

	_, _, updated, err := seedMCPBeads(bd)
	if err != nil {
		t.Fatalf("force seedMCPBeads failed: %v", err)
	}
	if updated != 1 {
		t.Errorf("expected 1 updated with --force, got %d", updated)
	}

	// Verify metadata was updated (should be same content since we're re-seeding
	// from the same embedded template, but the updated_at timestamp should change)
	_, newFields, err := bd.GetConfigBead(beads.ConfigBeadID("mcp-global"))
	if err != nil {
		t.Fatalf("GetConfigBead after force failed: %v", err)
	}

	// Metadata content should be equivalent
	if origMetadata != newFields.Metadata {
		// Content differs - that's unexpected since same template, but not a failure
		// if the JSON is semantically equivalent
		var origMap, newMap map[string]interface{}
		json.Unmarshal([]byte(origMetadata), &origMap)
		json.Unmarshal([]byte(newFields.Metadata), &newMap)
		origJSON, _ := json.Marshal(origMap)
		newJSON, _ := json.Marshal(newMap)
		if string(origJSON) != string(newJSON) {
			t.Error("force re-seed produced different metadata content")
		}
	}

	// UpdatedAt should be populated
	if newFields.UpdatedAt == "" {
		t.Error("expected updated_at to be set after force update")
	}
}

func TestSeedForceUpdateHookBeads(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// First seed
	created, _, _, err := seedHookBeads(bd)
	if err != nil {
		t.Fatalf("first seedHookBeads failed: %v", err)
	}
	if created == 0 {
		t.Fatal("expected beads created on first seed")
	}

	// Force re-seed
	oldForce := seedForce
	seedForce = true
	defer func() { seedForce = oldForce }()

	_, _, updated, err := seedHookBeads(bd)
	if err != nil {
		t.Fatalf("force seedHookBeads failed: %v", err)
	}
	if updated == 0 {
		t.Error("expected at least 1 updated with --force")
	}
	if updated != created {
		t.Errorf("expected %d updated (matching created count), got %d", created, updated)
	}
}

// --- Seed Role Beads + LoadRoleDefinitionFromBeads ---

func TestSeedRoleBeadsAndLoad(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed role beads (global builtin roles)
	created, _, _, err := seedRoleBeads(bd, dir)
	if err != nil {
		t.Fatalf("seedRoleBeads failed: %v", err)
	}
	if created == 0 {
		t.Fatal("expected at least 1 role bead created")
	}

	// Verify each role can be loaded from beads
	for _, roleName := range config.AllRoles() {
		t.Run("role_"+roleName, func(t *testing.T) {
			def, err := configbeads.LoadRoleDefinitionFromBeads(bd, "testtown", "", roleName)
			if err != nil {
				t.Fatalf("LoadRoleDefinitionFromBeads(%s) failed: %v", roleName, err)
			}
			if def == nil {
				t.Fatalf("expected non-nil RoleDefinition for %s", roleName)
			}
			if def.Role != roleName {
				t.Errorf("role = %q, want %q", def.Role, roleName)
			}
			if def.Scope == "" {
				t.Errorf("expected non-empty scope for role %s", roleName)
			}
		})
	}
}

func TestSeedRoleBeadsMatchBuiltin(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed roles
	if _, _, _, err := seedRoleBeads(bd, dir); err != nil {
		t.Fatalf("seedRoleBeads failed: %v", err)
	}

	// For each role, compare bead-loaded definition with builtin
	for _, roleName := range config.AllRoles() {
		t.Run("match_"+roleName, func(t *testing.T) {
			// Load from beads
			beadDef, err := configbeads.LoadRoleDefinitionFromBeads(bd, "testtown", "", roleName)
			if err != nil {
				t.Fatalf("LoadRoleDefinitionFromBeads(%s) failed: %v", roleName, err)
			}
			if beadDef == nil {
				t.Fatalf("no bead definition for %s", roleName)
			}

			// Load builtin
			builtinDef, err := config.LoadBuiltinRoleDefinition(roleName)
			if err != nil {
				t.Fatalf("LoadBuiltinRoleDefinition(%s) failed: %v", roleName, err)
			}

			// Compare key fields
			if beadDef.Role != builtinDef.Role {
				t.Errorf("role mismatch: bead=%q builtin=%q", beadDef.Role, builtinDef.Role)
			}
			if beadDef.Scope != builtinDef.Scope {
				t.Errorf("scope mismatch: bead=%q builtin=%q", beadDef.Scope, builtinDef.Scope)
			}
		})
	}
}

func TestSeedRoleBeadsWithRigOverride(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Create a rig with a role override
	rigName := "testrig"
	rigRolesDir := filepath.Join(dir, rigName, "roles")
	if err := os.MkdirAll(rigRolesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a rigs.json so seedRoleBeads finds the rig
	rigsConfig := &config.RigsConfig{
		Rigs: map[string]config.RigEntry{
			rigName: {
				GitURL:    "https://example.com/test.git",
				LocalRepo: filepath.Join(dir, rigName),
			},
		},
	}
	rigsJSON, _ := json.Marshal(rigsConfig)
	if err := os.WriteFile(filepath.Join(dir, "mayor", "rigs.json"), rigsJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// Write a polecat role override TOML
	polecatOverride := `role = "polecat"
scope = "rig"

[session]
pattern = "custom-{rig}-{name}"
`
	if err := os.WriteFile(filepath.Join(rigRolesDir, "polecat.toml"), []byte(polecatOverride), 0644); err != nil {
		t.Fatal(err)
	}

	// Seed role beads (includes rig override)
	created, _, _, err := seedRoleBeads(bd, dir)
	if err != nil {
		t.Fatalf("seedRoleBeads failed: %v", err)
	}
	if created == 0 {
		t.Fatal("expected beads created")
	}

	// Load polecat definition scoped to testrig - should get merged result
	def, err := configbeads.LoadRoleDefinitionFromBeads(bd, "testtown", rigName, "polecat")
	if err != nil {
		t.Fatalf("LoadRoleDefinitionFromBeads(polecat) failed: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil RoleDefinition for polecat with rig override")
	}
	if def.Role != "polecat" {
		t.Errorf("role = %q, want %q", def.Role, "polecat")
	}

	// The rig override should have merged the session.pattern
	if def.Session.Pattern != "custom-{rig}-{name}" {
		t.Errorf("session.pattern = %q, want custom pattern from rig override", def.Session.Pattern)
	}
}

// --- Full Pipeline Round-Trip ---

func TestFullPipelineRoundTrip(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed both hooks and MCP
	hookCreated, _, _, err := seedHookBeads(bd)
	if err != nil {
		t.Fatalf("seedHookBeads failed: %v", err)
	}
	mcpCreated, _, _, err := seedMCPBeads(bd)
	if err != nil {
		t.Fatalf("seedMCPBeads failed: %v", err)
	}
	t.Logf("Seeded %d hook beads and %d MCP beads", hookCreated, mcpCreated)

	// Materialize for a polecat workspace
	workDir := filepath.Join(dir, "polecat-workspace")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Resolve and materialize hooks
	hookLayers, err := bd.ResolveConfigMetadata(
		beads.ConfigCategoryClaudeHooks, "testtown", "", "polecat", "")
	if err != nil {
		t.Fatalf("ResolveConfigMetadata for hooks failed: %v", err)
	}
	if err := claude.MaterializeSettings(workDir, "polecat", hookLayers); err != nil {
		t.Fatalf("MaterializeSettings failed: %v", err)
	}

	// Resolve and materialize MCP
	mcpLayers, err := bd.ResolveConfigMetadata(
		beads.ConfigCategoryMCP, "testtown", "", "", "")
	if err != nil {
		t.Fatalf("ResolveConfigMetadata for MCP failed: %v", err)
	}
	if err := claude.MaterializeMCPConfig(workDir, mcpLayers); err != nil {
		t.Fatalf("MaterializeMCPConfig failed: %v", err)
	}

	// Verify both files exist and are valid JSON
	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	mcpPath := filepath.Join(workDir, ".mcp.json")

	for _, path := range []string{settingsPath, mcpPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("reading %s: %v", filepath.Base(path), err)
			continue
		}
		if !json.Valid(data) {
			t.Errorf("%s contains invalid JSON", filepath.Base(path))
		}
	}

	// Verify settings.json has hooks
	settingsData, _ := os.ReadFile(settingsPath)
	var settings map[string]interface{}
	json.Unmarshal(settingsData, &settings)
	if _, ok := settings["hooks"]; !ok {
		t.Error("materialized settings.json missing 'hooks' key")
	}

	// Verify .mcp.json has mcpServers
	mcpData, _ := os.ReadFile(mcpPath)
	var mcpConfig map[string]interface{}
	json.Unmarshal(mcpData, &mcpConfig)
	if _, ok := mcpConfig["mcpServers"]; !ok {
		t.Error("materialized .mcp.json missing 'mcpServers' key")
	}
}

// --- Edge Cases ---

func TestMaterializeWithEmptyLayers(t *testing.T) {
	workDir := t.TempDir()

	// MaterializeSettings with empty payloads should fall back to embedded template
	err := claude.MaterializeSettings(workDir, "polecat", nil)
	if err != nil {
		t.Fatalf("MaterializeSettings with nil payloads: %v", err)
	}

	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("settings.json not created on fallback: %v", err)
	}
	if !json.Valid(data) {
		t.Error("fallback settings.json is not valid JSON")
	}
}

func TestMaterializeWithEmptyMCPLayers(t *testing.T) {
	workDir := t.TempDir()

	// MaterializeMCPConfig with empty layers should fall back to embedded template
	err := claude.MaterializeMCPConfig(workDir, nil)
	if err != nil {
		t.Fatalf("MaterializeMCPConfig with nil layers: %v", err)
	}

	mcpPath := filepath.Join(workDir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".mcp.json not created on fallback: %v", err)
	}
	if !json.Valid(data) {
		t.Error("fallback .mcp.json is not valid JSON")
	}
}

func TestSeedHookBeadsMetadataIsValidJSON(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed hooks
	if _, _, _, err := seedHookBeads(bd); err != nil {
		t.Fatalf("seedHookBeads failed: %v", err)
	}

	// Get all hook config beads and verify their metadata is valid JSON
	issues, fields, err := bd.ListConfigBeadsForScope(
		beads.ConfigCategoryClaudeHooks, "testtown", "", "", "")
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope failed: %v", err)
	}

	if len(issues) == 0 {
		t.Fatal("expected at least 1 hook config bead")
	}

	for i, f := range fields {
		if f.Metadata == "" {
			t.Errorf("bead %s has empty metadata", issues[i].ID)
			continue
		}
		if !json.Valid([]byte(f.Metadata)) {
			t.Errorf("bead %s has invalid JSON metadata: %s", issues[i].ID, f.Metadata)
		}

		// Verify it parses to a map with expected structure
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(f.Metadata), &meta); err != nil {
			t.Errorf("bead %s metadata unmarshal failed: %v", issues[i].ID, err)
		}
	}
}

func TestSeedMCPBeadsMetadataIsValidJSON(t *testing.T) {
	dir := setupPipelineTestTown(t)
	bd := setupPipelineBeads(t, dir)

	// Seed MCP
	if _, _, _, err := seedMCPBeads(bd); err != nil {
		t.Fatalf("seedMCPBeads failed: %v", err)
	}

	// Verify metadata
	_, origFields, err := bd.GetConfigBead(beads.ConfigBeadID("mcp-global"))
	if err != nil {
		t.Fatalf("GetConfigBead failed: %v", err)
	}
	if origFields.Metadata == "" {
		t.Fatal("MCP bead has empty metadata")
	}
	if !json.Valid([]byte(origFields.Metadata)) {
		t.Fatalf("MCP bead metadata is invalid JSON: %s", origFields.Metadata)
	}

	// Should contain mcpServers
	if !strings.Contains(origFields.Metadata, "mcpServers") {
		t.Error("MCP metadata should contain mcpServers")
	}
}
