package constants

import (
	"testing"
)

func TestRoleEmoji(t *testing.T) {
	tests := []struct {
		role   string
		expect string
	}{
		{RoleMayor, EmojiMayor},
		{RoleDeacon, EmojiDeacon},
		{RoleWitness, EmojiWitness},
		{RoleRefinery, EmojiRefinery},
		{RoleCrew, EmojiCrew},
		{RolePolecat, EmojiPolecat},
		{"unknown", "❓"},
		{"", "❓"},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := RoleEmoji(tt.role)
			if got != tt.expect {
				t.Errorf("RoleEmoji(%q) = %q, want %q", tt.role, got, tt.expect)
			}
		})
	}
}

func TestBeadsCustomTypesList(t *testing.T) {
	types := BeadsCustomTypesList()
	expected := []string{"agent", "role", "rig", "convoy", "slot", "queue", "event", "message", "molecule", "gate", "merge-request"}

	if len(types) != len(expected) {
		t.Fatalf("BeadsCustomTypesList() returned %d items, want %d", len(types), len(expected))
	}
	for i, typ := range types {
		if typ != expected[i] {
			t.Errorf("BeadsCustomTypesList()[%d] = %q, want %q", i, typ, expected[i])
		}
	}
}

func TestMayorRigsPath(t *testing.T) {
	got := MayorRigsPath("/town")
	expect := "/town/mayor/rigs.json"
	if got != expect {
		t.Errorf("MayorRigsPath = %q, want %q", got, expect)
	}
}

func TestMayorTownPath(t *testing.T) {
	got := MayorTownPath("/town")
	expect := "/town/mayor/town.json"
	if got != expect {
		t.Errorf("MayorTownPath = %q, want %q", got, expect)
	}
}

func TestRigMayorPath(t *testing.T) {
	got := RigMayorPath("/rig")
	expect := "/rig/mayor/rig"
	if got != expect {
		t.Errorf("RigMayorPath = %q, want %q", got, expect)
	}
}

func TestRigBeadsPath(t *testing.T) {
	got := RigBeadsPath("/rig")
	expect := "/rig/mayor/rig/.beads"
	if got != expect {
		t.Errorf("RigBeadsPath = %q, want %q", got, expect)
	}
}

func TestRigPolecatsPath(t *testing.T) {
	got := RigPolecatsPath("/rig")
	expect := "/rig/polecats"
	if got != expect {
		t.Errorf("RigPolecatsPath = %q, want %q", got, expect)
	}
}

func TestRigCrewPath(t *testing.T) {
	got := RigCrewPath("/rig")
	expect := "/rig/crew"
	if got != expect {
		t.Errorf("RigCrewPath = %q, want %q", got, expect)
	}
}

func TestMayorConfigPath(t *testing.T) {
	got := MayorConfigPath("/town")
	expect := "/town/mayor/config.json"
	if got != expect {
		t.Errorf("MayorConfigPath = %q, want %q", got, expect)
	}
}

func TestTownRuntimePath(t *testing.T) {
	got := TownRuntimePath("/town")
	expect := "/town/.runtime"
	if got != expect {
		t.Errorf("TownRuntimePath = %q, want %q", got, expect)
	}
}

func TestRigRuntimePath(t *testing.T) {
	got := RigRuntimePath("/rig")
	expect := "/rig/.runtime"
	if got != expect {
		t.Errorf("RigRuntimePath = %q, want %q", got, expect)
	}
}

func TestRigSettingsPath(t *testing.T) {
	got := RigSettingsPath("/rig")
	expect := "/rig/settings"
	if got != expect {
		t.Errorf("RigSettingsPath = %q, want %q", got, expect)
	}
}

func TestMayorAccountsPath(t *testing.T) {
	got := MayorAccountsPath("/town")
	expect := "/town/mayor/accounts.json"
	if got != expect {
		t.Errorf("MayorAccountsPath = %q, want %q", got, expect)
	}
}

func TestMayorQuotaPath(t *testing.T) {
	got := MayorQuotaPath("/town")
	expect := "/town/mayor/quota.json"
	if got != expect {
		t.Errorf("MayorQuotaPath = %q, want %q", got, expect)
	}
}
