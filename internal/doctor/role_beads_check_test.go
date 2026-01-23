package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoleConfigCheck_Run(t *testing.T) {
	t.Run("no overrides returns OK with defaults message", func(t *testing.T) {
		tmpDir := t.TempDir()

		check := NewRoleBeadsCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
		}
		if result.Message != "Role config uses built-in defaults" {
			t.Errorf("unexpected message: %s", result.Message)
		}
	})

	t.Run("valid town override returns OK", func(t *testing.T) {
		tmpDir := t.TempDir()
		rolesDir := filepath.Join(tmpDir, "roles")
		if err := os.MkdirAll(rolesDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create a valid TOML override
		override := `
role = "witness"
scope = "rig"

[session]
start_command = "exec echo test"
`
		if err := os.WriteFile(filepath.Join(rolesDir, "witness.toml"), []byte(override), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRoleBeadsCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
		}
		if result.Message != "Role config valid (1 override file(s))" {
			t.Errorf("unexpected message: %s", result.Message)
		}
	})

	t.Run("invalid town override returns warning", func(t *testing.T) {
		tmpDir := t.TempDir()
		rolesDir := filepath.Join(tmpDir, "roles")
		if err := os.MkdirAll(rolesDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create an invalid TOML file
		if err := os.WriteFile(filepath.Join(rolesDir, "witness.toml"), []byte("invalid { toml"), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRoleBeadsCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusWarning {
			t.Errorf("expected StatusWarning, got %v: %s", result.Status, result.Message)
		}
		if len(result.Details) != 1 {
			t.Errorf("expected 1 warning detail, got %d", len(result.Details))
		}
	})

	t.Run("valid rig override returns OK", func(t *testing.T) {
		tmpDir := t.TempDir()
		rigName := "testrig"
		rigDir := filepath.Join(tmpDir, rigName)
		rigRolesDir := filepath.Join(rigDir, "roles")
		if err := os.MkdirAll(rigRolesDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create rig.json to mark this as a rig
		if err := os.WriteFile(filepath.Join(rigDir, "rig.json"), []byte(`{"name": "testrig"}`), 0644); err != nil {
			t.Fatal(err)
		}

		// Create a valid TOML override
		override := `
role = "refinery"
scope = "rig"

[session]
needs_pre_sync = true
`
		if err := os.WriteFile(filepath.Join(rigRolesDir, "refinery.toml"), []byte(override), 0644); err != nil {
			t.Fatal(err)
		}

		check := NewRoleBeadsCheck()
		ctx := &CheckContext{TownRoot: tmpDir}
		result := check.Run(ctx)

		if result.Status != StatusOK {
			t.Errorf("expected StatusOK, got %v: %s", result.Status, result.Message)
		}
	})

	t.Run("check is not fixable", func(t *testing.T) {
		check := NewRoleBeadsCheck()
		if check.CanFix() {
			t.Error("RoleConfigCheck should not be fixable (config issues need manual fix)")
		}
	})
}
