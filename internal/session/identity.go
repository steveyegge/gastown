// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"strings"
)

// Role represents the type of Gas Town agent.
type Role string

const (
	RoleMayor    Role = "mayor"
	RoleDeacon   Role = "deacon"
	RoleOverseer Role = "overseer"
	RoleWitness  Role = "witness"
	RoleRefinery Role = "refinery"
	RoleCrew     Role = "crew"
	RolePolecat  Role = "polecat"
)

// AgentIdentity represents a parsed Gas Town agent identity.
type AgentIdentity struct {
	Role   Role   // mayor, deacon, witness, refinery, crew, polecat
	Rig    string // rig name (empty for mayor/deacon)
	Name   string // crew/polecat name (empty for mayor/deacon/witness/refinery)
	Prefix string // beads prefix for rig-level agents (e.g., "gt", "bd", "hop")
}

// ParseAddress parses a mail-style address into an AgentIdentity.
func ParseAddress(address string) (*AgentIdentity, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("empty address")
	}

	if address == "mayor" || address == "mayor/" {
		return &AgentIdentity{Role: RoleMayor}, nil
	}
	if address == "deacon" || address == "deacon/" {
		return &AgentIdentity{Role: RoleDeacon}, nil
	}
	if address == "overseer" {
		return nil, fmt.Errorf("overseer has no session")
	}

	address = strings.TrimSuffix(address, "/")
	parts := strings.Split(address, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid address %q", address)
	}

	rig := parts[0]
	prefix := PrefixFor(rig)
	switch len(parts) {
	case 2:
		name := parts[1]
		switch name {
		case "witness":
			return &AgentIdentity{Role: RoleWitness, Rig: rig, Prefix: prefix}, nil
		case "refinery":
			return &AgentIdentity{Role: RoleRefinery, Rig: rig, Prefix: prefix}, nil
		case "crew", "polecats":
			return nil, fmt.Errorf("invalid address %q", address)
		default:
			return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: name, Prefix: prefix}, nil
		}
	case 3:
		role := parts[1]
		name := parts[2]
		switch role {
		case "crew":
			return &AgentIdentity{Role: RoleCrew, Rig: rig, Name: name, Prefix: prefix}, nil
		case "polecats":
			return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: name, Prefix: prefix}, nil
		default:
			return nil, fmt.Errorf("invalid address %q", address)
		}
	default:
		return nil, fmt.Errorf("invalid address %q", address)
	}
}

// ParseSessionName parses a tmux session name into an AgentIdentity.
// Uses the default PrefixRegistry to resolve rig-level prefixes to rig names.
//
// Session name formats:
//   - hq-mayor → Role: mayor (town-level, one per machine)
//   - hq-deacon → Role: deacon (town-level, one per machine)
//   - hq-boot → Role: deacon, Name: boot (boot watchdog)
//   - <prefix>-witness → Role: witness (e.g., gt-witness for gastown)
//   - <prefix>-refinery → Role: refinery (e.g., gt-refinery for gastown)
//   - <prefix>-crew-<name> → Role: crew (e.g., gt-crew-max for gastown)
//   - <prefix>-<name> → Role: polecat (e.g., gt-furiosa for gastown)
//
// The prefix is the rig's beads prefix (e.g., "gt" for gastown, "dolt" for beads).
// The rig name is resolved from the default PrefixRegistry. If the prefix is
// not in the registry, the prefix itself is used as the rig name.
func ParseSessionName(session string) (*AgentIdentity, error) {
	return ParseSessionNameWithRegistry(session, defaultRegistry)
}

// ParseSessionNameWithRegistry parses a tmux session name using a specific registry.
// If registry is nil, an empty registry is used (prefix will not resolve to rig name).
func ParseSessionNameWithRegistry(session string, registry *PrefixRegistry) (*AgentIdentity, error) {
	if registry == nil {
		registry = NewPrefixRegistry()
	}

	// Check for town-level roles (hq- prefix)
	if strings.HasPrefix(session, HQPrefix) {
		suffix := strings.TrimPrefix(session, HQPrefix)
		switch suffix {
		case "mayor":
			return &AgentIdentity{Role: RoleMayor}, nil
		case "deacon":
			return &AgentIdentity{Role: RoleDeacon}, nil
		case "boot":
			return &AgentIdentity{Role: RoleDeacon, Name: "boot"}, nil
		case "overseer":
			return &AgentIdentity{Role: RoleOverseer}, nil
		default:
			return nil, fmt.Errorf("invalid session name %q: unknown hq- role", session)
		}
	}

	// Rig-level roles: <prefix>-<rest>
	// Use registry to identify the prefix boundary
	prefix, rest, _ := registry.matchPrefix(session)
	if prefix == "" || rest == "" {
		return nil, fmt.Errorf("invalid session name %q: cannot determine prefix", session)
	}

	rig := registry.RigForPrefix(prefix)

	// Check for witness (suffix marker)
	if rest == "witness" {
		return &AgentIdentity{Role: RoleWitness, Rig: rig, Prefix: prefix}, nil
	}

	// Check for refinery (suffix marker)
	if rest == "refinery" {
		return &AgentIdentity{Role: RoleRefinery, Rig: rig, Prefix: prefix}, nil
	}

	// Check for crew (marker in rest)
	if strings.HasPrefix(rest, "crew-") {
		name := rest[5:] // len("crew-") = 5
		if name == "" {
			return nil, fmt.Errorf("invalid session name %q: empty crew name", session)
		}
		return &AgentIdentity{Role: RoleCrew, Rig: rig, Name: name, Prefix: prefix}, nil
	}

	// Default: polecat
	// rest is the polecat name (may contain dashes)
	if rest == "" {
		return nil, fmt.Errorf("invalid session name %q: empty polecat name", session)
	}
	return &AgentIdentity{Role: RolePolecat, Rig: rig, Name: rest, Prefix: prefix}, nil
}

// SessionName returns the tmux session name for this identity.
func (a *AgentIdentity) SessionName() string {
	switch a.Role {
	case RoleMayor:
		return MayorSessionName()
	case RoleDeacon:
		if a.Name == "boot" {
			return BootSessionName()
		}
		return DeaconSessionName()
	case RoleOverseer:
		return OverseerSessionName()
	case RoleWitness:
		return WitnessSessionName(a.prefix())
	case RoleRefinery:
		return RefinerySessionName(a.prefix())
	case RoleCrew:
		return CrewSessionName(a.prefix(), a.Name)
	case RolePolecat:
		return PolecatSessionName(a.prefix(), a.Name)
	default:
		return ""
	}
}

// prefix returns the rig prefix, falling back to registry lookup or DefaultPrefix.
func (a *AgentIdentity) prefix() string {
	if a.Prefix != "" {
		return a.Prefix
	}
	if a.Rig != "" {
		return PrefixFor(a.Rig)
	}
	return DefaultPrefix
}

// BeaconAddress returns a human-readable, non-path-like address for use in
// startup beacons. Unlike Address(), this format prevents LLMs from
// misinterpreting the recipient as a filesystem path.
// Examples:
//   - mayor → "mayor"
//   - deacon → "deacon"
//   - witness → "witness (rig: gastown)"
//   - crew → "crew max (rig: gastown)"
//   - polecat → "polecat Toast (rig: gastown)"
func (a *AgentIdentity) BeaconAddress() string {
	switch a.Role {
	case RoleMayor:
		return "mayor"
	case RoleDeacon:
		return "deacon"
	case RoleOverseer:
		return "overseer"
	case RoleWitness:
		return BeaconRecipient("witness", "", a.Rig)
	case RoleRefinery:
		return BeaconRecipient("refinery", "", a.Rig)
	case RoleCrew:
		return BeaconRecipient("crew", a.Name, a.Rig)
	case RolePolecat:
		return BeaconRecipient("polecat", a.Name, a.Rig)
	default:
		return ""
	}
}

// Address returns the mail-style address for this identity.
// Examples:
//   - mayor → "mayor"
//   - deacon → "deacon"
//   - witness → "gastown/witness"
//   - refinery → "gastown/refinery"
//   - crew → "gastown/crew/max"
//   - polecat → "gastown/polecats/Toast"
func (a *AgentIdentity) Address() string {
	switch a.Role {
	case RoleMayor:
		return "mayor"
	case RoleDeacon:
		return "deacon"
	case RoleOverseer:
		return "overseer"
	case RoleWitness:
		return fmt.Sprintf("%s/witness", a.Rig)
	case RoleRefinery:
		return fmt.Sprintf("%s/refinery", a.Rig)
	case RoleCrew:
		return fmt.Sprintf("%s/crew/%s", a.Rig, a.Name)
	case RolePolecat:
		return fmt.Sprintf("%s/polecats/%s", a.Rig, a.Name)
	default:
		return ""
	}
}

// GTRole returns the GT_ROLE environment variable format.
// This is the same as Address() for most roles.
func (a *AgentIdentity) GTRole() string {
	return a.Address()
}
