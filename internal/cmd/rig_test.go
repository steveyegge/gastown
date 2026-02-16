package cmd

import (
	"os/exec"
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
		{"@host:path", false},    // empty user
		{"user@:/path", false},   // empty host
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
	reg.Register("tr", "testrig1223")
	reg.Register("or", "otherrig")
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

	// Create sessions that match our test rig prefix (tr- for testrig1223)
	matching := []string{
		"tr-witness",
		"tr-refinery",
		"tr-alpha",
	}
	// Create a non-matching session (or- for otherrig)
	nonMatching := "or-witness"

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
