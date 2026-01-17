package upgrade

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// CompatibilityInfo describes version compatibility for a release.
// This is downloaded from the release assets as compatibility.json.
type CompatibilityInfo struct {
	// Version this compatibility info is for
	Version string `json:"version"`

	// MinWorkspaceVersion is the minimum gt_version a workspace must have
	// to safely upgrade to this version. Empty means no restriction.
	MinWorkspaceVersion string `json:"min_workspace_version,omitempty"`

	// BreakingChanges lists identifiers for breaking changes in this version.
	// These are keys that can be matched to documentation/migration guides.
	BreakingChanges []string `json:"breaking_changes,omitempty"`

	// MigrationRequiredFrom lists version patterns (e.g., "0.1.x") that
	// require running a migration before upgrading to this version.
	MigrationRequiredFrom []string `json:"migration_required_from,omitempty"`

	// MigrationGuideURL points to documentation for the migration process.
	MigrationGuideURL string `json:"migration_guide_url,omitempty"`
}

// CompatCheckResult describes the result of a compatibility check.
type CompatCheckResult struct {
	// Compatible is true if the workspace can safely upgrade.
	Compatible bool

	// WorkspaceVersion is the version recorded in the workspace.
	WorkspaceVersion string

	// TargetVersion is the version we're trying to upgrade to.
	TargetVersion string

	// BreakingChanges lists the breaking changes that affect this upgrade.
	BreakingChanges []string

	// MigrationRequired is true if a migration script needs to run first.
	MigrationRequired bool

	// MigrationGuideURL points to migration documentation.
	MigrationGuideURL string

	// Message is a human-readable explanation of the result.
	Message string
}

// FetchCompatibilityInfo downloads compatibility.json from a release's assets.
// Returns nil (not an error) if the release doesn't have compatibility info.
func FetchCompatibilityInfo(release *ReleaseInfo) (*CompatibilityInfo, error) {
	// Find compatibility.json asset
	var compatAsset *Asset
	for i := range release.Assets {
		if release.Assets[i].Name == "compatibility.json" {
			compatAsset = &release.Assets[i]
			break
		}
	}

	// No compatibility info is okay - older releases won't have it
	if compatAsset == nil {
		return nil, nil
	}

	// Download the file
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", compatAsset.BrowserDownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading compatibility info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("compatibility.json download failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading compatibility info: %w", err)
	}

	var info CompatibilityInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("parsing compatibility info: %w", err)
	}

	return &info, nil
}

// GetWorkspaceVersion reads the gt_version from a workspace's town.json.
// Returns empty string if not found or not in a workspace.
func GetWorkspaceVersion(workspaceRoot string) string {
	townPath := filepath.Join(workspaceRoot, "mayor", "town.json")
	data, err := os.ReadFile(townPath)
	if err != nil {
		return ""
	}

	var config struct {
		GTVersion string `json:"gt_version"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ""
	}

	return config.GTVersion
}

// SetWorkspaceVersion updates the gt_version in a workspace's town.json.
func SetWorkspaceVersion(workspaceRoot, version string) error {
	townPath := filepath.Join(workspaceRoot, "mayor", "town.json")

	// Read existing config
	data, err := os.ReadFile(townPath)
	if err != nil {
		return fmt.Errorf("reading town.json: %w", err)
	}

	// Parse as generic map to preserve other fields
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing town.json: %w", err)
	}

	// Update gt_version
	config["gt_version"] = version

	// Write back
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling town.json: %w", err)
	}

	if err := os.WriteFile(townPath, newData, 0644); err != nil {
		return fmt.Errorf("writing town.json: %w", err)
	}

	return nil
}

// CheckCompatibility verifies if a workspace can safely upgrade to a target version.
func CheckCompatibility(workspaceRoot string, currentVersion string, targetRelease *ReleaseInfo, compatInfo *CompatibilityInfo) *CompatCheckResult {
	result := &CompatCheckResult{
		Compatible:       true,
		WorkspaceVersion: currentVersion,
		TargetVersion:    targetRelease.TagName,
	}

	// If no compatibility info, assume compatible (older releases)
	if compatInfo == nil {
		result.Message = "No compatibility info for this release (assuming compatible)"
		return result
	}

	// Parse versions
	current, err := ParseVersion(currentVersion)
	if err != nil {
		// Can't parse current version - be conservative and allow upgrade
		result.Message = fmt.Sprintf("Could not parse current version %q - proceeding with upgrade", currentVersion)
		return result
	}

	// Check minimum workspace version
	if compatInfo.MinWorkspaceVersion != "" {
		minVersion, err := ParseVersion(compatInfo.MinWorkspaceVersion)
		if err == nil && current.LessThan(minVersion) {
			result.Compatible = false
			result.BreakingChanges = compatInfo.BreakingChanges
			result.MigrationRequired = true
			result.MigrationGuideURL = compatInfo.MigrationGuideURL
			result.Message = fmt.Sprintf("Workspace version %s is below minimum required %s",
				currentVersion, compatInfo.MinWorkspaceVersion)
			return result
		}
	}

	// Check if current version matches any migration-required patterns
	for _, pattern := range compatInfo.MigrationRequiredFrom {
		if current.MatchesPattern(pattern) {
			result.Compatible = false
			result.MigrationRequired = true
			result.BreakingChanges = compatInfo.BreakingChanges
			result.MigrationGuideURL = compatInfo.MigrationGuideURL
			result.Message = fmt.Sprintf("Upgrading from %s requires migration", currentVersion)
			return result
		}
	}

	// Note breaking changes even if compatible (informational)
	if len(compatInfo.BreakingChanges) > 0 {
		result.BreakingChanges = compatInfo.BreakingChanges
		result.Message = fmt.Sprintf("Compatible with %d noted changes", len(compatInfo.BreakingChanges))
	} else {
		result.Message = "Compatible"
	}

	return result
}

// FormatCompatWarning returns a formatted warning message for incompatible upgrades.
func FormatCompatWarning(result *CompatCheckResult) string {
	var msg string

	msg += fmt.Sprintf("⚠ Breaking changes detected:\n")
	for _, change := range result.BreakingChanges {
		msg += fmt.Sprintf("  - %s\n", change)
	}
	msg += "\n"
	msg += fmt.Sprintf("Your workspace (%s) requires migration before upgrading.\n", result.WorkspaceVersion)

	if result.MigrationGuideURL != "" {
		msg += fmt.Sprintf("See: %s\n", result.MigrationGuideURL)
	} else {
		msg += fmt.Sprintf("See: https://github.com/steveyegge/gastown/blob/main/docs/migrations/%s.md\n",
			result.TargetVersion)
	}

	msg += "\nRun 'gt upgrade --force' to upgrade anyway (may break workspace)"

	return msg
}
