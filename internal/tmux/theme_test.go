package tmux

import (
	"testing"
)

func TestAssignTheme_Deterministic(t *testing.T) {
	// Same rig name should always get same theme
	theme1 := AssignTheme("gastown")
	theme2 := AssignTheme("gastown")

	if theme1.Name != theme2.Name {
		t.Errorf("AssignTheme not deterministic: got %s and %s for same input", theme1.Name, theme2.Name)
	}
}

func TestAssignTheme_Distribution(t *testing.T) {
	// Different rig names should (mostly) get different themes
	// With 10 themes and good hashing, collisions should be rare
	rigs := []string{"gastown", "beads", "myproject", "frontend", "backend", "api", "web", "mobile"}
	themes := make(map[string]int)

	for _, rig := range rigs {
		theme := AssignTheme(rig)
		themes[theme.Name]++
	}

	// We should have at least 4 different themes for 8 rigs
	if len(themes) < 4 {
		t.Errorf("Poor distribution: only %d different themes for %d rigs", len(themes), len(rigs))
	}
}

func TestGetThemeByName(t *testing.T) {
	tests := []struct {
		name  string
		want  bool
	}{
		{"ocean", true},
		{"forest", true},
		{"nonexistent", false},
		{"", false},
	}

	for _, tt := range tests {
		theme := GetThemeByName(tt.name)
		got := theme != nil
		if got != tt.want {
			t.Errorf("GetThemeByName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestThemeStyle(t *testing.T) {
	theme := Theme{Name: "test", BG: "#1e3a5f", FG: "#e0e0e0"}
	want := "bg=#1e3a5f,fg=#e0e0e0"
	got := theme.Style()

	if got != want {
		t.Errorf("Theme.Style() = %q, want %q", got, want)
	}
}

func TestMayorTheme(t *testing.T) {
	theme := MayorTheme()

	if theme.Name != "mayor" {
		t.Errorf("MayorTheme().Name = %q, want %q", theme.Name, "mayor")
	}

	// Mayor should have distinct gold/dark colors
	if theme.BG == "" || theme.FG == "" {
		t.Error("MayorTheme() has empty colors")
	}
}

func TestListThemeNames(t *testing.T) {
	names := ListThemeNames()

	if len(names) != len(DefaultPalette) {
		t.Errorf("ListThemeNames() returned %d names, want %d", len(names), len(DefaultPalette))
	}

	// Check that known themes are in the list
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	for _, want := range []string{"ocean", "forest", "rust"} {
		if !found[want] {
			t.Errorf("ListThemeNames() missing %q", want)
		}
	}
}

func TestDefaultPaletteHasDistinctColors(t *testing.T) {
	// Ensure no duplicate colors in the palette
	bgColors := make(map[string]string)
	for _, theme := range DefaultPalette {
		if existing, ok := bgColors[theme.BG]; ok {
			t.Errorf("Duplicate BG color %s used by %s and %s", theme.BG, existing, theme.Name)
		}
		bgColors[theme.BG] = theme.Name
	}
}

func TestAssignThemeFromPalette_EmptyPalette(t *testing.T) {
	// Empty palette should return first default theme
	theme := AssignThemeFromPalette("test", []Theme{})
	if theme.Name != DefaultPalette[0].Name {
		t.Errorf("AssignThemeFromPalette with empty palette = %q, want %q", theme.Name, DefaultPalette[0].Name)
	}
}

func TestAssignThemeFromPalette_CustomPalette(t *testing.T) {
	custom := []Theme{
		{Name: "custom1", BG: "#111", FG: "#fff"},
		{Name: "custom2", BG: "#222", FG: "#fff"},
	}

	// Should only return themes from custom palette
	theme := AssignThemeFromPalette("test", custom)
	if theme.Name != "custom1" && theme.Name != "custom2" {
		t.Errorf("AssignThemeFromPalette returned %q, want one of custom themes", theme.Name)
	}
}
