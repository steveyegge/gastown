package daemon

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/google/uuid"
	beadsdk "github.com/steveyegge/beads"
	"gopkg.in/natefinch/lumberjack.v2"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/boot"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/feed"
	gitpkg "github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mayor"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/telemetry"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/wisp"
	"github.com/steveyegge/gastown/internal/witness"
)

// Daemon is the town-level background service.
// It ensures patrol agents (Deacon, Witnesses) are running and detects failures.
// This is recovery-focused: normal wake is handled by feed subscription (bd activity --follow).
// The daemon is the safety net for dead sessions, GUPP violations, and orphaned work.
type Daemon struct {
	config        *Config
	patrolConfig  *DaemonPatrolConfig
	tmux          *tmux.Tmux
	logger        *log.Logger
	ctx           context.Context
	cancel        context.CancelFunc
	curator       *feed.Curator
	convoyManager *ConvoyManager
	beadsStores   map[string]beadsdk.Storage
	doltServer *DoltServerManager
	krcPruner  *KRCPruner

	// Mass death detection: track recent session deaths
	deathsMu     sync.Mutex
	recentDeaths []sessionDeath

	// Deacon startup tracking: prevents race condition where newly started
	// sessions are immediately killed by the heartbeat check.
	// See: https://github.com/steveyegge/gastown/issues/567
	// Note: Only accessed from heartbeat loop goroutine - no sync needed.
	deaconLastStarted time.Time

	// syncFailures tracks consecutive git pull failures per workdir.
	// Used to escalate logging from WARN to ERROR after repeated failures.
	// Only accessed from heartbeat loop goroutine - no sync needed.
	syncFailures map[string]int

	// PATCH-006: Resolved binary paths to avoid PATH issues in subprocesses.
	gtPath string
	bdPath string

	// Boot spawn cooldown: prevents Boot from spawning on every heartbeat tick.
	// Only accessed from heartbeat loop goroutine - no sync needed.
	bootLastSpawned time.Time

	// Restart tracking with exponential backoff to prevent crash loops
	restartTracker *RestartTracker

	// telemetry exports metrics and logs to VictoriaMetrics / VictoriaLogs.
	// Nil when telemetry is disabled (GT_OTEL_METRICS_URL / GT_OTEL_LOGS_URL not set).
	otelProvider *telemetry.Provider
	metrics      *daemonMetrics

	// jsonlPushFailures tracks consecutive git push failures for JSONL backup.
	// Only accessed from heartbeat loop goroutine - no sync needed.
	jsonlPushFailures int

	// lastDoctorMolTime tracks when the last mol-dog-doctor molecule was poured.
	// Option B throttling: only pour when anomaly detected AND cooldown elapsed.
	// Only accessed from heartbeat loop goroutine - no sync needed.
	lastDoctorMolTime time.Time

	// lastMaintenanceRun tracks when scheduled maintenance last ran.
	// Only accessed from heartbeat loop goroutine - no sync needed.
	lastMaintenanceRun time.Time
}

// sessionDeath records a detected session death for mass death analysis.
type sessionDeath struct {
	sessionName string
	timestamp   time.Time
}

// Mass death detection parameters — these are fallback defaults.
// Prefer config.OperationalConfig.GetDaemonConfig() accessors when
// a TownSettings is available (loaded via d.loadOperationalConfig()).
const (
	massDeathWindow    = 30 * time.Second // Time window to detect mass death
	massDeathThreshold = 3                // Number of deaths to trigger alert

	// hungSessionThreshold is how long a refinery/witness session can be
	// inactive (no tmux output) before the daemon considers it hung and
	// kills it for restart. Derived from constants.HungSessionThreshold
	// (single source of truth). See: gt-tr3d
	hungSessionThreshold = constants.HungSessionThreshold

	// doctorMolCooldown is the minimum interval between mol-dog-doctor molecules.
	// Configurable via operational.daemon.doctor_mol_cooldown.
	doctorMolCooldown = 5 * time.Minute
)

// New creates a new daemon instance.
func New(config *Config) (*Daemon, error) {
	// Ensure daemon directory exists
	daemonDir := filepath.Dir(config.LogFile)
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return nil, fmt.Errorf("creating daemon directory: %w", err)
	}

	// Open log file with rotation (100MB max, 3 backups, 7 days, compressed)
	logWriter := &lumberjack.Logger{
		Filename:   config.LogFile,
		MaxSize:    100, // megabytes
		MaxBackups: 3,
		MaxAge:     7, // days
		Compress:   true,
	}

	logger := log.New(logWriter, "", log.LstdFlags)
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize session prefix and agent registries from town root.
	if err := session.InitRegistry(config.TownRoot); err != nil {
		logger.Printf("Warning: failed to initialize town registry: %v", err)
	}

	// Set GT_TOWN_ROOT in tmux global environment so run-shell subprocesses
	// (e.g., gt cycle next/prev) can find the workspace even when CWD is $HOME.
	// Non-fatal: tmux server may not be running yet — daemon creates sessions shortly.
	t := tmux.NewTmux()
	if err := t.SetGlobalEnvironment("GT_TOWN_ROOT", config.TownRoot); err != nil {
		logger.Printf("Warning: failed to set GT_TOWN_ROOT in tmux global env: %v", err)
	}

	// Load patrol config from mayor/daemon.json, ensuring lifecycle defaults
	// are populated for any missing data maintenance tickers. Without this,
	// opt-in patrols (compactor, reaper, doctor, JSONL backup, dolt backup)
	// remain disabled if the file was created before they were implemented.
	if err := EnsureLifecycleConfigFile(config.TownRoot); err != nil {
		logger.Printf("Warning: failed to ensure lifecycle config: %v", err)
	}
	patrolConfig := LoadPatrolConfig(config.TownRoot)
	if patrolConfig != nil {
		logger.Printf("Loaded patrol config from %s", PatrolConfigFile(config.TownRoot))
		// Propagate env vars from daemon.json to this process and all spawned sessions.
		for k, v := range patrolConfig.Env {
			os.Setenv(k, v)
			logger.Printf("Set env %s=%s from daemon.json", k, v)
		}
	}

	// Initialize Dolt server manager if configured
	var doltServer *DoltServerManager
	if patrolConfig != nil && patrolConfig.Patrols != nil && patrolConfig.Patrols.DoltServer != nil {
		doltServer = NewDoltServerManager(config.TownRoot, patrolConfig.Patrols.DoltServer, logger.Printf)
		if doltServer.IsEnabled() {
			logger.Printf("Dolt server management enabled (port %d)", patrolConfig.Patrols.DoltServer.Port)
		}
	}

	// PATCH-006: Resolve binary paths at startup.
	gtPath, err := exec.LookPath("gt")
	if err != nil {
		gtPath = "gt"
		logger.Printf("Warning: gt not found in PATH, subprocess calls may fail")
	}
	bdPath, err := exec.LookPath("bd")
	if err != nil {
		bdPath = "bd"
		logger.Printf("Warning: bd not found in PATH, subprocess calls may fail")
	}

	// Initialize restart tracker with exponential backoff.
	// Parameters are configurable via patrols.restart_tracker in daemon.json.
	var rtCfg RestartTrackerConfig
	if patrolConfig != nil && patrolConfig.Patrols != nil && patrolConfig.Patrols.RestartTracker != nil {
		rtCfg = *patrolConfig.Patrols.RestartTracker
	}
	restartTracker := NewRestartTracker(config.TownRoot, rtCfg)
	if err := restartTracker.Load(); err != nil {
		logger.Printf("Warning: failed to load restart state: %v", err)
	}

	// Initialize OpenTelemetry (best-effort — telemetry failure never blocks startup).
	// Activate by setting GT_OTEL_METRICS_URL and/or GT_OTEL_LOGS_URL.
	otelProvider, otelErr := telemetry.Init(ctx, "gastown-daemon", "")
	if otelErr != nil {
		logger.Printf("Warning: telemetry init failed: %v", otelErr)
	}
	var dm *daemonMetrics
	if otelProvider != nil {
		dm, err = newDaemonMetrics()
		if err != nil {
			logger.Printf("Warning: failed to register daemon metrics: %v", err)
			dm = nil
		} else {
			metricsURL := os.Getenv(telemetry.EnvMetricsURL)
			if metricsURL == "" {
				metricsURL = telemetry.DefaultMetricsURL
			}
			logsURL := os.Getenv(telemetry.EnvLogsURL)
			if logsURL == "" {
				logsURL = telemetry.DefaultLogsURL
			}
			logger.Printf("Telemetry active (metrics → %s, logs → %s)",
				metricsURL, logsURL)
		}
	}

	return &Daemon{
		config:         config,
		patrolConfig:   patrolConfig,
		tmux:           tmux.NewTmux(),
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
		doltServer:     doltServer,
		gtPath:         gtPath,
		bdPath:         bdPath,
		restartTracker: restartTracker,
		otelProvider:   otelProvider,
		metrics:        dm,
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

	// Pre-flight check: all rigs must be on Dolt backend.
	if err := d.checkAllRigsDolt(); err != nil {
		return err
	}

	// Repair metadata.json for all rigs on startup.
	// This ensures all rigs have proper Dolt server configuration.
	if _, errs := doltserver.EnsureAllMetadata(d.config.TownRoot); len(errs) > 0 {
		for _, e := range errs {
			d.logger.Printf("Warning: metadata repair: %v", e)
		}
	}

	// Write PID file with nonce for ownership verification
	if _, err := writePIDFile(d.config.PidFile, os.Getpid()); err != nil {
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
	timer := time.NewTimer(d.recoveryHeartbeatInterval())
	defer timer.Stop()

	d.logger.Printf("Daemon running, recovery heartbeat interval %v", d.recoveryHeartbeatInterval())

	// Start feed curator goroutine
	d.curator = feed.NewCurator(d.config.TownRoot)
	if err := d.curator.Start(); err != nil {
		d.logger.Printf("Warning: failed to start feed curator: %v", err)
	} else {
		d.logger.Println("Feed curator started")
	}

	// Start convoy manager (event-driven + periodic stranded scan)
	// Try opening beads stores eagerly; if Dolt isn't ready yet,
	// pass the opener as a callback for lazy retry on each poll tick.
	d.beadsStores = d.openBeadsStores()
	isRigParked := func(rigName string) bool {
		ok, _ := d.isRigOperational(rigName)
		return !ok
	}
	var storeOpener func() map[string]beadsdk.Storage
	if len(d.beadsStores) == 0 {
		storeOpener = d.openBeadsStores
	}
	d.convoyManager = NewConvoyManager(d.config.TownRoot, d.logger.Printf, d.gtPath, 0, d.beadsStores, storeOpener, isRigParked)
	if err := d.convoyManager.Start(); err != nil {
		d.logger.Printf("Warning: failed to start convoy manager: %v", err)
	} else {
		d.logger.Println("Convoy manager started")
	}

	// Wire a recovery callback so that when Dolt transitions from unhealthy
	// back to healthy, the convoy manager runs a sweep to catch any convoys
	// that completed during the outage and were missed by the event poller.
	if d.doltServer != nil {
		cm := d.convoyManager
		d.doltServer.SetRecoveryCallback(func() {
			d.logger.Printf("Dolt recovery detected: triggering convoy recovery sweep")
			cm.scan()
		})
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

	// Start dedicated Dolt health check ticker if Dolt server is configured.
	// This runs at a much higher frequency (default 30s) than the general
	// heartbeat (3 min) so Dolt crashes are detected quickly.
	var doltHealthTicker *time.Ticker
	var doltHealthChan <-chan time.Time
	if d.doltServer != nil && d.doltServer.IsEnabled() {
		interval := d.doltServer.HealthCheckInterval()
		doltHealthTicker = time.NewTicker(interval)
		doltHealthChan = doltHealthTicker.C
		defer doltHealthTicker.Stop()
		d.logger.Printf("Dolt health check ticker started (interval %v)", interval)
	}

	// Start dedicated Dolt remotes push ticker if configured.
	// This runs at a lower frequency (default 15 min) than the heartbeat (3 min)
	// to periodically push databases to their git remotes.
	var doltRemotesTicker *time.Ticker
	var doltRemotesChan <-chan time.Time
	if IsPatrolEnabled(d.patrolConfig, "dolt_remotes") {
		interval := doltRemotesInterval(d.patrolConfig)
		doltRemotesTicker = time.NewTicker(interval)
		doltRemotesChan = doltRemotesTicker.C
		defer doltRemotesTicker.Stop()
		d.logger.Printf("Dolt remotes push ticker started (interval %v)", interval)
	}

	// Start dedicated Dolt backup ticker if configured.
	// Runs filesystem backup sync (dolt backup sync) for production databases.
	var doltBackupTicker *time.Ticker
	var doltBackupChan <-chan time.Time
	if IsPatrolEnabled(d.patrolConfig, "dolt_backup") {
		interval := doltBackupInterval(d.patrolConfig)
		doltBackupTicker = time.NewTicker(interval)
		doltBackupChan = doltBackupTicker.C
		defer doltBackupTicker.Stop()
		d.logger.Printf("Dolt backup ticker started (interval %v)", interval)
	}

	// Start JSONL git backup ticker if configured.
	// Exports issues to JSONL, scrubs ephemeral data, pushes to git repo.
	var jsonlGitBackupTicker *time.Ticker
	var jsonlGitBackupChan <-chan time.Time
	if IsPatrolEnabled(d.patrolConfig, "jsonl_git_backup") {
		interval := jsonlGitBackupInterval(d.patrolConfig)
		jsonlGitBackupTicker = time.NewTicker(interval)
		jsonlGitBackupChan = jsonlGitBackupTicker.C
		defer jsonlGitBackupTicker.Stop()
		d.logger.Printf("JSONL git backup ticker started (interval %v)", interval)
	}

	// Start wisp reaper ticker if configured.
	// Closes stale wisps (abandoned molecule steps, old patrol data) across all databases.
	var wispReaperTicker *time.Ticker
	var wispReaperChan <-chan time.Time
	if IsPatrolEnabled(d.patrolConfig, "wisp_reaper") {
		interval := wispReaperInterval(d.patrolConfig)
		wispReaperTicker = time.NewTicker(interval)
		wispReaperChan = wispReaperTicker.C
		defer wispReaperTicker.Stop()
		d.logger.Printf("Wisp reaper ticker started (interval %v)", interval)
	}

	// Start doctor dog ticker if configured.
	// Health monitor: TCP check, latency, DB count, gc, zombie detection, backup/disk checks.
	var doctorDogTicker *time.Ticker
	var doctorDogChan <-chan time.Time
	if IsPatrolEnabled(d.patrolConfig, "doctor_dog") {
		interval := doctorDogInterval(d.patrolConfig)
		doctorDogTicker = time.NewTicker(interval)
		doctorDogChan = doctorDogTicker.C
		defer doctorDogTicker.Stop()
		d.logger.Printf("Doctor dog ticker started (interval %v)", interval)
	}

	// Start compactor dog ticker if configured.
	// Flattens Dolt commit history to reclaim graph storage (daily).
	var compactorDogTicker *time.Ticker
	var compactorDogChan <-chan time.Time
	if IsPatrolEnabled(d.patrolConfig, "compactor_dog") {
		interval := compactorDogInterval(d.patrolConfig)
		compactorDogTicker = time.NewTicker(interval)
		compactorDogChan = compactorDogTicker.C
		defer compactorDogTicker.Stop()
		d.logger.Printf("Compactor dog ticker started (interval %v)", interval)
	}

	// Start scheduled maintenance ticker if configured.
	// Checks periodically whether we're in the maintenance window and
	// runs `gt maintain --force` when commit counts exceed threshold.
	var scheduledMaintenanceTicker *time.Ticker
	var scheduledMaintenanceChan <-chan time.Time
	if IsPatrolEnabled(d.patrolConfig, "scheduled_maintenance") {
		interval := maintenanceCheckInterval(d.patrolConfig)
		scheduledMaintenanceTicker = time.NewTicker(interval)
		scheduledMaintenanceChan = scheduledMaintenanceTicker.C
		defer scheduledMaintenanceTicker.Stop()
		window := maintenanceWindow(d.patrolConfig)
		d.logger.Printf("Scheduled maintenance ticker started (check interval %v, window %s)", interval, window)
	}

	// Note: PATCH-010 uses per-session hooks in deacon/manager.go (SetAutoRespawnHook).
	// Global pane-died hooks don't fire reliably in tmux 3.2a, so we rely on the
	// per-session approach which has been tested to work for continuous recovery.

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
			} else if isReloadRestartSignal(sig) {
				// Reload restart tracker from disk (from 'gt daemon clear-backoff')
				d.logger.Println("Received reload-restart signal, reloading restart tracker from disk")
				if d.restartTracker != nil {
					if err := d.restartTracker.Load(); err != nil {
						d.logger.Printf("Warning: failed to reload restart tracker: %v", err)
					}
				}
			} else {
				d.logger.Printf("Received signal %v, shutting down", sig)
				return d.shutdown(state)
			}

		case <-doltHealthChan:
			// Dedicated Dolt health check — fast crash detection independent
			// of the 3-minute general heartbeat.
			if !d.isShutdownInProgress() {
				d.ensureDoltServerRunning()
			}

		case <-doltRemotesChan:
			// Periodic Dolt remote push — pushes databases to their configured
			// git remotes on a 15-minute cadence (independent of heartbeat).
			if !d.isShutdownInProgress() {
				d.pushDoltRemotes()
			}

		case <-doltBackupChan:
			// Periodic Dolt filesystem backup — syncs production databases to
			// local backup directory on a 15-minute cadence.
			if !d.isShutdownInProgress() {
				d.syncDoltBackups()
			}

		case <-jsonlGitBackupChan:
			// Periodic JSONL git backup — exports issues, scrubs ephemeral data,
			// commits and pushes to git repo.
			if !d.isShutdownInProgress() {
				d.syncJsonlGitBackup()
			}

		case <-wispReaperChan:
			// Periodic wisp reaper — closes stale wisps (abandoned molecule steps,
			// old patrol data) to prevent unbounded table growth (Clown Show audit).
			if !d.isShutdownInProgress() {
				d.reapWisps()
			}

		case <-doctorDogChan:
			// Doctor dog — comprehensive Dolt health monitor: connectivity, latency,
			// gc, zombie detection, backup staleness, and disk usage checks.
			if !d.isShutdownInProgress() {
				d.runDoctorDog()
			}

		case <-compactorDogChan:
			// Compactor dog — flattens Dolt commit history on production databases.
			// Reclaims commit graph storage, then runs gc to reclaim chunks.
			if !d.isShutdownInProgress() {
				d.runCompactorDog()
			}

		case <-scheduledMaintenanceChan:
			// Scheduled maintenance — checks if we're in the maintenance window
			// and runs `gt maintain --force` when commit counts exceed threshold.
			if !d.isShutdownInProgress() {
				d.runScheduledMaintenance()
			}

		case <-timer.C:
			d.heartbeat(state)

			// Fixed recovery interval (no activity-based backoff)
			timer.Reset(d.recoveryHeartbeatInterval())
		}
	}
}

// recoveryHeartbeatInterval returns the config-driven recovery heartbeat interval.
// Normal wake is handled by feed subscription (bd activity --follow).
// The daemon is a safety net for dead sessions, GUPP violations, and orphaned work.
// Default: 3 minutes — fast enough to detect stuck agents promptly.
func (d *Daemon) recoveryHeartbeatInterval() time.Duration {
	return d.loadOperationalConfig().GetDaemonConfig().RecoveryHeartbeatIntervalD()
}

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

	d.metrics.recordHeartbeat(d.ctx)
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
		// Kill leftover deacon/boot sessions from before patrol was disabled.
		// Without this, a stale deacon keeps running its own patrol loop,
		// spawning witnesses and refineries despite daemon config. (hq-2mstj)
		d.killDeaconSessions()
	}

	// 2. Poke Boot for intelligent triage (stuck/nudge/interrupt)
	// Boot handles nuanced "is Deacon responsive" decisions
	// Only run if Deacon patrol is enabled
	if IsPatrolEnabled(d.patrolConfig, "deacon") {
		d.ensureBootRunning()
	}

	// 3. Direct Deacon heartbeat check (belt-and-suspenders)
	// Boot may not detect all stuck states; this provides a fallback
	// Only run if Deacon patrol is enabled
	if IsPatrolEnabled(d.patrolConfig, "deacon") {
		d.checkDeaconHeartbeat()
	}

	// 4. Ensure Witnesses are running for all rigs (restart if dead)
	// Check patrol config - can be disabled in mayor/daemon.json
	if IsPatrolEnabled(d.patrolConfig, "witness") {
		d.ensureWitnessesRunning()
	} else {
		d.logger.Printf("Witness patrol disabled in config, skipping")
		// Kill leftover witness sessions from before patrol was disabled. (hq-2mstj)
		d.killWitnessSessions()
	}

	// 5. Ensure Refineries are running for all rigs (restart if dead)
	// Check patrol config - can be disabled in mayor/daemon.json
	if IsPatrolEnabled(d.patrolConfig, "refinery") {
		d.ensureRefineriesRunning()
	} else {
		d.logger.Printf("Refinery patrol disabled in config, skipping")
		// Kill leftover refinery sessions from before patrol was disabled. (hq-2mstj)
		d.killRefinerySessions()
	}

	// 6. Ensure Mayor is running (restart if dead)
	d.ensureMayorRunning()

	// 6.5. Handle Dog lifecycle: cleanup stuck dogs and dispatch plugins
	if IsPatrolEnabled(d.patrolConfig, "handler") {
		d.handleDogs()
	} else {
		d.logger.Printf("Handler patrol disabled in config, skipping")
	}

	// 7. Process lifecycle requests
	d.processLifecycleRequests()

	// 9. (Removed) Stale agent check - violated "discover, don't track"

	// 10. Check for GUPP violations (agents with work-on-hook not progressing)
	d.checkGUPPViolations()

	// 11. Check for orphaned work (assigned to dead agents)
	d.checkOrphanedWork()

	// 12. Check polecat session health (proactive crash detection)
	// This validates tmux sessions are still alive for polecats with work-on-hook
	d.checkPolecatSessionHealth()

	// 13. Clean up orphaned claude subagent processes (memory leak prevention)
	// These are Task tool subagents that didn't clean up after completion.
	// This is a safety net - Deacon patrol also does this more frequently.
	d.cleanupOrphanedProcesses()

	// 13. Prune stale local polecat tracking branches across all rig clones.
	// When polecats push branches to origin, other clones create local tracking
	// branches via git fetch. After merge, remote branches are deleted but local
	// branches persist indefinitely. This cleans them up periodically.
	d.pruneStaleBranches()

	// 14. Dispatch scheduled work (capacity-controlled polecat dispatch).
	// Shells out to `gt scheduler run` to avoid circular import between daemon and cmd.
	d.dispatchQueuedWork()

	// 15. Rotate oversized Dolt logs (copytruncate for child process fds).
	// daemon.log uses lumberjack for automatic rotation; this handles Dolt server logs.
	d.rotateOversizedLogs()

	// Update state
	state.LastHeartbeat = time.Now()
	state.HeartbeatCount++
	if err := SaveState(d.config.TownRoot, state); err != nil {
		d.logger.Printf("Warning: failed to save state: %v", err)
	}

	d.logger.Printf("Heartbeat complete (#%d)", state.HeartbeatCount)
}

// rotateOversizedLogs checks Dolt server log files and rotates any that exceed
// the size threshold. Uses copytruncate which is safe for logs held open by
// child processes. Runs every heartbeat but is cheap (just stat calls).
func (d *Daemon) rotateOversizedLogs() {
	result := RotateLogs(d.config.TownRoot)
	for _, path := range result.Rotated {
		d.logger.Printf("log_rotation: rotated %s", path)
	}
	for _, err := range result.Errors {
		d.logger.Printf("log_rotation: error: %v", err)
	}
}

// ensureDoltServerRunning ensures the Dolt SQL server is running if configured.
// This provides the backend for beads database access in server mode.
// Option B throttling: pours a mol-dog-doctor molecule only when health check
// warnings are detected, with a 5-minute cooldown to avoid wisp spam.
func (d *Daemon) ensureDoltServerRunning() {
	if d.doltServer == nil || !d.doltServer.IsEnabled() {
		return
	}

	if err := d.doltServer.EnsureRunning(); err != nil {
		d.logger.Printf("Error ensuring Dolt server is running: %v", err)
	}

	// Option B throttling: pour mol-dog-doctor only on anomaly with cooldown.
	if warnings := d.doltServer.LastWarnings(); len(warnings) > 0 {
		if time.Since(d.lastDoctorMolTime) >= doctorMolCooldown {
			d.lastDoctorMolTime = time.Now()
			go d.pourDoctorMolecule(warnings)
		}
	}

	// Update OTel gauges with the latest Dolt health snapshot.
	if d.metrics != nil {
		h := doltserver.GetHealthMetrics(d.config.TownRoot)
		d.metrics.updateDoltHealth(
			int64(h.Connections),
			int64(h.MaxConnections),
			float64(h.QueryLatency.Milliseconds()),
			h.DiskUsageBytes,
			h.Healthy,
		)
	}
}

// pourDoctorMolecule creates a mol-dog-doctor molecule to track a health anomaly.
// Runs asynchronously — molecule lifecycle is observability, not control flow.
func (d *Daemon) pourDoctorMolecule(warnings []string) {
	mol := d.pourDogMolecule(constants.MolDogDoctor, map[string]string{
		"port": strconv.Itoa(d.doltServer.config.Port),
	})
	defer mol.close()

	// Step 1: probe — connectivity was already checked (we got here because it passed).
	mol.closeStep("probe")

	// Step 2: inspect — resource checks produced the warnings.
	mol.closeStep("inspect")

	// Step 3: report — log the warning summary.
	summary := strings.Join(warnings, "; ")
	d.logger.Printf("Doctor molecule: %d warning(s): %s", len(warnings), summary)
	mol.closeStep("report")
}


// checkAllRigsDolt verifies all rigs are using the Dolt backend.
func (d *Daemon) checkAllRigsDolt() error {
	var problems []string

	// Check town-level beads
	townBeadsDir := filepath.Join(d.config.TownRoot, ".beads")
	if backend := readBeadsBackend(townBeadsDir); backend != "" && backend != "dolt" {
		problems = append(problems, fmt.Sprintf(
			"Rig %q is using %s backend.\n  Gas Town requires Dolt. Run: cd %s && bd migrate dolt",
			"town-root", backend, d.config.TownRoot))
	}

	// Check each registered rig
	for _, rigName := range d.getKnownRigs() {
		rigBeadsDir := filepath.Join(d.config.TownRoot, rigName, "mayor", "rig", ".beads")
		if backend := readBeadsBackend(rigBeadsDir); backend != "" && backend != "dolt" {
			rigPath := filepath.Join(d.config.TownRoot, rigName)
			problems = append(problems, fmt.Sprintf(
				"Rig %q is using %s backend.\n  Gas Town requires Dolt. Run: cd %s && bd migrate dolt",
				rigName, backend, rigPath))
		}
	}

	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("daemon startup blocked: %d rig(s) not on Dolt backend\n\n  %s",
		len(problems), strings.Join(problems, "\n\n  "))
}

// readBeadsBackend reads the backend field from metadata.json in a beads directory.
// Returns empty string if the directory or metadata doesn't exist.
func readBeadsBackend(beadsDir string) string {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return ""
	}

	var metadata struct {
		Backend string `json:"backend"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return ""
	}

	return metadata.Backend
}

// DeaconRole is the role name for the Deacon's handoff bead.
const DeaconRole = "deacon"

// getDeaconSessionName returns the Deacon session name for the daemon's town.
func (d *Daemon) getDeaconSessionName() string {
	return session.DeaconSessionName()
}

// ensureBootRunning spawns Boot to triage the Deacon.
// Boot is a fresh-each-tick watchdog that decides whether to start/wake/nudge
// the Deacon, centralizing the "when to wake" decision in an agent.
// In degraded mode (no tmux), falls back to mechanical checks.
// bootSpawnCooldown returns the config-driven boot spawn cooldown.
// Boot triage runs are expensive (AI reasoning); if one just ran, skip.
func (d *Daemon) bootSpawnCooldown() time.Duration {
	return d.loadOperationalConfig().GetDaemonConfig().BootSpawnCooldownD()
}

func (d *Daemon) ensureBootRunning() {
	// Cooldown gate: skip if Boot was spawned recently (fixes #2084)
	if !d.bootLastSpawned.IsZero() && time.Since(d.bootLastSpawned) < d.bootSpawnCooldown() {
		d.logger.Printf("Boot spawned %s ago, within cooldown (%s), skipping",
			time.Since(d.bootLastSpawned).Round(time.Second), d.bootSpawnCooldown())
		return
	}

	b := boot.New(d.config.TownRoot)

	// Check for degraded mode
	degraded := os.Getenv("GT_DEGRADED") == "true"
	if degraded || !d.tmux.IsAvailable() {
		// In degraded mode, run mechanical triage directly
		d.logger.Println("Degraded mode: running mechanical Boot triage")
		d.runDegradedBootTriage(b)
		return
	}

	// Spawn Boot in a fresh tmux session
	d.logger.Println("Spawning Boot for triage...")
	if err := b.Spawn(""); err != nil {
		d.logger.Printf("Error spawning Boot: %v, falling back to direct Deacon check", err)
		// Fallback: ensure Deacon is running directly
		d.ensureDeaconRunning()
		return
	}

	d.bootLastSpawned = time.Now()
	d.logger.Println("Boot spawned successfully")
}

// runDegradedBootTriage performs mechanical Boot logic without AI reasoning.
// This is for degraded mode when tmux is unavailable.
func (d *Daemon) runDegradedBootTriage(b *boot.Boot) {
	startTime := time.Now()
	status := &boot.Status{
		StartedAt: startTime,
	}

	// Simple check: is Deacon session alive?
	hasDeacon, err := d.tmux.HasSession(d.getDeaconSessionName())
	if err != nil {
		d.logger.Printf("Error checking Deacon session: %v", err)
		status.LastAction = "error"
		status.Error = err.Error()
	} else if !hasDeacon {
		d.logger.Println("Deacon not running, starting...")
		d.ensureDeaconRunning()
		status.LastAction = "start"
		status.Target = "deacon"
	} else {
		status.LastAction = "nothing"
	}

	status.CompletedAt = time.Now()

	if err := b.SaveStatus(status); err != nil {
		d.logger.Printf("Warning: failed to save Boot status: %v", err)
	}
}

// ensureDeaconRunning ensures the Deacon is running.
// Uses deacon.Manager for consistent startup behavior (WaitForShellReady, GUPP, etc.).
func (d *Daemon) ensureDeaconRunning() {
	const agentID = "deacon"

	// Check restart tracker for backoff/crash loop
	if d.restartTracker != nil {
		if d.restartTracker.IsInCrashLoop(agentID) {
			d.logger.Printf("Deacon is in crash loop, skipping restart (use 'gt daemon clear-backoff deacon' to reset)")
			return
		}
		if !d.restartTracker.CanRestart(agentID) {
			remaining := d.restartTracker.GetBackoffRemaining(agentID)
			d.logger.Printf("Deacon restart in backoff, %s remaining", remaining.Round(time.Second))
			return
		}
	}

	mgr := deacon.NewManager(d.config.TownRoot)

	if err := mgr.Start(""); err != nil {
		if err == deacon.ErrAlreadyRunning {
			// Deacon is running - record success to reset backoff
			if d.restartTracker != nil {
				d.restartTracker.RecordSuccess(agentID)
			}
			return
		}
		d.logger.Printf("Error starting Deacon: %v", err)
		return
	}

	// Record this restart attempt for backoff tracking
	if d.restartTracker != nil {
		d.restartTracker.RecordRestart(agentID)
		if err := d.restartTracker.Save(); err != nil {
			d.logger.Printf("Warning: failed to save restart state: %v", err)
		}
	}

	// Track when we started the Deacon to prevent race condition in checkDeaconHeartbeat.
	// The heartbeat file will still be stale until the Deacon runs a full patrol cycle.
	d.deaconLastStarted = time.Now()
	d.metrics.recordRestart(d.ctx, "deacon")
	telemetry.RecordDaemonRestart(d.ctx, "deacon")
	d.logger.Println("Deacon started successfully")
}

// deaconGracePeriod returns the config-driven deacon grace period.
// The Deacon needs time to initialize Claude, run SessionStart hooks, execute gt prime,
// run a patrol cycle, and write a fresh heartbeat. Default: 5 minutes.
func (d *Daemon) deaconGracePeriod() time.Duration {
	return d.loadOperationalConfig().GetDaemonConfig().DeaconGracePeriodD()
}

// checkDeaconHeartbeat checks if the Deacon is making progress.
// This is a belt-and-suspenders fallback in case Boot doesn't detect stuck states.
// Uses the heartbeat file that the Deacon updates on each patrol cycle.
//
// PATCH-005: Fixed grace period logic. Old logic skipped heartbeat check entirely
// during grace period, allowing stuck Deacons to go undetected. New logic:
// - Always read heartbeat first
// - Grace period only applies if heartbeat is from BEFORE we started Deacon
// - If heartbeat is from AFTER start but stale, Deacon is stuck
func (d *Daemon) checkDeaconHeartbeat() {
	// Respect crash-loop guard: if the restart tracker says Deacon is in a
	// crash loop, do not kill the session — the guard is deliberately holding
	// off restarts to break the cycle. (Fixes #2086)
	if d.restartTracker != nil && d.restartTracker.IsInCrashLoop("deacon") {
		d.logger.Printf("Deacon is in crash-loop state, skipping heartbeat kill check")
		return
	}

	// Always read heartbeat first (PATCH-005)
	hb := deacon.ReadHeartbeat(d.config.TownRoot)

	sessionName := d.getDeaconSessionName()

	// Check if we recently started a Deacon
	if !d.deaconLastStarted.IsZero() {
		timeSinceStart := time.Since(d.deaconLastStarted)

		if hb == nil {
			// No heartbeat file exists
			if timeSinceStart < d.deaconGracePeriod() {
				d.logger.Printf("Deacon started %s ago, awaiting first heartbeat...",
					timeSinceStart.Round(time.Second))
				return
			}
			// Grace period expired without any heartbeat - Deacon failed to start
			d.logger.Printf("Deacon started %s ago but hasn't written heartbeat - restarting",
				timeSinceStart.Round(time.Minute))
			d.restartStuckDeacon(sessionName)
			return
		}

		// Heartbeat exists - check if it's from BEFORE we started this Deacon
		if hb.Timestamp.Before(d.deaconLastStarted) {
			// Heartbeat is stale (from before restart)
			if timeSinceStart < d.deaconGracePeriod() {
				d.logger.Printf("Deacon started %s ago, heartbeat is pre-restart, awaiting fresh heartbeat...",
					timeSinceStart.Round(time.Second))
				return
			}
			// Grace period expired but heartbeat still from before start
			d.logger.Printf("Deacon started %s ago but heartbeat still pre-restart - Deacon stuck at startup",
				timeSinceStart.Round(time.Minute))
			d.restartStuckDeacon(sessionName)
			return
		}

		// Heartbeat is from AFTER we started - Deacon has written at least one heartbeat
		// Fall through to normal staleness check
	}

	// No recent start tracking or Deacon has written fresh heartbeat - check normally
	if hb == nil {
		// No heartbeat file - Deacon hasn't started a cycle yet
		return
	}

	age := hb.Age()

	// If heartbeat is fresh, nothing to do
	if !hb.IsVeryStale() {
		return
	}

	d.logger.Printf("Deacon heartbeat is stale (%s old), checking session...", age.Round(time.Minute))

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
	// PATCH-002: Reduced from 30m to 10m for faster recovery.
	// Must be > backoff-max (5m) to avoid false positive kills during legitimate sleep.
	if age > 10*time.Minute {
		d.restartStuckDeacon(sessionName)
	} else {
		// Stuck but not critically - nudge to wake up
		d.logger.Printf("Deacon stuck for %s - nudging session", age.Round(time.Minute))
		if err := d.tmux.NudgeSession(sessionName, "HEALTH_CHECK: heartbeat stale, respond to confirm responsiveness"); err != nil {
			d.logger.Printf("Error nudging stuck Deacon: %v", err)
		}
	}
}

// restartStuckDeacon kills and restarts a stuck Deacon session.
// Extracted for reuse by PATCH-005 grace period logic.
func (d *Daemon) restartStuckDeacon(sessionName string) {
	// Check if session exists before trying to kill
	hasSession, _ := d.tmux.HasSession(sessionName)
	if hasSession {
		d.logger.Printf("Killing stuck Deacon session %s", sessionName)
		if err := d.tmux.KillSessionWithProcesses(sessionName); err != nil {
			d.logger.Printf("Error killing stuck Deacon: %v", err)
		}
	}
	// Spawn new Deacon immediately
	d.ensureDeaconRunning()
}

// ensureWitnessesRunning ensures witnesses are running for configured rigs.
// Called on each heartbeat to maintain witness patrol loops.
// Respects the rigs filter in daemon.json patrol config.
func (d *Daemon) ensureWitnessesRunning() {
	rigs := d.getPatrolRigs("witness")
	for _, rigName := range rigs {
		d.ensureWitnessRunning(rigName)
	}
}

// hasPendingEvents checks if there are pending .event files in the given channel directory.
// Used to gate agent spawning: don't burn API credits starting a Claude session when
// there's nothing to process. The agent's await-event handles the actual consumption.
func (d *Daemon) hasPendingEvents(channel string) bool {
	eventDir := filepath.Join(d.config.TownRoot, "events", channel)
	entries, err := os.ReadDir(eventDir)
	if err != nil {
		return false // Directory doesn't exist or unreadable = no pending events
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".event") {
			return true
		}
	}
	return false
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

	// Check for hung session before Start (which only detects process-dead zombies).
	// A hung session has a live process but no tmux activity for an extended period,
	// indicating Claude is stuck. Kill it so Start() can recreate a fresh one.
	if status := mgr.IsHealthy(hungSessionThreshold); status == tmux.AgentHung {
		d.logger.Printf("Witness for %s is hung (no activity for %v), killing for restart", rigName, hungSessionThreshold)
		t := tmux.NewTmux()
		_ = t.KillSession(mgr.SessionName())
	}

	if err := mgr.Start(false, "", nil); err != nil {
		if err == witness.ErrAlreadyRunning {
			// Already running - this is the expected case
			d.logger.Printf("Witness for %s already running, skipping spawn", rigName)
			return
		}
		d.logger.Printf("Error starting witness for %s: %v", rigName, err)
		return
	}

	d.metrics.recordRestart(d.ctx, "witness")
	telemetry.RecordDaemonRestart(d.ctx, "witness-"+rigName)
	d.logger.Printf("Witness session for %s started successfully", rigName)
}

// ensureRefineriesRunning ensures refineries are running for configured rigs.
// Called on each heartbeat to maintain refinery merge queue processing.
// Respects the rigs filter in daemon.json patrol config.
func (d *Daemon) ensureRefineriesRunning() {
	rigs := d.getPatrolRigs("refinery")
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

	// Event gate: don't spawn a new Claude session when there's nothing to process.
	// If a refinery session is already running, Start() returns ErrAlreadyRunning (cheap).
	// But spawning a NEW session with an empty queue burns API credits for nothing.
	// The refinery formula uses await-event internally, so it will wake when events appear.
	if !d.hasPendingEvents("refinery") {
		// Check if session already exists before skipping — let running sessions continue
		r := &rig.Rig{
			Name: rigName,
			Path: filepath.Join(d.config.TownRoot, rigName),
		}
		mgr := refinery.NewManager(r)
		if running, _ := mgr.IsRunning(); !running {
			d.logger.Printf("No pending refinery events and no session running for %s, skipping spawn", rigName)
			return
		}
	}

	// Manager.Start() handles: zombie detection, session creation, env vars, theming,
	// WaitForClaudeReady, and crucially - startup/propulsion nudges (GUPP).
	// It returns ErrAlreadyRunning if Claude is already running in tmux.
	r := &rig.Rig{
		Name: rigName,
		Path: filepath.Join(d.config.TownRoot, rigName),
	}
	mgr := refinery.NewManager(r)

	// Check for hung session before Start (which only detects process-dead zombies).
	// A hung refinery means MRs pile up with no processing. Kill it so Start()
	// can recreate a fresh one. See: gt-tr3d
	if status := mgr.IsHealthy(hungSessionThreshold); status == tmux.AgentHung {
		d.logger.Printf("Refinery for %s is hung (no activity for %v), killing for restart", rigName, hungSessionThreshold)
		t := tmux.NewTmux()
		_ = t.KillSession(mgr.SessionName())
	}

	if err := mgr.Start(false, ""); err != nil {
		if err == refinery.ErrAlreadyRunning {
			// Already running - this is the expected case when fix is working
			d.logger.Printf("Refinery for %s already running, skipping spawn", rigName)
			return
		}
		d.logger.Printf("Error starting refinery for %s: %v", rigName, err)
		return
	}

	d.metrics.recordRestart(d.ctx, "refinery")
	telemetry.RecordDaemonRestart(d.ctx, "refinery-"+rigName)
	d.logger.Printf("Refinery session for %s started successfully", rigName)
}

// ensureMayorRunning ensures the Mayor is running.
// Uses mayor.Manager for consistent startup behavior (zombie detection, GUPP, etc.).
func (d *Daemon) ensureMayorRunning() {
	mgr := mayor.NewManager(d.config.TownRoot)

	if err := mgr.Start(""); err != nil {
		if err == mayor.ErrAlreadyRunning {
			// Mayor is running - nothing to do
			return
		}
		d.logger.Printf("Error starting Mayor: %v", err)
		return
	}

	d.logger.Println("Mayor started successfully")
}

// killDeaconSessions kills leftover deacon and boot tmux sessions.
// Called when the deacon patrol is disabled to prevent stale deacons from
// running their own patrol loops and spawning agents. (hq-2mstj)
func (d *Daemon) killDeaconSessions() {
	for _, name := range []string{session.DeaconSessionName(), session.BootSessionName()} {
		exists, _ := d.tmux.HasSession(name)
		if exists {
			d.logger.Printf("Killing leftover %s session (patrol disabled)", name)
			if err := d.tmux.KillSessionWithProcesses(name); err != nil {
				d.logger.Printf("Error killing %s session: %v", name, err)
			}
		}
	}
}

// killWitnessSessions kills leftover witness tmux sessions for all rigs.
// Called when the witness patrol is disabled. (hq-2mstj)
func (d *Daemon) killWitnessSessions() {
	for _, rigName := range d.getKnownRigs() {
		name := session.WitnessSessionName(session.PrefixFor(rigName))
		exists, _ := d.tmux.HasSession(name)
		if exists {
			d.logger.Printf("Killing leftover %s session (patrol disabled)", name)
			if err := d.tmux.KillSessionWithProcesses(name); err != nil {
				d.logger.Printf("Error killing %s session: %v", name, err)
			}
		}
	}
}

// killRefinerySessions kills leftover refinery tmux sessions for all rigs.
// Called when the refinery patrol is disabled. (hq-2mstj)
func (d *Daemon) killRefinerySessions() {
	for _, rigName := range d.getKnownRigs() {
		name := session.RefinerySessionName(session.PrefixFor(rigName))
		exists, _ := d.tmux.HasSession(name)
		if exists {
			d.logger.Printf("Killing leftover %s session (patrol disabled)", name)
			if err := d.tmux.KillSessionWithProcesses(name); err != nil {
				d.logger.Printf("Error killing %s session: %v", name, err)
			}
		}
	}
}

// openBeadsStores opens beads stores for the town (hq) and all known rigs.
// Returns a map keyed by "hq" for town-level and rig names for per-rig stores.
// Stores that fail to open are logged and skipped.
func (d *Daemon) openBeadsStores() map[string]beadsdk.Storage {
	stores := make(map[string]beadsdk.Storage)

	// Town-level store (hq)
	hqBeadsDir := filepath.Join(d.config.TownRoot, ".beads")
	if store, err := beadsdk.OpenFromConfig(d.ctx, hqBeadsDir); err == nil {
		stores["hq"] = store
	} else {
		d.logger.Printf("Convoy: hq beads store unavailable: %s", util.FirstLine(err.Error()))
	}

	// Per-rig stores
	for _, rigName := range d.getKnownRigs() {
		beadsDir := doltserver.FindRigBeadsDir(d.config.TownRoot, rigName)
		if beadsDir == "" {
			continue
		}
		store, err := beadsdk.OpenFromConfig(d.ctx, beadsDir)
		if err != nil {
			d.logger.Printf("Convoy: %s beads store unavailable: %s", rigName, util.FirstLine(err.Error()))
			continue
		}
		stores[rigName] = store
	}

	if len(stores) == 0 {
		d.logger.Printf("Convoy: no beads stores available, event polling disabled")
		return nil
	}

	names := make([]string, 0, len(stores))
	for name := range stores {
		names = append(names, name)
	}
	d.logger.Printf("Convoy: opened %d beads store(s): %v", len(stores), names)
	return stores
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

// getPatrolRigs returns the list of operational rigs for a patrol.
// If the patrol config specifies a rigs filter, only those rigs are returned.
// Otherwise, all known rigs are returned. In both cases, non-operational
// rigs (parked/docked) are filtered out at list-building time. (Fixes upstream #2082)
func (d *Daemon) getPatrolRigs(patrol string) []string {
	configRigs := GetPatrolRigs(d.patrolConfig, patrol)
	var candidates []string
	if len(configRigs) > 0 {
		candidates = configRigs
	} else {
		candidates = d.getKnownRigs()
	}

	// Filter out non-operational rigs early to avoid per-rig skip noise
	var operational []string
	for _, rigName := range candidates {
		if ok, reason := d.isRigOperational(rigName); ok {
			operational = append(operational, rigName)
		} else {
			d.logger.Printf("Excluding %s from %s patrol: %s", rigName, patrol, reason)
		}
	}
	return operational
}

// isRigOperational checks if a rig is in an operational state.
// Returns true if the rig can have agents auto-started.
// Returns false (with reason) if the rig is parked, docked, or has auto_restart blocked/disabled.
//
// TODO(#2120): This duplicates parked/docked checking logic from
// cmd.IsRigParkedOrDocked and cmd.hasRigBeadLabel. Consolidating into a
// shared package (e.g. internal/rig) would eliminate the third implementation
// and reduce drift risk. Not done here due to circular import constraints
// (daemon cannot import cmd).
func (d *Daemon) isRigOperational(rigName string) (bool, string) {
	cfg := wisp.NewConfig(d.config.TownRoot, rigName)

	// Warn if wisp config is missing - parked/docked state may have been lost
	if _, err := os.Stat(cfg.ConfigPath()); os.IsNotExist(err) {
		d.logger.Printf("Warning: no wisp config for %s - parked state may have been lost", rigName)
	}

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

	// Stop convoy manager (also closes beads stores)
	if d.convoyManager != nil {
		d.convoyManager.Stop()
		d.logger.Println("Convoy manager stopped")
	}
	d.beadsStores = nil

	// Stop KRC pruner
	if d.krcPruner != nil {
		d.krcPruner.Stop()
		d.logger.Println("KRC pruner stopped")
	}

	// Push Dolt remotes before stopping the server (if patrol is enabled)
	d.pushDoltRemotes()

	// Stop Dolt server if we're managing it
	if d.doltServer != nil && d.doltServer.IsEnabled() && !d.doltServer.IsExternal() {
		if err := d.doltServer.Stop(); err != nil {
			d.logger.Printf("Warning: failed to stop Dolt server: %v", err)
		} else {
			d.logger.Println("Dolt server stopped")
		}
	}

	// Flush and stop OTel providers (5s deadline to avoid blocking shutdown).
	if d.otelProvider != nil {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.otelProvider.Shutdown(shutCtx); err != nil {
			d.logger.Printf("Warning: telemetry shutdown: %v", err)
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
//
// Uses flock to check actual lock status rather than file existence, since
// the lock file persists after shutdown completes. The file is intentionally
// never removed: flock works on file descriptors, not paths, and removing
// the file while another process waits on the flock defeats mutual exclusion.
func (d *Daemon) isShutdownInProgress() bool {
	lockPath := filepath.Join(d.config.TownRoot, "daemon", "shutdown.lock")

	// If file doesn't exist, no shutdown in progress
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return false
	}

	// Try non-blocking lock acquisition to check if shutdown holds the lock
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		// Error acquiring lock - assume shutdown in progress to be safe
		return true
	}

	if locked {
		// We acquired the lock, so no shutdown is holding it
		// Release immediately; leave the file in place so all
		// concurrent callers flock the same inode.
		_ = lock.Unlock()
		return false
	}

	// Could not acquire lock - shutdown is in progress
	return true
}

// IsShutdownInProgress checks if a shutdown is currently in progress for the given town.
// This is the exported version of isShutdownInProgress for use by other packages
// (e.g., Boot triage) that need to avoid restarting sessions during shutdown.
func IsShutdownInProgress(townRoot string) bool {
	lockPath := filepath.Join(townRoot, "daemon", "shutdown.lock")

	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return false
	}

	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return true
	}

	if locked {
		_ = lock.Unlock()
		return false
	}

	return true
}

// IsRunning checks if a daemon is running for the given town.
// Uses the daemon.lock flock as the authoritative signal — if the lock is held,
// the daemon is running. Falls back to PID file for the process ID.
// This avoids fragile ps string matching for process identity (ZFC fix: gt-utuk).
func IsRunning(townRoot string) (bool, int, error) {
	// Primary check: is the daemon lock held?
	lockPath := filepath.Join(townRoot, "daemon", "daemon.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return false, 0, nil
	}

	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		// Can't check lock — fall back to PID file + signal check
		return isRunningFromPID(townRoot)
	}

	if locked {
		// We acquired the lock, so no daemon holds it
		_ = lock.Unlock()
		// Clean up stale PID file if present
		pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	// Lock is held — daemon is running. Read PID from file.
	// Use readPIDFile to handle the "PID\nNONCE" format introduced alongside
	// nonce-based ownership verification. A plain Atoi on the raw file content
	// fails when a nonce line is present, returning PID 0.
	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
	pid, _, err := readPIDFile(pidFile)
	if err != nil {
		// Lock held but no readable PID file — daemon running, PID unknown
		return true, 0, nil
	}

	return true, pid, nil
}

// isRunningFromPID is the fallback when flock check fails. Uses PID file + signal.
func isRunningFromPID(townRoot string) (bool, int, error) {
	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")

	pid, alive, err := verifyPIDOwnership(pidFile)
	if err != nil {
		return false, 0, fmt.Errorf("checking PID file: %w", err)
	}

	if pid == 0 {
		// No PID file
		return false, 0, nil
	}

	if !alive {
		// Process not running, clean up stale PID file.
		// This is a successful recovery, not an error — the caller can
		// proceed as if no daemon is running (fixes #2107).
		os.Remove(pidFile) // best-effort cleanup
		return false, 0, nil
	}

	return true, pid, nil
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

	if pid <= 0 {
		// Lock is held but PID is unknown (race: daemon starting, or stale lock).
		// Clean up the lock file so the next gt up can start fresh.
		lockPath := filepath.Join(townRoot, "daemon", "daemon.lock")
		_ = os.Remove(lockPath)
		pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
		_ = os.Remove(pidFile)
		return nil
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

// FindOrphanedDaemons detects daemon processes not tracked by the PID file.
// Uses flock on daemon.lock to detect running daemons without relying on
// pgrep or ps string matching (ZFC fix: gt-utuk).
//
// With flock-based daemon management, only one daemon can hold the lock.
// An "orphan" is detected when the lock is held but the PID file is stale
// (process dead) or missing. Returns the stale PID if available.
func FindOrphanedDaemons(townRoot string) ([]int, error) {
	lockPath := filepath.Join(townRoot, "daemon", "daemon.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return nil, nil // No lock file — no daemon has ever run
	}

	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return nil, nil // Can't check lock — assume no orphans
	}

	if locked {
		// We acquired the lock — no daemon holds it, no orphans possible
		_ = lock.Unlock()
		return nil, nil
	}

	// Lock is held — a daemon is running. Check if it's tracked.
	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
	trackedPID, _, err := readPIDFile(pidFile)
	if err != nil {
		// Lock held but no/invalid PID file — daemon is running but untracked.
		// We can't determine its PID without ps/pgrep, so return empty.
		// The caller (start.go) should use IsRunning() which handles this case.
		return nil, nil
	}

	// Check if the tracked PID is actually alive
	process, findErr := os.FindProcess(trackedPID)
	if findErr != nil {
		return nil, nil
	}
	if process.Signal(syscall.Signal(0)) != nil {
		// PID file exists but process is dead — stale PID file with held lock.
		// This shouldn't happen (lock should release on process death), but
		// report the stale PID for cleanup.
		return []int{trackedPID}, nil
	}

	// Lock held, PID alive, PID tracked — daemon is properly running, not orphaned.
	return nil, nil
}

// KillOrphanedDaemons finds and kills any orphaned gt daemon processes.
// Returns number of processes killed.
func KillOrphanedDaemons(townRoot string) (int, error) {
	pids, err := FindOrphanedDaemons(townRoot)
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
	sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)

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
	prefix := beads.GetPrefixForRig(d.config.TownRoot, rigName)
	agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, rigName, polecatName)
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

	// Spawning guard: skip polecats being actively started by gt sling.
	// agent_state='spawning' means the polecat bead was created (with hook_bead
	// set atomically) but the tmux session hasn't been launched yet. Restarting
	// here would create a second Claude process alongside the one gt sling is
	// about to start, causing the double-spawn bug (issue #1752).
	//
	// Time-bound: only skip if the bead was updated recently (within 5 minutes).
	// If gt sling crashed during spawn, the polecat would be stuck in 'spawning'
	// indefinitely. The Witness patrol also catches spawning-as-zombie, but a
	// time-bound here makes the daemon self-sufficient for this edge case.
	if beads.AgentState(info.State) == beads.AgentStateSpawning {
		if updatedAt, err := time.Parse(time.RFC3339, info.LastUpdate); err == nil {
			if time.Since(updatedAt) < 5*time.Minute {
				d.logger.Printf("Skipping restart for %s/%s: agent_state=spawning (gt sling in progress, updated %s ago)",
					rigName, polecatName, time.Since(updatedAt).Round(time.Second))
				return
			}
			d.logger.Printf("Spawning guard expired for %s/%s: agent_state=spawning but last updated %s ago (>5m), proceeding with crash detection",
				rigName, polecatName, time.Since(updatedAt).Round(time.Second))
		} else {
			// Can't parse timestamp — be safe, skip restart during spawning
			d.logger.Printf("Skipping restart for %s/%s: agent_state=spawning (gt sling in progress, unparseable updated_at)",
				rigName, polecatName)
			return
		}
	}

	// TOCTOU guard: re-verify session is still dead before restarting.
	// Between the initial check and now, the session may have been restarted
	// by another heartbeat cycle, witness, or the polecat itself.
	sessionRevived, err := d.tmux.HasSession(sessionName)
	if err == nil && sessionRevived {
		return // Session came back - no restart needed
	}

	// Polecat has work but session is dead - this is a crash!
	d.logger.Printf("CRASH DETECTED: polecat %s/%s has hook_bead=%s but session %s is dead",
		rigName, polecatName, info.HookBead, sessionName)

	// Track this death for mass death detection
	d.recordSessionDeath(sessionName)

	// Emit session_death event for audit trail / feed visibility
	_ = events.LogFeed(events.TypeSessionDeath, sessionName,
		events.SessionDeathPayload(sessionName, rigName+"/polecats/"+polecatName, "crash detected by daemon health check", "daemon"))

	// Auto-restart the polecat
	restartErr := d.restartPolecatSession(rigName, polecatName, sessionName)
	if restartErr != nil {
		d.logger.Printf("Error restarting polecat %s/%s: %v", rigName, polecatName, restartErr)
	} else {
		d.logger.Printf("Successfully restarted crashed polecat %s/%s", rigName, polecatName)
	}

	// Always notify witness of crash (with restart outcome)
	d.notifyWitnessOfCrashedPolecat(rigName, polecatName, info.HookBead, restartErr)
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

	// Generate a fresh run ID for this restart so all telemetry from the
	// crash-restarted session is correlated under a new GASTA root span.
	runID := uuid.New().String()

	// Build startup command BEFORE creating the session so we can use
	// NewSessionWithCommand (command as initial pane process). This eliminates
	// the race condition in the old EnsureSessionFresh + SendKeys pattern where
	// the shell might not be ready to receive keystrokes, producing empty windows.
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:        "polecat",
		Rig:         rigName,
		AgentName:   polecatName,
		TownRoot:    d.config.TownRoot,
		SessionName: sessionName,
	})
	// Inject GT_RUN into envVars before BuildStartupCommand so the startup
	// command includes it (exec env …) and the loop below propagates it to
	// the tmux session environment.
	envVars["GT_RUN"] = runID
	rc := config.ResolveRoleAgentConfig("polecat", d.config.TownRoot, rigPath)
	startCmd := config.BuildStartupCommand(envVars, rigPath, "")

	// Create session with command as initial process (replaces EnsureSessionFresh + SendKeys).
	// EnsureSessionFreshWithCommand kills zombie sessions and creates a new one atomically.
	if err := d.tmux.EnsureSessionFreshWithCommand(sessionName, workDir, startCmd); err != nil {
		if errors.Is(err, tmux.ErrSessionRunning) {
			d.logger.Printf("Session %s already running with healthy agent, skipping restart", sessionName)
			return nil
		}
		return fmt.Errorf("creating session: %w", err)
	}

	// Record polecat spawn metric.
	d.metrics.recordPolecatSpawn(d.ctx, rigName)

	// Set environment variables in tmux session table (for debugging/monitoring tools).
	// The process itself gets env vars via 'exec env ...' in the startup command.
	for k, v := range envVars {
		_ = d.tmux.SetEnvironment(sessionName, k, v)
	}

	// Set GT_AGENT in tmux session env so tools querying tmux environment
	// (e.g., witness patrol) can detect non-Claude agents.
	// BuildStartupCommand sets GT_AGENT in process env via exec env, but that
	// isn't visible to tmux show-environment.
	if rc.ResolvedAgent != "" {
		_ = d.tmux.SetEnvironment(sessionName, "GT_AGENT", rc.ResolvedAgent)
	}

	// Set GT_PROCESS_NAMES for accurate liveness detection of custom agents.
	processNames := config.ResolveProcessNames(rc.ResolvedAgent, rc.Command)
	_ = d.tmux.SetEnvironment(sessionName, "GT_PROCESS_NAMES", strings.Join(processNames, ","))

	// Apply theme
	theme := tmux.AssignTheme(rigName)
	_ = d.tmux.ConfigureGasTownSession(sessionName, theme, rigName, polecatName, "polecat")

	// Set pane-died hook for future crash detection
	agentID := fmt.Sprintf("%s/%s", rigName, polecatName)
	_ = d.tmux.SetPaneDiedHook(sessionName, agentID)

	// Wait for Claude to start, then accept startup dialogs if they appear.
	if err := d.tmux.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal - Claude might still start
	}
	_ = d.tmux.AcceptStartupDialogs(sessionName)

	return nil
}

// notifyWitnessOfCrashedPolecat notifies the witness when a polecat crash is detected.
// If restartErr is nil, the notification is informational (restart succeeded).
// If restartErr is non-nil, the notification indicates manual intervention may be needed.
func (d *Daemon) notifyWitnessOfCrashedPolecat(rigName, polecatName, hookBead string, restartErr error) {
	witnessAddr := rigName + "/witness"
	var subject, body string
	if restartErr == nil {
		subject = fmt.Sprintf("CRASHED_POLECAT: %s/%s restarted", rigName, polecatName)
		body = fmt.Sprintf(`Polecat %s crashed and was automatically restarted.

hook_bead: %s

No action required.`,
			polecatName, hookBead)
	} else {
		subject = fmt.Sprintf("CRASHED_POLECAT: %s/%s restart failed", rigName, polecatName)
		body = fmt.Sprintf(`Polecat %s crashed and automatic restart failed.

hook_bead: %s
restart_error: %v

Manual intervention may be required.`,
			polecatName, hookBead, restartErr)
	}

	cmd := exec.Command(d.gtPath, "mail", "send", witnessAddr, "-s", subject, "-m", body) //nolint:gosec // G204: args are constructed internally
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

// pruneStaleBranches removes stale local polecat tracking branches from all rig clones.
// This runs in every heartbeat but is very fast when there are no stale branches.
func (d *Daemon) pruneStaleBranches() {
	// pruneInDir prunes stale polecat branches in a single git directory.
	pruneInDir := func(dir, label string) {
		g := gitpkg.NewGit(dir)
		if !g.IsRepo() {
			return
		}

		// Fetch --prune first to clean up stale remote tracking refs
		_ = g.FetchPrune("origin")

		pruned, err := g.PruneStaleBranches("polecat/*", false)
		if err != nil {
			d.logger.Printf("Warning: branch prune failed for %s: %v", label, err)
			return
		}

		if len(pruned) > 0 {
			d.logger.Printf("Branch prune: removed %d stale polecat branch(es) in %s", len(pruned), label)
			for _, b := range pruned {
				d.logger.Printf("  %s (%s)", b.Name, b.Reason)
			}
		}
	}

	// Prune in each rig's git directory
	for _, rigName := range d.getKnownRigs() {
		rigPath := filepath.Join(d.config.TownRoot, rigName)
		pruneInDir(rigPath, rigName)
	}

	// Also prune in the town root itself (mayor clone)
	pruneInDir(d.config.TownRoot, "town-root")
}

// dispatchQueuedWork shells out to `gt scheduler run` to dispatch scheduled beads.
// This avoids circular import between the daemon and cmd packages.
// Uses a 5m timeout to allow multi-bead dispatch with formula cooking and hook retries.
//
// Timeout safety: if the timeout fires mid-dispatch, a bead may be left with
// metadata written but label not yet swapped (or vice versa). The dispatch flock
// is released on process death, and dispatchSingleBead's label swap retry logic
// prevents double-dispatch on the next cycle. The batch_size config (default: 1)
// limits how many beads are in-flight per heartbeat, reducing the timeout window.
func (d *Daemon) dispatchQueuedWork() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gt", "scheduler", "run")
	cmd.Dir = d.config.TownRoot
	cmd.Env = append(os.Environ(), "GT_DAEMON=1", "BD_DOLT_AUTO_COMMIT=off")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		d.logger.Printf("Scheduler dispatch timed out after 5m")
	} else if err != nil {
		d.logger.Printf("Scheduler dispatch failed: %v (output: %s)", err, string(out))
	} else if len(out) > 0 {
		d.logger.Printf("Scheduler dispatch: %s", string(out))
	}
}
