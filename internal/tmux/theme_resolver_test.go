package tmux

import (
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestResolveSessionTheme_AutoRigTheme(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	got := ResolveSessionTheme(townRoot, "gastown", "crew")
	want := AssignTheme("gastown")

	if got == nil {
		t.Fatal("ResolveSessionTheme returned nil, want auto theme")
	}
	if *got != want {
		t.Fatalf("ResolveSessionTheme = %+v, want %+v", *got, want)
	}
}

func TestResolveSessionTheme_DisabledRigTheme(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	settings := config.NewRigSettings()
	settings.Theme = &config.ThemeConfig{Disabled: true}
	rigPath := filepath.Join(townRoot, "gastown")
	if err := config.SaveRigSettings(config.RigSettingsPath(rigPath), settings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	if got := ResolveSessionTheme(townRoot, "gastown", "crew"); got != nil {
		t.Fatalf("ResolveSessionTheme = %+v, want nil", *got)
	}
}

func TestResolveSessionTheme_NamedRigTheme(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	settings := config.NewRigSettings()
	settings.Theme = &config.ThemeConfig{Name: "forest"}
	rigPath := filepath.Join(townRoot, "gastown")
	if err := config.SaveRigSettings(config.RigSettingsPath(rigPath), settings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	got := ResolveSessionTheme(townRoot, "gastown", "crew")
	if got == nil || got.Name != "forest" {
		t.Fatalf("ResolveSessionTheme = %+v, want forest", got)
	}
}

func TestResolveSessionTheme_CustomRigTheme(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	settings := config.NewRigSettings()
	settings.Theme = &config.ThemeConfig{
		Custom: &config.CustomTheme{BG: "#111111", FG: "#eeeeee"},
	}
	rigPath := filepath.Join(townRoot, "gastown")
	if err := config.SaveRigSettings(config.RigSettingsPath(rigPath), settings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	got := ResolveSessionTheme(townRoot, "gastown", "crew")
	if got == nil {
		t.Fatal("ResolveSessionTheme returned nil, want custom theme")
	}
	if got.BG != "#111111" || got.FG != "#eeeeee" {
		t.Fatalf("ResolveSessionTheme = %+v, want custom colors", *got)
	}
}

func TestResolveSessionTheme_RoleOverrideNoneWins(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	settings := config.NewRigSettings()
	settings.Theme = &config.ThemeConfig{
		Name: "forest",
		RoleThemes: map[string]string{
			"witness": "none",
		},
	}
	rigPath := filepath.Join(townRoot, "gastown")
	if err := config.SaveRigSettings(config.RigSettingsPath(rigPath), settings); err != nil {
		t.Fatalf("SaveRigSettings: %v", err)
	}

	if got := ResolveSessionTheme(townRoot, "gastown", "witness"); got != nil {
		t.Fatalf("ResolveSessionTheme = %+v, want nil", *got)
	}
}

func TestResolveSessionTheme_MayorAndDeaconTownOverrides(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()
	mayorCfg := config.NewMayorConfig()
	mayorCfg.Theme = &config.TownThemeConfig{
		RoleDefaults: map[string]string{
			"mayor":  "forest",
			"deacon": "plum",
		},
	}
	if err := config.SaveMayorConfig(filepath.Join(townRoot, "mayor", "config.json"), mayorCfg); err != nil {
		t.Fatalf("SaveMayorConfig: %v", err)
	}

	mayorTheme := ResolveSessionTheme(townRoot, "", "mayor")
	if mayorTheme == nil || mayorTheme.Name != "forest" {
		t.Fatalf("mayor theme = %+v, want forest", mayorTheme)
	}

	deaconTheme := ResolveSessionTheme(townRoot, "", "deacon")
	if deaconTheme == nil || deaconTheme.Name != "plum" {
		t.Fatalf("deacon theme = %+v, want plum", deaconTheme)
	}
}
