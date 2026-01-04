package beads

import "testing"

// TestMayorBeadIDTown tests the town-level Mayor bead ID.
func TestMayorBeadIDTown(t *testing.T) {
	got := MayorBeadIDTown()
	want := "hq-mayor"
	if got != want {
		t.Errorf("MayorBeadIDTown() = %q, want %q", got, want)
	}
}

// TestDeaconBeadIDTown tests the town-level Deacon bead ID.
func TestDeaconBeadIDTown(t *testing.T) {
	got := DeaconBeadIDTown()
	want := "hq-deacon"
	if got != want {
		t.Errorf("DeaconBeadIDTown() = %q, want %q", got, want)
	}
}

// TestDogBeadIDTown tests town-level Dog bead IDs.
func TestDogBeadIDTown(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"alpha", "hq-dog-alpha"},
		{"rex", "hq-dog-rex"},
		{"spot", "hq-dog-spot"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DogBeadIDTown(tt.name)
			if got != tt.want {
				t.Errorf("DogBeadIDTown(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestRoleBeadIDTown tests town-level role bead IDs.
func TestRoleBeadIDTown(t *testing.T) {
	tests := []struct {
		roleType string
		want     string
	}{
		{"mayor", "hq-mayor-role"},
		{"deacon", "hq-deacon-role"},
		{"dog", "hq-dog-role"},
		{"witness", "hq-witness-role"},
	}

	for _, tt := range tests {
		t.Run(tt.roleType, func(t *testing.T) {
			got := RoleBeadIDTown(tt.roleType)
			if got != tt.want {
				t.Errorf("RoleBeadIDTown(%q) = %q, want %q", tt.roleType, got, tt.want)
			}
		})
	}
}

// TestMayorRoleBeadIDTown tests the Mayor role bead ID for town-level.
func TestMayorRoleBeadIDTown(t *testing.T) {
	got := MayorRoleBeadIDTown()
	want := "hq-mayor-role"
	if got != want {
		t.Errorf("MayorRoleBeadIDTown() = %q, want %q", got, want)
	}
}

// TestDeaconRoleBeadIDTown tests the Deacon role bead ID for town-level.
func TestDeaconRoleBeadIDTown(t *testing.T) {
	got := DeaconRoleBeadIDTown()
	want := "hq-deacon-role"
	if got != want {
		t.Errorf("DeaconRoleBeadIDTown() = %q, want %q", got, want)
	}
}

// TestDogRoleBeadIDTown tests the Dog role bead ID for town-level.
func TestDogRoleBeadIDTown(t *testing.T) {
	got := DogRoleBeadIDTown()
	want := "hq-dog-role"
	if got != want {
		t.Errorf("DogRoleBeadIDTown() = %q, want %q", got, want)
	}
}
