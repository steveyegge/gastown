# Polecat Edge Cases & Failure Modes

> Detailed analysis of failure modes, race conditions, and zombie patterns

## Zombie Classification

The Witness agent monitors polecats and detects several zombie classes:

### 1. Stuck-in-Done Zombie

**File:** `internal/witness/handlers.go:1139-1154`

**Condition:** Polecat runs `gt done` but session hangs before cleanup completes.

**Detection:**
- `done-intent` label exists on agent bead
- Label timestamp > 60 seconds old
- tmux session still alive

**Code Path:**
```go
detectZombieLiveSession() → checks doneIntent.Timestamp
```

**Recovery:**
1. Witness kills tmux session
2. Witness continues cleanup pipeline
3. Worktree nuked if clean

**Risk:** If kill fails, polecat persists in undefined state.

---

### 2. Agent-Dead-in-Session Zombie

**File:** `internal/witness/handlers.go:1156-1171, 1066-1081`

**Condition:** tmux session exists but Claude process has crashed inside it.

**Detection:**
```go
t.IsAgentAlive(sessionName) == false
```

**Reference:** `gt-kj6r6` - agent process dies while tmux lives

**Recovery:**
1. NukePolecat()
2. Reset abandoned bead to open

**Edge Case:** Multiple checks occur (lines 1066 and 1157). Potential race if session
recreated between checks.

---

### 3. Bead-Closed-Still-Running Zombie

**File:** `internal/witness/handlers.go:1173-1189, 1083-1098`

**Condition:** Hooked bead is closed but polecat session still running.

**Detection:**
- Agent alive
- `getBeadStatus(hookBead) == "closed"`

**Reference:** `gt-h1l6i` - polecat occupying slot without work

**Problem:** Doesn't detect `gt done` incompleteness - just closed bead state.

---

### 4. Agent-Hung Zombie

**File:** `internal/witness/handlers.go:1100-1123`

**Condition:** Agent process alive but no tmux output for 30+ minutes.

**Threshold:**
```go
HungSessionThresholdMinutes = 30  // Line 28
```

**Reference:** `gt-tr3d` - infinite loop or blocked call

**Warning:** Conservative threshold may miss slower operations. Some legitimate work
(large refactors, complex builds) may take longer.

**Recovery:**
1. Kill session
2. Reset abandoned bead

---

### 5. Done-Intent-Dead Zombie

**File:** `internal/witness/handlers.go:1193-1211`

**Condition:** Session is dead but `done-intent` label still exists.

**Detection:**
- No tmux session
- `done-intent` label found on agent bead

**Recovery:**
- Auto-nuke if `cleanup_status` permits
- May have incomplete cleanup state

---

## Orphan Detection

**File:** `internal/witness/handlers.go:1248-1292`

**Condition:** Polecat directory exists but:
- No tmux session
- No `hook_bead` attached

**State Assessment:**
- **Clean:** No uncommitted changes, no stash, no unpushed → auto-nuke
- **Dirty:** Has uncommitted/stash/unpushed → escalate to Mayor

**State Transitions:**
- Tracked via `cleanup_status` field on agent bead
- Valid values: `clean`, `has_uncommitted`, `has_stash`, `has_unpushed`, `unknown`

---

## Race Conditions

### TOCTOU (Time-of-Check-Time-of-Use) Pattern

**File:** `internal/witness/handlers.go:1005-1008, 1222-1235`

The codebase implements TOCTOU guards to prevent nuking newly-spawned sessions:

```go
detectedAt := time.Now()  // Record detection time
// ... zombie checks happen ...
if sessionRecreated(t, sessionName, detectedAt) {
    return zombie, false  // Skip nuke if recreated after detection
}
```

**Implementation:** `sessionRecreated()` function (lines 1923-1935)

```go
func sessionRecreated(t *tmux.Tmux, name string, detectedAt time.Time) bool {
    creationTime := t.SessionCreationTime(name)
    return creationTime.After(detectedAt)
}
```

**Vulnerability:** If system clock drifts or tmux reports inaccurate creation times,
guard may fail.

---

### Multiple Dispatch Race

**File:** `internal/cmd/sling_dispatch.go:103-151`

**Dead-Agent Auto-Force:**

```go
if info.Status == "hooked" && info.Assignee != "" && isHookedAgentDeadFn(info.Assignee) {
    params.Force = true  // Auto-force when hooked agent's session is confirmed dead
}
```

**Race Window:** Between determining agent is dead and forcing dispatch, new polecat
could spawn.

**Mitigation:** Sends `LIFECYCLE:Shutdown` to witness (lines 142-161):

```go
if info.Status == "hooked" && params.Force && info.Assignee != "" {
    // Send LIFECYCLE:Shutdown to old polecat's witness
    // Notifies that hook was stolen
}
```

**Remaining Risk:** Window exists between detection and message delivery. Old polecat
continues running unaware until next patrol.

---

### Hook Attachment Race

**File:** `internal/cmd/sling_dispatch.go:77-80`

Between polecat spawn and hook attachment:
1. Polecat name allocated
2. Worktree created
3. Session started
4. **Hook attachment happens here** (with Dolt retry logic)
5. Polecat starts executing

**Risk:** If hook attachment fails and retries, polecat may start before hook is set.
GUPP (gt prime --hook) may not fire because hook not yet attached.

---

### Concurrent Polecats on Same Issue

**Prevention Mechanisms:**

1. **Hook is exclusive** - only one agent can hook a bead at a time
2. **Git branch naming** - includes unique suffix `@<timestamp>`
3. **TOCTOU guard** - records `detectedAt`, re-verifies before destructive action

**Failure Mode:** Second session fails to push (branch diverged) and escalates.

**Gap:** No visible prevention of simultaneous dispatch. Race between hook check and
hook assignment.

---

## Session Creation Failures

**File:** `internal/session/lifecycle.go:136-264`

### Partial Failure Handling

| Step | Line | Failure Handling | Risk |
|------|------|------------------|------|
| tmux creation | 184-186 | Return error | Polecat allocated but session never starts |
| RemainOnExit | 190 | Ignored (`_ =`) | Session may not persist for debugging |
| SetEnvironment | 204-208 | Ignored | Variables missing in session |
| WaitForAgent | 216-223 | May be silently ignored | Agent not ready but proceed |
| Theme application | 212 | Ignored (`_ =`) | No crash detection hook |
| Auto-respawn hook | 227-229 | Logged but continue | Hook not installed |
| VerifySurvived | 247-256 | Return error | Session died during startup |

**Impact:** Partial state leaves resources (polecat name, worktree) allocated but unusable.

---

## Cleanup Failures

**File:** `internal/witness/handlers.go:289-368`

### HandleMerged Failure Classes

**1. Commit Not on Main:**
```go
// Lines 314-324
if !commitOnMain {
    // Verification fails → blocks nuke with error
    // Creates cleanup wisp, escalates to Mayor
}
```

**Risk:** MERGED signal may be stale.

**2. Dirty cleanup_status:**

| Status | Lines | Action |
|--------|-------|--------|
| `has_uncommitted` | 334-339 | Block nuke, escalate |
| `has_stash` | 340-344 | Block nuke, escalate |
| `has_unpushed` | 361-365 | Block nuke, "DO NOT NUKE" warning |
| `unknown/empty` | 347-355 | Assume clean if commit on main |

**3. Nuke Failure:**
```go
// Lines 340-344, 361-365
if nukeErr := NukePolecat(); nukeErr != nil {
    // Records error in result
    // Cleanup wisp persists for manual intervention
    // Next patrol cycle may retry
}
```

**Gap:** No retry limit visible. Failed nukes could accumulate.

---

## Dolt Retry Logic Issues

**File:** `internal/polecat/manager.go:60-92`

### Configuration

| Setting | Value |
|---------|-------|
| Max retries | 10 |
| Base backoff | 500ms |
| Max backoff | 30s |
| Jitter | ±25% |

### Config Error Detection

```go
func isDoltConfigError(err error) bool {
    // Detects: "not initialized", "no such table", "no database", etc.
}
```

**Problem:** Comment references `gt-2ra: polecat spawn hang when Dolt DB not initialized`

These errors SHOULD be retried if Dolt is still starting up. Current logic treats
them as permanent config errors and aborts early.

**Gap:** No retry budget exhaustion logging visible.

---

## Mail Routing Failures

**File:** `internal/witness/handlers.go:142-161`

### Non-Fatal Error Handling

```go
if nudgeErr := nudgeRefinery(townRoot, rigName); nudgeErr != nil {
    if result.Error == nil {
        result.Error = fmt.Errorf("nudging refinery: %w (non-fatal)", nudgeErr)
    }
}
```

**Impact of Failure:**
- MERGE_READY not received by Refinery
- Refinery won't start merge processing
- Work stalls until next patrol cycle
- **No alarm raised to operator**

---

## Parsing & State Interpretation Gaps

**File:** `internal/witness/handlers.go:550-598`

### cleanup_status Parsing

```go
// Lines 583-595
// String parsing from agent bead description
// Case-insensitive prefix match: "cleanup_status:"
```

**Problem:** If description format changes, parsing silently fails.

**Assumption Risk:**
- Empty `cleanup_status` → assumed "clean"
- May nuke polecat with uncommitted work if description parsing fails

---

## Missing Crash Loop Detection

**Documented:** `docs/design/polecat-lifecycle-patrol.md` (line 343)

**Requirement:** "3+ crashes on same step triggers escalation"

**Code Status:**
- Referenced but implementation not clearly visible in handlers
- No crash counter visible in `ZombieResult`
- No escalation to Mayor on crash loop detected
- Likely handled elsewhere (Daemon or Boot)

---

## tmux Server Outage

**File:** `internal/witness/handlers.go:1045-1050`

```go
sessionAlive, err := t.HasSession(sessionName)
if err != nil {
    result.Errors = append(result.Errors, ...)
    continue  // Skip this polecat entirely
}
```

**Impact:**
- If tmux server becomes unavailable, all zombies go undetected
- No fallback detection mechanism
- Subsequent patrol cycles may also fail

---

## Summary: Risk Matrix

| Issue | Likelihood | Impact | Detection |
|-------|------------|--------|-----------|
| Zombie stuck-in-done | Medium | Medium | Good (60s timeout) |
| Agent-dead-in-session | Medium | Low | Good (IsAgentAlive) |
| TOCTOU race (nuke new session) | Low | High | Mitigated (detectedAt) |
| Multiple dispatch collision | Low | Medium | Partial (auto-force) |
| Hook attachment race | Low | Medium | Poor |
| Partial session startup | Medium | Medium | Poor (ignored errors) |
| Dolt config vs transient | Low | Medium | Poor |
| Mail delivery failure | Low | Medium | Poor (no alarm) |
| cleanup_status parsing | Low | High | Poor |
| tmux server outage | Very Low | High | None |
