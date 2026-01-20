# Gas Town Mail Protocol

> Reference for inter-agent mail communication in Gas Town

## Overview

Gas Town agents coordinate via mail messages routed through the beads system.
Mail uses `type=message` beads with routing handled by `gt mail`.

## Message Types

### POLECAT_DONE

**Route**: Polecat → Witness

**Purpose**: Signal work completion, trigger cleanup flow.

**Subject format**: `POLECAT_DONE <polecat-name>`

**Body format**:
```
Exit: MERGED|ESCALATED|DEFERRED
Issue: <issue-id>
MR: <mr-id>          # if exit=MERGED
Branch: <branch>
```

**Trigger**: `gt done` command generates this automatically.

**Handler**: Witness creates a cleanup wisp for the polecat.

### MERGE_READY

**Route**: Witness → Refinery

**Purpose**: Signal a branch is ready for merge queue processing.

**Subject format**: `MERGE_READY <polecat-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Polecat: <polecat-name>
Verified: clean git state, issue closed
```

**Trigger**: Witness sends after verifying polecat work is complete.

**Handler**: Refinery adds to merge queue, processes when ready.

### MERGED

**Route**: Refinery → Witness

**Purpose**: Confirm branch was merged successfully, safe to nuke polecat.

**Subject format**: `MERGED <polecat-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Polecat: <polecat-name>
Rig: <rig>
Target: <target-branch>
Merged-At: <timestamp>
Merge-Commit: <sha>
```

**Trigger**: Refinery sends after successful merge to main.

**Handler**: Witness completes cleanup wisp, nukes polecat worktree.

### MERGE_FAILED

**Route**: Refinery → Witness

**Purpose**: Notify that merge attempt failed (tests, build, or other non-conflict error).

**Subject format**: `MERGE_FAILED <polecat-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Polecat: <polecat-name>
Rig: <rig>
Target: <target-branch>
Failed-At: <timestamp>
Failure-Type: <tests|build|push|other>
Error: <error-message>
```

**Trigger**: Refinery sends when merge fails for non-conflict reasons.

**Handler**: Witness notifies polecat, assigns work back for rework.

### REWORK_REQUEST

**Route**: Refinery → Witness

**Purpose**: Request polecat to rebase branch due to merge conflicts.

**Subject format**: `REWORK_REQUEST <polecat-name>`

**Body format**:
```
Branch: <branch>
Issue: <issue-id>
Polecat: <polecat-name>
Rig: <rig>
Target: <target-branch>
Requested-At: <timestamp>
Conflict-Files: <file1>, <file2>, ...

Please rebase your changes onto <target-branch>:

  git fetch origin
  git rebase origin/<target-branch>
  # Resolve any conflicts
  git push -f

The Refinery will retry the merge after rebase is complete.
```

**Trigger**: Refinery sends when merge has conflicts with target branch.

**Handler**: Witness notifies polecat with rebase instructions.

### WITNESS_PING

**Route**: Witness → Deacon (all witnesses send)

**Purpose**: Second-order monitoring - ensure Deacon is alive.

**Subject format**: `WITNESS_PING <rig>`

**Body format**:
```
Rig: <rig>
Timestamp: <timestamp>
Patrol: <cycle-number>
```

**Trigger**: Each witness sends periodically (every N patrol cycles).

**Handler**: Deacon acknowledges. If no ack, witnesses escalate to Mayor.

### HELP

**Route**: Any → escalation target (usually Mayor)

**Purpose**: Request intervention for stuck/blocked work.

**Subject format**: `HELP: <brief-description>`

**Body format**:
```
Agent: <agent-id>
Issue: <issue-id>       # if applicable
Problem: <description>
Tried: <what was attempted>
```

**Trigger**: Agent unable to proceed, needs external help.

**Handler**: Escalation target assesses and intervenes.

### HANDOFF

**Route**: Agent → self (or successor)

**Purpose**: Session continuity across context limits/restarts.

**Subject format**: `🤝 HANDOFF: <brief-context>`

**Body format**:
```
attached_molecule: <molecule-id>   # if work in progress
attached_at: <timestamp>

## Context
<freeform notes for successor>

## Status
<where things stand>

## Next
<what successor should do>
```

**Trigger**: `gt handoff` command, or manual send before session end.

**Handler**: Next session reads handoff, continues from context.

### WORK_DONE

**Route**: Polecat → Crew (dispatcher)

**Purpose**: Notify crew that spawned polecat has completed work.

**Subject format**: `WORK_DONE: <issue-id>`

**Body format**:
```
Exit: COMPLETED|FAILED|ESCALATED
Issue: <issue-id>
Molecule: <molecule-id>
Polecat: <polecat-name>
Formula: <formula-name>
Duration: <seconds>
MR: <mr-id>              # if exit=COMPLETED
Error: <error-message>   # if exit=FAILED
```

**Trigger**: `gt done` sends to `dispatched_by` address in bead metadata.

**Handler**: Crew receives notification, can dispatch more work or review.

### EXECUTION_REPORT

**Route**: Polecat → Crew's feedback inbox (via `gt done`)

**Purpose**: Structured feedback on formula execution for crew improvement.

**Subject format**: `EXEC_REPORT: <formula>/<molecule-id>`

**Body format**:
```
Formula: <formula-name>
Version: <formula-version>
Molecule: <molecule-id>
Polecat: <polecat-name>
Crew: <crew-address>
Outcome: success|failure|partial
Steps-Completed: <n>
Steps-Total: <n>
Duration: <seconds>
Timestamp: <iso-timestamp>

## Step Timings
step-1: <seconds>
step-2: <seconds>
...

## Errors (if any)
- Step <n>: <error-message>

## Suggestions (optional)
- <improvement suggestion>
```

**Trigger**: Generated by `gt done`, written to crew's feedback directory.

**Handler**: Crew reviews during feedback phase, iterates formula if needed.

### POLECAT_FAILURE

**Route**: Witness → Crew (formula owner)

**Purpose**: Alert crew to critical polecat failure requiring attention.

**Subject format**: `POLECAT_FAILURE: <polecat-name>`

**Body format**:
```
Polecat: <polecat-name>
Issue: <issue-id>
Molecule: <molecule-id>
Formula: <formula-name>
Crew: <crew-address>
Failure-Type: stuck|crash|timeout|error
Detected-At: <timestamp>
Last-Activity: <timestamp>
Error: <error-message>

Action taken: recycled|waiting|escalated
```

**Trigger**: Witness detects polecat in failed state.

**Handler**: Crew investigates if formula-related, or acknowledges if transient.

### FORMULA_ALERT

**Route**: System → Crew (formula owner)

**Purpose**: Alert crew when formula metrics cross thresholds.

**Subject format**: `FORMULA_ALERT: <formula>/<alert-type>`

**Body format**:
```
Formula: <formula-name>
Alert-Type: low-success-rate|high-duration|frequent-failures
Threshold: <threshold-value>
Current: <current-value>
Period: <time-period>
Sample-Size: <n-executions>

## Recent Failures
- <issue-id>: <error>
- <issue-id>: <error>

Recommendation: Review formula for potential improvements
```

**Trigger**: Monitoring system detects metric threshold exceeded.

**Handler**: Crew reviews formula, identifies root cause, iterates.

## Format Conventions

### Subject Line

- **Type prefix**: Uppercase, identifies message type
- **Colon separator**: After type for structured info
- **Brief context**: Human-readable summary

Examples:
```
POLECAT_DONE nux
MERGE_READY greenplace/nux
HELP: Polecat stuck on test failures
🤝 HANDOFF: Schema work in progress
```

### Body Structure

- **Key-value pairs**: For structured data (one per line)
- **Blank line**: Separates structured data from freeform content
- **Markdown sections**: For freeform content (##, lists, code blocks)

### Addresses

Format: `<rig>/<role>` or `<rig>/<type>/<name>`

Examples:
```
greenplace/witness       # Witness for greenplace rig
beads/refinery           # Refinery for beads rig
greenplace/polecats/nux  # Specific polecat
mayor/                # Town-level Mayor
deacon/               # Town-level Deacon
```

## Protocol Flows

### Polecat Completion Flow

```
Polecat                    Witness                    Refinery
   │                          │                          │
   │ POLECAT_DONE             │                          │
   │─────────────────────────>│                          │
   │                          │                          │
   │                    (verify clean)                   │
   │                          │                          │
   │                          │ MERGE_READY              │
   │                          │─────────────────────────>│
   │                          │                          │
   │                          │                    (merge attempt)
   │                          │                          │
   │                          │ MERGED (success)         │
   │                          │<─────────────────────────│
   │                          │                          │
   │                    (nuke polecat)                   │
   │                          │                          │
```

### Merge Failure Flow

```
                           Witness                    Refinery
                              │                          │
                              │                    (merge fails)
                              │                          │
                              │ MERGE_FAILED             │
   ┌──────────────────────────│<─────────────────────────│
   │                          │                          │
   │ (failure notification)   │                          │
   │<─────────────────────────│                          │
   │                          │                          │
Polecat (rework needed)
```

### Rebase Required Flow

```
                           Witness                    Refinery
                              │                          │
                              │                    (conflict detected)
                              │                          │
                              │ REWORK_REQUEST           │
   ┌──────────────────────────│<─────────────────────────│
   │                          │                          │
   │ (rebase instructions)    │                          │
   │<─────────────────────────│                          │
   │                          │                          │
Polecat                       │                          │
   │                          │                          │
   │ (rebases, gt done)       │                          │
   │─────────────────────────>│ MERGE_READY              │
   │                          │─────────────────────────>│
   │                          │                    (retry merge)
```

### Second-Order Monitoring

```
Witness-1 ──┐
            │ WITNESS_PING
Witness-2 ──┼────────────────> Deacon
            │
Witness-N ──┘
                                 │
                          (if no response)
                                 │
            <────────────────────┘
            Escalate to Mayor
```

### Crew-Polecat Feedback Flow

```
Crew                     Polecat                    Witness
  │                          │                         │
  │ gt crew dispatch         │                         │
  │─────────────────────────>│                         │
  │                    (spawned with mol)              │
  │                          │                         │
  │                    (executes formula)              │
  │                          │                         │
  │                          │ gt done                 │
  │                          │────────────────────────>│
  │                          │                         │
  │ WORK_DONE                │                         │
  │<─────────────────────────│                         │
  │                          │                         │
  │ (execution report        │                         │
  │  written to feedback/)   │                         │
  │                          │                         │
  │ (reviews feedback,       │                         │
  │  iterates formula)       │                         │
```

### Polecat Failure Alert Flow

```
Crew                         Witness                 Polecat
  │                             │                       │
  │                             │              (polecat stuck)
  │                             │                       │
  │                             │ (detect stuck)        │
  │                             │<──────────────────────│
  │                             │                       │
  │ POLECAT_FAILURE             │                       │
  │<────────────────────────────│                       │
  │                             │                       │
  │ (investigate formula,       │ (recycle polecat)     │
  │  or acknowledge transient)  │──────────────────────>│
  │                             │                       │
```

## Implementation

### Sending Mail

```bash
# Basic send
gt mail send <addr> -s "Subject" -m "Body"

# With structured body
gt mail send greenplace/witness -s "MERGE_READY nux" -m "Branch: feature-xyz
Issue: gp-abc
Polecat: nux
Verified: clean"
```

### Receiving Mail

```bash
# Check inbox
gt mail inbox

# Read specific message
gt mail read <msg-id>

# Mark as read
gt mail ack <msg-id>
```

### In Patrol Formulas

Formulas should:
1. Check inbox at start of each cycle
2. Parse subject prefix to route handling
3. Extract structured data from body
4. Take appropriate action
5. Mark mail as read after processing

## Extensibility

New message types follow the pattern:
1. Define subject prefix (TYPE: or TYPE_SUBTYPE)
2. Document body format (key-value pairs + freeform)
3. Specify route (sender → receiver)
4. Implement handlers in relevant patrol formulas

The protocol is intentionally simple - structured enough for parsing,
flexible enough for human debugging.

## Related Documents

- `docs/agent-as-bead.md` - Agent identity and slots
- `.beads/formulas/mol-witness-patrol.formula.toml` - Witness handling
- `internal/mail/` - Mail routing implementation
- `internal/protocol/` - Protocol handlers for Witness-Refinery communication
