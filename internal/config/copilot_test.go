package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCopilotTrustedFolder_NoWorkDir(t *testing.T) {
	if err := EnsureCopilotTrustedFolder(CopilotTrustConfig{}); err != nil {
		t.Fatalf("EnsureCopilotTrustedFolder returned error: %v", err)
	}
}

func TestEnsureCopilotTrustedFolder_UpdatesWhenCopilot(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	workDir := filepath.Join(rigPath, "witness", "rig")

	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "copilot"
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	xdgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgHome)

	if err := EnsureCopilotTrustedFolder(CopilotTrustConfig{
		TownRoot: townRoot,
		RigPath:  rigPath,
		WorkDir:  workDir,
	}); err != nil {
		t.Fatalf("EnsureCopilotTrustedFolder: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(xdgHome, ".copilot", "config.json"))
	if err != nil {
		t.Fatalf("ReadFile config.json: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected config.json to be written")
	}
}

func TestEnsureCopilotTrustedFolder_SkipsNonCopilot(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "testrig")
	workDir := filepath.Join(rigPath, "witness", "rig")

	townSettings := NewTownSettings()
	townSettings.DefaultAgent = "claude"
	if err := SaveTownSettings(TownSettingsPath(townRoot), townSettings); err != nil {
		t.Fatalf("SaveTownSettings: %v", err)
	}
	if err := SaveRigSettings(RigSettingsPath(rigPath), NewRigSettings()); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	xdgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgHome)

	if err := EnsureCopilotTrustedFolder(CopilotTrustConfig{
		TownRoot: townRoot,
		RigPath:  rigPath,
		WorkDir:  workDir,
	}); err != nil {
		t.Fatalf("EnsureCopilotTrustedFolder: %v", err)
	}

	if _, err := os.Stat(filepath.Join(xdgHome, ".copilot", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("expected no config.json, got err=%v", err)
	}
}
