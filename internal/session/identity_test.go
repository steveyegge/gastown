package session

import (
	"testing"
)

func TestParseSessionName(t *testing.T) {
	tests := []struct {
		name     string
		session  string
		wantRole Role
		wantRig  string
		wantName string
		wantErr  bool
	}{
		// Town-level roles (hq-mayor, hq-deacon)
		{
			name:     "mayor",
			session:  "hq-mayor",
			wantRole: RoleMayor,
		},
		{
			name:     "deacon",
			session:  "hq-deacon",
			wantRole: RoleDeacon,
		},

		// Witness (simple rig)
		{
			name:     "witness simple rig",
			session:  "gt-gastown-witness",
			wantRole: RoleWitness,
			wantRig:  "gastown",
		},
		{
			name:     "witness hyphenated rig",
			session:  "gt-foo-bar-witness",
			wantRole: RoleWitness,
			wantRig:  "foo-bar",
		},

		// Refinery (simple rig)
		{
			name:     "refinery simple rig",
			session:  "gt-gastown-refinery",
			wantRole: RoleRefinery,
			wantRig:  "gastown",
		},
		{
			name:     "refinery hyphenated rig",
			session:  "gt-my-project-refinery",
			wantRole: RoleRefinery,
			wantRig:  "my-project",
		},

		// Crew (with marker)
		{
			name:     "crew simple",
			session:  "gt-gastown-crew-max",
			wantRole: RoleCrew,
			wantRig:  "gastown",
			wantName: "max",
		},
		{
			name:     "crew hyphenated rig",
			session:  "gt-foo-bar-crew-alice",
			wantRole: RoleCrew,
			wantRig:  "foo-bar",
			wantName: "alice",
		},
		{
			name:     "crew hyphenated name",
			session:  "gt-gastown-crew-my-worker",
			wantRole: RoleCrew,
			wantRig:  "gastown",
			wantName: "my-worker",
		},

		// Polecat (fallback)
		{
			name:     "polecat simple",
			session:  "gt-gastown-morsov",
			wantRole: RolePolecat,
			wantRig:  "gastown",
			wantName: "morsov",
		},
		{
			name:     "polecat hyphenated rig",
			session:  "gt-foo-bar-Toast",
			wantRole: RolePolecat,
			wantRig:  "foo-bar",
			wantName: "Toast",
		},

		// Error cases
		{
			name:    "missing prefix",
			session: "gastown-witness",
			wantErr: true,
		},
		{
			name:    "empty after prefix",
			session: "gt-",
			wantErr: true,
		},
		{
			name:    "just prefix single segment",
			session: "gt-x",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSessionName(tt.session)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSessionName(%q) error = %v, wantErr %v", tt.session, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Role != tt.wantRole {
				t.Errorf("ParseSessionName(%q).Role = %v, want %v", tt.session, got.Role, tt.wantRole)
			}
			if got.Rig != tt.wantRig {
				t.Errorf("ParseSessionName(%q).Rig = %v, want %v", tt.session, got.Rig, tt.wantRig)
			}
			if got.Name != tt.wantName {
				t.Errorf("ParseSessionName(%q).Name = %v, want %v", tt.session, got.Name, tt.wantName)
			}
		})
	}
}

func TestAgentIdentity_SessionName(t *testing.T) {
	tests := []struct {
		name     string
		identity AgentIdentity
		want     string
	}{
		{
			name:     "mayor",
			identity: AgentIdentity{Role: RoleMayor},
			want:     "hq-mayor",
		},
		{
			name:     "deacon",
			identity: AgentIdentity{Role: RoleDeacon},
			want:     "hq-deacon",
		},
		{
			name:     "witness",
			identity: AgentIdentity{Role: RoleWitness, Rig: "gastown"},
			want:     "gt-gastown-witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "my-project"},
			want:     "gt-my-project-refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max"},
			want:     "gt-gastown-crew-max",
		},
		{
			name:     "polecat",
			identity: AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "morsov"},
			want:     "gt-gastown-morsov",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.SessionName(); got != tt.want {
				t.Errorf("AgentIdentity.SessionName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentIdentity_Address(t *testing.T) {
	tests := []struct {
		name     string
		identity AgentIdentity
		want     string
	}{
		{
			name:     "mayor",
			identity: AgentIdentity{Role: RoleMayor},
			want:     "mayor",
		},
		{
			name:     "deacon",
			identity: AgentIdentity{Role: RoleDeacon},
			want:     "deacon",
		},
		{
			name:     "witness",
			identity: AgentIdentity{Role: RoleWitness, Rig: "gastown"},
			want:     "gastown/witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "my-project"},
			want:     "my-project/refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max"},
			want:     "gastown/crew/max",
		},
		{
			name:     "polecat",
			identity: AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "Toast"},
			want:     "gastown/polecats/Toast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.Address(); got != tt.want {
				t.Errorf("AgentIdentity.Address() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSessionName_RoundTrip(t *testing.T) {
	// Test that parsing then reconstructing gives the same result
	sessions := []string{
		"hq-mayor",
		"hq-deacon",
		"gt-gastown-witness",
		"gt-foo-bar-refinery",
		"gt-gastown-crew-max",
		"gt-gastown-morsov",
	}

	for _, sess := range sessions {
		t.Run(sess, func(t *testing.T) {
			identity, err := ParseSessionName(sess)
			if err != nil {
				t.Fatalf("ParseSessionName(%q) error = %v", sess, err)
			}
			if got := identity.SessionName(); got != sess {
				t.Errorf("Round-trip failed: ParseSessionName(%q).SessionName() = %q", sess, got)
			}
		})
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    AgentIdentity
		wantErr bool
	}{
		{
			name:    "mayor",
			address: "mayor/",
			want:    AgentIdentity{Role: RoleMayor},
		},
		{
			name:    "deacon",
			address: "deacon",
			want:    AgentIdentity{Role: RoleDeacon},
		},
		{
			name:    "witness",
			address: "gastown/witness",
			want:    AgentIdentity{Role: RoleWitness, Rig: "gastown"},
		},
		{
			name:    "refinery",
			address: "rig-a/refinery",
			want:    AgentIdentity{Role: RoleRefinery, Rig: "rig-a"},
		},
		{
			name:    "crew",
			address: "gastown/crew/max",
			want:    AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max"},
		},
		{
			name:    "polecat explicit",
			address: "gastown/polecats/nux",
			want:    AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "nux"},
		},
		{
			name:    "polecat canonical",
			address: "gastown/nux",
			want:    AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "nux"},
		},
		{
			name:    "invalid",
			address: "gastown/crew",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAddress(tt.address)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAddress(%q) error = %v", tt.address, err)
			}
			if *got != tt.want {
				t.Fatalf("ParseAddress(%q) = %#v, want %#v", tt.address, *got, tt.want)
			}
		})
	}
}
