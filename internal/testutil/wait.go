package testutil

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

func WaitForSession(t *testing.T, sessionName string, timeout time.Duration) bool {
	t.Helper()
	tm := tmux.NewTmux()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if exists, _ := tm.HasSession(sessionName); exists {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func WaitForPaneCommand(t *testing.T, sessionName string, expected []string, timeout time.Duration) bool {
	t.Helper()
	tm := tmux.NewTmux()
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if exists, _ := tm.HasSession(sessionName); exists {
			cmd, _ := tm.GetPaneCommand(sessionName)
			for _, exp := range expected {
				if cmd == exp {
					return true
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func LogDiagnostic(t *testing.T, sessionName string) {
	t.Helper()
	tm := tmux.NewTmux()

	exists, _ := tm.HasSession(sessionName)
	if !exists {
		t.Logf("Session %s does not exist", sessionName)
		return
	}

	paneCmd, _ := tm.GetPaneCommand(sessionName)
	panePid, _ := tm.GetPanePID(sessionName)
	paneContent, _ := tm.CapturePane(sessionName, 20)

	t.Logf("Session: %s, Cmd: %q, PID: %s", sessionName, paneCmd, panePid)
	t.Logf("Content:\n%s", paneContent)

	cmd := exec.Command("pgrep", "-P", panePid, "-l")
	if out, err := cmd.Output(); err == nil {
		t.Logf("Children: %s", string(out))
	}
}

func Truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > n {
		return s[:n]
	}
	return s
}
