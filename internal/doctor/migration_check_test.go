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
