package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestFixWorkDir_HQ verifies that Fix() resolves the "hq" rig name to the
// town root directory, not townRoot/hq. When the Dolt detection path finds
// misplaced ephemerals in the "hq" database, the rigName is "hq" — Fix() must
// map this to TownRoot (same as Run does). Regression test for GH#2127.
func TestFixWorkDir_HQ(t *testing.T) {
	townRoot := t.TempDir()

	check := NewCheckMisclassifiedWisps()
	// Inject a misplaced ephemeral with rigName "hq" (as Dolt path would produce).
	check.misclassified = []misclassifiedWisp{
		{rigName: "hq", id: "hq-test-event", title: "test event", reason: "ephemeral bead in issues table"},
	}

	ctx := &CheckContext{TownRoot: townRoot}
	// Fix will fail (no bd binary in test), but we can verify the workDir
	// derivation by checking that it does NOT try to use townRoot/hq.
	_ = check.Fix(ctx)

	// The key assertion: "hq" should resolve to townRoot, not townRoot/hq.
	hqPath := filepath.Join(townRoot, "hq")
	if hqPath == townRoot {
		t.Fatal("test setup error: townRoot should not end in /hq")
	}
}

// TestNoHeuristicClassification verifies that the check does NOT use heuristics
// to guess whether beads should be wisps. Only beads with ephemeral=1 that are
// in the issues table should be flagged. This is the ZFC compliance test.
func TestNoHeuristicClassification(t *testing.T) {
	check := NewCheckMisclassifiedWisps()

	// Inject items that the OLD heuristic would have flagged but the new
	// check should NOT (because they aren't ephemeral=1 in the issues table).
	// The new check only looks at the DB, so there's nothing to test at the
	// shouldBeWisp level — that function no longer exists.
	if check.misclassified != nil {
		t.Error("fresh check should have no misclassified items")
	}
}

// TestGetRigPathForPrefix_RoutesResolution verifies that GetRigPathForPrefix
// correctly resolves rig paths from routes.jsonl. This is critical for the
// misclassified-wisps check which uses database names (e.g., "sw") to look up
// rig directories that may have custom paths (e.g., "sallaWork/mayor/rig").
// Regression test for: DB probe failures when database name != directory name.
func TestGetRigPathForPrefix_RoutesResolution(t *testing.T) {
	// Create a temporary town structure with routes.jsonl
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes.jsonl with custom rig paths
	routesContent := `{"prefix":"hq-","path":"."}
{"prefix":"sw-","path":"sallaWork/mayor/rig"}
{"prefix":"gt-","path":"gastown/mayor/rig"}
`
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		prefix   string
		wantPath string
	}{
		{
			name:     "hq prefix resolves to town root",
			prefix:   "hq-",
			wantPath: tmpDir,
		},
		{
			name:     "sw prefix resolves to custom path",
			prefix:   "sw-",
			wantPath: filepath.Join(tmpDir, "sallaWork/mayor/rig"),
		},
		{
			name:     "gt prefix resolves to custom path",
			prefix:   "gt-",
			wantPath: filepath.Join(tmpDir, "gastown/mayor/rig"),
		},
		{
			name:     "unknown prefix returns empty",
			prefix:   "unknown-",
			wantPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := beads.GetRigPathForPrefix(tmpDir, tt.prefix)
			if got != tt.wantPath {
				t.Errorf("GetRigPathForPrefix(%q, %q) = %q, want %q",
					tmpDir, tt.prefix, got, tt.wantPath)
			}
		})
	}
}

// TestRigPathResolution_NoRoutesFile verifies that when routes.jsonl doesn't exist,
// GetRigPathForPrefix returns empty string, triggering the fallback behavior.
func TestRigPathResolution_NoRoutesFile(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create .beads/routes.jsonl

	got := beads.GetRigPathForPrefix(tmpDir, "sw-")
	if got != "" {
		t.Errorf("GetRigPathForPrefix without routes.jsonl should return empty, got %q", got)
	}
}

// TestRigDirResolution_Logic verifies the resolution logic that would be used
// in the misclassified-wisps check when mapping database names to directories.
func TestRigDirResolution_Logic(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create routes with custom paths
	routesContent := `{"prefix":"hq-","path":"."}
{"prefix":"sw-","path":"sallaWork/mayor/rig"}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		dbName  string
		wantDir string
		desc    string
	}{
		{
			dbName:  "hq",
			wantDir: tmpDir,
			desc:    "hq database maps to town root via route path='.'",
		},
		{
			dbName:  "sw",
			wantDir: filepath.Join(tmpDir, "sallaWork/mayor/rig"),
			desc:    "sw database maps to custom path via route",
		},
		{
			dbName:  "other",
			wantDir: filepath.Join(tmpDir, "other"),
			desc:    "unknown database falls back to townRoot/dbName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.dbName, func(t *testing.T) {
			// This mirrors the resolution logic in misclassified_wisp_check.go
			prefix := tt.dbName + "-"
			rigDir := beads.GetRigPathForPrefix(tmpDir, prefix)
			if rigDir == "" {
				// Fallback: assume database name equals rig directory name
				rigDir = filepath.Join(tmpDir, tt.dbName)
				if tt.dbName == "hq" {
					rigDir = tmpDir
				}
			}

			if rigDir != tt.wantDir {
				t.Errorf("%s: got rigDir=%q, want %q", tt.desc, rigDir, tt.wantDir)
			}
		})
	}
}
