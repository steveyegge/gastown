// Package plugin provides plugin discovery and management for Gas Town.
//
// Plugins are periodic automation tasks that run during Deacon patrol cycles.
// Each plugin is defined by a plugin.md file with TOML frontmatter.
//
// Plugin locations:
//   - Town-level: ~/gt/plugins/ (universal, apply everywhere)
//   - Rig-level: <rig>/plugins/ (project-specific)
package plugin

import (
	"time"
)

// Plugin represents a discovered plugin definition.
type Plugin struct {
	// Name is the unique plugin identifier (from frontmatter).
	Name string `json:"name"`

	// Description is a human-readable description.
	Description string `json:"description"`

	// Version is the schema version (for future evolution).
	Version int `json:"version"`

	// Location indicates where the plugin was discovered.
	Location Location `json:"location"`

	// Path is the absolute path to the plugin directory.
	Path string `json:"path"`

	// RigName is set for rig-level plugins (empty for town-level).
	RigName string `json:"rig_name,omitempty"`

	// Gate defines when the plugin should run.
	Gate *Gate `json:"gate,omitempty"`

	// Tracking defines labels and digest settings.
	Tracking *Tracking `json:"tracking,omitempty"`

	// Execution defines timeout and notification settings.
	Execution *Execution `json:"execution,omitempty"`

	// Instructions is the markdown body (after frontmatter).
	Instructions string `json:"instructions,omitempty"`
}

// Location indicates where a plugin was discovered.
type Location string

const (
	// LocationTown indicates a town-level plugin (~/gt/plugins/).
	LocationTown Location = "town"

	// LocationRig indicates a rig-level plugin (<rig>/plugins/).
	LocationRig Location = "rig"
)

// Gate defines when a plugin should run.
type Gate struct {
	// Type is the gate type: cooldown, cron, condition, event, or manual.
	Type GateType `json:"type" toml:"type"`

	// Duration is for cooldown gates (e.g., "1h", "24h").
	Duration string `json:"duration,omitempty" toml:"duration,omitempty"`

	// Schedule is for cron gates (e.g., "0 9 * * *").
	Schedule string `json:"schedule,omitempty" toml:"schedule,omitempty"`

	// Check is for condition gates (command that returns exit 0 to run).
	Check string `json:"check,omitempty" toml:"check,omitempty"`

	// On is for event gates (e.g., "startup").
	On string `json:"on,omitempty" toml:"on,omitempty"`
}

// GateType is the type of gate that controls plugin execution.
type GateType string

const (
	// GateCooldown runs if enough time has passed since last run.
	GateCooldown GateType = "cooldown"

	// GateCron runs on a cron schedule.
	GateCron GateType = "cron"

	// GateCondition runs if a check command returns exit 0.
	GateCondition GateType = "condition"

	// GateEvent runs on specific events (startup, etc).
	GateEvent GateType = "event"

	// GateManual never auto-runs, must be triggered explicitly.
	GateManual GateType = "manual"
)

// Tracking defines how plugin runs are tracked.
type Tracking struct {
	// Labels are applied to execution wisps.
	Labels []string `json:"labels,omitempty" toml:"labels,omitempty"`

	// Digest indicates whether to include in daily digest.
	Digest bool `json:"digest" toml:"digest"`
}

// Execution defines plugin execution settings.
type Execution struct {
	// Timeout is the maximum execution time (e.g., "5m").
	Timeout string `json:"timeout,omitempty" toml:"timeout,omitempty"`

	// NotifyOnFailure escalates on failure.
	NotifyOnFailure bool `json:"notify_on_failure" toml:"notify_on_failure"`

	// Severity is the escalation severity on failure.
	Severity string `json:"severity,omitempty" toml:"severity,omitempty"`
}

// PluginFrontmatter represents the TOML frontmatter in plugin.md files.
type PluginFrontmatter struct {
	Name        string     `toml:"name"`
	Description string     `toml:"description"`
	Version     int        `toml:"version"`
	Gate        *Gate      `toml:"gate,omitempty"`
	Tracking    *Tracking  `toml:"tracking,omitempty"`
	Execution   *Execution `toml:"execution,omitempty"`
}

// PluginSummary provides a concise overview of a plugin.
type PluginSummary struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Location    Location `json:"location"`
	RigName     string   `json:"rig_name,omitempty"`
	GateType    GateType `json:"gate_type,omitempty"`
	Path        string   `json:"path"`
}

// Summary returns a PluginSummary for this plugin.
func (p *Plugin) Summary() PluginSummary {
	var gateType GateType
	if p.Gate != nil {
		gateType = p.Gate.Type
	} else {
		gateType = GateManual
	}

	return PluginSummary{
		Name:        p.Name,
		Description: p.Description,
		Location:    p.Location,
		RigName:     p.RigName,
		GateType:    gateType,
		Path:        p.Path,
	}
}

// PluginRun represents a single execution of a plugin.
type PluginRun struct {
	PluginName string    `json:"plugin_name"`
	RigName    string    `json:"rig_name,omitempty"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time,omitempty"`
	Result     string    `json:"result"` // "success" or "failure"
	Message    string    `json:"message,omitempty"`
}
