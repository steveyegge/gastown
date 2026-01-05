# Design: Gastown Agent Initialization

**Bead**: ga-bll
**Author**: gastown/polecats/ace
**Date**: 2026-01-05
**Status**: DRAFT

## Problem Statement

Currently, Gas Town agents require manual startup coordination:
1. `gt daemon start` must be run separately
2. Mayor session starts, but daemon may not be running
3. Rig agents (Witnesses, Refineries) start lazily or require `--all` flag
4. No automatic health verification before proceeding

Goal: When Mayor starts, all required agents should auto-initialize with proper dependency ordering and health checks.

## Current Architecture Analysis

### Startup Entry Points

| Command | What it does |
|---------|--------------|
| `gt start` | Starts Mayor, Deacon, optionally rig agents (`--all`) |
| `gt daemon start` | Starts the Go daemon (separate process) |
| `gt witness start <rig>` | Starts Witness for one rig |
| `gt refinery start <rig>` | Starts Refinery for one rig |

### Current Startup Sequence (gt start)

```
1. startCoreAgents()
   ├─ Start Mayor session (if not running)
   └─ Start Deacon session (if not running)

2. if --all:
   └─ startRigAgents()
       └─ For each rig:
           ├─ ensureWitnessSession()
           └─ ensureRefinerySession()

3. startConfiguredCrew()
   └─ Start crew members from rig config
```

### Daemon Responsibilities

The daemon runs independently with a 3-minute heartbeat:
- `ensureBootRunning()` - Deacon watchdog
- `checkDeaconHeartbeat()` - Belt-and-suspenders Deacon check
- `ensureWitnessesRunning()` - All rigs
- `ensureRefineriesRunning()` - All rigs
- `triggerPendingSpawns()` - Nudge spawned polecats
- `processLifecycleRequests()` - Handle restart/shutdown mail
- `checkStaleAgents()` - Timeout fallback
- `checkGUPPViolations()` - Work-on-hook not progressing
- `checkOrphanedWork()` - Work on dead agents
- `checkPolecatSessionHealth()` - Proactive crash detection

### Current Gaps

1. **Daemon not auto-started**: Mayor assumes daemon exists
2. **No health checks between steps**: Start commands don't verify success
3. **No retry logic**: If an agent fails to start, it's not retried immediately
4. **No centralized "ensure everything running"**: Daemon does recovery, but no single bootstrap

## Proposed Design

### Initialization Dependency Order

```
                    ┌─────────────┐
                    │   Daemon    │ ◄── Must start first (Go process)
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Mayor     │ ◄── Town-level coordinator
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Deacon    │ ◄── Health monitor
                    └──────┬──────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
    ┌────▼────┐       ┌────▼────┐       ┌────▼────┐
    │ Witness │       │ Witness │       │ Witness │  ◄── Per-rig
    │ (rig 1) │       │ (rig 2) │       │ (rig N) │
    └────┬────┘       └────┬────┘       └────┬────┘
         │                 │                 │
    ┌────▼────┐       ┌────▼────┐       ┌────▼────┐
    │Refinery │       │Refinery │       │Refinery │  ◄── Per-rig
    │ (rig 1) │       │ (rig 2) │       │ (rig N) │
    └─────────┘       └─────────┘       └─────────┘
```

### Health Check Strategy

Each agent must pass a health check before proceeding:

| Agent | Health Check | Timeout |
|-------|--------------|---------|
| Daemon | PID file exists + process alive | 5s |
| Mayor | tmux session exists + Claude running | 10s |
| Deacon | tmux session exists + Claude running | 10s |
| Witness | tmux session exists + Claude running | 10s |
| Refinery | tmux session exists + Claude running | 10s |

"Claude running" check: `tmux.IsClaudeRunning(session)` - verifies the shell isn't at a prompt.

### Proposed Implementation

#### Option A: gt start Auto-Daemon (Recommended)

Modify `gt start` to automatically start the daemon if not running:

```go
// In runStart()
func runStart(cmd *cobra.Command, args []string) error {
    // 1. Ensure daemon is running first
    if err := ensureDaemonRunning(); err != nil {
        return fmt.Errorf("starting daemon: %w", err)
    }

    // 2. Start core agents (Mayor, Deacon)
    if err := startCoreAgents(t); err != nil {
        return err
    }

    // 3. Verify health before continuing
    if err := waitForCoreAgentsHealthy(t); err != nil {
        return fmt.Errorf("core agents unhealthy: %w", err)
    }

    // 4. Start rig agents (always, not just --all)
    // The daemon will keep them running anyway
    startRigAgents(t, townRoot)

    // 5. Start configured crew
    startConfiguredCrew(t, townRoot)

    return nil
}
```

#### Option B: Mayor Auto-Bootstrap

Mayor's first heartbeat could trigger full initialization:

```go
// In daemon heartbeat
func (d *Daemon) firstBootstrap() {
    if d.bootstrapDone {
        return
    }

    // Ensure Mayor is running
    d.ensureMayorRunning()

    // Ensure Deacon is running
    d.ensureDeaconRunning()

    // Ensure all rig agents
    d.ensureWitnessesRunning()
    d.ensureRefineriesRunning()

    d.bootstrapDone = true
}
```

#### Option C: Separate Bootstrap Command

Add `gt boot` as a one-shot bootstrap that ensures everything is running:

```bash
gt boot  # Ensures: daemon + mayor + deacon + witnesses + refineries
```

### Recommendation: Option A

**Rationale**:
- Users already run `gt start` - adding daemon auto-start is invisible
- No new commands to learn
- Idempotent: running `gt start` multiple times is safe
- Daemon continues doing periodic recovery after initial start

### Failure Handling

```
Start Attempt
     │
     ▼
 ┌───────────────┐
 │ Create Session│
 └───────┬───────┘
         │
         ▼
 ┌───────────────┐     ┌──────────────┐
 │  Wait 10s for │────►│ Health Check │
 │  Readiness    │     │   Passed?    │
 └───────────────┘     └──────┬───────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
         ┌────────┐                    ┌────────────┐
         │  Yes   │                    │    No      │
         └────────┘                    └─────┬──────┘
              │                              │
              ▼                              ▼
         Continue                    ┌────────────┐
                                     │ Retry ≤ 3  │
                                     │   times    │
                                     └─────┬──────┘
                                           │
                         ┌─────────────────┴─────────────────┐
                         ▼                                   ▼
                   ┌──────────┐                        ┌───────────┐
                   │ Success  │                        │ Log Error │
                   └──────────┘                        │ Continue  │
                                                       └───────────┘
```

**Key principle**: Don't block town startup on a single rig failure. Log the error and continue. The daemon's periodic recovery will retry.

### Scenarios

#### 1. Fresh Town Startup

```
User runs: gt start

1. gt start checks daemon PID file
2. Daemon not running → starts daemon
3. Wait for daemon health (PID file, signal 0)
4. Start Mayor session
5. Wait for Mayor healthy (IsClaudeRunning)
6. Start Deacon session
7. Wait for Deacon healthy
8. For each rig:
   a. Start Witness → verify healthy
   b. Start Refinery → verify healthy
9. Start configured crew
10. Print success
```

#### 2. Mayor Restart (Other Agents Running)

```
User runs: gt start (Mayor was killed)

1. Check daemon → running (skip)
2. Check Mayor → not running → start
3. Wait for Mayor healthy
4. Check Deacon → running (skip)
5. For each rig:
   - Witnesses running → skip
   - Refineries running → skip
6. Print success
```

#### 3. Multi-Rig Coordination

```
Town has 3 rigs: gastown, beads, greenplace

Startup order:
1. Daemon, Mayor, Deacon (sequential, verify each)
2. All Witnesses (parallel, but verify all before continuing)
3. All Refineries (parallel, but verify all before continuing)

If gastown-witness fails:
- Log error: "Failed to start gastown witness: <reason>"
- Continue starting beads-witness, greenplace-witness
- Daemon will retry gastown-witness on next heartbeat
```

#### 4. Partial Failures

```
beads-refinery fails to start after 3 retries

1. Log: "ERROR: beads-refinery failed to start after 3 attempts"
2. Continue with other agents
3. Print warning in final status:
   "✓ Gas Town started (1 agent failed - see logs)"
4. Daemon heartbeat will retry in 3 minutes
```

### Configuration Options

Add to `~/gt/settings/config.json`:

```json
{
  "startup": {
    "auto_start_daemon": true,        // Default: true
    "auto_start_rig_agents": true,    // Default: true (start witnesses/refineries)
    "health_check_timeout": "10s",    // Default: 10s per agent
    "max_retries": 3,                 // Default: 3 retries before giving up
    "parallel_rig_start": true        // Default: true (start all rigs in parallel)
  }
}
```

### Implementation Plan

#### Phase 1: Auto-Daemon in gt start

1. Add `ensureDaemonRunning()` to `start.go`
2. Call it first in `runStart()`
3. Add health check with 5s timeout
4. Test: `gt start` from clean state should start daemon

**Files**: `internal/cmd/start.go`

#### Phase 2: Always Start Rig Agents

1. Remove the `--all` flag check for rig agents
2. Start Witnesses and Refineries by default
3. Add `--no-rig-agents` flag for those who want old behavior

**Files**: `internal/cmd/start.go`

#### Phase 3: Health Checks with Retries

1. Add `waitForAgentHealthy(session, timeout, retries)` function
2. Integrate after each agent start
3. Add retry loop with backoff

**Files**: `internal/cmd/start.go`, `internal/session/health.go` (new)

#### Phase 4: Parallel Rig Agent Start

1. Start all Witnesses in goroutines
2. Wait for all with timeout
3. Start all Refineries in goroutines
4. Wait for all with timeout

**Files**: `internal/cmd/start.go`

#### Phase 5: Configuration

1. Add startup settings to config schema
2. Read and apply during gt start
3. Document in README

**Files**: `internal/config/settings.go`, `docs/configuration.md`

### Test Plan

| Test Case | Expected Result |
|-----------|-----------------|
| Fresh start with no daemon | Daemon auto-started, all agents up |
| Start with daemon already running | Daemon not restarted |
| Start with Mayor already running | Mayor not restarted |
| Kill Witness, run gt start | Witness restarted |
| Rig agent fails to start | Error logged, other agents continue |
| Config disables auto-daemon | Daemon not started |
| Health check timeout | Retry up to 3 times, then continue |

### Backward Compatibility

- `gt start --all` continues to work (but is now redundant)
- Users who explicitly run `gt daemon start` first see no change
- Users who never ran daemon manually get it for free

### Security Considerations

- Daemon uses file lock to prevent multiple instances
- No new privilege escalation
- Health checks are read-only (tmux session inspection)

### Open Questions

1. Should we add `gt stop` to complement `gt start`? (Currently `gt shutdown`)
2. Should failed rig agents be retried immediately or wait for daemon heartbeat?
3. Should crew auto-start be part of core initialization or remain separate?

## Conclusion

This design proposes making `gt start` the single entry point for Gas Town initialization. By auto-starting the daemon and always starting rig agents, users get a fully operational town with one command. The phased implementation allows incremental delivery with each phase providing standalone value.

---

**Next Steps**:
1. Review and approve design
2. Create implementation beads for each phase
3. Begin Phase 1 implementation
