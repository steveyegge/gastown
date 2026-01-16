package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runRigSetupCommand(repoRoot, command, workdir string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}

	dir := repoRoot
	if workdir != "" {
		clean := filepath.Clean(workdir)
		if strings.HasPrefix(clean, "..") {
			return fmt.Errorf("setup workdir must be within repo")
		}
		dir = filepath.Join(repoRoot, clean)
	}

	cmd := exec.Command("sh", "-c", command) //nolint:gosec // command is from manifest/preset
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("setup command failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func copyRigSettings(repoRoot, rigPath, settingsRelPath string) (bool, error) {
	if strings.TrimSpace(settingsRelPath) == "" {
		return false, nil
	}

	clean := filepath.Clean(settingsRelPath)
	if strings.HasPrefix(clean, "..") {
		return false, fmt.Errorf("settings path must be within repo")
	}

	source := filepath.Join(repoRoot, clean)
	data, err := os.ReadFile(source)
	if err != nil {
		return false, fmt.Errorf("reading settings source: %w", err)
	}

	destDir := filepath.Join(rigPath, "settings")
	destPath := filepath.Join(destDir, "config.json")

	if _, err := os.Stat(destPath); err == nil {
		return false, nil
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return false, fmt.Errorf("creating settings dir: %w", err)
	}
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return false, fmt.Errorf("writing settings file: %w", err)
	}
	return true, nil
}
