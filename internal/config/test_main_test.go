package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	// Isolate tmux sessions on a package-specific socket.
	// We can't call tmux.SetDefaultSocket here due to import cycle
	// (config ← tmux), so we set the package-level testTmuxSocket variable
	// defined in integration_test.go instead.
	if _, err := exec.LookPath("tmux"); err == nil {
		testTmuxSocket = fmt.Sprintf("gt-test-config-%d", os.Getpid())
	}

	stubDir, err := os.MkdirTemp("", "gt-agent-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create stub dir: %v\n", err)
		os.Exit(1)
	}

	stub := []byte("#!/bin/sh\nexit 0\n")
	binaries := []string{
		"claude",
		"gemini",
		"codex",
		"cursor-agent",
		"auggie",
		"amp",
		"opencode",
	}
	for _, name := range binaries {
		path := filepath.Join(stubDir, name)
		if err := os.WriteFile(path, stub, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "write stub %s: %v\n", name, err)
			os.Exit(1)
		}
	}

	originalPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", stubDir+string(os.PathListSeparator)+originalPath)

	code := m.Run()

	_ = os.Setenv("PATH", originalPath)
	_ = os.RemoveAll(stubDir)
	// Clean up tmux socket
	if testTmuxSocket != "" {
		_ = exec.Command("tmux", "-L", testTmuxSocket, "kill-server").Run()
		socketPath := filepath.Join(fmt.Sprintf("/tmp/tmux-%d", os.Getuid()), testTmuxSocket)
		_ = os.Remove(socketPath)
	}
	os.Exit(code)
}
