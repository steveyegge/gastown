package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func TestMigrationResultStatus(t *testing.T) {
	tests := []struct {
		name     string
		result   migrationResult
		wantIcon string
	}{
		{
			name: "migrated shows checkmark",
			result: migrationResult{
				OldID:   "gt-mayor",
				NewID:   "hq-mayor",
				Status:  "migrated",
				Message: "successfully migrated",
			},
			wantIcon: "  ✓",
		},
		{
			name: "would migrate shows checkmark",
			result: migrationResult{
				OldID:   "gt-mayor",
				NewID:   "hq-mayor",
				Status:  "would migrate",
				Message: "would copy state from gt-mayor",
			},
			wantIcon: "  ✓",
		},
		{
			name: "skipped shows empty circle",
			result: migrationResult{
				OldID:   "gt-mayor",
				NewID:   "hq-mayor",
				Status:  "skipped",
				Message: "already exists",
			},
			wantIcon: "  ⊘",
		},
		{
			name: "error shows X",
			result: migrationResult{
				OldID:   "gt-mayor",
				NewID:   "hq-mayor",
				Status:  "error",
				Message: "failed to create",
			},
			wantIcon: "  ✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var icon string
			switch tt.result.Status {
			case "migrated", "would migrate":
				icon = "  ✓"
			case "skipped":
				icon = "  ⊘"
			case "error":
				icon = "  ✗"
			}
			if icon != tt.wantIcon {
				t.Errorf("icon for status %q = %q, want %q", tt.result.Status, icon, tt.wantIcon)
			}
		})
	}
}

func TestTownBeadIDHelpers(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"MayorBeadIDTown", beads.MayorBeadIDTown(), "hq-mayor"},
		{"DeaconBeadIDTown", beads.DeaconBeadIDTown(), "hq-deacon"},
		{"DogBeadIDTown", beads.DogBeadIDTown("fido"), "hq-dog-fido"},
		{"RoleBeadIDTown mayor", beads.RoleBeadIDTown("mayor"), "hq-mayor-role"},
		{"RoleBeadIDTown witness", beads.RoleBeadIDTown("witness"), "hq-witness-role"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}
