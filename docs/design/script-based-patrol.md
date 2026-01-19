# Script-Based Patrol Design

> Design document for replacing prose-based patrol molecules with shell scripts.
> Written 2026-01-15, crew session.

## Problem Statement

Current patrol agents (Deacon, Witness) execute multi-step molecules where each step is prose instructions that Claude interprets and executes. This has issues:

- Claude must "become expert" on the molecule each cycle
- Prose interpretation is slow and context-heavy
- No mechanical execution path
- AI wakes even when nothing needs AI judgment

## Core Idea

**Invert the model:**

| Current | Script-Based |
|---------|--------------|
| AI runs patrol loop | Daemon runs scripts |
| AI interprets prose steps | Daemon executes shell scripts |
| AI always awake during patrol | AI wakes only when needed |
| Molecule defines structure | Scripts define actions, gates control execution |

**The separation:**

- **Daemon** (Go): Runs scripts mechanically, evaluates gates, records results
- **Deacon/Witness** (Claude): Monitors execution, builds/updates scripts, handles edge cases

## Architecture

```
Daemon Loop                          AI Agent (Deacon/Witness)
────────────                         ────────────────────────
1. Load script collection            (sleeping)
2. For each script:
   a. Evaluate gate
   b. If open: run script
   c. Record result (wisp)
   d. If needs_ai: set flag
3. If needs_ai flag set:  ─────────► Wake AI
4. Sleep (backoff)                   5. Review results
                                     6. Handle edge cases
                                     7. Update scripts if needed
                                     8. Clear flag, go back to sleep
```

## Script Format

Each script lives in a directory with metadata:

```
scripts/deacon/
├── 01-inbox-check/
│   ├── script.sh          # The actual shell script
│   ├── gate.toml          # Gate configuration
│   └── README.md          # Human documentation (optional)
├── 02-pending-spawns/
│   ├── script.sh
│   └── gate.toml
└── ...
```

### gate.toml

```toml
[gate]
type = "condition"                    # condition | cooldown | cron | none
check = "gt mail inbox --count"       # For condition type
expect = "nonzero"                    # nonzero | zero | exit0 | exit1

# OR for cooldown:
# type = "cooldown"
# duration = "5m"

# OR for cron:
# type = "cron"
# schedule = "0 */6 * * *"            # Every 6 hours

# OR always run:
# type = "none"

[execution]
timeout = "60s"                       # Max runtime
escalate_on_failure = true            # Set needs_ai flag on non-zero exit
```

### script.sh

Pure shell script. Exit codes:

| Exit Code | Meaning |
|-----------|---------|
| 0 | Success, no action needed |
| 1 | Success, action taken |
| 2 | Needs AI judgment (sets flag) |
| 3+ | Error (escalates based on gate.toml) |

Scripts can write to stdout/stderr - daemon captures for wisp body.

## Gate Types

### condition

Run a check command before executing script.

```toml
[gate]
type = "condition"
check = "gt mail inbox --count"
expect = "nonzero"    # Run script if check output is non-zero
```

Expect values:
- `nonzero`: Output is not "0" or empty
- `zero`: Output is "0" or empty
- `exit0`: Check command exits 0
- `exit1`: Check command exits non-zero

### cooldown

Don't run if executed recently. Queries wisps.

```toml
[gate]
type = "cooldown"
duration = "5m"
```

Daemon checks: `bd list --label=script:inbox-check --since=5m --limit=1`
- If found → gate closed (ran recently)
- If empty → gate open

### cron

Run on schedule.

```toml
[gate]
type = "cron"
schedule = "0 9 * * *"    # 9am daily
```

Daemon compares current time to schedule.

### none

Always run (no gate).

```toml
[gate]
type = "none"
```

## Deacon Scripts

```
scripts/deacon/
├── 01-inbox-check/
│   ├── script.sh         # gt mail inbox, archive processed
│   └── gate.toml         # type = "condition", check = "gt mail inbox --count"
│
├── 02-pending-spawns/
│   ├── script.sh         # gt deacon pending, nudge ready ones
│   └── gate.toml         # type = "condition", check = "gt deacon pending --count"
│
├── 03-health-scan/
│   ├── script.sh         # gt witness status, gt refinery status per rig
│   └── gate.toml         # type = "cooldown", duration = "5m"
│
├── 04-zombie-scan/
│   ├── script.sh         # gt deacon zombie-scan --dry-run
│   └── gate.toml         # type = "cooldown", duration = "10m"
│
├── 05-dog-pool/
│   ├── script.sh         # gt dog status, spawn if needed
│   └── gate.toml         # type = "condition", check = "gt dog status --idle-count | grep '^0$'"
│
├── 06-dog-health/
│   ├── script.sh         # Check stuck dogs
│   └── gate.toml         # type = "condition", check = "gt dog list --working --count"
│
├── 07-orphan-check/
│   ├── script.sh         # Quick orphan detection
│   └── gate.toml         # type = "cooldown", duration = "10m"
│
├── 08-session-gc/
│   ├── script.sh         # gt doctor -v, dispatch cleanup if needed
│   └── gate.toml         # type = "cooldown", duration = "30m"
│
└── 99-wake-deacon/
    ├── script.sh         # Check if AI judgment needed
    └── gate.toml         # type = "condition", check = "test -f /tmp/gt-needs-ai"
```

### Deacon Wake Conditions

Scripts exit with code 2 or write to `/tmp/gt-needs-ai` when:

- Zombie detected (zombie-scan)
- Orphan detected (orphan-check)
- Agent unresponsive for 3+ cycles (health-scan)
- Dog stuck (dog-health)
- Unhandled escalation in inbox (inbox-check)
- Any script fails unexpectedly

## Witness Scripts

```
scripts/witness/
├── 01-inbox-check/
│   ├── script.sh         # Handle POLECAT_DONE, MERGED, HELP, etc.
│   └── gate.toml         # type = "condition", check = "gt mail inbox --count"
│
├── 02-cleanup-wisps/
│   ├── script.sh         # Process dirty polecat cleanups
│   └── gate.toml         # type = "condition", check = "bd list --wisp --labels=cleanup --status=open --limit=1"
│
├── 03-survey-workers/
│   ├── script.sh         # Check polecat states, auto-nuke idle+clean
│   └── gate.toml         # type = "cooldown", duration = "2m"
│
├── 04-timer-gates/
│   ├── script.sh         # bd gate check --type=timer --escalate
│   └── gate.toml         # type = "cooldown", duration = "5m"
│
├── 05-ping-deacon/
│   ├── script.sh         # gt mail send deacon/ -s "WITNESS_PING"
│   └── gate.toml         # type = "cooldown", duration = "5m"
│
└── 99-wake-witness/
    ├── script.sh         # Check if AI judgment needed
    └── gate.toml         # type = "condition", check = "test -f /tmp/gt-needs-ai-witness-<rig>"
```

### Witness Wake Conditions

- Dirty polecat needs manual intervention
- Polecat stuck (not responding to nudges)
- Timer gate expired
- HELP message received
- Refinery down and MRs waiting

## State Tracking

**No shadow state files.** Everything is wisps on the ledger.

Each script run creates a wisp:

```bash
bd wisp create \
  --label type:script-run \
  --label script:inbox-check \
  --label agent:deacon \
  --label result:success \
  --body "Processed 3 messages, archived 3"
```

Gate evaluation queries wisps:

```bash
# Cooldown check
bd list --label=script:inbox-check --label=agent:deacon --since=5m --limit=1
```

Daily digest squashes script-run wisps.

## Daemon Implementation

```go
// Pseudocode for daemon script loop

func runScriptLoop(scriptsDir string) {
    scripts := discoverScripts(scriptsDir)

    for {
        for _, script := range scripts {
            if gateOpen(script.Gate) {
                result := runScript(script.Path, script.Timeout)
                recordWisp(script.Name, result)

                if result.NeedsAI {
                    setNeedsAIFlag(script.Agent)
                }
            }
        }

        if needsAIFlagSet() {
            wakeAgent()
            waitForAgentClear()
        }

        sleep(backoff())
    }
}
```

## AI Agent Role (New Model)

When woken, the Deacon/Witness:

1. **Reviews script results** - reads recent wisps
2. **Handles edge cases** - the situations scripts couldn't handle mechanically
3. **Updates scripts** - if a script needs improvement, edit it
4. **Clears flag** - signals daemon to continue
5. **Goes back to sleep**

The AI is an **overseer**, not a worker. It monitors the mechanical scripts and intervenes only when judgment is required.

## Migration Path

### Phase 1: Parallel Run
- Keep existing molecule patrols
- Add script daemon alongside
- Compare results

### Phase 2: Script Primary
- Scripts handle routine work
- AI wakes less frequently
- Molecules become fallback

### Phase 3: Scripts Only
- Remove molecule patrols
- AI is pure oversight
- Scripts are source of truth

## Benefits

| Aspect | Current | Script-Based |
|--------|---------|--------------|
| AI wake frequency | Every patrol cycle | Only when needed |
| Context usage | High (prose interpretation) | Low (just review results) |
| Execution speed | Slow (Claude thinks) | Fast (shell execution) |
| Debugging | Read Claude's reasoning | Read script output |
| Modification | Edit molecule prose | Edit shell script |
| Testing | Run full patrol | Run individual script |

## Open Questions

1. **Script discovery**: Scan directory or explicit registry?
2. **Per-rig scripts**: `scripts/witness/<rig>/` or parameterized scripts?
3. **Script versioning**: Track script changes? Git is natural fit.
4. **Recovery**: If script keeps failing, how does AI get notified?
5. **Parallelism**: Run independent scripts concurrently?

## References

- mol-deacon-patrol.formula.toml - Current Deacon structure
- mol-witness-patrol.formula.toml - Current Witness structure
- docs/design/plugin-system.md - Gate concepts originated here
