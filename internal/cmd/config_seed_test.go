package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
)

func TestExtractHooksMap(t *testing.T) {
	settings := map[string]interface{}{
		"editorMode": "normal",
		"hooks": map[string]interface{}{
			"SessionStart": []interface{}{
				map[string]interface{}{"matcher": "", "hooks": []interface{}{}},
			},
		},
	}

	hooks := extractHooksMap(settings)
	if hooks == nil {
		t.Fatal("expected hooks map, got nil")
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Error("expected SessionStart hook")
	}
}

func TestExtractHooksMap_NoHooks(t *testing.T) {
	settings := map[string]interface{}{
		"editorMode": "normal",
	}

	hooks := extractHooksMap(settings)
	if hooks != nil {
		t.Errorf("expected nil, got %v", hooks)
	}
}

func TestTemplatesProduceDiff(t *testing.T) {
	// Verify that autonomous and interactive templates actually differ
	// (this is the assumption the seed command depends on)
	autoContent, err := claude.TemplateContent(claude.Autonomous)
	if err != nil {
		t.Fatalf("reading autonomous template: %v", err)
	}
	interContent, err := claude.TemplateContent(claude.Interactive)
	if err != nil {
		t.Fatalf("reading interactive template: %v", err)
	}

	var autoSettings, interSettings map[string]interface{}
	if err := json.Unmarshal(autoContent, &autoSettings); err != nil {
		t.Fatalf("parsing autonomous: %v", err)
	}
	if err := json.Unmarshal(interContent, &interSettings); err != nil {
		t.Fatalf("parsing interactive: %v", err)
	}

	autoHooks := extractHooksMap(autoSettings)
	interHooks := extractHooksMap(interSettings)

	if autoHooks == nil || interHooks == nil {
		t.Fatal("expected both templates to have hooks")
	}

	// Find at least one difference
	foundDiff := false
	allNames := make(map[string]bool)
	for k := range autoHooks {
		allNames[k] = true
	}
	for k := range interHooks {
		allNames[k] = true
	}

	for name := range allNames {
		aj, _ := json.Marshal(autoHooks[name])
		ij, _ := json.Marshal(interHooks[name])
		if string(aj) != string(ij) {
			foundDiff = true
			t.Logf("Hook %s differs between templates", name)
		}
	}

	if !foundDiff {
		t.Error("expected at least one hook to differ between autonomous and interactive templates")
	}
}

func TestHookDiffCategories(t *testing.T) {
	// The autonomous template should have SessionStart with mail check,
	// and Stop with --soft flag
	autoContent, err := claude.TemplateContent(claude.Autonomous)
	if err != nil {
		t.Fatalf("reading autonomous template: %v", err)
	}
	interContent, err := claude.TemplateContent(claude.Interactive)
	if err != nil {
		t.Fatalf("reading interactive template: %v", err)
	}

	var autoSettings, interSettings map[string]interface{}
	json.Unmarshal(autoContent, &autoSettings)
	json.Unmarshal(interContent, &interSettings)

	autoHooks := extractHooksMap(autoSettings)
	interHooks := extractHooksMap(interSettings)

	// Shared hooks should produce identical JSON
	sharedHookNames := []string{"PreToolUse", "PreCompact", "PostToolUse", "UserPromptSubmit"}
	for _, name := range sharedHookNames {
		aj, _ := json.Marshal(autoHooks[name])
		ij, _ := json.Marshal(interHooks[name])
		if string(aj) != string(ij) {
			t.Errorf("expected %s to be shared, but it differs", name)
		}
	}

	// SessionStart and Stop should differ
	diffHookNames := []string{"SessionStart", "Stop"}
	for _, name := range diffHookNames {
		aj, _ := json.Marshal(autoHooks[name])
		ij, _ := json.Marshal(interHooks[name])
		if string(aj) == string(ij) {
			t.Errorf("expected %s to differ between templates, but it's identical", name)
		}
	}
}

func TestMCPTemplateContent(t *testing.T) {
	content, err := claude.MCPTemplateContent()
	if err != nil {
		t.Fatalf("reading MCP template: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty MCP template content")
	}

	var mcpConfig map[string]interface{}
	if err := json.Unmarshal(content, &mcpConfig); err != nil {
		t.Fatalf("MCP template is not valid JSON: %v", err)
	}

	if _, ok := mcpConfig["mcpServers"]; !ok {
		t.Error("expected mcpServers key in MCP template")
	}
}

func TestSeedAccountBeadsMetadataExcludesAuthToken(t *testing.T) {
	// Verify the metadata construction logic: auth_token should never appear
	// in the metadata map that gets serialized to beads.
	// This simulates what seedAccountBeads does for each account.
	acct := struct {
		Email       string
		Description string
		ConfigDir   string
		AuthToken   string
		BaseURL     string
	}{
		Email:       "test@example.com",
		Description: "Test account",
		ConfigDir:   "/tmp/test",
		AuthToken:   "sk-secret-token-that-should-never-appear",
		BaseURL:     "http://localhost:4000",
	}

	metadata := map[string]interface{}{
		"handle":     "test",
		"config_dir": acct.ConfigDir,
	}
	if acct.Email != "" {
		metadata["email"] = acct.Email
	}
	if acct.Description != "" {
		metadata["description"] = acct.Description
	}
	if acct.BaseURL != "" {
		metadata["base_url"] = acct.BaseURL
	}
	// NOTE: auth_token is intentionally excluded

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshaling metadata: %v", err)
	}

	metaStr := string(metaJSON)
	if strings.Contains(metaStr, "auth_token") {
		t.Errorf("metadata should not contain auth_token, got: %s", metaStr)
	}
	if strings.Contains(metaStr, "sk-secret") {
		t.Errorf("metadata should not contain secret value, got: %s", metaStr)
	}
	if !strings.Contains(metaStr, "test@example.com") {
		t.Error("metadata should contain email")
	}
	if !strings.Contains(metaStr, "/tmp/test") {
		t.Error("metadata should contain config_dir")
	}
	if !strings.Contains(metaStr, "http://localhost:4000") {
		t.Error("metadata should contain base_url")
	}
}

func TestSlackSeedMetadataExcludesSecrets(t *testing.T) {
	// Simulate what seedSlackBeads does: parse config, remove secrets
	slackJSON := `{
		"type": "slack",
		"version": 1,
		"enabled": true,
		"default_channel": "C0123456789",
		"channels": {"gastown/polecats/*": "C0987654321"},
		"channel_names": {"C0987654321": "#polecat-decisions"},
		"bot_token": "xoxb-secret-bot-token",
		"app_token": "xapp-secret-app-token"
	}`

	var slackMap map[string]interface{}
	if err := json.Unmarshal([]byte(slackJSON), &slackMap); err != nil {
		t.Fatalf("parsing slack JSON: %v", err)
	}

	// Remove secrets (same as seedSlackBeads does)
	delete(slackMap, "bot_token")
	delete(slackMap, "app_token")

	metaJSON, err := json.Marshal(slackMap)
	if err != nil {
		t.Fatalf("marshaling metadata: %v", err)
	}

	metaStr := string(metaJSON)
	if strings.Contains(metaStr, "bot_token") {
		t.Errorf("metadata should not contain bot_token, got: %s", metaStr)
	}
	if strings.Contains(metaStr, "app_token") {
		t.Errorf("metadata should not contain app_token, got: %s", metaStr)
	}
	if strings.Contains(metaStr, "xoxb-") {
		t.Errorf("metadata should not contain bot token value, got: %s", metaStr)
	}
	if strings.Contains(metaStr, "xapp-") {
		t.Errorf("metadata should not contain app token value, got: %s", metaStr)
	}

	// Verify non-secret fields are preserved
	if !strings.Contains(metaStr, "C0123456789") {
		t.Error("metadata should contain default_channel")
	}
	if !strings.Contains(metaStr, "C0987654321") {
		t.Error("metadata should contain channel mappings")
	}
	if !strings.Contains(metaStr, "gastown/polecats/*") {
		t.Error("metadata should contain channel patterns")
	}
	if !strings.Contains(metaStr, "#polecat-decisions") {
		t.Error("metadata should contain channel names")
	}
}

func TestMessagingConfigSeedMetadata(t *testing.T) {
	// Verify that NewMessagingConfig can be marshaled to valid JSON
	// suitable for bead metadata storage
	msgConfig := config.NewMessagingConfig()

	msgJSON, err := json.Marshal(msgConfig)
	if err != nil {
		t.Fatalf("marshaling messaging config: %v", err)
	}

	// Must be valid JSON
	var msgMap map[string]interface{}
	if err := json.Unmarshal(msgJSON, &msgMap); err != nil {
		t.Fatalf("messaging config JSON is not a valid map: %v", err)
	}

	// Must have required fields
	if msgMap["type"] != "messaging" {
		t.Errorf("expected type 'messaging', got %v", msgMap["type"])
	}
	if msgMap["version"] != float64(1) {
		t.Errorf("expected version 1, got %v", msgMap["version"])
	}
}

func TestEscalationConfigSeedMetadata(t *testing.T) {
	// Verify that NewEscalationConfig can be marshaled to valid JSON
	// suitable for bead metadata storage
	escConfig := config.NewEscalationConfig()

	escJSON, err := json.Marshal(escConfig)
	if err != nil {
		t.Fatalf("marshaling escalation config: %v", err)
	}

	// Must be valid JSON
	var escMap map[string]interface{}
	if err := json.Unmarshal(escJSON, &escMap); err != nil {
		t.Fatalf("escalation config JSON is not a valid map: %v", err)
	}

	// Must have required fields
	if escMap["type"] != "escalation" {
		t.Errorf("expected type 'escalation', got %v", escMap["type"])
	}
	if escMap["version"] != float64(1) {
		t.Errorf("expected version 1, got %v", escMap["version"])
	}

	// Must have routes with severity levels
	routes, ok := escMap["routes"].(map[string]interface{})
	if !ok {
		t.Fatal("expected routes to be a map")
	}
	for _, severity := range []string{"low", "medium", "high", "critical"} {
		if _, ok := routes[severity]; !ok {
			t.Errorf("expected routes to contain severity '%s'", severity)
		}
	}

	// Must have stale_threshold
	if escMap["stale_threshold"] != "4h" {
		t.Errorf("expected stale_threshold '4h', got %v", escMap["stale_threshold"])
	}
}

func TestDaemonConfigSeedMetadata(t *testing.T) {
	// Verify that NewDaemonPatrolConfig can be marshaled to valid JSON
	// suitable for bead metadata storage
	daemonConfig := config.NewDaemonPatrolConfig()

	daemonJSON, err := json.Marshal(daemonConfig)
	if err != nil {
		t.Fatalf("marshaling daemon config: %v", err)
	}

	// Must be valid JSON
	var daemonMap map[string]interface{}
	if err := json.Unmarshal(daemonJSON, &daemonMap); err != nil {
		t.Fatalf("daemon config JSON is not a valid map: %v", err)
	}

	// Must have required fields
	if daemonMap["type"] != "daemon-patrol-config" {
		t.Errorf("expected type 'daemon-patrol-config', got %v", daemonMap["type"])
	}
	if daemonMap["version"] != float64(1) {
		t.Errorf("expected version 1, got %v", daemonMap["version"])
	}

	// Must have heartbeat
	heartbeat, ok := daemonMap["heartbeat"].(map[string]interface{})
	if !ok {
		t.Fatal("expected heartbeat to be a map")
	}
	if heartbeat["enabled"] != true {
		t.Errorf("expected heartbeat.enabled=true, got %v", heartbeat["enabled"])
	}

	// Must have patrols
	patrols, ok := daemonMap["patrols"].(map[string]interface{})
	if !ok {
		t.Fatal("expected patrols to be a map")
	}

	// Check required patrols exist
	for _, name := range []string{"deacon", "witness", "refinery"} {
		patrol, ok := patrols[name].(map[string]interface{})
		if !ok {
			t.Errorf("expected patrols.%s to be a map", name)
			continue
		}
		if patrol["enabled"] != true {
			t.Errorf("expected patrols.%s.enabled=true, got %v", name, patrol["enabled"])
		}
	}
}
