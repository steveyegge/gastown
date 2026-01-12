package doctor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestRoleBeadsCheck_Run(t *testing.T) {
	t.Run("no town beads returns warning", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create minimal town structure without .beads
		if err := os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755); err != nil {
			t.Fatal(err)
		}

		check := NewRoleBeadsCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		// Without .beads directory, all role beads are "missing"
		expectedCount := len(beads.AllRoleBeadDefs())
		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
		}
		if len(result.Details) != expectedCount {
			t.Errorf("expected %d missing role beads, got %d: %v", expectedCount, len(result.Details), result.Details)
		}
	})

	t.Run("check is fixable", func(t *testing.T) {
		check := NewRoleBeadsCheck()
		if !check.CanFix() {
			t.Error("RoleBeadsCheck should be fixable")
		}
	})
}

func TestRoleBeadsCheck_usesSharedDefs(t *testing.T) {
	// Verify the check uses beads.AllRoleBeadDefs()
	roleDefs := beads.AllRoleBeadDefs()

	if len(roleDefs) < 7 {
		t.Errorf("expected at least 7 role beads, got %d", len(roleDefs))
	}

	// Verify key roles are present
	expectedIDs := map[string]bool{
		"hq-mayor-role":    false,
		"hq-deacon-role":   false,
		"hq-witness-role":  false,
		"hq-refinery-role": false,
	}

	for _, role := range roleDefs {
		if _, exists := expectedIDs[role.ID]; exists {
			expectedIDs[role.ID] = true
		}
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("expected role %s not found in AllRoleBeadDefs()", id)
		}
	}
}
