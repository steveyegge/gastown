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
	"syscall"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/boot"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/feed"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// Daemon is the town-level background service.
// It ensures patrol agents (Deacon, Witnesses) are running and detects failures.
// This is recovery-focused: normal wake is handled by feed subscription (bd activity --follow).
// The daemon is the safety net for dead sessions, GUPP violations, and orphaned work.
type Daemon struct {
	config  *Config
	tmux    *tmux.Tmux
	logger  *log.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	curator *feed.Curator
}

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

	return &Daemon{
		config: config,
		tmux:   tmux.NewTmux(),
		logger: logger,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// Run starts the daemon main loop.
func (d *Daemon) Run() error {
	d.logger.Printf("Daemon starting (PID %d)", os.Getpid())

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
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

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

	// Initial heartbeat
	d.heartbeat(state)

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Println("Daemon context canceled, shutting down")
			return d.shutdown(state)

		case sig := <-sigChan:
			if sig == syscall.SIGUSR1 {
				// SIGUSR1: immediate lifecycle processing (from gt handoff)
				d.logger.Println("Received SIGUSR1, processing lifecycle requests immediately")
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
	d.logger.Println("Heartbeat starting (recovery-focused)")

	// 1. Poke Boot (the Deacon's watchdog) instead of Deacon directly
	// Boot handles the "when to wake Deacon" decision via triage logic
	d.ensureBootRunning()

	// 1b. Direct Deacon heartbeat check (belt-and-suspenders)
	// Boot may not detect all stuck states; this provides a fallback
	d.checkDeaconHeartbeat()

	// 2. Ensure Witnesses are running for all rigs (restart if dead)
	d.ensureWitnessesRunning()

	// 2b. Ensure Refineries are running for all rigs (restart if dead)
	d.ensureRefineriesRunning()

	// 3. Trigger pending polecat spawns (bootstrap mode - ZFC violation acceptable)
	// This ensures polecats get nudged even when Deacon isn't in a patrol cycle.
	// Uses regex-based WaitForClaudeReady, which is acceptable for daemon bootstrap.
	d.triggerPendingSpawns()

	// 4. Process lifecycle requests
	d.processLifecycleRequests()

	// 5. Check for stale agents (timeout fallback)
	// Agents that report "running" but haven't updated in too long are marked dead
	d.checkStaleAgents()

	// 6. Check for GUPP violations (agents with work-on-hook not progressing)
	d.checkGUPPViolations()

	// 7. Check for orphaned work (assigned to dead agents)
	d.checkOrphanedWork()

	// 8. Check polecat session health (proactive crash detection)
	// This validates tmux sessions are still alive for polecats with work-on-hook
	d.checkPolecatSessionHealth()

	// Update state
	state.LastHeartbeat = time.Now()
	state.HeartbeatCount++
	if err := SaveState(d.config.TownRoot, state); err != nil {
		d.logger.Printf("Warning: failed to save state: %v", err)
	}

	d.logger.Printf("Heartbeat complete (#%d)", state.HeartbeatCount)
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
func (d *Daemon) ensureBootRunning() {
	b := boot.New(d.config.TownRoot)

	// Check if Boot is already running (recent marker)
	if b.IsRunning() {
		d.logger.Println("Boot already running, skipping spawn")
		return
	}

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
	if err := b.Spawn(); err != nil {
		d.logger.Printf("Error spawning Boot: %v, falling back to direct Deacon check", err)
		// Fallback: ensure Deacon is running directly
		d.ensureDeaconRunning()
		return
	}

	d.logger.Println("Boot spawned successfully")
}

// runDegradedBootTriage performs mechanical Boot logic without AI reasoning.
// This is for degraded mode when tmux is unavailable.
func (d *Daemon) runDegradedBootTriage(b *boot.Boot) {
	startTime := time.Now()
	status := &boot.Status{
		Running:   true,
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

	status.Running = false
	status.CompletedAt = time.Now()

	if err := b.SaveStatus(status); err != nil {
		d.logger.Printf("Warning: failed to save Boot status: %v", err)
	}
}

// ensureDeaconRunning ensures the Deacon is running.
// ZFC-compliant: trusts agent bead state, with tmux health check fallback.
// The Deacon is the system's heartbeat - it must always be running.
func (d *Daemon) ensureDeaconRunning() {
	// Check agent bead state (ZFC: trust what agent reports)
	beadState, beadErr := d.getAgentBeadState(d.getDeaconSessionName())
	if beadErr == nil {
		if beadState == "running" || beadState == "working" {
			// Agent reports it's running - trust it
			// Timeout fallback for stale state is in lifecycle.go
			return
		}

		// CIRCUIT BREAKER: If agent is marked "dead" by checkStaleAgents(),
		// force-kill the session and restart. This handles stuck agents that
		// are still alive (zombie Claude sessions that haven't updated their bead).
		if beadState == "dead" {
			d.logger.Println("Deacon is marked dead (circuit breaker triggered), forcing restart...")
			deaconSession := d.getDeaconSessionName()
			hasSession, _ := d.tmux.HasSession(deaconSession)
			if hasSession {
				if err := d.tmux.KillSession(deaconSession); err != nil {
					d.logger.Printf("Warning: failed to kill dead Deacon session: %v", err)
				}
			}
			// Fall through to restart
		}
	}

	// Agent bead check failed or state is not running/working.
	// FALLBACK: Check if tmux session is actually healthy before attempting restart.
	// This prevents killing healthy sessions when bead state is stale or unreadable.
	// Skip this check if agent was marked dead (we already handled that above).
	if beadState != "dead" {
		deaconSession := d.getDeaconSessionName()
		hasSession, sessionErr := d.tmux.HasSession(deaconSession)
		if sessionErr == nil && hasSession {
			if d.tmux.IsClaudeRunning(deaconSession) {
				d.logger.Println("Deacon session healthy (Claude running), skipping restart despite stale bead")
				return
			}
		}
	}

	// Agent not running (or bead not found) AND session is not healthy - start it
	d.logger.Println("Deacon not running per agent bead, starting...")

	// Create session in deacon directory (ensures correct CLAUDE.md is loaded)
	// Use EnsureSessionFresh to handle zombie sessions that exist but have dead Claude
	deaconDir := filepath.Join(d.config.TownRoot, "deacon")
	sessionName := d.getDeaconSessionName()
	if err := d.tmux.EnsureSessionFresh(sessionName, deaconDir); err != nil {
		d.logger.Printf("Error creating Deacon session: %v", err)
		return
	}

	// Set environment (non-fatal: session works without these)
	_ = d.tmux.SetEnvironment(sessionName, "GT_ROLE", "deacon")
	_ = d.tmux.SetEnvironment(sessionName, "BD_ACTOR", "deacon")

	// Launch Claude directly (no shell respawn loop)
	// The daemon will detect if Claude exits and restart it on next heartbeat
	// Export GT_ROLE and BD_ACTOR so Claude inherits them (tmux SetEnvironment doesn't export to processes)
	if err := d.tmux.SendKeys(sessionName, config.BuildAgentStartupCommand("deacon", "deacon", "", "")); err != nil {
		d.logger.Printf("Error launching Claude in Deacon session: %v", err)
		return
	}

	d.logger.Println("Deacon session started successfully")
}

// checkDeaconHeartbeat checks if the Deacon is making progress.
// This is a belt-and-suspenders fallback in case Boot doesn't detect stuck states.
// Uses the heartbeat file that the Deacon updates on each patrol cycle.
func (d *Daemon) checkDeaconHeartbeat() {
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
		// Session doesn't exist - ensureBootRunning will handle restart
		return
	}

	// Session exists but heartbeat is stale - Deacon is stuck
	if age > 30*time.Minute {
		// Very stuck - restart the session
		d.logger.Printf("Deacon stuck for %s - restarting session", age.Round(time.Minute))
		if err := d.tmux.KillSession(sessionName); err != nil {
			d.logger.Printf("Error killing stuck Deacon: %v", err)
		}
		// ensureDeaconRunning will be called next heartbeat to restart
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
func (d *Daemon) ensureWitnessRunning(rigName string) {
	agentID := beads.WitnessBeadID(rigName)
	sessionName := "gt-" + rigName + "-witness"

	// Check agent bead state (ZFC: trust what agent reports)
	beadState, beadErr := d.getAgentBeadState(agentID)
	if beadErr == nil {
		if beadState == "running" || beadState == "working" {
			// Agent reports it's running - trust it
			return
		}

		// CIRCUIT BREAKER: If agent is marked "dead" by checkStaleAgents(),
		// force-kill the session and restart. This handles stuck agents that
		// are still alive (zombie Claude sessions that haven't updated their bead).
		if beadState == "dead" {
			d.logger.Printf("Witness for %s is marked dead (circuit breaker triggered), forcing restart...", rigName)
			hasSession, _ := d.tmux.HasSession(sessionName)
			if hasSession {
				if err := d.tmux.KillSession(sessionName); err != nil {
					d.logger.Printf("Warning: failed to kill dead witness session for %s: %v", rigName, err)
				}
			}
			// Fall through to restart
		}
	}

	// Agent bead check failed or state is not running/working.
	// FALLBACK: Check if tmux session is actually healthy before attempting restart.
	// This prevents killing healthy sessions when bead state is stale or unreadable.
	// Skip this check if agent was marked dead (we already handled that above).
	if beadState != "dead" {
		hasSession, sessionErr := d.tmux.HasSession(sessionName)
		if sessionErr == nil && hasSession {
			// Session exists - check if Claude is actually running in it
			if d.tmux.IsClaudeRunning(sessionName) {
				// Session is healthy - don't restart it
				// The bead state may be stale; agent will update it on next activity
				d.logger.Printf("Witness for %s session healthy (Claude running), skipping restart despite stale bead", rigName)
				return
			}
		}
	}

	// Agent not running (or bead not found) AND session is not healthy - start it
	d.logger.Printf("Witness for %s not running per agent bead, starting...", rigName)

	// Create session in witness directory
	// Use EnsureSessionFresh to handle zombie sessions that exist but have dead Claude
	witnessDir := filepath.Join(d.config.TownRoot, rigName, "witness")
	if err := d.tmux.EnsureSessionFresh(sessionName, witnessDir); err != nil {
		d.logger.Printf("Error creating witness session for %s: %v", rigName, err)
		return
	}

	// Set environment
	_ = d.tmux.SetEnvironment(sessionName, "GT_ROLE", "witness")
	_ = d.tmux.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = d.tmux.SetEnvironment(sessionName, "BD_ACTOR", rigName+"-witness")

	// Launch Claude
	bdActor := fmt.Sprintf("%s/witness", rigName)
	envVars := map[string]string{
		"GT_ROLE":         "witness",
		"GT_RIG":          rigName,
		"BD_ACTOR":        bdActor,
		"GIT_AUTHOR_NAME": bdActor,
	}
	if err := d.tmux.SendKeys(sessionName, config.BuildStartupCommand(envVars, "", "")); err != nil {
		d.logger.Printf("Error launching Claude in witness session for %s: %v", rigName, err)
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
func (d *Daemon) ensureRefineryRunning(rigName string) {
	agentID := beads.RefineryBeadID(rigName)
	sessionName := "gt-" + rigName + "-refinery"

	// Check agent bead state (ZFC: trust what agent reports)
	beadState, beadErr := d.getAgentBeadState(agentID)
	if beadErr == nil {
		if beadState == "running" || beadState == "working" {
			// Agent reports it's running - trust it
			return
		}

		// CIRCUIT BREAKER: If agent is marked "dead" by checkStaleAgents(),
		// force-kill the session and restart. This handles stuck agents that
		// are still alive (zombie Claude sessions that haven't updated their bead).
		if beadState == "dead" {
			d.logger.Printf("Refinery for %s is marked dead (circuit breaker triggered), forcing restart...", rigName)
			hasSession, _ := d.tmux.HasSession(sessionName)
			if hasSession {
				if err := d.tmux.KillSession(sessionName); err != nil {
					d.logger.Printf("Warning: failed to kill dead refinery session for %s: %v", rigName, err)
				}
			}
			// Fall through to restart
		}
	}

	// Agent bead check failed or state is not running/working.
	// FALLBACK: Check if tmux session is actually healthy before attempting restart.
	// This prevents killing healthy sessions when bead state is stale or unreadable.
	// Skip this check if agent was marked dead (we already handled that above).
	if beadState != "dead" {
		hasSession, sessionErr := d.tmux.HasSession(sessionName)
		if sessionErr == nil && hasSession {
			// Session exists - check if Claude is actually running in it
			if d.tmux.IsClaudeRunning(sessionName) {
				// Session is healthy - don't restart it
				// The bead state may be stale; agent will update it on next activity
				d.logger.Printf("Refinery for %s session healthy (Claude running), skipping restart despite stale bead", rigName)
				return
			}
		}
	}

	// Agent not running (or bead not found) AND session is not healthy - start it
	d.logger.Printf("Refinery for %s not running per agent bead, starting...", rigName)

	// Determine working directory
	rigPath := filepath.Join(d.config.TownRoot, rigName)
	refineryDir := filepath.Join(rigPath, "refinery", "rig")
	if _, err := os.Stat(refineryDir); os.IsNotExist(err) {
		// Fall back to rig path if refinery/rig doesn't exist
		refineryDir = rigPath
	}

	// Create session in refinery directory
	// Use EnsureSessionFresh to handle zombie sessions that exist but have dead Claude
	if err := d.tmux.EnsureSessionFresh(sessionName, refineryDir); err != nil {
		d.logger.Printf("Error creating refinery session for %s: %v", rigName, err)
		return
	}

	// Set environment
	bdActor := fmt.Sprintf("%s/refinery", rigName)
	_ = d.tmux.SetEnvironment(sessionName, "GT_ROLE", "refinery")
	_ = d.tmux.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = d.tmux.SetEnvironment(sessionName, "BD_ACTOR", bdActor)

	// Set beads environment
	beadsDir := filepath.Join(rigPath, "mayor", "rig", ".beads")
	_ = d.tmux.SetEnvironment(sessionName, "BEADS_DIR", beadsDir)
	_ = d.tmux.SetEnvironment(sessionName, "BEADS_NO_DAEMON", "1")
	_ = d.tmux.SetEnvironment(sessionName, "BEADS_AGENT_NAME", bdActor)

	// Apply theming (non-fatal)
	theme := tmux.AssignTheme(rigName)
	_ = d.tmux.ConfigureGasTownSession(sessionName, theme, rigName, "refinery", "refinery")

	// Launch Claude with environment exported inline
	envVars := map[string]string{
		"GT_ROLE":         "refinery",
		"GT_RIG":          rigName,
		"BD_ACTOR":        bdActor,
		"GIT_AUTHOR_NAME": bdActor,
	}
	if err := d.tmux.SendKeys(sessionName, config.BuildStartupCommand(envVars, "", "")); err != nil {
		d.logger.Printf("Error launching Claude in refinery session for %s: %v", rigName, err)
		return
	}

	// Wait for Claude to start, then accept bypass permissions warning if it appears.
	if err := d.tmux.WaitForCommand(sessionName, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Non-fatal - Claude might still start
	}
	_ = d.tmux.AcceptBypassPermissionsWarning(sessionName)

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

// triggerPendingSpawns polls pending polecat spawns and triggers those that are ready.
// This is bootstrap mode - uses regex-based WaitForClaudeReady which is acceptable
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

	// Trigger pending spawns (uses WaitForClaudeReady with short timeout)
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

// IsRunning checks if a daemon is running for the given town.
func IsRunning(townRoot string) (bool, int, error) {
	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return false, 0, nil
	}

	// Check if process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0, nil
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process not running, clean up stale PID file (best-effort cleanup)
		_ = os.Remove(pidFile)
		return false, 0, nil
	}

	return true, pid, nil
}

// StopDaemon stops the running daemon for the given town.
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
		// Still running, force kill (best-effort)
		_ = process.Signal(syscall.SIGKILL)
	}

	// Clean up PID file (best-effort cleanup)
	pidFile := filepath.Join(townRoot, "daemon", "daemon.pid")
	_ = os.Remove(pidFile)

	return nil
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
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		return // No polecats directory - rig might not have polecats
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		polecatName := entry.Name()
		d.checkPolecatHealth(rigName, polecatName)
	}
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
	agentBeadID := beads.PolecatBeadID(rigName, polecatName)
	info, err := d.getAgentBeadInfo(agentBeadID)
	if err != nil {
		// Agent bead doesn't exist or error - polecat might not be registered
		return
	}

	// Check if polecat has hooked work
	if info.HookBead == "" {
		// No hooked work - no need to restart (polecat was idle)
		return
	}

	// Polecat has work but session is dead - this is a crash!
	d.logger.Printf("CRASH DETECTED: polecat %s/%s has hook_bead=%s but session %s is dead",
		rigName, polecatName, info.HookBead, sessionName)

	// Auto-restart the polecat
	if err := d.restartPolecatSession(rigName, polecatName, sessionName); err != nil {
		d.logger.Printf("Error restarting polecat %s/%s: %v", rigName, polecatName, err)
		// Notify witness as fallback
		d.notifyWitnessOfCrashedPolecat(rigName, polecatName, info.HookBead, err)
	} else {
		d.logger.Printf("Successfully restarted crashed polecat %s/%s", rigName, polecatName)
	}
}

// restartPolecatSession restarts a crashed polecat session.
func (d *Daemon) restartPolecatSession(rigName, polecatName, sessionName string) error {
	// Determine working directory
	workDir := filepath.Join(d.config.TownRoot, rigName, "polecats", polecatName)

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

	// Set environment variables
	_ = d.tmux.SetEnvironment(sessionName, "GT_ROLE", "polecat")
	_ = d.tmux.SetEnvironment(sessionName, "GT_RIG", rigName)
	_ = d.tmux.SetEnvironment(sessionName, "GT_POLECAT", polecatName)

	bdActor := fmt.Sprintf("%s/polecats/%s", rigName, polecatName)
	_ = d.tmux.SetEnvironment(sessionName, "BD_ACTOR", bdActor)

	beadsDir := filepath.Join(d.config.TownRoot, rigName, ".beads")
	_ = d.tmux.SetEnvironment(sessionName, "BEADS_DIR", beadsDir)
	_ = d.tmux.SetEnvironment(sessionName, "BEADS_NO_DAEMON", "1")
	_ = d.tmux.SetEnvironment(sessionName, "BEADS_AGENT_NAME", fmt.Sprintf("%s/%s", rigName, polecatName))

	// Apply theme
	theme := tmux.AssignTheme(rigName)
	_ = d.tmux.ConfigureGasTownSession(sessionName, theme, rigName, polecatName, "polecat")

	// Set pane-died hook for future crash detection
	agentID := fmt.Sprintf("%s/%s", rigName, polecatName)
	_ = d.tmux.SetPaneDiedHook(sessionName, agentID)

	// Launch Claude with environment exported inline
	startCmd := config.BuildPolecatStartupCommand(rigName, polecatName, "", "")
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
	if err := cmd.Run(); err != nil {
		d.logger.Printf("Warning: failed to notify witness of crashed polecat: %v", err)
	}
}
