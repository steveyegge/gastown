package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/constants"
)

// skipIfAgentBinaryMissing skips the test if any of the specified agent binaries
// are not found in PATH. This allows tests that depend on specific agents to be
// skipped in environments where those agents aren't installed.
func skipIfAgentBinaryMissing(t *testing.T, agents ...string) {
	t.Helper()
	for _, agent := range agents {
		if _, err := exec.LookPath(agent); err != nil {
			t.Skipf("skipping test: agent binary %q not found in PATH", agent)
		}
	}
}

// isClaudeCommand checks if a command is claude (either "claude" or a path ending in "/claude").
// This handles the case where resolveClaudePath returns the full path to the claude binary.
// Also handles Windows paths with .exe extension.
func isClaudeCommand(cmd string) bool {
	base := filepath.Base(cmd)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return base == "claude"
}

func TestTownConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mayor", "town.json")

	original := &TownConfig{
		Type:      "town",
		Version:   1,
		Name:      "test-town",
		CreatedAt: time.Now().Truncate(time.Second),
	}

	if err := SaveTownConfig(path, original); err != nil {
		t.Fatalf("SaveTownConfig: %v", err)
	}

	loaded, err := LoadTownConfig(path)
	if err != nil {
		t.Fatalf("LoadTownConfig: %v", err)
	}

	if loaded.Name != original.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, original.Name)
	}
	if loaded.Type != original.Type {
		t.Errorf("Type = %q, want %q", loaded.Type, original.Type)
	}
}

func TestRigsConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mayor", "rigs.json")

	original := &RigsConfig{
		Version: 1,
		Rigs: map[string]RigEntry{
			"gastown": {
				GitURL:    "git@github.com:steveyegge/gastown.git",
				LocalRepo: "/tmp/local-repo",
				AddedAt:   time.Now().Truncate(time.Second),
				BeadsConfig: &BeadsConfig{
					Repo:   "local",
					Prefix: "gt-",
				},
			},
		},
	}

	if err := SaveRigsConfig(path, original); err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}

	loaded, err := LoadRigsConfig(path)
	if err != nil {
		t.Fatalf("LoadRigsConfig: %v", err)
	}

	if len(loaded.Rigs) != 1 {
		t.Errorf("Rigs count = %d, want 1", len(loaded.Rigs))
	}

	rig, ok := loaded.Rigs["gastown"]
	if !ok {
		t.Fatal("missing 'gastown' rig")
	}
	if rig.BeadsConfig == nil || rig.BeadsConfig.Prefix != "gt-" {
		t.Errorf("BeadsConfig.Prefix = %v, want 'gt-'", rig.BeadsConfig)
	}
	if rig.LocalRepo != "/tmp/local-repo" {
		t.Errorf("LocalRepo = %q, want %q", rig.LocalRepo, "/tmp/local-repo")
	}
}

func TestLoadTownConfigNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadTownConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestValidationErrors(t *testing.T) {
	t.Parallel()
	// Missing name
	tc := &TownConfig{Type: "town", Version: 1}
	if err := validateTownConfig(tc); err == nil {
		t.Error("expected error for missing name")
	}

	// Wrong type
	tc = &TownConfig{Type: "wrong", Version: 1, Name: "test"}
	if err := validateTownConfig(tc); err == nil {
		t.Error("expected error for wrong type")
	}
}

func TestRigConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := NewRigConfig("gastown", "git@github.com:test/gastown.git")
	original.CreatedAt = time.Now().Truncate(time.Second)
	original.Beads = &BeadsConfig{Prefix: "gt-"}
	original.LocalRepo = "/tmp/local-repo"

	if err := SaveRigConfig(path, original); err != nil {
		t.Fatalf("SaveRigConfig: %v", err)
	}

	loaded, err := LoadRigConfig(path)
	if err != nil {
		t.Fatalf("LoadRigConfig: %v", err)
	}

	if loaded.Type != "rig" {
		t.Errorf("Type = %q, want 'rig'", loaded.Type)
	}
	if loaded.Version != CurrentRigConfigVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, CurrentRigConfigVersion)
	}
	if loaded.Name != "gastown" {
		t.Errorf("Name = %q, want 'gastown'", loaded.Name)
	}
	if loaded.GitURL != "git@github.com:test/gastown.git" {
		t.Errorf("GitURL = %q, want expected URL", loaded.GitURL)
	}
	if loaded.LocalRepo != "/tmp/local-repo" {
		t.Errorf("LocalRepo = %q, want %q", loaded.LocalRepo, "/tmp/local-repo")
	}
	if loaded.Beads == nil || loaded.Beads.Prefix != "gt-" {
		t.Error("Beads.Prefix not preserved")
	}
}

func TestRigSettingsRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings", "config.json")

	original := NewRigSettings()

	if err := SaveRigSettings(path, original); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	loaded, err := LoadRigSettings(path)
	if err != nil {
		t.Fatalf("LoadRigSettings: %v", err)
	}

	if loaded.Type != "rig-settings" {
		t.Errorf("Type = %q, want 'rig-settings'", loaded.Type)
	}
	if loaded.MergeQueue == nil {
		t.Fatal("MergeQueue is nil")
	}
	if !loaded.MergeQueue.Enabled {
		t.Error("MergeQueue.Enabled = false, want true")
	}
	if loaded.MergeQueue.TargetBranch != "main" {
		t.Errorf("MergeQueue.TargetBranch = %q, want 'main'", loaded.MergeQueue.TargetBranch)
	}
}

func TestRigSettingsWithCustomMergeQueue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	original := &RigSettings{
		Type:    "rig-settings",
		Version: 1,
		MergeQueue: &MergeQueueConfig{
			Enabled:              true,
			TargetBranch:         "develop",
			IntegrationBranches:  false,
			OnConflict:           OnConflictAutoRebase,
			RunTests:             true,
			TestCommand:          "make test",
			DeleteMergedBranches: false,
			RetryFlakyTests:      3,
			PollInterval:         "1m",
			MaxConcurrent:        2,
		},
	}

	if err := SaveRigSettings(path, original); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	loaded, err := LoadRigSettings(path)
	if err != nil {
		t.Fatalf("LoadRigSettings: %v", err)
	}

	mq := loaded.MergeQueue
	if mq.TargetBranch != "develop" {
		t.Errorf("TargetBranch = %q, want 'develop'", mq.TargetBranch)
	}
	if mq.OnConflict != OnConflictAutoRebase {
		t.Errorf("OnConflict = %q, want %q", mq.OnConflict, OnConflictAutoRebase)
	}
	if mq.TestCommand != "make test" {
		t.Errorf("TestCommand = %q, want 'make test'", mq.TestCommand)
	}
	if mq.RetryFlakyTests != 3 {
		t.Errorf("RetryFlakyTests = %d, want 3", mq.RetryFlakyTests)
	}
}

func TestRigConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  *RigConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &RigConfig{
				Type:    "rig",
				Version: 1,
				Name:    "test-rig",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: &RigConfig{
				Type:    "rig",
				Version: 1,
			},
			wantErr: true,
		},
		{
			name: "wrong type",
			config: &RigConfig{
				Type:    "wrong",
				Version: 1,
				Name:    "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRigConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRigConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRigSettingsValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		settings *RigSettings
		wantErr  bool
	}{
		{
			name: "valid settings",
			settings: &RigSettings{
				Type:       "rig-settings",
				Version:    1,
				MergeQueue: DefaultMergeQueueConfig(),
			},
			wantErr: false,
		},
		{
			name: "valid settings without merge queue",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
			},
			wantErr: false,
		},
		{
			name: "wrong type",
			settings: &RigSettings{
				Type:    "wrong",
				Version: 1,
			},
			wantErr: true,
		},
		{
			name: "invalid on_conflict",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
				MergeQueue: &MergeQueueConfig{
					OnConflict: "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid poll_interval",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
				MergeQueue: &MergeQueueConfig{
					PollInterval: "not-a-duration",
				},
			},
			wantErr: true,
		},
		{
			name: "valid merge strategy direct_merge",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
				MergeQueue: &MergeQueueConfig{
					Strategy: StrategyDirectMerge,
				},
			},
			wantErr: false,
		},
		{
			name: "valid merge strategy pr_to_main",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
				MergeQueue: &MergeQueueConfig{
					Strategy: StrategyPRToMain,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid merge strategy",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
				MergeQueue: &MergeQueueConfig{
					Strategy: "invalid_strategy",
				},
			},
			wantErr: true,
		},
		{
			name: "pr_options with non-PR strategy",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
				MergeQueue: &MergeQueueConfig{
					Strategy:  StrategyDirectMerge,
					PROptions: &PROptions{AutoMerge: true},
				},
			},
			wantErr: true,
		},
		{
			name: "pr_options with PR strategy is valid",
			settings: &RigSettings{
				Type:    "rig-settings",
				Version: 1,
				MergeQueue: &MergeQueueConfig{
					Strategy:  StrategyPRToMain,
					PROptions: &PROptions{AutoMerge: true, Labels: []string{"automated"}},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRigSettings(tt.settings)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRigSettings() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultMergeQueueConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultMergeQueueConfig()

	if !cfg.Enabled {
		t.Error("Enabled should be true by default")
	}
	if cfg.Strategy != StrategyDirectMerge {
		t.Errorf("Strategy = %q, want %q", cfg.Strategy, StrategyDirectMerge)
	}
	if cfg.TargetBranch != "main" {
		t.Errorf("TargetBranch = %q, want 'main'", cfg.TargetBranch)
	}
	if !cfg.IntegrationBranches {
		t.Error("IntegrationBranches should be true by default")
	}
	if cfg.OnConflict != OnConflictAssignBack {
		t.Errorf("OnConflict = %q, want %q", cfg.OnConflict, OnConflictAssignBack)
	}
	if !cfg.RunTests {
		t.Error("RunTests should be true by default")
	}
	if cfg.TestCommand != "go test ./..." {
		t.Errorf("TestCommand = %q, want 'go test ./...'", cfg.TestCommand)
	}
	if !cfg.DeleteMergedBranches {
		t.Error("DeleteMergedBranches should be true by default")
	}
	if cfg.RetryFlakyTests != 1 {
		t.Errorf("RetryFlakyTests = %d, want 1", cfg.RetryFlakyTests)
	}
	if cfg.PollInterval != "30s" {
		t.Errorf("PollInterval = %q, want '30s'", cfg.PollInterval)
	}
	if cfg.MaxConcurrent != 1 {
		t.Errorf("MaxConcurrent = %d, want 1", cfg.MaxConcurrent)
	}
}

func TestLoadRigConfigNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadRigConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadRigSettingsNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadRigSettings("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestMayorConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mayor", "config.json")

	original := NewMayorConfig()
	original.Theme = &TownThemeConfig{
		RoleDefaults: map[string]string{
			"witness": "rust",
		},
	}

	if err := SaveMayorConfig(path, original); err != nil {
		t.Fatalf("SaveMayorConfig: %v", err)
	}

	loaded, err := LoadMayorConfig(path)
	if err != nil {
		t.Fatalf("LoadMayorConfig: %v", err)
	}

	if loaded.Type != "mayor-config" {
		t.Errorf("Type = %q, want 'mayor-config'", loaded.Type)
	}
	if loaded.Version != CurrentMayorConfigVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, CurrentMayorConfigVersion)
	}
	if loaded.Theme == nil || loaded.Theme.RoleDefaults["witness"] != "rust" {
		t.Error("Theme.RoleDefaults not preserved")
	}
}

func TestLoadMayorConfigNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadMayorConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestAccountsConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mayor", "accounts.json")

	original := NewAccountsConfig()
	original.Accounts["yegge"] = Account{
		Email:       "steve.yegge@gmail.com",
		Description: "Personal account",
		ConfigDir:   "~/.claude-accounts/yegge",
	}
	original.Accounts["ghosttrack"] = Account{
		Email:       "steve@ghosttrack.com",
		Description: "Business account",
		ConfigDir:   "~/.claude-accounts/ghosttrack",
	}
	original.Default = "ghosttrack"

	if err := SaveAccountsConfig(path, original); err != nil {
		t.Fatalf("SaveAccountsConfig: %v", err)
	}

	loaded, err := LoadAccountsConfig(path)
	if err != nil {
		t.Fatalf("LoadAccountsConfig: %v", err)
	}

	if loaded.Version != CurrentAccountsVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, CurrentAccountsVersion)
	}
	if len(loaded.Accounts) != 2 {
		t.Errorf("Accounts count = %d, want 2", len(loaded.Accounts))
	}
	if loaded.Default != "ghosttrack" {
		t.Errorf("Default = %q, want 'ghosttrack'", loaded.Default)
	}

	yegge := loaded.GetAccount("yegge")
	if yegge == nil {
		t.Fatal("GetAccount('yegge') returned nil")
	}
	if yegge.Email != "steve.yegge@gmail.com" {
		t.Errorf("yegge.Email = %q, want 'steve.yegge@gmail.com'", yegge.Email)
	}

	defAcct := loaded.GetDefaultAccount()
	if defAcct == nil {
		t.Fatal("GetDefaultAccount() returned nil")
	}
	if defAcct.Email != "steve@ghosttrack.com" {
		t.Errorf("default.Email = %q, want 'steve@ghosttrack.com'", defAcct.Email)
	}
}

func TestAccountsConfigWithAuthToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mayor", "accounts.json")

	original := NewAccountsConfig()
	original.Accounts["litellm"] = Account{
		Description: "LiteLLM proxy account",
		ConfigDir:   "~/.claude-accounts/litellm",
		AuthToken:   "sk-test-12345",
		BaseURL:     "http://localhost:4000",
	}
	original.Default = "litellm"

	if err := SaveAccountsConfig(path, original); err != nil {
		t.Fatalf("SaveAccountsConfig: %v", err)
	}

	loaded, err := LoadAccountsConfig(path)
	if err != nil {
		t.Fatalf("LoadAccountsConfig: %v", err)
	}

	litellm := loaded.GetAccount("litellm")
	if litellm == nil {
		t.Fatal("GetAccount('litellm') returned nil")
	}
	if litellm.AuthToken != "sk-test-12345" {
		t.Errorf("litellm.AuthToken = %q, want 'sk-test-12345'", litellm.AuthToken)
	}
	if litellm.BaseURL != "http://localhost:4000" {
		t.Errorf("litellm.BaseURL = %q, want 'http://localhost:4000'", litellm.BaseURL)
	}
}

func TestAccountsConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  *AccountsConfig
		wantErr bool
	}{
		{
			name:    "valid empty config",
			config:  NewAccountsConfig(),
			wantErr: false,
		},
		{
			name: "valid config with accounts",
			config: &AccountsConfig{
				Version: 1,
				Accounts: map[string]Account{
					"test": {Email: "test@example.com", ConfigDir: "~/.claude-accounts/test"},
				},
				Default: "test",
			},
			wantErr: false,
		},
		{
			name: "default refers to nonexistent account",
			config: &AccountsConfig{
				Version: 1,
				Accounts: map[string]Account{
					"test": {Email: "test@example.com", ConfigDir: "~/.claude-accounts/test"},
				},
				Default: "nonexistent",
			},
			wantErr: true,
		},
		{
			name: "account missing config_dir",
			config: &AccountsConfig{
				Version: 1,
				Accounts: map[string]Account{
					"test": {Email: "test@example.com"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAccountsConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAccountsConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadAccountsConfigNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadAccountsConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestValidateAccountAuth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		account *ResolvedAccount
		wantErr bool
	}{
		{
			name:    "nil account is valid",
			account: nil,
			wantErr: false,
		},
		{
			name: "auth token is valid credentials",
			account: &ResolvedAccount{
				Handle:    "litellm",
				AuthToken: "sk-test-12345",
			},
			wantErr: false,
		},
		{
			name: "empty config dir with auth token is valid",
			account: &ResolvedAccount{
				Handle:    "litellm",
				AuthToken: "sk-test-12345",
				BaseURL:   "http://localhost:4000",
			},
			wantErr: false,
		},
		{
			name: "empty config dir without auth token is valid (let Claude handle)",
			account: &ResolvedAccount{
				Handle: "test",
			},
			wantErr: false,
		},
		{
			name: "config dir without credentials file fails",
			account: &ResolvedAccount{
				Handle:    "test",
				ConfigDir: "/nonexistent/path",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountAuth(tt.account)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAccountAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveAccount(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mayor", "accounts.json")

	cfg := NewAccountsConfig()
	cfg.Accounts["work"] = Account{
		Email:     "work@example.com",
		ConfigDir: "~/.claude-accounts/work",
	}
	cfg.Accounts["litellm"] = Account{
		Description: "LiteLLM proxy",
		ConfigDir:   "~/.claude-accounts/litellm",
		AuthToken:   "sk-test-12345",
		BaseURL:     "http://localhost:4000",
	}
	cfg.Default = "work"

	if err := SaveAccountsConfig(path, cfg); err != nil {
		t.Fatalf("SaveAccountsConfig: %v", err)
	}

	// Test resolving default account
	resolved, err := ResolveAccount(path, "")
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if resolved == nil {
		t.Fatal("ResolveAccount returned nil")
	}
	if resolved.Handle != "work" {
		t.Errorf("Handle = %q, want 'work'", resolved.Handle)
	}
	if resolved.AuthToken != "" {
		t.Errorf("AuthToken should be empty for work account")
	}

	// Test resolving specific account with auth token
	resolved, err = ResolveAccount(path, "litellm")
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if resolved.Handle != "litellm" {
		t.Errorf("Handle = %q, want 'litellm'", resolved.Handle)
	}
	if resolved.AuthToken != "sk-test-12345" {
		t.Errorf("AuthToken = %q, want 'sk-test-12345'", resolved.AuthToken)
	}
	if resolved.BaseURL != "http://localhost:4000" {
		t.Errorf("BaseURL = %q, want 'http://localhost:4000'", resolved.BaseURL)
	}
}

func TestMessagingConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config", "messaging.json")

	original := NewMessagingConfig()
	original.Lists["oncall"] = []string{"mayor/", "gastown/witness"}
	original.Lists["cleanup"] = []string{"gastown/witness", "deacon/"}
	original.Queues["work/gastown"] = QueueConfig{
		Workers:   []string{"gastown/polecats/*"},
		MaxClaims: 5,
	}
	original.Announces["alerts"] = AnnounceConfig{
		Readers:     []string{"@town"},
		RetainCount: 100,
	}
	original.NudgeChannels["workers"] = []string{"gastown/polecats/*", "gastown/crew/*"}
	original.NudgeChannels["witnesses"] = []string{"*/witness"}

	if err := SaveMessagingConfig(path, original); err != nil {
		t.Fatalf("SaveMessagingConfig: %v", err)
	}

	loaded, err := LoadMessagingConfig(path)
	if err != nil {
		t.Fatalf("LoadMessagingConfig: %v", err)
	}

	if loaded.Type != "messaging" {
		t.Errorf("Type = %q, want 'messaging'", loaded.Type)
	}
	if loaded.Version != CurrentMessagingVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, CurrentMessagingVersion)
	}

	// Check lists
	if len(loaded.Lists) != 2 {
		t.Errorf("Lists count = %d, want 2", len(loaded.Lists))
	}
	if oncall, ok := loaded.Lists["oncall"]; !ok || len(oncall) != 2 {
		t.Error("oncall list not preserved")
	}

	// Check queues
	if len(loaded.Queues) != 1 {
		t.Errorf("Queues count = %d, want 1", len(loaded.Queues))
	}
	if q, ok := loaded.Queues["work/gastown"]; !ok || q.MaxClaims != 5 {
		t.Error("queue not preserved")
	}

	// Check announces
	if len(loaded.Announces) != 1 {
		t.Errorf("Announces count = %d, want 1", len(loaded.Announces))
	}
	if a, ok := loaded.Announces["alerts"]; !ok || a.RetainCount != 100 {
		t.Error("announce not preserved")
	}

	// Check nudge channels
	if len(loaded.NudgeChannels) != 2 {
		t.Errorf("NudgeChannels count = %d, want 2", len(loaded.NudgeChannels))
	}
	if workers, ok := loaded.NudgeChannels["workers"]; !ok || len(workers) != 2 {
		t.Error("workers nudge channel not preserved")
	}
	if witnesses, ok := loaded.NudgeChannels["witnesses"]; !ok || len(witnesses) != 1 {
		t.Error("witnesses nudge channel not preserved")
	}
}

func TestMessagingConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  *MessagingConfig
		wantErr bool
	}{
		{
			name:    "valid empty config",
			config:  NewMessagingConfig(),
			wantErr: false,
		},
		{
			name: "valid config with lists",
			config: &MessagingConfig{
				Type:    "messaging",
				Version: 1,
				Lists: map[string][]string{
					"oncall": {"mayor/", "gastown/witness"},
				},
			},
			wantErr: false,
		},
		{
			name: "wrong type",
			config: &MessagingConfig{
				Type:    "wrong",
				Version: 1,
			},
			wantErr: true,
		},
		{
			name: "future version rejected",
			config: &MessagingConfig{
				Type:    "messaging",
				Version: 999,
			},
			wantErr: true,
		},
		{
			name: "list with no recipients",
			config: &MessagingConfig{
				Version: 1,
				Lists: map[string][]string{
					"empty": {},
				},
			},
			wantErr: true,
		},
		{
			name: "queue with no workers",
			config: &MessagingConfig{
				Version: 1,
				Queues: map[string]QueueConfig{
					"work": {Workers: []string{}},
				},
			},
			wantErr: true,
		},
		{
			name: "queue with negative max_claims",
			config: &MessagingConfig{
				Version: 1,
				Queues: map[string]QueueConfig{
					"work": {Workers: []string{"worker/"}, MaxClaims: -1},
				},
			},
			wantErr: true,
		},
		{
			name: "announce with no readers",
			config: &MessagingConfig{
				Version: 1,
				Announces: map[string]AnnounceConfig{
					"alerts": {Readers: []string{}},
				},
			},
			wantErr: true,
		},
		{
			name: "announce with negative retain_count",
			config: &MessagingConfig{
				Version: 1,
				Announces: map[string]AnnounceConfig{
					"alerts": {Readers: []string{"@town"}, RetainCount: -1},
				},
			},
			wantErr: true,
		},
		{
			name: "valid config with nudge channels",
			config: &MessagingConfig{
				Type:    "messaging",
				Version: 1,
				NudgeChannels: map[string][]string{
					"workers": {"gastown/polecats/*", "gastown/crew/*"},
				},
			},
			wantErr: false,
		},
		{
			name: "nudge channel with no recipients",
			config: &MessagingConfig{
				Version: 1,
				NudgeChannels: map[string][]string{
					"empty": {},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessagingConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMessagingConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadMessagingConfigNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadMessagingConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadMessagingConfigMalformedJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "messaging.json")

	// Write malformed JSON
	if err := os.WriteFile(path, []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, err := LoadMessagingConfig(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadOrCreateMessagingConfig(t *testing.T) {
	t.Parallel()
	// Test creating default when not found
	config, err := LoadOrCreateMessagingConfig("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("LoadOrCreateMessagingConfig: %v", err)
	}
	if config == nil {
		t.Fatal("expected non-nil config")
	}
	if config.Version != CurrentMessagingVersion {
		t.Errorf("Version = %d, want %d", config.Version, CurrentMessagingVersion)
	}

	// Test loading existing
	dir := t.TempDir()
	path := filepath.Join(dir, "messaging.json")
	original := NewMessagingConfig()
	original.Lists["test"] = []string{"mayor/"}
	if err := SaveMessagingConfig(path, original); err != nil {
		t.Fatalf("SaveMessagingConfig: %v", err)
	}

	loaded, err := LoadOrCreateMessagingConfig(path)
	if err != nil {
		t.Fatalf("LoadOrCreateMessagingConfig: %v", err)
	}
	if _, ok := loaded.Lists["test"]; !ok {
		t.Error("existing config not loaded")
	}
}

func TestMessagingConfigPath(t *testing.T) {
	t.Parallel()
	path := MessagingConfigPath("/home/user/gt")
	expected := "/home/user/gt/config/messaging.json"
	if filepath.ToSlash(path) != expected {
		t.Errorf("MessagingConfigPath = %q, want %q", path, expected)
	}
}

func TestRuntimeConfigDefaults(t *testing.T) {
	t.Parallel()
	rc := DefaultRuntimeConfig()
	if rc.Provider != "claude" {
		t.Errorf("Provider = %q, want %q", rc.Provider, "claude")
	}
	if !isClaudeCommand(rc.Command) {
		t.Errorf("Command = %q, want claude or path ending in /claude", rc.Command)
	}
	if len(rc.Args) != 1 || rc.Args[0] != "--dangerously-skip-permissions" {
		t.Errorf("Args = %v, want [--dangerously-skip-permissions]", rc.Args)
	}
	if rc.Session == nil || rc.Session.SessionIDEnv != "CLAUDE_SESSION_ID" {
		t.Errorf("SessionIDEnv = %q, want %q", rc.Session.SessionIDEnv, "CLAUDE_SESSION_ID")
	}
}

func TestRuntimeConfigBuildCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		rc           *RuntimeConfig
		wantContains []string // Parts the command should contain
		isClaudeCmd  bool     // Whether command should be claude (or path to claude)
	}{
		{
			name:         "nil config uses defaults",
			rc:           nil,
			wantContains: []string{"--dangerously-skip-permissions"},
			isClaudeCmd:  true,
		},
		{
			name:         "default config",
			rc:           DefaultRuntimeConfig(),
			wantContains: []string{"--dangerously-skip-permissions"},
			isClaudeCmd:  true,
		},
		{
			name:         "custom command",
			rc:           &RuntimeConfig{Command: "aider", Args: []string{"--no-git"}},
			wantContains: []string{"aider", "--no-git"},
			isClaudeCmd:  false,
		},
		{
			name:         "multiple args",
			rc:           &RuntimeConfig{Command: "claude", Args: []string{"--model", "opus", "--no-confirm"}},
			wantContains: []string{"--model", "opus", "--no-confirm"},
			isClaudeCmd:  true,
		},
		{
			name:         "empty command uses default",
			rc:           &RuntimeConfig{Command: "", Args: nil},
			wantContains: []string{"--dangerously-skip-permissions"},
			isClaudeCmd:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rc.BuildCommand()
			// Check command contains expected parts
			for _, part := range tt.wantContains {
				if !strings.Contains(got, part) {
					t.Errorf("BuildCommand() = %q, should contain %q", got, part)
				}
			}
			// Check if command starts with claude (or path to claude)
			if tt.isClaudeCmd {
				parts := strings.Fields(got)
				if len(parts) > 0 && !isClaudeCommand(parts[0]) {
					t.Errorf("BuildCommand() = %q, command should be claude or path to claude", got)
				}
			}
		})
	}
}

func TestRuntimeConfigBuildCommandWithPrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		rc           *RuntimeConfig
		prompt       string
		wantContains []string // Parts the command should contain
		isClaudeCmd  bool     // Whether command should be claude (or path to claude)
	}{
		{
			name:         "no prompt",
			rc:           DefaultRuntimeConfig(),
			prompt:       "",
			wantContains: []string{"--dangerously-skip-permissions"},
			isClaudeCmd:  true,
		},
		{
			name:         "with prompt",
			rc:           DefaultRuntimeConfig(),
			prompt:       "gt prime",
			wantContains: []string{"--dangerously-skip-permissions", `"gt prime"`},
			isClaudeCmd:  true,
		},
		{
			name:         "prompt with quotes",
			rc:           DefaultRuntimeConfig(),
			prompt:       `Hello "world"`,
			wantContains: []string{"--dangerously-skip-permissions", `"Hello \"world\""`},
			isClaudeCmd:  true,
		},
		{
			name:         "config initial prompt used if no override",
			rc:           &RuntimeConfig{Command: "aider", Args: []string{}, InitialPrompt: "/help"},
			prompt:       "",
			wantContains: []string{"aider", `"/help"`},
			isClaudeCmd:  false,
		},
		{
			name:         "override takes precedence over config",
			rc:           &RuntimeConfig{Command: "aider", Args: []string{}, InitialPrompt: "/help"},
			prompt:       "custom prompt",
			wantContains: []string{"aider", `"custom prompt"`},
			isClaudeCmd:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.rc.BuildCommandWithPrompt(tt.prompt)
			// Check command contains expected parts
			for _, part := range tt.wantContains {
				if !strings.Contains(got, part) {
					t.Errorf("BuildCommandWithPrompt(%q) = %q, should contain %q", tt.prompt, got, part)
				}
			}
			// Check if command starts with claude (or path to claude)
			if tt.isClaudeCmd {
				parts := strings.Fields(got)
				if len(parts) > 0 && !isClaudeCommand(parts[0]) {
					t.Errorf("BuildCommandWithPrompt(%q) = %q, command should be claude or path to claude", tt.prompt, got)
				}
			}
		})
	}
}

func TestBuildAgentStartupCommand(t *testing.T) {
	// BuildAgentStartupCommand auto-detects town root from cwd when rigPath is empty.
	// Use a temp directory to ensure we exercise the fallback default config path.
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpWD := t.TempDir()
	if err := os.Chdir(tmpWD); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	// Test without rig config (uses defaults)
	// New signature: (role, rig, townRoot, rigPath, prompt)
	cmd := BuildAgentStartupCommand("witness", "gastown", "", "", "")

	// Should contain environment variables (via 'exec env') and claude command
	if !strings.Contains(cmd, "exec env") {
		t.Error("expected 'exec env' in command")
	}
	if !strings.Contains(cmd, "GT_ROLE=gastown/witness") {
		t.Error("expected GT_ROLE=gastown/witness in command")
	}
	if !strings.Contains(cmd, "BD_ACTOR=gastown/witness") {
		t.Error("expected BD_ACTOR in command")
	}
	if !strings.Contains(cmd, "claude --dangerously-skip-permissions") {
		t.Error("expected claude command in output")
	}
}

func TestBuildPolecatStartupCommand(t *testing.T) {
	t.Parallel()
	cmd := BuildPolecatStartupCommand("gastown", "toast", "", "")

	if !strings.Contains(cmd, "GT_ROLE=gastown/polecats/toast") {
		t.Error("expected GT_ROLE=gastown/polecats/toast in command")
	}
	if !strings.Contains(cmd, "GT_RIG=gastown") {
		t.Error("expected GT_RIG=gastown in command")
	}
	if !strings.Contains(cmd, "GT_POLECAT=toast") {
		t.Error("expected GT_POLECAT=toast in command")
	}
	if !strings.Contains(cmd, "BD_ACTOR=gastown/polecats/toast") {
		t.Error("expected BD_ACTOR in command")
	}
}

func TestBuildCrewStartupCommand(t *testing.T) {
	t.Parallel()
	cmd := BuildCrewStartupCommand("gastown", "max", "", "")

	if !strings.Contains(cmd, "GT_ROLE=gastown/crew/max") {
		t.Error("expected GT_ROLE=gastown/crew/max in command")
	}
	if !strings.Contains(cmd, "GT_RIG=gastown") {
		t.Error("expected GT_RIG=gastown in command")
	}
	if !strings.Contains(cmd, "GT_CREW=max") {
		t.Error("expected GT_CREW=max in command")
	}
	if !strings.Contains(cmd, "BD_ACTOR=gastown/crew/max") {
		t.Error("expected BD_ACTOR in command")
	}
}

func TestResolveAgentConfigWithOverride(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Town settings: default agent is gemini, plus a custom alias.
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "gemini"
	townSettings.Agents["claude-haiku"] = &RuntimeConfig{
		Command: "claude",
		Args:    []string{"--model", "haiku", "--dangerously-skip-permissions"},
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Rig settings: prefer codex unless overridden.
	rigSettings := NewRigSettings()
	rigSettings.Agent = "codex"
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	t.Run("no override uses rig agent", func(t *testing.T) {
		rc, name, err := ResolveAgentConfigWithOverride(townRoot, rigPath, "")
		if err != nil {
			t.Fatalf("ResolveAgentConfigWithOverride: %v", err)
		}
		if name != "codex" {
			t.Fatalf("name = %q, want %q", name, "codex")
		}
		if rc.Command != "codex" {
			t.Fatalf("rc.Command = %q, want %q", rc.Command, "codex")
		}
	})

	t.Run("override uses built-in preset", func(t *testing.T) {
		rc, name, err := ResolveAgentConfigWithOverride(townRoot, rigPath, "gemini")
		if err != nil {
			t.Fatalf("ResolveAgentConfigWithOverride: %v", err)
		}
		if name != "gemini" {
			t.Fatalf("name = %q, want %q", name, "gemini")
		}
		if rc.Command != "gemini" {
			t.Fatalf("rc.Command = %q, want %q", rc.Command, "gemini")
		}
	})

	t.Run("override uses custom agent alias", func(t *testing.T) {
		rc, name, err := ResolveAgentConfigWithOverride(townRoot, rigPath, "claude-haiku")
		if err != nil {
			t.Fatalf("ResolveAgentConfigWithOverride: %v", err)
		}
		if name != "claude-haiku" {
			t.Fatalf("name = %q, want %q", name, "claude-haiku")
		}
		if !isClaudeCommand(rc.Command) {
			t.Fatalf("rc.Command = %q, want claude or path ending in /claude", rc.Command)
		}
		got := rc.BuildCommand()
		// Check command includes expected flags (path to claude may vary)
		if !strings.Contains(got, "--model haiku") || !strings.Contains(got, "--dangerously-skip-permissions") {
			t.Fatalf("BuildCommand() = %q, want command with --model haiku and --dangerously-skip-permissions", got)
		}
	})

	t.Run("unknown override errors", func(t *testing.T) {
		_, _, err := ResolveAgentConfigWithOverride(townRoot, rigPath, "nope-not-an-agent")
		if err == nil {
			t.Fatal("expected error for unknown agent override")
		}
	})
}

func TestBuildPolecatStartupCommandWithAgentOverride(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	townSettings := NewTownSettings()
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// The rig settings file must exist for resolver calls that load it.
	if err := SaveRigSettings(RigSettingsPath(rigPath), NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd, err := BuildPolecatStartupCommandWithAgentOverride("testrig", "toast", rigPath, "", "gemini")
	if err != nil {
		t.Fatalf("BuildPolecatStartupCommandWithAgentOverride: %v", err)
	}
	if !strings.Contains(cmd, "GT_ROLE=testrig/polecats/toast") {
		t.Fatalf("expected GT_ROLE export in command: %q", cmd)
	}
	if !strings.Contains(cmd, "GT_RIG=testrig") {
		t.Fatalf("expected GT_RIG export in command: %q", cmd)
	}
	if !strings.Contains(cmd, "GT_POLECAT=toast") {
		t.Fatalf("expected GT_POLECAT export in command: %q", cmd)
	}
	if !strings.Contains(cmd, "gemini --approval-mode yolo") {
		t.Fatalf("expected gemini command in output: %q", cmd)
	}
}

func TestBuildAgentStartupCommandWithAgentOverride(t *testing.T) {
	townRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte("{}"), 0600); err != nil {
		t.Fatalf("WriteFile town.json: %v", err)
	}

	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "gemini"
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	originalWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWd) })
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("Chdir: %v", err)
	}

	t.Run("empty override uses default agent", func(t *testing.T) {
		// New signature: (role, rig, townRoot, rigPath, prompt, agentOverride)
		cmd, err := BuildAgentStartupCommandWithAgentOverride("mayor", "", "", "", "", "")
		if err != nil {
			t.Fatalf("BuildAgentStartupCommandWithAgentOverride: %v", err)
		}
		if !strings.Contains(cmd, "GT_ROLE=mayor") {
			t.Fatalf("expected GT_ROLE export in command: %q", cmd)
		}
		if !strings.Contains(cmd, "BD_ACTOR=mayor") {
			t.Fatalf("expected BD_ACTOR export in command: %q", cmd)
		}
		if !strings.Contains(cmd, "gemini --approval-mode yolo") {
			t.Fatalf("expected gemini command in output: %q", cmd)
		}
	})

	t.Run("override switches agent", func(t *testing.T) {
		// New signature: (role, rig, townRoot, rigPath, prompt, agentOverride)
		cmd, err := BuildAgentStartupCommandWithAgentOverride("mayor", "", "", "", "", "codex")
		if err != nil {
			t.Fatalf("BuildAgentStartupCommandWithAgentOverride: %v", err)
		}
		if !strings.Contains(cmd, "codex") {
			t.Fatalf("expected codex command in output: %q", cmd)
		}
	})
}

func TestBuildCrewStartupCommandWithAgentOverride(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	townSettings := NewTownSettings()
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	if err := SaveRigSettings(RigSettingsPath(rigPath), NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd, err := BuildCrewStartupCommandWithAgentOverride("testrig", "max", rigPath, "gt prime", "gemini")
	if err != nil {
		t.Fatalf("BuildCrewStartupCommandWithAgentOverride: %v", err)
	}
	if !strings.Contains(cmd, "GT_ROLE=testrig/crew/max") {
		t.Fatalf("expected GT_ROLE export in command: %q", cmd)
	}
	if !strings.Contains(cmd, "GT_RIG=testrig") {
		t.Fatalf("expected GT_RIG export in command: %q", cmd)
	}
	if !strings.Contains(cmd, "GT_CREW=max") {
		t.Fatalf("expected GT_CREW export in command: %q", cmd)
	}
	if !strings.Contains(cmd, "BD_ACTOR=testrig/crew/max") {
		t.Fatalf("expected BD_ACTOR export in command: %q", cmd)
	}
	if !strings.Contains(cmd, "gemini --approval-mode yolo") {
		t.Fatalf("expected gemini command in output: %q", cmd)
	}
}

func TestBuildStartupCommand_UsesRigAgentWhenRigPathProvided(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "gemini"
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	rigSettings := NewRigSettings()
	rigSettings.Agent = "codex"
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd := BuildStartupCommand(map[string]string{"GT_ROLE": "witness"}, rigPath, "")
	if !strings.Contains(cmd, "codex") {
		t.Fatalf("expected rig agent (codex) in command: %q", cmd)
	}
	if strings.Contains(cmd, "gemini --approval-mode yolo") {
		t.Fatalf("did not expect town default agent in command: %q", cmd)
	}
}

func TestBuildStartupCommand_UsesRoleAgentsFromTownSettings(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	binDir := t.TempDir()
	for _, name := range []string{"gemini", "codex"} {
		if runtime.GOOS == "windows" {
			path := filepath.Join(binDir, name+".cmd")
			if err := os.WriteFile(path, []byte("@echo off\r\nexit /b 0\r\n"), 0644); err != nil {
				t.Fatalf("write %s stub: %v", name, err)
			}
			continue
		}
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write %s stub: %v", name, err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Configure town settings with role_agents
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		constants.RoleRefinery: "gemini",
		constants.RoleWitness:  "codex",
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create empty rig settings (no agent override)
	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	t.Run("refinery role gets gemini from role_agents", func(t *testing.T) {
		cmd := BuildStartupCommand(map[string]string{"GT_ROLE": constants.RoleRefinery}, rigPath, "")
		if !strings.Contains(cmd, "gemini") {
			t.Fatalf("expected gemini for refinery role, got: %q", cmd)
		}
	})

	t.Run("witness role gets codex from role_agents", func(t *testing.T) {
		cmd := BuildStartupCommand(map[string]string{"GT_ROLE": constants.RoleWitness}, rigPath, "")
		if !strings.Contains(cmd, "codex") {
			t.Fatalf("expected codex for witness role, got: %q", cmd)
		}
	})

	t.Run("crew role falls back to default_agent (not in role_agents)", func(t *testing.T) {
		cmd := BuildStartupCommand(map[string]string{"GT_ROLE": constants.RoleCrew}, rigPath, "")
		if !strings.Contains(cmd, "claude") {
			t.Fatalf("expected claude fallback for crew role, got: %q", cmd)
		}
	})

	t.Run("no role falls back to default resolution", func(t *testing.T) {
		cmd := BuildStartupCommand(map[string]string{}, rigPath, "")
		if !strings.Contains(cmd, "claude") {
			t.Fatalf("expected claude for no role, got: %q", cmd)
		}
	})
}

func TestBuildStartupCommand_RigRoleAgentsOverridesTownRoleAgents(t *testing.T) {
	skipIfAgentBinaryMissing(t, "gemini", "codex")
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Town settings has witness = gemini
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		constants.RoleWitness: "gemini",
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Rig settings overrides witness to codex
	rigSettings := NewRigSettings()
	rigSettings.RoleAgents = map[string]string{
		constants.RoleWitness: "codex",
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd := BuildStartupCommand(map[string]string{"GT_ROLE": constants.RoleWitness}, rigPath, "")
	if !strings.Contains(cmd, "codex") {
		t.Fatalf("expected codex from rig role_agents override, got: %q", cmd)
	}
	if strings.Contains(cmd, "gemini") {
		t.Fatalf("did not expect town role_agents (gemini) in command: %q", cmd)
	}
}

func TestBuildAgentStartupCommand_UsesRoleAgents(t *testing.T) {
	skipIfAgentBinaryMissing(t, "codex")
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Configure town settings with role_agents
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		constants.RoleRefinery: "codex",
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create empty rig settings
	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	// BuildAgentStartupCommand passes role via GT_ROLE env var (compound format)
	cmd := BuildAgentStartupCommand(constants.RoleRefinery, "testrig", townRoot, rigPath, "")
	if !strings.Contains(cmd, "codex") {
		t.Fatalf("expected codex for refinery role, got: %q", cmd)
	}
	if !strings.Contains(cmd, "GT_ROLE=testrig/refinery") {
		t.Fatalf("expected GT_ROLE=testrig/refinery in command: %q", cmd)
	}
}

func TestValidateAgentConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid built-in agent", func(t *testing.T) {
		// claude is a built-in preset and binary should exist
		err := ValidateAgentConfig("claude", nil, nil)
		// Note: This may fail if claude binary is not installed, which is expected
		if err != nil && !strings.Contains(err.Error(), "not found in PATH") {
			t.Errorf("unexpected error for claude: %v", err)
		}
	})

	t.Run("invalid agent name", func(t *testing.T) {
		err := ValidateAgentConfig("nonexistent-agent-xyz", nil, nil)
		if err == nil {
			t.Error("expected error for nonexistent agent")
		}
		if !strings.Contains(err.Error(), "not found in config or built-in presets") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("custom agent with missing binary", func(t *testing.T) {
		townSettings := NewTownSettings()
		townSettings.Agents = map[string]*RuntimeConfig{
			"my-custom-agent": {
				Command: "nonexistent-binary-xyz123",
				Args:    []string{"--some-flag"},
			},
		}
		err := ValidateAgentConfig("my-custom-agent", townSettings, nil)
		if err == nil {
			t.Error("expected error for missing binary")
		}
		if !strings.Contains(err.Error(), "not found in PATH") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestResolveRoleAgentConfig_FallsBackOnInvalidAgent(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Configure town settings with an invalid agent for refinery
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		constants.RoleRefinery: "nonexistent-agent-xyz", // Invalid agent
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create empty rig settings
	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	// Should fall back to default (claude) when agent is invalid
	rc := ResolveRoleAgentConfig(constants.RoleRefinery, townRoot, rigPath)
	// Command can be "claude" or full path to claude
	if rc.Command != "claude" && !strings.HasSuffix(rc.Command, "/claude") {
		t.Errorf("expected fallback to claude or path ending in /claude, got: %s", rc.Command)
	}
}

func TestGetRuntimeCommand_UsesRigAgentWhenRigPathProvided(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "gemini"
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	rigSettings := NewRigSettings()
	rigSettings.Agent = "codex"
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd := GetRuntimeCommand(rigPath)
	if !strings.HasPrefix(cmd, "codex") {
		t.Fatalf("GetRuntimeCommand() = %q, want prefix %q", cmd, "codex")
	}
}

func TestExpectedPaneCommands(t *testing.T) {
	t.Parallel()
	t.Run("claude maps to node and claude", func(t *testing.T) {
		got := ExpectedPaneCommands(&RuntimeConfig{Command: "claude"})
		want := []string{"node", "claude"}
		if len(got) != 2 || got[0] != "node" || got[1] != "claude" {
			t.Fatalf("ExpectedPaneCommands(claude) = %v, want %v", got, want)
		}
	})

	t.Run("codex maps to executable", func(t *testing.T) {
		got := ExpectedPaneCommands(&RuntimeConfig{Command: "codex"})
		if len(got) != 1 || got[0] != "codex" {
			t.Fatalf("ExpectedPaneCommands(codex) = %v, want %v", got, []string{"codex"})
		}
	})
}

func TestLoadRuntimeConfigFromSettings(t *testing.T) {
	t.Parallel()
	// Create temp rig with custom runtime config
	dir := t.TempDir()
	settingsDir := filepath.Join(dir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("creating settings dir: %v", err)
	}

	settings := NewRigSettings()
	settings.Runtime = &RuntimeConfig{
		Command: "aider",
		Args:    []string{"--no-git", "--model", "claude-3"},
	}
	if err := SaveRigSettings(filepath.Join(settingsDir, "config.json"), settings); err != nil {
		t.Fatalf("saving settings: %v", err)
	}

	// Load and verify
	rc := LoadRuntimeConfig(dir)
	if rc.Command != "aider" {
		t.Errorf("Command = %q, want %q", rc.Command, "aider")
	}
	if len(rc.Args) != 3 {
		t.Errorf("Args = %v, want 3 args", rc.Args)
	}

	cmd := rc.BuildCommand()
	if cmd != "aider --no-git --model claude-3" {
		t.Errorf("BuildCommand() = %q, want %q", cmd, "aider --no-git --model claude-3")
	}
}

func TestLoadRuntimeConfigFallsBackToDefaults(t *testing.T) {
	t.Parallel()
	// Non-existent path should use defaults
	rc := LoadRuntimeConfig("/nonexistent/path")
	if !isClaudeCommand(rc.Command) {
		t.Errorf("Command = %q, want claude or path ending in /claude (default)", rc.Command)
	}
}

func TestDaemonPatrolConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mayor", "daemon.json")

	original := NewDaemonPatrolConfig()
	original.Patrols["custom"] = PatrolConfig{
		Enabled:  true,
		Interval: "10m",
		Agent:    "custom-agent",
	}

	if err := SaveDaemonPatrolConfig(path, original); err != nil {
		t.Fatalf("SaveDaemonPatrolConfig: %v", err)
	}

	loaded, err := LoadDaemonPatrolConfig(path)
	if err != nil {
		t.Fatalf("LoadDaemonPatrolConfig: %v", err)
	}

	if loaded.Type != "daemon-patrol-config" {
		t.Errorf("Type = %q, want 'daemon-patrol-config'", loaded.Type)
	}
	if loaded.Version != CurrentDaemonPatrolConfigVersion {
		t.Errorf("Version = %d, want %d", loaded.Version, CurrentDaemonPatrolConfigVersion)
	}
	if loaded.Heartbeat == nil || !loaded.Heartbeat.Enabled {
		t.Error("Heartbeat not preserved")
	}
	if len(loaded.Patrols) != 4 {
		t.Errorf("Patrols count = %d, want 4", len(loaded.Patrols))
	}
	if custom, ok := loaded.Patrols["custom"]; !ok || custom.Agent != "custom-agent" {
		t.Error("custom patrol not preserved")
	}
}

func TestDaemonPatrolConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		config  *DaemonPatrolConfig
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  NewDaemonPatrolConfig(),
			wantErr: false,
		},
		{
			name: "valid minimal config",
			config: &DaemonPatrolConfig{
				Type:    "daemon-patrol-config",
				Version: 1,
			},
			wantErr: false,
		},
		{
			name: "wrong type",
			config: &DaemonPatrolConfig{
				Type:    "wrong",
				Version: 1,
			},
			wantErr: true,
		},
		{
			name: "future version rejected",
			config: &DaemonPatrolConfig{
				Type:    "daemon-patrol-config",
				Version: 999,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDaemonPatrolConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDaemonPatrolConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadDaemonPatrolConfigNotFound(t *testing.T) {
	t.Parallel()
	_, err := LoadDaemonPatrolConfig("/nonexistent/path.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestDaemonPatrolConfigPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		townRoot string
		expected string
	}{
		{"/home/user/gt", "/home/user/gt/mayor/daemon.json"},
		{"/var/lib/gastown", "/var/lib/gastown/mayor/daemon.json"},
		{"/tmp/test-workspace", "/tmp/test-workspace/mayor/daemon.json"},
		{"~/gt", "~/gt/mayor/daemon.json"},
	}

	for _, tt := range tests {
		t.Run(tt.townRoot, func(t *testing.T) {
			path := DaemonPatrolConfigPath(tt.townRoot)
			if filepath.ToSlash(path) != filepath.ToSlash(tt.expected) {
				t.Errorf("DaemonPatrolConfigPath(%q) = %q, want %q", tt.townRoot, path, tt.expected)
			}
		})
	}
}

func TestEnsureDaemonPatrolConfig(t *testing.T) {
	t.Parallel()
	t.Run("creates config if missing", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "mayor"), 0755); err != nil {
			t.Fatalf("creating mayor dir: %v", err)
		}

		err := EnsureDaemonPatrolConfig(dir)
		if err != nil {
			t.Fatalf("EnsureDaemonPatrolConfig: %v", err)
		}

		path := DaemonPatrolConfigPath(dir)
		loaded, err := LoadDaemonPatrolConfig(path)
		if err != nil {
			t.Fatalf("LoadDaemonPatrolConfig: %v", err)
		}
		if loaded.Type != "daemon-patrol-config" {
			t.Errorf("Type = %q, want 'daemon-patrol-config'", loaded.Type)
		}
		if len(loaded.Patrols) != 3 {
			t.Errorf("Patrols count = %d, want 3 (deacon, witness, refinery)", len(loaded.Patrols))
		}
	})

	t.Run("preserves existing config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "mayor", "daemon.json")

		existing := &DaemonPatrolConfig{
			Type:    "daemon-patrol-config",
			Version: 1,
			Patrols: map[string]PatrolConfig{
				"custom-only": {Enabled: true, Agent: "custom"},
			},
		}
		if err := SaveDaemonPatrolConfig(path, existing); err != nil {
			t.Fatalf("SaveDaemonPatrolConfig: %v", err)
		}

		err := EnsureDaemonPatrolConfig(dir)
		if err != nil {
			t.Fatalf("EnsureDaemonPatrolConfig: %v", err)
		}

		loaded, err := LoadDaemonPatrolConfig(path)
		if err != nil {
			t.Fatalf("LoadDaemonPatrolConfig: %v", err)
		}
		if len(loaded.Patrols) != 1 {
			t.Errorf("Patrols count = %d, want 1 (should preserve existing)", len(loaded.Patrols))
		}
		if _, ok := loaded.Patrols["custom-only"]; !ok {
			t.Error("existing custom patrol was overwritten")
		}
	})

}

func TestNewDaemonPatrolConfig(t *testing.T) {
	t.Parallel()
	cfg := NewDaemonPatrolConfig()

	if cfg.Type != "daemon-patrol-config" {
		t.Errorf("Type = %q, want 'daemon-patrol-config'", cfg.Type)
	}
	if cfg.Version != CurrentDaemonPatrolConfigVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, CurrentDaemonPatrolConfigVersion)
	}
	if cfg.Heartbeat == nil {
		t.Fatal("Heartbeat is nil")
	}
	if !cfg.Heartbeat.Enabled {
		t.Error("Heartbeat.Enabled should be true by default")
	}
	if cfg.Heartbeat.Interval != "3m" {
		t.Errorf("Heartbeat.Interval = %q, want '3m'", cfg.Heartbeat.Interval)
	}
	if len(cfg.Patrols) != 3 {
		t.Errorf("Patrols count = %d, want 3", len(cfg.Patrols))
	}

	for _, name := range []string{"deacon", "witness", "refinery"} {
		patrol, ok := cfg.Patrols[name]
		if !ok {
			t.Errorf("missing %s patrol", name)
			continue
		}
		if !patrol.Enabled {
			t.Errorf("%s patrol should be enabled by default", name)
		}
		if patrol.Agent != name {
			t.Errorf("%s patrol Agent = %q, want %q", name, patrol.Agent, name)
		}
	}
}

func TestSaveTownSettings(t *testing.T) {
	t.Parallel()
	t.Run("saves valid town settings", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "settings", "config.json")

		settings := &TownSettings{
			Type:         "town-settings",
			Version:      CurrentTownSettingsVersion,
			DefaultAgent: "gemini",
			Agents: map[string]*RuntimeConfig{
				"my-agent": {
					Command: "my-agent",
					Args:    []string{"--arg1", "--arg2"},
				},
			},
		}

		err := SaveTownSettings(settingsPath, settings)
		if err != nil {
			t.Fatalf("SaveTownSettings failed: %v", err)
		}

		// Verify file exists
		data, err := os.ReadFile(settingsPath)
		if err != nil {
			t.Fatalf("reading settings file: %v", err)
		}

		// Verify it contains expected content
		content := string(data)
		if !strings.Contains(content, `"type": "town-settings"`) {
			t.Errorf("missing type field")
		}
		if !strings.Contains(content, `"default_agent": "gemini"`) {
			t.Errorf("missing default_agent field")
		}
		if !strings.Contains(content, `"my-agent"`) {
			t.Errorf("missing custom agent")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "deeply", "nested", "settings", "config.json")

		settings := NewTownSettings()

		err := SaveTownSettings(settingsPath, settings)
		if err != nil {
			t.Fatalf("SaveTownSettings failed: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(settingsPath); err != nil {
			t.Errorf("settings file not created: %v", err)
		}
	})

	t.Run("rejects invalid type", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "config.json")

		settings := &TownSettings{
			Type:    "invalid-type",
			Version: CurrentTownSettingsVersion,
		}

		err := SaveTownSettings(settingsPath, settings)
		if err == nil {
			t.Error("expected error for invalid type")
		}
	})

	t.Run("rejects unsupported version", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "config.json")

		settings := &TownSettings{
			Type:    "town-settings",
			Version: CurrentTownSettingsVersion + 100,
		}

		err := SaveTownSettings(settingsPath, settings)
		if err == nil {
			t.Error("expected error for unsupported version")
		}
	})

	t.Run("roundtrip save and load", func(t *testing.T) {
		tmpDir := t.TempDir()
		settingsPath := filepath.Join(tmpDir, "config.json")

		original := &TownSettings{
			Type:         "town-settings",
			Version:      CurrentTownSettingsVersion,
			DefaultAgent: "codex",
			Agents: map[string]*RuntimeConfig{
				"custom-1": {
					Command: "custom-agent",
					Args:    []string{"--flag"},
				},
			},
		}

		err := SaveTownSettings(settingsPath, original)
		if err != nil {
			t.Fatalf("SaveTownSettings failed: %v", err)
		}

		loaded, err := LoadOrCreateTownSettings(settingsPath)
		if err != nil {
			t.Fatalf("LoadOrCreateTownSettings failed: %v", err)
		}

		if loaded.Type != original.Type {
			t.Errorf("Type = %q, want %q", loaded.Type, original.Type)
		}
		if loaded.Version != original.Version {
			t.Errorf("Version = %d, want %d", loaded.Version, original.Version)
		}
		if loaded.DefaultAgent != original.DefaultAgent {
			t.Errorf("DefaultAgent = %q, want %q", loaded.DefaultAgent, original.DefaultAgent)
		}

		if len(loaded.Agents) != len(original.Agents) {
			t.Errorf("Agents count = %d, want %d", len(loaded.Agents), len(original.Agents))
		}
	})
}

func TestGetDefaultFormula(t *testing.T) {
	t.Parallel()
	t.Run("returns empty string for nonexistent rig", func(t *testing.T) {
		result := GetDefaultFormula("/nonexistent/path")
		if result != "" {
			t.Errorf("GetDefaultFormula() = %q, want empty string", result)
		}
	})

	t.Run("returns empty string when no workflow config", func(t *testing.T) {
		dir := t.TempDir()
		settings := NewRigSettings()
		if err := SaveRigSettings(RigSettingsPath(dir), settings); err != nil {
			t.Fatalf("SaveRigSettings: %v", err)
		}

		result := GetDefaultFormula(dir)
		if result != "" {
			t.Errorf("GetDefaultFormula() = %q, want empty string", result)
		}
	})

	t.Run("returns default formula when configured", func(t *testing.T) {
		dir := t.TempDir()
		settings := NewRigSettings()
		settings.Workflow = &WorkflowConfig{
			DefaultFormula: "shiny",
		}
		if err := SaveRigSettings(RigSettingsPath(dir), settings); err != nil {
			t.Fatalf("SaveRigSettings: %v", err)
		}

		result := GetDefaultFormula(dir)
		if result != "shiny" {
			t.Errorf("GetDefaultFormula() = %q, want %q", result, "shiny")
		}
	})
}

// TestLookupAgentConfigWithRigSettings verifies that lookupAgentConfig checks
// rig-level agents first, then town-level agents, then built-ins.
func TestLookupAgentConfigWithRigSettings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		rigSettings     *RigSettings
		townSettings    *TownSettings
		expectedCommand string
		expectedFrom    string
	}{
		{
			name: "rig-custom-agent",
			rigSettings: &RigSettings{
				Agent: "default-rig-agent",
				Agents: map[string]*RuntimeConfig{
					"rig-custom-agent": {
						Command: "custom-rig-cmd",
						Args:    []string{"--rig-flag"},
					},
				},
			},
			townSettings: &TownSettings{
				Agents: map[string]*RuntimeConfig{
					"town-custom-agent": {
						Command: "custom-town-cmd",
						Args:    []string{"--town-flag"},
					},
				},
			},
			expectedCommand: "custom-rig-cmd",
			expectedFrom:    "rig",
		},
		{
			name: "town-custom-agent",
			rigSettings: &RigSettings{
				Agents: map[string]*RuntimeConfig{
					"other-rig-agent": {
						Command: "other-rig-cmd",
					},
				},
			},
			townSettings: &TownSettings{
				Agents: map[string]*RuntimeConfig{
					"town-custom-agent": {
						Command: "custom-town-cmd",
						Args:    []string{"--town-flag"},
					},
				},
			},
			expectedCommand: "custom-town-cmd",
			expectedFrom:    "town",
		},
		{
			name:            "unknown-agent",
			rigSettings:     nil,
			townSettings:    nil,
			expectedCommand: "claude",
			expectedFrom:    "builtin",
		},
		{
			name: "claude",
			rigSettings: &RigSettings{
				Agent: "claude",
			},
			townSettings: &TownSettings{
				Agents: map[string]*RuntimeConfig{
					"claude": {
						Command: "custom-claude",
					},
				},
			},
			expectedCommand: "custom-claude",
			expectedFrom:    "town",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := lookupAgentConfig(tt.name, tt.townSettings, tt.rigSettings)

			if rc == nil {
				t.Errorf("lookupAgentConfig(%s) returned nil", tt.name)
			}

			// For claude commands, allow either "claude" or path ending in /claude
			if tt.expectedCommand == "claude" {
				if !isClaudeCommand(rc.Command) {
					t.Errorf("lookupAgentConfig(%s).Command = %s, want claude or path ending in /claude", tt.name, rc.Command)
				}
			} else if rc.Command != tt.expectedCommand {
				t.Errorf("lookupAgentConfig(%s).Command = %s, want %s", tt.name, rc.Command, tt.expectedCommand)
			}
		})
	}
}

// TestFillRuntimeDefaults tests the fillRuntimeDefaults function comprehensively.
func TestFillRuntimeDefaults(t *testing.T) {
	t.Parallel()

	t.Run("preserves all fields", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Provider:      "codex",
			Command:       "opencode",
			Args:          []string{"-m", "gpt-5"},
			Env:           map[string]string{"OPENCODE_PERMISSION": `{"*":"allow"}`},
			InitialPrompt: "test prompt",
			PromptMode:    "none",
			Session: &RuntimeSessionConfig{
				SessionIDEnv: "OPENCODE_SESSION_ID",
			},
			Hooks: &RuntimeHooksConfig{
				Provider: "opencode",
			},
			Tmux: &RuntimeTmuxConfig{
				ProcessNames: []string{"opencode", "node"},
			},
			Instructions: &RuntimeInstructionsConfig{
				File: "OPENCODE.md",
			},
		}

		result := fillRuntimeDefaults(input)

		if result.Provider != input.Provider {
			t.Errorf("Provider: got %q, want %q", result.Provider, input.Provider)
		}
		if result.Command != input.Command {
			t.Errorf("Command: got %q, want %q", result.Command, input.Command)
		}
		if len(result.Args) != len(input.Args) {
			t.Errorf("Args: got %v, want %v", result.Args, input.Args)
		}
		if result.Env["OPENCODE_PERMISSION"] != input.Env["OPENCODE_PERMISSION"] {
			t.Errorf("Env: got %v, want %v", result.Env, input.Env)
		}
		if result.InitialPrompt != input.InitialPrompt {
			t.Errorf("InitialPrompt: got %q, want %q", result.InitialPrompt, input.InitialPrompt)
		}
		if result.PromptMode != input.PromptMode {
			t.Errorf("PromptMode: got %q, want %q", result.PromptMode, input.PromptMode)
		}
		if result.Session == nil || result.Session.SessionIDEnv != input.Session.SessionIDEnv {
			t.Errorf("Session: got %+v, want %+v", result.Session, input.Session)
		}
		if result.Hooks == nil || result.Hooks.Provider != input.Hooks.Provider {
			t.Errorf("Hooks: got %+v, want %+v", result.Hooks, input.Hooks)
		}
		if result.Tmux == nil || len(result.Tmux.ProcessNames) != len(input.Tmux.ProcessNames) {
			t.Errorf("Tmux: got %+v, want %+v", result.Tmux, input.Tmux)
		}
		if result.Instructions == nil || result.Instructions.File != input.Instructions.File {
			t.Errorf("Instructions: got %+v, want %+v", result.Instructions, input.Instructions)
		}
	})

	t.Run("nil input returns defaults", func(t *testing.T) {
		t.Parallel()
		result := fillRuntimeDefaults(nil)

		if result == nil {
			t.Fatal("fillRuntimeDefaults(nil) returned nil")
		}
		if result.Command == "" {
			t.Error("Command should have default value")
		}
	})

	t.Run("empty command defaults to claude", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "",
			Args:    []string{"--custom-flag"},
		}

		result := fillRuntimeDefaults(input)

		// Use isClaudeCommand to handle resolved paths (e.g., /opt/homebrew/bin/claude)
		if !isClaudeCommand(result.Command) {
			t.Errorf("Command: got %q, want claude or path ending in claude", result.Command)
		}
		// Args should be preserved, not overwritten
		if len(result.Args) != 1 || result.Args[0] != "--custom-flag" {
			t.Errorf("Args should be preserved: got %v", result.Args)
		}
	})

	t.Run("nil args defaults to skip-permissions", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "claude",
			Args:    nil,
		}

		result := fillRuntimeDefaults(input)

		if result.Args == nil || len(result.Args) == 0 {
			t.Error("Args should have default value")
		}
		if result.Args[0] != "--dangerously-skip-permissions" {
			t.Errorf("Args: got %v, want [--dangerously-skip-permissions]", result.Args)
		}
	})

	t.Run("empty args slice is preserved", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "claude",
			Args:    []string{}, // Explicitly empty, not nil
		}

		result := fillRuntimeDefaults(input)

		// Empty slice means "no args", not "use defaults"
		// This is intentional per RuntimeConfig docs
		if result.Args == nil {
			t.Error("Empty Args slice should be preserved as empty, not nil")
		}
	})

	t.Run("env map is copied not shared", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "opencode",
			Env:     map[string]string{"KEY": "value"},
		}

		result := fillRuntimeDefaults(input)

		// Modify result's env
		result.Env["NEW_KEY"] = "new_value"

		// Original should be unchanged
		if _, ok := input.Env["NEW_KEY"]; ok {
			t.Error("Env map was not copied - modifications affect original")
		}
	})

	t.Run("prompt_mode none is preserved for custom agents", func(t *testing.T) {
		t.Parallel()
		// This is the specific bug that was fixed - opencode needs prompt_mode: "none"
		// to prevent the startup beacon from being passed as an argument
		input := &RuntimeConfig{
			Provider:   "opencode",
			Command:    "opencode",
			Args:       []string{"-m", "gpt-5"},
			PromptMode: "none",
		}

		result := fillRuntimeDefaults(input)

		if result.PromptMode != "none" {
			t.Errorf("PromptMode: got %q, want %q - custom prompt_mode was not preserved", result.PromptMode, "none")
		}
	})

	t.Run("args slice is deep copied not shared", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "opencode",
			Args:    []string{"original-arg"},
		}

		result := fillRuntimeDefaults(input)

		// Modify result's args
		result.Args[0] = "modified-arg"

		// Original should be unchanged
		if input.Args[0] != "original-arg" {
			t.Errorf("Args slice was not deep copied - modifications affect original: got %q, want %q",
				input.Args[0], "original-arg")
		}
	})

	t.Run("session struct is deep copied", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "claude",
			Session: &RuntimeSessionConfig{
				SessionIDEnv: "ORIGINAL_SESSION_ID",
				ConfigDirEnv: "ORIGINAL_CONFIG_DIR",
			},
		}

		result := fillRuntimeDefaults(input)

		// Modify result's session
		result.Session.SessionIDEnv = "MODIFIED_SESSION_ID"

		// Original should be unchanged
		if input.Session.SessionIDEnv != "ORIGINAL_SESSION_ID" {
			t.Errorf("Session struct was not deep copied - modifications affect original: got %q, want %q",
				input.Session.SessionIDEnv, "ORIGINAL_SESSION_ID")
		}
	})

	t.Run("hooks struct is deep copied", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "claude",
			Hooks: &RuntimeHooksConfig{
				Provider:     "original-provider",
				Dir:          "original-dir",
				SettingsFile: "original-file",
			},
		}

		result := fillRuntimeDefaults(input)

		// Modify result's hooks
		result.Hooks.Provider = "modified-provider"

		// Original should be unchanged
		if input.Hooks.Provider != "original-provider" {
			t.Errorf("Hooks struct was not deep copied - modifications affect original: got %q, want %q",
				input.Hooks.Provider, "original-provider")
		}
	})

	t.Run("tmux struct and process_names are deep copied", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "opencode",
			Tmux: &RuntimeTmuxConfig{
				ProcessNames:      []string{"original-process"},
				ReadyPromptPrefix: "original-prefix",
				ReadyDelayMs:      5000,
			},
		}

		result := fillRuntimeDefaults(input)

		// Modify result's tmux
		result.Tmux.ProcessNames[0] = "modified-process"
		result.Tmux.ReadyPromptPrefix = "modified-prefix"

		// Original should be unchanged
		if input.Tmux.ProcessNames[0] != "original-process" {
			t.Errorf("Tmux.ProcessNames was not deep copied - modifications affect original: got %q, want %q",
				input.Tmux.ProcessNames[0], "original-process")
		}
		if input.Tmux.ReadyPromptPrefix != "original-prefix" {
			t.Errorf("Tmux struct was not deep copied - modifications affect original: got %q, want %q",
				input.Tmux.ReadyPromptPrefix, "original-prefix")
		}
	})

	t.Run("instructions struct is deep copied", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "opencode",
			Instructions: &RuntimeInstructionsConfig{
				File: "ORIGINAL.md",
			},
		}

		result := fillRuntimeDefaults(input)

		// Modify result's instructions
		result.Instructions.File = "MODIFIED.md"

		// Original should be unchanged
		if input.Instructions.File != "ORIGINAL.md" {
			t.Errorf("Instructions struct was not deep copied - modifications affect original: got %q, want %q",
				input.Instructions.File, "ORIGINAL.md")
		}
	})

	t.Run("nil nested structs remain nil", func(t *testing.T) {
		t.Parallel()
		input := &RuntimeConfig{
			Command: "claude",
			// All nested structs left nil
		}

		result := fillRuntimeDefaults(input)

		// Nil nested structs should remain nil (not get zero-value structs)
		if result.Session != nil {
			t.Error("Session should remain nil when input has nil Session")
		}
		if result.Hooks != nil {
			t.Error("Hooks should remain nil when input has nil Hooks")
		}
		if result.Tmux != nil {
			t.Error("Tmux should remain nil when input has nil Tmux")
		}
		if result.Instructions != nil {
			t.Error("Instructions should remain nil when input has nil Instructions")
		}
	})

	t.Run("partial nested struct is copied without defaults", func(t *testing.T) {
		t.Parallel()
		// User defines partial Tmux config - only ProcessNames, no other fields
		input := &RuntimeConfig{
			Command: "opencode",
			Tmux: &RuntimeTmuxConfig{
				ProcessNames: []string{"opencode"},
				// ReadyPromptPrefix and ReadyDelayMs left at zero values
			},
		}

		result := fillRuntimeDefaults(input)

		// ProcessNames should be copied
		if len(result.Tmux.ProcessNames) != 1 || result.Tmux.ProcessNames[0] != "opencode" {
			t.Errorf("Tmux.ProcessNames not copied correctly: got %v", result.Tmux.ProcessNames)
		}
		// Zero values should remain zero (fillRuntimeDefaults doesn't fill nested defaults)
		if result.Tmux.ReadyDelayMs != 0 {
			t.Errorf("Tmux.ReadyDelayMs should be 0 (unfilled), got %d", result.Tmux.ReadyDelayMs)
		}
	})
}

// TestLookupAgentConfigPreservesCustomFields verifies that custom agents
// have all their settings preserved through the lookup chain.
func TestLookupAgentConfigPreservesCustomFields(t *testing.T) {
	t.Parallel()

	townSettings := &TownSettings{
		Type:         "town-settings",
		Version:      1,
		DefaultAgent: "claude",
		Agents: map[string]*RuntimeConfig{
			"opencode-mayor": {
				Command:    "opencode",
				Args:       []string{"-m", "gpt-5"},
				PromptMode: "none",
				Env:        map[string]string{"OPENCODE_PERMISSION": `{"*":"allow"}`},
				Tmux: &RuntimeTmuxConfig{
					ProcessNames: []string{"opencode", "node"},
				},
			},
		},
	}

	rc := lookupAgentConfig("opencode-mayor", townSettings, nil)

	if rc == nil {
		t.Fatal("lookupAgentConfig returned nil for custom agent")
	}
	if rc.PromptMode != "none" {
		t.Errorf("PromptMode: got %q, want %q - setting was lost in lookup chain", rc.PromptMode, "none")
	}
	if rc.Command != "opencode" {
		t.Errorf("Command: got %q, want %q", rc.Command, "opencode")
	}
	if rc.Env["OPENCODE_PERMISSION"] != `{"*":"allow"}` {
		t.Errorf("Env was not preserved: got %v", rc.Env)
	}
	if rc.Tmux == nil || len(rc.Tmux.ProcessNames) != 2 {
		t.Errorf("Tmux.ProcessNames not preserved: got %+v", rc.Tmux)
	}
}

// TestBuildCommandWithPromptRespectsPromptModeNone verifies that when PromptMode
// is "none", the prompt is not appended to the command.
func TestBuildCommandWithPromptRespectsPromptModeNone(t *testing.T) {
	t.Parallel()

	rc := &RuntimeConfig{
		Command:    "opencode",
		Args:       []string{"-m", "gpt-5"},
		PromptMode: "none",
	}

	// Build command with a prompt that should be ignored
	cmd := rc.BuildCommandWithPrompt("This prompt should not appear")

	if strings.Contains(cmd, "This prompt should not appear") {
		t.Errorf("prompt_mode=none should prevent prompt from being added, got: %s", cmd)
	}
	if !strings.HasPrefix(cmd, "opencode") {
		t.Errorf("Command should start with opencode, got: %s", cmd)
	}
}

// TestRoleAgentConfigWithCustomAgent tests role-based agent resolution with
// custom agents that have special settings like prompt_mode: "none".
//
// This test mirrors manual verification using settings/config.json:
//
//	{
//	  "type": "town-settings",
//	  "version": 1,
//	  "default_agent": "claude-opus",
//	  "agents": {
//	    "amp-yolo": {
//	      "command": "amp",
//	      "args": ["--dangerously-allow-all"]
//	    },
//	    "opencode-mayor": {
//	      "command": "opencode",
//	      "args": ["-m", "openai/gpt-5.2-codex"],
//	      "prompt_mode": "none",
//	      "process_names": ["opencode", "node"],
//	      "env": {
//	        "OPENCODE_PERMISSION": "{\"*\":\"allow\"}"
//	      }
//	    }
//	  },
//	  "role_agents": {
//	    "crew": "claude-sonnet",
//	    "deacon": "claude-haiku",
//	    "mayor": "opencode-mayor",
//	    "polecat": "claude-opus",
//	    "refinery": "claude-opus",
//	    "witness": "claude-sonnet"
//	  }
//	}
//
// Manual test procedure:
//  1. Set role_agents.mayor to each agent (claude, gemini, codex, cursor, auggie, amp, opencode)
//  2. Run: gt start
//  3. Verify mayor starts with correct agent config
//  4. Run: GT_NUKE_ACKNOWLEDGED=1 gt down --nuke
//  5. Repeat for all 7 built-in agents
func TestRoleAgentConfigWithCustomAgent(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create town settings mirroring the manual test config
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude-opus"
	townSettings.RoleAgents = map[string]string{
		constants.RoleMayor:    "opencode-mayor",
		constants.RoleDeacon:   "claude-haiku",
		constants.RolePolecat:  "claude-opus",
		constants.RoleRefinery: "claude-opus",
		constants.RoleWitness:  "claude-sonnet",
		constants.RoleCrew:     "claude-sonnet",
	}
	townSettings.Agents = map[string]*RuntimeConfig{
		"opencode-mayor": {
			Command:    "opencode",
			Args:       []string{"-m", "openai/gpt-5.2-codex"},
			PromptMode: "none",
			Env:        map[string]string{"OPENCODE_PERMISSION": `{"*":"allow"}`},
			Tmux: &RuntimeTmuxConfig{
				ProcessNames: []string{"opencode", "node"},
			},
		},
		"amp-yolo": {
			Command: "amp",
			Args:    []string{"--dangerously-allow-all"},
		},
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create minimal rig settings
	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	// Test mayor role gets opencode-mayor with prompt_mode: none
	t.Run("mayor gets opencode-mayor config", func(t *testing.T) {
		rc := ResolveRoleAgentConfig(constants.RoleMayor, townRoot, rigPath)
		if rc == nil {
			t.Fatal("ResolveRoleAgentConfig returned nil for mayor")
		}
		if rc.Command != "opencode" {
			t.Errorf("Command: got %q, want %q", rc.Command, "opencode")
		}
		if rc.PromptMode != "none" {
			t.Errorf("PromptMode: got %q, want %q - critical for opencode", rc.PromptMode, "none")
		}
		if rc.Env["OPENCODE_PERMISSION"] != `{"*":"allow"}` {
			t.Errorf("Env not preserved: got %v", rc.Env)
		}

		// Verify startup beacon is NOT added to command
		cmd := rc.BuildCommandWithPrompt("[GAS TOWN] mayor <- human • cold-start")
		if strings.Contains(cmd, "GAS TOWN") {
			t.Errorf("prompt_mode=none should prevent beacon, got: %s", cmd)
		}
	})

	// Test other roles get their configured agents
	t.Run("deacon gets claude-haiku", func(t *testing.T) {
		rc := ResolveRoleAgentConfig(constants.RoleDeacon, townRoot, rigPath)
		if rc == nil {
			t.Fatal("ResolveRoleAgentConfig returned nil for deacon")
		}
		// claude-haiku is a built-in preset
		if !strings.Contains(rc.Command, "claude") && rc.Command != "claude" {
			t.Errorf("Command: got %q, want claude-based command", rc.Command)
		}
	})

	t.Run("polecat gets claude-opus", func(t *testing.T) {
		rc := ResolveRoleAgentConfig(constants.RolePolecat, townRoot, rigPath)
		if rc == nil {
			t.Fatal("ResolveRoleAgentConfig returned nil for polecat")
		}
		if !strings.Contains(rc.Command, "claude") && rc.Command != "claude" {
			t.Errorf("Command: got %q, want claude-based command", rc.Command)
		}
	})
}

// TestMultipleAgentTypes tests that various built-in agent presets work correctly.
// NOTE: Only these are actual built-in presets: claude, gemini, codex, cursor, auggie, amp, opencode.
// Variants like "claude-opus", "claude-haiku", "claude-sonnet" are NOT built-in - they need
// to be defined as custom agents in TownSettings.Agents if specific model selection is needed.
func TestMultipleAgentTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		agentName     string
		expectCommand string
		isBuiltIn     bool // true if this is an actual built-in preset
	}{
		{
			name:          "claude built-in preset",
			agentName:     "claude",
			expectCommand: "claude",
			isBuiltIn:     true,
		},
		{
			name:          "codex built-in preset",
			agentName:     "codex",
			expectCommand: "codex",
			isBuiltIn:     true,
		},
		{
			name:          "gemini built-in preset",
			agentName:     "gemini",
			expectCommand: "gemini",
			isBuiltIn:     true,
		},
		{
			name:          "amp built-in preset",
			agentName:     "amp",
			expectCommand: "amp",
			isBuiltIn:     true,
		},
		{
			name:          "opencode built-in preset",
			agentName:     "opencode",
			expectCommand: "opencode",
			isBuiltIn:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Skip if agent binary not installed (prevents flaky CI failures)
			skipIfAgentBinaryMissing(t, tc.agentName)

			// Verify it's actually a built-in preset
			if tc.isBuiltIn {
				preset := GetAgentPresetByName(tc.agentName)
				if preset == nil {
					t.Errorf("%s should be a built-in preset but GetAgentPresetByName returned nil", tc.agentName)
					return
				}
			}

			townRoot := t.TempDir()
			rigPath := filepath.Join(townRoot, "testrig")

			townSettings := NewTownSettings()
			townSettings.DefaultAgent = "claude"
			townSettings.RoleAgents = map[string]string{
				constants.RoleMayor: tc.agentName,
			}
			if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
				t.Fatalf("SaveTownSettings: %v", err)
			}

			rigSettings := NewRigSettings()
			if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
				t.Fatalf("SaveRigSettings: %v", err)
			}

			rc := ResolveRoleAgentConfig(constants.RoleMayor, townRoot, rigPath)
			if rc == nil {
				t.Fatalf("ResolveRoleAgentConfig returned nil for %s", tc.agentName)
			}

			// Allow path-based commands (e.g., /opt/homebrew/bin/claude)
			if !strings.Contains(rc.Command, tc.expectCommand) {
				t.Errorf("Command: got %q, want command containing %q", rc.Command, tc.expectCommand)
			}
		})
	}
}

// TestCustomClaudeVariants tests that Claude model variants (opus, sonnet, haiku) need
// to be explicitly defined as custom agents since they are NOT built-in presets.
func TestCustomClaudeVariants(t *testing.T) {
	t.Parallel()

	// Verify that claude-opus/sonnet/haiku are NOT built-in presets
	variants := []string{"claude-opus", "claude-sonnet", "claude-haiku"}
	for _, variant := range variants {
		if preset := GetAgentPresetByName(variant); preset != nil {
			t.Errorf("%s should NOT be a built-in preset (only 'claude' is), but GetAgentPresetByName returned non-nil", variant)
		}
	}

	// Test that custom claude variants work when explicitly defined
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		constants.RoleMayor:  "claude-opus",
		constants.RoleDeacon: "claude-haiku",
	}
	// Define the custom variants
	townSettings.Agents = map[string]*RuntimeConfig{
		"claude-opus": {
			Command: "claude",
			Args:    []string{"--model", "claude-opus-4", "--dangerously-skip-permissions"},
		},
		"claude-haiku": {
			Command: "claude",
			Args:    []string{"--model", "claude-haiku-3", "--dangerously-skip-permissions"},
		},
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	// Test claude-opus custom agent
	rc := ResolveRoleAgentConfig(constants.RoleMayor, townRoot, rigPath)
	if rc == nil {
		t.Fatal("ResolveRoleAgentConfig returned nil for claude-opus")
	}
	if !strings.Contains(rc.Command, "claude") {
		t.Errorf("claude-opus Command: got %q, want claude", rc.Command)
	}
	foundModel := false
	for _, arg := range rc.Args {
		if arg == "claude-opus-4" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Errorf("claude-opus Args should contain model flag: got %v", rc.Args)
	}

	// Test claude-haiku custom agent
	rc = ResolveRoleAgentConfig(constants.RoleDeacon, townRoot, rigPath)
	if rc == nil {
		t.Fatal("ResolveRoleAgentConfig returned nil for claude-haiku")
	}
	foundModel = false
	for _, arg := range rc.Args {
		if arg == "claude-haiku-3" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Errorf("claude-haiku Args should contain model flag: got %v", rc.Args)
	}
}

// TestCustomAgentWithAmp tests custom agent configuration for amp.
// This mirrors the manual test: amp-yolo started successfully with custom args.
func TestCustomAgentWithAmp(t *testing.T) {
	skipIfAgentBinaryMissing(t, "amp")
	t.Parallel()

	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		constants.RoleMayor: "amp-yolo",
	}
	townSettings.Agents = map[string]*RuntimeConfig{
		"amp-yolo": {
			Command: "amp",
			Args:    []string{"--dangerously-allow-all"},
		},
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	rc := ResolveRoleAgentConfig(constants.RoleMayor, townRoot, rigPath)
	if rc == nil {
		t.Fatal("ResolveRoleAgentConfig returned nil for amp-yolo")
	}

	if rc.Command != "amp" {
		t.Errorf("Command: got %q, want %q", rc.Command, "amp")
	}
	if len(rc.Args) != 1 || rc.Args[0] != "--dangerously-allow-all" {
		t.Errorf("Args: got %v, want [--dangerously-allow-all]", rc.Args)
	}

	// Verify command generation
	cmd := rc.BuildCommand()
	if !strings.Contains(cmd, "amp") {
		t.Errorf("BuildCommand should contain amp, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--dangerously-allow-all") {
		t.Errorf("BuildCommand should contain custom args, got: %s", cmd)
	}
}

func TestResolveRoleAgentConfig(t *testing.T) {
	skipIfAgentBinaryMissing(t, "gemini", "codex")
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create town settings with role-specific agents
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		"mayor":   "claude", // mayor uses default claude
		"witness": "gemini", // witness uses gemini
		"polecat": "codex",  // polecats use codex
	}
	townSettings.Agents = map[string]*RuntimeConfig{
		"claude-haiku": {
			Command: "claude",
			Args:    []string{"--model", "haiku", "--dangerously-skip-permissions"},
		},
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create rig settings that override some roles
	rigSettings := NewRigSettings()
	rigSettings.Agent = "gemini" // default for this rig
	rigSettings.RoleAgents = map[string]string{
		"witness": "claude-haiku", // override witness to use haiku
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	t.Run("rig RoleAgents overrides town RoleAgents", func(t *testing.T) {
		rc := ResolveRoleAgentConfig("witness", townRoot, rigPath)
		// Should get claude-haiku from rig's RoleAgents
		if !isClaudeCommand(rc.Command) {
			t.Errorf("Command = %q, want claude or path ending in /claude", rc.Command)
		}
		cmd := rc.BuildCommand()
		if !strings.Contains(cmd, "--model haiku") {
			t.Errorf("BuildCommand() = %q, should contain --model haiku", cmd)
		}
	})

	t.Run("town RoleAgents used when rig has no override", func(t *testing.T) {
		rc := ResolveRoleAgentConfig("polecat", townRoot, rigPath)
		// Should get codex from town's RoleAgents (rig doesn't override polecat)
		if rc.Command != "codex" {
			t.Errorf("Command = %q, want %q", rc.Command, "codex")
		}
	})

	t.Run("falls back to default agent when role not in RoleAgents", func(t *testing.T) {
		rc := ResolveRoleAgentConfig("crew", townRoot, rigPath)
		// crew is not in any RoleAgents, should use rig's default agent (gemini)
		if rc.Command != "gemini" {
			t.Errorf("Command = %q, want %q", rc.Command, "gemini")
		}
	})

	t.Run("town-level role (no rigPath) uses town RoleAgents", func(t *testing.T) {
		rc := ResolveRoleAgentConfig("mayor", townRoot, "")
		// mayor is in town's RoleAgents - command can be "claude" or full path to claude
		if rc.Command != "claude" && !strings.HasSuffix(rc.Command, "/claude") {
			t.Errorf("Command = %q, want claude or path ending in /claude", rc.Command)
		}
	})
}

func TestResolveRoleAgentName(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create town settings with role-specific agents
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		"witness": "gemini",
		"polecat": "codex",
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create rig settings
	rigSettings := NewRigSettings()
	rigSettings.Agent = "amp"
	rigSettings.RoleAgents = map[string]string{
		"witness": "cursor", // override witness
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	t.Run("rig role-specific agent", func(t *testing.T) {
		name, isRoleSpecific := ResolveRoleAgentName("witness", townRoot, rigPath)
		if name != "cursor" {
			t.Errorf("name = %q, want %q", name, "cursor")
		}
		if !isRoleSpecific {
			t.Error("isRoleSpecific = false, want true")
		}
	})

	t.Run("town role-specific agent", func(t *testing.T) {
		name, isRoleSpecific := ResolveRoleAgentName("polecat", townRoot, rigPath)
		if name != "codex" {
			t.Errorf("name = %q, want %q", name, "codex")
		}
		if !isRoleSpecific {
			t.Error("isRoleSpecific = false, want true")
		}
	})

	t.Run("falls back to rig default agent", func(t *testing.T) {
		name, isRoleSpecific := ResolveRoleAgentName("crew", townRoot, rigPath)
		if name != "amp" {
			t.Errorf("name = %q, want %q", name, "amp")
		}
		if isRoleSpecific {
			t.Error("isRoleSpecific = true, want false")
		}
	})

	t.Run("falls back to town default agent when no rig path", func(t *testing.T) {
		name, isRoleSpecific := ResolveRoleAgentName("refinery", townRoot, "")
		if name != "claude" {
			t.Errorf("name = %q, want %q", name, "claude")
		}
		if isRoleSpecific {
			t.Error("isRoleSpecific = true, want false")
		}
	})
}

func TestRoleAgentsRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	townSettingsPath := filepath.Join(dir, "settings", "config.json")
	rigSettingsPath := filepath.Join(dir, "rig", "settings", "config.json")

	// Test TownSettings with RoleAgents
	t.Run("town settings with role_agents", func(t *testing.T) {
		original := NewTownSettings()
		original.RoleAgents = map[string]string{
			"mayor":   "claude-opus",
			"witness": "claude-haiku",
			"polecat": "claude-sonnet",
		}

		if err := SaveTownSettings(townSettingsPath, original); err != nil {
			t.Fatalf("SaveTownSettings: %v", err)
		}

		loaded, err := LoadOrCreateTownSettings(townSettingsPath)
		if err != nil {
			t.Fatalf("LoadOrCreateTownSettings: %v", err)
		}

		if len(loaded.RoleAgents) != 3 {
			t.Errorf("RoleAgents count = %d, want 3", len(loaded.RoleAgents))
		}
		if loaded.RoleAgents["mayor"] != "claude-opus" {
			t.Errorf("RoleAgents[mayor] = %q, want %q", loaded.RoleAgents["mayor"], "claude-opus")
		}
		if loaded.RoleAgents["witness"] != "claude-haiku" {
			t.Errorf("RoleAgents[witness] = %q, want %q", loaded.RoleAgents["witness"], "claude-haiku")
		}
		if loaded.RoleAgents["polecat"] != "claude-sonnet" {
			t.Errorf("RoleAgents[polecat] = %q, want %q", loaded.RoleAgents["polecat"], "claude-sonnet")
		}
	})

	// Test RigSettings with RoleAgents
	t.Run("rig settings with role_agents", func(t *testing.T) {
		original := NewRigSettings()
		original.RoleAgents = map[string]string{
			"witness": "gemini",
			"crew":    "codex",
		}

		if err := SaveRigSettings(rigSettingsPath, original); err != nil {
			t.Fatalf("SaveRigSettings: %v", err)
		}

		loaded, err := LoadRigSettings(rigSettingsPath)
		if err != nil {
			t.Fatalf("LoadRigSettings: %v", err)
		}

		if len(loaded.RoleAgents) != 2 {
			t.Errorf("RoleAgents count = %d, want 2", len(loaded.RoleAgents))
		}
		if loaded.RoleAgents["witness"] != "gemini" {
			t.Errorf("RoleAgents[witness] = %q, want %q", loaded.RoleAgents["witness"], "gemini")
		}
		if loaded.RoleAgents["crew"] != "codex" {
			t.Errorf("RoleAgents[crew] = %q, want %q", loaded.RoleAgents["crew"], "codex")
		}
	})
}

// Escalation config tests

func TestEscalationConfigRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings", "escalation.json")

	original := &EscalationConfig{
		Type:    "escalation",
		Version: CurrentEscalationVersion,
		Routes: map[string][]string{
			SeverityLow:      {"bead"},
			SeverityMedium:   {"bead", "mail:mayor"},
			SeverityHigh:     {"bead", "mail:mayor", "email:human"},
			SeverityCritical: {"bead", "mail:mayor", "email:human", "sms:human"},
		},
		Contacts: EscalationContacts{
			HumanEmail: "test@example.com",
			HumanSMS:   "+15551234567",
		},
		StaleThreshold:   "2h",
		MaxReescalations: 3,
	}

	if err := SaveEscalationConfig(path, original); err != nil {
		t.Fatalf("SaveEscalationConfig: %v", err)
	}

	loaded, err := LoadEscalationConfig(path)
	if err != nil {
		t.Fatalf("LoadEscalationConfig: %v", err)
	}

	if loaded.Type != original.Type {
		t.Errorf("Type = %q, want %q", loaded.Type, original.Type)
	}
	if loaded.Version != original.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, original.Version)
	}
	if loaded.StaleThreshold != original.StaleThreshold {
		t.Errorf("StaleThreshold = %q, want %q", loaded.StaleThreshold, original.StaleThreshold)
	}
	if loaded.MaxReescalations != original.MaxReescalations {
		t.Errorf("MaxReescalations = %d, want %d", loaded.MaxReescalations, original.MaxReescalations)
	}
	if loaded.Contacts.HumanEmail != original.Contacts.HumanEmail {
		t.Errorf("Contacts.HumanEmail = %q, want %q", loaded.Contacts.HumanEmail, original.Contacts.HumanEmail)
	}
	if loaded.Contacts.HumanSMS != original.Contacts.HumanSMS {
		t.Errorf("Contacts.HumanSMS = %q, want %q", loaded.Contacts.HumanSMS, original.Contacts.HumanSMS)
	}

	// Check routes
	for severity, actions := range original.Routes {
		loadedActions := loaded.Routes[severity]
		if len(loadedActions) != len(actions) {
			t.Errorf("Routes[%s] len = %d, want %d", severity, len(loadedActions), len(actions))
			continue
		}
		for i, action := range actions {
			if loadedActions[i] != action {
				t.Errorf("Routes[%s][%d] = %q, want %q", severity, i, loadedActions[i], action)
			}
		}
	}
}

func TestEscalationConfigDefaults(t *testing.T) {
	t.Parallel()

	cfg := NewEscalationConfig()

	if cfg.Type != "escalation" {
		t.Errorf("Type = %q, want %q", cfg.Type, "escalation")
	}
	if cfg.Version != CurrentEscalationVersion {
		t.Errorf("Version = %d, want %d", cfg.Version, CurrentEscalationVersion)
	}
	if cfg.StaleThreshold != "4h" {
		t.Errorf("StaleThreshold = %q, want %q", cfg.StaleThreshold, "4h")
	}
	if cfg.MaxReescalations != 2 {
		t.Errorf("MaxReescalations = %d, want %d", cfg.MaxReescalations, 2)
	}

	// Check default routes
	if len(cfg.Routes) != 4 {
		t.Errorf("Routes count = %d, want 4", len(cfg.Routes))
	}
	if len(cfg.Routes[SeverityLow]) != 1 || cfg.Routes[SeverityLow][0] != "bead" {
		t.Errorf("Routes[low] = %v, want [bead]", cfg.Routes[SeverityLow])
	}
	if len(cfg.Routes[SeverityCritical]) != 4 {
		t.Errorf("Routes[critical] len = %d, want 4", len(cfg.Routes[SeverityCritical]))
	}
}

func TestEscalationConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *EscalationConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &EscalationConfig{
				Type:    "escalation",
				Version: 1,
				Routes: map[string][]string{
					SeverityLow: {"bead"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			config: &EscalationConfig{
				Type:    "wrong-type",
				Version: 1,
			},
			wantErr: true,
			errMsg:  "invalid config type",
		},
		{
			name: "unsupported version",
			config: &EscalationConfig{
				Type:    "escalation",
				Version: 999,
			},
			wantErr: true,
			errMsg:  "unsupported config version",
		},
		{
			name: "invalid stale threshold",
			config: &EscalationConfig{
				Type:           "escalation",
				Version:        1,
				StaleThreshold: "not-a-duration",
			},
			wantErr: true,
			errMsg:  "invalid stale_threshold",
		},
		{
			name: "invalid severity key",
			config: &EscalationConfig{
				Type:    "escalation",
				Version: 1,
				Routes: map[string][]string{
					"invalid-severity": {"bead"},
				},
			},
			wantErr: true,
			errMsg:  "unknown severity",
		},
		{
			name: "negative max reescalations",
			config: &EscalationConfig{
				Type:             "escalation",
				Version:          1,
				MaxReescalations: -1,
			},
			wantErr: true,
			errMsg:  "max_reescalations must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEscalationConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateEscalationConfig() expected error containing %q, got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateEscalationConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateEscalationConfig() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestEscalationConfigGetStaleThreshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *EscalationConfig
		expected time.Duration
	}{
		{
			name:     "default when empty",
			config:   &EscalationConfig{},
			expected: 4 * time.Hour,
		},
		{
			name: "2 hours",
			config: &EscalationConfig{
				StaleThreshold: "2h",
			},
			expected: 2 * time.Hour,
		},
		{
			name: "30 minutes",
			config: &EscalationConfig{
				StaleThreshold: "30m",
			},
			expected: 30 * time.Minute,
		},
		{
			name: "invalid duration falls back to default",
			config: &EscalationConfig{
				StaleThreshold: "invalid",
			},
			expected: 4 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetStaleThreshold()
			if got != tt.expected {
				t.Errorf("GetStaleThreshold() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEscalationConfigGetRouteForSeverity(t *testing.T) {
	t.Parallel()

	cfg := &EscalationConfig{
		Routes: map[string][]string{
			SeverityLow:    {"bead"},
			SeverityMedium: {"bead", "mail:mayor"},
		},
	}

	tests := []struct {
		severity string
		expected []string
	}{
		{SeverityLow, []string{"bead"}},
		{SeverityMedium, []string{"bead", "mail:mayor"}},
		{SeverityHigh, []string{"bead", "mail:mayor"}},     // fallback for missing
		{SeverityCritical, []string{"bead", "mail:mayor"}}, // fallback for missing
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := cfg.GetRouteForSeverity(tt.severity)
			if len(got) != len(tt.expected) {
				t.Errorf("GetRouteForSeverity(%s) len = %d, want %d", tt.severity, len(got), len(tt.expected))
				return
			}
			for i, action := range tt.expected {
				if got[i] != action {
					t.Errorf("GetRouteForSeverity(%s)[%d] = %q, want %q", tt.severity, i, got[i], action)
				}
			}
		})
	}
}

func TestEscalationConfigGetMaxReescalations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   *EscalationConfig
		expected int
	}{
		{
			name:     "default when zero",
			config:   &EscalationConfig{},
			expected: 2,
		},
		{
			name: "custom value",
			config: &EscalationConfig{
				MaxReescalations: 5,
			},
			expected: 5,
		},
		{
			name: "default when negative (should not happen after validation)",
			config: &EscalationConfig{
				MaxReescalations: -1,
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetMaxReescalations()
			if got != tt.expected {
				t.Errorf("GetMaxReescalations() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestLoadOrCreateEscalationConfig(t *testing.T) {
	t.Parallel()

	t.Run("creates default when not found", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings", "escalation.json")

		cfg, err := LoadOrCreateEscalationConfig(path)
		if err != nil {
			t.Fatalf("LoadOrCreateEscalationConfig: %v", err)
		}

		if cfg.Type != "escalation" {
			t.Errorf("Type = %q, want %q", cfg.Type, "escalation")
		}
		if len(cfg.Routes) != 4 {
			t.Errorf("Routes count = %d, want 4", len(cfg.Routes))
		}
	})

	t.Run("loads existing config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings", "escalation.json")

		// Create a config first
		original := &EscalationConfig{
			Type:           "escalation",
			Version:        1,
			StaleThreshold: "1h",
			Routes: map[string][]string{
				SeverityLow: {"bead"},
			},
		}
		if err := SaveEscalationConfig(path, original); err != nil {
			t.Fatalf("SaveEscalationConfig: %v", err)
		}

		// Load it
		cfg, err := LoadOrCreateEscalationConfig(path)
		if err != nil {
			t.Fatalf("LoadOrCreateEscalationConfig: %v", err)
		}

		if cfg.StaleThreshold != "1h" {
			t.Errorf("StaleThreshold = %q, want %q", cfg.StaleThreshold, "1h")
		}
	})
}

func TestEscalationConfigPath(t *testing.T) {
	t.Parallel()

	path := EscalationConfigPath("/home/user/gt")
	expected := "/home/user/gt/settings/escalation.json"
	if filepath.ToSlash(path) != expected {
		t.Errorf("EscalationConfigPath = %q, want %q", path, expected)
	}
}

func TestBuildStartupCommandWithAgentOverride_PriorityOverRoleAgents(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Configure town settings with role_agents: refinery = codex
	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	townSettings.RoleAgents = map[string]string{
		constants.RoleRefinery: "codex",
	}
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}

	// Create empty rig settings
	rigSettings := NewRigSettings()
	if err := SaveRigSettings(RigSettingsPath(rigPath), rigSettings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	// agentOverride = "gemini" should take priority over role_agents[refinery] = "codex"
	cmd, err := BuildStartupCommandWithAgentOverride(
		map[string]string{"GT_ROLE": constants.RoleRefinery},
		rigPath,
		"",
		"gemini", // explicit override
	)
	if err != nil {
		t.Fatalf("BuildStartupCommandWithAgentOverride: %v", err)
	}

	if !strings.Contains(cmd, "gemini") {
		t.Errorf("expected gemini (override) in command, got: %q", cmd)
	}
	if strings.Contains(cmd, "codex") {
		t.Errorf("did not expect codex (role_agents) when override is set: %q", cmd)
	}
}

func TestBuildStartupCommandWithAgentOverride_IncludesGTRoot(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create necessary config files
	townSettings := NewTownSettings()
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd, err := BuildStartupCommandWithAgentOverride(
		map[string]string{"GT_ROLE": constants.RoleWitness},
		rigPath,
		"",
		"gemini",
	)
	if err != nil {
		t.Fatalf("BuildStartupCommandWithAgentOverride: %v", err)
	}

	// Should include GT_ROOT in export.
	// Accept both quoted and unquoted forms since path quoting varies by platform.
	// Windows paths contain backslashes which trigger ShellQuote's quoting logic.
	unquotedForm := "GT_ROOT=" + townRoot
	quotedForm := "GT_ROOT=" + ShellQuote(townRoot)
	if !strings.Contains(cmd, unquotedForm) && !strings.Contains(cmd, quotedForm) {
		t.Errorf("expected GT_ROOT=%s (quoted or unquoted) in command, got: %q", townRoot, cmd)
	}
}

func TestQuoteForShell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple string",
			input: "hello",
			want:  `"hello"`,
		},
		{
			name:  "string with double quote",
			input: `say "hello"`,
			want:  `"say \"hello\""`,
		},
		{
			name:  "string with backslash",
			input: `path\to\file`,
			want:  `"path\\to\\file"`,
		},
		{
			name:  "string with backtick",
			input: "run `cmd`",
			want:  "\"run \\`cmd\\`\"",
		},
		{
			name:  "string with dollar sign",
			input: "cost is $100",
			want:  `"cost is \$100"`,
		},
		{
			name:  "variable expansion prevented",
			input: "$HOME/path",
			want:  `"\$HOME/path"`,
		},
		{
			name:  "empty string",
			input: "",
			want:  `""`,
		},
		{
			name:  "combined special chars",
			input: "`$HOME`",
			want:  "\"\\`\\$HOME\\`\"",
		},
		{
			name:  "string with newline",
			input: "line1\nline2",
			want:  `"line1\nline2"`,
		},
		{
			name:  "string with carriage return",
			input: "line1\rline2",
			want:  `"line1\rline2"`,
		},
		{
			name:  "string with tab",
			input: "col1\tcol2",
			want:  `"col1\tcol2"`,
		},
		{
			name:  "multiline prompt",
			input: "[GAS TOWN] test <- sender • time • topic\n\nInstructions here",
			want:  `"[GAS TOWN] test <- sender • time • topic\n\nInstructions here"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteForShell(tt.input)
			if got != tt.want {
				t.Errorf("quoteForShell(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildStartupCommandWithAgentOverride_SetsGTAgent(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create necessary config files
	townSettings := NewTownSettings()
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd, err := BuildStartupCommandWithAgentOverride(
		map[string]string{"GT_ROLE": constants.RoleWitness},
		rigPath,
		"",
		"gemini",
	)
	if err != nil {
		t.Fatalf("BuildStartupCommandWithAgentOverride: %v", err)
	}

	// Should include GT_AGENT=gemini in export so handoff can preserve it
	if !strings.Contains(cmd, "GT_AGENT=gemini") {
		t.Errorf("expected GT_AGENT=gemini in command, got: %q", cmd)
	}
}

func TestBuildStartupCommandWithAgentOverride_NoGTAgentWhenNoOverride(t *testing.T) {
	t.Parallel()
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")

	// Create necessary config files
	townSettings := NewTownSettings()
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	cmd, err := BuildStartupCommandWithAgentOverride(
		map[string]string{"GT_ROLE": constants.RoleWitness},
		rigPath,
		"",
		"", // No override
	)
	if err != nil {
		t.Fatalf("BuildStartupCommandWithAgentOverride: %v", err)
	}

	// Should NOT include GT_AGENT when no override is used
	if strings.Contains(cmd, "GT_AGENT=") {
		t.Errorf("expected no GT_AGENT in command when no override, got: %q", cmd)
	}
}
