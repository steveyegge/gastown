package doctor

import (
	"strings"
	"testing"
)

func TestDiskSpaceCheck_Run(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := &CheckContext{TownRoot: tmpDir}

	check := NewDiskSpaceCheck()
	result := check.Run(ctx)

	if result.Name != "disk-space" {
		t.Errorf("Name = %q, want %q", result.Name, "disk-space")
	}

	// In a normal test environment, disk space should be OK or Warning (not Error)
	if result.Status == StatusError {
		t.Logf("Disk space is critical in test environment: %s", result.Message)
	}

	// Message should always be non-empty
	if result.Message == "" {
		t.Error("Message should not be empty")
	}
}

func TestDiskSpaceCheck_InvalidPath(t *testing.T) {
	ctx := &CheckContext{TownRoot: "/nonexistent/path/that/should/not/exist"}

	check := NewDiskSpaceCheck()
	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want Warning for invalid path", result.Status)
	}

	if !strings.Contains(result.Message, "Could not check") {
		t.Errorf("Message = %q, want to contain 'Could not check'", result.Message)
	}
}

func TestDiskSpaceCheck_Properties(t *testing.T) {
	check := NewDiskSpaceCheck()

	if check.Name() != "disk-space" {
		t.Errorf("Name() = %q, want %q", check.Name(), "disk-space")
	}

	if check.Description() == "" {
		t.Error("Description() should not be empty")
	}

	if check.CanFix() {
		t.Error("CanFix() should be false — disk space can't be auto-fixed")
	}

	if check.Category() != CategoryInfrastructure {
		t.Errorf("Category() = %q, want %q", check.Category(), CategoryInfrastructure)
	}
}
