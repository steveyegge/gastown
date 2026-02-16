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

func TestOverseerSessionName(t *testing.T) {
	want := "hq-overseer"
	got := OverseerSessionName()
	if got != want {
		t.Errorf("OverseerSessionName() = %q, want %q", got, want)
	}
}

func TestWitnessSessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		want      string
	}{
		{"gt", "gt-witness"},
		{"bd", "bd-witness"},
		{"hop", "hop-witness"},
		{"sky", "sky-witness"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix, func(t *testing.T) {
			got := WitnessSessionName(tt.rigPrefix)
			if got != tt.want {
				t.Errorf("WitnessSessionName(%q) = %q, want %q", tt.rigPrefix, got, tt.want)
			}
		})
	}
}

func TestRefinerySessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		want      string
	}{
		{"gt", "gt-refinery"},
		{"bd", "bd-refinery"},
		{"hop", "hop-refinery"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix, func(t *testing.T) {
			got := RefinerySessionName(tt.rigPrefix)
			if got != tt.want {
				t.Errorf("RefinerySessionName(%q) = %q, want %q", tt.rigPrefix, got, tt.want)
			}
		})
	}
}

func TestCrewSessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		name      string
		want      string
	}{
		{"gt", "max", "gt-crew-max"},
		{"bd", "alice", "bd-crew-alice"},
		{"hop", "bar", "hop-crew-bar"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix+"/"+tt.name, func(t *testing.T) {
			got := CrewSessionName(tt.rigPrefix, tt.name)
			if got != tt.want {
				t.Errorf("CrewSessionName(%q, %q) = %q, want %q", tt.rigPrefix, tt.name, got, tt.want)
			}
		})
	}
}

func TestPolecatSessionName(t *testing.T) {
	tests := []struct {
		rigPrefix string
		name      string
		want      string
	}{
		{"gt", "Toast", "gt-Toast"},
		{"gt", "Furiosa", "gt-Furiosa"},
		{"bd", "worker1", "bd-worker1"},
		{"hop", "ostrom", "hop-ostrom"},
	}
	for _, tt := range tests {
		t.Run(tt.rigPrefix+"/"+tt.name, func(t *testing.T) {
			got := PolecatSessionName(tt.rigPrefix, tt.name)
			if got != tt.want {
				t.Errorf("PolecatSessionName(%q, %q) = %q, want %q", tt.rigPrefix, tt.name, got, tt.want)
			}
		})
	}
}

func TestDefaultPrefix(t *testing.T) {
	want := "gt"
	if DefaultPrefix != want {
		t.Errorf("DefaultPrefix = %q, want %q", DefaultPrefix, want)
	}
}
