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

// TestBootBeadIDTown tests the town-level Boot bead ID.
func TestBootBeadIDTown(t *testing.T) {
	got := BootBeadIDTown()
	want := "hq-boot"
	if got != want {
		t.Errorf("BootBeadIDTown() = %q, want %q", got, want)
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

// TestPolecatBeadIDWithPrefixDeduplication tests that prefix==rig doesn't duplicate.
// Fixes issue gt-u6qwz: fhc-fhc-polecat-stoat should be fhc-polecat-stoat
func TestPolecatBeadIDWithPrefixDeduplication(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		pname  string
		want   string
	}{
		// prefix != rig: include rig in ID
		{"prefix_ne_rig", "gt", "gastown", "toast", "gt-gastown-polecat-toast"},
		// prefix == rig: omit rig to avoid duplication
		{"prefix_eq_rig", "fhc", "fhc", "stoat", "fhc-polecat-stoat"},
		// another prefix == rig case
		{"helm_rig", "helm", "helm", "worker", "helm-polecat-worker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PolecatBeadIDWithPrefix(tt.prefix, tt.rig, tt.pname)
			if got != tt.want {
				t.Errorf("PolecatBeadIDWithPrefix(%q, %q, %q) = %q, want %q",
					tt.prefix, tt.rig, tt.pname, got, tt.want)
			}
		})
	}
}

// TestWitnessBeadIDWithPrefixDeduplication tests that prefix==rig doesn't duplicate.
func TestWitnessBeadIDWithPrefixDeduplication(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		want   string
	}{
		{"prefix_ne_rig", "gt", "gastown", "gt-gastown-witness"},
		{"prefix_eq_rig", "fhc", "fhc", "fhc-witness"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WitnessBeadIDWithPrefix(tt.prefix, tt.rig)
			if got != tt.want {
				t.Errorf("WitnessBeadIDWithPrefix(%q, %q) = %q, want %q",
					tt.prefix, tt.rig, got, tt.want)
			}
		})
	}
}

// TestRefineryBeadIDWithPrefixDeduplication tests that prefix==rig doesn't duplicate.
func TestRefineryBeadIDWithPrefixDeduplication(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		want   string
	}{
		{"prefix_ne_rig", "gt", "gastown", "gt-gastown-refinery"},
		{"prefix_eq_rig", "fhc", "fhc", "fhc-refinery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RefineryBeadIDWithPrefix(tt.prefix, tt.rig)
			if got != tt.want {
				t.Errorf("RefineryBeadIDWithPrefix(%q, %q) = %q, want %q",
					tt.prefix, tt.rig, got, tt.want)
			}
		})
	}
}

// TestCrewBeadIDWithPrefixDeduplication tests that prefix==rig doesn't duplicate.
func TestCrewBeadIDWithPrefixDeduplication(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		cname  string
		want   string
	}{
		{"prefix_ne_rig", "gt", "gastown", "max", "gt-gastown-crew-max"},
		{"prefix_eq_rig", "fhc", "fhc", "max", "fhc-crew-max"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CrewBeadIDWithPrefix(tt.prefix, tt.rig, tt.cname)
			if got != tt.want {
				t.Errorf("CrewBeadIDWithPrefix(%q, %q, %q) = %q, want %q",
					tt.prefix, tt.rig, tt.cname, got, tt.want)
			}
		})
	}
}

// TestPolecatBeadIDTown tests the town-level Polecat bead ID (fix for loc-1augh).
func TestPolecatBeadIDTown(t *testing.T) {
	tests := []struct {
		rig   string
		pname string
		want  string
	}{
		{"gastown", "toast", "hq-gastown-polecat-toast"},
		{"local", "marten", "hq-local-polecat-marten"},
		{"fhc", "stoat", "hq-fhc-polecat-stoat"},
	}

	for _, tt := range tests {
		t.Run(tt.rig+"/"+tt.pname, func(t *testing.T) {
			got := PolecatBeadIDTown(tt.rig, tt.pname)
			if got != tt.want {
				t.Errorf("PolecatBeadIDTown(%q, %q) = %q, want %q",
					tt.rig, tt.pname, got, tt.want)
			}
		})
	}
}

// TestWitnessBeadIDTown tests the town-level Witness bead ID.
func TestWitnessBeadIDTown(t *testing.T) {
	tests := []struct {
		rig  string
		want string
	}{
		{"gastown", "hq-gastown-witness"},
		{"local", "hq-local-witness"},
	}

	for _, tt := range tests {
		t.Run(tt.rig, func(t *testing.T) {
			got := WitnessBeadIDTown(tt.rig)
			if got != tt.want {
				t.Errorf("WitnessBeadIDTown(%q) = %q, want %q",
					tt.rig, got, tt.want)
			}
		})
	}
}

// TestRefineryBeadIDTown tests the town-level Refinery bead ID.
func TestRefineryBeadIDTown(t *testing.T) {
	tests := []struct {
		rig  string
		want string
	}{
		{"gastown", "hq-gastown-refinery"},
		{"local", "hq-local-refinery"},
	}

	for _, tt := range tests {
		t.Run(tt.rig, func(t *testing.T) {
			got := RefineryBeadIDTown(tt.rig)
			if got != tt.want {
				t.Errorf("RefineryBeadIDTown(%q) = %q, want %q",
					tt.rig, got, tt.want)
			}
		})
	}
}

// TestCrewBeadIDTown tests the town-level Crew bead ID.
func TestCrewBeadIDTown(t *testing.T) {
	tests := []struct {
		rig   string
		cname string
		want  string
	}{
		{"gastown", "max", "hq-gastown-crew-max"},
		{"local", "dave", "hq-local-crew-dave"},
	}

	for _, tt := range tests {
		t.Run(tt.rig+"/"+tt.cname, func(t *testing.T) {
			got := CrewBeadIDTown(tt.rig, tt.cname)
			if got != tt.want {
				t.Errorf("CrewBeadIDTown(%q, %q) = %q, want %q",
					tt.rig, tt.cname, got, tt.want)
			}
		})
	}
}
