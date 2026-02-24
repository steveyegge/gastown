package beads

import (
	"strings"
	"testing"
)

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

// TestAgentBeadIDWithPrefix tests agent bead ID generation, including dedup when prefix == rig.
func TestAgentBeadIDWithPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		role   string
		wname  string
		want   string
	}{
		// Normal cases (prefix != rig)
		{"town-level mayor", "gt", "", "mayor", "", "gt-mayor"},
		{"rig witness", "gt", "gastown", "witness", "", "gt-gastown-witness"},
		{"rig polecat", "gt", "gastown", "polecat", "nux", "gt-gastown-polecat-nux"},
		{"rig crew", "bd", "beads", "crew", "dave", "bd-beads-crew-dave"},

		// Collapsed cases (prefix == rig) — should NOT stutter
		{"dedup witness", "ff", "ff", "witness", "", "ff-witness"},
		{"dedup refinery", "ff", "ff", "refinery", "", "ff-refinery"},
		{"dedup polecat", "ff", "ff", "polecat", "nux", "ff-polecat-nux"},
		{"dedup crew", "ff", "ff", "crew", "dave", "ff-crew-dave"},
		{"dedup bd-beads", "bd", "bd", "witness", "", "bd-witness"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AgentBeadIDWithPrefix(tt.prefix, tt.rig, tt.role, tt.wname)
			if got != tt.want {
				t.Errorf("AgentBeadIDWithPrefix(%q, %q, %q, %q) = %q, want %q",
					tt.prefix, tt.rig, tt.role, tt.wname, got, tt.want)
			}
		})
	}
}

// TestValidateAgentID tests agent ID validation.
func TestValidateAgentID(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		wantError     bool
		errorContains string
	}{
		// Town-level agents (no rig)
		{"valid mayor", "gt-mayor", false, ""},
		{"valid deacon", "gt-deacon", false, ""},

		// Town-level named agents (dogs)
		{"valid dog", "gt-dog-alpha", false, ""},
		{"valid dog with hyphen", "gt-dog-war-boy", false, ""},

		// Per-rig agents (canonical format: gt-<rig>-<role>)
		{"valid witness gastown", "gt-gastown-witness", false, ""},
		{"valid refinery beads", "gt-beads-refinery", false, ""},

		// Named agents (canonical format: gt-<rig>-<role>-<name>)
		{"valid polecat", "gt-gastown-polecat-nux", false, ""},
		{"valid crew", "gt-beads-crew-dave", false, ""},
		{"valid polecat with complex name", "gt-gastown-polecat-war-boy-1", false, ""},

		// Valid: alternative prefixes (beads uses bd-)
		{"valid bd-mayor", "bd-mayor", false, ""},
		{"valid bd-beads-polecat-pearl", "bd-beads-polecat-pearl", false, ""},
		{"valid bd-beads-witness", "bd-beads-witness", false, ""},

		// Valid: hyphenated rig names
		{"hyphenated rig witness", "ob-my-project-witness", false, ""},
		{"hyphenated rig refinery", "gt-foo-bar-refinery", false, ""},
		{"hyphenated rig crew", "bd-my-cool-project-crew-fang", false, ""},
		{"hyphenated rig polecat", "gt-some-long-rig-name-polecat-nux", false, ""},
		{"hyphenated rig and name", "gt-my-rig-polecat-war-boy", false, ""},
		{"multi-hyphen rig crew", "ob-a-b-c-d-crew-dave", false, ""},

		// Invalid: no prefix (missing hyphen)
		{"no prefix", "mayor", true, "must have a prefix followed by '-'"},

		// Invalid: empty
		{"empty id", "", true, "agent ID is required"},

		// Invalid: unknown role in position 2
		{"unknown role", "gt-gastown-admin", true, "invalid agent format"},

		// Invalid: town-level with rig (put role first)
		{"mayor with rig suffix", "gt-gastown-mayor", true, "cannot have rig/name suffixes"},
		{"deacon with rig suffix", "gt-beads-deacon", true, "cannot have rig/name suffixes"},

		// Collapsed form: rig-level role without rig (prefix == rig)
		{"collapsed witness", "gt-witness", false, ""},
		{"collapsed refinery", "gt-refinery", false, ""},
		{"collapsed polecat", "ff-polecat-nux", false, ""},
		{"collapsed crew", "ff-crew-dave", false, ""},

		// Invalid: named agent without name
		{"crew no name", "gt-beads-crew", true, "requires name"},
		{"polecat no name", "gt-gastown-polecat", true, "requires name"},
		{"dog no name", "gt-dog", true, "requires name"},

		// Valid: worker name collides with role keyword
		{"polecat named witness", "gt-gastown-polecat-witness", false, ""},
		{"polecat named refinery", "gt-gastown-polecat-refinery", false, ""},
		{"crew named witness", "gt-gastown-crew-witness", false, ""},
		{"crew named refinery", "gt-gastown-crew-refinery", false, ""},
		{"polecat named crew", "gt-gastown-polecat-crew", false, ""},
		{"crew named polecat", "gt-gastown-crew-polecat", false, ""},

		// Invalid: witness/refinery with extra parts (no named role to the left)
		{"witness with name", "gt-gastown-witness-extra", true, "cannot have name suffix"},
		{"refinery with name", "gt-beads-refinery-extra", true, "cannot have name suffix"},

		// Invalid: empty components
		{"empty after prefix", "gt-", true, "must include content after prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentID(tt.id)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateAgentID(%q) error = %v, wantError %v", tt.id, err, tt.wantError)
				return
			}
			if err != nil && tt.errorContains != "" {
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("ValidateAgentID(%q) error = %q, should contain %q", tt.id, err.Error(), tt.errorContains)
				}
			}
		})
	}
}

// TestExtractAgentPrefix tests prefix extraction from agent IDs.
func TestExtractAgentPrefix(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantPrefix string
	}{
		// Town-level agents
		{"mayor", "gt-mayor", "gt"},
		{"deacon", "gt-deacon", "gt"},
		{"bd mayor", "bd-mayor", "bd"},

		// Town-level named (dogs)
		{"dog", "gt-dog-alpha", "gt"},
		{"dog hyphen name", "gt-dog-war-boy", "gt"},

		// Per-rig agents
		{"witness", "gt-gastown-witness", "gt"},
		{"refinery", "bd-beads-refinery", "bd"},

		// Named agents - the bug case
		{"polecat 3-char name", "nx-nexus-polecat-nux", "nx"},
		{"polecat regular", "gt-gastown-polecat-phoenix", "gt"},
		{"crew", "gt-beads-crew-dave", "gt"},

		// Hyphenated rig names
		{"hyphenated rig", "gt-my-project-witness", "gt"},
		{"multi-hyphen rig polecat", "bd-my-cool-app-polecat-bob", "bd"},

		// Edge cases
		{"no hyphen", "nohyphen", ""},
		{"empty", "", ""},
		{"just prefix", "gt-", "gt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractAgentPrefix(tt.id)
			if got != tt.wantPrefix {
				t.Errorf("ExtractAgentPrefix(%q) = %q, want %q", tt.id, got, tt.wantPrefix)
			}
		})
	}
}

// TestAgentBeadIDRoundTrip verifies that generating an ID and parsing it back
// produces consistent results, especially for the collapsed form (GH#1877).
func TestAgentBeadIDRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		rig    string
		role   string
		wname  string
	}{
		// Normal cases
		{"normal witness", "gt", "gastown", "witness", ""},
		{"normal polecat", "gt", "gastown", "polecat", "nux"},
		{"normal crew", "bd", "beads", "crew", "dave"},

		// Collapsed cases (prefix == rig)
		{"collapsed witness", "ff", "ff", "witness", ""},
		{"collapsed refinery", "ff", "ff", "refinery", ""},
		{"collapsed polecat", "ff", "ff", "polecat", "nux"},
		{"collapsed crew", "ff", "ff", "crew", "dave"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := AgentBeadIDWithPrefix(tt.prefix, tt.rig, tt.role, tt.wname)

			// Validate the generated ID
			if err := ValidateAgentID(id); err != nil {
				t.Errorf("AgentBeadIDWithPrefix(%q,%q,%q,%q) = %q, ValidateAgentID error: %v",
					tt.prefix, tt.rig, tt.role, tt.wname, id, err)
			}

			// Parse back and verify role and name
			_, gotRole, gotName, ok := ParseAgentBeadID(id)
			if !ok {
				t.Errorf("ParseAgentBeadID(%q) failed", id)
				return
			}
			if gotRole != tt.role {
				t.Errorf("ParseAgentBeadID(%q) role = %q, want %q", id, gotRole, tt.role)
			}
			if gotName != tt.wname {
				t.Errorf("ParseAgentBeadID(%q) name = %q, want %q", id, gotName, tt.wname)
			}
		})
	}
}

