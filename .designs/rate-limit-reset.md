# Design: Rate Limit Reset Handling

> **Status**: Design Exploration
> **Bead**: gt-oix
> **Blocks**: gt-y5m (implementation)
> **Author**: nux (polecat)
> **Date**: 2026-01-28

## Problem Statement

When Claude Pro/Max users hit usage limits, all Claude Code sessions halt. Gas Town
currently has no mechanism to automatically resume work when rate limits reset.

**Impact**:
- All polecats stop mid-task
- Deacon, witnesses, and refineries become unresponsive
- Work sits idle until human intervention
- The entire propulsion system stalls

**Constraint**: The solution must operate **outside** Claude Code, since Claude Code
itself is halted when rate-limited.

## Requirements

### Must Have
1. Detect when rate limit is hit (capture reset time)
2. Persist waiting state (survives daemon restarts)
3. Wake agents when rate limit resets
4. Work without human intervention

### Should Have
5. Graceful degradation during rate limit period
6. Preserve work context across the rate limit gap
7. Prioritized restart order (Deacon first, then cascading)
8. Observable status (dashboard, `gt status`)

### Nice to Have
9. Configurable behavior per role
10. Cost tracking for rate limit events
11. Proactive warnings before hitting limits

---

## Exploration Dimension 1: Detection Mechanisms

### Option 1A: Capture from Claude Code Output

**Approach**: Parse Claude Code's stderr/stdout for rate limit messages.

**Detection Points**:
- Tmux pane capture (`tmux capture-pane`)
- Process stdout/stderr redirection
- Claude Code log files (if any)

**Pros**:
- Direct source of truth
- Captures exact reset time from API response

**Cons**:
- Rate limit message format may change without notice
- Requires parsing unstructured text
- May not be visible in all output modes

**Implementation Sketch**:
```go
// In daemon heartbeat or dedicated monitor
func (d *Daemon) checkForRateLimit(sessionName string) (*RateLimitInfo, error) {
    output, _ := d.tmux.CapturePane(sessionName, 50) // last 50 lines
    if match := rateLimitPattern.FindStringSubmatch(output); match != nil {
        resetTime, _ := time.Parse(time.RFC3339, match[1])
        return &RateLimitInfo{
            DetectedAt: time.Now(),
            ResetsAt:   resetTime,
            Session:    sessionName,
        }, nil
    }
    return nil, nil
}
```

### Option 1B: Claude Code Exit Code Detection

**Approach**: Monitor for specific exit codes indicating rate limit.

**Pros**:
- Clean signal (exit code vs 0)
- No text parsing required

**Cons**:
- Claude Code may not have rate-limit-specific exit codes
- Loses the reset time information
- Can't distinguish rate limit from other failures

### Option 1C: Keepalive Pattern + Heuristics

**Approach**: Use existing keepalive system to detect "agent stopped without handoff".

**REJECTED**: This approach doesn't work because agents only receive the rate limit
message when they receive a prompt AFTER the rate limit is hit. Idle agents never
hit the rate limit, so "all agents stopped simultaneously" is not a reliable signal.

### Option 1D: Anthropic API Probe (External)

**Approach**: Daemon directly queries Anthropic API for rate limit status.

**Pros**:
- Authoritative source
- Gets exact reset time
- Works when Claude Code is halted

**Cons**:
- Requires API key access in daemon
- May itself be rate-limited
- Privacy/security concerns (daemon shouldn't have API key)

### Recommendation: 1A (Tmux Output Parsing)

Parse tmux output for rate limit messages. The message format is documented in
the original GitHub issue. When a rate-limited session is detected, record the
reset time and mark that agent as rate-limited.

No heuristic fallback needed - we simply check agents that are marked as
rate-limited and poke them to continue when the time comes.

---

## Exploration Dimension 2: Solution Architecture

### Option 2A: Enhanced Daemon (Go Process)

**Approach**: Extend the existing daemon to handle rate limit wake-ups.

**Components**:
```
Daemon (Go process)
├── Existing: 3-min heartbeat, Boot spawn, lifecycle requests
├── New: Rate limit monitor
├── New: Wake scheduler (via time.AfterFunc or cron lib)
└── New: Staged restart orchestration
```

**Pros**:
- Daemon already survives rate limits (it's Go, not Claude)
- Existing session restart infrastructure
- Single process to manage

**Cons**:
- Daemon currently is "dumb transport" by design
- Adding scheduling adds complexity
- If daemon crashes, wake-up is lost

**Implementation**:
```go
type RateLimitState struct {
    Active     bool      `json:"active"`
    DetectedAt time.Time `json:"detected_at"`
    ResetsAt   time.Time `json:"resets_at"`
    Agents     []string  `json:"affected_agents"` // Sessions to restart
}

func (d *Daemon) scheduleWakeUp(resetTime time.Time) {
    // Add buffer for API recovery
    wakeTime := resetTime.Add(2 * time.Minute)

    d.wakeTimer = time.AfterFunc(time.Until(wakeTime), func() {
        d.executeRateLimitRecovery()
    })
}
```

### Option 2B: External Scheduler (cron/systemd timer)

**Approach**: Use OS scheduling to trigger wake-up.

**Components**:
```
Rate Limit Detection → Write wake time to file
                    → Schedule cron job / at job / systemd timer

At wake time → Script runs `gt up` or similar
            → Daemon notices, starts cascade
```

**Pros**:
- Survives daemon crashes
- OS-level reliability
- Works even if daemon is restarted

**Cons**:
- Adds external dependency
- Platform-specific (cron vs Windows Task Scheduler)
- Harder to observe/debug

**Implementation**:
```bash
# Scheduled job at reset time
#!/bin/bash
gt daemon wake-from-rate-limit
```

### Option 2C: Hybrid: Daemon + Persistent File

**Approach**: Daemon handles scheduling, but persists state to survive restarts.

**Components**:
```
Detection → Write rate-limit-state.json
         → Daemon schedules in-memory timer

If daemon restarts → Reads rate-limit-state.json
                   → Checks if past reset time → immediate wake
                   → Otherwise → reschedules timer
```

**State File**:
```json
{
    "active": true,
    "detected_at": "2026-01-28T15:00:00Z",
    "resets_at": "2026-01-28T16:00:00Z",
    "affected_agents": [
        "hq-deacon",
        "gt-gastown-witness",
        "gt-gastown-polecat-Toast"
    ],
    "recovery_status": "pending"
}
```

**Pros**:
- Survives daemon restarts
- Self-contained (no external schedulers)
- Observable via file

**Cons**:
- Slightly more complex than pure in-memory
- Must check file on daemon startup

### Recommendation: 2C (Daemon + Persistent State)

The hybrid approach gives us reliability (persisted state) without external
dependencies (cron). It fits the existing architecture where daemon.json and
heartbeat files already persist operational state.

---

## Exploration Dimension 3: Wake-Up Orchestration

### Option 3A: Simple Cascade

**Approach**: Restart all agents in fixed order.

```
1. Deacon (2-min grace period)
2. Witnesses (parallel, per-rig)
3. Refineries (parallel, per-rig)
4. Polecats with hooked work (sequential within rig)
```

**Pros**:
- Predictable behavior
- Deacon comes up first to coordinate

**Cons**:
- Fixed order may not be optimal
- All polecats restart even if no work

### Option 3B: Work-Driven Wake

**Approach**: Only restart agents with pending work.

```
1. Deacon (always, for coordination)
2. Witnesses (if any polecat has hooked work)
3. Polecats with hooked work only
4. Refineries with pending MRs only
```

**Pros**:
- Minimal cost (only wake what's needed)
- Faster recovery for critical work

**Cons**:
- More complex logic
- May miss agents that had context but no hooked work

### Option 3C: Deacon-Orchestrated Recovery

**Approach**: Wake Deacon only, let Deacon decide what else to wake.

```
1. Daemon wakes Deacon with rate-limit-recovery context
2. Deacon runs `gt status` to assess town state
3. Deacon spawns/wakes agents as needed
4. Deacon reports completion to daemon
```

**Pros**:
- Intelligent decision-making
- Deacon has context about town state
- Fits existing architecture (Deacon orchestrates)

**Cons**:
- Depends on Deacon coming up successfully
- Adds latency (AI startup time)
- What if Deacon itself is rate-limited again?

### Recommendation: 3A with 3B Optimization

Start with simple cascade (3A) for reliability. In the cascade:
- Skip agents without hooked work (3B optimization)
- Add grace periods between tiers to avoid immediate re-rate-limit

```go
func (d *Daemon) executeRateLimitRecovery() {
    // Tier 1: Core coordination
    d.restartIfDead("hq-deacon")
    time.Sleep(2 * time.Minute) // Let Deacon stabilize

    // Tier 2: Per-rig watchers
    for _, rig := range d.getKnownRigs() {
        d.restartIfDead(fmt.Sprintf("gt-%s-witness", rig))
    }
    time.Sleep(1 * time.Minute)

    // Tier 3: Workers with hooked work
    for _, agent := range d.getAgentsWithHookedWork() {
        d.restartIfDead(agent.SessionName)
        time.Sleep(30 * time.Second) // Stagger to avoid burst
    }
}
```

---

## Exploration Dimension 4: Context Preservation

### Key Insight: Context is Already Present

When a session hits a rate limit, the Claude Code session is **paused, not killed**.
The context remains in the session. Recovery is simple:

1. Send input "1" to select the retry option (if presented)
2. Send message: "Your rate limit has reset, continue your work"

**No checkpoint recovery or handoff mail needed** - the agent's context is intact.

### Implementation

```go
func (d *Daemon) pokeRateLimitedSession(sessionName string) {
    // Send "1" to select retry option if rate limit dialog is showing
    d.tmux.SendKeys(sessionName, "1", true)
    time.Sleep(500 * time.Millisecond)

    // Send continuation message
    d.tmux.SendKeys(sessionName, "Your rate limit has reset, continue your work", true)
    d.tmux.SendKeys(sessionName, "Enter", false)
}
```

### Recommendation

No complex context preservation needed. Simply poke rate-limited sessions with
a continuation message when their reset time arrives.

---

## Exploration Dimension 5: Edge Cases

### E1: Multiple Rate Limit Periods

**Scenario**: Rate limit hits, recovery starts, second rate limit hits.

**Solution**:
- Track recovery_in_progress flag
- If rate-limited during recovery, update reset time and re-schedule
- Don't double-restart agents

```go
type RateLimitState struct {
    // ... existing fields
    RecoveryInProgress bool      `json:"recovery_in_progress"`
    RecoveryStartedAt  time.Time `json:"recovery_started_at"`
}
```

### E2: Daemon Restart During Rate Limit

**Scenario**: Daemon crashes/restarts while waiting for rate limit reset.

**Solution**:
- Persist state to `daemon/rate-limit-state.json`
- On startup, check if past reset time → immediate recovery
- Otherwise → reschedule wake timer

```go
func (d *Daemon) onStartup() {
    state := d.loadRateLimitState()
    if state.Active {
        if time.Now().After(state.ResetsAt) {
            d.executeRateLimitRecovery()
        } else {
            d.scheduleWakeUp(state.ResetsAt)
        }
    }
}
```

### E3: User Upgrades Plan Mid-Wait

**Scenario**: User upgrades from Pro to Max during rate limit period.

**Solution**:
- Allow manual wake command: `gt daemon wake --force`
- Deacon can send lifecycle:restart mail to trigger early wake
- No automatic detection (daemon can't know about plan changes)

### E4: Different Reset Times Per Model

**Scenario**: Claude 4.5 Opus resets at 6pm, Haiku at 5pm.

**Solution**:
- Track reset times per agent/model
- Wake agents as their individual limits reset
- More complex but more efficient

**Simplification**: Use latest reset time for all (conservative but simpler).

### E5: Partial Rate Limit (Some Models Still Work)

**Scenario**: Opus is rate-limited but Haiku is available.

**Solution**:
- Detect which models are limited
- Downgrade roles to available models temporarily
- Requires model switching infrastructure

**Out of Scope**: This is complex. Initial implementation should treat any
rate limit as global.

---

## Exploration Dimension 6: Configuration

### Proposed Configuration Structure

```go
// In config/types.go
type RateLimitConfig struct {
    // Whether rate limit handling is enabled
    Enabled bool `json:"enabled"`

    // How often to check for rate limits (in heartbeat)
    CheckInterval string `json:"check_interval"` // "1m", "3m"

    // Buffer after reset time before waking
    WakeBuffer string `json:"wake_buffer"` // "2m" default

    // Grace period between agent restarts
    StagingInterval string `json:"staging_interval"` // "30s" default

    // Whether to skip agents without hooked work
    OnlyActiveAgents bool `json:"only_active_agents"` // true default
}

// Location: TownSettings.RateLimits
type TownSettings struct {
    // ... existing fields
    RateLimits *RateLimitConfig `json:"rate_limits,omitempty"`
}
```

### Default Values

```json
{
    "rate_limits": {
        "enabled": true,
        "check_interval": "3m",
        "wake_buffer": "2m",
        "staging_interval": "30s",
        "only_active_agents": true
    }
}
```

---

## Implementation Phases

### Phase 1: Detection & State (MVP)

**Goal**: Detect rate limits and persist state.

**Deliverables**:
- `checkRateLimitStatus()` in daemon heartbeat
- `RateLimitState` struct and file persistence
- Tmux output parsing for rate limit messages (format from GitHub issue)
- `gt daemon rate-limit-status` command

**Effort**: Small
**Risk**: Low

### Phase 2: Scheduled Poke

**Goal**: Automatically poke rate-limited agents at reset time.

**Deliverables**:
- In-memory timer scheduling
- State recovery on daemon restart
- `pokeRateLimitedSession()` - send "1" + continuation message
- Simple cascade with grace periods between agents
- KRC event logging

**Effort**: Medium
**Risk**: Medium (timer reliability)

### Phase 3: Observability (Optional)

**Goal**: Make rate limit status visible in `gt status`.

**Deliverables**:
- `gt status` shows rate limit state
- Feed events for rate limit detection/recovery

**Effort**: Small
**Risk**: Low

---

## Recommended Approach

### Summary

| Dimension | Recommendation | Rationale |
|-----------|---------------|-----------|
| Detection | Tmux output parsing | Parse rate limit message for reset time |
| Architecture | Daemon + persistent state | Self-contained, survives restarts |
| Wake-up | Simple cascade with work filter | Reliable, efficient |
| Context | **None needed** | Session context intact, just poke to continue |
| Config | TownSettings.RateLimits | Fits existing pattern |

### Key Design Decisions

1. **Daemon handles wake-up, not external scheduler**: Keeps system self-contained.

2. **Persist state to file**: Survives daemon restarts without external deps.

3. **Cascade poke with grace periods**: Prevents immediate re-rate-limit.

4. **No context recovery needed**: Sessions are paused, not killed. Just poke to continue.

5. **Simple poke mechanism**: Send "1" + "Your rate limit has reset, continue your work".

### Open Questions (Resolved)

1. **Rate limit message format**: See the original GitHub issue for the exact
   message format that Claude Code outputs when rate-limited.

2. **API key in daemon**: **No.** Daemon should not probe Anthropic API directly.
   Simply track which agents are rate-limited and poke them to continue when the
   reset time arrives.

3. **Per-model tracking**: **Not needed for v1.** Global tracking is sufficient.

4. **Dashboard integration**: **Not a priority.** Skip for initial implementation.

5. **Proactive warnings**: Out of scope for v1.

---

## Appendix A: Existing Infrastructure Touchpoints

### Files to Modify

| File | Change |
|------|--------|
| `internal/config/types.go` | Add `RateLimitConfig` |
| `internal/config/loader.go` | Load rate limit config |
| `internal/daemon/daemon.go` | Add rate limit check to heartbeat |
| `internal/daemon/rate_limit.go` | New file for rate limit handling |
| `internal/krc/krc.go` | Add rate-limit event TTL |
| `internal/cmd/daemon.go` | Add `rate-limit-status` subcommand |

### New Files

| File | Purpose |
|------|---------|
| `internal/daemon/rate_limit.go` | Rate limit detection, state, wake-up |
| `daemon/rate-limit-state.json` | Persisted rate limit state |

### Existing Patterns to Reuse

- `keepalive.State` pattern for rate limit state
- `ProcessLifecycleRequests()` pattern for staged restart
- `restartSession()` for agent wake-up
- Checkpoint system for context preservation
- Mail system for handoff

---

## Appendix B: Rate Limit Message Examples

**Note**: These are hypothetical. Need to capture actual Claude Code output.

```
# Hypothetical rate limit message format
Error: Rate limit exceeded. Resets at 2026-01-28T16:00:00Z

# Alternative format
You've reached your usage limit. Try again after 4:00 PM PST.

# Another possible format
Claude Pro limit reached. Next reset in 47 minutes.
```

**TODO**: Capture real rate limit messages for accurate regex patterns.

---

## Appendix C: Testing Strategy

### Unit Tests

1. Rate limit detection parsing (various message formats)
2. State persistence and recovery
3. Timer scheduling logic
4. Cascade restart ordering

### Integration Tests

1. Daemon restart during rate limit wait
2. Multiple rate limit events
3. Rate limit during recovery
4. Full wake-up cycle with mock sessions

### Manual Testing

1. Induce real rate limit (use Claude heavily)
2. Verify detection and state persistence
3. Wait for reset, verify automatic wake-up
4. Check context preservation in restarted sessions
