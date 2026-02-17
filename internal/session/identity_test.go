package session

import (
	"testing"
)

// testRegistry returns a PrefixRegistry populated with test rig prefixes.
func testRegistry() *PrefixRegistry {
	r := NewPrefixRegistry()
	r.Register("gt", "gastown")
	r.Register("bd", "beads")
	r.Register("hop", "hop")
	r.Register("sky", "sky")
	r.Register("mp", "my-project")
	return r
}

func TestParseSessionName(t *testing.T) {
	reg := testRegistry()
	// Also set as default for ParseSessionName (no-registry variant)
	old := defaultRegistry
	defaultRegistry = reg
	defer func() { defaultRegistry = old }()

	tests := []struct {
		name       string
		session    string
		wantRole   Role
		wantRig    string
		wantName   string
		wantPrefix string
		wantErr    bool
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
		{
			name:     "boot",
			session:  "hq-boot",
			wantRole: RoleDeacon,
			wantName: "boot",
		},

		// Witness (new format: <prefix>-witness)
		{
			name:       "witness gastown",
			session:    "gt-witness",
			wantRole:   RoleWitness,
			wantRig:    "gastown",
			wantPrefix: "gt",
		},
		{
			name:       "witness beads",
			session:    "bd-witness",
			wantRole:   RoleWitness,
			wantRig:    "beads",
			wantPrefix: "bd",
		},
		{
			name:       "witness hop",
			session:    "hop-witness",
			wantRole:   RoleWitness,
			wantRig:    "hop",
			wantPrefix: "hop",
		},

		// Refinery (new format: <prefix>-refinery)
		{
			name:       "refinery gastown",
			session:    "gt-refinery",
			wantRole:   RoleRefinery,
			wantRig:    "gastown",
			wantPrefix: "gt",
		},
		{
			name:       "refinery multi-word prefix",
			session:    "mp-refinery",
			wantRole:   RoleRefinery,
			wantRig:    "my-project",
			wantPrefix: "mp",
		},

		// Crew (new format: <prefix>-crew-<name>)
		{
			name:       "crew gastown",
			session:    "gt-crew-max",
			wantRole:   RoleCrew,
			wantRig:    "gastown",
			wantName:   "max",
			wantPrefix: "gt",
		},
		{
			name:       "crew beads",
			session:    "bd-crew-alice",
			wantRole:   RoleCrew,
			wantRig:    "beads",
			wantName:   "alice",
			wantPrefix: "bd",
		},
		{
			name:       "crew hyphenated name",
			session:    "gt-crew-my-worker",
			wantRole:   RoleCrew,
			wantRig:    "gastown",
			wantName:   "my-worker",
			wantPrefix: "gt",
		},

		// Polecat (new format: <prefix>-<name>)
		{
			name:       "polecat gastown",
			session:    "gt-morsov",
			wantRole:   RolePolecat,
			wantRig:    "gastown",
			wantName:   "morsov",
			wantPrefix: "gt",
		},
		{
			name:       "polecat beads",
			session:    "bd-worker1",
			wantRole:   RolePolecat,
			wantRig:    "beads",
			wantName:   "worker1",
			wantPrefix: "bd",
		},
		{
			name:       "polecat hop",
			session:    "hop-ostrom",
			wantRole:   RolePolecat,
			wantRig:    "hop",
			wantName:   "ostrom",
			wantPrefix: "hop",
		},
		{
			name:       "polecat sky",
			session:    "sky-furiosa",
			wantRole:   RolePolecat,
			wantRig:    "sky",
			wantName:   "furiosa",
			wantPrefix: "sky",
		},

		// Unknown prefix (fallback: prefix = first segment, rig = prefix)
		{
			name:       "unknown prefix polecat",
			session:    "zz-alpha",
			wantRole:   RolePolecat,
			wantRig:    "zz",
			wantName:   "alpha",
			wantPrefix: "zz",
		},
		{
			name:       "unknown prefix witness",
			session:    "foo-witness",
			wantRole:   RoleWitness,
			wantRig:    "foo",
			wantName:   "",
			wantPrefix: "foo",
		},

		// Error cases
		{
			name:    "empty string",
			session: "",
			wantErr: true,
		},
		{
			name:    "no dash",
			session: "gtwitness",
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
			if tt.wantPrefix != "" && got.Prefix != tt.wantPrefix {
				t.Errorf("ParseSessionName(%q).Prefix = %v, want %v", tt.session, got.Prefix, tt.wantPrefix)
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
			name:     "boot",
			identity: AgentIdentity{Role: RoleDeacon, Name: "boot"},
			want:     "hq-boot",
		},
		{
			name:     "witness",
			identity: AgentIdentity{Role: RoleWitness, Rig: "gastown", Prefix: "gt"},
			want:     "gt-witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "beads", Prefix: "bd"},
			want:     "bd-refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max", Prefix: "gt"},
			want:     "gt-crew-max",
		},
		{
			name:     "polecat",
			identity: AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "morsov", Prefix: "gt"},
			want:     "gt-morsov",
		},
		{
			name:     "polecat hop",
			identity: AgentIdentity{Role: RolePolecat, Rig: "hop", Name: "ostrom", Prefix: "hop"},
			want:     "hop-ostrom",
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
			identity: AgentIdentity{Role: RoleWitness, Rig: "gastown", Prefix: "gt"},
			want:     "gastown/witness",
		},
		{
			name:     "refinery",
			identity: AgentIdentity{Role: RoleRefinery, Rig: "my-project", Prefix: "mp"},
			want:     "my-project/refinery",
		},
		{
			name:     "crew",
			identity: AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max", Prefix: "gt"},
			want:     "gastown/crew/max",
		},
		{
			name:     "polecat",
			identity: AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "Toast", Prefix: "gt"},
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
	reg := testRegistry()
	old := defaultRegistry
	defaultRegistry = reg
	defer func() { defaultRegistry = old }()

	// Test that parsing then reconstructing gives the same result
	sessions := []string{
		"hq-mayor",
		"hq-deacon",
		"gt-witness",
		"bd-refinery",
		"gt-crew-max",
		"gt-morsov",
		"hop-ostrom",
		"sky-furiosa",
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
			want:    AgentIdentity{Role: RoleWitness, Rig: "gastown", Prefix: PrefixFor("gastown")},
		},
		{
			name:    "refinery",
			address: "rig-a/refinery",
			want:    AgentIdentity{Role: RoleRefinery, Rig: "rig-a", Prefix: PrefixFor("rig-a")},
		},
		{
			name:    "crew",
			address: "gastown/crew/max",
			want:    AgentIdentity{Role: RoleCrew, Rig: "gastown", Name: "max", Prefix: PrefixFor("gastown")},
		},
		{
			name:    "polecat explicit",
			address: "gastown/polecats/nux",
			want:    AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "nux", Prefix: PrefixFor("gastown")},
		},
		{
			name:    "polecat canonical",
			address: "gastown/nux",
			want:    AgentIdentity{Role: RolePolecat, Rig: "gastown", Name: "nux", Prefix: PrefixFor("gastown")},
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

func TestPrefixRegistry(t *testing.T) {
	r := NewPrefixRegistry()
	r.Register("gt", "gastown")
	r.Register("bd", "beads")

	if got := r.PrefixForRig("gastown"); got != "gt" {
		t.Errorf("PrefixForRig(gastown) = %q, want %q", got, "gt")
	}
	if got := r.RigForPrefix("bd"); got != "beads" {
		t.Errorf("RigForPrefix(bd) = %q, want %q", got, "beads")
	}
	// Unknown rig returns default
	if got := r.PrefixForRig("unknown"); got != DefaultPrefix {
		t.Errorf("PrefixForRig(unknown) = %q, want %q", got, DefaultPrefix)
	}
	// Unknown prefix returns the prefix itself
	if got := r.RigForPrefix("zz"); got != "zz" {
		t.Errorf("RigForPrefix(zz) = %q, want %q", got, "zz")
	}
}
