package session

import (
	"testing"

	"github.com/steveyegge/gastown/internal/ids"
)

func TestParseSessionName(t *testing.T) {
	tests := []struct {
		name       string
		session    string
		wantRole   string
		wantRig    string
		wantWorker string
		wantErr    bool
	}{
		// Town-level roles (hq-mayor, hq-deacon)
		{
			name:     "mayor",
			session:  "hq-mayor",
			wantRole: "mayor",
		},
		{
			name:     "deacon",
			session:  "hq-deacon",
			wantRole: "deacon",
		},

		// Witness (simple rig)
		{
			name:     "witness simple rig",
			session:  "gt-gastown-witness",
			wantRole: "witness",
			wantRig:  "gastown",
		},
		{
			name:     "witness hyphenated rig",
			session:  "gt-foo-bar-witness",
			wantRole: "witness",
			wantRig:  "foo-bar",
		},

		// Refinery (simple rig)
		{
			name:     "refinery simple rig",
			session:  "gt-gastown-refinery",
			wantRole: "refinery",
			wantRig:  "gastown",
		},
		{
			name:     "refinery hyphenated rig",
			session:  "gt-my-project-refinery",
			wantRole: "refinery",
			wantRig:  "my-project",
		},

		// Crew (with marker)
		{
			name:       "crew simple",
			session:    "gt-gastown-crew-max",
			wantRole:   "crew",
			wantRig:    "gastown",
			wantWorker: "max",
		},
		{
			name:       "crew hyphenated rig",
			session:    "gt-foo-bar-crew-alice",
			wantRole:   "crew",
			wantRig:    "foo-bar",
			wantWorker: "alice",
		},
		{
			name:       "crew hyphenated name",
			session:    "gt-gastown-crew-my-worker",
			wantRole:   "crew",
			wantRig:    "gastown",
			wantWorker: "my-worker",
		},

		// Polecat (fallback)
		{
			name:       "polecat simple",
			session:    "gt-gastown-morsov",
			wantRole:   "polecat",
			wantRig:    "gastown",
			wantWorker: "morsov",
		},
		{
			name:       "polecat hyphenated rig",
			session:    "gt-foo-bar-Toast",
			wantRole:   "polecat",
			wantRig:    "foo-bar",
			wantWorker: "Toast",
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
			if got.Worker != tt.wantWorker {
				t.Errorf("ParseSessionName(%q).Worker = %v, want %v", tt.session, got.Worker, tt.wantWorker)
			}
		})
	}
}

func TestSessionNameFromAgentID(t *testing.T) {
	tests := []struct {
		name string
		id   ids.AgentID
		want string
	}{
		{
			name: "mayor",
			id:   ids.AgentID{Role: "mayor"},
			want: "hq-mayor",
		},
		{
			name: "deacon",
			id:   ids.AgentID{Role: "deacon"},
			want: "hq-deacon",
		},
		{
			name: "witness",
			id:   ids.AgentID{Role: "witness", Rig: "gastown"},
			want: "gt-gastown-witness",
		},
		{
			name: "refinery",
			id:   ids.AgentID{Role: "refinery", Rig: "my-project"},
			want: "gt-my-project-refinery",
		},
		{
			name: "crew",
			id:   ids.AgentID{Role: "crew", Rig: "gastown", Worker: "max"},
			want: "gt-gastown-crew-max",
		},
		{
			name: "polecat",
			id:   ids.AgentID{Role: "polecat", Rig: "gastown", Worker: "morsov"},
			want: "gt-gastown-morsov",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SessionNameFromAgentID(tt.id); got != tt.want {
				t.Errorf("SessionNameFromAgentID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentID_String(t *testing.T) {
	tests := []struct {
		name string
		id   ids.AgentID
		want string
	}{
		{
			name: "mayor",
			id:   ids.AgentID{Role: "mayor"},
			want: "mayor",
		},
		{
			name: "deacon",
			id:   ids.AgentID{Role: "deacon"},
			want: "deacon",
		},
		{
			name: "witness",
			id:   ids.AgentID{Role: "witness", Rig: "gastown"},
			want: "gastown/witness",
		},
		{
			name: "refinery",
			id:   ids.AgentID{Role: "refinery", Rig: "my-project"},
			want: "my-project/refinery",
		},
		{
			name: "crew",
			id:   ids.AgentID{Role: "crew", Rig: "gastown", Worker: "max"},
			want: "gastown/crew/max",
		},
		{
			name: "polecat",
			id:   ids.AgentID{Role: "polecat", Rig: "gastown", Worker: "Toast"},
			want: "gastown/polecat/Toast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.String(); got != tt.want {
				t.Errorf("AgentID.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSessionName_RoundTrip(t *testing.T) {
	// Test that parsing then reconstructing gives the same result
	sessions := []string{
		// Town-level
		"hq-mayor",
		"hq-deacon",
		// Witness with simple and hyphenated rig names
		"gt-gastown-witness",
		"gt-foo-bar-witness",
		"gt-a-b-c-witness",
		// Refinery with hyphenated rig
		"gt-foo-bar-refinery",
		// Crew with hyphenated rig and/or worker name
		"gt-gastown-crew-max",
		"gt-foo-bar-crew-alice",
		"gt-gastown-crew-joe-bob",
		"gt-foo-bar-crew-alice-bob",
		// Polecat with simple and hyphenated rig names
		"gt-gastown-morsov",
		"gt-foo-bar-Toast",
	}

	for _, sess := range sessions {
		t.Run(sess, func(t *testing.T) {
			id, err := ParseSessionName(sess)
			if err != nil {
				t.Fatalf("ParseSessionName(%q) error = %v", sess, err)
			}
			if got := SessionNameFromAgentID(id); got != sess {
				t.Errorf("Round-trip failed: ParseSessionName(%q) -> SessionNameFromAgentID() = %q", sess, got)
			}
		})
	}
}

func TestParseSessionName_KnownLimitations(t *testing.T) {
	// Document known limitations where parsing is ambiguous.
	// A polecat named "witness" or "refinery" cannot be distinguished from the
	// rig-level singleton agents without additional context.

	// This WILL be parsed as witness, not polecat - this is a known limitation
	sess := "gt-gastown-witness"
	id, _ := ParseSessionName(sess)
	if id.Role != "witness" {
		t.Errorf("Expected role=witness (known limitation: polecat named 'witness' is indistinguishable)")
	}

	// Similarly for refinery
	sess = "gt-gastown-refinery"
	id, _ = ParseSessionName(sess)
	if id.Role != "refinery" {
		t.Errorf("Expected role=refinery (known limitation: polecat named 'refinery' is indistinguishable)")
	}
}
