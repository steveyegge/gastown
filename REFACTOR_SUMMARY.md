# Refactor Summary: Unified Factory Architecture

This document summarizes the changes, bugs fixed, and motivation for the unified agent architecture refactor.

---

## Motivation

The Gas Town codebase had evolved with **four separate manager implementations** for agent lifecycle:
- `internal/deacon/manager.go`
- `internal/mayor/manager.go`
- `internal/witness/manager.go`
- `internal/refinery/manager.go`
- `internal/boot/` startup orchestration

Each manager implemented its own:
- Session creation logic
- Zombie detection (or lack thereof)
- Startup waiting behavior
- Error handling patterns
- Environment variable setup

This led to **systematic bugs** where fixes in one manager weren't applied to others, and **feature regressions** when refactors touched multiple files.

---

## Bugs Implicitly Fixed

### Race Conditions Eliminated

| Bug | Before | After |
|-----|--------|-------|
| **Nudge interleaving** | Concurrent nudges to same session caused garbled input | Per-session mutex (`sessionNudgeLocks`) serializes all nudges |
| **HookBead race** | Polecat spawned before hook attached | Cook → wisp → spawn with HookBead atomic |
| **Startup race** | Fixed 8s sleep, prompt might not be ready | Polling via `ReadinessChecker` with configurable timeout |
| **Zombie TOCTOU** | Check exists → race → create fails | `EnsureSessionFresh` atomically detects and kills zombies |

**Key commit:** `0b807d06` - serialize nudges to prevent interleaving

### Orphan Process Issues Fixed

| Issue | Root Cause | Fix |
|-------|------------|-----|
| **812 refinery sessions spawned** | `IsAgentRunning("node")` too strict; Claude reports version string | Unified `IsClaudeRunning()` handles node/claude/version patterns |
| **Zombie sessions on restart** | Session exists but process dead, blocks new start | Zombie detection in `StartWithConfig` kills before recreating |
| **Orphan daemons** | Multiple daemons could run simultaneously | File locking prevents duplicate daemons |
| **setsid orphans** | `tmux kill-session` didn't kill process tree | Explicit descendant killing in `Stop()` |

**Key commits:**
- `f89ac47f` - kill pane process explicitly to prevent setsid orphans
- `c25ff34b` - prevent orphan daemons via file locking
- `29f8dd67` - fix runaway refinery session spawning

### State Inconsistencies Fixed

| Issue | Before | After |
|-------|--------|-------|
| **Status() vs reality** | Persisted state could differ from tmux | Bidirectional reconciliation in `Status()` |
| **WaitForCommand non-fatal** | `gt up` reported success when Claude failed | Fatal error handling, cleanup zombie on timeout |
| **Agent override lost** | Refactors broke `--agent` flag | Unified `StartConfig` preserves all options |

**Key commit:** `15d1dc8f` - Make WaitForCommand/WaitForRuntimeReady fatal in manager Start()

### Error Handling Gaps Closed

- **Callback failures**: Session cleaned up if `OnCreated` fails
- **Startup hook failures**: Non-fatal but logged, startup continues
- **Stop on nonexistent**: Idempotent, returns nil
- **Respawn on nonexistent**: Returns `ErrNotRunning`

### Resource Leaks Plugged

| Leak Type | Fix |
|-----------|-----|
| **Goroutine leaks** | `WaitReady` runs in background, doesn't block caller |
| **Scrollback buildup** | `ClearHistory()` called before respawn |
| **Descendant processes** | `killDescendants()` walks tree deepest-first |

---

## New Features for Free

### Zombie Detection

```go
// Before: Manual per-manager implementation (often missing)
// After: Automatic for all agents with configured ProcessNames

agents := agent.New(tmux, &agent.Config{
    ProcessNames: []string{"claude", "node"},
})

// Exists() returns false if session alive but process dead
if !agents.Exists(id) {
    // Agent is zombie or not running
}
```

- Session-level: tmux `has-session` check
- Process-level: `GetPaneCommand()` + `hasClaudeChild()` for shell wrappers
- Automatic cleanup: `StartWithConfig` kills zombies before creating new session

### Blocking-Until-Ready Startup

```go
// factory.Start() blocks until agent prompt is visible
if err := factory.Start(townRoot, ids.MayorAddress); err != nil {
    return err // Agent failed to become ready
}
// Agent is now ready for commands
```

- Configurable `ReadinessChecker` interface
- Built-in `PromptChecker` looks for `>` prefix
- Configurable timeout (default 30s, Claude 60s)
- Fallback `StartupDelay` for agents without prompt detection

### Graceful vs Force Shutdown

```go
// Graceful: Send Ctrl-C, wait 100ms, then force
agents.Stop(id, true)

// Force: Immediately kill all descendants
agents.Stop(id, false)
```

- Graceful sends SIGINT first, allows cleanup
- Force skips signal, directly kills process tree
- Both clean up descendants deepest-first to prevent orphans

### Agent-Agnostic Nudge/Capture

```go
// Same interface for all agent types
agents.Nudge(ids.MayorAddress, "Check status")
agents.Nudge(ids.WitnessAddress("myrig"), "Start patrol")
agents.Nudge(ids.PolecatAddress("myrig", "Toast"), "Continue")

output, _ := agents.Capture(id, 50)  // Last 50 lines
full, _ := agents.CaptureAll(id)     // Entire scrollback
```

- Per-session mutex prevents interleaved nudges
- Vim mode detection and retry logic
- Consistent across mayor, deacon, witness, refinery, polecat, crew

### Unified Status Reporting

```go
info, err := agents.GetInfo(id)
// info.Name, info.Created, info.Attached, info.Windows, info.Activity
```

- Consistent `session.Info` struct for all agent types
- Enables unified status displays and monitoring

---

## Motivating Commits/PRs

### Bugs That Touched Multiple Managers

| Commit | Title | Files Touched |
|--------|-------|---------------|
| `15d1dc8f` | Make WaitForCommand fatal | deacon, mayor, witness |
| `29f8dd67` | Grace period for restart loop | refinery, daemon, deacon |
| `ea8bef20` | Unify agent startup with Manager pattern | all 4 managers |
| `c91ab854` | Restore agent override support | deacon, crew |

### PRs with Duplicated Logic

The manager refactor (`ea8bef20`) was motivated by discovering duplicate code:
- Session startup logic in `up.go`, `start.go`, `mayor.go`
- Zombie detection reimplemented per-manager
- Environment variable handling scattered

### Test Failures from Inconsistent Patterns

- **Issue #525**: `gt up` reports success but deacon fails to start
  - Root cause: Non-fatal `WaitForCommand` in some managers but not others
  - Fix: `15d1dc8f` made all managers consistent

- **Issue #566**: 812 refinery sessions spawned over 4 days
  - Root cause: `IsAgentRunning("node")` worked for most, but Claude reports version
  - Fix: `29f8dd67` unified detection with `IsClaudeRunning()`

### Feature Requests Blocked by Old Architecture

| Feature | Blocker | Now Enabled |
|---------|---------|-------------|
| **Horizontal scaling** | Per-manager session handling | `Sessions` interface abstracts backend |
| **Alternative agents** | Hardcoded Claude detection | Configurable `ProcessNames` and `ReadinessChecker` |
| **Test doubles** | No clean interface boundaries | `agent.Double` implements `Agents` interface |
| **Conformance testing** | Different manager behaviors | Single implementation, one conformance suite |

### Code Review Feedback Addressed

The `24289c72` commit ("address review findings for factory.Start() refactor") consolidated:
- P0: Restore refinery notifications, parallel startup
- P1: Beads version caching, deacon grace period
- P2: Cross-platform signal handling
- P3: Route `done.go` through `factory.Agents()`, fix Status() reconciliation

---

## Consolidated Code

The refactor unified lifecycle logic across:

| Package | Change |
|---------|--------|
| `internal/deacon/` | Manager startup replaced by `factory.Start()` |
| `internal/mayor/` | Manager startup replaced by `factory.Start()` |
| `internal/witness/` | Manager startup consolidated into factory |
| `internal/refinery/` | Manager startup consolidated into factory |
| `internal/boot/` | Startup orchestration simplified |

**Key benefit:** Duplicated lifecycle logic now lives in one place (`factory.Start()` + `agent.Agents`)

---

## Architecture Summary

```
Before:
  cmd/start.go → deacon.Manager.Start()
  cmd/start.go → mayor.Manager.Start()
  cmd/start.go → witness.Manager.Start()
  cmd/start.go → refinery.Manager.Start()
  (4 different implementations, 4 sets of bugs)

After:
  cmd/start.go → factory.Start(townRoot, agentID)
                        ↓
                 agent.Agents interface
                        ↓
                 session.Sessions interface
                        ↓
                 tmux.Tmux implementation
  (1 implementation, 1 set of behaviors, 1 conformance test)
```

---

## Verification

All behaviors verified by:
- `internal/agent/conformance_test.go` - Tests Double matches Implementation
- `internal/agent/agent_test.go` - Unit tests for zombie detection, callbacks
- `internal/session/conformance.go` - Session interface conformance
- Integration tests via `gt up`, `gt down`, `gt status` commands

### Test Coverage Enabled by Doubles

The `agent.Double` fake enables **171 new unit tests** across manager packages that previously couldn't be unit tested without real tmux:

| Package | Tests Using `agent.Double` |
|---------|---------------------------|
| `polecat/` | 54 tests |
| `refinery/` | 48 tests |
| `factory/` | 43 tests |
| `witness/` | 16 tests |
| `crew/` | 10 tests |

Plus **118 tests** in the `agent/` package itself.

These tests run without tmux, enabling fast CI and rapid iteration.
