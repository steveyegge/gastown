package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRunbookVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "standard version header",
			content: "# @gt-library: gastown/sling\n# @gt-version: 1.0.0\n#\n# Description...\n\njob \"gt-sling\" {\n",
			want:    "1.0.0",
		},
		{
			name:    "version only",
			content: "# @gt-version: 2.1.3\njob \"foo\" {\n",
			want:    "2.1.3",
		},
		{
			name:    "no version header",
			content: "# Some runbook\n\njob \"foo\" {\n",
			want:    "",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "version after code line (not in header)",
			content: "job \"foo\" {\n# @gt-version: 1.0.0\n}\n",
			want:    "",
		},
		{
			name:    "extra whitespace",
			content: "# @gt-version:   3.0.0  \njob \"x\" {\n",
			want:    "3.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRunbookVersion(tt.content)
			if got != tt.want {
				t.Errorf("parseRunbookVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseOjJobID(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "json format",
			output: `{"id":"job-abc-123"}`,
			want:   "job-abc-123",
		},
		{
			name:   "Job started format",
			output: "Job started: oj-xyz-789\n",
			want:   "oj-xyz-789",
		},
		{
			name:   "job_id format",
			output: "job_id: j-42\n",
			want:   "j-42",
		},
		{
			name:   "single line fallback",
			output: "simple-id",
			want:   "simple-id",
		},
		{
			name:   "empty output",
			output: "",
			want:   "",
		},
		{
			name:   "multiline non-matching",
			output: "Starting job...\nProcessing...\nDone.\n",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOjJobID(tt.output)
			if got != tt.want {
				t.Errorf("parseOjJobID(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestEnsureOjRunbook_NewInstall(t *testing.T) {
	townRoot := t.TempDir()

	// Create library source
	libDir := filepath.Join(townRoot, "library", "gastown")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	libContent := "# @gt-version: 1.0.0\n\njob \"gt-sling\" {\n}\n"
	if err := os.WriteFile(filepath.Join(libDir, "sling.hcl"), []byte(libContent), 0644); err != nil {
		t.Fatal(err)
	}

	// No target exists yet
	if err := ensureOjRunbook(townRoot); err != nil {
		t.Fatalf("ensureOjRunbook: %v", err)
	}

	// Verify target was created
	targetPath := filepath.Join(townRoot, ".oj", "runbooks", "gt-sling.hcl")
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("target not created: %v", err)
	}
	if string(data) != libContent {
		t.Errorf("target content mismatch: got %q", string(data))
	}
}

func TestEnsureOjRunbook_AlreadyCurrent(t *testing.T) {
	townRoot := t.TempDir()

	libContent := "# @gt-version: 1.0.0\n\njob \"gt-sling\" {\n}\n"

	// Create library source
	libDir := filepath.Join(townRoot, "library", "gastown")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "sling.hcl"), []byte(libContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create target with same version
	targetDir := filepath.Join(townRoot, ".oj", "runbooks")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "gt-sling.hcl"), []byte(libContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Should succeed without changes
	if err := ensureOjRunbook(townRoot); err != nil {
		t.Fatalf("ensureOjRunbook: %v", err)
	}
}

func TestEnsureOjRunbook_StaleUpdate(t *testing.T) {
	townRoot := t.TempDir()

	// Create library with v2
	libDir := filepath.Join(townRoot, "library", "gastown")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	newContent := "# @gt-version: 2.0.0\n\njob \"gt-sling\" {\n  # v2 changes\n}\n"
	if err := os.WriteFile(filepath.Join(libDir, "sling.hcl"), []byte(newContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create target with v1 (stale)
	targetDir := filepath.Join(townRoot, ".oj", "runbooks")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	oldContent := "# @gt-version: 1.0.0\n\njob \"gt-sling\" {\n}\n"
	if err := os.WriteFile(filepath.Join(targetDir, "gt-sling.hcl"), []byte(oldContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Should update target
	if err := ensureOjRunbook(townRoot); err != nil {
		t.Fatalf("ensureOjRunbook: %v", err)
	}

	// Verify target was updated to v2
	data, err := os.ReadFile(filepath.Join(targetDir, "gt-sling.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != newContent {
		t.Errorf("target not updated: got %q", string(data))
	}
}

func TestEnsureOjRunbook_NoVersionHeaderUpdates(t *testing.T) {
	townRoot := t.TempDir()

	// Library without version header
	libDir := filepath.Join(townRoot, "library", "gastown")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatal(err)
	}
	libContent := "# no version\njob \"gt-sling\" {\n}\n"
	if err := os.WriteFile(filepath.Join(libDir, "sling.hcl"), []byte(libContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Target also without version (different content)
	targetDir := filepath.Join(townRoot, ".oj", "runbooks")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	targetContent := "# old content\njob \"gt-sling\" {\n}\n"
	if err := os.WriteFile(filepath.Join(targetDir, "gt-sling.hcl"), []byte(targetContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Both have version "" — should match and NOT update
	if err := ensureOjRunbook(townRoot); err != nil {
		t.Fatalf("ensureOjRunbook: %v", err)
	}

	// Target should be unchanged
	data, err := os.ReadFile(filepath.Join(targetDir, "gt-sling.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != targetContent {
		t.Errorf("target should be unchanged when versions match: got %q", string(data))
	}
}

func TestEnsureOjRunbook_MissingSource(t *testing.T) {
	townRoot := t.TempDir()

	// No library source
	err := ensureOjRunbook(townRoot)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestOjSlingEnabled(t *testing.T) {
	tests := []struct {
		name   string
		envVal string
		setEnv bool // whether to set GT_SLING_OJ at all
		want   bool
	}{
		{
			name:   "enabled with 1",
			envVal: "1",
			setEnv: true,
			want:   true,
		},
		{
			name:   "disabled with 0",
			envVal: "0",
			setEnv: true,
			want:   false,
		},
		{
			name:   "unset returns false",
			setEnv: false,
			want:   false,
		},
		{
			name:   "true string returns false (only 1 works)",
			envVal: "true",
			setEnv: true,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv("GT_SLING_OJ", tt.envVal)
			} else {
				// Ensure the env var is not set for this subtest.
				// t.Setenv sets it and restores on cleanup, so set to empty
				// then unset to guarantee absence.
				t.Setenv("GT_SLING_OJ", "")
				os.Unsetenv("GT_SLING_OJ")
			}

			got := ojSlingEnabled()
			if got != tt.want {
				t.Errorf("ojSlingEnabled() = %v, want %v (GT_SLING_OJ=%q, set=%v)",
					got, tt.want, tt.envVal, tt.setEnv)
			}
		})
	}
}

func TestGetBeadBase(t *testing.T) {
	tests := []struct {
		name   string
		beadID string
		want   string
	}{
		{
			name:   "empty bead ID returns main",
			beadID: "",
			want:   "main",
		},
		{
			name:   "non-existent bead returns main",
			beadID: "nonexistent-bead-xyz-999",
			want:   "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBeadBase(tt.beadID)
			if got != tt.want {
				t.Errorf("GetBeadBase(%q) = %q, want %q", tt.beadID, got, tt.want)
			}
		})
	}
}

func TestStoreOjJobIDInBead_EmptyJobID(t *testing.T) {
	// storeOjJobIDInBead should return nil immediately for empty jobID (no-op).
	err := storeOjJobIDInBead("some-bead", "")
	if err != nil {
		t.Errorf("storeOjJobIDInBead(\"some-bead\", \"\") = %v, want nil", err)
	}
}
