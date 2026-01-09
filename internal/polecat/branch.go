package polecat

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

// invalidBranchCharsRegex matches characters that are invalid in git branch names.
// Git branch names cannot contain: ~ ^ : \ space, .., @{, or end with .lock
var invalidBranchCharsRegex = regexp.MustCompile(`[~^:\s\\]|\.\.|\.\.|@\{`)

// BuildPolecatBranchName expands a polecat branch template with variables.
// Variables supported:
//   - {name}: Polecat name (e.g., "Toast")
//   - {timestamp}: Base36 Unix milliseconds (e.g., "1gvp7k5")
//   - {rig}: Rig name (e.g., "gastown")
//
// If template is empty, uses DefaultPolecatBranchTemplate.
func BuildPolecatBranchName(template, name, rigName string) string {
	if template == "" {
		template = config.DefaultPolecatBranchTemplate
	}

	// Generate timestamp in base36 for shorter branch names
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 36)

	result := template
	result = strings.ReplaceAll(result, "{name}", name)
	result = strings.ReplaceAll(result, "{timestamp}", timestamp)
	result = strings.ReplaceAll(result, "{rig}", rigName)

	return result
}

// GetPolecatBranchTemplate returns the polecat branch template from rig settings.
// Falls back to DefaultPolecatBranchTemplate if settings don't exist or don't specify a template.
func GetPolecatBranchTemplate(rigPath string) string {
	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return config.DefaultPolecatBranchTemplate
	}

	if settings.BranchNaming != nil && settings.BranchNaming.PolecatBranchTemplate != "" {
		return settings.BranchNaming.PolecatBranchTemplate
	}

	return config.DefaultPolecatBranchTemplate
}

// ValidateBranchName checks if a branch name is valid for git.
// Returns an error if the branch name contains invalid characters.
func ValidateBranchName(branchName string) error {
	if branchName == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Check for invalid characters
	if invalidBranchCharsRegex.MatchString(branchName) {
		return fmt.Errorf("branch name %q contains invalid characters (~ ^ : \\ space, .., or @{)", branchName)
	}

	// Check for .lock suffix
	if strings.HasSuffix(branchName, ".lock") {
		return fmt.Errorf("branch name %q cannot end with .lock", branchName)
	}

	// Check for leading/trailing slashes or dots
	if strings.HasPrefix(branchName, "/") || strings.HasSuffix(branchName, "/") {
		return fmt.Errorf("branch name %q cannot start or end with /", branchName)
	}
	if strings.HasPrefix(branchName, ".") || strings.HasSuffix(branchName, ".") {
		return fmt.Errorf("branch name %q cannot start or end with .", branchName)
	}

	// Check for consecutive slashes
	if strings.Contains(branchName, "//") {
		return fmt.Errorf("branch name %q cannot contain consecutive slashes", branchName)
	}

	return nil
}
