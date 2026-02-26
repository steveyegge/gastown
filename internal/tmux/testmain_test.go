package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testSocketName is the package-level tmux socket used by all tests in this
// package. Set by TestMain before any tests run, cleaned up after all tests
// finish. Using a single socket per package run avoids cross-package
// interference while keeping per-test overhead near zero.
var testSocketName string

func TestMain(m *testing.M) {
	// Skip tmux socket setup if tmux isn't installed — tests that need it
	// will skip individually via hasTmux().
	if _, err := exec.LookPath("tmux"); err != nil {
		os.Exit(m.Run())
	}

	testSocketName = fmt.Sprintf("gt-test-tmux-%d", os.Getpid())
	SetDefaultSocket(testSocketName)

	code := m.Run()

	// Tear down: kill the tmux server and remove the socket file.
	_ = exec.Command("tmux", "-L", testSocketName, "kill-server").Run()
	socketPath := filepath.Join(fmt.Sprintf("/tmp/tmux-%d", os.Getuid()), testSocketName)
	_ = os.Remove(socketPath)

	os.Exit(code)
}
