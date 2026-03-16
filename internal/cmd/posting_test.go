package cmd

import "testing"

// ---------------------------------------------------------------------------
// PostingDisplay bracket notation tests
// ---------------------------------------------------------------------------

func TestPostingDisplay_Empty(t *testing.T) {
	t.Parallel()
	info := RoleInfo{Role: RolePolecat, Rig: "gastown", Polecat: "Toast"}
	got := info.PostingDisplay(false)
	if got != "" {
		t.Errorf("PostingDisplay() = %q, want empty when no posting set", got)
	}
}

func TestPostingDisplay_Simple(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "Toast",
		Posting: "dispatcher",
	}
	got := info.PostingDisplay(false)
	if got != "[dispatcher]" {
		t.Errorf("PostingDisplay(false) = %q, want %q", got, "[dispatcher]")
	}
}

func TestPostingDisplay_AmbiguousWithLevel(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:         RolePolecat,
		Rig:          "gastown",
		Polecat:      "Toast",
		Posting:      "dispatcher",
		PostingLevel: "rig",
	}
	got := info.PostingDisplay(true)
	if got != "[rig:dispatcher]" {
		t.Errorf("PostingDisplay(true) = %q, want %q", got, "[rig:dispatcher]")
	}
}

func TestPostingDisplay_EmbeddedLevel(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:         RolePolecat,
		Rig:          "gastown",
		Polecat:      "Toast",
		Posting:      "dispatcher",
		PostingLevel: "embedded",
	}
	got := info.PostingDisplay(true)
	if got != "[embedded:dispatcher]" {
		t.Errorf("PostingDisplay(true) = %q, want %q", got, "[embedded:dispatcher]")
	}
}

func TestPostingDisplay_TownLevel(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:         RolePolecat,
		Rig:          "gastown",
		Polecat:      "Toast",
		Posting:      "scout",
		PostingLevel: "town",
	}
	got := info.PostingDisplay(true)
	if got != "[town:scout]" {
		t.Errorf("PostingDisplay(true) = %q, want %q", got, "[town:scout]")
	}
}

func TestPostingDisplay_AmbiguousWithoutLevel(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "Toast",
		Posting: "scout",
		// PostingLevel intentionally empty
	}
	// When ambiguous=true but no level, should fall back to simple bracket
	got := info.PostingDisplay(true)
	if got != "[scout]" {
		t.Errorf("PostingDisplay(true, no level) = %q, want %q", got, "[scout]")
	}
}

func TestPostingDisplay_NotAmbiguous(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:         RolePolecat,
		Rig:          "gastown",
		Polecat:      "Toast",
		Posting:      "dispatcher",
		PostingLevel: "embedded",
	}
	// When ambiguous=false, level should be omitted even if set
	got := info.PostingDisplay(false)
	if got != "[dispatcher]" {
		t.Errorf("PostingDisplay(false) = %q, want %q (level should be omitted)", got, "[dispatcher]")
	}
}

// ---------------------------------------------------------------------------
// ActorString with posting bracket notation
// ---------------------------------------------------------------------------

func TestActorString_PolecatWithPosting(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "Toast",
		Posting: "dispatcher",
	}
	got := info.ActorString()
	want := "gastown/polecats/Toast[dispatcher]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q", got, want)
	}
}

func TestActorString_PolecatWithAmbiguousPosting(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:             RolePolecat,
		Rig:              "gastown",
		Polecat:          "Toast",
		Posting:          "dispatcher",
		PostingLevel:     "rig",
		PostingAmbiguous: true,
	}
	got := info.ActorString()
	want := "gastown/polecats/Toast[rig:dispatcher]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q", got, want)
	}
}

func TestActorString_PolecatWithoutPosting(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "Toast",
	}
	got := info.ActorString()
	want := "gastown/polecats/Toast"
	if got != want {
		t.Errorf("ActorString() = %q, want %q", got, want)
	}
}

func TestActorString_CrewWithPosting(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RoleCrew,
		Rig:     "gastown",
		Polecat: "max",
		Posting: "scout",
	}
	got := info.ActorString()
	want := "gastown/crew/max[scout]"
	if got != want {
		t.Errorf("ActorString() = %q, want %q", got, want)
	}
}

func TestActorString_CrewWithoutPosting(t *testing.T) {
	t.Parallel()
	info := RoleInfo{
		Role:    RoleCrew,
		Rig:     "gastown",
		Polecat: "max",
	}
	got := info.ActorString()
	want := "gastown/crew/max"
	if got != want {
		t.Errorf("ActorString() = %q, want %q", got, want)
	}
}

func TestActorString_NonWorkerRolesIgnorePosting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		info RoleInfo
		want string
	}{
		{
			name: "mayor",
			info: RoleInfo{Role: RoleMayor, Posting: "dispatcher"},
			want: "mayor",
		},
		{
			name: "deacon",
			info: RoleInfo{Role: RoleDeacon, Posting: "scout"},
			want: "deacon",
		},
		{
			name: "witness",
			info: RoleInfo{Role: RoleWitness, Rig: "gastown", Posting: "inspector"},
			want: "gastown/witness",
		},
		{
			name: "refinery",
			info: RoleInfo{Role: RoleRefinery, Rig: "gastown", Posting: "dispatcher"},
			want: "gastown/refinery",
		},
		{
			name: "boot",
			info: RoleInfo{Role: RoleBoot, Posting: "scout"},
			want: "deacon-boot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.info.ActorString()
			if got != tt.want {
				t.Errorf("ActorString() = %q, want %q (posting should not appear for %s)", got, tt.want, tt.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Posting display across all PostingLevel values
// ---------------------------------------------------------------------------

func TestPostingDisplay_AllLevels(t *testing.T) {
	t.Parallel()
	levels := []struct {
		level string
		want  string
	}{
		{"embedded", "[embedded:dispatcher]"},
		{"town", "[town:dispatcher]"},
		{"rig", "[rig:dispatcher]"},
	}

	for _, tt := range levels {
		t.Run(tt.level, func(t *testing.T) {
			t.Parallel()
			info := RoleInfo{
				Role:         RolePolecat,
				Rig:          "gastown",
				Polecat:      "Toast",
				Posting:      "dispatcher",
				PostingLevel: tt.level,
			}
			got := info.PostingDisplay(true)
			if got != tt.want {
				t.Errorf("PostingDisplay(true) with level=%q: got %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}
