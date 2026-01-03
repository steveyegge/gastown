package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	// ErrNotFound indicates the config file does not exist.
	ErrNotFound = errors.New("config file not found")

	// ErrInvalidVersion indicates an unsupported schema version.
	ErrInvalidVersion = errors.New("unsupported config version")

	// ErrInvalidType indicates an unexpected config type.
	ErrInvalidType = errors.New("invalid config type")

	// ErrMissingField indicates a required field is missing.
	ErrMissingField = errors.New("missing required field")
)

// LoadTownConfig loads and validates a town configuration file.
func LoadTownConfig(path string) (*TownConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from trusted config location
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config TownConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validateTownConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveTownConfig saves a town configuration to a file.
func SaveTownConfig(path string, config *TownConfig) error {
	if err := validateTownConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// LoadRigsConfig loads and validates a rigs registry file.
func LoadRigsConfig(path string) (*RigsConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config RigsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validateRigsConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveRigsConfig saves a rigs registry to a file.
func SaveRigsConfig(path string, config *RigsConfig) error {
	if err := validateRigsConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// LoadAgentState loads an agent state file.
func LoadAgentState(path string) (*AgentState, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var state AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	if err := validateAgentState(&state); err != nil {
		return nil, err
	}

	return &state, nil
}

// SaveAgentState saves an agent state to a file.
func SaveAgentState(path string, state *AgentState) error {
	if err := validateAgentState(state); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: state files don't contain secrets
		return fmt.Errorf("writing state: %w", err)
	}

	return nil
}

// validateTownConfig validates a TownConfig.
func validateTownConfig(c *TownConfig) error {
	if c.Type != "town" && c.Type != "" {
		return fmt.Errorf("%w: expected type 'town', got '%s'", ErrInvalidType, c.Type)
	}
	if c.Version > CurrentTownVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentTownVersion)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: name", ErrMissingField)
	}
	return nil
}

// validateRigsConfig validates a RigsConfig.
func validateRigsConfig(c *RigsConfig) error {
	if c.Version > CurrentRigsVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentRigsVersion)
	}
	if c.Rigs == nil {
		c.Rigs = make(map[string]RigEntry)
	}
	return nil
}

// validateAgentState validates an AgentState.
func validateAgentState(s *AgentState) error {
	if s.Role == "" {
		return fmt.Errorf("%w: role", ErrMissingField)
	}
	return nil
}

// LoadRigConfig loads and validates a rig configuration file.
func LoadRigConfig(path string) (*RigConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config RigConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validateRigConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveRigConfig saves a rig configuration to a file.
func SaveRigConfig(path string, config *RigConfig) error {
	if err := validateRigConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: config files don't contain secrets
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// validateRigConfig validates a RigConfig (identity only).
func validateRigConfig(c *RigConfig) error {
	if c.Type != "rig" && c.Type != "" {
		return fmt.Errorf("%w: expected type 'rig', got '%s'", ErrInvalidType, c.Type)
	}
	if c.Version > CurrentRigConfigVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentRigConfigVersion)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: name", ErrMissingField)
	}
	return nil
}

// validateRigSettings validates a RigSettings.
func validateRigSettings(c *RigSettings) error {
	if c.Type != "rig-settings" && c.Type != "" {
		return fmt.Errorf("%w: expected type 'rig-settings', got '%s'", ErrInvalidType, c.Type)
	}
	if c.Version > CurrentRigSettingsVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentRigSettingsVersion)
	}
	if c.MergeQueue != nil {
		if err := validateMergeQueueConfig(c.MergeQueue); err != nil {
			return err
		}
	}
	return nil
}

// ErrInvalidOnConflict indicates an invalid on_conflict strategy.
var ErrInvalidOnConflict = errors.New("invalid on_conflict strategy")

// validateMergeQueueConfig validates a MergeQueueConfig.
func validateMergeQueueConfig(c *MergeQueueConfig) error {
	// Validate on_conflict strategy
	if c.OnConflict != "" && c.OnConflict != OnConflictAssignBack && c.OnConflict != OnConflictAutoRebase {
		return fmt.Errorf("%w: got '%s', want '%s' or '%s'",
			ErrInvalidOnConflict, c.OnConflict, OnConflictAssignBack, OnConflictAutoRebase)
	}

	// Validate poll_interval if specified
	if c.PollInterval != "" {
		if _, err := time.ParseDuration(c.PollInterval); err != nil {
			return fmt.Errorf("invalid poll_interval: %w", err)
		}
	}

	// Validate non-negative values
	if c.RetryFlakyTests < 0 {
		return fmt.Errorf("%w: retry_flaky_tests must be non-negative", ErrMissingField)
	}
	if c.MaxConcurrent < 0 {
		return fmt.Errorf("%w: max_concurrent must be non-negative", ErrMissingField)
	}

	return nil
}

// NewRigConfig creates a new RigConfig (identity only).
func NewRigConfig(name, gitURL string) *RigConfig {
	return &RigConfig{
		Type:    "rig",
		Version: CurrentRigConfigVersion,
		Name:    name,
		GitURL:  gitURL,
	}
}

// NewRigSettings creates a new RigSettings with defaults.
func NewRigSettings() *RigSettings {
	return &RigSettings{
		Type:       "rig-settings",
		Version:    CurrentRigSettingsVersion,
		MergeQueue: DefaultMergeQueueConfig(),
		Namepool:   DefaultNamepoolConfig(),
	}
}

// LoadRigSettings loads and validates a rig settings file.
func LoadRigSettings(path string) (*RigSettings, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading settings: %w", err)
	}

	var settings RigSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing settings: %w", err)
	}

	if err := validateRigSettings(&settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

// SaveRigSettings saves rig settings to a file.
func SaveRigSettings(path string, settings *RigSettings) error {
	if err := validateRigSettings(settings); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding settings: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: settings files don't contain secrets
		return fmt.Errorf("writing settings: %w", err)
	}

	return nil
}

// LoadMayorConfig loads and validates a mayor config file.
func LoadMayorConfig(path string) (*MayorConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var config MayorConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if err := validateMayorConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveMayorConfig saves a mayor config to a file.
func SaveMayorConfig(path string, config *MayorConfig) error {
	if err := validateMayorConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil { //nolint:gosec // G306: config files don't contain secrets
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// validateMayorConfig validates a MayorConfig.
func validateMayorConfig(c *MayorConfig) error {
	if c.Type != "mayor-config" && c.Type != "" {
		return fmt.Errorf("%w: expected type 'mayor-config', got '%s'", ErrInvalidType, c.Type)
	}
	if c.Version > CurrentMayorConfigVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentMayorConfigVersion)
	}
	return nil
}

// NewMayorConfig creates a new MayorConfig with defaults.
func NewMayorConfig() *MayorConfig {
	return &MayorConfig{
		Type:    "mayor-config",
		Version: CurrentMayorConfigVersion,
	}
}

// LoadAccountsConfig loads and validates an accounts configuration file.
func LoadAccountsConfig(path string) (*AccountsConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed internally, not from user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading accounts config: %w", err)
	}

	var config AccountsConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing accounts config: %w", err)
	}

	if err := validateAccountsConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveAccountsConfig saves an accounts configuration to a file.
func SaveAccountsConfig(path string, config *AccountsConfig) error {
	if err := validateAccountsConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding accounts config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing accounts config: %w", err)
	}

	return nil
}

// validateAccountsConfig validates an AccountsConfig.
func validateAccountsConfig(c *AccountsConfig) error {
	if c.Version > CurrentAccountsVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentAccountsVersion)
	}
	if c.Accounts == nil {
		c.Accounts = make(map[string]Account)
	}
	// Validate default refers to an existing account (if set and accounts exist)
	if c.Default != "" && len(c.Accounts) > 0 {
		if _, ok := c.Accounts[c.Default]; !ok {
			return fmt.Errorf("%w: default account '%s' not found in accounts", ErrMissingField, c.Default)
		}
	}
	// Validate each account has required fields
	for handle, acct := range c.Accounts {
		if acct.ConfigDir == "" {
			return fmt.Errorf("%w: config_dir for account '%s'", ErrMissingField, handle)
		}
	}
	return nil
}

// NewAccountsConfig creates a new AccountsConfig with defaults.
func NewAccountsConfig() *AccountsConfig {
	return &AccountsConfig{
		Version:  CurrentAccountsVersion,
		Accounts: make(map[string]Account),
	}
}

// GetAccount returns an account by handle, or nil if not found.
func (c *AccountsConfig) GetAccount(handle string) *Account {
	if acct, ok := c.Accounts[handle]; ok {
		return &acct
	}
	return nil
}

// GetDefaultAccount returns the default account, or nil if not set.
func (c *AccountsConfig) GetDefaultAccount() *Account {
	if c.Default == "" {
		return nil
	}
	return c.GetAccount(c.Default)
}

// ResolveAccountConfigDir resolves the CLAUDE_CONFIG_DIR for account selection.
// Priority order:
//  1. GT_ACCOUNT environment variable
//  2. accountFlag (from --account command flag)
//  3. Default account from config
//
// Returns empty string if no account configured or resolved.
// Returns the handle that was resolved as second value.
func ResolveAccountConfigDir(accountsPath, accountFlag string) (configDir, handle string, err error) {
	// Load accounts config
	cfg, loadErr := LoadAccountsConfig(accountsPath)
	if loadErr != nil {
		// No accounts configured - that's OK, return empty
		return "", "", nil
	}

	// Priority 1: GT_ACCOUNT env var
	if envAccount := os.Getenv("GT_ACCOUNT"); envAccount != "" {
		acct := cfg.GetAccount(envAccount)
		if acct == nil {
			return "", "", fmt.Errorf("GT_ACCOUNT '%s' not found in accounts config", envAccount)
		}
		return expandPath(acct.ConfigDir), envAccount, nil
	}

	// Priority 2: --account flag
	if accountFlag != "" {
		acct := cfg.GetAccount(accountFlag)
		if acct == nil {
			return "", "", fmt.Errorf("account '%s' not found in accounts config", accountFlag)
		}
		return expandPath(acct.ConfigDir), accountFlag, nil
	}

	// Priority 3: Default account
	if cfg.Default != "" {
		acct := cfg.GetDefaultAccount()
		if acct != nil {
			return expandPath(acct.ConfigDir), cfg.Default, nil
		}
	}

	return "", "", nil
}

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// LoadMessagingConfig loads and validates a messaging configuration file.
func LoadMessagingConfig(path string) (*MessagingConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, fmt.Errorf("reading messaging config: %w", err)
	}

	var config MessagingConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing messaging config: %w", err)
	}

	if err := validateMessagingConfig(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveMessagingConfig saves a messaging configuration to a file.
func SaveMessagingConfig(path string, config *MessagingConfig) error {
	if err := validateMessagingConfig(config); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding messaging config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing messaging config: %w", err)
	}

	return nil
}

// validateMessagingConfig validates a MessagingConfig.
func validateMessagingConfig(c *MessagingConfig) error {
	if c.Type != "messaging" && c.Type != "" {
		return fmt.Errorf("%w: expected type 'messaging', got '%s'", ErrInvalidType, c.Type)
	}
	if c.Version > CurrentMessagingVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrInvalidVersion, c.Version, CurrentMessagingVersion)
	}

	// Initialize nil maps
	if c.Lists == nil {
		c.Lists = make(map[string][]string)
	}
	if c.Queues == nil {
		c.Queues = make(map[string]QueueConfig)
	}
	if c.Announces == nil {
		c.Announces = make(map[string]AnnounceConfig)
	}
	if c.NudgeChannels == nil {
		c.NudgeChannels = make(map[string][]string)
	}

	// Validate lists have at least one recipient
	for name, recipients := range c.Lists {
		if len(recipients) == 0 {
			return fmt.Errorf("%w: list '%s' has no recipients", ErrMissingField, name)
		}
	}

	// Validate queues have at least one worker
	for name, queue := range c.Queues {
		if len(queue.Workers) == 0 {
			return fmt.Errorf("%w: queue '%s' workers", ErrMissingField, name)
		}
		if queue.MaxClaims < 0 {
			return fmt.Errorf("%w: queue '%s' max_claims must be non-negative", ErrMissingField, name)
		}
	}

	// Validate announces have at least one reader
	for name, announce := range c.Announces {
		if len(announce.Readers) == 0 {
			return fmt.Errorf("%w: announce '%s' readers", ErrMissingField, name)
		}
		if announce.RetainCount < 0 {
			return fmt.Errorf("%w: announce '%s' retain_count must be non-negative", ErrMissingField, name)
		}
	}

	// Validate nudge channels have non-empty names and at least one recipient
	for name, recipients := range c.NudgeChannels {
		if name == "" {
			return fmt.Errorf("%w: nudge channel name cannot be empty", ErrMissingField)
		}
		if len(recipients) == 0 {
			return fmt.Errorf("%w: nudge channel '%s' has no recipients", ErrMissingField, name)
		}
	}

	return nil
}

// MessagingConfigPath returns the standard path for messaging config in a town.
func MessagingConfigPath(townRoot string) string {
	return filepath.Join(townRoot, "config", "messaging.json")
}

// LoadOrCreateMessagingConfig loads the messaging config, creating a default if not found.
func LoadOrCreateMessagingConfig(path string) (*MessagingConfig, error) {
	config, err := LoadMessagingConfig(path)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return NewMessagingConfig(), nil
		}
		return nil, err
	}
	return config, nil
}

// LoadRuntimeConfig loads the RuntimeConfig from a rig's settings.
// Falls back to defaults if settings don't exist or don't specify runtime config.
// rigPath should be the path to the rig directory (e.g., ~/gt/gastown).
func LoadRuntimeConfig(rigPath string) *RuntimeConfig {
	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	settings, err := LoadRigSettings(settingsPath)
	if err != nil {
		return DefaultRuntimeConfig()
	}
	if settings.Runtime == nil {
		return DefaultRuntimeConfig()
	}
	// Fill in defaults for empty fields
	rc := settings.Runtime
	if rc.Command == "" {
		rc.Command = "claude"
	}
	if rc.Args == nil {
		rc.Args = []string{"--dangerously-skip-permissions"}
	}
	return rc
}

// GetRuntimeCommand is a convenience function that returns the full command string
// for starting an LLM session. It loads the config and builds the command.
func GetRuntimeCommand(rigPath string) string {
	return LoadRuntimeConfig(rigPath).BuildCommand()
}

// GetRuntimeCommandWithPrompt returns the full command with an initial prompt.
func GetRuntimeCommandWithPrompt(rigPath, prompt string) string {
	return LoadRuntimeConfig(rigPath).BuildCommandWithPrompt(prompt)
}

// BuildStartupCommand builds a full startup command with environment exports.
// envVars is a map of environment variable names to values.
// rigPath is optional - if empty, uses defaults.
// prompt is optional - if provided, appended as the initial prompt.
func BuildStartupCommand(envVars map[string]string, rigPath, prompt string) string {
	var rc *RuntimeConfig
	if rigPath != "" {
		rc = LoadRuntimeConfig(rigPath)
	} else {
		rc = DefaultRuntimeConfig()
	}

	// Build environment export prefix
	var exports []string
	for k, v := range envVars {
		exports = append(exports, fmt.Sprintf("%s=%s", k, v))
	}

	// Sort for deterministic output
	sort.Strings(exports)

	var cmd string
	if len(exports) > 0 {
		cmd = "export " + strings.Join(exports, " ") + " && "
	}

	// Add runtime command
	if prompt != "" {
		cmd += rc.BuildCommandWithPrompt(prompt)
	} else {
		cmd += rc.BuildCommand()
	}

	return cmd
}

// BuildAgentStartupCommand is a convenience function for starting agent sessions.
// It sets standard environment variables (GT_ROLE, BD_ACTOR, GIT_AUTHOR_NAME)
// and builds the full startup command.
func BuildAgentStartupCommand(role, bdActor, rigPath, prompt string) string {
	envVars := map[string]string{
		"GT_ROLE":         role,
		"BD_ACTOR":        bdActor,
		"GIT_AUTHOR_NAME": bdActor,
	}
	return BuildStartupCommand(envVars, rigPath, prompt)
}

// BuildPolecatStartupCommand builds the startup command for a polecat.
// Sets GT_ROLE, GT_RIG, GT_POLECAT, BD_ACTOR, and GIT_AUTHOR_NAME.
func BuildPolecatStartupCommand(rigName, polecatName, rigPath, prompt string) string {
	bdActor := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	envVars := map[string]string{
		"GT_ROLE":         "polecat",
		"GT_RIG":          rigName,
		"GT_POLECAT":      polecatName,
		"BD_ACTOR":        bdActor,
		"GIT_AUTHOR_NAME": polecatName,
	}
	return BuildStartupCommand(envVars, rigPath, prompt)
}

// BuildCrewStartupCommand builds the startup command for a crew member.
// Sets GT_ROLE, GT_RIG, GT_CREW, BD_ACTOR, and GIT_AUTHOR_NAME.
func BuildCrewStartupCommand(rigName, crewName, rigPath, prompt string) string {
	bdActor := fmt.Sprintf("%s/crew/%s", rigName, crewName)
	envVars := map[string]string{
		"GT_ROLE":         "crew",
		"GT_RIG":          rigName,
		"GT_CREW":         crewName,
		"BD_ACTOR":        bdActor,
		"GIT_AUTHOR_NAME": crewName,
	}
	return BuildStartupCommand(envVars, rigPath, prompt)
}
