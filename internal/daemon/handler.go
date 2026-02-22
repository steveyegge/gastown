package daemon

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/dog"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/plugin"
	"github.com/steveyegge/gastown/internal/tmux"
)

// Constants for idle dog reaping.
const (
	// dogIdleSessionTimeout is how long a dog can be idle with a live tmux
	// session before the session is killed.
	dogIdleSessionTimeout = 1 * time.Hour

	// dogIdleRemoveTimeout is how long a dog can be idle before it is removed
	// from the kennel entirely (only when pool is oversized).
	dogIdleRemoveTimeout = 4 * time.Hour

	// staleWorkingTimeout is how long a dog can be in state=working with no
	// activity updates before it is considered stuck. Covers the gap where
	// a tmux session is alive but sitting at an idle prompt.
	staleWorkingTimeout = 2 * time.Hour

	// maxDogPoolSize is the target pool size. Dogs idle beyond
	// dogIdleRemoveTimeout are removed when the pool exceeds this count.
	maxDogPoolSize = 4
)

// handleDogs manages Dog lifecycle: cleanup stuck dogs, reap idle dogs, then dispatch plugins.
// This is the main entry point called from heartbeat.
func (d *Daemon) handleDogs() {
	rigsConfig, err := d.loadRigsConfig()
	if err != nil {
		d.logger.Printf("Handler: failed to load rigs config: %v", err)
		return
	}

	mgr := dog.NewManager(d.config.TownRoot, rigsConfig)
	t := tmux.NewTmux()
	sm := dog.NewSessionManager(t, d.config.TownRoot, mgr)

	d.cleanupStuckDogs(mgr, sm)
	d.detectStaleWorkingDogs(mgr, sm)
	d.reapIdleDogs(mgr, sm)
	d.dispatchPlugins(mgr, sm, rigsConfig)
}

// cleanupStuckDogs finds dogs in state=working whose tmux session is dead and
// clears their work so they return to idle.
func (d *Daemon) cleanupStuckDogs(mgr *dog.Manager, sm *dog.SessionManager) {
	dogs, err := mgr.List()
	if err != nil {
		d.logger.Printf("Handler: failed to list dogs: %v", err)
		return
	}

	for _, dg := range dogs {
		if dg.State != dog.StateWorking {
			continue
		}

		running, err := sm.IsRunning(dg.Name)
		if err != nil {
			d.logger.Printf("Handler: error checking session for dog %s: %v", dg.Name, err)
			continue
		}

		if running {
			continue
		}

		// Dog is marked working but session is dead — clean it up.
		d.logger.Printf("Handler: dog %s is working but session is dead, clearing work", dg.Name)
		if err := mgr.ClearWork(dg.Name); err != nil {
			d.logger.Printf("Handler: failed to clear work for dog %s: %v", dg.Name, err)
		}
	}
}

// detectStaleWorkingDogs finds dogs in state=working whose last_active exceeds
// staleWorkingTimeout. These dogs have live tmux sessions sitting idle at a
// prompt — neither cleanupStuckDogs (needs dead session) nor reapIdleDogs
// (needs state=idle) will catch them.
func (d *Daemon) detectStaleWorkingDogs(mgr *dog.Manager, sm *dog.SessionManager) {
	dogs, err := mgr.List()
	if err != nil {
		d.logger.Printf("Handler: failed to list dogs for stale-working check: %v", err)
		return
	}

	now := time.Now()
	for _, dg := range dogs {
		if dg.State != dog.StateWorking {
			continue
		}

		staleDuration := now.Sub(dg.LastActive)
		if staleDuration < staleWorkingTimeout {
			continue
		}

		d.logger.Printf("Handler: dog %s stuck in working state (inactive %v, work: %s), clearing",
			dg.Name, staleDuration.Truncate(time.Minute), dg.Work)

		if err := mgr.ClearWork(dg.Name); err != nil {
			d.logger.Printf("Handler: failed to clear work for stale dog %s: %v", dg.Name, err)
			continue
		}

		// Kill the tmux session — it's not doing anything useful.
		running, err := sm.IsRunning(dg.Name)
		if err != nil {
			d.logger.Printf("Handler: error checking session for stale dog %s: %v", dg.Name, err)
			continue
		}
		if running {
			if err := sm.Stop(dg.Name, true); err != nil {
				d.logger.Printf("Handler: failed to stop session for stale dog %s: %v", dg.Name, err)
			}
		}
	}
}

// reapIdleDogs kills tmux sessions for dogs that have been idle too long, and
// removes long-idle dogs from the kennel when the pool is oversized.
func (d *Daemon) reapIdleDogs(mgr *dog.Manager, sm *dog.SessionManager) {
	dogs, err := mgr.List()
	if err != nil {
		d.logger.Printf("Handler: failed to list dogs for reaping: %v", err)
		return
	}

	now := time.Now()
	poolSize := len(dogs)

	for _, dg := range dogs {
		if dg.State != dog.StateIdle {
			continue
		}

		idleDuration := now.Sub(dg.LastActive)

		// Phase 1: kill stale tmux sessions for idle dogs.
		if idleDuration >= dogIdleSessionTimeout {
			running, err := sm.IsRunning(dg.Name)
			if err != nil {
				d.logger.Printf("Handler: error checking session for idle dog %s: %v", dg.Name, err)
				continue
			}
			if running {
				d.logger.Printf("Handler: reaping idle dog %s session (idle %v)", dg.Name, idleDuration.Truncate(time.Minute))
				if err := sm.Stop(dg.Name, true); err != nil {
					d.logger.Printf("Handler: failed to stop session for idle dog %s: %v", dg.Name, err)
				}
			}
		}

		// Phase 2: remove long-idle dogs when pool is oversized.
		if poolSize > maxDogPoolSize && idleDuration >= dogIdleRemoveTimeout {
			d.logger.Printf("Handler: removing long-idle dog %s from kennel (idle %v, pool %d/%d)",
				dg.Name, idleDuration.Truncate(time.Minute), poolSize, maxDogPoolSize)

			// Ensure session is dead before removing.
			running, _ := sm.IsRunning(dg.Name)
			if running {
				_ = sm.Stop(dg.Name, true)
			}

			if err := mgr.Remove(dg.Name); err != nil {
				d.logger.Printf("Handler: failed to remove idle dog %s: %v", dg.Name, err)
				continue
			}
			poolSize--
		}
	}
}

// dispatchPlugins scans for plugins, evaluates cooldown gates, and dispatches
// eligible plugins to idle dogs.
func (d *Daemon) dispatchPlugins(mgr *dog.Manager, sm *dog.SessionManager, rigsConfig *config.RigsConfig) {
	// Get rig names for scanner
	var rigNames []string
	if rigsConfig != nil {
		for name := range rigsConfig.Rigs {
			rigNames = append(rigNames, name)
		}
	}

	scanner := plugin.NewScanner(d.config.TownRoot, rigNames)
	plugins, err := scanner.DiscoverAll()
	if err != nil {
		d.logger.Printf("Handler: failed to discover plugins: %v", err)
		return
	}

	if len(plugins) == 0 {
		return
	}

	recorder := plugin.NewRecorder(d.config.TownRoot)
	router := mail.NewRouterWithTownRoot(d.config.TownRoot, d.config.TownRoot)

	for _, p := range plugins {
		// Only dispatch plugins with cooldown gates.
		if p.Gate == nil || p.Gate.Type != plugin.GateCooldown {
			continue
		}

		// Evaluate cooldown: skip if plugin ran recently.
		if p.Gate.Duration != "" {
			count, err := recorder.CountRunsSince(p.Name, p.Gate.Duration)
			if err != nil {
				d.logger.Printf("Handler: error checking cooldown for plugin %s: %v", p.Name, err)
				continue
			}
			if count > 0 {
				continue // Still in cooldown
			}
		}

		// Find an idle dog.
		idleDog, err := mgr.GetIdleDog()
		if err != nil {
			d.logger.Printf("Handler: error finding idle dog: %v", err)
			return // No point continuing if we can't list dogs
		}
		if idleDog == nil {
			d.logger.Printf("Handler: no idle dogs available, deferring remaining plugins")
			return
		}

		// Assign work and start session.
		workDesc := fmt.Sprintf("plugin:%s", p.Name)
		if err := mgr.AssignWork(idleDog.Name, workDesc); err != nil {
			d.logger.Printf("Handler: failed to assign work to dog %s: %v", idleDog.Name, err)
			continue
		}

		if err := sm.Start(idleDog.Name, dog.SessionStartOptions{
			WorkDesc: workDesc,
		}); err != nil {
			d.logger.Printf("Handler: failed to start session for dog %s: %v", idleDog.Name, err)
			// Roll back assignment on session start failure.
			if clearErr := mgr.ClearWork(idleDog.Name); clearErr != nil {
				d.logger.Printf("Handler: failed to clear work after start failure for dog %s: %v", idleDog.Name, clearErr)
			}
			continue
		}

		// Send mail with plugin instructions.
		msg := mail.NewMessage(
			"daemon",
			fmt.Sprintf("dog/%s", idleDog.Name),
			fmt.Sprintf("Plugin: %s", p.Name),
			p.Instructions,
		)
		msg.Type = mail.TypeTask
		msg.Timestamp = time.Now()
		if err := router.Send(msg); err != nil {
			d.logger.Printf("Handler: failed to send mail to dog %s: %v", idleDog.Name, err)
			// Session is already started — dog will find no mail and idle out.
		}

		d.logger.Printf("Handler: dispatched plugin %s to dog %s", p.Name, idleDog.Name)
	}
}

// loadRigsConfig loads the rigs configuration from mayor/rigs.json.
func (d *Daemon) loadRigsConfig() (*config.RigsConfig, error) {
	rigsPath := filepath.Join(d.config.TownRoot, "mayor", "rigs.json")
	return config.LoadRigsConfig(rigsPath)
}
