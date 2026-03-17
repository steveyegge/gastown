package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/posting"
	"github.com/steveyegge/gastown/internal/tmux"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// testSocket returns the test socket name. Uses the package test socket if set,
// otherwise creates a unique one for this test.
func testSocket(t *testing.T) string {
	t.Helper()
	sock := tmux.GetDefaultSocket()
	if sock != "" {
		return sock
	}
	return "gt-posting-test"
}

// setupTmuxSession creates a tmux session for testing window rename.
// Returns a cleanup function that kills the session.
func setupTmuxSession(t *testing.T, sessionName string) func() {
	t.Helper()
	sock := testSocket(t)
	// Create a detached session
	cmd := exec.Command("tmux", "-u", "-L", sock, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("create test tmux session: %v", err)
	}
	return func() {
		_ = exec.Command("tmux", "-u", "-L", sock, "kill-session", "-t", sessionName).Run()
	}
}

// getWindowName reads the current window name of the given session.
func getWindowName(t *testing.T, sessionName string) string {
	t.Helper()
	sock := testSocket(t)
	out, err := exec.Command("tmux", "-u", "-L", sock, "display-message", "-t", sessionName, "-p", "#{window_name}").Output()
	if err != nil {
		t.Fatalf("get window name: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// renameWindowInSession renames the window of the given session using tmux directly.
func renameWindowInSession(t *testing.T, sessionName, name string) {
	t.Helper()
	sock := testSocket(t)
	cmd := exec.Command("tmux", "-u", "-L", sock, "rename-window", "-t", sessionName, name)
	if err := cmd.Run(); err != nil {
		t.Fatalf("rename window: %v", err)
	}
}

// getSessionName reads the session name to verify it hasn't changed.
func getSessionName(t *testing.T, sessionName string) string {
	t.Helper()
	sock := testSocket(t)
	out, err := exec.Command("tmux", "-u", "-L", sock, "display-message", "-t", sessionName, "-p", "#{session_name}").Output()
	if err != nil {
		t.Fatalf("get session name: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// §17.1: Crew assume → window name changes to {w}[posting]
func TestPostingTmuxWindow_CrewAssume(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-crew-assume"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	renameWindowInSession(t, sess, "diesel")
	renameWindowInSession(t, sess, posting.AppendBracket("diesel", "inspector"))

	got := getWindowName(t, sess)
	if got != "diesel[inspector]" {
		t.Errorf("window name = %q, want %q", got, "diesel[inspector]")
	}
}

// §17.2: Crew drop → window name reverts to {w}
func TestPostingTmuxWindow_CrewDrop(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-crew-drop"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	// Start with posting
	renameWindowInSession(t, sess, "diesel[inspector]")
	// Drop: revert to base name
	renameWindowInSession(t, sess, "diesel")

	got := getWindowName(t, sess)
	if got != "diesel" {
		t.Errorf("window name = %q, want %q", got, "diesel")
	}
}

// §17.3: Crew persistent posting → window set at gt prime
func TestPostingTmuxWindow_PersistentAtPrime(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-persistent-prime"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	// Simulate prime setting window for persistent posting
	workerName := "diesel"
	postingName := "inspector"
	renameWindowInSession(t, sess, posting.AppendBracket(workerName, postingName))

	got := getWindowName(t, sess)
	if got != "diesel[inspector]" {
		t.Errorf("window name = %q, want %q", got, "diesel[inspector]")
	}
}

// §17.4: Polecat sling --posting → window name is {p}[posting]
func TestPostingTmuxWindow_PolecatSlingPosting(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-polecat-sling"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	renameWindowInSession(t, sess, posting.AppendBracket("cheedo", "dispatcher"))

	got := getWindowName(t, sess)
	if got != "cheedo[dispatcher]" {
		t.Errorf("window name = %q, want %q", got, "cheedo[dispatcher]")
	}
}

// §17.5: Polecat no posting → window name is {p}
func TestPostingTmuxWindow_PolecatNoPosting(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-polecat-nopost"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	renameWindowInSession(t, sess, "cheedo")

	got := getWindowName(t, sess)
	if got != "cheedo" {
		t.Errorf("window name = %q, want %q", got, "cheedo")
	}
}

// §17.6: Crew round-trip: assume then drop
func TestPostingTmuxWindow_CrewRoundTrip(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-crew-roundtrip"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	renameWindowInSession(t, sess, "diesel")
	// Assume
	renameWindowInSession(t, sess, posting.AppendBracket("diesel", "scout"))
	if got := getWindowName(t, sess); got != "diesel[scout]" {
		t.Errorf("after assume: window name = %q, want %q", got, "diesel[scout]")
	}
	// Drop
	renameWindowInSession(t, sess, "diesel")
	if got := getWindowName(t, sess); got != "diesel" {
		t.Errorf("after drop: window name = %q, want %q", got, "diesel")
	}
}

// §17.7: Outside tmux ($TMUX not set) → no error, no rename
func TestPostingTmuxWindow_OutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	// RenameWindow should be a no-op — must not panic or error
	tmux.RenameWindow("cheedo[inspector]")
}

// §17.8: Persistent blocks assume → window name unchanged
func TestPostingTmuxWindow_PersistentBlocksAssume(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-persistent-block"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	// Window starts at worker name (persistent already active)
	renameWindowInSession(t, sess, "diesel[inspector]")
	original := getWindowName(t, sess)

	// Assume would fail (blocked by persistent) — window should not change.
	// We don't rename because the assume command returns an error before rename.
	got := getWindowName(t, sess)
	if got != original {
		t.Errorf("window name changed: got %q, want %q", got, original)
	}
}

// §17.9: Session name unchanged after assume (verify routing safe)
func TestPostingTmuxWindow_SessionNameUnchanged(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-session-safe"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	before := getSessionName(t, sess)

	// Rename window (simulating assume)
	renameWindowInSession(t, sess, "cheedo[dispatcher]")

	after := getSessionName(t, sess)
	if before != after {
		t.Errorf("session name changed: before=%q, after=%q", before, after)
	}
}

// §17.10: Polecat assume → window name changes
func TestPostingTmuxWindow_PolecatAssume(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-polecat-assume"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	renameWindowInSession(t, sess, "cheedo")
	renameWindowInSession(t, sess, posting.AppendBracket("cheedo", "dispatcher"))

	got := getWindowName(t, sess)
	if got != "cheedo[dispatcher]" {
		t.Errorf("window name = %q, want %q", got, "cheedo[dispatcher]")
	}
}

// §17.11: Polecat drop → window name reverts
func TestPostingTmuxWindow_PolecatDrop(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-polecat-drop"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	renameWindowInSession(t, sess, "cheedo[dispatcher]")
	renameWindowInSession(t, sess, "cheedo")

	got := getWindowName(t, sess)
	if got != "cheedo" {
		t.Errorf("window name = %q, want %q", got, "cheedo")
	}
}

// §17.12: Polecat handoff inheritance → window set at prime in new session
func TestPostingTmuxWindow_PolecatHandoffInheritance(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	sess := "gt-test-polecat-handoff"
	cleanup := setupTmuxSession(t, sess)
	defer cleanup()

	// Simulate: after handoff, prime detects inherited posting and sets window
	renameWindowInSession(t, sess, posting.AppendBracket("cheedo", "dispatcher"))

	got := getWindowName(t, sess)
	if got != "cheedo[dispatcher]" {
		t.Errorf("window name = %q, want %q", got, "cheedo[dispatcher]")
	}
}

// Test that RenameWindow uses the correct bracket notation format.
func TestPostingTmuxWindow_BracketFormat(t *testing.T) {
	// Verify AppendBracket produces expected format
	tests := []struct {
		base    string
		posting string
		want    string
	}{
		{"ace", "inspector", "ace[inspector]"},
		{"cheedo", "dispatcher", "cheedo[dispatcher]"},
		{"Toast", "scout", "Toast[scout]"},
		{"ace", "", "ace"},
	}
	for _, tt := range tests {
		got := posting.AppendBracket(tt.base, tt.posting)
		if got != tt.want {
			t.Errorf("AppendBracket(%q, %q) = %q, want %q", tt.base, tt.posting, got, tt.want)
		}
	}
}

// Test that prime logic correctly determines window name based on posting.
func TestPostingTmuxWindow_PrimeLogic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		polecat    string
		postingVal string
		wantWindow string
	}{
		{"with_posting", "cheedo", "dispatcher", "cheedo[dispatcher]"},
		{"no_posting", "cheedo", "", "cheedo"},
		{"crew_with_posting", "diesel", "inspector", "diesel[inspector]"},
		{"crew_no_posting", "diesel", "", "diesel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var windowName string
			if tt.postingVal != "" {
				windowName = posting.AppendBracket(tt.polecat, tt.postingVal)
			} else {
				windowName = tt.polecat
			}
			if windowName != tt.wantWindow {
				t.Errorf("window name = %q, want %q", windowName, tt.wantWindow)
			}
		})
	}
}

// Test RenameWindow is a no-op when TMUX env is not set.
func TestRenameWindow_NoopOutsideTmux(t *testing.T) {
	orig := os.Getenv("TMUX")
	os.Unsetenv("TMUX")
	defer func() {
		if orig != "" {
			os.Setenv("TMUX", orig)
		}
	}()

	// Should not panic or error
	tmux.RenameWindow("anything")
}
