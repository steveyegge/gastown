// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
)

// Prefix is the common prefix for rig-level Gas Town tmux sessions.
const Prefix = "gt-"

// HQPrefix is the prefix for town-level services (Mayor, Deacon).
const HQPrefix = "hq-"

// MayorSessionName returns the session name for the Mayor agent.
// When town is provided and non-empty, produces "hq-{town}-mayor" for multi-town support.
// When called with no args, produces legacy "hq-mayor" format.
func MayorSessionName(town ...string) string {
	if len(town) > 0 && town[0] != "" {
		return fmt.Sprintf("%s%s-mayor", HQPrefix, town[0])
	}
	return HQPrefix + "mayor"
}

// DeaconSessionName returns the session name for the Deacon agent.
// When town is provided and non-empty, produces "hq-{town}-deacon" for multi-town support.
// When called with no args, produces legacy "hq-deacon" format.
func DeaconSessionName(town ...string) string {
	if len(town) > 0 && town[0] != "" {
		return fmt.Sprintf("%s%s-deacon", HQPrefix, town[0])
	}
	return HQPrefix + "deacon"
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
// When town is provided and non-empty, produces "hq-{town}-boot" for multi-town support.
// When called with no args, produces legacy "gt-boot" format.
// Note: Legacy format uses "gt-boot" instead of "hq-boot" to avoid tmux prefix
// matching collisions. The new format "hq-{town}-boot" is safe because
// "hq-gt11-boot" won't prefix-match "hq-gt11-deacon".
func BootSessionName(town ...string) string {
	if len(town) > 0 && town[0] != "" {
		return fmt.Sprintf("%s%s-boot", HQPrefix, town[0])
	}
	return Prefix + "boot"
}

