package cmd

import "testing"

func TestExtractBeadIDFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"simple", []string{"myproject-abc"}, "myproject-abc"},
		{"with flags after", []string{"gt-abc123", "--json"}, "gt-abc123"},
		{"with flags before", []string{"--json", "hq-xyz"}, "hq-xyz"},
		{"flags only", []string{"--json", "-v"}, ""},
		{"empty", []string{}, ""},
		{"mixed", []string{"-v", "bd-def456", "--json"}, "bd-def456"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractBeadIDFromArgs(tc.args)
			if got != tc.want {
				t.Errorf("extractBeadIDFromArgs(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestStripEnvKey(t *testing.T) {
	env := []string{"PATH=/usr/bin", "BEADS_DIR=/town/.beads", "HOME=/home/user", "BEADS_DIR=/other"}
	got := stripEnvKey(env, "BEADS_DIR")

	for _, e := range got {
		if e == "BEADS_DIR=/town/.beads" || e == "BEADS_DIR=/other" {
			t.Errorf("BEADS_DIR should be stripped, found: %s", e)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries after stripping, got %d", len(got))
	}
}

func TestStripEnvKey_NoMatch(t *testing.T) {
	env := []string{"PATH=/usr/bin", "HOME=/home/user"}
	got := stripEnvKey(env, "BEADS_DIR")
	if len(got) != 2 {
		t.Errorf("expected 2 entries (no change), got %d", len(got))
	}
}
