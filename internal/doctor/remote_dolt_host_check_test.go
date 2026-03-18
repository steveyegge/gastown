package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoteDoltHostCheck_NoTownRoot(t *testing.T) {
	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: ""})
	if result.Status != StatusOK {
		t.Errorf("expected OK for empty TownRoot, got %s", result.Status)
	}
}

func TestRemoteDoltHostCheck_NoDaemonJSON(t *testing.T) {
	tmpDir := t.TempDir()
	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: tmpDir})
	if result.Status != StatusOK {
		t.Errorf("expected OK when daemon.json missing, got %s: %s", result.Status, result.Message)
	}
}

func TestRemoteDoltHostCheck_LocalHost(t *testing.T) {
	tmpDir := t.TempDir()
	writeDaemonJSONHost(t, tmpDir, "127.0.0.1")

	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: tmpDir})
	if result.Status != StatusOK {
		t.Errorf("expected OK for local host, got %s: %s", result.Status, result.Message)
	}
}

func TestRemoteDoltHostCheck_LocalhostKeyword(t *testing.T) {
	tmpDir := t.TempDir()
	writeDaemonJSONHost(t, tmpDir, "localhost")

	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: tmpDir})
	if result.Status != StatusOK {
		t.Errorf("expected OK for 'localhost', got %s: %s", result.Status, result.Message)
	}
}

func TestRemoteDoltHostCheck_RemoteHostNoEnv(t *testing.T) {
	tmpDir := t.TempDir()
	writeDaemonJSONHost(t, tmpDir, "mini2.local")

	t.Setenv("GT_DOLT_HOST", "")

	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: tmpDir})
	if result.Status != StatusWarning {
		t.Errorf("expected Warning when remote host set but GT_DOLT_HOST not in env, got %s: %s", result.Status, result.Message)
	}
	if result.FixHint == "" {
		t.Error("expected FixHint to be set")
	}
}

func TestRemoteDoltHostCheck_RemoteHostMatchesEnv(t *testing.T) {
	tmpDir := t.TempDir()
	writeDaemonJSONHost(t, tmpDir, "100.64.1.5")

	t.Setenv("GT_DOLT_HOST", "100.64.1.5")

	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: tmpDir})
	if result.Status != StatusOK {
		t.Errorf("expected OK when GT_DOLT_HOST matches daemon.json, got %s: %s", result.Status, result.Message)
	}
}

func TestRemoteDoltHostCheck_ConflictingHosts(t *testing.T) {
	tmpDir := t.TempDir()
	writeDaemonJSONHost(t, tmpDir, "mini2.local")

	t.Setenv("GT_DOLT_HOST", "mini3.local")

	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: tmpDir})
	if result.Status != StatusWarning {
		t.Errorf("expected Warning when hosts conflict, got %s: %s", result.Status, result.Message)
	}
}

func TestRemoteDoltHostCheck_EmptyDoltServerInDaemonJSON(t *testing.T) {
	tmpDir := t.TempDir()
	// Write daemon.json with no dolt_server section
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"daemon-patrol-config","version":1}`
	if err := os.WriteFile(filepath.Join(mayorDir, "daemon.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewRemoteDoltHostCheck()
	result := check.Run(&CheckContext{TownRoot: tmpDir})
	if result.Status != StatusOK {
		t.Errorf("expected OK when no dolt_server in daemon.json, got %s", result.Status)
	}
}

func TestRemoteDoltHostCheck_CannotFix(t *testing.T) {
	check := NewRemoteDoltHostCheck()
	if check.CanFix() {
		t.Error("RemoteDoltHostCheck should not be auto-fixable (cannot set shell env vars)")
	}
}

// writeDaemonJSONHost writes a daemon.json with the given dolt_server.host.
func writeDaemonJSONHost(t *testing.T, townRoot, host string) {
	t.Helper()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	config := map[string]interface{}{
		"type":    "daemon-patrol-config",
		"version": 1,
		"patrols": map[string]interface{}{
			"dolt_server": map[string]interface{}{
				"host": host,
			},
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "daemon.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
