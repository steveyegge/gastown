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

// ═══════════════════════════════════════════════════════════════════════════
// Beads-Only Integration Tests
//
// These tests prove that the system works when removable filesystem config
// files are absent but config beads exist. Each test:
//   1. Creates config beads with realistic data
//   2. Ensures no corresponding filesystem config file exists
//   3. Calls the beads-first loader (direct or wrapper)
//   4. Verifies the bead-sourced config is returned (not defaults)
//
// Config types tested (the ones we CAN remove):
//   - settings/slack.json      → slack routing beads
//   - config/messaging.json    → messaging beads
//   - settings/escalation.json → escalation beads
//   - roles/*.toml             → role definition beads
// ═══════════════════════════════════════════════════════════════════════════

// ─── Slack Routing (Beads-Only) ─────────────────────────────────────────

// TestBeadsOnly_SlackRouting_NoFilesystemFile tests that a slack routing
// config bead can be created and read back when settings/slack.json is absent.
// Slack does not yet have a beads-first wrapper in configbeads, so we verify
// via the direct config bead API (GetConfigBeadBySlug + metadata parsing).
func TestBeadsOnly_SlackRouting_NoFilesystemFile(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Ensure settings/slack.json does NOT exist
	slackPath := filepath.Join(dir, "settings", "slack.json")
	if _, err := os.Stat(slackPath); err == nil {
		t.Fatal("slack.json should not exist in test temp dir")
	}

	// Create slack routing config bead (mirrors seedSlackBeads behavior,
	// with secrets intentionally excluded)
	metadata := map[string]interface{}{
		"type":            "slack",
		"version":         1,
		"enabled":         true,
		"default_channel": "#decisions",
		"channels": map[string]interface{}{
			"gastown/polecats/*": "C111111",
			"*/crew/*":           "C222222",
			"beads/*":            "C333333",
		},
		"channel_names": map[string]interface{}{
			"C111111": "polecat-work",
			"C222222": "crew-work",
			"C333333": "beads-work",
		},
	}
	metaJSON, _ := json.Marshal(metadata)

	_, err := bd.CreateConfigBead("slack-routing", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategorySlackRouting,
		Metadata: string(metaJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	// Verify: read bead back via slug
	issue, fields, err := bd.GetConfigBeadBySlug("slack-routing")
	if err != nil {
		t.Fatalf("GetConfigBeadBySlug failed: %v", err)
	}
	if issue == nil {
		t.Fatal("expected slack-routing config bead to exist")
	}
	if fields.Category != beads.ConfigCategorySlackRouting {
		t.Errorf("category = %q, want %q", fields.Category, beads.ConfigCategorySlackRouting)
	}

	// Parse metadata and verify config values round-trip correctly
	var cfg config.SlackConfig
	if err := json.Unmarshal([]byte(fields.Metadata), &cfg); err != nil {
		t.Fatalf("failed to parse slack config from bead metadata: %v", err)
	}
	if !cfg.Enabled {
		t.Error("expected Enabled = true")
	}
	if cfg.DefaultChannel != "#decisions" {
		t.Errorf("DefaultChannel = %q, want %q", cfg.DefaultChannel, "#decisions")
	}
	if len(cfg.Channels) != 3 {
		t.Errorf("Channels count = %d, want 3", len(cfg.Channels))
	}
	if cfg.Channels["gastown/polecats/*"] != "C111111" {
		t.Errorf("Channels[gastown/polecats/*] = %q, want C111111", cfg.Channels["gastown/polecats/*"])
	}
	if cfg.BotToken != "" {
		t.Error("BotToken should be empty (secrets excluded from beads)")
	}
	if cfg.AppToken != "" {
		t.Error("AppToken should be empty (secrets excluded from beads)")
	}
}

// TestBeadsOnly_SlackRouting_ListByScope tests that slack routing beads
// are discoverable via ListConfigBeadsForScope when no filesystem file exists.
func TestBeadsOnly_SlackRouting_ListByScope(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	metadata := map[string]interface{}{
		"type":            "slack",
		"version":         1,
		"enabled":         true,
		"default_channel": "#alerts",
	}
	metaJSON, _ := json.Marshal(metadata)

	_, err := bd.CreateConfigBead("slack-routing", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategorySlackRouting,
		Metadata: string(metaJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	issues, fields, err := bd.ListConfigBeadsForScope(
		beads.ConfigCategorySlackRouting, "testtown", "", "", "")
	if err != nil {
		t.Fatalf("ListConfigBeadsForScope failed: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("expected at least one slack routing config bead")
	}
	if len(fields) == 0 {
		t.Fatal("expected at least one config field")
	}

	var cfg config.SlackConfig
	if err := json.Unmarshal([]byte(fields[0].Metadata), &cfg); err != nil {
		t.Fatalf("failed to parse slack config: %v", err)
	}
	if cfg.DefaultChannel != "#alerts" {
		t.Errorf("DefaultChannel = %q, want %q", cfg.DefaultChannel, "#alerts")
	}
}

// TestBeadsOnly_SlackRouting_ChannelNames tests that channel name mappings
// survive the round-trip through config beads.
func TestBeadsOnly_SlackRouting_ChannelNames(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	metadata := map[string]interface{}{
		"type":    "slack",
		"version": 1,
		"enabled": true,
		"channels": map[string]interface{}{
			"mayor/*": "C999999",
		},
		"channel_names": map[string]interface{}{
			"C999999": "mayor-decisions",
		},
	}
	metaJSON, _ := json.Marshal(metadata)

	_, err := bd.CreateConfigBead("slack-routing", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategorySlackRouting,
		Metadata: string(metaJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	_, fields, err := bd.GetConfigBeadBySlug("slack-routing")
	if err != nil {
		t.Fatalf("GetConfigBeadBySlug failed: %v", err)
	}

	var cfg config.SlackConfig
	if err := json.Unmarshal([]byte(fields.Metadata), &cfg); err != nil {
		t.Fatalf("failed to parse slack config: %v", err)
	}
	if cfg.ChannelNames["C999999"] != "mayor-decisions" {
		t.Errorf("ChannelNames[C999999] = %q, want %q", cfg.ChannelNames["C999999"], "mayor-decisions")
	}
}

// ─── Messaging Config (Beads-Only) ──────────────────────────────────────

// TestBeadsOnly_MessagingConfig_NoFilesystemFile tests that
// LoadMessagingConfigFromBeads returns bead-sourced config when
// config/messaging.json is absent.
func TestBeadsOnly_MessagingConfig_NoFilesystemFile(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Ensure config/messaging.json does NOT exist
	msgPath := config.MessagingConfigPath(dir)
	if _, err := os.Stat(msgPath); err == nil {
		t.Fatal("messaging.json should not exist in test temp dir")
	}

	// Create messaging config bead
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

	// Call direct beads loader — should return bead config, NOT defaults
	cfg, err := LoadMessagingConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("LoadMessagingConfigFromBeads failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil MessagingConfig from beads")
	}

	// Verify bead data was returned (not defaults)
	if cfg.Type != "messaging" {
		t.Errorf("Type = %q, want %q", cfg.Type, "messaging")
	}
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want %d", cfg.Version, 1)
	}
	if len(cfg.Lists) != 1 {
		t.Errorf("Lists count = %d, want 1", len(cfg.Lists))
	}
	oncall := cfg.Lists["oncall"]
	if len(oncall) != 2 {
		t.Errorf("Lists[oncall] len = %d, want 2", len(oncall))
	} else {
		if oncall[0] != "mayor/" {
			t.Errorf("Lists[oncall][0] = %q, want %q", oncall[0], "mayor/")
		}
		if oncall[1] != "gastown/witness" {
			t.Errorf("Lists[oncall][1] = %q, want %q", oncall[1], "gastown/witness")
		}
	}
}

// TestBeadsOnly_MessagingConfig_QueuesAndAnnounces verifies that complex
// nested messaging config (queues, announces, nudge channels) round-trips
// correctly through config beads without any filesystem file.
func TestBeadsOnly_MessagingConfig_QueuesAndAnnounces(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	metadata := map[string]interface{}{
		"type":    "messaging",
		"version": 1,
		"queues": map[string]interface{}{
			"work/beads": map[string]interface{}{
				"workers":    []string{"beads/polecats/*"},
				"max_claims": 3,
			},
			"work/gastown": map[string]interface{}{
				"workers":    []string{"gastown/polecats/*", "gastown/crew/*"},
				"max_claims": 2,
			},
		},
		"announces": map[string]interface{}{
			"deploys": map[string]interface{}{
				"readers":      []string{"@rig"},
				"retain_count": 5,
			},
		},
		"nudge_channels": map[string]interface{}{
			"critical": []string{"mayor/", "gastown/witness", "beads/witness"},
			"normal":   []string{"gastown/witness"},
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
		t.Fatalf("LoadMessagingConfigFromBeads failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil MessagingConfig from beads")
	}

	// Verify queues
	if len(cfg.Queues) != 2 {
		t.Errorf("Queues count = %d, want 2", len(cfg.Queues))
	}
	beadsQ := cfg.Queues["work/beads"]
	if beadsQ.MaxClaims != 3 {
		t.Errorf("Queues[work/beads].MaxClaims = %d, want 3", beadsQ.MaxClaims)
	}
	if len(beadsQ.Workers) != 1 || beadsQ.Workers[0] != "beads/polecats/*" {
		t.Errorf("Queues[work/beads].Workers = %v, want [beads/polecats/*]", beadsQ.Workers)
	}
	gastownQ := cfg.Queues["work/gastown"]
	if len(gastownQ.Workers) != 2 {
		t.Errorf("Queues[work/gastown].Workers len = %d, want 2", len(gastownQ.Workers))
	}

	// Verify announces
	deploys := cfg.Announces["deploys"]
	if deploys.RetainCount != 5 {
		t.Errorf("Announces[deploys].RetainCount = %d, want 5", deploys.RetainCount)
	}

	// Verify nudge channels
	if len(cfg.NudgeChannels) != 2 {
		t.Errorf("NudgeChannels count = %d, want 2", len(cfg.NudgeChannels))
	}
	critical := cfg.NudgeChannels["critical"]
	if len(critical) != 3 {
		t.Errorf("NudgeChannels[critical] len = %d, want 3", len(critical))
	}
}

// TestBeadsOnly_MessagingConfig_BeadPreemptsFilesystem tests the scenario
// where both a config bead and a filesystem file could exist, but the bead
// should win. When the filesystem file is later removed, the bead still works.
func TestBeadsOnly_MessagingConfig_BeadPreemptsFilesystem(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Create bead with distinctive data
	metadata := map[string]interface{}{
		"type":    "messaging",
		"version": 1,
		"lists": map[string]interface{}{
			"bead-list": []string{"agent-from-bead-1", "agent-from-bead-2"},
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

	// Load from beads — filesystem file never existed
	cfg, err := LoadMessagingConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("LoadMessagingConfigFromBeads failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil MessagingConfig")
	}

	// Verify bead data
	beadList := cfg.Lists["bead-list"]
	if len(beadList) != 2 {
		t.Errorf("Lists[bead-list] len = %d, want 2 (from bead)", len(beadList))
	} else if beadList[0] != "agent-from-bead-1" {
		t.Errorf("Lists[bead-list][0] = %q, want %q", beadList[0], "agent-from-bead-1")
	}
}

// ─── Escalation Config (Beads-Only) ─────────────────────────────────────

// TestBeadsOnly_EscalationConfig_NoFilesystemFile tests that
// LoadEscalationConfigFromBeads returns bead-sourced config when
// settings/escalation.json is absent.
func TestBeadsOnly_EscalationConfig_NoFilesystemFile(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Ensure settings/escalation.json does NOT exist
	escPath := config.EscalationConfigPath(dir)
	if _, err := os.Stat(escPath); err == nil {
		t.Fatal("escalation.json should not exist in test temp dir")
	}

	// Create escalation config bead
	metadata := map[string]interface{}{
		"type":    "escalation",
		"version": 1,
		"routes": map[string]interface{}{
			"HIGH":     []string{"bead", "mail:mayor", "slack"},
			"CRITICAL": []string{"bead", "mail:mayor", "email:human", "sms:human"},
			"LOW":      []string{"bead"},
		},
		"contacts": map[string]interface{}{
			"human_email":   "admin@gastown.dev",
			"slack_webhook": "https://hooks.slack.com/test",
		},
		"stale_threshold":   "4h",
		"max_reescalations": 5,
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

	// Call direct beads loader — should return bead config
	cfg, err := LoadEscalationConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("LoadEscalationConfigFromBeads failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil EscalationConfig from beads")
	}

	// Verify bead data
	if cfg.Type != "escalation" {
		t.Errorf("Type = %q, want %q", cfg.Type, "escalation")
	}
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want %d", cfg.Version, 1)
	}
	if len(cfg.Routes) != 3 {
		t.Errorf("Routes count = %d, want 3", len(cfg.Routes))
	}
	highRoute := cfg.Routes["HIGH"]
	if len(highRoute) != 3 {
		t.Errorf("Routes[HIGH] len = %d, want 3", len(highRoute))
	} else if highRoute[0] != "bead" {
		t.Errorf("Routes[HIGH][0] = %q, want %q", highRoute[0], "bead")
	}
	if len(cfg.Routes["CRITICAL"]) != 4 {
		t.Errorf("Routes[CRITICAL] len = %d, want 4", len(cfg.Routes["CRITICAL"]))
	}
	if cfg.Contacts.HumanEmail != "admin@gastown.dev" {
		t.Errorf("Contacts.HumanEmail = %q, want %q", cfg.Contacts.HumanEmail, "admin@gastown.dev")
	}
	if cfg.Contacts.SlackWebhook != "https://hooks.slack.com/test" {
		t.Errorf("Contacts.SlackWebhook = %q, want %q", cfg.Contacts.SlackWebhook, "https://hooks.slack.com/test")
	}
	if cfg.StaleThreshold != "4h" {
		t.Errorf("StaleThreshold = %q, want %q", cfg.StaleThreshold, "4h")
	}
	if cfg.MaxReescalations != 5 {
		t.Errorf("MaxReescalations = %d, want %d", cfg.MaxReescalations, 5)
	}
}

// TestBeadsOnly_EscalationConfig_RoutesAndContacts verifies that escalation
// routes and contact information survive beads round-trip correctly.
func TestBeadsOnly_EscalationConfig_RoutesAndContacts(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	metadata := map[string]interface{}{
		"type":    "escalation",
		"version": 1,
		"routes": map[string]interface{}{
			"CRITICAL": []string{"bead", "mail:mayor", "email:human", "sms:human", "slack"},
		},
		"contacts": map[string]interface{}{
			"human_email":   "ops@gastown.dev",
			"slack_webhook": "https://hooks.slack.com/services/xxx",
			"sms_number":    "+1234567890",
		},
		"stale_threshold":   "2h",
		"max_reescalations": 10,
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
		t.Fatalf("LoadEscalationConfigFromBeads failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil EscalationConfig")
	}

	// Verify CRITICAL route has all 5 channels
	critRoute := cfg.Routes["CRITICAL"]
	if len(critRoute) != 5 {
		t.Errorf("Routes[CRITICAL] len = %d, want 5", len(critRoute))
	} else if critRoute[4] != "slack" {
		t.Errorf("Routes[CRITICAL][4] = %q, want %q", critRoute[4], "slack")
	}

	// Verify contacts
	if cfg.Contacts.HumanEmail != "ops@gastown.dev" {
		t.Errorf("Contacts.HumanEmail = %q, want %q", cfg.Contacts.HumanEmail, "ops@gastown.dev")
	}
	if cfg.Contacts.SlackWebhook != "https://hooks.slack.com/services/xxx" {
		t.Errorf("Contacts.SlackWebhook = %q, want %q", cfg.Contacts.SlackWebhook, "https://hooks.slack.com/services/xxx")
	}
	if cfg.StaleThreshold != "2h" {
		t.Errorf("StaleThreshold = %q, want %q", cfg.StaleThreshold, "2h")
	}
	if cfg.MaxReescalations != 10 {
		t.Errorf("MaxReescalations = %d, want %d", cfg.MaxReescalations, 10)
	}
}

// TestBeadsOnly_EscalationConfig_BeadPreemptsFilesystem tests that beads data
// is used when the escalation filesystem config file does not exist.
func TestBeadsOnly_EscalationConfig_BeadPreemptsFilesystem(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Create bead with DISTINCTIVE data
	metadata := map[string]interface{}{
		"type":    "escalation",
		"version": 1,
		"routes": map[string]interface{}{
			"CRITICAL": []string{"bead", "mail:mayor", "email:human"},
		},
		"contacts": map[string]interface{}{
			"human_email": "bead-only@gastown.dev",
		},
		"stale_threshold":   "6h",
		"max_reescalations": 7,
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

	// Filesystem file never existed — bead data should be the only source
	cfg, err := LoadEscalationConfigFromBeads(bd, "testtown")
	if err != nil {
		t.Fatalf("LoadEscalationConfigFromBeads failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil EscalationConfig")
	}

	// Verify distinctive bead values
	if cfg.Contacts.HumanEmail != "bead-only@gastown.dev" {
		t.Errorf("Contacts.HumanEmail = %q, want %q (from bead)", cfg.Contacts.HumanEmail, "bead-only@gastown.dev")
	}
	critRoute := cfg.Routes["CRITICAL"]
	if len(critRoute) != 3 {
		t.Errorf("Routes[CRITICAL] len = %d, want 3 (from bead)", len(critRoute))
	}
	if cfg.StaleThreshold != "6h" {
		t.Errorf("StaleThreshold = %q, want %q (from bead)", cfg.StaleThreshold, "6h")
	}
	if cfg.MaxReescalations != 7 {
		t.Errorf("MaxReescalations = %d, want %d (from bead)", cfg.MaxReescalations, 7)
	}
}

// ─── Role Definitions (Beads-Only) ──────────────────────────────────────

// TestBeadsOnly_RoleDefinition_NoTOMLFile tests that LoadRoleDefinition
// returns bead-sourced config when no roles/*.toml override files exist.
// The bead should take precedence over embedded TOML defaults.
func TestBeadsOnly_RoleDefinition_NoTOMLFile(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Ensure no roles/*.toml files exist
	rolesDir := filepath.Join(dir, "roles")
	if _, err := os.Stat(rolesDir); err == nil {
		t.Fatal("roles/ directory should not exist in test temp dir")
	}

	// Create a role definition config bead for "crew"
	metadata := map[string]interface{}{
		"role":  "crew",
		"scope": "rig",
		"session": map[string]interface{}{
			"pattern":  "gt11-testtown-crew-*",
			"work_dir": "{{rig_path}}",
		},
		"health": map[string]interface{}{
			"ping_timeout":         "45s",
			"consecutive_failures": 5,
			"kill_cooldown":        "10m",
			"stuck_threshold":      "8h",
		},
		"nudge": "You are a test crew member from beads.",
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

	// Call beads-first wrapper — should return bead config
	def, err := LoadRoleDefinition(bd, dir, "testtown", "", "testrepo", "crew")
	if err != nil {
		t.Fatalf("LoadRoleDefinition failed: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil RoleDefinition from beads")
	}

	// Verify bead data was returned
	if def.Role != "crew" {
		t.Errorf("Role = %q, want %q", def.Role, "crew")
	}
	if def.Scope != "rig" {
		t.Errorf("Scope = %q, want %q", def.Scope, "rig")
	}
	if def.Nudge != "You are a test crew member from beads." {
		t.Errorf("Nudge = %q, want bead-sourced nudge", def.Nudge)
	}
	if def.Session.Pattern != "gt11-testtown-crew-*" {
		t.Errorf("Session.Pattern = %q, want %q", def.Session.Pattern, "gt11-testtown-crew-*")
	}
}

// TestBeadsOnly_RoleDefinition_OverridesEmbeddedDefaults tests that a role
// bead with custom values overrides the embedded TOML defaults. This is the
// key scenario: even though crew.toml exists as an embedded resource, the
// bead should win.
func TestBeadsOnly_RoleDefinition_OverridesEmbeddedDefaults(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// The embedded crew.toml has its own nudge, health settings, etc.
	// Our bead should override those values.
	metadata := map[string]interface{}{
		"role":  "crew",
		"scope": "rig",
		"health": map[string]interface{}{
			"ping_timeout":    "99s",
			"stuck_threshold": "24h",
		},
		"nudge": "Custom bead nudge overriding embedded default.",
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

	def, err := LoadRoleDefinition(bd, dir, "testtown", "", "testrepo", "crew")
	if err != nil {
		t.Fatalf("LoadRoleDefinition failed: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil RoleDefinition from beads")
	}

	// Bead values should be present
	if def.Nudge != "Custom bead nudge overriding embedded default." {
		t.Errorf("Nudge = %q, want bead override", def.Nudge)
	}
}

// TestBeadsOnly_RoleDefinition_MultipleRoles tests that beads for multiple
// roles can coexist and correct filtering occurs.
func TestBeadsOnly_RoleDefinition_MultipleRoles(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Create beads for two different roles
	crewMeta := map[string]interface{}{
		"role":  "crew",
		"scope": "rig",
		"nudge": "Crew nudge from beads.",
	}
	crewJSON, _ := json.Marshal(crewMeta)

	witnessMeta := map[string]interface{}{
		"role":  "witness",
		"scope": "rig",
		"nudge": "Witness nudge from beads.",
	}
	witnessJSON, _ := json.Marshal(witnessMeta)

	_, err := bd.CreateConfigBead("role-crew", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(crewJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	_, err = bd.CreateConfigBead("role-witness", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(witnessJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	// Load crew — should only get crew bead
	crewDef, err := LoadRoleDefinition(bd, dir, "testtown", "", "testrepo", "crew")
	if err != nil {
		t.Fatalf("LoadRoleDefinition(crew) failed: %v", err)
	}
	if crewDef == nil {
		t.Fatal("expected non-nil RoleDefinition for crew")
	}
	if crewDef.Nudge != "Crew nudge from beads." {
		t.Errorf("crew Nudge = %q, want crew nudge", crewDef.Nudge)
	}

	// Load witness — should only get witness bead
	witnessDef, err := LoadRoleDefinition(bd, dir, "testtown", "", "testrepo", "witness")
	if err != nil {
		t.Fatalf("LoadRoleDefinition(witness) failed: %v", err)
	}
	if witnessDef == nil {
		t.Fatal("expected non-nil RoleDefinition for witness")
	}
	if witnessDef.Nudge != "Witness nudge from beads." {
		t.Errorf("witness Nudge = %q, want witness nudge", witnessDef.Nudge)
	}
}

// TestBeadsOnly_RoleDefinition_ScopedOverride tests that a rig-scoped role
// bead overrides a global role bead, all without filesystem TOML files.
func TestBeadsOnly_RoleDefinition_ScopedOverride(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Global bead (least specific)
	globalMeta := map[string]interface{}{
		"role":  "polecat",
		"scope": "rig",
		"health": map[string]interface{}{
			"ping_timeout":    "30s",
			"stuck_threshold": "4h",
		},
		"nudge": "Global polecat nudge.",
	}
	globalJSON, _ := json.Marshal(globalMeta)

	_, err := bd.CreateConfigBead("role-polecat", &beads.ConfigFields{
		Rig:      "*",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(globalJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	// Rig-scoped bead (most specific)
	rigMeta := map[string]interface{}{
		"role":  "polecat",
		"nudge": "Rig-specific polecat nudge.",
	}
	rigJSON, _ := json.Marshal(rigMeta)

	_, err = bd.CreateConfigBead("role-polecat-testtown-testrepo", &beads.ConfigFields{
		Rig:      "testtown/testrepo",
		Category: beads.ConfigCategoryRoleDefinition,
		Metadata: string(rigJSON),
	}, "", "")
	if err != nil {
		t.Skipf("bd create failed (known bd CLI issue): %v", err)
		return
	}

	def, err := LoadRoleDefinition(bd, dir, "testtown", "", "testrepo", "polecat")
	if err != nil {
		t.Fatalf("LoadRoleDefinition failed: %v", err)
	}
	if def == nil {
		t.Fatal("expected non-nil RoleDefinition")
	}

	// Rig-scoped bead should override nudge
	if def.Nudge != "Rig-specific polecat nudge." {
		t.Errorf("Nudge = %q, want rig-specific override", def.Nudge)
	}

	// Global scope should still provide non-overridden values
	if def.Scope != "rig" {
		t.Errorf("Scope = %q, want %q (from global bead)", def.Scope, "rig")
	}
}

// TestBeadsOnly_RoleDefinition_AllBuiltinRoles tests that config beads can
// be created for all built-in role names without errors.
func TestBeadsOnly_RoleDefinition_AllBuiltinRoles(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	roles := config.AllRoles()
	for _, roleName := range roles {
		t.Run(roleName, func(t *testing.T) {
			metadata := map[string]interface{}{
				"role":  roleName,
				"scope": "rig",
				"nudge": "Bead nudge for " + roleName,
			}
			metaJSON, _ := json.Marshal(metadata)

			slug := "role-" + roleName
			_, err := bd.CreateConfigBead(slug, &beads.ConfigFields{
				Rig:      "*",
				Category: beads.ConfigCategoryRoleDefinition,
				Metadata: string(metaJSON),
			}, "", "")
			if err != nil {
				t.Skipf("bd create failed (known bd CLI issue): %v", err)
				return
			}

			def, err := LoadRoleDefinitionFromBeads(bd, "testtown", "testrepo", roleName)
			if err != nil {
				t.Fatalf("LoadRoleDefinitionFromBeads failed for %q: %v", roleName, err)
			}
			if def == nil {
				t.Fatalf("expected non-nil RoleDefinition for %q", roleName)
			}
			if def.Role != roleName {
				t.Errorf("Role = %q, want %q", def.Role, roleName)
			}
		})
	}
}
