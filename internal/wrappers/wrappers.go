// ABOUTME: Manages wrapper scripts for non-Claude agentic coding tools.
// ABOUTME: Provides gt-codex and gt-opencode wrappers that run gt prime first.

package wrappers

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed scripts/*
var scriptsFS embed.FS

func Install() error {
	binDir, err := binPath()
	if err != nil {
		return fmt.Errorf("determining bin directory: %w", err)
	}

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("creating bin directory: %w", err)
	}

	wrappers := []string{"gt-codex", "gt-opencode"}
	for _, name := range wrappers {
		content, err := scriptsFS.ReadFile("scripts/" + name)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", name, err)
		}

		destPath := filepath.Join(binDir, name)
		if err := os.WriteFile(destPath, content, 0755); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	return nil
}

func Remove() error {
	binDir, err := binPath()
	if err != nil {
		return err
	}

	wrappers := []string{"gt-codex", "gt-opencode"}
	for _, name := range wrappers {
		destPath := filepath.Join(binDir, name)
		if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", name, err)
		}
	}

	return nil
}

func binPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "bin"), nil
}

func BinDir() string {
	p, _ := binPath()
	return p
}
