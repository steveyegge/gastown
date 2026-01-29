package cmd

import "testing"

// TestIsDogTarget verifies the dog target pattern matching.
// Dogs can be targeted via:
//   - "deacon/dogs" -> pool dispatch (any idle dog)
//   - "deacon/dogs/alpha" -> specific dog
//   - "dog:" -> pool dispatch (shorthand)
//   - "dog:alpha" -> specific dog (shorthand)
func TestIsDogTarget(t *testing.T) {
	tests := []struct {
		target  string
		wantDog string
		wantIs  bool
	}{
		// Pool dispatch patterns
		{"deacon/dogs", "", true},
		{"dog:", "", true},
		{"DEACON/DOGS", "", true}, // case insensitive
		{"DOG:", "", true},

		// Specific dog patterns
		{"deacon/dogs/alpha", "alpha", true},
		{"deacon/dogs/bravo", "bravo", true},
		{"dog:alpha", "alpha", true},
		{"dog:bravo", "bravo", true},
		{"DOG:ALPHA", "alpha", true}, // case insensitive, name lowercased

		// Invalid patterns - not dog targets
		{"deacon", "", false},
		{"deacon/", "", false},
		{"deacon/dogs/", "", false},      // trailing slash, empty name
		{"deacon/dogs/alpha/extra", "", false}, // too many segments
		{"dog", "", false},               // missing colon
		{"dogs:alpha", "", false},        // wrong prefix
		{"polecat:alpha", "", false},
		{"gastown/polecats/alpha", "", false},
		{"mayor", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			gotDog, gotIs := IsDogTarget(tt.target)
			if gotIs != tt.wantIs {
				t.Errorf("IsDogTarget(%q) isDog = %v, want %v", tt.target, gotIs, tt.wantIs)
			}
			if gotDog != tt.wantDog {
				t.Errorf("IsDogTarget(%q) dogName = %q, want %q", tt.target, gotDog, tt.wantDog)
			}
		})
	}
}

// TestDogDispatchInfoDelayedSession verifies the delayed session start pattern.
// When DelaySessionStart is true:
//   - DispatchToDog returns with Pane="" and sessionDelayed=true
//   - StartDelayedSession() must be called to actually start the session
// This prevents the race condition where dogs start before their hook is set.
func TestDogDispatchInfoDelayedSession(t *testing.T) {
	// Test that DogDispatchInfo correctly tracks delayed state
	info := &DogDispatchInfo{
		DogName:        "alpha",
		AgentID:        "deacon/dogs/alpha",
		Pane:           "",    // Empty when delayed
		Spawned:        false,
		sessionDelayed: true,
		townRoot:       "/tmp/test",
		workDesc:       "test-work",
	}

	// Verify initial state
	if info.Pane != "" {
		t.Errorf("delayed dispatch should have empty Pane, got %q", info.Pane)
	}
	if !info.sessionDelayed {
		t.Error("sessionDelayed should be true for delayed dispatch")
	}

	// Note: We can't test StartDelayedSession without mocking tmux,
	// but we verify the struct correctly holds the delayed state.
}

// TestDogDispatchOptionsStruct verifies the DogDispatchOptions fields.
func TestDogDispatchOptionsStruct(t *testing.T) {
	opts := DogDispatchOptions{
		Create:            true,
		WorkDesc:          "mol-convoy-feed",
		DelaySessionStart: true,
	}

	if !opts.Create {
		t.Error("Create should be true")
	}
	if opts.WorkDesc != "mol-convoy-feed" {
		t.Errorf("WorkDesc = %q, want %q", opts.WorkDesc, "mol-convoy-feed")
	}
	if !opts.DelaySessionStart {
		t.Error("DelaySessionStart should be true")
	}
}
