// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
)

// Prefix is the common prefix for rig-level Gas Town tmux sessions.
const Prefix = "gt-"

// HQPrefix is the legacy prefix for town-level services.
// Deprecated: Town-level sessions now use the town name as prefix (e.g., "mytown-mayor").
// Kept for backward compatibility in ParseSessionName.
const HQPrefix = "hq-"

// currentTownName is the town name used for generating town-level session names.
// Set via SetTownName during CLI initialization. Defaults to "hq" for backward
// compatibility with single-town installations.
var currentTownName = "hq"

// SetTownName configures the town name used for town-level tmux session names.
// This should be called once during CLI initialization with the name from town.json.
// When multiple Gas Town instances run concurrently (e.g., ~/gt-redos, ~/gt-nyx),
// each uses its own town name to avoid tmux session name collisions.
//
// The name "gt" is rejected because it would create session names like "gt-mayor"
// that collide with the rig-level "gt-" prefix used by ParseSessionName.
func SetTownName(name string) {
	if name == "" || name == "gt" {
		return
	}
	currentTownName = name
}

// GetTownName returns the currently configured town name for session naming.
func GetTownName() string {
	return currentTownName
}

// townPrefix returns the prefix for town-level session names.
func townPrefix() string {
	return currentTownName + "-"
}

// MayorSessionName returns the session name for the Mayor agent.
// Format: <townName>-mayor (e.g., "hq-mayor", "redos-mayor", "nyx-mayor").
func MayorSessionName() string {
	return townPrefix() + "mayor"
}

// DeaconSessionName returns the session name for the Deacon agent.
// Format: <townName>-deacon (e.g., "gt-deacon", "redos-deacon").
func DeaconSessionName() string {
	return townPrefix() + "deacon"
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

// OverseerSessionName returns the session name for the human operator.
// The overseer is the human who controls Gas Town, not an AI agent.
// Format: <townName>-overseer (e.g., "gt-overseer", "redos-overseer").
func OverseerSessionName() string {
	return townPrefix() + "overseer"
}

// BootSessionName returns the session name for the Boot watchdog.
// Format: <townName>-boot (e.g., "gt-boot", "redos-boot").
// Using <town>-boot avoids tmux prefix matching collisions with <town>-deacon.
func BootSessionName() string {
	return townPrefix() + "boot"
}

