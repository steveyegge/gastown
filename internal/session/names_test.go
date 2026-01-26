package session

import (
	"testing"
)

func TestMayorSessionName(t *testing.T) {
	// Mayor session name is now fixed (one per machine), uses HQ prefix
	want := "hq-mayor"
	got := MayorSessionName()
	if got != want {
		t.Errorf("MayorSessionName() = %q, want %q", got, want)
	}
}

func TestDeaconSessionName(t *testing.T) {
	// Deacon session name is now fixed (one per machine), uses HQ prefix
	want := "hq-deacon"
	got := DeaconSessionName()
	if got != want {
		t.Errorf("DeaconSessionName() = %q, want %q", got, want)
	}
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
