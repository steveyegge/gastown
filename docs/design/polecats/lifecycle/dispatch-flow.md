# Polecat Dispatch Flow

> Visual reference for all dispatch paths and decision points

## Entry Point Matrix

| Entry Point | Command | File | Condition |
|-------------|---------|------|-----------|
| Manual sling | `gt sling <bead> <rig>` | `sling.go:161` | User invocation |
| Batch sling | `gt sling <bead1> <bead2> <rig>` | `sling_batch.go` | Multiple beads |
| Convoy launch | `gt convoy launch <convoy>` | `convoy_launch.go:55` | Epic dispatch |
| Scheduler | Daemon heartbeat (3 min) | `capacity_dispatch.go:26` | Deferred dispatch |
| Queue processor | Internal | `capacity_dispatch.go` | Capacity-controlled |

---

## Complete Dispatch Sequence Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DISPATCH ENTRY POINTS                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
        ┌─────────────────────────────┼─────────────────────────────┐
        │                             │                             │
        ▼                             ▼                             ▼
┌───────────────┐           ┌───────────────┐           ┌───────────────┐
│   gt sling    │           │ gt convoy     │           │  Scheduler    │
│  <bead> <rig> │           │    launch     │           │  (daemon)     │
└───────┬───────┘           └───────┬───────┘           └───────┬───────┘
        │                           │                           │
        ▼                           ▼                           ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                         RESOLVE TARGET                                     │
│                    sling_target.go:resolveTarget()                         │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  • Parse target (rig name, agent address, or explicit)              │  │
│  │  • Check IsRigName() → if true, dispatch to polecat                 │  │
│  │  • Check IsRigParked() → error if parked                            │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                      SHOULD DEFER DISPATCH?                                │
│                    sling_schedule.go:shouldDeferDispatch()                 │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  Load town settings → Get max_polecats                              │  │
│  │  • max_polecats <= 0 → Direct dispatch (spawn now)                  │  │
│  │  • max_polecats > 0  → Deferred dispatch (queue bead)               │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                      │
                    ┌─────────────────┴─────────────────┐
                    │                                   │
              [DIRECT]                            [DEFERRED]
                    │                                   │
                    ▼                                   ▼
┌───────────────────────────────┐   ┌───────────────────────────────────────┐
│      EXECUTE SLING            │   │    CREATE SLING CONTEXT BEAD          │
│ sling_dispatch.go:executeSling│   │    sling_schedule.go:scheduleBead()   │
│                               │   │  ┌─────────────────────────────────┐  │
│  [Continue to spawn below]    │   │  │  fields = {                     │  │
│                               │   │  │    WorkBeadID: beadID,          │  │
│                               │   │  │    TargetRig: rigName,          │  │
│                               │   │  │    EnqueuedAt: now(),           │  │
│                               │   │  │  }                              │  │
│                               │   │  │  CreateSlingContext(fields)     │  │
│                               │   │  └─────────────────────────────────┘  │
└───────────────┬───────────────┘   └───────────────────┬───────────────────┘
                │                                       │
                │                                       ▼
                │                   ┌───────────────────────────────────────┐
                │                   │   SCHEDULER HEARTBEAT (3 min cycle)   │
                │                   │   capacity_dispatch.go                │
                │                   │  ┌─────────────────────────────────┐  │
                │                   │  │  1. Query open sling contexts   │  │
                │                   │  │  2. Filter by readiness         │  │
                │                   │  │  3. Check available capacity    │  │
                │                   │  │  4. Plan dispatch (batch_size)  │  │
                │                   │  │  5. Execute for each slot       │  │
                │                   │  └─────────────────────────────────┘  │
                │                   └───────────────────┬───────────────────┘
                │                                       │
                └───────────────────┬───────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                           ADMISSION CONTROL                                │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  ✓ Parked rig check (IsRigParked)                                   │  │
│  │  ✓ Capacity check (max_polecats - active)                           │  │
│  │  ✓ Bead readiness (no blockers)                                     │  │
│  │  ✓ Circuit breaker (dispatch_failures < 3)                          │  │
│  │  ✓ Database connection (Dolt available)                             │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                     SPAWN POLECAT FOR SLING                                │
│                     polecat_spawn.go:SpawnPolecatForSling()                │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  1. AllocateName() or FindIdlePolecat()                             │  │
│  │  2. AddWithOptions() → Create worktree                              │  │
│  │     OR RepairWorktreeWithOptions() → Fix existing                   │  │
│  │  3. Return SpawnedPolecatInfo                                       │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                         START SESSION                                      │
│                         session_manager.go:Start()                         │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  186-208:  Check & kill stale sessions                              │  │
│  │  210-241:  Load runtime config, resolve agent                       │  │
│  │  244-249:  Ensure settings directory                                │  │
│  │  251-266:  Create beacon (work assignment)                          │  │
│  │  268-284:  Build startup command with beacon                        │  │
│  │  285-292:  Inject BD_DOLT_AUTO_COMMIT=off                           │  │
│  │  307-317:  Inject GT_RIG, GT_POLECAT, GT_ROLE, paths                │  │
│  │  ─────────────────────────────────────────────────────────────────  │  │
│  │  321:      ████ tmux.NewSessionWithCommand() ████  [SESSION BORN]   │  │
│  │  ─────────────────────────────────────────────────────────────────  │  │
│  │  325-361:  tmux.SetEnvironment() for each var                       │  │
│  │  368-374:  HookIssueToPolecat()                                     │  │
│  │  376-382:  Apply theme + crash detection hook                       │  │
│  │  384-421:  WaitForReady() + NudgeSession() fallbacks                │  │
│  │  423-449:  VerifySurvived() - ensure session didn't crash           │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                         WAKE RIG AGENTS                                    │
│                         sling_helpers.go:wakeRigAgents()                   │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  1. exec.Command("gt", "rig", "boot", rigName)                      │  │
│  │     → Starts witness if not running                                 │  │
│  │  2. Check daemon is running (warn if not)                           │  │
│  │  3. NudgeSession(witness, "Polecat dispatched - check for work")    │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                    POLECAT EXECUTES AUTONOMOUSLY                           │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  1. gt prime --hook                                                 │  │
│  │     → Loads role context                                            │  │
│  │     → Reads hooked bead                                             │  │
│  │     → Begins work                                                   │  │
│  │  2. ... work execution ...                                          │  │
│  │  3. git add && git commit && git push                               │  │
│  │  4. gt done                                                         │  │
│  │     → Creates MR bead                                               │  │
│  │     → Sets done-intent label                                        │  │
│  │     → Notifies witness/refinery                                     │  │
│  │     → Session exits                                                 │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                           CLEANUP PIPELINE                                 │
│                           witness/handlers.go                              │
│  ┌─────────────────────────────────────────────────────────────────────┐  │
│  │  1. Witness detects done-intent or session exit                     │  │
│  │  2. Refinery processes MR                                           │  │
│  │  3. Merge to main                                                   │  │
│  │  4. MERGED signal sent to Witness                                   │  │
│  │  5. Witness verifies commit on main                                 │  │
│  │  6. Witness checks cleanup_status                                   │  │
│  │  7. NukePolecat() → Remove worktree                                 │  │
│  │  8. Close work bead                                                 │  │
│  └─────────────────────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────────────────────┘
```

---

## Decision Points Quick Reference

### Is Target a Rig?

```go
// sling_target.go
if rigName, isRig := IsRigName(target); isRig {
    // Dispatch to polecat
}
```

### Is Rig Parked?

```go
// rig_park.go:179-184
func IsRigParked(townRoot, rigName string) bool {
    wispCfg := wisp.NewConfig(townRoot, rigName)
    return wispCfg.GetString("status") == "parked"
}
```

### Should Defer Dispatch?

```go
// sling_schedule.go:19-44
maxPol := settings.Scheduler.GetMaxPolecats()
if maxPol > 0 {
    return true  // Deferred
}
return false    // Direct
```

### Is Capacity Available?

```go
// capacity_dispatch.go:101-108
active := countActivePolecats()
cap := maxPolecats - active
if cap <= 0 {
    return 0  // No slots
}
return cap
```

### Is Bead Ready?

```go
// capacity_dispatch.go:381-384
if !readyWorkIDs[fields.WorkBeadID] {
    continue  // Has blockers, skip
}
```

### Circuit Breaker Tripped?

```go
// capacity_dispatch.go:376-379
if fields.DispatchFailures >= 3 {
    continue  // Too many failures, skip
}
```

---

## tmux Session Creation

**The actual session birth happens at line 321:**

```go
// session_manager.go:321
if err := m.tmux.NewSessionWithCommand(sessionID, workDir, command); err != nil {
    return nil, fmt.Errorf("creating session: %w", err)
}
```

**tmux command executed:**
```bash
tmux -u new-session -d -s gt-gastown-Toast -c /path/to/worktree 'export GT_RIG=gastown ... && claude-code "<beacon>"'
```

**Flags:**
- `-u`: UTF-8 mode
- `-d`: Detached (no attach)
- `-s`: Session name
- `-c`: Working directory

---

## Timing Reference

| Phase | Typical Duration |
|-------|------------------|
| Target resolution | < 10ms |
| Parked check | < 10ms |
| Worktree creation | 1-5s (git operations) |
| Session creation | < 100ms |
| Claude startup | 5-15s |
| Beacon delivery | < 100ms |
| Total spawn time | 10-25s |
| Scheduler heartbeat | Every 3 min |
| Zombie detection | Every patrol cycle (~1 min) |
| Hung threshold | 30 min |
| Done-intent timeout | 60s |
