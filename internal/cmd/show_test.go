package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestResolveBeadDir_UsesRoutesForNonHQPrefix(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", "rig"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0755); err != nil {
		t.Fatal(err)
	}
	routes := []byte("{\"prefix\":\"gs-\",\"path\":\"gastown/mayor/rig\"}\n{\"prefix\":\"hq-\",\"path\":\".\"}\n")
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), routes, 0644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(townRoot); err != nil {
		t.Fatal(err)
	}

	if got := resolveBeadDir("gs-60j"); got != filepath.Join(townRoot, "gastown", "mayor", "rig") {
		t.Fatalf("resolveBeadDir(gs-60j) = %q, want %q", got, filepath.Join(townRoot, "gastown", "mayor", "rig"))
	}
	if got := resolveBeadDir("hq-123"); got != townRoot {
		t.Fatalf("resolveBeadDir(hq-123) = %q, want %q", got, townRoot)
	}
}
