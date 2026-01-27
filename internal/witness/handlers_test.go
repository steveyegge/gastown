package witness

import (
	"testing"
)

func TestIsCrewPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		rigName string
		want    bool
	}{
		{
			name:    "old format crew",
			path:    "gastown/decision",
			rigName: "gastown",
			want:    true,
		},
		{
			name:    "witness is not crew",
			path:    "gastown/witness",
			rigName: "gastown",
			want:    false,
		},
		{
			name:    "refinery is not crew",
			path:    "gastown/refinery",
			rigName: "gastown",
			want:    false,
		},
		{
			name:    "polecats is not crew",
			path:    "gastown/polecats",
			rigName: "gastown",
			want:    false,
		},
		{
			name:    "different rig",
			path:    "beads/decision",
			rigName: "gastown",
			want:    false,
		},
		{
			name:    "new format not matched by this func",
			path:    "gastown/crew/decision",
			rigName: "gastown",
			want:    false, // 3 parts, handled differently
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCrewPath(tt.path, tt.rigName)
			if got != tt.want {
				t.Errorf("isCrewPath(%q, %q) = %v, want %v", tt.path, tt.rigName, got, tt.want)
			}
		})
	}
}

func TestCrewSessionName(t *testing.T) {
	tests := []struct {
		name        string
		requestedBy string
		rigName     string
		want        string
	}{
		{
			name:        "new format crew",
			requestedBy: "gastown/crew/decision",
			rigName:     "gastown",
			want:        "gt-gastown-decision",
		},
		{
			name:        "old format crew",
			requestedBy: "gastown/decision",
			rigName:     "gastown",
			want:        "gt-gastown-decision",
		},
		{
			name:        "witness returns empty",
			requestedBy: "gastown/witness",
			rigName:     "gastown",
			want:        "",
		},
		{
			name:        "refinery returns empty",
			requestedBy: "gastown/refinery",
			rigName:     "gastown",
			want:        "",
		},
		{
			name:        "polecats returns empty",
			requestedBy: "gastown/polecats",
			rigName:     "gastown",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crewSessionName(tt.requestedBy, tt.rigName)
			if got != tt.want {
				t.Errorf("crewSessionName(%q, %q) = %q, want %q", tt.requestedBy, tt.rigName, got, tt.want)
			}
		})
	}
}
