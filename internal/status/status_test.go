package status

import (
	"testing"
	"time"
)

func TestStateColorClass(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateActive, "status-active"},
		{StateThinking, "status-thinking"},
		{StateSlow, "status-slow"},
		{StateUnresponsive, "status-unresponsive"},
		{StateDead, "status-dead"},
		{StateBlocked, "status-blocked"},
		{StateDone, "status-done"},
		{State("invalid"), "status-unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.ColorClass(); got != tt.want {
				t.Errorf("State(%q).ColorClass() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestStateLabel(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateActive, "Active"},
		{StateThinking, "Thinking"},
		{StateSlow, "Slow"},
		{StateUnresponsive, "Unresponsive"},
		{StateDead, "Dead"},
		{StateBlocked, "Blocked"},
		{StateDone, "Done"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.Label(); got != tt.want {
				t.Errorf("State(%q).Label() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestStateIsHealthy(t *testing.T) {
	healthyStates := []State{StateActive, StateThinking, StateDone}
	unhealthyStates := []State{StateSlow, StateUnresponsive, StateDead, StateBlocked}

	for _, s := range healthyStates {
		if !s.IsHealthy() {
			t.Errorf("State(%q).IsHealthy() = false, want true", s)
		}
	}

	for _, s := range unhealthyStates {
		if s.IsHealthy() {
			t.Errorf("State(%q).IsHealthy() = true, want false", s)
		}
	}
}

func TestStateNeedsAttention(t *testing.T) {
	needsAttention := []State{StateSlow, StateUnresponsive, StateDead, StateBlocked}
	noAttention := []State{StateActive, StateThinking, StateDone}

	for _, s := range needsAttention {
		if !s.NeedsAttention() {
			t.Errorf("State(%q).NeedsAttention() = false, want true", s)
		}
	}

	for _, s := range noAttention {
		if s.NeedsAttention() {
			t.Errorf("State(%q).NeedsAttention() = true, want false", s)
		}
	}
}

func TestSignalsMostRecentActivity(t *testing.T) {
	now := time.Now()
	old := now.Add(-1 * time.Hour)
	older := now.Add(-2 * time.Hour)

	tests := []struct {
		name    string
		signals Signals
		want    *time.Time
	}{
		{
			name:    "no signals",
			signals: Signals{},
			want:    nil,
		},
		{
			name: "only git",
			signals: Signals{
				GitCommit: &now,
			},
			want: &now,
		},
		{
			name: "git is most recent",
			signals: Signals{
				GitCommit:       &now,
				BeadsUpdate:     &old,
				SessionActivity: &older,
			},
			want: &now,
		},
		{
			name: "beads is most recent",
			signals: Signals{
				GitCommit:       &old,
				BeadsUpdate:     &now,
				SessionActivity: &older,
			},
			want: &now,
		},
		{
			name: "session is most recent",
			signals: Signals{
				GitCommit:       &older,
				BeadsUpdate:     &old,
				SessionActivity: &now,
			},
			want: &now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.signals.MostRecentActivity()
			if tt.want == nil && got != nil {
				t.Errorf("MostRecentActivity() = %v, want nil", got)
			} else if tt.want != nil && (got == nil || !got.Equal(*tt.want)) {
				t.Errorf("MostRecentActivity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompute(t *testing.T) {
	now := time.Now()
	recentActivity := now.Add(-2 * time.Minute)    // < 5 min = Active
	thinkingActivity := now.Add(-10 * time.Minute) // 5-15 min = Thinking
	slowActivity := now.Add(-20 * time.Minute)     // 15-30 min = Slow
	oldActivity := now.Add(-45 * time.Minute)      // > 30 min = Unresponsive

	tests := []struct {
		name      string
		signals   Signals
		wantState State
	}{
		{
			name: "closed issue returns Done",
			signals: Signals{
				IsClosed:      true,
				SessionExists: true,
				GitCommit:     &recentActivity,
			},
			wantState: StateDone,
		},
		{
			name: "no session returns Dead",
			signals: Signals{
				SessionExists: false,
				GitCommit:     &recentActivity,
			},
			wantState: StateDead,
		},
		{
			name: "blocked returns Blocked",
			signals: Signals{
				IsBlocked:     true,
				SessionExists: true,
				GitCommit:     &recentActivity,
			},
			wantState: StateBlocked,
		},
		{
			name: "recent activity returns Active",
			signals: Signals{
				SessionExists:   true,
				SessionActivity: &recentActivity,
			},
			wantState: StateActive,
		},
		{
			name: "moderate quiet returns Thinking",
			signals: Signals{
				SessionExists:   true,
				SessionActivity: &thinkingActivity,
			},
			wantState: StateThinking,
		},
		{
			name: "long quiet returns Slow",
			signals: Signals{
				SessionExists:   true,
				SessionActivity: &slowActivity,
			},
			wantState: StateSlow,
		},
		{
			name: "very long quiet returns Unresponsive",
			signals: Signals{
				SessionExists:   true,
				SessionActivity: &oldActivity,
			},
			wantState: StateUnresponsive,
		},
		{
			name: "no activity data returns Unresponsive",
			signals: Signals{
				SessionExists: true,
			},
			wantState: StateUnresponsive,
		},
		{
			name: "git commit can trigger Active",
			signals: Signals{
				SessionExists:   true,
				GitCommit:       &recentActivity,
				SessionActivity: &oldActivity, // Old session but recent commit
			},
			wantState: StateActive,
		},
		{
			name: "beads update can trigger Active",
			signals: Signals{
				SessionExists:   true,
				BeadsUpdate:     &recentActivity,
				SessionActivity: &oldActivity, // Old session but recent beads update
			},
			wantState: StateActive,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := Compute(tt.signals)
			if status.State != tt.wantState {
				t.Errorf("Compute().State = %q, want %q", status.State, tt.wantState)
			}
		})
	}
}

func TestComputePriority(t *testing.T) {
	// Test that terminal states take priority over time-based states
	now := time.Now()
	recentActivity := now.Add(-2 * time.Minute)

	// Done takes priority over everything
	status := Compute(Signals{
		IsClosed:        true,
		IsBlocked:       true,
		SessionExists:   true,
		SessionActivity: &recentActivity,
	})
	if status.State != StateDone {
		t.Errorf("Done should take priority, got %q", status.State)
	}

	// Dead takes priority over Blocked
	status = Compute(Signals{
		IsClosed:      false,
		IsBlocked:     true,
		SessionExists: false,
	})
	if status.State != StateDead {
		t.Errorf("Dead should take priority over Blocked, got %q", status.State)
	}

	// Blocked takes priority over time-based states
	status = Compute(Signals{
		IsClosed:        false,
		IsBlocked:       true,
		SessionExists:   true,
		SessionActivity: &recentActivity,
	})
	if status.State != StateBlocked {
		t.Errorf("Blocked should take priority over Active, got %q", status.State)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "<1m"},
		{1 * time.Minute, "1m"},
		{5 * time.Minute, "5m"},
		{59 * time.Minute, "59m"},
		{1 * time.Hour, "1h"},
		{3 * time.Hour, "3h"},
		{23 * time.Hour, "23h"},
		{24 * time.Hour, "1d"},
		{48 * time.Hour, "2d"},
		{72 * time.Hour, "3d"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatDuration(tt.d); got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestDetermineMostRecentSignal(t *testing.T) {
	now := time.Now()
	old := now.Add(-1 * time.Hour)
	older := now.Add(-2 * time.Hour)

	tests := []struct {
		name    string
		signals Signals
		recent  time.Time
		want    string
	}{
		{
			name: "git most recent",
			signals: Signals{
				GitCommit:       &now,
				BeadsUpdate:     &old,
				SessionActivity: &older,
			},
			recent: now,
			want:   "git",
		},
		{
			name: "beads most recent",
			signals: Signals{
				GitCommit:       &old,
				BeadsUpdate:     &now,
				SessionActivity: &older,
			},
			recent: now,
			want:   "beads",
		},
		{
			name: "session most recent",
			signals: Signals{
				GitCommit:       &older,
				BeadsUpdate:     &old,
				SessionActivity: &now,
			},
			recent: now,
			want:   "session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := determineMostRecentSignal(tt.signals, tt.recent); got != tt.want {
				t.Errorf("determineMostRecentSignal() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateFromDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want State
	}{
		{0, StateActive},
		{2 * time.Minute, StateActive},
		{4 * time.Minute, StateActive},
		{5 * time.Minute, StateThinking},
		{10 * time.Minute, StateThinking},
		{14 * time.Minute, StateThinking},
		{15 * time.Minute, StateSlow},
		{20 * time.Minute, StateSlow},
		{29 * time.Minute, StateSlow},
		{30 * time.Minute, StateUnresponsive},
		{45 * time.Minute, StateUnresponsive},
		{2 * time.Hour, StateUnresponsive},
	}

	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			if got := stateFromDuration(tt.d); got != tt.want {
				t.Errorf("stateFromDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
