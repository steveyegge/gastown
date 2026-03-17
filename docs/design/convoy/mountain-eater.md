# The Mountain-Eater: Autonomous Epic Grinding

> Judgment layer for convoy-driven epic execution.
>
> **Status**: Design
> **Depends on**: Convoy Milestones 0-2 (ConvoyManager, stage-launch)
> **Related**: [roadmap.md](roadmap.md) | [spec.md](spec.md) | [swarm-architecture.md](../../../docs/swarm-architecture.md)

---

## 1. Problem Statement

Gas Town has all the pieces for autonomous epic execution:
- ConvoyManager feeds ready issues as blocking deps close (event-driven, 5s)
- Stranded scan catches missed feeding (periodic, 30s)
- Stage-launch validates DAGs and computes waves (Kahn's algorithm)
- Polecats execute individual issues
- Witnesses monitor polecats, Refineries merge

Yet users report that large epics "get stuck." They create a mountain of
beads, launch a convoy, go away for a few hours, and come back to find the
convoy stalled at 40% with no indication of why.

**Root cause**: The ConvoyManager is mechanical. It feeds the next ready
issue when one closes, but it cannot reason about failure patterns, make
skip decisions, or escalate intelligently. When a polecat fails repeatedly
on the same issue, the mechanical system re-slings it endlessly. When a
subtle blocking condition exists outside the dep graph, nothing notices.

The Mountain-Eater adds a judgment layer — agent-driven stall detection,
skip-after-N-failures, intelligent escalation, and completion notification
— on top of the existing mechanical feeding.

---

## 2. Design Principle: No Agent Holds the Thread

The reason single-coordinator approaches fail is **hysteresis**. Any agent
maintaining an "I'm driving this epic" loop will lose that thread at
compaction. Even with the epic hooked, the re-primed agent doesn't remember
the coordination context.

The Mountain-Eater sidesteps this entirely:

- **The epic IS the thread.** The beads ARE the state.
- **No agent needs to remember anything.** Each check discovers state fresh.
- **Dogs bring fresh context every time.** Zero hysteresis by construction.
- **The label triggers patrol behavior.** No persistent coordinator needed.

This aligns with core Gas Town principles:
- **ZFC**: Agents decide, Go transports. ConvoyManager is transport; Dogs make judgment calls.
- **NDI**: Any Dog can check any mountain. Different agents, same outcome.
- **Discover, Don't Track**: `bd ready --epic=X` and convoy status derive state.
- **Float over Integer**: A stuck issue doesn't halt the mountain — work flows around it.

---

## 3. Architecture: Four-Layer Grinding

```
Layer 0: CONVOY MANAGER (mechanical, Go daemon — already built)
    Event-driven feeding + stranded scan
    Handles the happy path: issue closes → feed next ready

Layer 1: WITNESS (reactive, per-rig — enhancement)
    Polecat failure tracking for mountain convoy issues
    Same issue failed 3+ times → mark blocked, skip, feed next

Layer 2: DEACON DOG (periodic, cross-rig — new)
    "Has this mountain progressed since last check?"
    Fresh Dog investigates stalls with full context
    Makes judgment calls: skip, restructure, escalate
    Notifies Mayor on stalls and completion

Layer 3: MAYOR (strategic, user-facing — enhancement)
    Receives stall escalations from Layer 2
    Cross-rig judgment calls
    Notifies user on completion or unrecoverable stalls
```

**Layer 0** already exists and handles ~80% of convoy execution.
**Layers 1-2** are the Mountain-Eater — they handle the 20% that gets stuck.
**Layer 3** is the escalation path for the ~2% that requires human judgment.

### Why Four Layers?

Redundant monitoring is resilience. If the Witness misses a completion
(crash, compaction), the ConvoyManager catches it (5s event poll). If the
ConvoyManager feeds a bad issue repeatedly, the Witness catches the failure
pattern. If both miss a stall, the Deacon Dog catches it on the next patrol
cycle. Each layer operates independently and discovers state from beads.

---

## 4. The `mountain` Label

A mountain is a convoy with the `mountain` label. No new entity types,
no new database schema. The label IS the opt-in for Layers 1-2.

```bash
# Activate the Mountain-Eater on an epic
gt mountain <epic-id>

# Internally:
#   1. gt convoy stage <epic-id>          ← validate DAG, compute waves
#   2. bd update <convoy> --add-label mountain  ← trigger judgment layers
#   3. gt convoy launch <convoy-id>       ← dispatch wave 1, ConvoyManager takes over

# Check progress
gt mountain status [epic-id|convoy-id]

# Pause/resume (keeps label, stops/starts dispatch)
gt mountain pause <epic-id|convoy-id>
gt mountain resume <epic-id|convoy-id>

# Cancel (removes label, leaves convoy for manual management)
gt mountain cancel <epic-id|convoy-id>
```

Regular convoys (no `mountain` label) continue working exactly as today.
The `mountain` label opts a convoy into enhanced stall detection,
skip-after-N-failures, and active progress monitoring.

### When to Use Mountains vs Regular Convoys

| Scenario | Use |
|----------|-----|
| Batch sling of 3-5 tasks | Regular convoy (ConvoyManager is sufficient) |
| Large epic with 10+ tasks and DAG deps | Mountain |
| Cross-rig epic | Mountain (needs the Dog's cross-rig visibility) |
| "Go to lunch and come back to it done" | Mountain |
| Quick parallel tasks, no deps | Regular convoy |

---

## 5. Layer 1: Witness Failure Tracking

### Problem

When a polecat fails on a mountain issue, the ConvoyManager's stranded
scan re-slings it. If the issue has a fundamental problem (bad description,
impossible task, missing context), this creates an infinite sling-fail loop.

### Enhancement

The Witness already monitors polecat completions. Add failure tracking
for issues belonging to mountain convoys:

```
WITNESS PATROL — mountain failure tracking:

For each polecat that exited without completing its issue:
  issue = polecat's hooked bead
  convoy = tracking convoy for this issue (if any)
  if convoy has "mountain" label:
    increment failure count for this issue (stored as issue note or label)
    if failure_count >= 3:
      bd update <issue> --status=blocked --add-label mountain:skipped
      bd update <issue> --notes "Skipped by Mountain-Eater after 3 polecat failures"
      log: "Mountain: skipped <issue> after 3 failures"
      # ConvoyManager's next feed will skip this issue (blocked status)
      # and feed the next ready issue instead
```

**Failure count storage**: Use a label like `mountain:failures:3` on the
issue. Labels are cheap, queryable, and visible in `bd show`. No new
schema needed.

**Why the Witness and not the ConvoyManager?** The Witness already observes
polecat lifecycle. It knows whether a polecat completed successfully or
crashed. The ConvoyManager only sees issue status changes — it can't
distinguish "polecat failed" from "polecat is still working."

### Skip Semantics

A skipped issue (`mountain:skipped` label, `blocked` status) is:
- Excluded from the ready front (blocked status)
- Visible in `gt mountain status` output
- Escalated to Mayor by Layer 2 (Deacon Dog)
- Recoverable: `bd update <issue> --status=open --remove-label mountain:skipped`

The mountain continues grinding around the skipped issue. If the skipped
issue was blocking other work in the DAG, those dependents remain blocked.
The Dog reports this in its stall diagnosis.

---

## 6. Layer 2: Deacon Dog Mountain Audit

### The Core Loop

The Deacon's patrol formula gains a `mountain-audit` step:

```
DEACON PATROL — mountain-audit step:

mountains = bd list --label mountain --status=open --type=convoy
for each mountain:
  dog_needed = false

  # Progress check (compare against last audit)
  current_closed = count of closed issues in this convoy
  last_closed = read from mountain:audit:<convoy-id> label on deacon bead

  if current_closed > last_closed:
    # Making progress — update audit mark, continue
    update mountain:audit:<convoy-id> = current_closed

  else if current_closed == total_issues:
    # Complete — dispatch Dog for cleanup + notification
    dog_needed = true
    dog_task = "complete"

  else:
    # No progress since last check — dispatch Dog to investigate
    dog_needed = true
    dog_task = "stall"

  if dog_needed:
    sling mountain-dog formula to a Dog with convoy-id and task type
```

### The Mountain Dog Formula

`mol-mountain-dog.formula.toml` — a short-lived Dog formula for
investigating mountain progress:

```toml
[formula]
name = "mountain-dog"
description = "Investigate mountain convoy progress"
type = "worker"

[formula.variables]
convoy_id = { required = true }
task = { required = true }  # "stall" or "complete"

[[formula.steps]]
name = "investigate"
description = """
You are a Mountain Dog investigating a mountain convoy.

Convoy: {{convoy_id}}
Task: {{task}}

If task is "stall":
  1. Run: gt convoy status {{convoy_id}}
  2. Identify why no progress:
     - Are there skipped issues (mountain:skipped label)?
     - Are all remaining issues blocked? By what?
     - Are polecats active but slow?
     - Is the refinery backed up?
  3. If there are ready issues with no polecats: sling them
  4. If all remaining issues are skipped/blocked:
     Mail Mayor: "Mountain {{convoy_id}} stalled: N skipped, M blocked.
     Remaining DAG cannot progress without intervention."
  5. If polecats are active: this is fine, no action needed

If task is "complete":
  1. Run: gt convoy status {{convoy_id}}
  2. Verify all tracked issues are closed
  3. If any skipped issues remain:
     Mail Mayor: "Mountain {{convoy_id}} finished with N skipped issues.
     Review skipped work: [list issue IDs]"
  4. If all clean:
     Mail Mayor: "Mountain {{convoy_id}} complete. N issues closed in Xh Ym."
  5. Run: gt convoy close {{convoy_id}}
"""
```

### Dog Properties That Make This Work

- **Fresh context**: The Dog starts with zero state. It reads the convoy
  and beads from scratch. No hysteresis from prior sessions.
- **Narrow scope**: One convoy, one question ("stalled?" or "complete?").
  Fits easily in a single context window.
- **Ephemeral**: Does its job, reports, dies. No long-running coordination.
- **Cross-rig visibility**: Dogs have worktrees into multiple rigs. They can
  check beads status across rigs for cross-rig convoys.

### Audit Frequency

The Deacon patrol cycle determines how often mountains are audited. Current
Deacon patrol runs on a feed-driven + heartbeat model. For mountains, the
relevant question is: "How long can a mountain be stalled before someone
notices?"

- **Target**: Stall detected within 10-15 minutes
- **Mechanism**: Deacon's heartbeat interval (daemon pokes Deacon every
  5-10 minutes depending on activity). Each heartbeat runs the patrol
  formula including the mountain-audit step.
- **Cost**: One `bd list --label mountain` query per patrol cycle (cheap),
  plus one Dog spawn per stalled mountain (only when needed).

---

## 7. Layer 3: Mayor Notification

The Mayor receives two types of mountain mail from Dogs:

### Stall Notification

```
Subject: Mountain stalled: <convoy-title>
Body:
  Convoy: hq-cv-abc "Rebuild auth system"
  Progress: 23/35 closed (65%)
  Stalled for: 15 minutes

  Skipped issues (polecat failure):
    gt-xyz "Migrate session store" (failed 3 times)
    gt-abc "Update JWT validation" (failed 3 times)

  Blocked issues (DAG):
    gt-def "Integration tests" (blocked by gt-xyz)
    gt-ghi "E2E tests" (blocked by gt-def)

  Active polecats: 0
  Ready issues: 0

  Action needed: Review skipped issues. Possible fixes:
    bd update gt-xyz --status=open --remove-label mountain:skipped  (retry)
    bd close gt-xyz --reason="Descoped"  (skip permanently, unblocks dependents)
```

### Completion Notification

```
Subject: Mountain complete: <convoy-title>
Body:
  Convoy: hq-cv-abc "Rebuild auth system"
  Result: 33/35 closed, 2 skipped
  Elapsed: 3h 42m

  Skipped issues:
    gt-xyz "Migrate session store" (failed 3 times — needs manual review)
    gt-abc "Update JWT validation" (failed 3 times — needs manual review)
```

### Mayor's Role

The Mayor is NOT part of the grinding loop. It receives notifications and
can take action, but the mountain grinds autonomously without Mayor
involvement. The Mayor's actions are:

- **Retry a skipped issue**: `bd update <id> --status=open --remove-label mountain:skipped`
- **Permanently skip**: `bd close <id> --reason="Descoped"` (unblocks dependents)
- **Notify user**: Forward the stall/completion notification
- **Restructure DAG**: Remove or add dependencies to work around a blocker

---

## 8. User Experience

### Starting a Mountain

```bash
$ gt mountain gt-epic-auth-rebuild

Validating epic structure...
  Epic: gt-epic-auth-rebuild "Rebuild auth system"
  Tasks: 35 (31 slingable, 4 epics)
  Waves: 6 (computed from blocking deps)
  Max parallelism: 4

  Warnings:
    gt-migrate-sessions has no description (may cause polecat confusion)

  Errors: none

Creating convoy...
  Convoy: hq-cv-m7x "Mountain: Rebuild auth system"
  Label: mountain

Launching Wave 1 (4 tasks)...
  Slung gt-foundation-types → gastown
  Slung gt-config-schema → gastown
  Slung gt-test-fixtures → gastown
  Slung gt-error-types → gastown

Mountain active. ConvoyManager will feed subsequent waves.
Deacon will audit progress every ~10 minutes.
Check status: gt mountain status hq-cv-m7x
```

### Checking Status

```bash
$ gt mountain status

Active Mountains:
  hq-cv-m7x "Rebuild auth system"
    Progress: ████████████░░░░░░░░ 23/35 (65%)
    Active: 3 polecats working
    Ready: 1 issue waiting for polecat
    Blocked: 6 issues (DAG deps)
    Skipped: 2 issues (polecat failures)
    Elapsed: 1h 47m

  hq-cv-n9y "Migrate database layer"
    Progress: ██████████████████░░ 18/20 (90%)
    Active: 2 polecats working
    Elapsed: 52m
```

### Detailed Status

```bash
$ gt mountain status hq-cv-m7x

Mountain: hq-cv-m7x "Rebuild auth system"
Epic: gt-epic-auth-rebuild

Progress: 23/35 closed (65%)
Elapsed: 1h 47m
Wave: 4 of 6

Completed (23):
  ✓ gt-foundation-types, gt-config-schema, gt-test-fixtures, ...

Active (3):
  ⟳ gt-session-handler (polecat: gastown/nux, 12m)
  ⟳ gt-middleware-chain (polecat: gastown/furiosa, 8m)
  ⟳ gt-rate-limiter (polecat: gastown/max, 3m)

Ready (1):
  ○ gt-cache-layer (unblocked, waiting for polecat)

Skipped (2):
  ⊘ gt-migrate-sessions (failed 3 times — no description)
  ⊘ gt-jwt-validation (failed 3 times — test dependency missing)

Blocked (6):
  ◌ gt-auth-integration (needs: gt-session-handler, gt-jwt-validation⊘)
  ◌ gt-e2e-auth-tests (needs: gt-auth-integration)
  ...

Stall risk: gt-jwt-validation⊘ blocks 4 downstream issues.
  Fix: bd update gt-jwt-validation --status=open --remove-label mountain:skipped
  Or:  bd close gt-jwt-validation --reason="Descoped"
```

---

## 9. Global Improvements (All Convoys)

The Mountain-Eater design reveals improvements that benefit ALL convoys,
not just mountains. These should be applied globally:

### 9.1 Polecat Failure Tracking

Even non-mountain convoys benefit from knowing "this issue has failed 3
times." The Witness should track failure counts for all convoy-tracked
issues, not just mountain ones. The difference: mountains auto-skip after
3 failures; regular convoys just log a warning.

### 9.2 Stall Detection in Stranded Scan

The ConvoyManager's stranded scan currently feeds the first ready issue.
Add: if the same issue has been slung 3+ times and keeps appearing as
stranded, stop re-slinging it and log a warning. This prevents the
infinite sling-fail loop for all convoys.

### 9.3 Progress Visibility

`gt convoy status` should show the same rich information as
`gt mountain status` — active polecats, ready front, blocked issues,
skipped issues. This is useful for all convoys, not just mountains.

---

## 10. Relationship to Swarm Architecture

The [swarm architecture doc](../../../docs/swarm-architecture.md) describes
a design where swarms are persistent molecules coordinated by a dedicated
agent. The Mountain-Eater achieves the same outcome through a different
mechanism:

| Swarm Architecture | Mountain-Eater |
|--------------------|----------------|
| Dedicated coordinator agent | No coordinator — patrol steps + Dogs |
| Swarm molecule tracks state | Label triggers patrol behavior |
| Coordinator survives via molecule | Dogs bring fresh context (no survival needed) |
| Ready Front computed by coordinator | Ready Front computed by ConvoyManager + Dogs |
| Recovery via molecule resume | Recovery via beads state discovery |

The Mountain-Eater is the implementation path for the swarm architecture's
goals. The swarm doc's "ready front" model, "gate issues," and "batch
management" concepts apply directly. The difference is mechanism:
patrol-driven grinding instead of coordinator-driven grinding.

The swarm architecture doc should be updated to reference the Mountain-Eater
as the concrete implementation.

---

## 11. Implementation Plan

See [roadmap.md](roadmap.md) Milestone 5 for the phased implementation.

### Summary of Changes

| Component | Change | Scope |
|-----------|--------|-------|
| `gt mountain` CLI | New command (stage + label + launch) | ~200 lines |
| `gt mountain status` | New command (query + format) | ~300 lines |
| `gt mountain pause/resume/cancel` | Label management | ~100 lines |
| Witness patrol formula | Failure tracking for convoy issues | Formula step |
| Deacon patrol formula | Mountain audit step | Formula step |
| `mol-mountain-dog.formula.toml` | Dog formula for stall investigation | New formula |
| ConvoyManager stranded scan | Skip after N failures (global) | ~30 lines |
| `gt convoy status` | Enhanced output (active, ready, blocked) | ~100 lines |

### What Does NOT Change

- Convoy data model (still `hq-cv-*` beads with `tracks` deps)
- ConvoyManager event poll (still 5s, still feeds on close)
- ConvoyManager stranded scan (still 30s, enhanced with skip logic)
- Stage-launch workflow (mountain uses it directly)
- Polecat lifecycle (unchanged)
- Refinery (unchanged)

---

## 12. Open Questions

1. **Should `gt mountain` auto-undock a docked rig?** If the epic's issues
   route to a docked rig, should the mountain automatically undock it?
   Current thinking: no — require the rig to be active. Mountains only
   grind active rigs.

2. **Max concurrent polecats per mountain.** Should mountains have a
   configurable concurrency limit? The ConvoyManager feeds one issue per
   close event. For mountains, we might want to dispatch multiple ready
   issues when a wave transition happens (e.g., wave 1 completes, wave 2
   has 8 ready issues — dispatch all 8, not one-at-a-time).

3. **Mountain-to-mountain dependencies.** Can one mountain depend on
   another? Probably not needed initially — cross-mountain deps are just
   cross-issue deps in the DAG.

4. **Notification channel.** Mayor mail is the current notification path.
   Should mountains also support webhook/Slack notification for the user?
   Defer to future work.
