package cmd

import (
	"fmt"
	"strings"
	"testing"
)

// mockCounter implements polecatCounter for testing checkMaxPolecatsCap.
type mockCounter struct {
	count int
	err   error
}

func (m *mockCounter) CountWorkingPolecats() (int, error) {
	return m.count, m.err
}

func TestCheckMaxPolecatsCap(t *testing.T) {
	tests := []struct {
		name        string
		rigName     string
		maxPolecats int
		working     int
		wantErr     bool
		errContains string
	}{
		{
			name:        "unlimited when max is zero",
			rigName:     "rig1",
			maxPolecats: 0,
			working:     100,
			wantErr:     false,
		},
		{
			name:        "unlimited when max is negative",
			rigName:     "rig1",
			maxPolecats: -1,
			working:     100,
			wantErr:     false,
		},
		{
			name:        "under limit allows spawn",
			rigName:     "rig1",
			maxPolecats: 5,
			working:     3,
			wantErr:     false,
		},
		{
			name:        "at limit blocks spawn",
			rigName:     "ScaledTest",
			maxPolecats: 2,
			working:     2,
			wantErr:     true,
			errContains: "polecat limit reached (2/2 active)",
		},
		{
			name:        "over limit blocks spawn",
			rigName:     "ScaledTest",
			maxPolecats: 2,
			working:     5,
			wantErr:     true,
			errContains: "polecat limit reached (5/2 active)",
		},
		{
			name:        "error message includes rig name",
			rigName:     "MyRig",
			maxPolecats: 1,
			working:     1,
			wantErr:     true,
			errContains: "rig MyRig",
		},
		{
			name:        "error message includes config hint",
			rigName:     "MyRig",
			maxPolecats: 1,
			working:     1,
			wantErr:     true,
			errContains: "gt rig config set MyRig max_polecats N",
		},
		{
			name:        "exactly one below limit allows spawn",
			rigName:     "rig1",
			maxPolecats: 3,
			working:     2,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter := &mockCounter{count: tt.working}
			err := checkMaxPolecatsCap(tt.rigName, tt.maxPolecats, counter)

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.errContains != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

func TestCheckMaxPolecatsCapCounterError(t *testing.T) {
	// When the counter returns an error, don't block spawning — fail open
	counter := &mockCounter{count: 0, err: fmt.Errorf("beads unavailable")}
	err := checkMaxPolecatsCap("rig1", 2, counter)
	if err != nil {
		t.Fatalf("counter error should not block spawn, got: %v", err)
	}
}
