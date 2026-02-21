package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// setupDoltDB creates a fake Dolt database directory under .dolt-data/.
func setupDoltDB(t *testing.T, townRoot, dbName string) {
	t.Helper()
	doltDir := filepath.Join(townRoot, ".dolt-data", dbName, ".dolt")
	if err := os.MkdirAll(doltDir, 0755); err != nil {
		t.Fatalf("creating dolt dir for %s: %v", dbName, err)
	}
	if err := os.WriteFile(filepath.Join(doltDir, "manifest"), []byte("test"), 0644); err != nil {
		t.Fatalf("writing manifest for %s: %v", dbName, err)
	}
}

// setupRigMetadata creates a .beads/metadata.json for a rig with Dolt server config.
func setupRigMetadata(t *testing.T, townRoot, rigName, doltDatabase string) {
	t.Helper()
	var beadsDir string
	if rigName == "hq" {
		beadsDir = filepath.Join(townRoot, ".beads")
	} else {
		beadsDir = filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	}
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("creating beads dir for %s: %v", rigName, err)
	}
	meta := map[string]interface{}{
		"backend":       "dolt",
		"dolt_mode":     "server",
		"dolt_database": doltDatabase,
		"jsonl_export":  "issues.jsonl",
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshaling metadata for %s: %v", rigName, err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
		t.Fatalf("writing metadata for %s: %v", rigName, err)
	}
}

// setupServerMetadata creates a .beads/metadata.json with optional host/port fields.
func setupServerMetadata(t *testing.T, beadsDir, host string, port int) {
	t.Helper()
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("creating beads dir: %v", err)
	}
	meta := map[string]interface{}{
		"backend":       "dolt",
		"dolt_mode":     "server",
		"dolt_database": "test",
		"jsonl_export":  "issues.jsonl",
	}
	if host != "" {
		meta["dolt_server_host"] = host
	}
	if port != 0 {
		meta["dolt_server_port"] = port
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshaling metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
		t.Fatalf("writing metadata: %v", err)
	}
}

// setupRigsJSON creates a minimal mayor/rigs.json for tests.
func setupRigsJSON(t *testing.T, townRoot string, rigNames []string) {
	t.Helper()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigs := "{"
	for i, name := range rigNames {
		if i > 0 {
			rigs += ","
		}
		rigs += `"` + name + `":{"git_url":"https://example.com/` + name + `.git","added_at":"2025-01-01T00:00:00Z"}`
	}
	rigs += "}"
	content := `{"version":1,"rigs":` + rigs + `}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGetServerAddr(t *testing.T) {
	check := NewDoltServerReachableCheck()

	tests := []struct {
		name     string
		host     string
		port     int
		wantAddr string
		wantOK   bool
	}{
		{
			name:     "defaults to 127.0.0.1:3307",
			wantAddr: "127.0.0.1:3307",
			wantOK:   true,
		},
		{
			name:     "explicit IPv4 host and port",
			host:     "10.0.0.5",
			port:     3308,
			wantAddr: "10.0.0.5:3308",
			wantOK:   true,
		},
		{
			name:     "IPv6 host gets bracketed",
			host:     "::1",
			wantAddr: "[::1]:3307",
			wantOK:   true,
		},
		{
			name:     "IPv6 host with explicit port",
			host:     "::1",
			port:     3309,
			wantAddr: "[::1]:3309",
			wantOK:   true,
		},
		{
			name:     "explicit host with default port",
			host:     "dolt.example.com",
			wantAddr: "dolt.example.com:3307",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadsDir := filepath.Join(t.TempDir(), ".beads")
			setupServerMetadata(t, beadsDir, tt.host, tt.port)

			addr, ok := check.getServerAddr(beadsDir)
			if ok != tt.wantOK {
				t.Fatalf("getServerAddr() ok = %v, want %v", ok, tt.wantOK)
			}
			if addr != tt.wantAddr {
				t.Errorf("getServerAddr() = %q, want %q", addr, tt.wantAddr)
			}
		})
	}
}

func TestGetServerAddr_NotServerMode(t *testing.T) {
	check := NewDoltServerReachableCheck()
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := map[string]interface{}{
		"backend":   "dolt",
		"dolt_mode": "local",
	}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	_, ok := check.getServerAddr(beadsDir)
	if ok {
		t.Error("getServerAddr() returned ok=true for local mode, want false")
	}
}

func TestGetServerAddr_NoMetadata(t *testing.T) {
	check := NewDoltServerReachableCheck()
	_, ok := check.getServerAddr(filepath.Join(t.TempDir(), "nonexistent"))
	if ok {
		t.Error("getServerAddr() returned ok=true for missing metadata, want false")
	}
}

func TestDoltOrphanedDatabaseCheck_NoOrphans(t *testing.T) {
	townRoot := t.TempDir()

	setupDoltDB(t, townRoot, "hq")
	setupDoltDB(t, townRoot, "gastown")

	setupRigsJSON(t, townRoot, []string{"gastown"})
	setupRigMetadata(t, townRoot, "hq", "hq")
	setupRigMetadata(t, townRoot, "gastown", "gastown")

	check := NewDoltOrphanedDatabaseCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltOrphanedDatabaseCheck_DetectsOrphans(t *testing.T) {
	townRoot := t.TempDir()

	setupDoltDB(t, townRoot, "hq")
	setupDoltDB(t, townRoot, "wyvern")
	setupDoltDB(t, townRoot, "beads_wy") // orphan

	setupRigsJSON(t, townRoot, []string{"wyvern"})
	setupRigMetadata(t, townRoot, "hq", "hq")
	setupRigMetadata(t, townRoot, "wyvern", "wyvern")

	check := NewDoltOrphanedDatabaseCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "1 orphaned database(s) in .dolt-data/" {
		t.Errorf("unexpected message: %s", result.Message)
	}
	if len(result.Details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(result.Details))
	}
	if result.FixHint == "" {
		t.Error("expected a fix hint")
	}
}

func TestDoltOrphanedDatabaseCheck_Fix(t *testing.T) {
	townRoot := t.TempDir()

	setupDoltDB(t, townRoot, "hq")
	setupDoltDB(t, townRoot, "orphan1")
	setupDoltDB(t, townRoot, "orphan2")

	setupRigsJSON(t, townRoot, []string{})
	setupRigMetadata(t, townRoot, "hq", "hq")

	check := NewDoltOrphanedDatabaseCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	// Run to populate orphan names
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if len(check.orphanNames) != 2 {
		t.Fatalf("expected 2 cached orphan names, got %d", len(check.orphanNames))
	}

	// Fix should remove the orphans
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix: %v", err)
	}

	// Verify orphans are gone
	for _, name := range []string{"orphan1", "orphan2"} {
		path := filepath.Join(townRoot, ".dolt-data", name)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed after Fix", name)
		}
	}

	// Verify referenced database still exists
	hqPath := filepath.Join(townRoot, ".dolt-data", "hq")
	if _, err := os.Stat(hqPath); err != nil {
		t.Errorf("expected hq database to survive Fix, but got error: %v", err)
	}
}

func TestDoltOrphanedDatabaseCheck_NoDoltData(t *testing.T) {
	townRoot := t.TempDir()

	check := NewDoltOrphanedDatabaseCheck()
	ctx := &CheckContext{TownRoot: townRoot}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for missing .dolt-data/, got %v: %s", result.Status, result.Message)
	}
}

func TestDoltOrphanedDatabaseCheck_CanFix(t *testing.T) {
	check := NewDoltOrphanedDatabaseCheck()
	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestDoltOrphanedDatabaseCheck_Name(t *testing.T) {
	check := NewDoltOrphanedDatabaseCheck()
	if check.Name() != "dolt-orphaned-databases" {
		t.Errorf("expected name 'dolt-orphaned-databases', got %q", check.Name())
	}
}

// TestHasDoltMetadata_AcceptsAnyDatabaseName verifies that hasDoltMetadata
// accepts any non-empty dolt_database value, not just the rig name.
// This is critical for shared-database mode where all rigs use beads_hq.
func TestHasDoltMetadata_AcceptsAnyDatabaseName(t *testing.T) {
	check := NewDoltMetadataCheck()

	tests := []struct {
		name       string
		dbName     string
		expectedDB string
		want       bool
	}{
		{
			name:       "shared hq database accepted for rig",
			dbName:     "beads_hq",
			expectedDB: "myrig",
			want:       true,
		},
		{
			name:       "rig-named database still accepted",
			dbName:     "myrig",
			expectedDB: "myrig",
			want:       true,
		},
		{
			name:       "empty database rejected",
			dbName:     "",
			expectedDB: "myrig",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadsDir := filepath.Join(t.TempDir(), ".beads")
			if err := os.MkdirAll(beadsDir, 0755); err != nil {
				t.Fatal(err)
			}
			meta := map[string]interface{}{
				"backend":       "dolt",
				"dolt_mode":     "server",
				"dolt_database": tt.dbName,
				"jsonl_export":  "issues.jsonl",
			}
			data, _ := json.Marshal(meta)
			if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0644); err != nil {
				t.Fatal(err)
			}

			got := check.hasDoltMetadata(beadsDir, tt.expectedDB)
			if got != tt.want {
				t.Errorf("hasDoltMetadata(db=%q, expected=%q) = %v, want %v",
					tt.dbName, tt.expectedDB, got, tt.want)
			}
		})
	}
}

// TestWriteDoltMetadata_UsesHQDatabase verifies that writeDoltMetadata uses
// the town's hq database name instead of the rig name for new metadata.
func TestWriteDoltMetadata_UsesHQDatabase(t *testing.T) {
	townRoot := t.TempDir()

	// Set up town-level .beads with hq database name
	hqBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(hqBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	hqMeta := map[string]interface{}{
		"dolt_database": "beads_hq",
		"backend":       "dolt",
		"dolt_mode":     "server",
	}
	data, _ := json.Marshal(hqMeta)
	if err := os.WriteFile(filepath.Join(hqBeadsDir, "metadata.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig beads directory (no existing metadata)
	rigBeadsDir := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewDoltMetadataCheck()
	if err := check.writeDoltMetadata(townRoot, "myrig"); err != nil {
		t.Fatalf("writeDoltMetadata failed: %v", err)
	}

	// Read and verify
	result, err := os.ReadFile(filepath.Join(rigBeadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(result, &metadata); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}

	// Should use hq's database name, NOT "myrig"
	if metadata["dolt_database"] != "beads_hq" {
		t.Errorf("dolt_database = %v, want beads_hq", metadata["dolt_database"])
	}
}

// TestWriteDoltMetadata_PreservesExistingDatabase verifies that writeDoltMetadata
// does not overwrite an existing dolt_database value.
func TestWriteDoltMetadata_PreservesExistingDatabase(t *testing.T) {
	townRoot := t.TempDir()

	// Set up town-level .beads with hq database
	hqBeadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(hqBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	hqMeta := map[string]interface{}{"dolt_database": "beads_hq"}
	data, _ := json.Marshal(hqMeta)
	if err := os.WriteFile(filepath.Join(hqBeadsDir, "metadata.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig beads with existing dolt_database
	rigBeadsDir := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigMeta := map[string]interface{}{"dolt_database": "custom_db"}
	data, _ = json.Marshal(rigMeta)
	if err := os.WriteFile(filepath.Join(rigBeadsDir, "metadata.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	check := NewDoltMetadataCheck()
	if err := check.writeDoltMetadata(townRoot, "myrig"); err != nil {
		t.Fatalf("writeDoltMetadata failed: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(rigBeadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(result, &metadata); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}

	// Should preserve existing value
	if metadata["dolt_database"] != "custom_db" {
		t.Errorf("dolt_database = %v, want custom_db (preserved)", metadata["dolt_database"])
	}
}

// TestWriteDoltMetadata_FallsBackToRigName verifies that writeDoltMetadata
// falls back to rigName when no hq metadata exists.
func TestWriteDoltMetadata_FallsBackToRigName(t *testing.T) {
	townRoot := t.TempDir()

	// NO town-level .beads/metadata.json

	// Create rig beads directory (no existing metadata)
	rigBeadsDir := filepath.Join(townRoot, "myrig", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewDoltMetadataCheck()
	if err := check.writeDoltMetadata(townRoot, "myrig"); err != nil {
		t.Fatalf("writeDoltMetadata failed: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(rigBeadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("reading metadata: %v", err)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal(result, &metadata); err != nil {
		t.Fatalf("parsing metadata: %v", err)
	}

	// No hq metadata → falls back to rigName
	if metadata["dolt_database"] != "myrig" {
		t.Errorf("dolt_database = %v, want myrig (fallback)", metadata["dolt_database"])
	}
}
