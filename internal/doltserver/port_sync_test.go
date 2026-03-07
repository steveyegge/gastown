package doltserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncPortFiles_WritesToAllRigBeadsDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"alpha":{},"beta":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .beads dirs for town and rigs
	dirs := []string{
		filepath.Join(tmpDir, ".beads"),
		filepath.Join(tmpDir, "alpha", ".beads"),
		filepath.Join(tmpDir, "beta", ".beads"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Sync with port 3307
	if err := SyncPortFiles(tmpDir, 3307); err != nil {
		t.Fatalf("SyncPortFiles: %v", err)
	}

	// Verify all port files have correct content
	for _, d := range dirs {
		portFile := filepath.Join(d, "dolt-server.port")
		data, err := os.ReadFile(portFile)
		if err != nil {
			t.Errorf("missing port file in %s: %v", d, err)
			continue
		}
		if string(data) != "3307" {
			t.Errorf("port file in %s = %q, want %q", d, string(data), "3307")
		}
	}
}

func TestSyncPortFiles_SkipsMissingBeadsDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create rigs.json with a rig that has no .beads/ dir
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"nobeads":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// No .beads/ dir created for the rig — should not error
	if err := SyncPortFiles(tmpDir, 3307); err != nil {
		t.Fatalf("SyncPortFiles should not error for missing dirs: %v", err)
	}

	// Verify no port file was created
	portFile := filepath.Join(tmpDir, "nobeads", ".beads", "dolt-server.port")
	if _, err := os.Stat(portFile); !os.IsNotExist(err) {
		t.Errorf("port file should not be created for missing .beads/ dir")
	}
}

func TestSyncPortFiles_NoRigsJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// No rigs.json — should not error
	if err := SyncPortFiles(tmpDir, 3307); err != nil {
		t.Fatalf("SyncPortFiles should not error without rigs.json: %v", err)
	}
}

func TestCheckPortFiles_DetectsDrift(t *testing.T) {
	tmpDir := t.TempDir()

	// Create rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"myrig":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .beads dirs
	townBeads := filepath.Join(tmpDir, ".beads")
	rigBeads := filepath.Join(tmpDir, "myrig", ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}

	// Write wrong port to town, leave rig missing
	if err := os.WriteFile(filepath.Join(townBeads, "dolt-server.port"), []byte("13332"), 0644); err != nil {
		t.Fatal(err)
	}

	drifted := CheckPortFiles(tmpDir, 3307)
	if len(drifted) != 2 {
		t.Fatalf("expected 2 drifted, got %d", len(drifted))
	}

	// Verify drift details
	found := map[string]bool{}
	for _, d := range drifted {
		found[d.BeadsDir] = true
		if d.ExpectedPort != 3307 {
			t.Errorf("expected port 3307, got %d", d.ExpectedPort)
		}
	}
	if !found[townBeads] {
		t.Errorf("town beads dir not in drifted list")
	}
	if !found[rigBeads] {
		t.Errorf("rig beads dir not in drifted list")
	}
}

func TestCheckPortFiles_NoDriftWhenCorrect(t *testing.T) {
	tmpDir := t.TempDir()

	// Create rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"myrig":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .beads dirs with correct port
	for _, dir := range []string{
		filepath.Join(tmpDir, ".beads"),
		filepath.Join(tmpDir, "myrig", ".beads"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "dolt-server.port"), []byte("3307"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	drifted := CheckPortFiles(tmpDir, 3307)
	if len(drifted) != 0 {
		t.Errorf("expected 0 drifted, got %d", len(drifted))
	}
}

func TestSyncPortFiles_OverwritesStalePort(t *testing.T) {
	tmpDir := t.TempDir()

	// Create rigs.json
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	rigsJSON := `{"version":1,"rigs":{"rig1":{}}}`
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(rigsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .beads with stale port
	rigBeads := filepath.Join(tmpDir, "rig1", ".beads")
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rigBeads, "dolt-server.port"), []byte("13523"), 0644); err != nil {
		t.Fatal(err)
	}

	// Sync should overwrite
	if err := SyncPortFiles(tmpDir, 3307); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(rigBeads, "dolt-server.port"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "3307" {
		t.Errorf("port file = %q, want %q", string(data), "3307")
	}
}
