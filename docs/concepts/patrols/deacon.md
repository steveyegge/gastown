# Deacon Patrol

> The town-wide orchestrator. Spawns workers, monitors health, manages gates,
> dispatches dogs, and keeps every rig running.

## The Patrol Loop

The Deacon patrol is a formula-driven loop. Each iteration handles mail,
maintains infrastructure, dispatches work, checks health, and runs housekeeping.
The Deacon is the longest patrol formula (~1000 lines, 20+ steps) because it
covers the widest scope: the entire town.

```mermaid
graph TD
    A["inbox-check: Callbacks, escalations, DOG_DONE messages"] --> infra

    subgraph infra["infrastructure"]
        I1["orphan-cleanup: Kill orphaned Claude processes"] --> I2["trigger-pending: Nudge spawned polecats"]
    end

    infra --> disp

    subgraph disp["dispatch"]
        D1["gate-evaluation: Close expired timer gates"] --> D2["dispatch-gated molecules: Sling gate-ready molecules"]
        D2 --> D3["check-convoys: Auto-close done convoys"]
        D3 --> D4["feed-stranded convoys: Dispatch dogs to convoys with no workers"]
    end

    disp --> mon

    subgraph mon["monitoring"]
        M1["health-scan: Check Witness/Refinery for each rig"] --> M2["zombie-scan: Detect zombie polecats (DRY RUN - file warrant)"]
    end

    mon --> house

    subgraph house["housekeeping"]
        H1["dog-pool: Maintain idle dogs, retire stale ones"] --> H2["orphan-check: Find in_progress issues with dead agents"]
        H2 --> H3["wisp-compact: TTL-based wisp compaction"]
        H3 --> H4["log-maintenance: Rotate, archive, prune logs"]
    end

    house --> PC["patrol-cleanup: Inbox hygiene"]
    PC --> CC["context-check: RSS, session age"]
    CC --> LE{"loop-or-exit"}
    LE -->|"continue: squash wisp, create new wisp"| A
    LE -->|"hand off: squash wisp, gt handoff"| EXIT["Exit"]
```

## Step Groups

The Deacon's steps fall into four groups, executed in order each cycle.

### Infrastructure

**orphan-cleanup** — Kill orphaned Claude subagent processes (those with
`TTY="?"` that have lost their parent session). Prevents memory leaks from
crashed sessions.

**trigger-pending** — Find spawned polecats that are ready for their initial
input and send them a nudge trigger. This is the bootstrap mechanism for
newly-created polecat sessions.

### Dispatch

**gate-evaluation** — Check all open gates. Close timer gates when their
timeout has elapsed. Notify waiters when gates close. See
[Gates](#gates-async-coordination) below.

**dispatch-gated-molecules** — Find molecules that were blocked on gates which
are now closed. Dispatch each one via `gt sling` to the appropriate rig.

**check-convoys** — Run `gt convoy check` to auto-close convoys where all
tracked issues are resolved. Fire completion notifications.

**feed-stranded-convoys** — Find convoys that have ready issues but no workers
assigned. Dispatch dogs to pick up the stranded work.

### Monitoring

**health-scan** — Check Witness and Refinery health for each rig. Skips
docked/parked rigs. When no active work exists, only verifies sessions exist
(no nudge spam). When active work exists, sends `HEALTH_CHECK` nudges.
Tracks consecutive failures; escalates to Mayor after 5+ failed cycles.

**zombie-scan** — Detect zombie polecats across all rigs. The Deacon does NOT
kill zombies directly — it files death warrants that Boot handles. This
separation prevents the orchestrator from accidentally killing live sessions.

### Housekeeping

**dog-pool** — Ensure at least one idle dog is available. Spawn on demand if
the pool is empty. Optionally retire dogs that have been idle for >24 hours.
Maximum 4 dogs total. See [Dog Pool](#the-dog-pool) below.

**orphan-check** — Find `in_progress` issues whose assigned agents are dead.
Dispatch a dog for recovery rather than fixing inline.

**wisp-compact** — Run TTL-based compaction on expired wisps, freeing space
in the beads database.

**log-maintenance** — Rotate logs >10MB, archive logs >7 days, prune stale
`state.json` files.

## Gates: Async Coordination

Gates are async coordination primitives that let molecules pause and resume
without tying up an agent. A molecule step can create a gate, and the Deacon
evaluates gates each cycle.

| Gate type | Created by | Closed when |
|-----------|-----------|-------------|
| `timer` | Molecule step | Time elapsed (`CreatedAt + Timeout < Now`) |
| `gh:run` | Polecat/workflow | GitHub Actions run completes |
| `gh:pr` | Polecat/workflow | Pull request merged or closed |
| `human` | Any step | Human runs `bd gate approve <gate-id>` |
| `mail` | Any step | Expected message arrives |

The evaluation flow:

```mermaid
graph TD
    A["Deacon cycle"] -->|"gate-evaluation"| B["List open gates (bd gate list --json)"]
    B --> C{"Gate type?"}
    C -->|"timer gate expired"| D["Close, notify waiters"]
    C -->|"gh:run completed"| E["Close, notify waiters"]
    C -->|"gh:pr resolved"| F["Close, notify waiters"]
    C -->|"human gate"| G["Skip (requires external action)"]
    C -->|"mail gate"| H["Skip (subscription handles wake)"]
    D --> I
    E --> I
    F --> I
    I["dispatch-gated-molecules: Find molecules with closed gates"] --> J["Dispatch each via gt sling"]
```

Gates enable async resume without explicit waiter tracking. The Deacon
discovers reality each cycle — if a gate is closed and a molecule is ready,
it dispatches. No persistent subscription state needed.

## The Dog Pool

Dogs are reusable infrastructure workers managed by the Deacon. Unlike polecats
(which are spawned for one task and nuked), dogs persist between tasks and
return to idle when their work is done.

| Property | Polecats | Dogs |
|----------|----------|------|
| **Lifecycle** | Spawn → work → nuke | Idle → work → idle |
| **Scope** | Single rig | Cross-rig (multi-worktree) |
| **Manager** | Witness | Deacon |
| **Max count** | Configurable per rig | 4 total |
| **Purpose** | Feature work | Infrastructure/cleanup |

### Dog states

```mermaid
graph LR
    A["idle"] --> B["working"]
    B --> A
    A -->|">24h idle"| C["retired"]
```

### Dispatch flow

```mermaid
graph TD
    A["Deacon identifies infrastructure task"] -->|"gt dog dispatch --plugin name"| B["Dog marked as working"]
    B --> C["Mail delivered with work instructions"]
    C --> D["Dog executes (tmux session, Claude agent)"]
    D -->|"sends DOG_DONE mail"| E["Deacon receives, parses metrics, archives"]
    E --> F["Dog returns to idle"]
```

Dogs have worktrees into every configured rig, so they can work across rig
boundaries without the overhead of creating temporary clones. This makes them
ideal for cross-rig cleanup, dependency resolution, and infrastructure tasks.

## The Watchdog Chain

The Deacon sits in the middle of a multi-layer monitoring hierarchy:

```mermaid
graph TD
    A["Daemon (Go process, 3-min heartbeat)"] --> B["Boot (ephemeral, fresh each tick)"]
    B --> B1["Triages Deacon: start / wake / nudge"]
    A --> C["Deacon (persistent, runs patrol cycles)"]
    C --> C1["Monitors Witnesses and Refineries"]
    C --> C2["Manages Dogs for infrastructure"]
    C --> C3["Dispatches gated molecules"]
    A --> D["Witnesses (one per rig, persistent)"]
    D --> D1["Monitors Polecats in their rig"]
    A --> E["Refineries (one per rig, persistent)"]
    E --> E1["Processes merge queue for their rig"]
```

Each layer only monitors the layer below it. The Daemon ensures the Deacon is
running. The Deacon ensures Witnesses and Refineries are running. Witnesses
ensure polecats are running.

### Second-order monitoring

Who watches the Deacon? Witnesses do — passively. They check the Deacon's
`last_activity` timestamp on its agent bead each cycle. If the Deacon is
stale for >5 minutes, the Witness escalates to the Mayor. No heartbeat mail
spam required.

### Boot: the ephemeral triage agent

Boot is spawned fresh every daemon heartbeat. Its sole job: look at the Deacon
and decide whether to start, nudge, or kill it. Boot is intentionally
short-lived — it makes one decision and exits. This prevents Boot itself from
becoming a reliability concern.

## Health Check Protocol

The Deacon monitors Witness and Refinery health using a nudge-and-wait pattern:

```mermaid
graph TD
    A["Deacon sends HEALTH_CHECK nudge to agent"] --> B["Wait for agent bead update (30s timeout)"]
    B -->|"Activity detected"| C["Healthy, reset failure counter"]
    B -->|"No response"| D["Increment failure counter"]
    D -->|"Failures < 3"| E["Try again next cycle"]
    D -->|"Failures >= 3"| F["Recommend force-kill"]
    F --> G["1. Log intervention (mail)"]
    G --> H["2. Kill tmux session"]
    H --> I["3. Update agent bead"]
    I --> J["4. Cooldown (5 min)"]
    J --> K["5. Daemon restarts on next tick"]
```

### Idle town optimization

When no active work exists across any rig, the Deacon skips health check
nudges and only verifies that sessions exist. This prevents unnecessary noise
in quiet towns. The Deacon stays silent when the town is healthy and idle.

## Resilience

### Auto-respawn

The Deacon's tmux session has a pane-died hook that automatically restarts
Claude if it exits. This handles crashes without waiting for the next daemon
heartbeat (3 minutes). The daemon is a safety net, not the primary recovery
mechanism.

### Grace periods

| Scenario | Grace period |
|----------|-------------|
| Startup | 5 min before heartbeat check |
| Health check response | 30s wait for bead update |
| Force-kill cooldown | 5 min between kills of same agent |
| Stale hook max age | 1 hour before cleanup |

### Nudge and kill thresholds

| Deacon age | Action |
|------------|--------|
| Fresh (< threshold) | Nothing |
| 5-10 min stale | Nudge to wake |
| >10 min stale | Restart |

## The Idle Town Principle

The Deacon follows a key design principle: silence when healthy.

- No `HEALTH_CHECK` nudges when no active work exists
- Exponential backoff between cycles (60s → 120s → 240s → 5m max)
- Feed subscription wakes immediately on real events
- Daemon heartbeat is safety net only, not the driver

This means a quiet town consumes minimal resources. The Deacon checks in every
5 minutes at most, and only runs full monitoring when work is actually happening.

## Architecture

```mermaid
graph TD
    A["Daemon (heartbeat every 3 min)"] --> B["ensureDeaconRunning()"]
    B --> C["deacon.Manager"]
    C --> D["tmux session: hq-deacon"]
    D --> E["Claude Code + mol-deacon-patrol formula"]
    E --> F["infrastructure (orphan cleanup, pending triggers)"]
    E --> G["dispatch (gates, convoys, stranded work)"]
    E --> H["monitoring (health scan, zombie scan)"]
    E --> I["housekeeping (dogs, compaction, logs)"]
```

The Deacon is a town-level singleton — one per Gas Town installation. Unlike
Witnesses and Refineries (which are per-rig), the Deacon sees across all rigs.
Its session name is `hq-deacon`, reflecting its town-wide scope.

See [Patrols Overview](./overview.md) for the universal patrol lifecycle,
[Witness Patrol](./witness.md) for per-rig polecat monitoring, and
[Refinery Patrol](./refinery.md) for merge queue processing.
