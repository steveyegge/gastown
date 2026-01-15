package cmd

import "testing"

func TestIdentityToEmail(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		domain   string
		want     string
	}{
		{
			name:     "crew member",
			identity: "gastown/crew/jack",
			domain:   "gastown.local",
			want:     "gastown.crew.jack@gastown.local",
		},
		{
			name:     "polecat",
			identity: "gastown/polecats/max",
			domain:   "gastown.local",
			want:     "gastown.polecats.max@gastown.local",
		},
		{
			name:     "witness",
			identity: "gastown/witness",
			domain:   "gastown.local",
			want:     "gastown.witness@gastown.local",
		},
		{
			name:     "refinery",
			identity: "gastown/refinery",
			domain:   "gastown.local",
			want:     "gastown.refinery@gastown.local",
		},
		{
			name:     "mayor with trailing slash",
			identity: "mayor/",
			domain:   "gastown.local",
			want:     "mayor@gastown.local",
		},
		{
			name:     "deacon with trailing slash",
			identity: "deacon/",
			domain:   "gastown.local",
			want:     "deacon@gastown.local",
		},
		{
			name:     "custom domain",
			identity: "myrig/crew/alice",
			domain:   "example.com",
			want:     "myrig.crew.alice@example.com",
		},
		{
			name:     "deeply nested",
			identity: "rig/polecats/nested/deep",
			domain:   "test.io",
			want:     "rig.polecats.nested.deep@test.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := identityToEmail(tt.identity, tt.domain)
			if got != tt.want {
				t.Errorf("identityToEmail(%q, %q) = %q, want %q",
					tt.identity, tt.domain, got, tt.want)
			}
		})
	}
}
