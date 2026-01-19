package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/upgrade"
)

const (
	// LayoutV01x is the 0.1.x workspace layout.
	// Characteristics: town.json at root, single-level beads
	LayoutV01x = "0.1.x"

	// LayoutV02x is the 0.2.x workspace layout.
	// Characteristics: town.json in mayor/, two-level beads, settings/ directories
	LayoutV02x = "0.2.x"

	// LayoutUnknown indicates an unrecognized layout.
	LayoutUnknown = "unknown"
)

// DetectLayout analyzes a workspace and returns its layout characteristics.
func DetectLayout(townRoot string) (*WorkspaceLayout, error) {
	layout := &WorkspaceLayout{
		TownRoot: townRoot,
		Type:     LayoutUnknown,
	}

	// Check for mayor/ directory
	mayorDir := filepath.Join(townRoot, "mayor")
	if info, err := os.Stat(mayorDir); err == nil && info.IsDir() {
		layout.HasMayorDir = true
	}

	// Check for root-level town.json (0.1.x style)
	rootTownJSON := filepath.Join(townRoot, "town.json")
	if _, err := os.Stat(rootTownJSON); err == nil {
		layout.HasLegacyTownJSON = true
	}

	// Check for mayor/town.json (0.2.x style)
	mayorTownJSON := filepath.Join(townRoot, "mayor", "town.json")
	if _, err := os.Stat(mayorTownJSON); err == nil {
		layout.ConfigPath = mayorTownJSON
		layout.RigsPath = filepath.Join(townRoot, "mayor", "rigs.json")
	} else if layout.HasLegacyTownJSON {
		layout.ConfigPath = rootTownJSON
		layout.RigsPath = filepath.Join(townRoot, "rigs.json")
	}

	// Try to load version from config
	if layout.ConfigPath != "" {
		if ver := loadVersionFromConfig(layout.ConfigPath); ver != "" {
			layout.Version = ver
		}
	}

	// Detect beads location
	// 0.2.x: ~/gt/.beads/ for town-level, rig/.beads for rig-level
	// 0.1.x: town_root/.beads/
	townBeads := filepath.Join(townRoot, ".beads")
	if _, err := os.Stat(townBeads); err == nil {
		layout.BeadsPath = townBeads
	}

	// Detect rigs
	layout.Rigs = detectRigs(townRoot)

	// Determine layout type based on detected characteristics
	layout.Type = determineLayoutType(layout)

	return layout, nil
}

// determineLayoutType analyzes the layout characteristics to determine the version.
func determineLayoutType(layout *WorkspaceLayout) string {
	// If we have a version in config, use it to determine layout
	if layout.Version != "" {
		ver, err := upgrade.ParseVersion(layout.Version)
		if err == nil {
			if ver.MatchesPattern("0.1.x") {
				return LayoutV01x
			}
			if ver.MatchesPattern("0.2.x") || ver.MatchesPattern("0.3.x") {
				return LayoutV02x
			}
		}
	}

	// Check for 0.2.x markers
	hasV02Markers := false
	if layout.HasMayorDir && !layout.HasLegacyTownJSON {
		// Has mayor/ but no root town.json - likely 0.2.x
		hasV02Markers = true
	}

	// Check for settings/ directories in rigs (0.2.x feature)
	for _, rig := range layout.Rigs {
		settingsDir := filepath.Join(rig, "settings")
		if _, err := os.Stat(settingsDir); err == nil {
			hasV02Markers = true
			break
		}
	}

	if hasV02Markers {
		return LayoutV02x
	}

	// Check for 0.1.x markers
	if layout.HasLegacyTownJSON {
		return LayoutV01x
	}

	// If we have mayor/ directory with town.json inside, it's 0.2.x
	if layout.HasMayorDir {
		mayorTownJSON := filepath.Join(layout.TownRoot, "mayor", "town.json")
		if _, err := os.Stat(mayorTownJSON); err == nil {
			return LayoutV02x
		}
	}

	return LayoutUnknown
}

// loadVersionFromConfig reads the gt_version field from a town.json file.
func loadVersionFromConfig(configPath string) string {
	data, err := os.ReadFile(configPath)
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

// detectRigs finds all rig directories within a town root.
// This is the single source of truth for rig discovery across the migrate package.
func detectRigs(townRoot string) []string {
	var rigs []string

	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return rigs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Skip non-rig directories using constants
		isExcluded := false
		for _, excluded := range ExcludedDirs {
			if name == excluded {
				isExcluded = true
				break
			}
		}
		if isExcluded || strings.HasPrefix(name, ".") {
			continue
		}

		rigPath := filepath.Join(townRoot, name)

		// Check if this looks like a rig (has crew/, polecats/, witness/, or refinery/)
		// Also check for mayor/rig which indicates a rig in 0.2.x layout
		markers := append(RigMarkers, MayorDir+"/rig")
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(rigPath, marker)); err == nil {
				rigs = append(rigs, rigPath)
				break
			}
		}
	}

	return rigs
}

// NeedsMigration checks if a workspace needs migration and returns details.
func NeedsMigration(townRoot string) (*CheckResult, error) {
	layout, err := DetectLayout(townRoot)
	if err != nil {
		return nil, err
	}

	result := &CheckResult{
		LayoutType: layout.Type,
	}

	// If we can't determine the layout, we can't migrate
	if layout.Type == LayoutUnknown {
		result.NeedsMigration = false
		result.Message = "Unknown workspace layout - cannot determine migration needs"
		return result, nil
	}

	// Get current version
	if layout.Version != "" {
		result.CurrentVersion = layout.Version
	} else {
		// Infer version from layout type
		switch layout.Type {
		case LayoutV01x:
			result.CurrentVersion = "0.1.0"
		case LayoutV02x:
			result.CurrentVersion = "0.2.0"
		}
	}

	// Parse current version
	currentVer, err := upgrade.ParseVersion(result.CurrentVersion)
	if err != nil {
		result.NeedsMigration = false
		result.Message = "Could not parse current version"
		return result, nil
	}

	// Find available migrations
	migration, err := FindMigration(currentVer)
	if err != nil {
		// No migration available - workspace is up to date
		result.NeedsMigration = false
		result.Message = "Workspace is up to date"
		return result, nil
	}

	result.NeedsMigration = true
	result.TargetVersion = migration.ToVersion
	result.MigrationPath = []string{migration.ID}
	result.Message = migration.Description

	// Check for chained migrations
	targetVer, err := upgrade.ParseVersion(migration.ToVersion)
	if err != nil {
		// If we can't parse the target version, we still have a valid single migration
		return result, nil
	}
	for {
		nextMigration, err := FindMigration(targetVer)
		if err != nil {
			break
		}
		result.MigrationPath = append(result.MigrationPath, nextMigration.ID)
		nextTargetVer, parseErr := upgrade.ParseVersion(nextMigration.ToVersion)
		if parseErr != nil {
			// Can't continue the chain if we can't parse the version
			result.TargetVersion = nextMigration.ToVersion
			break
		}
		targetVer = nextTargetVer
		result.TargetVersion = nextMigration.ToVersion
	}

	return result, nil
}

// GetWorkspaceVersion returns the current workspace version from town.json.
func GetWorkspaceVersion(townRoot string) (string, error) {
	layout, err := DetectLayout(townRoot)
	if err != nil {
		return "", err
	}

	if layout.Version != "" {
		return layout.Version, nil
	}

	// Infer from layout type
	switch layout.Type {
	case LayoutV01x:
		return "0.1.0", nil
	case LayoutV02x:
		return "0.2.0", nil
	default:
		return "", nil
	}
}
