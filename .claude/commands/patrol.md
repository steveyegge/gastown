---
description: Run a patrol cycle for the current agent role (witness, deacon, or refinery)
allowed-tools: Bash(gt patrol:*), Bash(gt hook:*), Bash(gt mail:*), Bash(gt nudge:*), Bash(gt peek:*), Bash(gt escalate:*), Bash(gt dolt status:*), Bash(bd :*), Bash(gt mol:*)
argument-hint: [witness|deacon|refinery]
---

# Patrol

Run one patrol cycle for the specified role. Patrol is a continuous monitoring
loop — each invocation executes one cycle of the patrol formula.

Arguments: $ARGUMENTS
If no role specified, detect from current GT_ROLE environment variable.

## Role Detection

```bash
echo $GT_ROLE
```

Map to patrol type:
- `*/witness` → witness patrol (`mol-witness-patrol`)
- `*/deacon` or `*/deacon/*` → deacon patrol (`mol-deacon-patrol`)
- `*/refinery` → refinery patrol (`mol-refinery-patrol`)
- Explicit argument overrides detection

## Patrol Entry Point

```bash
gt patrol new --role <role>
```

This creates a hooked wisp with steps from the patrol formula.
If a patrol is already running (wisp exists on hook), resume it instead.

## Witness Patrol Steps

The witness is the per-rig polecat supervisor. Execute in order:

### 1. inbox-check
```bash
gt mail inbox
```
Process any pending messages: POLECAT_DONE, MERGED, HELP, escalations.
Read each with `gt mail read <id>` and take appropriate action.

### 2. process-cleanups
Check for cleanup wisps (dirty state from dead polecats):
```bash
bd list --status=open --label=cleanup --json
```
Process each: verify git state, clean worktrees, close cleanup wisps.

### 3. check-refinery
```bash
gt peek gastown/refinery
```
Verify refinery is alive and processing the merge queue.
If stuck, nudge: `gt nudge gastown/refinery "Health check — are you processing?"`

### 4. survey-workers
Check all active polecats in the rig:
```bash
gt peek gastown/polecats
```
For each active polecat:
- Check if session is alive (has recent activity)
- Check if work is progressing (commits, bead updates)
- Detect zombies: session dead but agent_state says working
- Detect stale spawns: spawning > 10 minutes

Nudge idle polecats:
```bash
gt nudge gastown/polecats/<name> "Progress check — what's your status?"
```

### 5. check-timer-gates
```bash
bd list --label=gate:timer --status=open --json
```
Evaluate any timer-based gates that may have elapsed.

### 6. check-swarm
Check for convoy/swarm completion across coordinated work:
```bash
bd list --label=convoy --status=open --json
```

### 7. patrol-cleanup
Close completed patrol wisps, update metrics.

### 8. context-check
Check remaining context budget. If approaching limit:
```bash
gt handoff -s "Patrol cycling" -m "Patrol cycle N complete, cycling for fresh context"
```

### 9. loop-or-exit
Report cycle results and spawn next cycle:
```bash
gt patrol report --summary "<cycle summary>" --steps "inbox:OK,cleanup:OK,..."
```

## Deacon Patrol Steps

The deacon is the town-wide daemon monitor. Key steps:

1. **inbox-check** — Process callbacks from witnesses, refineries, polecats
2. **trigger-pending-spawns** — Launch queued polecat spawns
3. **gate-evaluation** — Check async gates (timer, dependency)
4. **dispatch-gated-molecules** — Release molecules whose gates cleared
5. **check-convoy-completion** — Track multi-rig coordinated work
6. **health-scan** — Check Dolt health (`gt dolt status`), agent health
7. **zombie-scan** — Find dead sessions, orphaned wisps
8. **plugin-run** — Execute enabled plugins (backup, reaper, etc.)
9. **dog-pool-maintenance** — Manage utility worker pool
10. **orphan-check** — Find orphaned test databases (`gt dolt cleanup`)
11. **session-gc** — Clean up dead session artifacts
12. **patrol-cleanup** — Close completed wisps, update metrics
13. **context-check** — Check context budget, handoff if needed
14. **loop-or-exit** — Report and spawn next cycle

## Refinery Patrol Steps

The refinery processes the merge queue sequentially:

1. **inbox-check** — Look for MERGE_READY messages from witness
2. **queue-scan** — List pending MRs in merge queue
3. **process-branch** — Fetch and rebase next MR branch onto target
4. **run-tests** — Execute configured gate suite on rebased branch
5. **handle-failures** — On test failure: bisect, isolate culprit, notify
6. **merge-push** — Fast-forward push to main: `git push origin temp:main`
7. **notify** — Send MERGED mail to witness immediately after push
8. **cleanup** — Close MR bead, delete remote branch, archive mail
9. **context-check** — Check context budget
10. **loop-or-exit** — Report and spawn next cycle

## Cycle Completion

Each cycle ends with:
```bash
gt patrol report --summary "<what happened>" --steps "<step1:OK,step2:SKIP,...>"
```

This closes the current patrol wisp and spawns the next cycle automatically.
