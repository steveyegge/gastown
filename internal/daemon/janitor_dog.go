package daemon

import "time"

const defaultJanitorDogInterval = 15 * time.Minute

// JanitorDogConfig holds configuration for the janitor_dog patrol.
type JanitorDogConfig struct {
	Enabled     bool   `json:"enabled"`
	IntervalStr string `json:"interval,omitempty"`
}

// janitorDogInterval returns the configured interval, or the default (15m).
func janitorDogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.JanitorDog != nil {
		if config.Patrols.JanitorDog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.JanitorDog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultJanitorDogInterval
}

// runJanitorDog pours a janitor molecule for agent execution.
// The formula (mol-dog-janitor) describes the cleanup steps declaratively.
// An agent interprets and executes them — no imperative Go logic here.
func (d *Daemon) runJanitorDog() {
	if !IsPatrolEnabled(d.patrolConfig, "janitor_dog") {
		return
	}
	d.logger.Printf("janitor_dog: pouring molecule")
	mol := d.pourDogMolecule("mol-dog-janitor", nil)
	defer mol.close()
}
