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

### RECOVERED_BEAD

**Route**: Witness → Deacon

**Purpose**: Notify Deacon that a dead polecat's abandoned work has been recovered
and needs re-dispatch.

**Subject format**: `RECOVERED_BEAD <bead-id>`

**Body format**:
```
Recovered abandoned bead from dead polecat.

Bead: <bead-id>
Polecat: <rig>/<polecat-name>
Previous Status: <hooked|in_progress>

The bead has been reset to open with no assignee.
Please re-dispatch to an available polecat.
```

**Trigger**: Witness detects a zombie polecat with work still hooked/in_progress.
The bead is reset to open status and this mail is sent for re-dispatch.

**Handler**: Deacon runs `gt deacon redispatch <bead-id>` which:
- Rate-limits re-dispatches (5-minute cooldown per bead)
- Tracks failure count (after 3 failures, escalates to Mayor)
- Auto-detects target rig from bead prefix
- Slings the bead to an available polecat via `gt sling`

### RECOVERY_NEEDED

**Route**: Witness → Deacon

**Purpose**: Escalate a dirty polecat that has unpushed/uncommitted work needing
manual recovery before cleanup.

**Subject format**: `RECOVERY_NEEDED <rig>/<polecat-name>`

**Body format**:
```
Polecat: <rig>/<polecat-name>
Cleanup Status: <has_uncommitted|has_stash|has_unpushed>
Branch: <branch>
Issue: <issue-id>
Detected: <timestamp>
```

**Trigger**: Witness detects zombie polecat with dirty git state.

**Handler**: Deacon coordinates recovery (push branch, save work) before
authorizing cleanup. Only escalates to Mayor if Deacon cannot resolve.

### WITNESS_PING (deprecated)

**Status**: Removed. Witnesses no longer send WITNESS_PING mail.

**Previous behavior**: Witnesses sent heartbeat mail to the Deacon every patrol
cycle, which spammed inboxes with routine noise.

**Current behavior**: Witnesses passively check the Deacon's agent bead
`last_activity` timestamp. If stale (>5 minutes), they escalate directly to
the Mayor with an `ALERT: Deacon appears unresponsive` message. No routine
heartbeat mail is sent — only escalations when a problem is detected.

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

### Abandoned Work Recovery Flow

```
Dead Polecat               Witness                    Deacon
     │                        │                          │
     │ (session dies)         │                          │
     │                        │                          │
     │                  (detects zombie)                 │
     │                  (bead status=hooked)             │
     │                        │                          │
     │                  resetAbandonedBead()             │
     │                  bd update --status=open          │
     │                        │                          │
     │                        │ RECOVERED_BEAD           │
     │                        │─────────────────────────>│
     │                        │                          │
     │                        │                    gt deacon redispatch
     │                        │                    gt sling <bead> <rig>
     │                        │                          │
     │                        │                          ├──> New Polecat
     │                        │                          │    (re-dispatched)
```

### Second-Order Monitoring

```
Witness-1 ──┐
            │ (check agent bead last_activity)
Witness-2 ──┼────────────────> Deacon agent bead
            │
Witness-N ──┘
                                 │
                          (if stale >5min)
                                 │
            ─────────────────────┘
            ALERT to Mayor (mail only on failure)
```

## Communication Hygiene: Mail vs Nudge

Agents overuse mail for routine communication, generating permanent beads and
Dolt commits for messages that should be ephemeral. Every `gt mail send` creates
a wisp bead in Dolt -- a permanent record with its own commit in the git-like
history. This is a critical pollution source.

### The Two Channels

**`gt nudge` (ephemeral, preferred for routine comms)**
- Sends a message directly to an agent's tmux session
- No beads created. No Dolt commits. Zero storage cost.
- Message appears as a `<system-reminder>` in the agent's context
- Suitable for: health checks, status requests, simple instructions, "wake up" signals
- Limitation: if the target session is dead, the nudge is lost

**`gt mail send` (persistent, for structured protocol messages only)**
- Creates a bead (wisp) in the Dolt database
- Generates at least one Dolt commit (the write)
- Persists across session restarts -- survives agent death
- Suitable for: HANDOFF context, MERGE_READY/MERGED protocol, escalations, HELP
  requests, anything that MUST survive session death

### The Rule

**Default to `gt nudge`. Only use `gt mail send` when the message MUST survive
the recipient's session death.**

The litmus test: "If the recipient's session dies and restarts, do they need this
message?" If yes -> mail. If no -> nudge.

### Role-Specific Guidance

| Role | Mail Budget | When to Mail | When to Nudge |
|------|-------------|-------------|---------------|
| **Polecat** | 0-1 per session | HELP/ESCALATE only (gt escalate preferred) | Everything else |
| **Witness** | Protocol msgs only | MERGE_READY, RECOVERED_BEAD, RECOVERY_NEEDED, escalations to Mayor | Polecat health checks, status pings, nudge-and-observe |
| **Refinery** | Protocol msgs only | MERGED, MERGE_FAILED, REWORK_REQUEST | Status updates to Witness |
| **Deacon** | Escalations only | Escalations to Mayor, HANDOFF to self | TIMER callbacks, HEALTH_CHECK, lifecycle pokes |
| **Dogs** | Zero | Never (results go to event beads or logs) | Report completion to Deacon via nudge |
| **Mayor** | Strategic only | Cross-rig coordination, HANDOFF to self | Instructions to Deacon/Witness |

### Why This Matters (The Commit Graph)

Dolt is git under the hood. Every mail creates a Dolt commit. Over a day of
normal operations:
- 4 agents x 15 patrol cycles x 2 mails per cycle = 120 commits just for routine chatter
- These commits live in the git history forever, even after mail rows are deleted
- Rebase can remove them, but prevention is always cheaper than cleanup

### Anti-Patterns

**DOG_DONE as mail** -- Dogs should not mail their completion status. Use
`gt nudge deacon/ "DOG_DONE: plugin-name success"` instead.

**Duplicate escalations** -- Witnesses sending 2+ mails about the same issue
minutes apart. Check inbox before sending: if you already sent about this topic,
don't send again.

**HANDOFF for routine cycles** -- Patrol agents (Witness, Deacon) doing routine
handoffs should use minimal mail. If there's nothing extraordinary, just cycle --
the next session discovers state from beads, not from mail.

**Health check responses via mail** -- When Deacon sends a health check nudge, do
NOT respond with mail. The Deacon tracks health via session status, not mail
responses.

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
