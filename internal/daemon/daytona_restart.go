package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
)

// daytonaWorkspace represents a workspace entry from "daytona list --output json".
type daytonaWorkspace struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// buildDaytonaExecCommand constructs a tmux pane command string for running
// an agent inside a Daytona workspace via `daytona exec`.
func (d *Daemon) buildDaytonaExecCommand(wsName string, envVars map[string]string, rc *config.RuntimeConfig) string {
	var parts []string
	parts = append(parts, "exec", "daytona", "exec", wsName)

	// Sort env keys for deterministic output.
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		parts = append(parts, "--env", fmt.Sprintf("%s=%s", k, envVars[k]))
	}

	parts = append(parts, "--")
	parts = append(parts, rc.Command)
	parts = append(parts, rc.Args...)

	return strings.Join(parts, " ")
}

// restartPolecatSession restarts a polecat session, delegating to the
// daytona restart path if the rig has a remote backend configured.
func (d *Daemon) restartPolecatSession(rigName, polecatName, sessionName string) error {
	rigDir := filepath.Join(d.config.TownRoot, rigName)
	settingsPath := config.RigSettingsPath(rigDir)
	settings, err := config.LoadRigSettings(settingsPath)
	if err == nil && settings.RemoteBackend != nil {
		return d.restartDaytonaPolecatSession(rigName, polecatName, sessionName, rigDir)
	}

	// Local restart path: check that worktree exists.
	worktree := filepath.Join(rigDir, "polecats", polecatName)
	if _, err := os.Stat(worktree); err != nil {
		return fmt.Errorf("worktree does not exist: %s", worktree)
	}
	return nil
}

// isPolecatDaytona checks whether a polecat runs on a Daytona workspace
// by querying the agent bead for sandbox metadata.
func (d *Daemon) isPolecatDaytona(rigName, polecatName string) (bool, error) {
	cmd := exec.Command(d.bdPath, "show", fmt.Sprintf("%s-polecat-%s", rigName, polecatName))
	cmd.Dir = d.config.TownRoot
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("bd show failed: %w", err)
	}
	return true, nil
}

// restartDaytonaPolecatSession restarts a polecat that runs inside a Daytona
// workspace. It loads the town config to get the InstallationID, then attempts
// to interact with the daytona CLI.
func (d *Daemon) restartDaytonaPolecatSession(rigName, polecatName, _ /* sessionName */, rigDir string) error {
	townConfigPath := filepath.Join(d.config.TownRoot, "mayor", "town.json")
	townCfg, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		return fmt.Errorf("daytona restart: cannot load town config: %w", err)
	}

	if townCfg.InstallationID == "" {
		return fmt.Errorf("daytona restart: InstallationID is empty")
	}

	// Derive workspace name from installation prefix.
	installPrefix := townCfg.InstallationID
	if len(installPrefix) > 12 {
		installPrefix = installPrefix[:12]
	}
	wsName := fmt.Sprintf("gt-%s-%s--%s", installPrefix, rigName, polecatName)

	// List workspaces to find ours.
	cmd := exec.Command("daytona", "list", "--output", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("daytona list failed for workspace %s: %w", wsName, err)
	}

	var workspaces []daytonaWorkspace
	if err := json.Unmarshal(output, &workspaces); err != nil {
		return fmt.Errorf("daytona restart: cannot parse workspace list: %w", err)
	}

	// Find workspace by name.
	var found *daytonaWorkspace
	for i := range workspaces {
		if workspaces[i].Name == wsName {
			found = &workspaces[i]
			break
		}
	}

	if found == nil {
		// Workspace not found — likely auto-deleted.
		d.logger.Printf("daytona restart: workspace %s not found in workspace list (auto-deleted?)", wsName)
		d.cleanupAutoDeletedWorkspace(rigName, polecatName, rigDir)
		return fmt.Errorf("daytona restart: workspace %s has been deleted; re-dispatch the polecat", wsName)
	}

	// Handle workspace state.
	switch found.State {
	case "running":
		// Ready to use.
		return nil
	case "stopped":
		d.logger.Printf("Starting stopped daytona workspace %s", wsName)
		startCmd := exec.Command("daytona", "start", wsName)
		if startOut, startErr := startCmd.CombinedOutput(); startErr != nil {
			return fmt.Errorf("error starting daytona workspace %s: %w (%s)", wsName, startErr, strings.TrimSpace(string(startOut)))
		}
		d.logger.Printf("Daytona workspace %s started successfully", wsName)
		return nil
	case "creating", "starting":
		return fmt.Errorf("daytona restart: workspace %s is in transitional state %q; retry later", wsName, found.State)
	case "stopping":
		return fmt.Errorf("daytona restart: workspace %s is stopping; retry after it has stopped", wsName)
	case "error":
		return fmt.Errorf("daytona restart: workspace %s is in error state; manual intervention required", wsName)
	default:
		return fmt.Errorf("daytona restart: workspace %s has unknown state %q", wsName, found.State)
	}
}

// cleanupAutoDeletedWorkspace performs best-effort cleanup when a Daytona
// workspace has been auto-deleted. Closes the agent bead and removes
// any orphaned state.
func (d *Daemon) cleanupAutoDeletedWorkspace(rigName, polecatName, _ /* rigDir */ string) {
	// Compute agent bead ID.
	prefix := config.GetRigPrefix(d.config.TownRoot, rigName)
	if prefix == "" {
		prefix = rigName[:3] // fallback
	}
	agentBeadID := fmt.Sprintf("%s-%s-polecat-%s", prefix, rigName, polecatName)
	d.logger.Printf("cleanupAutoDeletedWorkspace: closing agent bead %s", agentBeadID)

	// Close the agent bead (best-effort).
	cmd := exec.CommandContext(d.ctx, d.bdPath, "close", agentBeadID, "--reason", "workspace auto-deleted")
	cmd.Dir = d.config.TownRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		d.logger.Printf("Warning: failed to close agent bead %s: %v (%s)", agentBeadID, err, strings.TrimSpace(string(output)))
	}

	// Kill any orphaned tmux session.
	sessionName := fmt.Sprintf("%s-%s", rigName, polecatName)
	killCmd := exec.CommandContext(d.ctx, "tmux", "kill-session", "-t", sessionName)
	if output, err := killCmd.CombinedOutput(); err != nil {
		d.logger.Printf("Warning: failed to kill session %s: %v (%s)", sessionName, err, strings.TrimSpace(string(output)))
	}

	d.logger.Printf("Cleaned up auto-deleted workspace for %s/%s (agent bead: %s)", rigName, polecatName, agentBeadID)
}
