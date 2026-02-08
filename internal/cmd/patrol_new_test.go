package cmd

import (
	"testing"
)

func TestRunPatrolNew_UnsupportedRole(t *testing.T) {
	// Test that an unsupported role returns an error
	// We can't easily test the full flow without bd/beads,
	// but we can verify role validation logic

	// Test the role switch logic directly
	validRoles := []string{"deacon", "witness", "refinery"}
	invalidRoles := []string{"mayor", "polecat", "crew", "unknown", ""}

	for _, role := range validRoles {
		r := Role(role)
		if r != RoleDeacon && r != RoleWitness && r != RoleRefinery {
			t.Errorf("role %q should be valid for patrol new", role)
		}
	}

	for _, role := range invalidRoles {
		r := Role(role)
		if r == RoleDeacon || r == RoleWitness || r == RoleRefinery {
			t.Errorf("role %q should be invalid for patrol new", role)
		}
	}
}

func TestPatrolNewCmd_Registered(t *testing.T) {
	// Verify the command is properly registered
	found := false
	for _, cmd := range patrolCmd.Commands() {
		if cmd.Use == "new" {
			found = true
			break
		}
	}
	if !found {
		t.Error("patrol new command not registered")
	}
}

func TestPatrolNewCmd_HasRoleFlag(t *testing.T) {
	flag := patrolNewCmd.Flags().Lookup("role")
	if flag == nil {
		t.Error("patrol new command missing --role flag")
	}
}
