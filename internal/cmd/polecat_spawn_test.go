package cmd

import (
	"testing"
)

func TestSpawnedPolecatInfo_AgentID(t *testing.T) {
	tests := []struct {
		name        string
		rigName     string
		polecatName string
		want        string
	}{
		{
			name:        "standard names",
			rigName:     "gastown",
			polecatName: "Toast",
			want:        "gastown/polecats/Toast",
		},
		{
			name:        "different rig",
			rigName:     "beads",
			polecatName: "obsidian",
			want:        "beads/polecats/obsidian",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &SpawnedPolecatInfo{
				RigName:     tt.rigName,
				PolecatName: tt.polecatName,
			}
			if got := info.AgentID(); got != tt.want {
				t.Errorf("AgentID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpawnedPolecatInfo_SessionStarted(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want bool
	}{
		{
			name: "empty pane means not started",
			pane: "",
			want: false,
		},
		{
			name: "non-empty pane means started",
			pane: "%5",
			want: true,
		},
		{
			name: "full pane ID",
			pane: "gt-gastown-p-Toast:0.0",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &SpawnedPolecatInfo{
				Pane: tt.pane,
			}
			if got := info.SessionStarted(); got != tt.want {
				t.Errorf("SessionStarted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpawnedPolecatInfo_StartSession_AlreadyStarted(t *testing.T) {
	// When session is already started, StartSession should return existing pane
	info := &SpawnedPolecatInfo{
		RigName:     "gastown",
		PolecatName: "Toast",
		Pane:        "%5",
	}

	pane, err := info.StartSession()
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if pane != "%5" {
		t.Errorf("StartSession() = %q, want %q", pane, "%5")
	}
}
