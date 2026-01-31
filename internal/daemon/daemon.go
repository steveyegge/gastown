package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/boot"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/feed"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/wisp"
	"github.com/steveyegge/gastown/internal/witness"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Daemon is the town-level background service.
// It ensures patrol agents (Deacon, Witnesses) are running and detects failures.
// This is recovery-focused: normal wake is handled by feed subscription (bd activity --follow).
// The daemon is the safety net for dead sessions, GUPP violations, and orphaned work.
type Daemon struct {
	config       *Config
	patrolConfig *DaemonPatrolConfig
	tmux         *tmux.Tmux
	logger       *log.Logger
	ctx          context.Context
	cancel       context.CancelFunc
	curator       *feed.Curator
	convoyWatcher *ConvoyWatcher
	doltServer    *DoltServerManager
	krcPruner     *KRCPruner

	// Mass death detection: track recent session deaths
	deathsMu     sync.Mutex
	recentDeaths []sessionDeath

	// Deacon startup tracking: prevents race condition where newly started
	// sessions are immediately killed by the heartbeat check.
	// See: https://github.com/steveyegge/gastown/issues/567
	// Note: Only accessed from heartbeat loop goroutine - no sync needed.
	deaconLastStarted time.Time
}

// sessionDeath records a detected session death for mass death analysis.
type sessionDeath struct {
	sessionName string
	timestamp   time.Time
}

// Mass death detection parameters
const (
	massDeathWindow    = 30 * time.Second // Time window to detect mass death
	massDeathThreshold = 3                // Number of deaths to trigger alert
)

// New creates a new daemon instance.
func New(config *Config) (*Daemon, error) {
	// Ensure daemon directory exists
	daemonDir := filepath.Dir(config.LogFile)
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return nil, fmt.Errorf("creating daemon directory: %w", err)
	}

	// Open log file
	logFile, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	logger := log.New(logFile, "", log.LstdFlags)
	ctx, cancel := context.WithCancel(context.Background())

	// Load patrol config from mayor/daemon.json (optional - nil if missing)
	patrolConfig := LoadPatrolConfig(config.TownRoot)
	if patrolConfig != nil {
		logger.Printf("Loaded patrol config from %s", PatrolConfigFile(config.TownRoot))
	}

	// Initialize Dolt server manager if configured
	var doltServer *DoltServerManager
	if patrolConfig != nil && patrolConfig.Patrols != nil && patrolConfig.Patrols.DoltServer != nil {
		doltServer = NewDoltServerManager(config.TownRoot, patrolConfig.Patrols.DoltServer, logger.Printf)
		if doltServer.IsEnabled() {
			logger.Printf("Dolt server management enabled (port %d)", patrolConfig.Patrols.DoltServer.Port)
		}
	}

	return &Daemon{
		config:       config,
		patrolConfig: patrolConfig,
		tmux:         tmux.NewTmux(),
		logger:       logger,
		ctx:          ctx,
		cancel:       cancel,
		doltServer:   doltServer,
	}, nil
}

// Run starts the daemon main loop.
func (d *Daemon) Run() error {
	d.logger.Printf("Daemon starting (PID %d)", os.Getpid())

	// Acquire exclusive lock to prevent multiple daemons from running.
	// This prevents the TOCTOU race condition where multiple concurrent starts
	// can all pass the IsRunning() check before any writes the PID file.
	// Uses gofrs/flock for cross-platform compatibility (Unix + Windows).
	lockFile := filepath.Join(d.config.TownRoot, "daemon", "daemon.lock")
	fileLock := flock.New(lockFile)

	// Try to acquire exclusive lock (non-blocking)
	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("daemon already running (lock held by another process)")
	}
	defer func() { _ = fileLock.Unlock() }()

	// Write PID file
	if err := os.WriteFile(d.config.PidFile, []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		return fmt.Errorf("writing PID file: %w", err)
	}
	defer func() { _ = os.Remove(d.config.PidFile) }() // best-effort cleanup

	// Update state
	state := &State{
		Running:   true,
		PID:       os.Getpid(),
		StartedAt: time.Now(),
	}
	if err := SaveState(d.config.TownRoot, state); err != nil {
		d.logger.Printf("Warning: failed to save state: %v", err)
	}

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, daemonSignals()...)

	// Fixed recovery-focused heartbeat (no activity-based backoff)
	// Normal wake is handled by feed subscription (bd activity --follow)
	timer := time.NewTimer(recoveryHeartbeatInterval)
	defer timer.Stop()

	d.logger.Printf("Daemon running, recovery heartbeat interval %v", recoveryHeartbeatInterval)

	// Start feed curator goroutine
	d.curator = feed.NewCurator(d.config.TownRoot)
	if err := d.curator.Start(); err != nil {
		d.logger.Printf("Warning: failed to start feed curator: %v", err)
	} else {
		d.logger.Println("Feed curator started")
	}

	// Start convoy watcher for event-driven convoy completion
	d.convoyWatcher = NewConvoyWatcher(d.config.TownRoot, d.logger.Printf)
	if err := d.convoyWatcher.Start(); err != nil {
		d.logger.Printf("Warning: failed to start convoy watcher: %v", err)
	} else {
		d.logger.Println("Convoy watcher started")
	}

	// Start KRC pruner for automatic ephemeral data cleanup
	krcPruner, err := NewKRCPruner(d.config.TownRoot, d.logger.Printf)
	if err != nil {
		d.logger.Printf("Warning: failed to create KRC pruner: %v", err)
	} else {
		d.krcPruner = krcPruner
		if err := d.krcPruner.Start(); err != nil {
			d.logger.Printf("Warning: failed to start KRC pruner: %v", err)
		} else {
			d.logger.Println("KRC pruner started")
		}
	}

	// Initial heartbeat
	d.heartbeat(state)

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Println("Daemon context canceled, shutting down")
			return d.shutdown(state)

		case sig := <-sigChan:
			if isLifecycleSignal(sig) {
				// Lifecycle signal: immediate lifecycle processing (from gt handoff)
				d.logger.Println("Received lifecycle signal, processing lifecycle requests immediately")
				d.processLifecycleRequests()
			} else {
				d.logger.Printf("Received signal %v, shutting down", sig)
				return d.shutdown(state)
			}

		case <-timer.C:
			d.heartbeat(state)

			// Fixed recovery interval (no activity-based backoff)
			timer.Reset(recoveryHeartbeatInterval)
		}
	}
}

// recoveryHeartbeatInterval is the fixed interval for recovery-focused daemon.
// Normal wake is handled by feed subscription (bd activity --follow).
// The daemon is a safety net for dead sessions, GUPP violations, and orphaned work.
// 3 minutes is fast enough to detect stuck agents promptly while avoiding excessive overhead.
const recoveryHeartbeatInterval = 3 * time.Minute

// heartbeat performs one heartbeat cycle.
// The daemon is recovery-focused: it ensures agents are running and detects failures.
// Normal wake is handled by feed subscription (bd activity --follow).
// The daemon is the safety net for edge cases:
// - Dead sessions that need restart
// - Agents with work-on-hook not progressing (GUPP violation)
// - Orphaned work (assigned to dead agents)
func (d *Daemon) heartbeat(state *State) {
	// Skip heartbeat if shutdown is in progress.
	// This prevents the daemon from fighting shutdown by auto-restarting killed agents.
	// The shutdown.lock file is created by gt down before terminating sessions.
	if d.isShutdownInProgress() {
		d.logger.Println("Shutdown in progress, skipping heartbeat")
		return
	}

	d.logger.Println("Heartbeat starting (recovery-focused)")

	// 0. Ensure Dolt server is running (if configured)
	// This must happen before beads operations that depend on Dolt.
	d.ensureDoltServerRunning()

	// 1. Ensure Deacon is running (restart if dead)
	// Check patrol config - can be disabled in mayor/daemon.json
	if IsPatrolEnabled(d.patrolConfig, "deacon") {
		d.ensureDeaconRunning()
	} else {
		d.logger.Printf("Deacon patrol disabled in config, skipping")
	}

	// 2. Run Boot triage (checks Deacon health: session exists + heartbeat fresh)
	// Boot handles "is Deacon responsive" decisions using mechanical (non-Claude) triage.
	// This replaces the previous belt-and-suspenders checkDeaconHeartbeat() call.
	// Only run if Deacon patrol is enabled
	if IsPatrolEnabled(d.patrolConfig, "deacon") {
		d.ensureBootRunning()
	}

	// 3. Ensure Witnesses are running for all rigs (restart if dead)
	// Check patrol config - can be disabled in mayor/daemon.json
	if IsPatrolEnabled(d.patrolConfig, "witness") {
		d.ensureWitnessesRunning()
	} else {
		d.logger.Printf("Witness patrol disabled in config, skipping")
	}

	// 4. Ensure Refineries are running for all rigs (restart if dead)
	// Check patrol config - can be disabled in mayor/daemon.json
	if IsPatrolEnabled(d.patrolConfig, "refinery") {
		d.ensureRefineriesRunning()
	} else {
		d.logger.Printf("Refinery patrol disabled in config, skipping")
	}

	// 5. Trigger pending polecat spawns (bootstrap mode - ZFC violation acceptable)
	// This ensures polecats get nudged even when Deacon isn't in a patrol cycle.
	// Uses regex-based WaitForRuntimeReady, which is acceptable for daemon bootstrap.
	d.triggerPendingSpawns()

	// 6. Process lifecycle requests
	d.processLifecycleRequests()

	// 7. Check for GUPP violations (agents with work-on-hook not progressing)
	d.checkGUPPViolations()

	// 8. Check for orphaned work (assigned to dead agents)
	d.checkOrphanedWork()

	// 9. Check polecat session health (proactive crash detection)
	// This validates tmux sessions are still alive for polecats with work-on-hook
	d.checkPolecatSessionHealth()

	// 10. Clean up orphaned claude subagent processes (memory leak prevention)
	// These are Task tool subagents that didn't clean up after completion.
	// This is a safety net - Deacon patrol also does this more frequently.
	d.cleanupOrphanedProcesses()

	// Update state
	state.LastHeartbeat = time.Now()
	state.HeartbeatCount++
	if err := SaveState(d.config.TownRoot, state); err != nil {
		d.logger.Printf("Warning: failed to save state: %v", err)
	}

	d.logger.Printf("Heartbeat complete (#%d)", state.HeartbeatCount)
}

// ensureDoltServerRunning ensures the Dolt SQL server is running if configured.
// This provides the backend for beads database access in server mode.
func (d *Daemon) ensureDoltServerRunning() {
	if d.doltServer == nil || !d.doltServer.IsEnabled() {
		return
	}

	if err := d.doltServer.EnsureRunning(); err != nil {
		d.logger.Printf("Error ensuring Dolt server is running: %v", err)
	}
}

// DeaconRole is the role name for the Deacon's handoff bead.
const DeaconRole = "deacon"

// getDeaconSessionName returns the Deacon session name for the daemon's town.
func (d *Daemon) getDeaconSessionName() string {
	return session.DeaconSessionName()
}

// ensureBootRunning runs Boot triage to check on the Deacon.
// Boot is a fresh-each-tick watchdog that decides whether to start/wake/nudge
// the Deacon, centralizing the "when to wake" decision.
//
// IMPORTANT: We always use degraded (mechanical) mode to avoid Boot's Claude
// session getting stuck waiting for user input. The mechanical triage is:
// - Fast (milliseconds, not seconds waiting for Claude)
// - Reliable (no interactive prompts that can block)
// - Sufficient (checks heartbeat, restarts stuck Deacon)
func (d *Daemon) ensureBootRunning() {
	b := boot.New(d.config.TownRoot)

	// Check if Boot is already running
	if b.IsRunning() {
		d.logger.Println("Boot already running, skipping triage")
		return
	}

	// Always use degraded (mechanical) triage - it's more reliable than spawning
	// a Claude session that might ask questions and block.
	d.logger.Println("Running Boot triage (mechanical mode)")
	d.runDegradedBootTriage(b)
}

// runDegradedBootTriage performs mechanical Boot triage without AI reasoning.
// This checks both session existence AND heartbeat staleness.
func (d *Daemon) runDegradedBootTriage(b *boot.Boot) {
	startTime := time.Now()
	status := &boot.Status{
		Running:   true,
		StartedAt: startTime,
	}

	deaconSession := d.getDeaconSessionName()

	// Step 1: Check if Deacon session exists
	hasDeacon, err := d.tmux.HasSession(deaconSession)
	if err != nil {
		d.logger.Printf("Error checking Deacon session: %v", err)
		status.LastAction = "error"
		status.Error = err.Error()
		status.Running = false
		status.CompletedAt = time.Now()
		_ = b.SaveStatus(status)
		return
	}

	if !hasDeacon {
		// Deacon not running - start it
		d.logger.Println("Deacon not running, starting...")
		d.ensureDeaconRunning()
		status.LastAction = "start"
		status.Target = "deacon"
		status.Running = false
		status.CompletedAt = time.Now()
		_ = b.SaveStatus(status)
		return
	}

	// Step 2: Deacon exists - check heartbeat to detect stuck sessions
	hb := deacon.ReadHeartbeat(d.config.TownRoot)
	if hb != nil && hb.ShouldPoke() {
		age := hb.Age()
		if age > 30*time.Minute {
			// Very stuck - restart the session
			d.logger.Printf("Deacon heartbeat is %s old - restarting session", age.Round(time.Minute))
			if err := d.tmux.KillSession(deaconSession); err == nil {
				status.LastAction = "restart"
				status.Target = "deacon-stuck"
			} else {
				d.logger.Printf("Error killing stuck Deacon: %v", err)
				status.LastAction = "restart-failed"
				status.Error = err.Error()
			}
		} else if age > 15*time.Minute {
			// Stuck but not critically - nudge to wake up
			d.logger.Printf("Deacon heartbeat is %s old - nudging session", age.Round(time.Minute))
			_ = d.tmux.NudgeSession(deaconSession, "HEALTH_CHECK: heartbeat is stale, respond to confirm responsiveness")
			status.LastAction = "nudge"
			status.Target = "deacon-stale"
		} else {
			status.LastAction = "nothing"
		}
	} else {
		status.LastAction = "nothing"
	}

	status.Running = false
	status.CompletedAt = time.Now()

	if err := b.SaveStatus(status); err != nil {
		d.logger.Printf("Warning: failed to save Boot status: %v", err)
	}
}

// ensureDeaconRunning ensures the Deacon is running.
// Uses deacon.Manager for consistent startup behavior (WaitForShellReady, GUPP, etc.).
func (d *Daemon) ensureDeaconRunning() {
	mgr := deacon.NewManager(d.config.TownRoot)

	if err := mgr.Start(""); err != nil {
		if err == deacon.ErrAlreadyRunning {
			// Deacon is running - nothing to do
			return
		}
		d.logger.Printf("Error starting Deacon: %v", err)
		return
	}

	// Track when we started the Deacon to prevent race condition in checkDeaconHeartbeat.
	// The heartbeat file will still be stale until the Deacon runs a full patrol cycle.
	d.deaconLastStarted = time.Now()
	d.logger.Println("Deacon started successfully")
}

// deaconGracePeriod is the time to wait after starting a Deacon before checking heartbeat.
// The Deacon needs time to initialize Claude, run SessionStart hooks, execute gt prime,
// run a patrol cycle, and write a fresh heartbeat. 5 minutes is conservative.
const deaconGracePeriod = 5 * time.Minute

// checkDeaconHeartbeat checks if the Deacon is making progress.
// This is a belt-and-suspenders fallback in case Boot doesn't detect stuck states.
// Uses the heartbeat file that the Deacon updates on each patrol cycle.
func (d *Daemon) checkDeaconHeartbeat() {
	// Grace period: don't check heartbeat for newly started sessions.
	// This prevents the race condition where we start a Deacon, then immediately
	// see a stale heartbeat (from before the crash) and kill the session we just started.
	// See: https://github.com/steveyegge/gastown/issues/567
	if !d.deaconLastStarted.IsZero() && time.Since(d.deaconLastStarted) < deaconGracePeriod {
		d.logger.Printf("Deacon started recently (%s ago), skipping heartbeat check",
			time.Since(d.deaconLastStarted).Round(time.Second))
		return
	}

	hb := deacon.ReadHeartbeat(d.config.TownRoot)
	if hb == nil {
		// No heartbeat file - Deacon hasn't started a cycle yet
		return
	}

	age := hb.Age()

	// If heartbeat is very stale (>15 min), the Deacon is likely stuck
	if !hb.ShouldPoke() {
		// Heartbeat is fresh enough
		return
	}

	d.logger.Printf("Deacon heartbeat is stale (%s old), checking session...", age.Round(time.Minute))

	sessionName := d.getDeaconSessionName()

	// Check if session exists
	hasSession, err := d.tmux.HasSession(sessionName)
	if err != nil {
		d.logger.Printf("Error checking Deacon session: %v", err)
		return
	}

	if !hasSession {
		// Session doesn't exist - ensureDeaconRunning already ran earlier
		// in heartbeat, so Deacon should be starting
		return
	}

	// Session exists but heartbeat is stale - Deacon is stuck
	if age > 30*time.Minute {
		// Very stuck - restart the session.
		// Use KillSessionWithProcesses to ensure all descendant processes are killed.
		d.logger.Printf("Deacon stuck for %s - restarting session", age.Round(time.Minute))
		if err := d.tmux.KillSessionWithProcesses(sessionName); err != nil {
			d.logger.Printf("Error killing stuck Deacon: %v", err)
		}
		// Spawn new Deacon immediately instead of waiting for next heartbeat
		// (kill may fail if session disappeared between check and kill)
		d.ensureDeaconRunning()
	} else {
		// Stuck but not critically - nudge to wake up
		d.logger.Printf("Deacon stuck for %s - nudging session", age.Round(time.Minute))
		if err := d.tmux.NudgeSession(sessionName, "HEALTH_CHECK: heartbeat stale, respond to confirm responsiveness"); err != nil {
			d.logger.Printf("Error nudging stuck Deacon: %v", err)
		}
	}
}

// ensureWitnessesRunning ensures witnesses are running for all rigs.
// Called on each heartbeat to maintain witness patrol loops.
func (d *Daemon) ensureWitnessesRunning() {
	rigs := d.getKnownRigs()
	for _, rigName := range rigs {
		d.ensureWitnessRunning(rigName)
	}
}

// ensureWitnessRunning ensures the witness for a specific rig is running.
// Discover, don't track: uses Manager.Start() which checks tmux directly (gt-zecmc).
func (d *Daemon) ensureWitnessRunning(rigName string) {
	// Check rig operational state before auto-starting
	if operational, reason := d.isRigOperational(rigName); !operational {
		d.logger.Printf("Skipping witness auto-start for %s: %s", rigName, reason)
		return
	}

	// Manager.Start() handles: zombie detection, session creation, env vars, theming,
	// startup readiness waits, and crucially - startup/propulsion nudges (GUPP).
	// It returns ErrAlreadyRunning if Claude is already running in tmux.
	r := &rig.Rig{
		Name: rigName,
		Path: filepath.Join(d.config.TownRoot, rigName),
	}
	mgr := witness.NewManager(r)

	if err := mgr.Start(false, "", nil); err != nil {
		if err == witness.ErrAlreadyRunning {
			// Already running - this is the expected case
			d.logger.Printf("Witness for %s already running, skipping spawn", rigName)
			return
		}
		d.logger.Printf("Error starting witness for %s: %v", rigName, err)
		return
	}

	d.logger.Printf("Witness session for %s started successfully", rigName)
}

// ensureRefineriesRunning ensures refineries are running for all rigs.
// Called on each heartbeat to maintain refinery merge queue processing.
func (d *Daemon) ensureRefineriesRunning() {
	rigs := d.getKnownRigs()
	for _, rigName := range rigs {
		d.ensureRefineryRunning(rigName)
	}
}

// ensureRefineryRunning ensures the refinery for a specific rig is running.
// Discover, don't track: uses Manager.Start() which checks tmux directly (gt-zecmc).
func (d *Daemon) ensureRefineryRunning(rigName string) {
	// Check rig operational state before auto-starting
	if operational, reason := d.isRigOperational(rigName); !operational {
		d.logger.Printf("Skipping refinery auto-start for %s: %s", rigName, reason)
		return
	}

	// Manager.Start() handles: zombie detection, session creation, env vars, theming,
	// WaitForClaudeReady, and crucially - startup/propulsion nudges (GUPP).
	// It returns ErrAlreadyRunning if Claude is already running in tmux.
	r := &rig.Rig{
		Name: rigName,
		Path: filepath.Join(d.config.TownRoot, rigName),
	}
	mgr := refinery.NewManager(r)

	if err := mgr.Start(false, ""); err != nil {
		if err == refinery.ErrAlreadyRunning {
			// Already running - this is the expected case when fix is working
			d.logger.Printf("Refinery for %s already running, skipping spawn", rigName)
			return
		}
		d.logger.Printf("Error starting refinery for %s: %v", rigName, err)
		return
	}

	d.logger.Printf("Refinery session for %s started successfully", rigName)
}

// getKnownRigs returns list of registered rig names.
func (d *Daemon) getKnownRigs() []string {
	rigsPath := filepath.Join(d.config.TownRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return nil
	}

	var parsed struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}

	var rigs []string
	for name := range parsed.Rigs {
		rigs = append(rigs, name)
	}
	return rigs
}

// isRigOperational checks if a rig is in an operational state.
// Returns true if the rig can have agents auto-started.
// Returns false (with reason) if the rig is parked, docked, or has auto_restart blocked/disabled.
func (d *Daemon) isRigOperational(rigName string) (bool, string) {
	cfg := wisp.NewConfig(d.config.TownRoot, rigName)

	// Note: wisp config not existing is normal - it means "use defaults" (operational).
	// The config is only created when a rig is explicitly parked/docked via gt commands.

	// Check wisp layer first (local/ephemeral overrides)
	status := cfg.GetString("status")
	switch status {
	case "parked":
		return false, "rig is parked"
	case "docked":
		return false, "rig is docked"
	}

	// Check rig bead labels (global/synced docked status)
	// This is the persistent docked state set by 'gt rig dock'
	rigPath := filepath.Join(d.config.TownRoot, rigName)
	if rigCfg, err := rig.LoadRigConfig(rigPath); err == nil && rigCfg.Beads != nil {
		rigBeadID := fmt.Sprintf("%s-rig-%s", rigCfg.Beads.Prefix, rigName)
		rigBeadsDir := beads.ResolveBeadsDir(rigPath)
		bd := beads.NewWithBeadsDir(rigPath, rigBeadsDir)
		if issue, err := bd.Show(rigBeadID); err == nil {
			for _, label := range issue.Labels {
				if label == "status:docked" {
					return false, "rig is docked (global)"
				}
				if label == "status:parked" {
					return false, "rig is parked (global)"
				}
			}
		}
	}

	// Check auto_restart config
	// If explicitly blocked (nil), auto-restart is disabled
	if cfg.IsBlocked("auto_restart") {
		return false, "auto_restart is blocked"
	}

	// If explicitly set to false, auto-restart is disabled
	// Note: GetBool returns false for unset keys, so we need to check if it's explicitly set
	val := cfg.Get("auto_restart")
	if val != nil {
		if autoRestart, ok := val.(bool); ok && !autoRestart {
			return false, "auto_restart is disabled"
		}
	}

	return true, ""
}

// triggerPendingSpawns polls pending polecat spawns and triggers those that are ready.
// This is bootstrap mode - uses regex-based WaitForRuntimeReady which is acceptable
// for daemon operations when no AI agent is guaranteed to be running.
// The timeout is short (2s) to avoid blocking the heartbeat.
func (d *Daemon) triggerPendingSpawns() {
	const triggerTimeout = 2 * time.Second

	// Check for pending spawns (from POLECAT_STARTED messages in Deacon inbox)
	pending, err := polecat.CheckInboxForSpawns(d.config.TownRoot)
	if err != nil {
		d.logger.Printf("Error checking pending spawns: %v", err)
		return
	}

	if len(pending) == 0 {
		return
	}

	d.logger.Printf("Found %d pending spawn(s), attempting to trigger...", len(pending))

	// Trigger pending spawns (uses WaitForRuntimeReady with short timeout)
	results, err := polecat.TriggerPendingSpawns(d.config.TownRoot, triggerTimeout)
	if err != nil {
		d.logger.Printf("Error triggering spawns: %v", err)
		return
	}

	// Log results
	triggered := 0
	for _, r := range results {
		if r.Triggered {
			triggered++
			d.logger.Printf("Triggered polecat: %s/%s", r.Spawn.Rig, r.Spawn.Polecat)
		} else if r.Error != nil {
			d.logger.Printf("Error triggering %s: %v", r.Spawn.Session, r.Error)
		}
	}

	if triggered > 0 {
		d.logger.Printf("Triggered %d/%d pending spawn(s)", triggered, len(pending))
	}

	// Prune stale pending spawns (older than 5 minutes - likely dead sessions)
	pruned, _ := polecat.PruneStalePending(d.config.TownRoot, 5*time.Minute)
	if pruned > 0 {
		d.logger.Printf("Pruned %d stale pending spawn(s)", pruned)
	}
}

// processLifecycleRequests checks for and processes lifecycle requests.
func (d *Daemon) processLifecycleRequests() {
	d.ProcessLifecycleRequests()
}

// shutdown performs graceful shutdown.
func (d *Daemon) shutdown(state *State) error { //nolint:unparam // error return kept for future use
	d.logger.Println("Daemon shutting down")

	// Stop feed curator
	if d.curator != nil {
		d.curator.Stop()
		d.logger.Println("Feed curator stopped")
	}

	// Stop convoy watcher
	if d.convoyWatcher != nil {
		d.convoyWatcher.Stop()
		d.logger.Println("Convoy watcher stopped")
	}

	// Stop KRC pruner
	if d.krcPruner != nil {
		d.krcPruner.Stop()
		d.logger.Println("KRC pruner stopped")
	}

	// Stop Dolt server if we're managing it
	if d.doltServer != nil && d.doltServer.IsEnabled() && !d.doltServer.IsExternal() {
		if err := d.doltServer.Stop(); err != nil {
			d.logger.Printf("Warning: failed to stop Dolt server: %v", err)
		} else {
			d.logger.Println("Dolt server stopped")
		}
	}

	state.Running = false
	if err := SaveState(d.config.TownRoot, state); err != nil {
		d.logger.Printf("Warning: failed to save final state: %v", err)
	}

	d.logger.Println("Daemon stopped")
	return nil
}

// Stop signals the daemon to stop.
func (d *Daemon) Stop() {
	d.cancel()
}

// isShutdownInProgress checks if a shutdown is currently in progress.
// The shutdown.lock file is created by gt down before terminating sessions.
// This prevents the daemon from fighting shutdown by auto-restarting killed agents.
func (d *Daemon) isShutdownInProgress() bool {
	lockPath := filepath.Join(d.config.TownRoot, "daemon", "shutdown.lock")
	_, err := os.Stat(lockPath)
	return err == nil
}

// IsRunning checks if a daemon is running for the given town.
// It checks the PID file and verifies the process is alive.
// Note: The file lock in Run() is the authoritative mechanism for preventing
// duplicate daemons. This function is for status checks and cleanup.
func IsRunning(townRoot string) (bool, int, error) {
	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		// Return error for other failures (permissions, I/O)
		return false, 0, fmt.Errorf("reading PID file: %w", err)
	}

	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		// Corrupted PID file - return error, not silent false
		return false, 0, fmt.Errorf("invalid PID in file %q: %w", pidStr, err)
	}

	// Check if process is alive
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0, nil
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process not running, clean up stale PID file
		if err := os.Remove(pidFile); err == nil {
			// Successfully cleaned up stale file
			return false, 0, fmt.Errorf("removed stale PID file (process %d not found)", pid)
		}
		return false, 0, nil
	}

	// CRITICAL: Verify it's actually our daemon, not PID reuse
	if !isGasTownDaemon(pid) {
		// PID reused by different process
		if err := os.Remove(pidFile); err == nil {
			return false, 0, fmt.Errorf("removed stale PID file (PID %d is not gt daemon)", pid)
		}
		return false, 0, nil
	}

	return true, pid, nil
}

// isGasTownDaemon checks if a PID is actually a gt daemon run process.
// This prevents false positives from PID reuse.
// Uses ps command for cross-platform compatibility (Linux, macOS).
func isGasTownDaemon(pid int) bool {
	// Use ps to get command for the PID (works on Linux and macOS)
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	cmdline := strings.TrimSpace(string(output))

	// Check if it's "gt daemon run" or "/path/to/gt daemon run"
	return strings.Contains(cmdline, "gt") && strings.Contains(cmdline, "daemon") && strings.Contains(cmdline, "run")
}

// StopDaemon stops the running daemon for the given town.
// Note: The file lock in Run() prevents multiple daemons per town, so we only
// need to kill the process from the PID file.
func StopDaemon(townRoot string) error {
	running, pid, err := IsRunning(townRoot)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("daemon is not running")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM: %w", err)
	}

	// Wait a bit for graceful shutdown
	time.Sleep(constants.ShutdownNotifyDelay)

	// Check if still running
	if err := process.Signal(syscall.Signal(0)); err == nil {
		// Still running, force kill
		_ = process.Signal(syscall.SIGKILL)
	}

	// Clean up PID file
	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
	_ = os.Remove(pidFile)

	return nil
}

// FindOrphanedDaemons finds all gt daemon run processes that aren't tracked by PID file.
// Returns list of orphaned PIDs.
func FindOrphanedDaemons() ([]int, error) {
	// Use pgrep to find all "daemon run" processes (broad search, then verify with isGasTownDaemon)
	cmd := exec.Command("pgrep", "-f", "daemon run")
	output, err := cmd.Output()
	if err != nil {
		// Exit code 1 means no processes found - that's OK
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("pgrep failed: %w", err)
	}

	// Parse PIDs
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		// Verify it's actually gt daemon (filters out unrelated processes)
		if isGasTownDaemon(pid) {
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

// KillOrphanedDaemons finds and kills any orphaned gt daemon processes.
// Returns number of processes killed.
func KillOrphanedDaemons() (int, error) {
	pids, err := FindOrphanedDaemons()
	if err != nil {
		return 0, err
	}

	killed := 0
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			continue
		}

		// Try SIGTERM first
		if err := process.Signal(syscall.SIGTERM); err != nil {
			continue
		}

		// Wait for graceful shutdown
		time.Sleep(200 * time.Millisecond)

		// Check if still alive
		if err := process.Signal(syscall.Signal(0)); err == nil {
			// Still alive, force kill
			_ = process.Signal(syscall.SIGKILL)
		}

		killed++
	}

	return killed, nil
}

// checkPolecatSessionHealth proactively validates polecat tmux sessions.
// This detects crashed polecats that:
// 1. Have work-on-hook (assigned work)
// 2. Report state=running/working in their agent bead
// 3. But the tmux session is actually dead
//
// When a crash is detected, the polecat is automatically restarted.
// This provides faster recovery than waiting for GUPP timeout or Witness detection.
func (d *Daemon) checkPolecatSessionHealth() {
	rigs := d.getKnownRigs()
	for _, rigName := range rigs {
		d.checkRigPolecatHealth(rigName)
	}
}

// checkRigPolecatHealth checks polecat session health for a specific rig.
func (d *Daemon) checkRigPolecatHealth(rigName string) {
	// Get polecat directories for this rig
	polecatsDir := filepath.Join(d.config.TownRoot, rigName, "polecats")
	polecats, err := listPolecatWorktrees(polecatsDir)
	if err != nil {
		return // No polecats directory - rig might not have polecats
	}

	for _, polecatName := range polecats {
		d.checkPolecatHealth(rigName, polecatName)
	}
}

func listPolecatWorktrees(polecatsDir string) ([]string, error) {
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		return nil, err
	}

	polecats := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		polecats = append(polecats, name)
	}

	return polecats, nil
}

// checkPolecatHealth checks a single polecat's session health.
// If the polecat has work-on-hook but the tmux session is dead, it's restarted.
func (d *Daemon) checkPolecatHealth(rigName, polecatName string) {
	// Build the expected tmux session name
	sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)

	// Check if tmux session exists
	sessionAlive, err := d.tmux.HasSession(sessionName)
	if err != nil {
		d.logger.Printf("Error checking session %s: %v", sessionName, err)
		return
	}

	if sessionAlive {
		// Session is alive - nothing to do
		return
	}

	// Session is dead. Check if the polecat has work-on-hook.
	// Agent beads use hq- prefix and are stored in town beads (fix for gt-myc).
	townName, _ := workspace.GetTownName(d.config.TownRoot)
	agentBeadID := beads.PolecatBeadIDTown(townName, rigName, polecatName)
	info, err := d.getAgentBeadInfo(agentBeadID)
	if err != nil {
		// Agent bead doesn't exist or error - polecat might not be registered
		return
	}

	// Check if polecat has hooked work
	if info.HookBead == "" {
		// No hooked work - this polecat is orphaned (should have self-nuked).
		// Self-cleaning model: polecats nuke themselves on completion.
		// An orphan with a dead session doesn't need restart - it needs cleanup.
		// Let the Witness handle orphan detection/cleanup during patrol.
		return
	}

	// Polecat has work but session is dead - this is a crash!
	d.logger.Printf("CRASH DETECTED: polecat %s/%s has hook_bead=%s but session %s is dead",
		rigName, polecatName, info.HookBead, sessionName)

	// Track this death for mass death detection
	d.recordSessionDeath(sessionName)

	// Auto-restart the polecat
	if err := d.restartPolecatSession(rigName, polecatName, sessionName); err != nil {
		d.logger.Printf("Error restarting polecat %s/%s: %v", rigName, polecatName, err)
		// Notify witness as fallback
		d.notifyWitnessOfCrashedPolecat(rigName, polecatName, info.HookBead, err)
	} else {
		d.logger.Printf("Successfully restarted crashed polecat %s/%s", rigName, polecatName)
	}
}

// recordSessionDeath records a session death and checks for mass death pattern.
func (d *Daemon) recordSessionDeath(sessionName string) {
	d.deathsMu.Lock()
	defer d.deathsMu.Unlock()

	now := time.Now()

	// Add this death
	d.recentDeaths = append(d.recentDeaths, sessionDeath{
		sessionName: sessionName,
		timestamp:   now,
	})

	// Prune deaths outside the window
	cutoff := now.Add(-massDeathWindow)
	var recent []sessionDeath
	for _, death := range d.recentDeaths {
		if death.timestamp.After(cutoff) {
			recent = append(recent, death)
		}
	}
	d.recentDeaths = recent

	// Check for mass death
	if len(d.recentDeaths) >= massDeathThreshold {
		d.emitMassDeathEvent()
	}
}

// emitMassDeathEvent logs a mass death event when multiple sessions die in a short window.
func (d *Daemon) emitMassDeathEvent() {
	// Collect session names
	var sessions []string
	for _, death := range d.recentDeaths {
		sessions = append(sessions, death.sessionName)
	}

	count := len(sessions)
	window := massDeathWindow.String()

	d.logger.Printf("MASS DEATH DETECTED: %d sessions died in %s: %v", count, window, sessions)

	// Emit feed event
	_ = events.LogFeed(events.TypeMassDeath, "daemon",
		events.MassDeathPayload(count, window, sessions, ""))

	// Clear the deaths to avoid repeated alerts
	d.recentDeaths = nil
}

// restartPolecatSession restarts a crashed polecat session.
func (d *Daemon) restartPolecatSession(rigName, polecatName, sessionName string) error {
	// Check rig operational state before auto-restarting
	if operational, reason := d.isRigOperational(rigName); !operational {
		return fmt.Errorf("cannot restart polecat: %s", reason)
	}

	// Calculate rig path for agent config resolution
	rigPath := filepath.Join(d.config.TownRoot, rigName)

	// Determine working directory (handle both new and old structures)
	// New structure: polecats/<name>/<rigname>/
	// Old structure: polecats/<name>/
	workDir := filepath.Join(rigPath, "polecats", polecatName, rigName)
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		// Fall back to old structure
		workDir = filepath.Join(rigPath, "polecats", polecatName)
	}

	// Verify the worktree exists
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("polecat worktree does not exist: %s", workDir)
	}

	// Pre-sync workspace (ensure beads are current)
	d.syncWorkspace(workDir)

	// Create new tmux session
	// Use EnsureSessionFresh to handle zombie sessions that exist but have dead Claude
	if err := d.tmux.EnsureSessionFresh(sessionName, workDir); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment variables using centralized AgentEnv
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:          "polecat",
		Rig:           rigName,
		AgentName:     polecatName,
		TownRoot:      d.config.TownRoot,
		BeadsNoDaemon: true,
	})

	// Set all env vars in tmux session (for debugging) and they'll also be exported to Claude
	for k, v := range envVars {
		_ = d.tmux.SetEnvironment(sessionName, k, v)
	}

	// Apply theme
	theme := tmux.AssignTheme(rigName)
	_ = d.tmux.ConfigureGasTownSession(sessionName, theme, rigName, polecatName, "polecat")

	// Set pane-died hook for future crash detection
	agentID := fmt.Sprintf("%s/%s", rigName, polecatName)
	_ = d.tmux.SetPaneDiedHook(sessionName, agentID)

	// Launch Claude with environment exported inline
	// Pass rigPath so rig agent settings are honored (not town-level defaults)
	startCmd := config.BuildStartupCommand(envVars, rigPath, "")
	if err := d.tmux.SendKeys(sessionName, startCmd); err != nil {
		return fmt.Errorf("sending startup command: %w", err)
	}

	// Wait for Claude to start, then accept bypass permissions warning if it appears.
	// This ensures automated restarts aren't blocked by the warning dialog.
	if err := d.tmux.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal - Claude might still start
	}
	_ = d.tmux.AcceptBypassPermissionsWarning(sessionName)

	return nil
}

// notifyWitnessOfCrashedPolecat notifies the witness when a polecat restart fails.
func (d *Daemon) notifyWitnessOfCrashedPolecat(rigName, polecatName, hookBead string, restartErr error) {
	witnessAddr := rigName + "/witness"
	subject := fmt.Sprintf("CRASHED_POLECAT: %s/%s restart failed", rigName, polecatName)
	body := fmt.Sprintf(`Polecat %s crashed and automatic restart failed.

hook_bead: %s
restart_error: %v

Manual intervention may be required.`,
		polecatName, hookBead, restartErr)

	cmd := exec.Command("gt", "mail", "send", witnessAddr, "-s", subject, "-m", body) //nolint:gosec // G204: args are constructed internally
	cmd.Dir = d.config.TownRoot
	cmd.Env = os.Environ() // Inherit PATH to find gt executable
	if err := cmd.Run(); err != nil {
		d.logger.Printf("Warning: failed to notify witness of crashed polecat: %v", err)
	}
}

// cleanupOrphanedProcesses kills orphaned claude subagent processes.
// These are Task tool subagents that didn't clean up after completion.
// Detection uses TTY column: processes with TTY "?" have no controlling terminal.
// This is a safety net fallback - Deacon patrol also runs this more frequently.
func (d *Daemon) cleanupOrphanedProcesses() {
	results, err := util.CleanupOrphanedClaudeProcesses()
	if err != nil {
		d.logger.Printf("Warning: orphan process cleanup failed: %v", err)
		return
	}

	if len(results) > 0 {
		d.logger.Printf("Orphan cleanup: processed %d process(es)", len(results))
		for _, r := range results {
			if r.Signal == "UNKILLABLE" {
				d.logger.Printf("  WARNING: PID %d (%s) survived SIGKILL", r.Process.PID, r.Process.Cmd)
			} else {
				d.logger.Printf("  Sent %s to PID %d (%s)", r.Signal, r.Process.PID, r.Process.Cmd)
			}
		}
	}
}
