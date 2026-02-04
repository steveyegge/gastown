// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Prefix is the common prefix for rig-level Gas Town tmux sessions.
const Prefix = "gt-"

// HQPrefix is the prefix for town-level services (Mayor, Deacon).
const HQPrefix = "hq-"

// TownIDFromRoot derives a unique town identifier from the town root path.
// Uses the base directory name, sanitized for tmux session names.
// Examples:
//   - /Users/user/gt → "gt"
//   - /Users/user/gastown → "gastown"
//   - /home/user/my-town → "my-town"
func TownIDFromRoot(townRoot string) string {
	// Clean and get base name
	clean := filepath.Clean(townRoot)
	base := filepath.Base(clean)

	// Sanitize for tmux session names (alphanumeric, dash, underscore)
	var sanitized strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			sanitized.WriteRune(r)
		}
	}

	result := sanitized.String()
	if result == "" {
		result = "default"
	}
	return result
}

// MayorSessionName returns the session name for the Mayor agent.
// Deprecated: Use MayorSessionNameForTown for multi-town support.
func MayorSessionName() string {
	return HQPrefix + "mayor"
}

// MayorSessionNameForTown returns the session name for the Mayor agent in a specific town.
// Format: hq-{townID}-mayor
// This allows multiple towns on the same machine, each with their own Mayor.
func MayorSessionNameForTown(townRoot string) string {
	townID := TownIDFromRoot(townRoot)
	return fmt.Sprintf("%s%s-mayor", HQPrefix, townID)
}

// DeaconSessionName returns the session name for the Deacon agent.
// Deprecated: Use DeaconSessionNameForTown for multi-town support.
func DeaconSessionName() string {
	return HQPrefix + "deacon"
}

// DeaconSessionNameForTown returns the session name for the Deacon agent in a specific town.
// Format: hq-{townID}-deacon
// This allows multiple towns on the same machine, each with their own Deacon.
func DeaconSessionNameForTown(townRoot string) string {
	townID := TownIDFromRoot(townRoot)
	return fmt.Sprintf("%s%s-deacon", HQPrefix, townID)
}

// WitnessSessionName returns the session name for a rig's Witness agent.
func WitnessSessionName(rig string) string {
	return fmt.Sprintf("%s%s-witness", Prefix, rig)
}

// RefinerySessionName returns the session name for a rig's Refinery agent.
func RefinerySessionName(rig string) string {
	return fmt.Sprintf("%s%s-refinery", Prefix, rig)
}

// CrewSessionName returns the session name for a crew worker in a rig.
func CrewSessionName(rig, name string) string {
	return fmt.Sprintf("%s%s-crew-%s", Prefix, rig, name)
}

// PolecatSessionName returns the session name for a polecat in a rig.
func PolecatSessionName(rig, name string) string {
	return fmt.Sprintf("%s%s-%s", Prefix, rig, name)
}

// BootSessionName returns the session name for the Boot watchdog.
// Note: We use "gt-boot" instead of "hq-deacon-boot" to avoid tmux prefix
// matching collisions. Tmux matches session names by prefix, so "hq-deacon-boot"
// would match when checking for "hq-deacon", causing HasSession("hq-deacon")
// to return true when only Boot is running.
func BootSessionName() string {
	return Prefix + "boot"
}

