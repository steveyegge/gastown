package daemon

import (
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/deacon"
)

func TestCheckDeaconHeartbeatActionWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		age       time.Duration
		wantFresh bool
		wantNudge bool
	}{
		{
			name:      "fresh heartbeat does nothing",
			age:       2 * time.Minute,
			wantFresh: true,
			wantNudge: false,
		},
		{
			name:      "stale heartbeat triggers nudge window",
			age:       7 * time.Minute,
			wantFresh: false,
			wantNudge: true,
		},
		{
			name:      "stale heartbeat still nudges before escalation threshold",
			age:       12 * time.Minute,
			wantFresh: false,
			wantNudge: true,
		},
		{
			name:      "very stale heartbeat escalates instead of nudging",
			age:       20 * time.Minute,
			wantFresh: false,
			wantNudge: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hb := &deacon.Heartbeat{
				Timestamp: time.Now().Add(-tc.age),
				Cycle:     1,
			}

			if got := hb.IsFresh(); got != tc.wantFresh {
				t.Fatalf("IsFresh() = %v, want %v", got, tc.wantFresh)
			}

			gotNudge := !hb.IsFresh() && !shouldEscalateStaleDeacon(hb.Age())
			if gotNudge != tc.wantNudge {
				t.Fatalf("nudge-window = %v, want %v", gotNudge, tc.wantNudge)
			}
		})
	}
}
