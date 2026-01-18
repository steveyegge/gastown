// Package tmux provides theme support for Gas Town tmux sessions.
package tmux

import (
	"hash/fnv"
)

// Theme represents visual styling for a tmux session.
type Theme struct {
	Name string // Human-readable theme name
	BG   string // Background color (hex or color name)
	FG   string // Foreground color (hex or color name)
}

// Style returns the style string for this theme (e.g., "bg=#1e3a5f,fg=#e0e0e0").
func (t Theme) Style() string {
	return "bg=" + t.BG + ",fg=" + t.FG
}

// DefaultPalette is the curated set of distinct, professional color themes.
// Each theme has good contrast and is visually distinct from others.
var DefaultPalette = []Theme{
	{Name: "ocean", BG: "#1e3a5f", FG: "#e0e0e0"},    // Deep blue
	{Name: "forest", BG: "#2d5a3d", FG: "#e0e0e0"},   // Forest green
	{Name: "rust", BG: "#8b4513", FG: "#f5f5dc"},     // Rust/brown
	{Name: "plum", BG: "#4a3050", FG: "#e0e0e0"},     // Purple
	{Name: "slate", BG: "#4a5568", FG: "#e0e0e0"},    // Slate gray
	{Name: "ember", BG: "#b33a00", FG: "#f5f5dc"},    // Burnt orange
	{Name: "midnight", BG: "#1a1a2e", FG: "#c0c0c0"}, // Dark blue-black
	{Name: "wine", BG: "#722f37", FG: "#f5f5dc"},     // Burgundy
	{Name: "teal", BG: "#0d5c63", FG: "#e0e0e0"},     // Teal
	{Name: "copper", BG: "#6d4c41", FG: "#f5f5dc"},   // Warm brown
}

// DefaultTheme returns a neutral theme for sessions without a specific theme.
func DefaultTheme() Theme {
	return Theme{Name: "default", BG: "#4a5568", FG: "#e0e0e0"} // Slate gray
}

// MayorTheme returns the special theme for the Mayor session.
// Gold/dark to distinguish it from rig themes.
func MayorTheme() Theme {
	return Theme{Name: "mayor", BG: "#3d3200", FG: "#ffd700"}
}

// DeaconTheme returns the special theme for the Deacon session.
// Purple/silver - ecclesiastical, distinct from Mayor's gold.
func DeaconTheme() Theme {
	return Theme{Name: "deacon", BG: "#2d1f3d", FG: "#c0b0d0"}
}

// GetThemeByName finds a theme by name from the default palette.
// Returns nil if not found.
func GetThemeByName(name string) *Theme {
	for _, t := range DefaultPalette {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

// AssignTheme picks a theme for a rig based on its name.
// Uses consistent hashing so the same rig always gets the same color.
func AssignTheme(rigName string) Theme {
	return AssignThemeFromPalette(rigName, DefaultPalette)
}

// AssignThemeFromPalette picks a theme using a custom palette.
func AssignThemeFromPalette(rigName string, palette []Theme) Theme {
	if len(palette) == 0 {
		return DefaultPalette[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(rigName))
	idx := int(h.Sum32()) % len(palette)
	return palette[idx]
}

// ListThemeNames returns the names of all themes in the default palette.
func ListThemeNames() []string {
	names := make([]string, len(DefaultPalette))
	for i, t := range DefaultPalette {
		names[i] = t.Name
	}
	return names
}

// SessionConfigForRole returns the visual session configuration for a role.
// This centralizes role identity (theme, worker name) in one place.
// For rig-level roles (witness, refinery, crew, polecat), pass the rigName.
// EnvVars should be set separately using config.AgentEnv().
func SessionConfigForRole(role, rigName string) SessionConfig {
	cfg := SessionConfig{}

	switch role {
	case "mayor":
		cfg.Theme = MayorTheme()
		cfg.Worker = "Mayor"
		cfg.Role = "coordinator"
	case "deacon":
		cfg.Theme = DeaconTheme()
		cfg.Worker = "Deacon"
		cfg.Role = "health-check"
	case "witness":
		cfg.Theme = AssignTheme(rigName)
		cfg.Worker = "Witness"
		cfg.Role = "witness"
		cfg.Rig = rigName
	case "refinery":
		cfg.Theme = AssignTheme(rigName)
		cfg.Worker = "Refinery"
		cfg.Role = "refinery"
		cfg.Rig = rigName
	case "crew":
		cfg.Theme = AssignTheme(rigName)
		cfg.Worker = "Crew"
		cfg.Role = "crew"
		cfg.Rig = rigName
	case "polecat":
		cfg.Theme = AssignTheme(rigName)
		cfg.Worker = "Polecat"
		cfg.Role = "polecat"
		cfg.Rig = rigName
	default:
		cfg.Theme = DefaultTheme()
		cfg.Worker = role
		cfg.Role = role
	}

	return cfg
}
