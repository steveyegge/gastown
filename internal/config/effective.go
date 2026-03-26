package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadEffectiveRigSettings loads committed repo settings and overlays rig-local
// operator settings. repoRoot should point at a tracked clone for the rig; when
// empty, the conventional mayor clone is used.
func LoadEffectiveRigSettings(rigPath, repoRoot string) (*RigSettings, error) {
	if rigPath == "" {
		return nil, fmt.Errorf("rig path is required")
	}
	if repoRoot == "" {
		repoRoot = filepath.Join(rigPath, "mayor", "rig")
	}

	repoSettings, err := LoadRepoSettings(repoRoot)
	if err != nil {
		return nil, err
	}

	localSettingsPath := filepath.Join(rigPath, "settings", "config.json")
	localSettings, err := LoadRigSettings(localSettingsPath)
	if err != nil && !errorsIsNotFound(err) {
		return nil, err
	}

	return MergeRigSettings(repoSettings, localSettings), nil
}

// MergeRigSettings overlays repo defaults with rig-local overrides and injects
// repo-contract derived defaults into merge_queue when needed.
func MergeRigSettings(repo, local *RigSettings) *RigSettings {
	if repo == nil && local == nil {
		return nil
	}

	result := &RigSettings{
		Type:    "rig-settings",
		Version: CurrentRigSettingsVersion,
	}

	if repo != nil {
		mergeRigSettingsInto(result, repo)
	}
	if local != nil {
		mergeRigSettingsInto(result, local)
	}

	result.MergeQueue = MergeSettingsCommand(
		valueMergeQueue(repo),
		valueMergeQueue(local),
	)
	result.RepoContract = MergeRepoContract(valueRepoContract(repo), valueRepoContract(local))
	ApplyRepoContractDefaults(result)
	return result
}

// MergeRepoContract overlays local repo-contract overrides on top of committed defaults.
func MergeRepoContract(repo, local *RepoContract) *RepoContract {
	if repo == nil && local == nil {
		return nil
	}

	result := &RepoContract{}
	if repo != nil {
		*result = *repo
		if len(repo.CriticalPaths) > 0 {
			result.CriticalPaths = append([]string(nil), repo.CriticalPaths...)
		}
	}
	if local != nil {
		if local.RepoType != "" {
			result.RepoType = local.RepoType
		}
		if local.VerifyCommand != "" {
			result.VerifyCommand = local.VerifyCommand
		}
		if local.SmokeCommand != "" {
			result.SmokeCommand = local.SmokeCommand
		}
		if local.ReleaseCheckCommand != "" {
			result.ReleaseCheckCommand = local.ReleaseCheckCommand
		}
		if local.IntegrationCommand != "" {
			result.IntegrationCommand = local.IntegrationCommand
		}
		if local.E2ECommand != "" {
			result.E2ECommand = local.E2ECommand
		}
		if local.PerformanceCommand != "" {
			result.PerformanceCommand = local.PerformanceCommand
		}
		if len(local.CriticalPaths) > 0 {
			result.CriticalPaths = append([]string(nil), local.CriticalPaths...)
		}
		if local.GitHubCI != nil {
			result.GitHubCI = &GitHubCIConfig{
				Workflow: local.GitHubCI.Workflow,
				Required: local.GitHubCI.Required,
			}
		}
	}
	return result
}

// ApplyRepoContractDefaults injects merge queue gates from the repo contract when
// the repo declares canonical verifier entrypoints and merge_queue does not.
func ApplyRepoContractDefaults(settings *RigSettings) {
	if settings == nil || settings.RepoContract == nil {
		return
	}
	contract := settings.RepoContract
	if settings.MergeQueue == nil {
		settings.MergeQueue = DefaultMergeQueueConfig()
	}

	if settings.MergeQueue.Gates == nil {
		settings.MergeQueue.Gates = make(map[string]*VerificationGateConfig)
	}
	if len(settings.MergeQueue.Gates) == 0 {
		if strings.TrimSpace(contract.VerifyCommand) != "" {
			settings.MergeQueue.Gates["verify"] = &VerificationGateConfig{
				Cmd:   strings.TrimSpace(contract.VerifyCommand),
				Phase: "pre-merge",
			}
		}
		if strings.TrimSpace(contract.SmokeCommand) != "" {
			settings.MergeQueue.Gates["smoke"] = &VerificationGateConfig{
				Cmd:   strings.TrimSpace(contract.SmokeCommand),
				Phase: "post-squash",
			}
		}
	}
	if settings.MergeQueue.TestCommand == "" && strings.TrimSpace(contract.VerifyCommand) != "" {
		settings.MergeQueue.TestCommand = strings.TrimSpace(contract.VerifyCommand)
	}
}

// HasPreMergeGate reports whether the config includes at least one pre-merge gate.
func HasPreMergeGate(mq *MergeQueueConfig) bool {
	if mq == nil {
		return false
	}
	for _, gate := range mq.Gates {
		if gate == nil {
			continue
		}
		switch strings.TrimSpace(gate.Phase) {
		case "", "pre-merge":
			if strings.TrimSpace(gate.Cmd) != "" {
				return true
			}
		}
	}
	return false
}

// ValidateStrictRepoContract enforces fail-closed requirements for strict rigs.
func ValidateStrictRepoContract(settings *RigSettings) error {
	if settings == nil || settings.MergeQueue == nil || !settings.MergeQueue.IsStrictVerification() {
		return nil
	}
	if !HasPreMergeGate(settings.MergeQueue) {
		return fmt.Errorf("strict verification requires at least one pre-merge gate")
	}
	if settings.RepoContract == nil || strings.TrimSpace(settings.RepoContract.VerifyCommand) == "" {
		return fmt.Errorf("strict verification requires repo_contract.verify_command")
	}
	if !IsRepoLocalCommand(settings.RepoContract.VerifyCommand) {
		return fmt.Errorf("repo_contract.verify_command must be repo-local, got %q", settings.RepoContract.VerifyCommand)
	}
	return nil
}

// EffectiveGitHubCIForRemote returns the effective GitHub CI policy for a rig.
// Strict GitHub rigs default to required workflow "CI" even if the repo
// contract omits an explicit github_ci block.
func EffectiveGitHubCIForRemote(settings *RigSettings, remoteURL string) *GitHubCIConfig {
	isGitHub := strings.Contains(remoteURL, "github.com")
	if !isGitHub {
		if settings == nil || settings.RepoContract == nil {
			return nil
		}
		return settings.RepoContract.GitHubCI
	}

	if settings != nil && settings.RepoContract != nil && settings.RepoContract.GitHubCI != nil {
		cfg := *settings.RepoContract.GitHubCI
		if strings.TrimSpace(cfg.Workflow) == "" {
			cfg.Workflow = "CI"
		}
		if cfg.Required == nil && settings.MergeQueue != nil && settings.MergeQueue.IsStrictVerification() {
			required := true
			cfg.Required = &required
		}
		return &cfg
	}

	if settings != nil && settings.MergeQueue != nil && settings.MergeQueue.IsStrictVerification() {
		required := true
		return &GitHubCIConfig{Workflow: "CI", Required: &required}
	}
	return nil
}

// IsRepoLocalCommand returns true when the command is anchored within the repo
// rather than pointing at an arbitrary absolute path outside the workspace.
func IsRepoLocalCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}
	if strings.HasPrefix(cmd, "/") || strings.Contains(cmd, " ../") || strings.HasPrefix(cmd, "../") {
		return false
	}
	if strings.HasPrefix(cmd, "./") || strings.HasPrefix(cmd, "scripts/") {
		return true
	}
	if strings.HasPrefix(cmd, "make ") || cmd == "make" {
		return true
	}
	return !strings.Contains(cmd, "/")
}

func cloneVerificationGates(src map[string]*VerificationGateConfig) map[string]*VerificationGateConfig {
	if src == nil {
		return nil
	}
	dst := make(map[string]*VerificationGateConfig, len(src))
	for name, gate := range src {
		if gate == nil {
			continue
		}
		copy := *gate
		dst[name] = &copy
	}
	return dst
}

func mergeRigSettingsInto(dst, src *RigSettings) {
	if dst == nil || src == nil {
		return
	}
	if src.Theme != nil {
		dst.Theme = src.Theme
	}
	if src.Namepool != nil {
		dst.Namepool = src.Namepool
	}
	if src.Crew != nil {
		dst.Crew = src.Crew
	}
	if src.Workflow != nil {
		dst.Workflow = src.Workflow
	}
	if src.Runtime != nil {
		dst.Runtime = src.Runtime
	}
	if src.Agent != "" {
		dst.Agent = src.Agent
	}
	if src.Agents != nil {
		dst.Agents = src.Agents
	}
	if src.RoleAgents != nil {
		dst.RoleAgents = src.RoleAgents
	}
	if src.WorkerAgents != nil {
		dst.WorkerAgents = src.WorkerAgents
	}
}

func valueMergeQueue(settings *RigSettings) *MergeQueueConfig {
	if settings == nil {
		return nil
	}
	return settings.MergeQueue
}

func valueRepoContract(settings *RigSettings) *RepoContract {
	if settings == nil {
		return nil
	}
	return settings.RepoContract
}

func errorsIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ErrNotFound.Error()) || os.IsNotExist(err)
}
