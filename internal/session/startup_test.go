package session

import (
	"strings"
	"testing"
)

func TestFormatStartupBeacon(t *testing.T) {
	tests := []struct {
		name     string
		cfg      BeaconConfig
		wantSub  []string // substrings that must appear
		wantNot  []string // substrings that must NOT appear
	}{
		{
			name: "assigned with mol-id",
			cfg: BeaconConfig{
				Recipient: "gastown/crew/gus",
				Sender:    "deacon",
				Topic:     "assigned",
				MolID:     "gt-abc12",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/crew/gus",
				"<- deacon",
				"assigned:gt-abc12",
				"Work is on your hook", // assigned includes actionable instructions
				"gt hook",
			},
		},
		{
			name: "cold-start no mol-id",
			cfg: BeaconConfig{
				Recipient: "deacon",
				Sender:    "mayor",
				Topic:     "cold-start",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"deacon",
				"<- mayor",
				"cold-start",
				"Check your hook and mail", // cold-start includes explicit instructions (like handoff)
				"gt hook",
				"gt mail inbox",
			},
			// No wantNot - timestamp contains ":"
		},
		{
			name: "handoff self",
			cfg: BeaconConfig{
				Recipient: "gastown/witness",
				Sender:    "self",
				Topic:     "handoff",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/witness",
				"<- self",
				"handoff",
				"Check your hook and mail", // handoff includes explicit instructions
				"gt hook",
				"gt mail inbox",
			},
		},
		{
			name: "mol-id only",
			cfg: BeaconConfig{
				Recipient: "gastown/polecats/Toast",
				Sender:    "witness",
				MolID:     "gt-xyz99",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/polecats/Toast",
				"<- witness",
				"gt-xyz99",
			},
		},
		{
			name: "empty topic defaults to ready",
			cfg: BeaconConfig{
				Recipient: "deacon",
				Sender:    "mayor",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"ready",
			},
		},
		{
			name: "start beacon has no prime instruction",
			cfg: BeaconConfig{
				Recipient: "beads/crew/fang",
				Sender:    "human",
				Topic:     "start",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"beads/crew/fang",
				"<- human",
				"start",
			},
			wantNot: []string{
				"gt prime",
			},
		},
		{
			name: "restart beacon has no prime instruction",
			cfg: BeaconConfig{
				Recipient: "gastown/crew/george",
				Sender:    "human",
				Topic:     "restart",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/crew/george",
				"restart",
			},
			wantNot: []string{
				"gt prime",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatStartupBeacon(tt.cfg)

			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("FormatStartupBeacon() = %q, want to contain %q", got, sub)
				}
			}

			for _, sub := range tt.wantNot {
				if strings.Contains(got, sub) {
					t.Errorf("FormatStartupBeacon() = %q, should NOT contain %q", got, sub)
				}
			}
		})
	}
}

func TestBuildStartupPrompt(t *testing.T) {
	// BuildStartupPrompt combines beacon + instructions
	cfg := BeaconConfig{
		Recipient: "deacon",
		Sender:    "daemon",
		Topic:     "patrol",
	}
	instructions := "Start patrol immediately."

	got := BuildStartupPrompt(cfg, instructions)

	// Should contain beacon parts
	if !strings.Contains(got, "[GAS TOWN]") {
		t.Errorf("BuildStartupPrompt() missing beacon header")
	}
	if !strings.Contains(got, "deacon") {
		t.Errorf("BuildStartupPrompt() missing recipient")
	}
	if !strings.Contains(got, "<- daemon") {
		t.Errorf("BuildStartupPrompt() missing sender")
	}
	if !strings.Contains(got, "patrol") {
		t.Errorf("BuildStartupPrompt() missing topic")
	}

	// Should contain instructions after beacon
	if !strings.Contains(got, instructions) {
		t.Errorf("BuildStartupPrompt() missing instructions")
	}

	// Should have blank line between beacon and instructions
	if !strings.Contains(got, "\n\n"+instructions) {
		t.Errorf("BuildStartupPrompt() missing blank line before instructions")
	}
}
