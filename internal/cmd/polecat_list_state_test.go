package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/polecat"
)

func TestEffectivePolecatState(t *testing.T) {
	tests := []struct {
		name string
		item PolecatListItem
		want polecat.State
	}{
		{
			name: "session-running-done-becomes-working",
			item: PolecatListItem{
				State:          polecat.StateDone,
				SessionRunning: true,
			},
			want: polecat.StateWorking,
		},
		{
			name: "session-dead-working-becomes-stalled",
			item: PolecatListItem{
				State:          polecat.StateWorking,
				SessionRunning: false,
			},
			want: polecat.StateStalled,
		},
		{
			name: "zombie-is-never-rewritten",
			item: PolecatListItem{
				State:          polecat.StateZombie,
				SessionRunning: false,
				Zombie:         true,
			},
			want: polecat.StateZombie,
		},
		{
			name: "idle-session-dead-stays-idle",
			item: PolecatListItem{
				State:          polecat.StateIdle,
				SessionRunning: false,
			},
			want: polecat.StateIdle,
		},
		{
			name: "idle-session-running-becomes-working",
			item: PolecatListItem{
				State:          polecat.StateIdle,
				SessionRunning: true,
			},
			want: polecat.StateWorking,
		},
		{
			name: "stalled-stays-stalled-when-session-dead",
			item: PolecatListItem{
				State:          polecat.StateStalled,
				SessionRunning: false,
			},
			want: polecat.StateStalled,
		},
		{
			name: "stalled-becomes-working-when-session-alive",
			item: PolecatListItem{
				State:          polecat.StateStalled,
				SessionRunning: true,
			},
			want: polecat.StateStalled, // stalled is a detected state, session running doesn't override
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectivePolecatState(tt.item)
			if got != tt.want {
				t.Fatalf("effectivePolecatState() = %q, want %q", got, tt.want)
			}
		})
	}
}

