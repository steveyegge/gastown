package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

func TestIsGitRemoteURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Remote URLs — should return true
		{"https://github.com/org/repo.git", true},
		{"http://github.com/org/repo.git", true},
		{"git@github.com:org/repo.git", true},
		{"ssh://git@github.com/org/repo.git", true},
		{"git://github.com/org/repo.git", true},
		{"deploy@private-host.internal:repos/app.git", true},

		// Local paths — should return false
		{"/Users/scott/projects/foo", false},
		{"/tmp/repo", false},
		{"./foo", false},
		{"../foo", false},
		{"~/projects/foo", false},
		{"C:\\Users\\scott\\projects\\foo", false},
		{"C:/Users/scott/projects/foo", false},

		// Bare directory name — should return false
		{"foo", false},

		// file:// URIs — should return false (local filesystem)
		{"file:///tmp/evil-repo", false},
		{"file:///Users/scott/projects/foo", false},
		{"file://user@localhost:/tmp/evil-repo", false},

		// Argument injection — should return false
		{"-oProxyCommand=evil", false},
		{"--upload-pack=touch /tmp/pwned", false},
		{"-c", false},

		// Malformed SCP-style — should return false
		{"@host:path", false},     // empty user
		{"user@:/path", false},    // empty host
		{"localhost:path", false}, // no user (not SCP-style)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isGitRemoteURL(tt.input)
			if got != tt.want {
				t.Errorf("isGitRemoteURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func setupRigTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	// Use zz-prefixed names to avoid collisions with real rig sessions
	// (e.g. "tr" collides with production rigs that use that prefix).
	reg.Register("zztr", "testrig1223")
	reg.Register("zzor", "otherrig")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestFindRigSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	setupRigTestRegistry(t)

	tm := tmux.NewTmux()

	// Create sessions that match our test rig prefix (zztr- for testrig1223)
	matching := []string{
		"zztr-witness",
		"zztr-refinery",
		"zztr-alpha",
	}
	// Create a non-matching session (zzor- for otherrig)
	nonMatching := "zzor-witness"

	for _, name := range append(matching, nonMatching) {
		_ = tm.KillSession(name) // clean up any leftovers
		if err := tm.NewSessionWithCommand(name, "", "sleep 300"); err != nil {
			t.Fatalf("creating session %s: %v", name, err)
		}
	}
	defer func() {
		for _, name := range append(matching, nonMatching) {
			_ = tm.KillSession(name)
		}
	}()

	got, err := findRigSessions(tm, "testrig1223")
	if err != nil {
		t.Fatalf("findRigSessions: %v", err)
	}

	// Verify all matching sessions are returned
	gotSet := make(map[string]bool, len(got))
	for _, s := range got {
		gotSet[s] = true
	}

	for _, want := range matching {
		if !gotSet[want] {
			t.Errorf("expected session %q in results, got %v", want, got)
		}
	}

	// Verify non-matching session is excluded
	if gotSet[nonMatching] {
		t.Errorf("did not expect session %q in results, got %v", nonMatching, got)
	}

	// Verify count
	if len(got) != len(matching) {
		t.Errorf("expected %d sessions, got %d: %v", len(matching), len(got), got)
	}
}

func TestFindRigSessions_NoSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	// Register a unique prefix for a rig that has no sessions
	reg := session.NewPrefixRegistry()
	reg.Register("zz", "nonexistentrig999")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	defer session.SetDefaultRegistry(old)

	tm := tmux.NewTmux()
	got, err := findRigSessions(tm, "nonexistentrig999")
	if err != nil {
		t.Fatalf("findRigSessions: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 sessions, got %d: %v", len(got), got)
	}
}

func TestIsStandardBeadHashStr(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Standard 5-char hashes — should return true
		{"mawit", true},
		{"z0ixd", true},
		{"a1b2c", true},
		{"00000", true},
		{"zzzzz", true},

		// Agent bead name suffixes — should return false
		{"nux", false},     // 3-char polecat name
		{"ace", false},     // 3-char polecat name
		{"max", false},     // 3-char polecat name
		{"witness", false}, // role name (7 chars)
		{"refinery", false}, // role name (8 chars)
		{"mayor", true},    // 5 lowercase chars — structurally valid hash (role names that happen to be 5 chars are OK)

		// Wrong length — should return false
		{"ab", false},          // too short
		{"abcdef", false},      // too long (6 chars)
		{"abcdefghij", false},  // merge request length (10 chars)

		// Contains uppercase — should return false
		{"ABCDE", false},
		{"Mawit", false},

		// Contains non-alphanumeric — should return false
		{"ab-cd", false},
		{"ab_cd", false},
		{"ab cd", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isStandardBeadHashStr(tt.input)
			if got != tt.want {
				t.Errorf("isStandardBeadHashStr(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// TestPrefixFromIssuesJSONL verifies that prefix detection from issues.jsonl
// correctly skips agent bead IDs (which have non-hash suffixes like 3-char names)
// and uses standard bead IDs to determine the prefix.
// This tests the hash-suffix filtering logic that guards isStandardBeadHashStr.
func TestPrefixFromIssuesJSONL(t *testing.T) {
	tests := []struct {
		name       string
		ids        []string // lines in issues.jsonl (in order)
		wantPrefix string   // expected detected prefix, "" means none detected
	}{
		{
			name:       "standard bead first",
			ids:        []string{"gt-mawit"},
			wantPrefix: "gt",
		},
		{
			name: "agent beads first then standard bead",
			ids: []string{
				"gt-gastown-polecat-nux", // 3-char name, should be skipped
				"gt-gastown-polecat-ace", // 3-char name, should be skipped
				"gt-gastown-witness",     // role name, should be skipped
				"gt-mawit",              // standard bead — use this
			},
			wantPrefix: "gt",
		},
		{
			name:       "only agent beads",
			ids:        []string{"gt-gastown-witness", "gt-gastown-polecat-nux"},
			wantPrefix: "", // nothing detected
		},
		{
			name:       "3-char polecat name nux",
			ids:        []string{"nx-nexus-polecat-nux", "nx-ab12c"},
			wantPrefix: "nx",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			beadsDir := filepath.Join(dir, ".beads")
			if err := os.MkdirAll(beadsDir, 0755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(beadsDir, "issues.jsonl")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range tt.ids {
				b, _ := json.Marshal(map[string]string{"id": id})
				f.Write(b)
				f.WriteString("\n")
			}
			f.Close()

			got := extractPrefixFromIssuesJSONL(path)
			if got != tt.wantPrefix {
				t.Errorf("extractPrefixFromIssuesJSONL: got %q, want %q", got, tt.wantPrefix)
			}
		})
	}
}
