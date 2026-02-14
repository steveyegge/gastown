package session

import (
	"testing"
)

func TestMayorSessionName(t *testing.T) {
	// Default town name is "hq" for backward compatibility
	SetTownName("hq")
	want := "hq-mayor"
	got := MayorSessionName()
	if got != want {
		t.Errorf("MayorSessionName() = %q, want %q", got, want)
	}
}

func TestMayorSessionNameCustomTown(t *testing.T) {
	SetTownName("redos")
	defer SetTownName("hq") // restore default
	want := "redos-mayor"
	got := MayorSessionName()
	if got != want {
		t.Errorf("MayorSessionName() = %q, want %q", got, want)
	}
}

func TestDeaconSessionName(t *testing.T) {
	SetTownName("hq")
	want := "hq-deacon"
	got := DeaconSessionName()
	if got != want {
		t.Errorf("DeaconSessionName() = %q, want %q", got, want)
	}
}

func TestDeaconSessionNameCustomTown(t *testing.T) {
	SetTownName("nyx")
	defer SetTownName("hq")
	want := "nyx-deacon"
	got := DeaconSessionName()
	if got != want {
		t.Errorf("DeaconSessionName() = %q, want %q", got, want)
	}
}

func TestOverseerSessionName(t *testing.T) {
	SetTownName("hq")
	want := "hq-overseer"
	got := OverseerSessionName()
	if got != want {
		t.Errorf("OverseerSessionName() = %q, want %q", got, want)
	}
}

func TestOverseerSessionNameCustomTown(t *testing.T) {
	SetTownName("legend")
	defer SetTownName("hq")
	want := "legend-overseer"
	got := OverseerSessionName()
	if got != want {
		t.Errorf("OverseerSessionName() = %q, want %q", got, want)
	}
}

func TestBootSessionName(t *testing.T) {
	SetTownName("hq")
	want := "hq-boot"
	got := BootSessionName()
	if got != want {
		t.Errorf("BootSessionName() = %q, want %q", got, want)
	}
}

func TestBootSessionNameCustomTown(t *testing.T) {
	SetTownName("redos")
	defer SetTownName("hq")
	want := "redos-boot"
	got := BootSessionName()
	if got != want {
		t.Errorf("BootSessionName() = %q, want %q", got, want)
	}
}

func TestMultipleTownsUniqueNames(t *testing.T) {
	towns := []string{"redos", "nyx", "legend"}
	mayorNames := make(map[string]bool)
	deaconNames := make(map[string]bool)

	for _, town := range towns {
		SetTownName(town)
		mayor := MayorSessionName()
		deacon := DeaconSessionName()

		if mayorNames[mayor] {
			t.Errorf("Duplicate mayor session name: %q", mayor)
		}
		if deaconNames[deacon] {
			t.Errorf("Duplicate deacon session name: %q", deacon)
		}
		mayorNames[mayor] = true
		deaconNames[deacon] = true
	}

	SetTownName("hq") // restore
}

func TestWitnessSessionName(t *testing.T) {
	tests := []struct {
		rig  string
		want string
	}{
		{"gastown", "gt-gastown-witness"},
		{"beads", "gt-beads-witness"},
		{"foo", "gt-foo-witness"},
	}
	for _, tt := range tests {
		t.Run(tt.rig, func(t *testing.T) {
			got := WitnessSessionName(tt.rig)
			if got != tt.want {
				t.Errorf("WitnessSessionName(%q) = %q, want %q", tt.rig, got, tt.want)
			}
		})
	}
}

func TestRefinerySessionName(t *testing.T) {
	tests := []struct {
		rig  string
		want string
	}{
		{"gastown", "gt-gastown-refinery"},
		{"beads", "gt-beads-refinery"},
		{"foo", "gt-foo-refinery"},
	}
	for _, tt := range tests {
		t.Run(tt.rig, func(t *testing.T) {
			got := RefinerySessionName(tt.rig)
			if got != tt.want {
				t.Errorf("RefinerySessionName(%q) = %q, want %q", tt.rig, got, tt.want)
			}
		})
	}
}

func TestCrewSessionName(t *testing.T) {
	tests := []struct {
		rig  string
		name string
		want string
	}{
		{"gastown", "max", "gt-gastown-crew-max"},
		{"beads", "alice", "gt-beads-crew-alice"},
		{"foo", "bar", "gt-foo-crew-bar"},
	}
	for _, tt := range tests {
		t.Run(tt.rig+"/"+tt.name, func(t *testing.T) {
			got := CrewSessionName(tt.rig, tt.name)
			if got != tt.want {
				t.Errorf("CrewSessionName(%q, %q) = %q, want %q", tt.rig, tt.name, got, tt.want)
			}
		})
	}
}

func TestPolecatSessionName(t *testing.T) {
	tests := []struct {
		rig  string
		name string
		want string
	}{
		{"gastown", "Toast", "gt-gastown-Toast"},
		{"gastown", "Furiosa", "gt-gastown-Furiosa"},
		{"beads", "worker1", "gt-beads-worker1"},
	}
	for _, tt := range tests {
		t.Run(tt.rig+"/"+tt.name, func(t *testing.T) {
			got := PolecatSessionName(tt.rig, tt.name)
			if got != tt.want {
				t.Errorf("PolecatSessionName(%q, %q) = %q, want %q", tt.rig, tt.name, got, tt.want)
			}
		})
	}
}

func TestPrefix(t *testing.T) {
	want := "gt-"
	if Prefix != want {
		t.Errorf("Prefix = %q, want %q", Prefix, want)
	}
}

func TestSetTownName(t *testing.T) {
	// Empty string should not change the current name
	SetTownName("test")
	SetTownName("")
	if got := GetTownName(); got != "test" {
		t.Errorf("SetTownName(\"\") changed town name to %q", got)
	}
	SetTownName("hq") // restore
}

func TestSetTownName_RejectsGt(t *testing.T) {
	// "gt" would collide with the rig-level "gt-" prefix
	SetTownName("hq")
	SetTownName("gt")
	if got := GetTownName(); got != "hq" {
		t.Errorf("SetTownName(\"gt\") should be rejected, but town name changed to %q", got)
	}
}

func TestGetTownName(t *testing.T) {
	SetTownName("mytown")
	if got := GetTownName(); got != "mytown" {
		t.Errorf("GetTownName() = %q, want %q", got, "mytown")
	}
	SetTownName("hq") // restore
}
