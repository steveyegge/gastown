package cmd

import (
	"fmt"
)

// resolveTargetAgent converts a target spec to agent ID.
func resolveTargetAgent(target string) (agentID string, err error) {
	id, err := resolveRoleToAgentID(target)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// resolveSelfTarget determines agent identity for slinging to self.
func resolveSelfTarget() (agentID string, err error) {
	roleInfo, err := GetRole()
	if err != nil {
		return "", fmt.Errorf("detecting role: %w", err)
	}

	// Build agent identity from role
	// Town-level agents use trailing slash to match addressToIdentity() normalization
	switch roleInfo.Role {
	case RoleMayor:
		agentID = "mayor/"
	case RoleDeacon:
		agentID = "deacon/"
	case RoleWitness:
		agentID = fmt.Sprintf("%s/witness", roleInfo.Rig)
	case RoleRefinery:
		agentID = fmt.Sprintf("%s/refinery", roleInfo.Rig)
	case RolePolecat:
		agentID = fmt.Sprintf("%s/polecats/%s", roleInfo.Rig, roleInfo.Polecat)
	case RoleCrew:
		agentID = fmt.Sprintf("%s/crew/%s", roleInfo.Rig, roleInfo.Polecat)
	default:
		return "", fmt.Errorf("cannot determine agent identity (role: %s)", roleInfo.Role)
	}

	return agentID, nil
}
