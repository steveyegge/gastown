//go:build integration

package daemon

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

// TestGetRoleConfigForIdentity_UsesBuiltinDefaults tests that the daemon
// uses built-in role definitions from embedded TOML files when no overrides exist.
func TestGetRoleConfigForIdentity_UsesBuiltinDefaults(t *testing.T) {
	townRoot := t.TempDir()

	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(io.Discard, "", 0),
	}

	// Should load witness role from built-in defaults
	cfg, parsed, err := d.getRoleConfigForIdentity("myrig-witness")
	if err != nil {
		t.Fatalf("getRoleConfigForIdentity: %v", err)
	}
	if parsed == nil || parsed.RoleType != "witness" {
		t.Fatalf("parsed = %#v, want roleType witness", parsed)
	}
	if cfg == nil {
		t.Fatal("cfg is nil, expected built-in defaults")
	}
	// Built-in witness has session pattern "{prefix}-witness"
	if cfg.SessionPattern != "{prefix}-witness" {
		t.Errorf("cfg.SessionPattern = %q, want %q", cfg.SessionPattern, "{prefix}-witness")
	}
}

// TestGetRoleConfigForIdentity_TownOverride tests that town-level TOML overrides
// are merged with built-in defaults.
func TestGetRoleConfigForIdentity_TownOverride(t *testing.T) {
	townRoot := t.TempDir()

	// Create town-level override
	rolesDir := filepath.Join(townRoot, "roles")
	if err := os.MkdirAll(rolesDir, 0755); err != nil {
		t.Fatalf("mkdir roles: %v", err)
	}

	// Override start_command for witness role
	witnessOverride := `
role = "witness"
scope = "rig"

[session]
start_command = "exec echo custom-town-command"
`
	if err := os.WriteFile(filepath.Join(rolesDir, "witness.toml"), []byte(witnessOverride), 0644); err != nil {
		t.Fatalf("write witness.toml: %v", err)
	}

	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(io.Discard, "", 0),
	}

	cfg, parsed, err := d.getRoleConfigForIdentity("myrig-witness")
	if err != nil {
		t.Fatalf("getRoleConfigForIdentity: %v", err)
	}
	if parsed == nil || parsed.RoleType != "witness" {
		t.Fatalf("parsed = %#v, want roleType witness", parsed)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
	// Should have the overridden start_command
	if cfg.StartCommand != "exec echo custom-town-command" {
		t.Errorf("cfg.StartCommand = %q, want %q", cfg.StartCommand, "exec echo custom-town-command")
	}
	// Should still have built-in session pattern (not overridden)
	if cfg.SessionPattern != "{prefix}-witness" {
		t.Errorf("cfg.SessionPattern = %q, want %q", cfg.SessionPattern, "{prefix}-witness")
	}
}

// TestGetRoleConfigForIdentity_RigOverride tests that rig-level TOML overrides
// take precedence over town-level overrides.
func TestGetRoleConfigForIdentity_RigOverride(t *testing.T) {
	townRoot := t.TempDir()
	rigPath := filepath.Join(townRoot, "myrig")

	// Create town-level override
	townRolesDir := filepath.Join(townRoot, "roles")
	if err := os.MkdirAll(townRolesDir, 0755); err != nil {
		t.Fatalf("mkdir town roles: %v", err)
	}
	townOverride := `
role = "witness"
scope = "rig"

[session]
start_command = "exec echo town-command"
`
	if err := os.WriteFile(filepath.Join(townRolesDir, "witness.toml"), []byte(townOverride), 0644); err != nil {
		t.Fatalf("write town witness.toml: %v", err)
	}

	// Create rig-level override (should take precedence)
	rigRolesDir := filepath.Join(rigPath, "roles")
	if err := os.MkdirAll(rigRolesDir, 0755); err != nil {
		t.Fatalf("mkdir rig roles: %v", err)
	}
	rigOverride := `
role = "witness"
scope = "rig"

[session]
start_command = "exec echo rig-command"
`
	if err := os.WriteFile(filepath.Join(rigRolesDir, "witness.toml"), []byte(rigOverride), 0644); err != nil {
		t.Fatalf("write rig witness.toml: %v", err)
	}

	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(io.Discard, "", 0),
	}

	cfg, parsed, err := d.getRoleConfigForIdentity("myrig-witness")
	if err != nil {
		t.Fatalf("getRoleConfigForIdentity: %v", err)
	}
	if parsed == nil || parsed.RoleType != "witness" {
		t.Fatalf("parsed = %#v, want roleType witness", parsed)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
	}
	// Should have the rig-level override (takes precedence over town)
	if cfg.StartCommand != "exec echo rig-command" {
		t.Errorf("cfg.StartCommand = %q, want %q", cfg.StartCommand, "exec echo rig-command")
	}
}
