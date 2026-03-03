package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

// TestInitRegistry_SocketFromTownName verifies GT_TMUX_SOCKET socket selection:
//   - unset / "default" → "default" socket (backward-compatible)
//   - "auto"            → socket derived from town directory name
//   - explicit value    → that value verbatim
func TestInitRegistry_SocketFromTownName(t *testing.T) {
	origTMUX := os.Getenv("TMUX")
	origSocket := tmux.GetDefaultSocket()
	origGTSocket := os.Getenv("GT_TMUX_SOCKET")
	t.Cleanup(func() {
		os.Setenv("TMUX", origTMUX)
		os.Setenv("GT_TMUX_SOCKET", origGTSocket)
		tmux.SetDefaultSocket(origSocket)
	})

	tests := []struct {
		name        string
		gtTmuxSocket string // GT_TMUX_SOCKET value ("" = unset)
		tmuxEnv     string  // $TMUX value
		townDir     string  // basename of the town root directory
		wantSocket  string  // expected tmux socket name
	}{
		{
			name:        "unset → default (backward compat)",
			gtTmuxSocket: "",
			townDir:     "gt",
			wantSocket:  "default",
		},
		{
			name:        "explicit default → default",
			gtTmuxSocket: "default",
			townDir:     "gt",
			wantSocket:  "default",
		},
		{
			name:        "auto → town name",
			gtTmuxSocket: "auto",
			townDir:     "gt",
			wantSocket:  "gt",
		},
		{
			name:        "auto → sanitized town name with spaces",
			gtTmuxSocket: "auto",
			townDir:     "My Town",
			wantSocket:  "my-town",
		},
		{
			name:        "auto → sanitized town name with caps",
			gtTmuxSocket: "auto",
			townDir:     "GasTown",
			wantSocket:  "gastown",
		},
		{
			name:        "explicit custom socket name",
			gtTmuxSocket: "mysocket",
			townDir:     "gt",
			wantSocket:  "mysocket",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmux.SetDefaultSocket("")

			if tt.gtTmuxSocket != "" {
				os.Setenv("GT_TMUX_SOCKET", tt.gtTmuxSocket)
			} else {
				os.Unsetenv("GT_TMUX_SOCKET")
			}
			if tt.tmuxEnv != "" {
				os.Setenv("TMUX", tt.tmuxEnv)
			} else {
				os.Unsetenv("TMUX")
			}

			townRoot := filepath.Join(t.TempDir(), tt.townDir)
			os.MkdirAll(townRoot, 0o755)
			_ = InitRegistry(townRoot)

			got := tmux.GetDefaultSocket()
			if got != tt.wantSocket {
				t.Errorf("after InitRegistry(%q) with GT_TMUX_SOCKET=%q:\n  socket = %q, want %q",
					townRoot, tt.gtTmuxSocket, got, tt.wantSocket)
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
