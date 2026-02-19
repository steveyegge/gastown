package tmuxinator

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// IsAvailable checks if tmuxinator is installed and accessible on PATH.
func IsAvailable() bool {
	_, err := exec.LookPath("tmuxinator")
	return err == nil
}

// Start runs tmuxinator start with the given config file path.
// The session is created in detached mode (--no-attach).
func Start(configPath string) error {
	cmd := exec.Command("tmuxinator", "start", "-p", configPath, "--no-attach")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmuxinator start: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// Stop runs tmuxinator stop for the given project name.
func Stop(name string) error {
	cmd := exec.Command("tmuxinator", "stop", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmuxinator stop: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return nil
}

// Version returns the installed tmuxinator version string.
func Version() (string, error) {
	cmd := exec.Command("tmuxinator", "version")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmuxinator version: %w", err)
	}
	return strings.TrimSpace(stdout.String()), nil
}
