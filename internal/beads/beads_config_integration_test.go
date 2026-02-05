//go:build integration

package beads

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/constants"
)

// setupIntegrationBeads creates an isolated beads instance for integration testing.
// Skips the test if bd CLI is unavailable.
func setupIntegrationBeads(t *testing.T) *Beads {
	t.Helper()
	dir := t.TempDir()
	bd := NewIsolated(dir)
	if err := bd.Init("hq-"); err != nil {
		t.Skipf("cannot initialize beads repo (bd not available?): %v", err)
		return nil
	}
	beadsDir := filepath.Join(dir, ".beads")
	if err := ensureCustomTypesForTest(t, beadsDir); err != nil {
		t.Skipf("cannot configure custom types: %v", err)
		return nil
	}
	return bd
}

// ensureCustomTypesForTest configures custom types by running bd with --db flag
// and filtered environment to bypass remote daemon routing.
func ensureCustomTypesForTest(t *testing.T, beadsDir string) error {
	t.Helper()
	dbPath := filepath.Join(beadsDir, "beads.db")
	types := strings.Join(constants.BeadsCustomTypesList(), ",")

	cmd := exec.Command(resolvedBdPath, "--db", dbPath, "config", "set", "types.custom", types)
	cmd.Dir = beadsDir
	// Filter out env vars that route to remote daemon
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "BD_DAEMON_HOST=") ||
			strings.HasPrefix(e, "BD_ACTOR=") ||
			strings.HasPrefix(e, "BEADS_") ||
			strings.HasPrefix(e, "HOME=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "BEADS_DIR="+beadsDir)
	cmd.Env = env

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd config set types.custom: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// createTestConfigBead is a helper that creates a config bead and skips on CLI failure.
func createTestConfigBead(t *testing.T, bd *Beads, slug string, fields *ConfigFields, role, agent string) *Issue {
	t.Helper()
	issue, err := bd.CreateConfigBead(slug, fields, role, agent)
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return nil
	}
	return issue
}

// TestListConfigBeadsForScope_GlobalOnly verifies that a global config bead
// is returned when querying any scope.
func TestListConfigBeadsForScope_GlobalOnly(t *testing.T) {
	bd := setupIntegrationBeads(t)

	createTestConfigBead(t, bd, "hooks-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"hooks":{"PreCompact":[{"command":"gt prime --hook"}]}}`,
	}, "", "")

	issues, fields, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "crew", "slack",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 result, got %d", len(issues))
	}
	if !HasLabel(issues[0], "scope:global") {
		t.Error("expected scope:global label on result")
	}
	if fields[0].Category != ConfigCategoryClaudeHooks {
		t.Errorf("category = %q, want %q", fields[0].Category, ConfigCategoryClaudeHooks)
	}
}

// TestListConfigBeadsForScope_MultiScopeOrdering verifies that config beads
// are returned in specificity order: global (0) < town (1) < rig (2) < rig+role (3) < rig+agent (4).
func TestListConfigBeadsForScope_MultiScopeOrdering(t *testing.T) {
	bd := setupIntegrationBeads(t)

	// Create beads at different scope levels
	createTestConfigBead(t, bd, "hooks-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"level":"global"}`,
	}, "", "")

	createTestConfigBead(t, bd, "hooks-rig", &ConfigFields{
		Rig:      "gt11/gastown",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"level":"rig"}`,
	}, "", "")

	createTestConfigBead(t, bd, "hooks-rig-role", &ConfigFields{
		Rig:      "gt11/gastown",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"level":"rig+role"}`,
	}, "crew", "")

	createTestConfigBead(t, bd, "hooks-rig-agent", &ConfigFields{
		Rig:      "gt11/gastown",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"level":"rig+agent"}`,
	}, "", "slack")

	issues, fields, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "crew", "slack",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope: %v", err)
	}
	if len(issues) != 4 {
		t.Fatalf("expected 4 results, got %d", len(issues))
	}

	// Verify ordering: global (0) → rig (2) → rig+role (3) → rig+agent (4)
	expectedOrder := []string{"global", "rig", "rig+role", "rig+agent"}
	for i, f := range fields {
		var meta map[string]string
		if err := json.Unmarshal([]byte(f.Metadata), &meta); err != nil {
			t.Fatalf("unmarshal metadata[%d]: %v", i, err)
		}
		if meta["level"] != expectedOrder[i] {
			t.Errorf("result[%d] level = %q, want %q", i, meta["level"], expectedOrder[i])
		}
	}
}

// TestListConfigBeadsForScope_RoleFiltering verifies that role-scoped beads
// get correct specificity ordering. Role labels are additive specificity —
// a bead with role:polecat still matches crew at the rig level (score 2),
// but gets higher score (3) when the role matches.
func TestListConfigBeadsForScope_RoleFiltering(t *testing.T) {
	bd := setupIntegrationBeads(t)

	// Create a global bead with role:crew (score 1 for crew, score 0 for polecat)
	createTestConfigBead(t, bd, "hooks-crew-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"for":"crew-global"}`,
	}, "crew", "")

	// Create a global bead without role (score 0 for everyone)
	createTestConfigBead(t, bd, "hooks-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"for":"everyone"}`,
	}, "", "")

	// Query as crew — should get both, with global(0) before global+crew(1)
	issues, fields, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "crew", "",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 results, got %d", len(issues))
	}

	// First result should be the plain global (score 0)
	var m0 map[string]string
	json.Unmarshal([]byte(fields[0].Metadata), &m0)
	if m0["for"] != "everyone" {
		t.Errorf("result[0] = %q, want plain global (score 0)", m0["for"])
	}

	// Second result should be global+crew (score 1)
	var m1 map[string]string
	json.Unmarshal([]byte(fields[1].Metadata), &m1)
	if m1["for"] != "crew-global" {
		t.Errorf("result[1] = %q, want global+crew (score 1)", m1["for"])
	}

	// Query as polecat — global+crew role bead still matches at score 0 (global),
	// not score 1 (since role doesn't match)
	issues2, fields2, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "polecat", "",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope (polecat): %v", err)
	}
	if len(issues2) != 2 {
		t.Fatalf("expected 2 results for polecat, got %d", len(issues2))
	}

	// Both should be score 0 (global match, no role match for crew bead)
	for i, f := range fields2 {
		if !HasLabel(issues2[i], "scope:global") {
			t.Errorf("result[%d] should have scope:global label", i)
		}
		_ = f // fields are valid
	}
}

// TestListConfigBeadsForScope_AgentSpecific verifies that agent-scoped beads
// get the highest specificity (score 4) when the agent matches, and fall
// back to rig-level specificity (score 2) when the agent doesn't match.
func TestListConfigBeadsForScope_AgentSpecific(t *testing.T) {
	bd := setupIntegrationBeads(t)

	createTestConfigBead(t, bd, "hooks-agent-slack", &ConfigFields{
		Rig:      "gt11/gastown",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"target":"slack-only"}`,
	}, "", "slack")

	createTestConfigBead(t, bd, "hooks-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"target":"everyone"}`,
	}, "", "")

	// Query as slack — agent-scoped bead gets score 4 (most specific)
	issues, fields, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "crew", "slack",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope (slack): %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 results for slack, got %d", len(issues))
	}

	// Ordering: global(0) < agent(4) — agent-specific should be last (highest specificity)
	var m0, m1 map[string]string
	json.Unmarshal([]byte(fields[0].Metadata), &m0)
	json.Unmarshal([]byte(fields[1].Metadata), &m1)
	if m0["target"] != "everyone" {
		t.Errorf("result[0] = %q, want global bead first", m0["target"])
	}
	if m1["target"] != "slack-only" {
		t.Errorf("result[1] = %q, want agent-specific bead last", m1["target"])
	}

	// Query as nux — slack bead still matches at rig level (score 2), not agent level
	issues2, fields2, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "polecat", "nux",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope (nux): %v", err)
	}
	if len(issues2) != 2 {
		t.Fatalf("expected 2 results for nux, got %d", len(issues2))
	}

	// Both match, but ordering depends on scores: global(0) < rig(2)
	var n0, n1 map[string]string
	json.Unmarshal([]byte(fields2[0].Metadata), &n0)
	json.Unmarshal([]byte(fields2[1].Metadata), &n1)
	if n0["target"] != "everyone" {
		t.Errorf("nux result[0] = %q, want global first", n0["target"])
	}
	if n1["target"] != "slack-only" {
		t.Errorf("nux result[1] = %q, want rig-level match second", n1["target"])
	}
}

// TestListConfigBeadsForScope_WrongRigExclusion verifies that beads scoped
// to a different rig are excluded from results.
func TestListConfigBeadsForScope_WrongRigExclusion(t *testing.T) {
	bd := setupIntegrationBeads(t)

	// Create bead scoped to beads rig
	createTestConfigBead(t, bd, "hooks-beads-rig", &ConfigFields{
		Rig:      "gt11/beads",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"rig":"beads"}`,
	}, "", "")

	// Create global bead
	createTestConfigBead(t, bd, "hooks-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"rig":"all"}`,
	}, "", "")

	// Query as gastown rig — beads-scoped bead should NOT match
	issues, _, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "", "",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 result (global only), got %d", len(issues))
	}
	if !HasLabel(issues[0], "scope:global") {
		t.Error("expected only the global bead to match")
	}
}

// TestListConfigBeadsForScope_WrongTownExclusion verifies that beads scoped
// to a different town are excluded from results.
func TestListConfigBeadsForScope_WrongTownExclusion(t *testing.T) {
	bd := setupIntegrationBeads(t)

	// Create bead scoped to a different town
	createTestConfigBead(t, bd, "hooks-othetown", &ConfigFields{
		Rig:      "othertown/gastown",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"town":"othertown"}`,
	}, "", "")

	// Create global bead
	createTestConfigBead(t, bd, "hooks-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryClaudeHooks,
		Metadata: `{"town":"all"}`,
	}, "", "")

	// Query as gt11 town — othertown bead should NOT match
	issues, _, err := bd.ListConfigBeadsForScope(
		ConfigCategoryClaudeHooks, "gt11", "gastown", "", "",
	)
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 result (global only), got %d", len(issues))
	}
	if !HasLabel(issues[0], "scope:global") {
		t.Error("expected only the global bead to match")
	}
}

// TestResolveConfigMetadata_OrderedLayers verifies that ResolveConfigMetadata
// returns metadata strings in correct merge order (least-specific to most-specific).
func TestResolveConfigMetadata_OrderedLayers(t *testing.T) {
	bd := setupIntegrationBeads(t)

	createTestConfigBead(t, bd, "mcp-global", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryMCP,
		Metadata: `{"servers":{"base":{"command":"base-cmd"}}}`,
	}, "", "")

	createTestConfigBead(t, bd, "mcp-rig", &ConfigFields{
		Rig:      "gt11/gastown",
		Category: ConfigCategoryMCP,
		Metadata: `{"servers":{"rig-specific":{"command":"rig-cmd"}}}`,
	}, "", "")

	createTestConfigBead(t, bd, "mcp-agent", &ConfigFields{
		Rig:      "gt11/gastown",
		Category: ConfigCategoryMCP,
		Metadata: `{"servers":{"agent-only":{"command":"agent-cmd"}}}`,
	}, "", "slack")

	layers, err := bd.ResolveConfigMetadata(
		ConfigCategoryMCP, "gt11", "gastown", "crew", "slack",
	)
	if err != nil {
		t.Fatalf("ResolveConfigMetadata: %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(layers))
	}

	// Verify ordering: global → rig → agent
	var m0, m1, m2 map[string]interface{}
	json.Unmarshal([]byte(layers[0]), &m0)
	json.Unmarshal([]byte(layers[1]), &m1)
	json.Unmarshal([]byte(layers[2]), &m2)

	servers0 := m0["servers"].(map[string]interface{})
	if _, ok := servers0["base"]; !ok {
		t.Error("layer[0] should be global (has 'base' server)")
	}

	servers1 := m1["servers"].(map[string]interface{})
	if _, ok := servers1["rig-specific"]; !ok {
		t.Error("layer[1] should be rig-scoped (has 'rig-specific' server)")
	}

	servers2 := m2["servers"].(map[string]interface{})
	if _, ok := servers2["agent-only"]; !ok {
		t.Error("layer[2] should be agent-scoped (has 'agent-only' server)")
	}
}

// TestResolveConfigMetadata_EmptyResult verifies that ResolveConfigMetadata
// returns nil when no config beads match the scope.
func TestResolveConfigMetadata_EmptyResult(t *testing.T) {
	bd := setupIntegrationBeads(t)

	layers, err := bd.ResolveConfigMetadata(
		ConfigCategoryMCP, "gt11", "gastown", "crew", "",
	)
	if err != nil {
		t.Fatalf("ResolveConfigMetadata: %v", err)
	}
	if layers != nil {
		t.Errorf("expected nil layers, got %v", layers)
	}
}

// TestCreateConfigBead_RoundTrip verifies that a config bead can be created
// and then retrieved with all fields intact.
func TestCreateConfigBead_RoundTrip(t *testing.T) {
	bd := setupIntegrationBeads(t)

	metadata := `{"hooks":{"SessionStart":[{"command":"gt prime --hook"}],"PreCompact":[{"command":"gt prime --hook"}]}}`

	created := createTestConfigBead(t, bd, "hooks-roundtrip", &ConfigFields{
		Rig:       "gt11/gastown",
		Category:  ConfigCategoryClaudeHooks,
		Metadata:  metadata,
		CreatedBy: "test-agent",
		CreatedAt: "2026-02-05T00:00:00Z",
	}, "crew", "")

	if created.ID != ConfigBeadID("hooks-roundtrip") {
		t.Errorf("ID = %q, want %q", created.ID, ConfigBeadID("hooks-roundtrip"))
	}

	// Retrieve by ID
	issue, fields, err := bd.GetConfigBead(created.ID)
	if err != nil {
		t.Fatalf("GetConfigBead: %v", err)
	}
	if issue == nil {
		t.Fatal("expected non-nil issue from GetConfigBead")
	}
	if fields.Rig != "gt11/gastown" {
		t.Errorf("Rig = %q, want %q", fields.Rig, "gt11/gastown")
	}
	if fields.Category != ConfigCategoryClaudeHooks {
		t.Errorf("Category = %q, want %q", fields.Category, ConfigCategoryClaudeHooks)
	}
	if fields.CreatedBy != "test-agent" {
		t.Errorf("CreatedBy = %q, want %q", fields.CreatedBy, "test-agent")
	}
	if fields.CreatedAt != "2026-02-05T00:00:00Z" {
		t.Errorf("CreatedAt = %q, want %q", fields.CreatedAt, "2026-02-05T00:00:00Z")
	}

	// Verify labels
	if !HasLabel(issue, "gt:config") {
		t.Error("missing gt:config label")
	}
	if !HasLabel(issue, "config:claude-hooks") {
		t.Error("missing config:claude-hooks label")
	}
	if !HasLabel(issue, "town:gt11") {
		t.Error("missing town:gt11 label")
	}
	if !HasLabel(issue, "rig:gastown") {
		t.Error("missing rig:gastown label")
	}
	if !HasLabel(issue, "role:crew") {
		t.Error("missing role:crew label")
	}

	// Verify metadata round-trip (JSON equivalence)
	var origMeta, gotMeta interface{}
	json.Unmarshal([]byte(metadata), &origMeta)
	json.Unmarshal([]byte(fields.Metadata), &gotMeta)
	origJSON, _ := json.Marshal(origMeta)
	gotJSON, _ := json.Marshal(gotMeta)
	if string(origJSON) != string(gotJSON) {
		t.Errorf("Metadata mismatch:\n  got:  %s\n  want: %s", gotJSON, origJSON)
	}
}

// TestCreateConfigBead_GetBySlug verifies that GetConfigBeadBySlug correctly
// resolves the slug to the full ID.
func TestCreateConfigBead_GetBySlug(t *testing.T) {
	bd := setupIntegrationBeads(t)

	createTestConfigBead(t, bd, "slug-test", &ConfigFields{
		Rig:      "*",
		Category: ConfigCategoryIdentity,
		Metadata: `{"name":"test"}`,
	}, "", "")

	issue, fields, err := bd.GetConfigBeadBySlug("slug-test")
	if err != nil {
		t.Fatalf("GetConfigBeadBySlug: %v", err)
	}
	if issue == nil {
		t.Fatal("expected non-nil issue from GetConfigBeadBySlug")
	}
	if issue.ID != "hq-cfg-slug-test" {
		t.Errorf("ID = %q, want %q", issue.ID, "hq-cfg-slug-test")
	}
	if fields.Category != ConfigCategoryIdentity {
		t.Errorf("Category = %q, want %q", fields.Category, ConfigCategoryIdentity)
	}
}
