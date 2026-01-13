package session

import (
	"strings"
	"testing"
)

func TestFormatStartupNudge(t *testing.T) {
	tests := []struct {
		name     string
		cfg      StartupNudgeConfig
		wantSub  []string // substrings that must appear
		wantNot  []string // substrings that must NOT appear
	}{
		{
			name: "assigned with mol-id",
			cfg: StartupNudgeConfig{
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
			cfg: StartupNudgeConfig{
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
			cfg: StartupNudgeConfig{
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
			cfg: StartupNudgeConfig{
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
			cfg: StartupNudgeConfig{
				Recipient: "deacon",
				Sender:    "mayor",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"ready",
			},
		},
		{
			name: "start includes fallback instructions",
			cfg: StartupNudgeConfig{
				Recipient: "beads/crew/fang",
				Sender:    "human",
				Topic:     "start",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"beads/crew/fang",
				"<- human",
				"start",
				"gt prime", // fallback instruction for when SessionStart hook fails
			},
		},
		{
			name: "restart includes fallback instructions",
			cfg: StartupNudgeConfig{
				Recipient: "gastown/crew/george",
				Sender:    "human",
				Topic:     "restart",
			},
			wantSub: []string{
				"[GAS TOWN]",
				"gastown/crew/george",
				"restart",
				"gt prime", // fallback instruction for when SessionStart hook fails
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatStartupNudge(tt.cfg)

			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("FormatStartupNudge() = %q, want to contain %q", got, sub)
				}
			}

			for _, sub := range tt.wantNot {
				if strings.Contains(got, sub) {
					t.Errorf("FormatStartupNudge() = %q, should NOT contain %q", got, sub)
				}
			}
		})
	}
}
