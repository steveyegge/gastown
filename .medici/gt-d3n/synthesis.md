# Medici Synthesis: Emergency Stop (E-stop) for Gas Town

## Executive Summary

Gas Town needs a town-wide emergency stop that works when the infrastructure it
protects is the thing that's broken. The design must handle two regimes (daemon
alive / daemon dead) and two triggers (human manual / automatic detection). The
recommended approach is a file-based sentinel with daemon-driven auto-triggering
and a three-state recovery probe.

## What Each Lens Added

| Lens | Key Contribution |
|------|-----------------|
| **Domain** | De-energize-to-safe principle: the default should be stopped, not running. Lease/watchdog pattern. |
| **User** | The UX of the crisis IS the crisis. Visual receipt (red status bars) matters as much as the mechanism. SIGTSTP freeze preserves state. |
| **Constraints** | Simplest reliable signal: one file, `touch ESTOP`. Works with Dolt down, works with daemon down. Existing park/dock pattern already teaches agents to check files. |
| **Incentives** | E-stop has a cost (destroyed context). Threshold must account for restart cost, not just burn rate. Brief hiccups shouldn't halt everything. |
| **Adversary** | Split-brain window during propagation. Frozen-with-no-resume risk. Agent-side self-suspension as defense-in-depth. |
| **Outsider** | Half-open state is critical for recovery. Three-state circuit breaker (CLOSED/OPEN/HALF_OPEN). Recovery probe before resuming full traffic. |

## Tensions That Matter

1. **Simplicity vs fail-safe** (Constraints vs Domain): Sentinel file is simplest
   but requires positive action. Lease is fail-safe but adds complexity and false
   positives. Resolution: sentinel file with daemon auto-creation gives both.

2. **Fast stop vs graceful stop** (User vs Incentives): SIGTSTP freeze is instant
   but may catch agents mid-write. Checkpoint-then-park is safer but slower.
   Resolution: two-tier — freeze first (stop the bleeding), then let agents
   checkpoint on resume.

3. **Automatic vs manual trigger** (Domain vs Adversary): Auto-trigger prevents
   waste when no human is watching. But false positives on transient hiccups can
   cause more harm than the failure. Resolution: require both a failure condition
   AND a minimum duration before auto-trigger.

## Candidate Approaches

### Option A: Simple Sentinel (Minimum Viable)

A single `ESTOP` file at town root. `gt estop` creates it; `gt resume` removes it.
Daemon checks on each heartbeat loop, stops all sessions. Agents check on each
poll cycle. No automatic trigger — purely manual.

- **Why it could work**: Dead simple. Works with everything broken. Can be typed
  from memory during a panic. Ships in a day.
- **Risks**: No protection when human is AFK. No recovery probe — human must
  verify infrastructure before resuming. No visual feedback beyond CLI output.
- **Who it serves best**: Solo operator who is always watching.

### Option B: Sentinel + Auto-Trigger + Recovery Probe (Recommended)

Everything in Option A, plus:
- Daemon auto-creates ESTOP when it detects sustained infrastructure failure
  (configurable: N failures over M seconds, default 5 failures over 90 seconds)
- ESTOP file contains metadata: `manual` vs `auto:<reason>:<timestamp>`
- For auto-triggered stops, daemon runs a recovery probe every 60s (lightweight
  health check). On success, removes ESTOP and resumes.
- Manual ESTOP always requires manual `gt resume`
- Visual indicators: tmux status bar turns red with ESTOP timestamp
- `gt status` shows E-stop state prominently

- **Why it could work**: Covers both human-present and human-absent cases.
  Recovery probe prevents indefinite freeze. Minimum duration threshold avoids
  false positives on transient hiccups. Metadata distinguishes manual from auto.
- **Risks**: More moving parts. Auto-trigger threshold needs tuning. Recovery
  probe could flap if infrastructure is unstable.
- **Who it serves best**: Multi-agent setup that runs unattended for hours.

### Option C: Full Circuit Breaker (Three-State)

Everything in Option B, plus:
- Agent-side circuit breaker wrapping all Dolt operations
- Three states: CLOSED (normal), OPEN (all Dolt ops blocked), HALF_OPEN (probe)
- Agents individually track consecutive failures and can self-open their circuit
- Buffered escalations to flat file during OPEN state
- Coordinated probe with lock file to prevent race conditions

- **Why it could work**: Most resilient. Agents self-protect even without daemon.
  Granular — individual agents can circuit-break without town-wide stop.
- **Risks**: Significant complexity. Race conditions on state file. Lock file
  management. Agents must be modified to wrap every Dolt call. Overkill for a
  single-machine system.
- **Who it serves best**: A mature system with many agents and complex failure
  modes that need per-agent granularity.

## Recommended Approach

**Option B: Sentinel + Auto-Trigger + Recovery Probe.**

This is the sweet spot between simplicity and protection. The key insights that
shaped this recommendation:

- **Constraints lens** showed that a file is the only signal that works in all
  failure regimes. No new infrastructure needed.
- **Domain lens** showed that automated fail-safe is essential — manual-only
  doesn't protect unattended operation.
- **Adversary lens** showed that auto-trigger needs a duration floor to avoid
  false positives, and that recovery must be designed upfront.
- **Outsider lens** showed that the recovery probe (half-open concept) is critical
  — otherwise you need a human for every resume.
- **User lens** showed that visual feedback (red tmux bars) is as important as
  the mechanism — the overseer needs to see the stop happen.
- **Incentives lens** showed that a minimum duration threshold is essential to
  avoid halting on transient hiccups.

Option C (full circuit breaker) is the right long-term evolution but is premature
for the current system maturity. Option A is too bare — it doesn't protect
unattended operation, which is the primary use case.

## Cheap Experiments (Before Building)

1. **Manual drill** (15 min): Touch a test file in town root during agent
   activity. Observe if anything notices. Maps existing poll boundaries.
2. **Post-mortem cost audit** (1 hr): Reconstruct the 2.1GB incident's actual
   token waste. Validates whether auto-trigger would have saved meaningful cost.
3. **Dolt failure frequency** (30 min): Query historical daemon logs for Dolt
   connection failures. Calibrate auto-trigger threshold against real data.

## Open Questions

1. Should crew workers (persistent, expensive context) be treated differently
   from polecats (transient, cheap context) during E-stop? Crew might deserve
   a "freeze" (SIGTSTP) while polecats get killed.
2. What's the right auto-trigger threshold? Need real failure data to calibrate.
3. Should the Mayor be exempt from E-stop (so it can coordinate recovery)?
4. How does E-stop interact with the merge queue / refinery? In-flight merges
   may be the highest-risk thing to interrupt.
