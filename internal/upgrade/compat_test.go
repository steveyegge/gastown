package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCompatibility(t *testing.T) {
	tests := []struct {
		name            string
		currentVersion  string
		targetTag       string
		compatInfo      *CompatibilityInfo
		wantCompatible  bool
		wantMigration   bool
	}{
		{
			name:           "no_compat_info",
			currentVersion: "0.2.5",
			targetTag:      "v0.2.6",
			compatInfo:     nil,
			wantCompatible: true,
			wantMigration:  false,
		},
		{
			name:           "compatible_upgrade",
			currentVersion: "0.2.5",
			targetTag:      "v0.2.6",
			compatInfo: &CompatibilityInfo{
				Version:             "0.2.6",
				MinWorkspaceVersion: "0.2.0",
			},
			wantCompatible: true,
			wantMigration:  false,
		},
		{
			name:           "incompatible_min_version",
			currentVersion: "0.1.9",
			targetTag:      "v0.3.0",
			compatInfo: &CompatibilityInfo{
				Version:             "0.3.0",
				MinWorkspaceVersion: "0.2.0",
				BreakingChanges:     []string{"beads-v2-schema"},
			},
			wantCompatible: false,
			wantMigration:  true,
		},
		{
			name:           "migration_required_from_pattern",
			currentVersion: "0.1.5",
			targetTag:      "v0.3.0",
			compatInfo: &CompatibilityInfo{
				Version:               "0.3.0",
				MigrationRequiredFrom: []string{"0.1.x"},
				BreakingChanges:       []string{"config-v2"},
			},
			wantCompatible: false,
			wantMigration:  true,
		},
		{
			name:           "migration_not_required_different_pattern",
			currentVersion: "0.2.5",
			targetTag:      "v0.3.0",
			compatInfo: &CompatibilityInfo{
				Version:               "0.3.0",
				MigrationRequiredFrom: []string{"0.1.x"},
			},
			wantCompatible: true,
			wantMigration:  false,
		},
		{
			name:           "breaking_changes_noted_but_compatible",
			currentVersion: "0.2.5",
			targetTag:      "v0.3.0",
			compatInfo: &CompatibilityInfo{
				Version:         "0.3.0",
				BreakingChanges: []string{"api-change"},
				// No MinWorkspaceVersion or MigrationRequiredFrom
			},
			wantCompatible: true,
			wantMigration:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := &ReleaseInfo{TagName: tt.targetTag}
			result := CheckCompatibility(tt.currentVersion, release, tt.compatInfo)

			if result.Compatible != tt.wantCompatible {
				t.Errorf("Compatible = %v, want %v", result.Compatible, tt.wantCompatible)
			}
			if result.MigrationRequired != tt.wantMigration {
				t.Errorf("MigrationRequired = %v, want %v", result.MigrationRequired, tt.wantMigration)
			}
		})
	}
}

func TestGetWorkspaceVersion(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write town.json with gt_version
	townJSON := `{
  "type": "town",
  "name": "test",
  "gt_version": "0.2.5"
}`
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(townJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Test reading version
	got := GetWorkspaceVersion(tmpDir)
	if got != "0.2.5" {
		t.Errorf("GetWorkspaceVersion() = %q, want %q", got, "0.2.5")
	}

	// Test with non-existent workspace
	got = GetWorkspaceVersion("/nonexistent/path")
	if got != "" {
		t.Errorf("GetWorkspaceVersion(nonexistent) = %q, want empty", got)
	}
}

func TestSetWorkspaceVersion(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write initial town.json
	townJSON := `{
  "type": "town",
  "name": "test",
  "version": 1
}`
	townPath := filepath.Join(mayorDir, "town.json")
	if err := os.WriteFile(townPath, []byte(townJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Set version
	if err := SetWorkspaceVersion(tmpDir, "0.2.6"); err != nil {
		t.Fatalf("SetWorkspaceVersion() error = %v", err)
	}

	// Verify version was set
	got := GetWorkspaceVersion(tmpDir)
	if got != "0.2.6" {
		t.Errorf("After SetWorkspaceVersion, GetWorkspaceVersion() = %q, want %q", got, "0.2.6")
	}

	// Verify other fields preserved by reading raw JSON
	data, err := os.ReadFile(townPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `"type": "town"`) && !strings.Contains(content, `"type":"town"`) {
		t.Errorf("town.json missing type field: %s", content)
	}
	if !strings.Contains(content, `"name": "test"`) && !strings.Contains(content, `"name":"test"`) {
		t.Errorf("town.json missing name field: %s", content)
	}
}

func TestFormatCompatWarning(t *testing.T) {
	result := &CompatCheckResult{
		Compatible:        false,
		WorkspaceVersion:  "0.2.5",
		TargetVersion:     "v0.3.0",
		BreakingChanges:   []string{"beads-v2-schema", "config-format"},
		MigrationRequired: true,
		MigrationGuideURL: "https://example.com/migration",
	}

	output := FormatCompatWarning(result)

	// Check that it contains key information
	if !strings.Contains(output, "Breaking changes detected") {
		t.Error("Warning should mention breaking changes")
	}
	if !strings.Contains(output, "beads-v2-schema") {
		t.Error("Warning should list breaking changes")
	}
	if !strings.Contains(output, "0.2.5") {
		t.Error("Warning should show workspace version")
	}
	if !strings.Contains(output, "https://example.com/migration") {
		t.Error("Warning should show migration guide URL")
	}
	if !strings.Contains(output, "--force") {
		t.Error("Warning should mention --force option")
	}
}
