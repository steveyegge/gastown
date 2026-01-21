## Summary

This PR consolidates four separate agent lifecycle managers into a unified factory architecture, replacing duplicated startup/shutdown logic with a single implementation and fixing several classes of bugs that were impossible to fix consistently in the old design.

**The core problem:** Every agent type (deacon, mayor, witness, refinery) had its own manager implementing startup, shutdown, zombie detection, and error handling. Bugs fixed in one manager weren't applied to others. Features added to one manager had to be reimplemented four times. The codebase accumulated subtle behavioral differences that caused user-facing issues.

**The solution:** A three-tier architecture where all agents share identical lifecycle logic:

```
Commands → factory.Start() / factory.Agents()
               ↓
         agent.Agents interface (lifecycle, zombie detection, readiness)
               ↓
         session.Sessions interface (tmux abstraction)
               ↓
         tmux.Tmux (concrete implementation)
```

---

## Why This Refactor? (The Case for Review)

### User-Facing Bugs Fixed

| Issue | Impact | Root Cause |
|-------|--------|------------|
| **#525**: `gt up` reports success but agent fails to start | Users think system is running when it isn't | `WaitForCommand` was non-fatal in deacon but fatal in mayor |
| **#566**: 812 refinery sessions spawned over 4 days | Resource exhaustion, system instability | `IsAgentRunning("node")` worked in 3 managers but not refinery (Claude reports version string) |
| Nudge messages garbled | Commands corrupted mid-execution | No mutex protecting concurrent nudges to same session |
| Zombie sessions block restart | `gt up` fails after crash | Some managers had zombie detection, others didn't |

These bugs share a pattern: **they were fixed in some managers but not others**, or the fix worked for some agent types but not all.

### Evidence: Commits That Touched Multiple Managers

The git history shows repeated multi-file fixes—a code smell indicating duplicated logic:

| Commit | What It Fixed | Files Touched |
|--------|---------------|---------------|
| `15d1dc8f` | Make WaitForCommand failures fatal | deacon, mayor, witness |
| `29f8dd67` | Grace period for restart loop detection | refinery, daemon, deacon |
| `ea8bef20` | Unify agent startup patterns | all 4 managers |
| `c91ab854` | Restore `--agent` override flag | deacon, crew |
| `0b807d06` | Serialize nudges to prevent interleaving | scattered across managers |

Each of these commits required understanding and modifying multiple files with subtly different patterns. The new architecture ensures these fixes happen in **one place** and apply to **all agent types**.

### Race Conditions Eliminated

| Race | Before | After |
|------|--------|-------|
| **Nudge interleaving** | Concurrent nudges garbled input | Per-session mutex serializes all nudges |
| **Startup race** | Fixed 8s sleep, prompt might not be ready | Polling `ReadinessChecker` with configurable timeout |
| **Zombie TOCTOU** | Check exists → race → create fails | `EnsureSessionFresh` atomically detects and kills |
| **HookBead race** | Polecat spawned before hook attached | Atomic cook → wisp → spawn sequence |

### Orphan Process Issues Fixed

| Issue | Before | After |
|-------|--------|-------|
| **setsid orphans** | `tmux kill-session` didn't kill process tree | Explicit `killDescendants()` walks tree deepest-first |
| **Zombie sessions** | Session exists but process dead | Unified zombie detection before all starts |
| **Multiple daemons** | Could run simultaneously | File locking prevents duplicates |

---

## What Changed

### Code Reduction

| Package | Change |
|---------|--------|
| `internal/deacon/manager.go` | Lifecycle logic removed, replaced by `factory.Start()` |
| `internal/mayor/manager.go` | Lifecycle logic removed, replaced by `factory.Start()` |
| `internal/witness/` | Manager startup consolidated into factory |
| `internal/refinery/` | Manager startup consolidated into factory |
| `internal/boot/` | Startup orchestration simplified |

The key win isn't raw line count—it's eliminating **duplicated logic** that had to be kept in sync across managers. Each manager previously implemented its own:
- Session creation and zombie detection
- Startup waiting and readiness checking
- Error handling and cleanup
- Environment variable setup

Now there's one implementation in `factory.Start()` that handles all agent types identically.

### New Packages

| Package | Purpose |
|---------|---------|
| `internal/agent/` | Unified `Agents` interface with segregated sub-interfaces |
| `internal/factory/` | Orchestration via `Start()` with functional options |

### New Files

| File | Purpose |
|------|---------|
| `internal/agent/interfaces.go` | Interface definitions (AgentObserver, AgentStopper, AgentStarter, AgentCommunicator, AgentRespawner) |
| `internal/agent/agent.go` | Production implementation wrapping `session.Sessions` |
| `internal/agent/double.go` | Full fake with spy capabilities for testing |
| `internal/agent/double_observer.go` | Minimal read-only fake |
| `internal/agent/stub.go` | Error injection wrapper |
| `internal/agent/start_config.go` | Per-start configuration (working dir, env, callbacks) |
| `internal/agent/hooks.go` | Startup hooks and readiness checking |
| `internal/agent/conformance_test.go` | Ensures Double matches Implementation behavior |

### Modified Files

| File | Change |
|------|--------|
| `internal/session/session.go` | Added `Sessions` interface |
| `internal/tmux/tmux.go` | Implements `Sessions`, added per-session nudge mutex |
| `internal/ids/ids.go` | Shared `AgentID` type with hierarchical addressing |

### Behavior Changes

| Behavior | Before | After |
|----------|--------|-------|
| Startup failure | Sometimes silent, sometimes fatal | Always fatal with cleanup |
| Zombie detection | Per-manager (often missing) | Automatic for all agents |
| Nudge serialization | None | Per-session mutex prevents interleaving |
| Graceful shutdown | Inconsistent | Configurable: SIGINT + wait vs immediate kill |

---

## New Capabilities (Free with the Refactor)

### Automatic Zombie Detection

```go
// Before: Manual check, different per manager, often missing
if tmux.HasSession(name) && isProcessRunning(name) { ... }

// After: Automatic for all agents
if agents.Exists(id) {
    // Guaranteed: session exists AND process is alive
}
```

Zombie detection now works at two levels:
- **Session-level:** tmux `has-session` check
- **Process-level:** `GetPaneCommand()` + `hasClaudeChild()` detects dead processes in live sessions

### Blocking-Until-Ready Startup

```go
// factory.Start() blocks until agent is actually ready
if err := factory.Start(townRoot, ids.MayorAddress); err != nil {
    return err // Agent failed to start or become ready
}
// Safe to send commands now
```

- Configurable `ReadinessChecker` interface (default: looks for `>` prompt)
- Configurable timeout (default 30s, Claude agents 60s)
- No more "hope the 8 second sleep was enough"

### Graceful vs Force Shutdown

```go
agents.Stop(id, true)   // Graceful: Ctrl-C, wait 100ms, then force
agents.Stop(id, false)  // Force: Immediately kill process tree
```

Both modes clean up descendant processes deepest-first to prevent orphans.

### Unified Status Reporting

```go
info, _ := agents.GetInfo(id)
// info.Name, info.Created, info.Attached, info.Windows, info.Activity

ids, _ := agents.List()
// All running agents, regardless of type
```

---

## Risk Mitigation

### Conformance Testing

The `agent.Double` test fake is verified against the real implementation:

```go
func TestConformance(t *testing.T) {
    testCases := []struct {
        name string
        test func(t *testing.T, agents agent.Agents)
    }{
        {"StartAlreadyRunning", testStartAlreadyRunning},
        {"StopRunning", testStopRunning},
        {"StopNotRunning", testStopNotRunning},
        {"WaitReadyRunning", testWaitReadyRunning},
        {"WaitReadyNotRunning", testWaitReadyNotRunning},
        // ...
    }

    for _, tc := range testCases {
        t.Run("Real/"+tc.name, func(t *testing.T) {
            tc.test(t, realAgents)
        })
        t.Run("Double/"+tc.name, func(t *testing.T) {
            tc.test(t, agent.NewDouble())
        })
    }
}
```

This ensures:
- Test doubles behave identically to production code
- Tests using doubles are testing realistic behavior
- Behavioral changes are caught by conformance failures

### Test Coverage Enabled by Doubles

Previously, testing manager logic required either spinning up real tmux sessions (slow, flaky) or no tests at all. The `agent.Double` fake enables **171 new unit tests** across manager packages:

| Package | Tests Using `agent.Double` |
|---------|---------------------------|
| `polecat/` | 54 tests |
| `refinery/` | 48 tests |
| `factory/` | 43 tests |
| `witness/` | 16 tests |
| `crew/` | 10 tests |

Plus **118 tests** in the `agent/` package itself (conformance, hooks, state management).

These tests run in milliseconds without tmux, making them suitable for CI and rapid iteration.

### Interface Segregation

Consumers depend only on the capabilities they need:

```go
// Status display only needs observation
func showStatus(agents agent.AgentObserver) { ... }

// Cleanup only needs stopping
func teardown(agents agent.AgentStopper) { ... }

// Full control for commands
func handleCommand(agents agent.Agents) { ... }
```

This limits blast radius of changes and makes dependencies explicit.

### Backwards Compatibility

All existing commands work unchanged:
- `gt up` / `gt down` / `gt status`
- `gt attach` / `gt logs`
- Agent override via `--agent` flag
- Environment variable passthrough

---

## Testing

- [x] `go test ./...` passes
- [x] Conformance tests verify Double matches Implementation
- [x] Integration tests via `gt up`, `gt down`, `gt status`
- [x] Manual testing: mayor/deacon/witness/refinery startup and shutdown
- [x] Zombie detection verified (kill process, confirm detection)
- [x] Graceful shutdown verified (Ctrl-C sent before force kill)

### Test Coverage

| Package | Coverage |
|---------|----------|
| `internal/agent` | Conformance + unit tests |
| `internal/session` | Conformance tests |
| `internal/factory` | Integration via commands |

---

## Migration Notes

For maintainers updating code that used the old managers:

**Starting an agent:**
```go
// Before
mgr := deacon.NewManager(tmux, townRoot)
mgr.Start(ctx, agentOverride)

// After
factory.Start(townRoot, ids.DeaconAddress, factory.WithAgent(agentOverride))
```

**Checking if agent exists:**
```go
// Before (no zombie detection)
tmux.HasSession(sessionName)

// After (with zombie detection)
agents := factory.Agents()
agents.Exists(ids.MayorAddress)
```

**Stopping an agent:**
```go
// Before
tmux.KillSession(sessionName)

// After (with descendant cleanup)
agents.Stop(ids.MayorAddress, true)  // graceful
```

**Testing with fakes:**
```go
// Before: Mock tmux directly, hope behavior matches
mock := &mockTmux{}

// After: Use conformance-tested double
fake := agent.NewDouble()
fake.CreateAgent(id, "/work/dir", "command")
// fake.StopCalls(), fake.GetNudgeLog(id), etc.
```

---

## Future Extensibility

The `session.Sessions` interface abstracts the execution environment, enabling:

| Future Backend | Use Case |
|----------------|----------|
| `remote.SSHSessions` | Run polecats on remote machines |
| `k8s.PodSessions` | Each polecat as a Kubernetes pod |
| `docker.ContainerSessions` | Isolated container execution |

The agent and factory layers require **no changes** to support new backends—only a new `Sessions` implementation.

---

## Checklist

- [x] Code follows project style (gofmt, golint)
- [x] Documentation updated (`docs/AGENT_ARCHITECTURE.md`)
- [x] No breaking changes to external APIs
- [x] Backwards compatible (all commands work as before)
- [x] Test doubles provided for all interfaces
- [x] Conformance tests verify double behavior
- [x] Error messages are clear and actionable

---

## Related Issues

**Issues Fixed by This Refactor:**
- Fixes #525: `gt up` reports success but agent fails to start
- Fixes #566: Runaway refinery session spawning (812 sessions over 4 days)

---

🤖 Generated with [Claude Code](https://claude.com/claude-code)
