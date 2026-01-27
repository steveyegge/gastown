// Package constants defines shared constant values used throughout Gas Town.
// Centralizing these magic strings improves maintainability and consistency.
package constants

import "time"

// Timing constants for session management and tmux operations.
const (
	// ShutdownNotifyDelay is the pause after sending shutdown notification.
	ShutdownNotifyDelay = 500 * time.Millisecond

	// ClaudeStartTimeout is how long to wait for Claude to start in a session.
	// Increased to 60s because Claude can take 30s+ on slower machines.
	ClaudeStartTimeout = 60 * time.Second

	// ShellReadyTimeout is how long to wait for shell prompt after command.
	ShellReadyTimeout = 5 * time.Second

	// DefaultDebounceMs is the default debounce for SendKeys operations.
	// 500ms is required for Claude Code to reliably process paste before Enter.
	// See NudgeSession comment: "Wait 500ms for paste to complete (tested, required)"
	DefaultDebounceMs = 500

	// DefaultDisplayMs is the default duration for tmux display-message.
	DefaultDisplayMs = 5000

	// PollInterval is the default polling interval for wait loops.
	PollInterval = 100 * time.Millisecond
)

// Directory names within a Gas Town workspace.
const (
	// DirMayor is the directory containing mayor configuration and state.
	DirMayor = "mayor"

	// DirPolecats is the directory containing polecat worktrees.
	DirPolecats = "polecats"

	// DirCrew is the directory containing crew workspaces.
	DirCrew = "crew"

	// DirRefinery is the directory containing the refinery clone.
	DirRefinery = "refinery"

	// DirWitness is the directory containing witness state.
	DirWitness = "witness"

	// DirRig is the subdirectory containing the actual git clone.
	DirRig = "rig"

	// DirBeads is the beads database directory.
	DirBeads = ".beads"

	// DirRuntime is the runtime state directory (gitignored).
	DirRuntime = ".runtime"

	// DirSettings is the rig settings directory (git-tracked).
	DirSettings = "settings"
)

// File names for configuration and state.
const (
	// FileRigsJSON is the rig registry file in mayor/.
	FileRigsJSON = "rigs.json"

	// FileTownJSON is the town configuration file in mayor/.
	FileTownJSON = "town.json"

	// FileConfigJSON is the general config file.
	FileConfigJSON = "config.json"

	// FileAccountsJSON is the accounts configuration file in mayor/.
	FileAccountsJSON = "accounts.json"

	// FileHandoffMarker is the marker file indicating a handoff just occurred.
	// Written by gt handoff before respawn, cleared by gt prime after detection.
	// This prevents the handoff loop bug where agents re-run /handoff from context.
	FileHandoffMarker = "handoff_to_successor"
)

// Beads configuration constants.
const (
	// BeadsCustomTypes is the comma-separated list of custom issue types that
	// Gas Town registers with beads. These types were extracted from beads core
	// in v0.46.0 and now require explicit configuration.
	//
	// Type origins:
	//   agent         - Agent identity beads (gt install, rig init)
	//   role          - Agent role definitions (gt doctor role checks)
	//   rig           - Rig identity beads (gt rig init)
	//   convoy        - Cross-project work tracking
	//   slot          - Exclusive access / merge slots
	//   queue         - Message queue routing (gt mail queue)
	//   event         - Session/cost events (gt costs record)
	//   message       - Mail system (gt mail send, mailbox, router)
	//   molecule      - Work decomposition (patrol checks, gt swarm)
	//   gate          - Async coordination (bd gate wait, park/resume)
	//   merge-request - Refinery MR processing (gt done, refinery)
	BeadsCustomTypes = "agent,role,rig,convoy,slot,queue,event,message,molecule,gate,merge-request"
)

// BeadsCustomTypesList returns the custom types as a slice.
func BeadsCustomTypesList() []string {
	return []string{"agent", "role", "rig", "convoy", "slot", "queue", "event", "message", "molecule", "gate", "merge-request"}
}

// Git branch names.
const (
	// BranchMain is the default main branch name.
	BranchMain = "main"

	// BranchBeadsSync is the branch used for beads synchronization.
	BranchBeadsSync = "beads-sync"

	// BranchPolecatPrefix is the prefix for polecat work branches.
	BranchPolecatPrefix = "polecat/"

	// BranchIntegrationPrefix is the prefix for integration branches.
	BranchIntegrationPrefix = "integration/"
)

// Tmux session names.
// Mayor and Deacon use hq- prefix: hq-mayor, hq-deacon (town-level, one per machine).
// Rig-level services use gt- prefix: gt-<rig>-witness, gt-<rig>-refinery, etc.
// Use session.MayorSessionName() and session.DeaconSessionName().
const (
	// SessionPrefix is the prefix for rig-level Gas Town tmux sessions.
	SessionPrefix = "gt-"

	// HQSessionPrefix is the prefix for town-level services (Mayor, Deacon).
	HQSessionPrefix = "hq-"
)

// Agent role names.
const (
	// RoleMayor is the mayor agent role.
	RoleMayor = "mayor"

	// RoleWitness is the witness agent role.
	RoleWitness = "witness"

	// RoleRefinery is the refinery agent role.
	RoleRefinery = "refinery"

	// RolePolecat is the polecat agent role.
	RolePolecat = "polecat"

	// RoleCrew is the crew agent role.
	RoleCrew = "crew"

	// RoleDeacon is the deacon agent role.
	RoleDeacon = "deacon"

	// RoleBoot is the boot/startup role for initialization.
	RoleBoot = "boot"
)

// Role emojis - centralized for easy customization.
// These match the Gas Town visual identity (see ~/Desktop/Gas Town/ prompts).
const (
	// EmojiMayor is the mayor emoji (fox conductor).
	EmojiMayor = "🎩"

	// EmojiDeacon is the deacon emoji (wolf in the engine room).
	EmojiDeacon = "🐺"

	// EmojiWitness is the witness emoji (watchful owl).
	EmojiWitness = "🦉"

	// EmojiRefinery is the refinery emoji (industrial).
	EmojiRefinery = "🏭"

	// EmojiCrew is the crew emoji (established worker).
	EmojiCrew = "👷"

	// EmojiPolecat is the polecat emoji (transient worker).
	EmojiPolecat = "😺"

	// EmojiBoot is the boot/startup emoji.
	EmojiBoot = "🚀"
)

// RoleEmoji returns the emoji for a given role name.
func RoleEmoji(role string) string {
	switch role {
	case RoleMayor:
		return EmojiMayor
	case RoleDeacon:
		return EmojiDeacon
	case RoleWitness:
		return EmojiWitness
	case RoleRefinery:
		return EmojiRefinery
	case RoleCrew:
		return EmojiCrew
	case RolePolecat:
		return EmojiPolecat
	case RoleBoot:
		return EmojiBoot
	default:
		return "❓"
	}
}

// SupportedShells lists shell binaries that Gas Town can detect and work with.
// Used to identify if a tmux pane is at a shell prompt vs running a command.
var SupportedShells = []string{"bash", "zsh", "sh", "fish", "tcsh", "ksh"}

// Path helpers construct common paths.

// MayorRigsPath returns the path to rigs.json within a town root.
func MayorRigsPath(townRoot string) string {
	return townRoot + "/" + DirMayor + "/" + FileRigsJSON
}

// MayorTownPath returns the path to town.json within a town root.
func MayorTownPath(townRoot string) string {
	return townRoot + "/" + DirMayor + "/" + FileTownJSON
}

// RigMayorPath returns the path to mayor/rig within a rig.
func RigMayorPath(rigPath string) string {
	return rigPath + "/" + DirMayor + "/" + DirRig
}

// RigBeadsPath returns the path to mayor/rig/.beads within a rig.
func RigBeadsPath(rigPath string) string {
	return rigPath + "/" + DirMayor + "/" + DirRig + "/" + DirBeads
}

// RigPolecatsPath returns the path to polecats/ within a rig.
func RigPolecatsPath(rigPath string) string {
	return rigPath + "/" + DirPolecats
}

// RigCrewPath returns the path to crew/ within a rig.
func RigCrewPath(rigPath string) string {
	return rigPath + "/" + DirCrew
}

// MayorConfigPath returns the path to mayor/config.json within a town root.
func MayorConfigPath(townRoot string) string {
	return townRoot + "/" + DirMayor + "/" + FileConfigJSON
}

// TownRuntimePath returns the path to .runtime/ at the town root.
func TownRuntimePath(townRoot string) string {
	return townRoot + "/" + DirRuntime
}

// RigRuntimePath returns the path to .runtime/ within a rig.
func RigRuntimePath(rigPath string) string {
	return rigPath + "/" + DirRuntime
}

// RigSettingsPath returns the path to settings/ within a rig.
func RigSettingsPath(rigPath string) string {
	return rigPath + "/" + DirSettings
}

// MayorAccountsPath returns the path to mayor/accounts.json within a town root.
func MayorAccountsPath(townRoot string) string {
	return townRoot + "/" + DirMayor + "/" + FileAccountsJSON
}
