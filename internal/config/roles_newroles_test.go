package config

import (
	"testing"
)

// Phase 4: Specification tests for new roles in the config/roles system.
// These tests define the contract for artisan, architect, and conductor
// role definitions. They should FAIL until the implementation is complete.

// TestAllRoles_IncludesNewRoles validates that AllRoles includes the three new roles.
func TestAllRoles_IncludesNewRoles(t *testing.T) {
	roles := AllRoles()

	newRoles := []string{"artisan", "architect", "conductor"}
	for _, want := range newRoles {
		found := false
		for _, r := range roles {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AllRoles() missing %q", want)
		}
	}
}

// TestRigRoles_IncludesNewRoles validates that RigRoles includes the three new roles.
func TestRigRoles_IncludesNewRoles(t *testing.T) {
	roles := RigRoles()

	newRoles := []string{"artisan", "architect", "conductor"}
	for _, want := range newRoles {
		found := false
		for _, r := range roles {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RigRoles() missing %q", want)
		}
	}
}

// TestLoadBuiltinRoleDefinition_Artisan validates the artisan TOML role config.
func TestLoadBuiltinRoleDefinition_Artisan(t *testing.T) {
	def, err := loadBuiltinRoleDefinition("artisan")
	if err != nil {
		t.Fatalf("loadBuiltinRoleDefinition(artisan) error: %v", err)
	}

	if def.Role != "artisan" {
		t.Errorf("Role = %q, want %q", def.Role, "artisan")
	}
	if def.Scope != "rig" {
		t.Errorf("Scope = %q, want %q", def.Scope, "rig")
	}
	if def.Session.NeedsPreSync != true {
		t.Errorf("Session.NeedsPreSync = %v, want true", def.Session.NeedsPreSync)
	}
	if def.Health.PingTimeout.Duration == 0 {
		t.Error("Health.PingTimeout should not be zero")
	}
	if def.Health.ConsecutiveFailures == 0 {
		t.Error("Health.ConsecutiveFailures should not be zero")
	}
	if def.PromptTemplate == "" {
		t.Error("PromptTemplate should not be empty")
	}
}

// TestLoadBuiltinRoleDefinition_Architect validates the architect TOML role config.
func TestLoadBuiltinRoleDefinition_Architect(t *testing.T) {
	def, err := loadBuiltinRoleDefinition("architect")
	if err != nil {
		t.Fatalf("loadBuiltinRoleDefinition(architect) error: %v", err)
	}

	if def.Role != "architect" {
		t.Errorf("Role = %q, want %q", def.Role, "architect")
	}
	if def.Scope != "rig" {
		t.Errorf("Scope = %q, want %q", def.Scope, "rig")
	}
	if def.Health.PingTimeout.Duration == 0 {
		t.Error("Health.PingTimeout should not be zero")
	}
	if def.Health.ConsecutiveFailures == 0 {
		t.Error("Health.ConsecutiveFailures should not be zero")
	}
	if def.PromptTemplate == "" {
		t.Error("PromptTemplate should not be empty")
	}
}

// TestLoadBuiltinRoleDefinition_Conductor validates the conductor TOML role config.
func TestLoadBuiltinRoleDefinition_Conductor(t *testing.T) {
	def, err := loadBuiltinRoleDefinition("conductor")
	if err != nil {
		t.Fatalf("loadBuiltinRoleDefinition(conductor) error: %v", err)
	}

	if def.Role != "conductor" {
		t.Errorf("Role = %q, want %q", def.Role, "conductor")
	}
	if def.Scope != "rig" {
		t.Errorf("Scope = %q, want %q", def.Scope, "rig")
	}
	if def.Health.PingTimeout.Duration == 0 {
		t.Error("Health.PingTimeout should not be zero")
	}
	if def.Health.ConsecutiveFailures == 0 {
		t.Error("Health.ConsecutiveFailures should not be zero")
	}
	if def.PromptTemplate == "" {
		t.Error("PromptTemplate should not be empty")
	}
}

// TestLoadRoleDefinition_NewRolesValid validates that LoadRoleDefinition accepts new roles.
func TestLoadRoleDefinition_NewRolesValid(t *testing.T) {
	townRoot := t.TempDir()

	for _, role := range []string{"artisan", "architect", "conductor"} {
		t.Run(role, func(t *testing.T) {
			def, err := LoadRoleDefinition(townRoot, "", role)
			if err != nil {
				t.Fatalf("LoadRoleDefinition(%s) error: %v", role, err)
			}
			if def.Role != role {
				t.Errorf("Role = %q, want %q", def.Role, role)
			}
		})
	}
}
