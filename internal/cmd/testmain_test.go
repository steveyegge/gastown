//go:build !integration

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

func TestMain(m *testing.M) {
	// Set up an isolated tmux socket for this package's tests.
	// Tests that create tmux sessions (e.g., TestFindRigSessions) will use
	// this socket instead of the system default, preventing interference with
	// the user's interactive tmux and with other packages running in parallel.
	var tmuxSocket string
	if _, err := exec.LookPath("tmux"); err == nil {
		tmuxSocket = fmt.Sprintf("gt-test-cmd-%d", os.Getpid())
		tmux.SetDefaultSocket(tmuxSocket)
	}

	code := m.Run()

	// Clean up tmux socket. NOTE: os.Exit does NOT run deferred functions,
	// so cleanup must happen explicitly before os.Exit.
	if tmuxSocket != "" {
		_ = exec.Command("tmux", "-L", tmuxSocket, "kill-server").Run()
		socketPath := filepath.Join(fmt.Sprintf("/tmp/tmux-%d", os.Getuid()), tmuxSocket)
		_ = os.Remove(socketPath)
	}

	// Clean up the cached buildGT() test binary if it was created.
	binaryName := "gt-integration-test"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	_ = os.Remove(filepath.Join(os.TempDir(), binaryName))

	os.Exit(code)
}
