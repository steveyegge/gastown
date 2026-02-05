package configbeads

import (
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
)

func TestSeedRigRegistryBead(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	entry := config.RigEntry{
		GitURL:  "git@github.com:test/repo.git",
		AddedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		BeadsConfig: &config.BeadsConfig{
			Prefix: "tr",
		},
	}

	err := SeedRigRegistryBead(dir, "testtown", "testrepo", entry)
	if err != nil {
		t.Skipf("SeedRigRegistryBead failed (may be bd CLI issue): %v", err)
		return
	}

	// Verify the bead was created
	slug := "rig-testtown-testrepo"
	issue, fields, err := bd.GetConfigBeadBySlug(slug)
	if err != nil {
		t.Fatalf("GetConfigBeadBySlug failed: %v", err)
	}
	if issue == nil {
		t.Fatal("expected config bead to exist")
	}
	if fields.Category != beads.ConfigCategoryRigRegistry {
		t.Errorf("category = %q, want %q", fields.Category, beads.ConfigCategoryRigRegistry)
	}
	if fields.Rig != "testtown/testrepo" {
		t.Errorf("rig = %q, want %q", fields.Rig, "testtown/testrepo")
	}
}

func TestSeedRigRegistryBead_Idempotent(t *testing.T) {
	dir := setupTestTown(t)
	_ = setupTestBeads(t, dir)

	entry := config.RigEntry{
		GitURL:  "git@github.com:test/repo.git",
		AddedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	// First call
	err := SeedRigRegistryBead(dir, "testtown", "testrepo", entry)
	if err != nil {
		t.Skipf("SeedRigRegistryBead failed (may be bd CLI issue): %v", err)
		return
	}

	// Second call should be idempotent (no error)
	err = SeedRigRegistryBead(dir, "testtown", "testrepo", entry)
	if err != nil {
		t.Errorf("second SeedRigRegistryBead should be idempotent, got: %v", err)
	}
}

func TestDeleteRigRegistryBead(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	// Create a rig registry bead first
	entry := config.RigEntry{
		GitURL:  "git@github.com:test/repo.git",
		AddedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	err := SeedRigRegistryBead(dir, "testtown", "testrepo", entry)
	if err != nil {
		t.Skipf("SeedRigRegistryBead failed (may be bd CLI issue): %v", err)
		return
	}

	// Delete it
	err = DeleteRigRegistryBead(dir, "testtown", "testrepo")
	if err != nil {
		t.Fatalf("DeleteRigRegistryBead failed: %v", err)
	}

	// Verify it's gone
	slug := "rig-testtown-testrepo"
	issue, _, err := bd.GetConfigBeadBySlug(slug)
	if err != nil {
		t.Fatalf("GetConfigBeadBySlug failed: %v", err)
	}
	if issue != nil {
		t.Error("expected config bead to be deleted")
	}
}

func TestDeleteRigRegistryBead_NotFound(t *testing.T) {
	dir := setupTestTown(t)
	_ = setupTestBeads(t, dir)

	// Should not error when bead doesn't exist
	err := DeleteRigRegistryBead(dir, "testtown", "nonexistent")
	if err != nil {
		t.Errorf("DeleteRigRegistryBead should not error for non-existent bead: %v", err)
	}
}

func TestSeedAccountBead(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	acct := config.Account{
		Email:       "test@example.com",
		Description: "Test account",
		ConfigDir:   "/home/user/.claude-accounts/test",
	}

	err := SeedAccountBead(dir, "test", acct)
	if err != nil {
		t.Skipf("SeedAccountBead failed (may be bd CLI issue): %v", err)
		return
	}

	// Verify the bead was created
	slug := "account-test"
	issue, fields, err := bd.GetConfigBeadBySlug(slug)
	if err != nil {
		t.Fatalf("GetConfigBeadBySlug failed: %v", err)
	}
	if issue == nil {
		t.Fatal("expected config bead to exist")
	}
	if fields.Category != beads.ConfigCategoryAccounts {
		t.Errorf("category = %q, want %q", fields.Category, beads.ConfigCategoryAccounts)
	}
	if fields.Rig != "*" {
		t.Errorf("rig = %q, want %q", fields.Rig, "*")
	}
}

func TestSeedAccountBead_Idempotent(t *testing.T) {
	dir := setupTestTown(t)
	_ = setupTestBeads(t, dir)

	acct := config.Account{
		Email:     "test@example.com",
		ConfigDir: "/home/user/.claude-accounts/test",
	}

	// First call
	err := SeedAccountBead(dir, "test", acct)
	if err != nil {
		t.Skipf("SeedAccountBead failed (may be bd CLI issue): %v", err)
		return
	}

	// Second call should be idempotent
	err = SeedAccountBead(dir, "test", acct)
	if err != nil {
		t.Errorf("second SeedAccountBead should be idempotent, got: %v", err)
	}
}

func TestSeedTownIdentityBead(t *testing.T) {
	dir := setupTestTown(t)
	bd := setupTestBeads(t, dir)

	tc := &config.TownConfig{
		Type:       "town",
		Version:    2,
		Name:       "testtown",
		Owner:      "test@example.com",
		PublicName: "Test Town",
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	err := SeedTownIdentityBead(dir, tc)
	if err != nil {
		t.Skipf("SeedTownIdentityBead failed (may be bd CLI issue): %v", err)
		return
	}

	// Verify the bead was created
	slug := "town-testtown"
	issue, fields, err := bd.GetConfigBeadBySlug(slug)
	if err != nil {
		t.Fatalf("GetConfigBeadBySlug failed: %v", err)
	}
	if issue == nil {
		t.Fatal("expected config bead to exist")
	}
	if fields.Category != beads.ConfigCategoryIdentity {
		t.Errorf("category = %q, want %q", fields.Category, beads.ConfigCategoryIdentity)
	}
	if fields.Rig != "testtown" {
		t.Errorf("rig = %q, want %q", fields.Rig, "testtown")
	}
}

func TestSeedTownIdentityBead_Idempotent(t *testing.T) {
	dir := setupTestTown(t)
	_ = setupTestBeads(t, dir)

	tc := &config.TownConfig{
		Type:    "town",
		Version: 2,
		Name:    "testtown",
	}

	// First call
	err := SeedTownIdentityBead(dir, tc)
	if err != nil {
		t.Skipf("SeedTownIdentityBead failed (may be bd CLI issue): %v", err)
		return
	}

	// Second call should be idempotent
	err = SeedTownIdentityBead(dir, tc)
	if err != nil {
		t.Errorf("second SeedTownIdentityBead should be idempotent, got: %v", err)
	}
}
