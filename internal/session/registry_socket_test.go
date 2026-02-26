package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

// TestInitRegistry_SocketFromTownName verifies that InitRegistry derives the
// tmux socket name from the town directory name, NOT from $TMUX.
//
// This is the regression test for gt-qkekp: when gt up is run from a terminal
// attached to the default tmux server ($TMUX=/tmp/tmux-1000/default,...),
// InitRegistry used to parse "default" from $TMUX and set that as the socket.
// This caused all sessions to land on the default server instead of a
// town-specific socket (e.g., -L gt).
func TestInitRegistry_SocketFromTownName(t *testing.T) {
	// Save and restore $TMUX and the default socket
	origTMUX := os.Getenv("TMUX")
	origSocket := tmux.GetDefaultSocket()
	t.Cleanup(func() {
		os.Setenv("TMUX", origTMUX)
		tmux.SetDefaultSocket(origSocket)
	})

	tests := []struct {
		name       string
		tmuxEnv    string // $TMUX value (simulating being inside tmux)
		townDir    string // basename of the town root directory
		wantSocket string // expected tmux socket name
	}{
		{
			name:       "inside default tmux, town=gt",
			tmuxEnv:    "/tmp/tmux-1000/default,12345,0",
			townDir:    "gt",
			wantSocket: "gt",
		},
		{
			name:       "inside gt tmux, town=gt",
			tmuxEnv:    "/tmp/tmux-1000/gt,12345,0",
			townDir:    "gt",
			wantSocket: "gt",
		},
		{
			name:       "outside tmux (daemon), town=gt",
			tmuxEnv:    "",
			townDir:    "gt",
			wantSocket: "gt",
		},
		{
			name:       "town name with spaces",
			tmuxEnv:    "/tmp/tmux-1000/default,99,0",
			townDir:    "My Town",
			wantSocket: "my-town",
		},
		{
			name:       "town name with caps",
			tmuxEnv:    "",
			townDir:    "GasTown",
			wantSocket: "gastown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset socket before each test
			tmux.SetDefaultSocket("")

			// Set $TMUX to simulate the terminal environment
			if tt.tmuxEnv != "" {
				os.Setenv("TMUX", tt.tmuxEnv)
			} else {
				os.Unsetenv("TMUX")
			}

			// Create a minimal fake town root. InitRegistry will fail to load
			// rigs.json and agents.json but that's fine — we only care about
			// the socket name it sets.
			townRoot := filepath.Join(t.TempDir(), tt.townDir)
			os.MkdirAll(townRoot, 0o755)

			// InitRegistry may return errors for missing config — ignore them.
			// The socket is set unconditionally before any config loading.
			_ = InitRegistry(townRoot)

			got := tmux.GetDefaultSocket()
			if got != tt.wantSocket {
				t.Errorf("after InitRegistry(%q) with TMUX=%q:\n  socket = %q, want %q",
					townRoot, tt.tmuxEnv, got, tt.wantSocket)
			}
		})
	}
}

func TestSanitizeTownName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mytown", "mytown"},
		{"MyTown", "mytown"},
		{"my town", "my-town"},
		{"my_town!", "my-town"},
		{"  spaces  ", "spaces"},
		{"My-Town-123", "my-town-123"},
		{"café", "caf"},
		{"", "default"},
		{"!!!!", "default"},
		{"a/b/c", "a-b-c"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeTownName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTownName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
