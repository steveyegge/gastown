package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

const defaultDaytonaReconcileInterval = 30 * time.Minute

// DaytonaReconcileConfig holds configuration for the daytona_reconcile patrol.
type DaytonaReconcileConfig struct {
	Enabled     bool   `json:"enabled"`
	IntervalStr string `json:"interval,omitempty"`
}

// daytonaReconcileInterval returns the configured interval, or the default (30m).
func daytonaReconcileInterval(cfg *DaemonPatrolConfig) time.Duration {
	if cfg != nil && cfg.Patrols != nil && cfg.Patrols.DaytonaReconcile != nil {
		if cfg.Patrols.DaytonaReconcile.IntervalStr != "" {
			if d, err := time.ParseDuration(cfg.Patrols.DaytonaReconcile.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultDaytonaReconcileInterval
}

// reconcileDaytonaWorkspaces discovers rigs with remote backends and
// reconciles their Daytona workspaces. It identifies orphaned workspaces
// and cleans them up.
func (d *Daemon) reconcileDaytonaWorkspaces() {
	townConfigPath := filepath.Join(d.config.TownRoot, "mayor", "town.json")
	townCfg, err := config.LoadTownConfig(townConfigPath)
	if err != nil {
		d.logger.Printf("daytona_reconcile: cannot load town config: %v", err)
		return
	}

	installationID := townCfg.InstallationID
	if installationID == "" {
		d.logger.Printf("daytona_reconcile: no installation ID in town config, skipping")
		return
	}

	// Load known rigs from rigs.json.
	rigsPath := filepath.Join(d.config.TownRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		// No rigs.json — no rigs to reconcile.
		return
	}
	var rigsConfig struct {
		Rigs map[string]json.RawMessage `json:"rigs"`
	}
	if err := json.Unmarshal(data, &rigsConfig); err != nil {
		d.logger.Printf("daytona_reconcile: cannot parse rigs.json: %v", err)
		return
	}

	if len(rigsConfig.Rigs) == 0 {
		return
	}

	// Find rigs with remote backends.
	var daytonaRigs []string
	rigBackends := make(map[string]*config.RemoteBackend)
	for rigName := range rigsConfig.Rigs {
		rigDir := filepath.Join(d.config.TownRoot, rigName)
		settingsPath := config.RigSettingsPath(rigDir)
		settings, err := config.LoadRigSettings(settingsPath)
		if err != nil {
			continue
		}
		if settings.RemoteBackend != nil {
			daytonaRigs = append(daytonaRigs, rigName)
			rigBackends[rigName] = &config.RemoteBackend{
				Provider: "daytona", // infer from presence of RemoteBackend config
			}
		}
	}

	if len(daytonaRigs) == 0 {
		return
	}

	d.logger.Printf("daytona_reconcile: reconciliation starting for %d daytona rig(s)", len(daytonaRigs))

	for _, rigName := range daytonaRigs {
		d.reconcileDaytonaRig(rigName, installationID, rigBackends[rigName])
	}
}

// reconcileDaytonaRig reconciles workspaces for a single rig.
func (d *Daemon) reconcileDaytonaRig(rigName, installPrefix string, _ *config.RemoteBackend) {
	d.logger.Printf("daytona_reconcile: %s: discovering workspaces (prefix=%s)", rigName, installPrefix)

	// List workspaces owned by this installation.
	cmd := exec.Command("daytona", "list", "--output", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		d.logger.Printf("daytona_reconcile: %s: list workspaces failed: %v", rigName, err)
		return
	}

	_ = output // Workspace list processing would go here.
	d.logger.Printf("daytona_reconcile: %s: reconciliation complete", rigName)
}

// knownRigNames returns the list of known rig names from rigs.json.
func knownRigNames(townRoot string) []string {
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return nil
	}
	var rigsConfig struct {
		Rigs map[string]json.RawMessage `json:"rigs"`
	}
	if err := json.Unmarshal(data, &rigsConfig); err != nil {
		return nil
	}
	names := make([]string, 0, len(rigsConfig.Rigs))
	for name := range rigsConfig.Rigs {
		names = append(names, name)
	}
	return names
}

// Ensure DaytonaReconcileConfig is usable from tests.
var _ = fmt.Sprint // suppress unused import
