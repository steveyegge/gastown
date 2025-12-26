# Polecat Wisp Architecture: Proto → Wisp → Mol

> Issue: gt-9g82

## Executive Summary

This document proposes a three-layer architecture for agent work: **Proto → Wisp → Mol**.
The key insight is that agents should run session wisps that wrap their assigned work.
This creates a unified "engineer in a box" pattern that handles onboarding, execution,
and cleanup within a single ephemeral container.

---

## 1. The Three-Layer Model

```
┌─────────────────────────────────────────────────────────────────────┐
│                          PROTO LAYER                                 │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  polecat.md / crew.md (Template/Instructions)                  │  │
│  │  - Role identity and context                                   │  │
│  │  - How to be this type of agent                                │  │
│  │  - Workflow patterns                                           │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  mol-polecat-session / mol-crew-session                        │  │
│  │  - Onboarding step                                             │  │
│  │  - Execute work step                                           │  │
│  │  - Cleanup/handoff step                                        │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                │ instantiate
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                          WISP LAYER                                  │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  wisp-<agent-name>-<timestamp>                                 │  │
│  │  Storage: .beads-wisp/ (ephemeral, gitignored)                 │  │
│  │                                                                 │  │
│  │  Steps:                                                         │  │
│  │  1. orient   - Load context, read role template                │  │
│  │  2. handoff  - Check for predecessor handoff                   │  │
│  │  3. execute  - Run the attached work molecule                  │  │
│  │  4. cleanup  - Sync, handoff to successor, exit                │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                │ attachment
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                           MOL LAYER                                  │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │  The Actual Work (Permanent, Auditable)                        │  │
│  │  Storage: .beads/ (git-tracked)                                │  │
│  │                                                                 │  │
│  │  Could be:                                                      │  │
│  │  - mol-shiny (full workflow)                                   │  │
│  │  - mol-quick-fix (fast path)                                   │  │
│  │  - Any custom work molecule                                    │  │
│  │  - Direct issue (no molecule, just task)                       │  │
│  │  - Long-lived epic spanning many sessions                      │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. Agent Types and Their Wisps

### 2.1 Comparison: Patrol vs Work Agents

| Aspect | Patrol Wisp (Deacon/Refinery) | Work Wisp (Polecat/Crew) |
|--------|-------------------------------|--------------------------|
| **Lifecycle** | Looping - spawn, execute, burn, repeat | One-shot per session |
| **Inner Work** | Patrol steps (fixed, no attach) | Attached work mol (variable) |
| **Persistence** | Burns between cycles | Burns on session end |
| **Session** | Persists across wisps | Polecat: terminates; Crew: cycles |
| **Audit Trail** | Digests only (summaries) | Work mol is permanent |

### 2.2 Polecat Wisp (Ephemeral Worker)

Polecats are transient workers that:
- Get spawned for specific work
- Self-destruct on completion
- Have no persistent identity

```
Polecat Session Flow:
─────────────────────
spawn → orient → handoff(read) → execute → cleanup → TERMINATE
                                    │
                                    └──▶ [attached work mol]
```

### 2.3 Crew Worker Wisp (Persistent Worker)

Crew workers are long-lived agents that:
- Maintain persistent identity across sessions
- Work on long-lived molecules (epics spanning days/weeks)
- Hand off to successor sessions (themselves)
- **Can work autonomously overnight** if attached mol exists

```
Crew Worker Session Flow:
─────────────────────────
start → orient → handoff(read) → check attachment
                                       │
                        ┌──────────────┴──────────────┐
                        │                             │
                   ATTACHED                      NO ATTACHMENT
                        │                             │
                        ▼                             ▼
                ┌───────────────┐             ┌───────────────┐
                │ AUTO-CONTINUE │             │ AWAIT INPUT   │
                │ (no prompt)   │             │ (human directs)│
                └───────┬───────┘             └───────────────┘
                        │
                        ▼
                   execute mol
                        │
                        ▼
              cleanup → handoff(write) → END SESSION
                                              │
                                              ▼
                                    (successor picks up)
```

**Key insight**: If there's an attached mol, crew workers continue working
without awaiting input. This enables autonomous overnight work on long molecules.

---

## 3. The Onboard-Execute-Cleanup Pattern

### 3.1 mol-polecat-session

```markdown
## Step: orient
Read polecat.md protocol. Understand:
- Your identity (rig/polecat-name)
- The beads system
- Exit strategies
- Handoff protocols

Load context:
- gt prime
- gt mail inbox

## Step: handoff-read
Check for predecessor handoff mail.
If found, load context from previous session.

## Step: execute
Find attached work:
- gt mol status (shows what's on hook)
- bd show <work-mol-id>

Run the attached work molecule to completion.
Continue until reaching exit-decision step.

## Step: cleanup
1. Sync state: bd sync, git push
2. Close or defer work mol based on exit type
3. Burn this session wisp
4. Request shutdown from Witness
```

### 3.2 mol-crew-session (Light Harness)

```markdown
## Step: orient
Read crew.md protocol. Load context:
- gt prime
- Identify self (crew member name, rig)

## Step: handoff-read
Check inbox for 🤝 HANDOFF messages:
- gt mail inbox
- If found: read and load predecessor context

## Step: check-attachment
Look for pinned work:
- bd list --pinned --assignee=<self> --status=in_progress
- gt mol status

If attachment found:
  → Proceed to execute (no human input needed)
If no attachment:
  → Await user instruction

## Step: execute
Work the attached molecule:
- bd ready --parent=<work-mol-root>
- Continue from last completed step
- Work until context full or natural stopping point

## Step: cleanup
Before session ends:
1. git status / git push
2. bd sync
3. Write handoff mail to self:
   gt mail send <self-addr> -s "🤝 HANDOFF: <context>" -m "<details>"
4. Session ends (successor will pick up)
```

---

## 4. Autonomous Overnight Work

### 4.1 The Vision

With session wisps, crew workers can work autonomously:

```
Night 1, Session 1:
  Human: "Work on gt-abc (big feature epic)"
  Crew: Attaches gt-abc, works 2 hours, context full
  Crew: Writes handoff, ends session

Night 1, Session 2 (auto-spawned):
  Crew: Reads handoff, finds attached gt-abc
  Crew: AUTO-CONTINUES (no human needed)
  Crew: Works 2 more hours, context full
  Crew: Writes handoff, ends session

... repeat through the night ...

Morning:
  Human: Checks progress on gt-abc
  Crew: "Completed 15 of 23 subtasks overnight"
```

### 4.2 Requirements for Autonomous Work

1. **Attached mol** - Work must be pinned/attached
2. **Clear exit conditions** - Mol defines when to stop
3. **Session cycling** - Auto-spawn successor sessions
4. **Handoff protocol** - Each session writes handoff for next

### 4.3 Safety Rails

- Work mol defines scope (can't go off-rails)
- Each session is bounded (context limit)
- Handoffs create audit trail
- Human can intervene at any session boundary

---

## 5. Data Structures

### 5.1 Session Wisp

```json
{
  "id": "wisp-max-2024-12-22T15:00:00Z",
  "type": "wisp",
  "title": "Crew Session: max",
  "status": "in_progress",
  "current_step": "execute",
  "agent_type": "crew",
  "agent_name": "max",
  "attached_mol": "gt-abc123",
  "attached_at": "2024-12-22T15:01:00Z"
}
```

### 5.2 Attachment Mechanism

```bash
# Attach a work mol to current session wisp
bd mol attach <mol-id>

# Check current attachment
gt mol status

# Detach (human override)
bd mol detach
```

---

## 6. Implementation Phases

### Phase 1: Core Infrastructure (P0)

1. Create `mol-crew-session.formula.json` in `.beads/formulas/`
2. Create `mol-polecat-session.formula.json` in `.beads/formulas/`
3. Add wisp attachment mechanism to beads
4. Update spawn.go for polecat session wisps

### Phase 2: Crew Worker Integration (P0)

1. Update crew startup hook to instantiate session wisp
2. Implement auto-continue logic (if attached → work)
3. Update crew.md template for session wisp model

### Phase 3: Session Cycling (P1)

1. Add session successor spawning
2. Implement context-full detection
3. Auto-handoff on session end

### Phase 4: Monitoring & Safety (P1)

1. Witness monitors session wisps
2. Add emergency stop mechanism
3. Progress reporting between sessions

---

## 7. Open Questions

1. **Session cycling trigger**: Context percentage? Time limit? Both?
2. **Emergency stop**: How does human halt autonomous work?
3. **Progress visibility**: Dashboard for overnight work progress?
4. **Mol attachment UI**: How does human attach/detach work?

---

## 8. Summary

The session wisp architecture enables:

- **Separation of concerns**: How to work (proto) vs What to work on (mol)
- **Session continuity**: Handoffs preserve context across sessions
- **Autonomous work**: Crew workers can work overnight on attached mols
- **Unified pattern**: Same model for polecats and crew workers
- **Audit trail**: Work mol is permanent, session wisps are ephemeral

The key unlock is: **if there's attached work, continue without prompting**.
This transforms crew workers from interactive assistants to autonomous workers.
