# /crank - Autonomous Epic Execution

> **Fully autonomous epic execution via ODMCR loop. Runs until ALL children are CLOSED.**

For full details, see: `~/.claude/skills/sk-crank/`

---

## /crank vs /autopilot

| Aspect | /autopilot | /crank |
|--------|------------|--------|
| Human gates | Pauses for validation | NO pauses |
| Validation | Runs, waits for approval | Runs, auto-proceeds |
| Failure handling | Stops, asks human | Retries, escalates, continues |
| Context | Single-agent (Task()) | Multi-polecat orchestration |
| Context cost | Returns all output (~80K for 8 agents) | ~100 tokens per convoy poll |
| Use case | Supervised execution | Overnight/AFK execution |

**Rule of thumb**: Use `/autopilot` when you're watching, `/crank` when you're AFK.

---

## ODMCR Reconciliation Loop

Kubernetes-inspired reconciliation: declare the goal (epic complete), let the loop drive toward it.

```
┌──────────────────────────────────────────────────┐
│                  ODMCR LOOP                       │
│                                                   │
│  OBSERVE ──► DISPATCH ──► MONITOR                 │
│     ▲                        │                    │
│     │        RETRY ◄─────────┤                    │
│     │          │             ▼                    │
│     └──────────┴──────── COLLECT                  │
│                                                   │
│  EXIT: All children status=closed                 │
└──────────────────────────────────────────────────┘
```

### Observe
Query current epic state:
```bash
bd ready --parent=<epic>                    # Ready issues (unblocked)
bd list --parent=<epic> --status=in_progress  # In-flight
bd list --parent=<epic> --status=closed       # Completed
```

### Dispatch
Send work to polecats:
```bash
gt wave <epic>              # Dispatch all ready issues (preferred)
gt sling <issue> <rig>      # Individual dispatch
```
Respects `MAX_POLECATS` limit (default: 4).

### Monitor
Poll convoy status (low-token operation):
```bash
gt convoy status <convoy-id>   # ~100 tokens output
```
Poll interval: 30 seconds.

### Collect
Verify completions:
```bash
bd show <issue> | grep "status: closed"
git -C ~/gt/<rig>/polecats/<polecat> log -1
```

### Retry
Handle failures with exponential backoff:

| Attempt | Backoff | Action |
|---------|---------|--------|
| 1 | 30s | Re-sling to fresh polecat |
| 2 | 60s | Re-sling with hint context |
| 3 | 120s | Re-sling with explicit hints |
| 4+ | -- | Escalate: BLOCKER + mail |

---

## Failure Taxonomy

| Failure Type | Detection | Auto-Recovery | Max Retries |
|--------------|-----------|---------------|-------------|
| **Polecat Stuck** | No status change 5+ polls | Nudge, then nuke | 3 |
| **Validation Fail** | Issue not closed, test failures | Add hints, re-sling | 3 |
| **Dependency Deadlock** | No ready issues, circular deps | Remove weak dep | 1 (then escalate) |
| **Context Limit** | Token limit message | Checkpoint, fresh polecat | 2 |
| **Git Conflict** | Merge failures | Auto-resolve beads, nudge code | 2 |
| **External Service** | Timeouts, 429/500 errors | Backoff retry | 5 |
| **Polecat Crash** | tmux session gone | Clean re-sling | 2 |

**Escalation**: After max retries, mark BLOCKER, mail human, continue epic.

---

## Completion Conditions

Epic is complete when:
```bash
total=$(bd list --parent=<epic> | wc -l)
closed=$(bd list --parent=<epic> --status=closed | wc -l)
[ "$total" -eq "$closed" ]  # All children closed
```

On completion:
1. Close epic: `bd close <epic> --reason "All children complete"`
2. Run retro: `/retro <epic-id>`
3. Sync and push: `bd sync && git push`
4. Mail notice: `gt mail send mayor/ -s "CRANK DONE: <epic>"`

---

## Command Reference

| Command | Purpose |
|---------|---------|
| `/crank <epic-id>` | Execute epic to completion |
| `/crank <epic-id> --dry-run` | Preview waves without executing |
| `/crank <epic-id> --max N` | Limit concurrent polecats (default: 4) |
| `/crank status` | Show active crank sessions |
| `/crank stop` | Graceful stop at next checkpoint |

### Examples

```bash
/crank gt-0100                    # Full autonomous execution
/crank gt-0100 --dry-run          # Preview wave assignments
/crank gt-0100 --max 2            # Limit to 2 concurrent polecats
/crank status                     # Check progress
/crank stop                       # Stop after current wave
```

---

## Context Efficiency

| Operation | Token Cost |
|-----------|------------|
| Convoy poll | ~100 tokens |
| ODMCR iteration (30s) | ~750 tokens |
| Per hour | ~90K tokens |
| 8-hour run | ~720K tokens |

Compare to `/autopilot` with 8 Task() agents: ~80K tokens consumed immediately.

---

## State Tracking

All state in beads comments (no external files):

| Comment | Meaning |
|---------|---------|
| `CRANK_START: max=N` | Session started |
| `CRANK_STATE: wave=N, remaining=M` | Checkpoint |
| `CRANK_CHECKPOINT: ...` | Context-fill checkpoint |
| `CRANK_STOP: ...` | Graceful stop requested |
| `CRANK_COMPLETE: ...` | All children closed |

**Resume after crash**: Just run `/crank <epic>` again - state reconstructs from beads.

---

## Related

- `/gastown` - Status checks and utility operations
- `/autopilot` - Supervised execution with validation gates
- `~/.claude/skills/sk-crank/odmcr.md` - Full ODMCR specification
- `~/.claude/skills/sk-crank/failure-taxonomy.md` - Detailed failure handling
