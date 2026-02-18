// Package beads provides role bead management.
//
// DEPRECATED: Role beads are deprecated. Role definitions are now config-based.
// See internal/config/roles/*.toml and config-based-roles.md for the new system.
//
// This file is kept for backward compatibility with existing role beads but
// new code should use config.LoadRoleDefinition() instead of reading role beads.
// The daemon no longer uses role beads as of Phase 2 (config-based roles).
package beads

import (
	"errors"
	"fmt"
)

// DEPRECATED: Role bead ID naming convention is no longer used.
// Role definitions are now config-based (internal/config/roles/*.toml).
//
// Role beads were stored in town beads (~/.beads/) with hq- prefix.
//
// Canonical format was: hq-<role>-role
//
// Examples:
//   - hq-mayor-role
//   - hq-deacon-role
//   - hq-witness-role
//   - hq-refinery-role
//   - hq-crew-role
//   - hq-polecat-role
//
// Use RoleBeadIDTown() from agent_ids.go for role bead IDs (hq- prefix).

// GetRoleConfig looks up a role bead and returns its parsed RoleConfig.
// Returns nil, nil if the role bead doesn't exist or has no config.
//
// Deprecated: Use config.LoadRoleDefinition() instead. Role definitions
// are now config-based, not stored as beads.
func (b *Beads) GetRoleConfig(roleBeadID string) (*RoleConfig, error) {
	issue, err := b.Show(roleBeadID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if !HasLabel(issue, "gt:role") {
		return nil, fmt.Errorf("bead %s is not a role bead (missing gt:role label)", roleBeadID)
	}

	return ParseRoleConfig(issue.Description), nil
}

