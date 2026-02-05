//go:build integration

package configbeads

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// ─── Role Definition Tests ──────────────────────────────────────────────

// TestLoadRoleDefinitionFromBeads_SingleGlobal tests loading a single
// global-scope role-definition bead for a valid role.
func TestLoadRoleDefinitionFromBeads_SingleGlobal(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	metadata := map[string]interface{}{
		"role":  "crew",
		"scope": "rig",
		"session": map[string]interface{}{
			"pattern":  "gt11-gastown-crew-*",
			"work_dir": "{{rig_path}}",
		},
		"health": map[string]interface{}{
			"ping_timeout":         "30s",
			"consecutive_failures": 3,
			"kill_cooldown":        "5m",
			"stuck_threshold":      "4h",
		},
		"nudge": "You are crew member.",
	}
	metaJSON, _ := json.Marshal(metadata)

	_, err := bd.CreateConfigBead("role-crew", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(metaJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	def, err := LoadRoleDefinitionFromBeads(bd, "testtown", "testrepo", "crew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil RoleDefinition")
	}
	if def.Role != "crew" {
		t.Errorf("Role = %q, want %q", def.Role, "crew")
	}
	if def.Scope != "rig" {
		t.Errorf("Scope = %q, want %q", def.Scope, "rig")
	}
	if def.Nudge != "You are crew member." {
		t.Errorf("Nudge = %q, want %q", def.Nudge, "You are crew member.")
	}
	if def.Session.Pattern != "gt11-gastown-crew-*" {
		t.Errorf("Session.Pattern = %q, want %q", def.Session.Pattern, "gt11-gastown-crew-*")
	}
}

// TestLoadRoleDefinitionFromBeads_ThreeLayerMerge tests that global, town,
// and rig layers are merged in correct specificity order, with more-specific
// values overriding less-specific ones while preserving non-overlapping keys.
func TestLoadRoleDefinitionFromBeads_ThreeLayerMerge(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Layer 1: global (least specific)
	globalMeta := map[string]interface{}{
		"role":  "witness",
		"scope": "rig",
		"health": map[string]interface{}{
			"ping_timeout":         "30s",
			"consecutive_failures": 3,
			"kill_cooldown":        "5m",
			"stuck_threshold":      "4h",
		},
		"nudge": "Global nudge.",
	}
	globalJSON, _ := json.Marshal(globalMeta)

	_, err := bd.CreateConfigBead("role-witness", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(globalJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	// Layer 2: town-scoped (medium specific)
	townMeta := map[string]interface{}{
		"role": "witness",
		"health": map[string]interface{}{
			"stuck_threshold": "6h", // Override global 4h
		},
		"nudge": "Town-level nudge.", // Override global nudge
	}
	townJSON, _ := json.Marshal(townMeta)

	_, err = bd.CreateConfigBead("role-witness-testtown", &beads.ConfigFields{
		Rig:      "testtown",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(townJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	// Layer 3: rig-scoped (most specific)
	rigMeta := map[string]interface{}{
		"role":  "witness",
		"nudge": "Rig-specific nudge.", // Override town nudge
	}
	rigJSON, _ := json.Marshal(rigMeta)

	_, err = bd.CreateConfigBead("role-witness-testtown-testrepo", &beads.ConfigFields{
		Rig:      "testtown/testrepo",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(rigJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	def, err := LoadRoleDefinitionFromBeads(bd, "testtown", "testrepo", "witness")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil RoleDefinition")
	}

	// Rig layer should win for nudge
	if def.Nudge != "Rig-specific nudge." {
		t.Errorf("Nudge = %q, want %q (rig override)", def.Nudge, "Rig-specific nudge.")
	}

	// Role should be preserved from global
	if def.Role != "witness" {
		t.Errorf("Role = %q, want %q", def.Role, "witness")
	}

	// Scope from global should be preserved (not overridden)
	if def.Scope != "rig" {
		t.Errorf("Scope = %q, want %q", def.Scope, "rig")
	}
}

// TestLoadRoleDefinitionFromBeads_NoBeads tests that querying with no
// role-definition beads returns nil, nil (not an error).
func TestLoadRoleDefinitionFromBeads_NoBeads(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	def, err := LoadRoleDefinitionFromBeads(bd, "testtown", "testrepo", "crew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def != nil {
		t.Error("expected nil when no beads exist")
	}
}

// TestLoadRoleDefinitionFromBeads_WrongRoleFiltered tests that beads for a
// different role are filtered out when loading a specific role.
func TestLoadRoleDefinitionFromBeads_WrongRoleFiltered(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Create a role-definition bead for "witness"
	witnessMeta := map[string]interface{}{
		"role":  "witness",
		"scope": "rig",
		"nudge": "Witness nudge.",
	}
	witnessJSON, _ := json.Marshal(witnessMeta)

	_, err := bd.CreateConfigBead("role-witness", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(witnessJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	// Query for "crew" — the witness bead should be filtered out
	def, err := LoadRoleDefinitionFromBeads(bd, "testtown", "testrepo", "crew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def != nil {
		t.Error("expected nil when only beads for wrong role exist")
	}
}

// TestLoadRoleDefinitionFromBeads_InvalidRole tests that an invalid role name
// returns an error rather than silently returning nil.
func TestLoadRoleDefinitionFromBeads_InvalidRole(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	_, err := LoadRoleDefinitionFromBeads(bd, "testtown", "testrepo", "invalid-role")
	if err == nil {
		t.Fatal("expected error for invalid role name")
	}
}

// TestLoadRoleDefinition_TOMLFallback tests that LoadRoleDefinition falls back
// to TOML-based loading when no beads exist for the given role.
func TestLoadRoleDefinition_TOMLFallback(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// No role beads exist — should fall back to TOML (built-in defaults)
	def, err := LoadRoleDefinition(bd, dir, "testtown", "", "testrepo", "crew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil RoleDefinition from TOML fallback")
	}
	if def.Role != "crew" {
		t.Errorf("Role = %q, want %q (from built-in defaults)", def.Role, "crew")
	}
}

// ─── Escalation Config Tests ────────────────────────────────────────────

// TestLoadEscalationConfigFromBeads_Found tests loading an escalation
// config bead with valid metadata.
func TestLoadEscalationConfigFromBeads_Found(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	metadata := map[string]interface{}{
		"type":    "escalation",
		"version": 1,
		"routes": map[string]interface{}{
			"HIGH":     []string{"bead", "mail:mayor", "slack"},
			"CRITICAL": []string{"bead", "mail:mayor", "email:human", "sms:human"},
		},
		"contacts": map[string]interface{}{
			"human_email":   "admin@example.com",
			"slack_webhook": "https://hooks.slack.com/test",
		},
		"stale_threshold":  "4h",
		"max_reescalations": 3,
	}
	metaJSON, _ := json.Marshal(metadata)

	_, err := bd.CreateConfigBead("escalation-testtown", &beads.ConfigFields{
		Rig:      "testtown",
		Category: beads.ConfigCategoryEscalation,
		Metadata: string(metaJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	cfg, err := LoadEscalationConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil EscalationConfig")
	}
	if cfg.Type != "escalation" {
		t.Errorf("Type = %q, want %q", cfg.Type, "escalation")
	}
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want %d", cfg.Version, 1)
	}
	if len(cfg.Routes) != 2 {
		t.Errorf("Routes count = %d, want 2", len(cfg.Routes))
	}
	if cfg.Routes["HIGH"][0] != "bead" {
		t.Errorf("Routes[HIGH][0] = %q, want %q", cfg.Routes["HIGH"][0], "bead")
	}
	if cfg.Contacts.HumanEmail != "admin@example.com" {
		t.Errorf("Contacts.HumanEmail = %q, want %q", cfg.Contacts.HumanEmail, "admin@example.com")
	}
	if cfg.Contacts.SlackWebhook != "https://hooks.slack.com/test" {
		t.Errorf("Contacts.SlackWebhook = %q, want %q", cfg.Contacts.SlackWebhook, "https://hooks.slack.com/test")
	}
	if cfg.StaleThreshold != "4h" {
		t.Errorf("StaleThreshold = %q, want %q", cfg.StaleThreshold, "4h")
	}
	if cfg.MaxReescalations != 3 {
		t.Errorf("MaxReescalations = %d, want %d", cfg.MaxReescalations, 3)
	}
}

// TestLoadEscalationConfigFromBeads_NotFound tests that loading escalation
// config when no beads exist returns nil, nil.
func TestLoadEscalationConfigFromBeads_NotFound(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	cfg, err := LoadEscalationConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil when no escalation beads exist")
	}
}

// TestLoadEscalationConfig_Fallback tests that LoadEscalationConfig falls back
// to filesystem-based loading when no beads exist.
func TestLoadEscalationConfig_Fallback(t *testing.T) {
	dir := setupTestTown(t)

	// Create the settings directory with an escalation config file
	settingsDir := filepath.Join(dir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	escalationCfg := &config.EscalationConfig{
		Type:    "escalation",
		Version: 1,
		Routes: map[string][]string{
			"high": {"bead", "mail:mayor"},
		},
		Contacts: config.EscalationContacts{
			HumanEmail: "fallback@example.com",
		},
	}
	cfgJSON, _ := json.Marshal(escalationCfg)
	if err := os.WriteFile(filepath.Join(settingsDir, "escalation.json"), cfgJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// LoadEscalationConfig creates its own beads instance; no beads exist → fallback
	cfg, err := LoadEscalationConfig(dir, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil EscalationConfig from fallback")
	}
	if cfg.Contacts.HumanEmail != "fallback@example.com" {
		t.Errorf("Contacts.HumanEmail = %q, want %q (from filesystem fallback)", cfg.Contacts.HumanEmail, "fallback@example.com")
	}
}

// TestLoadEscalationConfig_FallbackDefault tests that LoadEscalationConfig
// returns a default config when neither beads nor filesystem config exists.
func TestLoadEscalationConfig_FallbackDefault(t *testing.T) {
	dir := setupTestTown(t)

	// No escalation.json exists, should return default config
	cfg, err := LoadEscalationConfig(dir, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil default EscalationConfig")
	}
}

// ─── Messaging Config Tests ─────────────────────────────────────────────

// TestLoadMessagingConfigFromBeads_Found tests loading a messaging
// config bead with valid metadata.
func TestLoadMessagingConfigFromBeads_Found(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	metadata := map[string]interface{}{
		"type":    "messaging",
		"version": 1,
		"lists": map[string]interface{}{
			"oncall": []string{"mayor/", "gastown/witness"},
		},
		"queues": map[string]interface{}{
			"work/gastown": map[string]interface{}{
				"workers":    []string{"gastown/polecats/*"},
				"max_claims": 1,
			},
		},
		"announces": map[string]interface{}{
			"alerts": map[string]interface{}{
				"readers":      []string{"@town"},
				"retain_count": 10,
			},
		},
		"nudge_channels": map[string]interface{}{
			"urgent": []string{"mayor/", "gastown/witness"},
		},
	}
	metaJSON, _ := json.Marshal(metadata)

	_, err := bd.CreateConfigBead("messaging-testtown", &beads.ConfigFields{
		Rig:      "testtown",
		Category: beads.ConfigCategoryMessaging,
		Metadata: string(metaJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	cfg, err := LoadMessagingConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil MessagingConfig")
	}
	if cfg.Type != "messaging" {
		t.Errorf("Type = %q, want %q", cfg.Type, "messaging")
	}
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want %d", cfg.Version, 1)
	}

	// Verify lists
	if len(cfg.Lists) != 1 {
		t.Errorf("Lists count = %d, want 1", len(cfg.Lists))
	}
	oncall := cfg.Lists["oncall"]
	if len(oncall) != 2 {
		t.Errorf("Lists[oncall] len = %d, want 2", len(oncall))
	} else if oncall[0] != "mayor/" {
		t.Errorf("Lists[oncall][0] = %q, want %q", oncall[0], "mayor/")
	}

	// Verify queues
	if len(cfg.Queues) != 1 {
		t.Errorf("Queues count = %d, want 1", len(cfg.Queues))
	}
	workQ := cfg.Queues["work/gastown"]
	if len(workQ.Workers) != 1 || workQ.Workers[0] != "gastown/polecats/*" {
		t.Errorf("Queues[work/gastown].Workers = %v, want [gastown/polecats/*]", workQ.Workers)
	}
	if workQ.MaxClaims != 1 {
		t.Errorf("Queues[work/gastown].MaxClaims = %d, want 1", workQ.MaxClaims)
	}

	// Verify announces
	if len(cfg.Announces) != 1 {
		t.Errorf("Announces count = %d, want 1", len(cfg.Announces))
	}
	alerts := cfg.Announces["alerts"]
	if len(alerts.Readers) != 1 || alerts.Readers[0] != "@town" {
		t.Errorf("Announces[alerts].Readers = %v, want [@town]", alerts.Readers)
	}
	if alerts.RetainCount != 10 {
		t.Errorf("Announces[alerts].RetainCount = %d, want 10", alerts.RetainCount)
	}

	// Verify nudge channels
	if len(cfg.NudgeChannels) != 1 {
		t.Errorf("NudgeChannels count = %d, want 1", len(cfg.NudgeChannels))
	}
	urgent := cfg.NudgeChannels["urgent"]
	if len(urgent) != 2 {
		t.Errorf("NudgeChannels[urgent] len = %d, want 2", len(urgent))
	}
}

// TestLoadMessagingConfigFromBeads_NotFound tests that loading messaging
// config when no beads exist returns nil, nil.
func TestLoadMessagingConfigFromBeads_NotFound(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	cfg, err := LoadMessagingConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil when no messaging beads exist")
	}
}

// TestLoadMessagingConfig_Fallback tests that LoadMessagingConfig falls back
// to filesystem-based loading when no beads exist.
func TestLoadMessagingConfig_Fallback(t *testing.T) {
	dir := setupTestTown(t)

	// Create the config directory with a messaging config file
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	msgCfg := &config.MessagingConfig{
		Type:    "messaging",
		Version: 1,
		Lists: map[string][]string{
			"fallback-list": {"agent1", "agent2"},
		},
	}
	cfgJSON, _ := json.Marshal(msgCfg)
	if err := os.WriteFile(filepath.Join(configDir, "messaging.json"), cfgJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// LoadMessagingConfig creates its own beads instance; no beads exist → fallback
	cfg, err := LoadMessagingConfig(dir, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil MessagingConfig from fallback")
	}
	if len(cfg.Lists["fallback-list"]) != 2 {
		t.Errorf("Lists[fallback-list] len = %d, want 2 (from filesystem fallback)", len(cfg.Lists["fallback-list"]))
	}
}

// TestLoadMessagingConfig_FallbackDefault tests that LoadMessagingConfig
// returns a default config when neither beads nor filesystem config exists.
func TestLoadMessagingConfig_FallbackDefault(t *testing.T) {
	dir := setupTestTown(t)

	// No messaging.json exists, should return default config
	cfg, err := LoadMessagingConfig(dir, "testtown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil default MessagingConfig")
	}
}
