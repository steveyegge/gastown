// Package configbeads provides functions for loading configuration from beads.
// This file handles rig registry config bead operations.
package configbeads

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

// SeedRigRegistryBead creates a rig registry config bead when a rig is added.
// The bead ID is hq-cfg-rig-<town>-<rigName>.
// This stores the same data as the rigs.json entry, making beads the source of truth.
func SeedRigRegistryBead(townRoot string, townName, rigName string, entry config.RigEntry) error {
	bd := beads.New(townRoot)

	slug := "rig-" + townName + "-" + rigName
	rigScope := townName + "/" + rigName

	// Check if already exists
	existing, _, err := bd.GetConfigBeadBySlug(slug)
	if err == nil && existing != nil {
		return nil // Already seeded
	}

	// Build metadata matching the rigs.json entry format
	metadata := map[string]interface{}{
		"git_url":  entry.GitURL,
		"added_at": entry.AddedAt,
	}
	if entry.LocalRepo != "" {
		metadata["local_repo"] = entry.LocalRepo
	}
	if entry.BeadsConfig != nil {
		metadata["beads"] = map[string]interface{}{
			"prefix": entry.BeadsConfig.Prefix,
		}
		if entry.BeadsConfig.Repo != "" {
			beadsMap := metadata["beads"].(map[string]interface{})
			beadsMap["repo"] = entry.BeadsConfig.Repo
		}
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshaling rig registry metadata: %w", err)
	}

	fields := &beads.ConfigFields{
		Rig:      rigScope,
		Category: beads.ConfigCategoryRigRegistry,
		Metadata: string(metadataJSON),
	}

	_, err = bd.CreateConfigBead(slug, fields, "", "")
	return err
}

// DeleteRigRegistryBead removes the rig registry config bead when a rig is removed.
// Deletes bead with ID hq-cfg-rig-<town>-<rigName>.
func DeleteRigRegistryBead(townRoot, townName, rigName string) error {
	bd := beads.New(townRoot)

	slug := "rig-" + townName + "-" + rigName
	id := beads.ConfigBeadID(slug)

	// Check if it exists before deleting
	existing, _, err := bd.GetConfigBead(id)
	if err != nil || existing == nil {
		return nil // Nothing to delete
	}

	return bd.DeleteConfigBead(id)
}

// SeedAccountBead creates a config bead for an account (excluding auth_token).
// The bead ID is hq-cfg-account-<handle>.
// Secrets (auth_token) are intentionally excluded from beads.
func SeedAccountBead(townRoot string, handle string, acct config.Account) error {
	bd := beads.New(townRoot)

	slug := "account-" + handle

	// Check if already exists
	existing, _, err := bd.GetConfigBeadBySlug(slug)
	if err == nil && existing != nil {
		return nil // Already seeded
	}

	metadata := map[string]interface{}{
		"handle":     handle,
		"config_dir": acct.ConfigDir,
		"created_at": time.Now().Format(time.RFC3339),
	}
	if acct.Email != "" {
		metadata["email"] = acct.Email
	}
	if acct.Description != "" {
		metadata["description"] = acct.Description
	}
	if acct.BaseURL != "" {
		metadata["base_url"] = acct.BaseURL
	}
	// NOTE: auth_token is intentionally excluded - secrets never go in beads

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshaling account metadata: %w", err)
	}

	fields := &beads.ConfigFields{
		Rig:      "*", // Global scope - accounts are cross-rig
		Category: beads.ConfigCategoryAccounts,
		Metadata: string(metadataJSON),
	}

	_, err = bd.CreateConfigBead(slug, fields, "", "")
	return err
}
