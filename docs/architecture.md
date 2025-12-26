# Gas Town Architecture

Gas Town is a multi-agent workspace manager that coordinates AI coding agents working on software projects. It provides the infrastructure for spawning workers, processing work through a priority queue, and coordinating agents through mail and issue tracking.

**Gas Town is a stream engine, not a swarm engine.** Polecats (workers) can start and land at any time, independently. They're ephemeral - once their work merges, they evaporate. You can spawn a batch of polecats to tackle parallelizable work, but there's no "swarm ID" or batch boundary. The merge queue optionally serializes landing, preventing monkey-knife-fights when parallel polecats touch the same code. A single polecat is just a batch of one.

**Key insight**: Work is a stream, not discrete batches. The Refinery's merge queue is the coordination mechanism. Beads (issues) are the data plane. There are no "swarm IDs" - just epics with children, processed by workers, merged through the queue.

**Molecule-first paradigm**: Gas Town is fundamentally a molecule execution engine. Workers don't just "work on issues" - they execute molecules. The issue is seed data; the molecule defines the workflow. This enables nondeterministic idempotence: any worker can pick up where another left off, surviving crashes, context compaction, and restarts. If a process requires cognition, it should be a molecule. See [Molecules](#molecules-composable-workflow-templates) for full details.

**The Steam Engine Metaphor**: Gas Town is an engine. Engines do work and generate steam. In our system:
- **Proto molecules** are the fuel (templates that define workflows)
- **Wisps** are the steam (transient execution traces that rise and dissipate)
- **Digests** are the distillate (condensed permanent records of completed work)

Just as steam can dissipate or be condensed into useful output, wisps can be burned (cleaned up) or squashed (compressed into digests). This metaphor runs through all of Gas Town's vocabulary: bond, burn, squash, wisp.

## System Overview

```mermaid
graph TB
    subgraph "Gas Town"
        Overseer["👤 Overseer<br/>(Human Operator)"]

        subgraph Town["Town (~/gt/)"]
            Mayor["🎩 Mayor<br/>(Global Coordinator)"]

            subgraph Rig1["Rig: wyvern"]
                W1["👁 Witness"]
                R1["🔧 Refinery"]
                P1["🐱 Polecat"]
                P2["🐱 Polecat"]
                P3["🐱 Polecat"]
            end

            subgraph Rig2["Rig: beads"]
                W2["👁 Witness"]
                R2["🔧 Refinery"]
                P4["🐱 Polecat"]
            end
        end
    end

    Overseer --> Mayor
    Mayor --> W1
    Mayor --> W2
    W1 --> P1
    W1 --> P2
    W1 --> P3
    W2 --> P4
    P1 -.-> R1
    P2 -.-> R1
    P3 -.-> R1
    P4 -.-> R2
```

## Core Concepts

### HQ (Town)

The **HQ** (headquarters) is the installation directory where Gas Town lives - the physical root of your workspace. The terms "HQ" and "town" are often used interchangeably:
- **HQ** = physical (the directory at `~/gt/`)
- **Town** = logical (the Gas Town workspace concept)

An HQ contains:
- `CLAUDE.md` - Mayor role context (Mayor runs from HQ root)
- `mayor/` - Mayor configuration, state, and registry
- `.beads/` - Town-level beads (hq-* prefix for mayor mail)
- `rigs/` or rig directories - Managed project containers

Create an HQ with `gt install`:
```bash
gt install ~/gt --git  # Create HQ with git
```

**See**: [docs/hq.md](hq.md) for comprehensive HQ documentation, including:
- Beads redirect patterns for complex setups
- HQ templates for organizations
- Migration between HQs

### Rig

A **Rig** is a container directory for managing a project and its agents. Importantly, the rig itself is NOT a git clone - it's a pure container that holds:
- Rig configuration (`config.json`)
- Rig-level beads database (`.beads/`) for coordinating work
- Agent directories, each with their own git clone

This design prevents agent confusion: each agent has exactly one place to work (their own clone), with no ambiguous "rig root" that could tempt a lost agent.

### Overseer (Human Operator)

The **Overseer** is the human operator of Gas Town - not an AI agent, but the person who runs the system. The Overseer:

- **Sets strategy**: Defines project goals and priorities
- **Provisions resources**: Adds machines, polecats, and rigs
- **Reviews output**: Approves merged code and completed work
- **Handles escalations**: Makes final decisions on stuck or ambiguous work
- **Operates the system**: Runs `gt` commands, monitors dashboards

The Mayor reports to the Overseer. When agents can't resolve issues, they escalate up through the chain: Polecat → Witness → Mayor → Overseer.

### Agents

Gas Town has four AI agent roles:

| Agent | Scope | Responsibility |
|-------|-------|----------------|
| **Mayor** | Town-wide | Global coordination, work dispatch, cross-rig decisions |
| **Witness** | Per-rig | Worker lifecycle, nudging, pre-kill verification, session cycling |
| **Refinery** | Per-rig | Merge queue processing, PR review, integration |
| **Polecat** | Per-rig | Implementation work on assigned issues |

### Village Architecture

We're experimenting with a "village" model for agent coordination.

The pattern we're trying to avoid:
```
Centralized Monitor → watches all workers → single point of failure
                   → fragile protocols → cascading failures
```

The pattern we're exploring:
```
Every worker → understands the whole → can help any neighbor
           → peek is encouraged → distributed awareness
```

**Properties we're building toward:**

- **Distributed awareness**: Agents understand the system, not just their task
- **Mutual monitoring**: Any agent can peek at any other agent's health
- **Collective intervention**: If you see something stuck, you can help

Whether this achieves actual resilience at scale is still being tested.

**Practical implications:**

1. **Every patrol includes neighbor-checking** - Witness peeks at Refinery, Refinery peeks at Witness, everyone can peek at the Deacon
2. **`gt peek` is universal vocabulary** - Any agent can check any other agent's health
3. **Exit state enums are teaching tools** - COMPLETED, BLOCKED, REFACTOR, ESCALATE are shared vocabulary
4. **Mail is the nervous system** - Asynchronous, persistent, auditable coordination

### Mail

Agents communicate via **mail** - messages stored as beads issues with `type=message`. Mail enables:
- Work assignment (Mayor → Refinery → Polecat)
- Status reporting (Polecat → Witness → Mayor)
- Session handoff (Agent → Self for context cycling)
- Escalation (Witness → Mayor for stuck workers)

**Two-tier mail architecture:**
- **Town beads** (prefix: `gm-`): Mayor inbox, cross-rig coordination, handoffs
- **Rig beads** (prefix: varies): Rig-local agent communication

Mail commands use beads issues with type=message:
```bash
gt mail send mayor/ -s "Subject" -m "Body"   # Creates message issue
gt mail inbox                                  # Lists message issues
gt mail read gm-abc                           # Shows message issue
```

```mermaid
flowchart LR
    subgraph "Communication Flows"
        direction LR
        Mayor -->|"dispatch work"| Refinery
        Refinery -->|"assign issue"| Polecat
        Polecat -->|"done signal"| Witness
        Witness -->|"work complete"| Mayor
        Witness -->|"escalation"| Mayor
        Mayor -->|"escalation"| Overseer["👤 Overseer"]
    end
```

### Beads

**Beads** is the issue tracking system. Gas Town agents use beads to:
- Track work items (`bd ready`, `bd list`)
- Create issues for discovered work (`bd create`)
- Claim and complete work (`bd update`, `bd close`)
- Sync state to git (`bd sync`)

Polecats have direct beads write access and file their own issues.

#### Beads Configuration for Multi-Agent

Gas Town uses beads in a **shared database** configuration where all agents in a rig share one `.beads/` directory. This requires careful configuration:

| Agent Type | BEADS_DIR | BEADS_NO_DAEMON | sync-branch | Notes |
|------------|-----------|-----------------|-------------|-------|
| Polecat (worktree) | rig/.beads | **YES (required)** | recommended | Daemon can't handle worktrees |
| Polecat (full clone) | rig/.beads | Optional | recommended | Daemon safe but sync-branch helps |
| Refinery | rig/.beads | No | optional | Owns main, daemon is fine |
| Witness | rig/.beads | No | optional | Read-mostly access |
| Mayor | rig/.beads | No | optional | Infrequent access |

**Critical: Worktrees require no-daemon mode.** The beads daemon doesn't know which branch each worktree has checked out, and can commit/push to the wrong branch.

**Environment setup when spawning agents:**

```bash
# For worktree polecats (REQUIRED)
export BEADS_DIR=/path/to/rig/.beads
export BEADS_NO_DAEMON=1

# For full-clone polecats (recommended)
export BEADS_DIR=/path/to/rig/.beads
# Daemon is safe, but consider sync-branch for coordination

# Rig beads config.yaml should include:
sync-branch: beads-sync    # Separate branch for beads commits
```

**Why sync-branch?** When multiple agents share a beads database, using a dedicated sync branch prevents beads commits from interleaving with code commits on feature branches.

#### Beads as Data Plane (CLAW)

Gas Town uses Beads for persistence - **C**ommits as **L**edger of **A**ll **W**ork.
We've combined the data plane and control plane:
- Traditional systems separate "what's stored" from "what to do next"
- We store both in beads, so agent state survives crashes

In traditional systems:
- **Data plane**: Stores information (issues, messages)
- **Control plane**: Coordinates behavior (what to do next, who does what)

In Gas Town, **the control state IS data in beads**. Molecule steps, dependencies, and status ARE the control plane. Agents read beads to know what to do next.

This intentional blur provides:
- **Fault tolerance**: Control state survives agent crashes (it's in beads, not agent memory)
- **Observability**: `bd list` shows the full system state
- **Decentralization**: Each agent reads its own state from beads
- **Recovery**: Restart = re-read beads = continue from where you left off

There is no separate orchestrator maintaining workflow state. Beads IS the orchestrator.

| Category | Description | Status |
|----------|-------------|--------|
| **Work items** | Issues, tasks, epics | Core |
| **Mail** | Messages between agents (`type: message`) | Core |
| **Merge requests** | Queue entries (`type: merge-request`) | In progress |
| **Molecules** | Composable workflow templates | Planned (v1) |
| **Timed beads** | Scheduled recurring work | Planned (post-v1) |
| **Pinned beads** | Ongoing concerns that don't close | Planned (post-v1) |
| **Resource beads** | Leases, locks, quotas | Planned (post-v1) |

#### Two-Level Beads Architecture

Gas Town uses a **two-level beads architecture**. This is critical to understand:

```
~/gt/                              # Town repo (stevey-gt.git)
├── .beads/                        # TOWN-LEVEL: HQ beads (tracked here)
│   ├── config.yaml                # NO sync-branch (single clone)
│   └── issues.jsonl               # hq-* prefix for mayor mail
│
└── gastown/                       # Rig container (NOT a git clone)
    ├── .beads/                    # GITIGNORED - local runtime state
    │   └── (populated at runtime)
    │
    └── crew/max/                  # Project repo clone (gastown.git)
        └── .beads/                # RIG-LEVEL: Project beads (tracked in gastown.git)
            ├── config.yaml        # sync-branch: beads-sync
            └── issues.jsonl       # gt-* prefix for project issues
```

**Key points:**

| Level | Location | Git Repo | sync-branch | Prefix | Purpose |
|-------|----------|----------|-------------|--------|---------|
| Town | `~/gt/.beads/` | stevey-gt.git | NOT set | `hq-*` | Mayor mail, cross-rig coordination |
| Rig | `~/gt/gastown/crew/max/.beads/` | gastown.git | `beads-sync` | `gt-*` | Project bugs, features, tasks |

**Why two levels?**
- **Town beads** are for the Gas Town installation itself (only one clone)
- **Rig beads** are for the project (shared across multiple clones: crew, polecats, mayor/rig)

**Why different sync-branch settings?**
- **Town beads**: Single clone at HQ, no coordination needed, commits to main
- **Rig beads**: Multiple clones (polecats, crew, refinery), need `beads-sync` branch to avoid conflicts with code commits

**Common confusion:**
- `~/gt/gastown/.beads/` at the rig container level is **gitignored** (local runtime state)
- The real project beads live in the **gastown.git worktrees** (e.g., `crew/max/.beads/`)
- All clones share the same beads via git sync on the `beads-sync` branch

#### Mail Routing

Mail is routed to the correct beads database based on recipient address. The `Router` (in `internal/mail/router.go`) handles this:

```
Sender → Router.Send() → resolveBeadsDir(recipient) → creates message issue in target beads
```

**Routing logic (`resolveBeadsDir`):**

| Recipient | Beads Location | Example |
|-----------|----------------|---------|
| Town-level (`mayor/`, `deacon/`) | `{townRoot}/.beads` | `~/gt/.beads` |
| Rig-level (`rig/polecat`) | `{townRoot}/{rig}/.beads` | `~/gt/gastown/.beads` |
| Unknown/fallback | Town-level beads | `~/gt/.beads` |

**Town root detection:**
The router finds the town root by walking up directories looking for `mayor/town.json`. If not found, it falls back to the caller's workDir.

**Environment setup:**
All `bd` commands are invoked with:
- `BEADS_DIR=<resolved-path>` - Routes to correct database
- `BEADS_AGENT_NAME=<sender-identity>` - Identifies sender

#### Shared Beads for Polecats

Polecats use **redirect files** instead of their own beads databases. This eliminates git sync overhead between polecat worktrees.

**Structure:**
```
rig/
  .beads/                    ← Shared database (rig-level)
  polecats/
    <name>/
      .beads/
        redirect             ← Contains "../../.beads"
```

**How it works:**
1. When a polecat is spawned, `setupSharedBeads()` (in `internal/polecat/manager.go`) creates the redirect file
2. The beads CLI reads the redirect file and follows it to the rig's shared database
3. All polecats read/write the same beads database - no git sync needed

**Benefits:**
- No JSONL merge conflicts between polecats
- Instant visibility of issue updates across all workers
- Reduced git operations (no beads-sync branch coordination for polecats)

**Redirect vs Clone beads:**

| Agent Type | Beads Location | Method |
|------------|----------------|--------|
| Polecat (worktree) | Redirect to `rig/.beads` | `.beads/redirect` file |
| Crew worker (clone) | Own `.beads/` | Git sync on `beads-sync` |
| Mayor/Refinery | Rig's `.beads/` | Direct or symlink |

**Molecules** are crystallized workflow patterns that can be attached to work items. See the dedicated **Molecules** section below for full details on composition, nondeterministic idempotence, and built-in workflows.

**The OS Metaphor**: Gas Town is an operating system for work:

| OS Concept | Gas Town |
|------------|----------|
| Kernel | Daemon |
| Process scheduler | Ready work + dependencies |
| Timer interrupts | Timed beads |
| Semaphores | Resource beads |
| Background services | Pinned beads |
| Process templates | Proto molecules |
| Running processes | Wisp molecules |
| Process termination | Burn (discard) or squash (save state) |
| IPC | Mail beads |

## Molecules: Composable Workflow Templates (MEOW)

**MEOW** - **M**olecular **E**xpression **O**f **W**ork. Molecules are **crystallized,
composable, nondeterministic-idempotent workflow templates**. They encode structured
workflows that any worker can execute, with full auditability and the ability for
any worker to pick up where another left off.

Work flows through the **Rig → Cook → Run** lifecycle:
- **Rig**: Compose workflow formulas (YAML source files with `extends`, `compose`)
- **Cook**: Transform formulas into executable protos (`bd cook` expands macros, applies aspects)
- **Run**: Agents execute the cooked workflow (pour creates mols, wisp creates ephemeral traces)

See [molecular-chemistry.md](molecular-chemistry.md) for the full lifecycle specification.

### Core Concepts

| Concept | Name | Description |
|---------|------|-------------|
| Source | **Formula** | YAML workflow definition with composition rules (.formula.yaml) |
| Template | **Proto Molecule** | Cooked workflow pattern (the "fuel") |
| Running execution | **Wisp Molecule** | Transient execution trace (the "steam") |
| Permanent record | **Digest** | Compressed summary of completed work (the "distillate") |
| Individual step | **Atom/Step** | Smallest unit of work within a molecule |
| Dependency | **Bond** | Connection between steps (Needs: directive); also the act of instantiation |
| Composed molecule | **Polymer/Derived** | Molecule built from other molecules |
| Discard execution | **Burn** | Delete wisps without saving (routine work) |
| Compress execution | **Squash** | Compress wisps into a digest (preserve outcome) |

### Molecule Phase Lifecycle

Molecules follow a **states of matter** metaphor through their lifecycle:

```
                    ┌─────────────┐
                    │   Proto     │
                    │  (crystal)  │
                    └──────┬──────┘
                           │
                      bd mol bond
                           │
              ┌────────────┴────────────┐
              │                         │
              ▼                         ▼
      ┌───────────────┐        ┌───────────────┐
      │     Mol       │        │    Wisp       │
      │   (liquid)    │        │    (gas)      │
      │   durable     │        │  transient    │
      │  main beads   │        │ .beads-wisp/  │
      └───────┬───────┘        └───────┬───────┘
              │                        │
         bd mol squash            bd mol squash
              │                        │
              ▼                        ▼
      ┌───────────────┐        ┌───────────────┐
      │    Digest     │        │   (nothing)   │
      │ (distillate)  │        │  evaporates   │
      │  in git hist  │        └───────────────┘
      └───────────────┘
```

**Phase transitions:**
- **Proto → Mol/Wisp** (`bd mol bond`): Instantiate a template into a running execution
- **Mol → Digest** (`bd mol squash`): Compress completed work into permanent record
- **Wisp → (evaporates)** (`bd mol squash` or `bd mol burn`): Transient trace disappears

### When to Use Mol vs Wisp

The choice between **Mol** (durable) and **Wisp** (transient) depends on the work's importance and audit requirements:

| Aspect | Mol (Durable) | Wisp (Transient) |
|--------|---------------|------------------|
| **Storage** | Main `.beads/` database | `.beads-wisp/` directory |
| **Persistence** | Survives indefinitely | Evaporates on squash/burn |
| **Git tracking** | Committed, synced | Never committed |
| **Audit trail** | Full history preserved | Only digest (if squashed) |
| **Use case** | Important work | Routine operations |

**Use Mol for:**
- Code review waves (need audit trail of findings)
- Epic implementation (track progress across sessions)
- Feature work (preserve design decisions)
- Anything you might need to reference later

**Use Wisp for:**
- Orchestration tasks (witness patrols, health checks)
- Polecat work sessions (transient by nature)
- Patrol loops (continuous monitoring)
- Routine operations (no audit value)

**Rule of thumb**: If you'd regret losing the execution trace, use Mol. If the work is routine and only the outcome matters, use Wisp.

### Molecule Format

Molecules are stored as JSONL in `molecules.jsonl`:

```json
{
  "id": "mol-shiny",
  "title": "Shiny: {{feature}}",
  "description": "Full workflow from design to merge.",
  "labels": ["template"],
  "issue_type": "epic"
}
```

Steps are hierarchical children with dependencies encoded as beads edges:

```
mol-shiny
├── .1 design      "Think about architecture"
├── .2 implement   "Write the code"           ← depends on .1
├── .3 review      "Self-review changes"      ← depends on .2
├── .4 test        "Write and run tests"      ← depends on .2
└── .5 submit      "Submit for merge"         ← depends on .3, .4
```

**Key points:**
- Storage is JSONL (standard beads issue format)
- `{{var}}` placeholders resolved at bond time
- Steps are child issues, not embedded markdown
- Dependencies are beads edges, not text directives

### Molecule Composition

Molecules compose via bonding - attaching one molecule to another:

```
mol-gastown-polecat (composed)
├── [mol-shiny steps bonded here]
│   ├── .1 design
│   ├── .2 implement
│   ├── .3 review
│   ├── .4 test
│   └── .5 submit
└── .6 install-binary  ← depends on .5 (submit)
```

**Composition via bond:**
```bash
bd mol bond mol-shiny $PARENT_MOL
bd create "Install binary" --parent=$PARENT_MOL --deps=.5
```

**Semantics:**
- Bonding attaches one molecule's steps as children
- New steps can depend on bonded steps
- Multiple bonds create complex polymers
- Dependencies are standard beads edges

### Nondeterministic Idempotence

This is the key property enabling distributed molecule execution:

1. **Deterministic Structure**: Molecule defines exactly what steps exist and their dependencies
2. **Nondeterministic Execution**: Any worker can execute any ready step
3. **Idempotent Progress**: Completed steps stay completed; re-entry is safe

**How it works:**

```
Worker A picks up "design" (pending → in_progress)
Worker A completes "design" (in_progress → completed)
Worker A dies before "implement"
Worker B queries bd ready, sees "implement" is now ready
Worker B picks up "implement" (any worker can continue)
```

This is like a **distributed work queue** backed by beads:
- Beads is the queue (steps are issues with status)
- Git is the persistence layer
- No separate message broker needed
- Full auditability of who did what, when

### Wisp Molecules: Transient Execution Traces

**Wisps** are transient execution traces - the "steam" in Gas Town's engine metaphor. Claude is fire; Claude Code is a Steam engine; Gas Town is a Steam Train, with Beads as the tracks. Wisps are steam vapors that dissipate after the work is done.

**Why wisps?**
- **Observability**: See what's happening during execution without cluttering the permanent ledger
- **Recovery**: Wisps provide checkpoints for crash recovery
- **Compression**: Squash wisps into a digest when done - keep the outcome, discard the trace
- **Clean ledger**: Permanent beads show what was accomplished; wisps show how (temporarily)

**Wisp workflow:**

```
Proto Molecule (template)
         │
         ▼ bond
    Wisp Molecule (execution)
         │
    ┌────┴────┐
    ▼         ▼
  burn     squash
(discard)  (digest)
```

1. **Bond**: Instantiate a proto molecule as a wisp molecule with transient children
2. **Execute**: Agents work on wisp children, marking steps complete
3. **Complete**: When done, either:
   - **Burn**: Discard wisps entirely (routine work, no audit needed)
   - **Squash**: Compress wisps into a digest (preserve summary of what was done)

**Wisp structure:**

```
gt-abc123 (Proto: shiny)
    │
    ▼ bond
gt-abc123.exec-001 (Wisp Molecule)  ← wisp=true, parent=gt-abc123
    ├── gt-abc123.exec-001.design     ← wisp child
    ├── gt-abc123.exec-001.implement  ← wisp child
    ├── gt-abc123.exec-001.review     ← wisp child
    └── gt-abc123.exec-001.test       ← wisp child
    │
    ▼ squash
gt-abc123.digest-001 (Digest)       ← wisp=false, permanent
```

**Wisp commands:**

```bash
bd mol bond gt-abc123                    # Create wisp molecule from proto
bd mol bond gt-abc123 --template=quick   # Use specific template
bd mol squash gt-abc123.exec-001         # Compress wisps to digest
bd mol squash --summary="Agent summary"  # With agent-generated summary
bd mol burn gt-abc123.exec-001           # Discard wisps without digest
```

**Wisp storage:**

Wisps are stored in a per-rig wisp database:
- `<rig>/.beads-wisp/` - Separate from permanent beads, **gitignored**
- Fast writes, no sync overhead
- Auto-cleaned on squash/burn
- Digests write to permanent beads

**See**: [wisp-architecture.md](wisp-architecture.md) for full specification including:
- Canonical storage locations
- Role assignments (which agents use wisps)
- Patrol pattern implementation
- Beads implementation requirements

**Patrols use wisps:**

Patrol agents (long-running monitors) execute in infinite wisp loops:
1. Execute molecule as wisp
2. Squash to digest (compressed activity record)
3. Sleep/wait for trigger
4. Repeat

This gives patrols full audit trails without ledger bloat.

### Dynamic Bonding: The Christmas Ornament Pattern

Some workflows need **dynamic structure** - steps that emerge at runtime based
on discovered work. Consider mol-witness-patrol: it monitors N polecats where
N varies. A static molecule can't express "for each polecat, do these steps."

The solution is **dynamic bonding** - creating child molecules at runtime:

```bash
# In survey-workers step:
for polecat in $(gt polecat list gastown); do
  bd mol bond mol-polecat-arm $PATROL_WISP_ID --var polecat_name=$polecat
done
```

This creates the **Christmas Ornament** shape:

```
                    ★ mol-witness-patrol (trunk)
                   /|\
            ┌─────┘ │ └─────┐
            ●       ●       ●     mol-polecat-arm (dynamic arms)
           ace     nux    toast
            │       │       │
         ┌──┴──┐ ┌──┴──┐ ┌──┴──┐
         │steps│ │steps│ │steps│  (each arm has 4-5 steps)
         └──┬──┘ └──┬──┘ └──┬──┘
            └───────┴───────┘
                    │
                 ⬣ base (cleanup)
```

Arms execute in **parallel**. The `aggregate` step uses `WaitsFor: all-children`
to gate until all dynamically-bonded children complete.

See [molecular-chemistry.md](molecular-chemistry.md#dynamic-bonding-the-christmas-ornament-pattern)
for full documentation.

### The Activity Feed

Dynamic bonding enables a **real-time activity feed** - structured work state
instead of parsing agent logs:

```
[14:32:01] ✓ patrol-x7k.inbox-check completed
[14:32:03] ✓ patrol-x7k.check-refinery completed
[14:32:08] + patrol-x7k.arm-ace bonded (5 steps)
[14:32:08] + patrol-x7k.arm-nux bonded (5 steps)
[14:32:09] → patrol-x7k.arm-ace.capture in_progress
[14:32:10] ✓ patrol-x7k.arm-ace.capture completed
[14:32:14] ✓ patrol-x7k.arm-ace.decide completed (action: nudge-1)
[14:32:17] ✓ patrol-x7k.arm-ace COMPLETE
[14:32:23] ✓ patrol-x7k SQUASHED → digest-x7k
```

**This is what you want to see.** Not Claude's internal monologue. WORK STATE.

The beads ledger becomes a real-time activity feed:
- `bd activity --follow` - Stream state transitions
- `bd activity --mol <id>` - Activity for specific molecule
- Control plane IS data plane - state transitions are queryable data

This transforms debugging from "read the logs" to "watch the feed."

### Step States

```
pending → in_progress → completed
                     ↘ failed
```

| State | Meaning |
|-------|---------|
| `pending` (open) | Step not yet started, waiting for dependencies |
| `in_progress` | Worker has claimed this step |
| `completed` (closed) | Step finished successfully |
| `failed` | Step failed (needs intervention) |

**Recovery mechanism:**
- If worker dies mid-step, step stays `in_progress`
- After timeout (default 30 min), step can be reclaimed
- `bd release <step-id>` manually releases stuck steps
- Another worker can then pick it up

### Instantiation

When a molecule is attached to an issue:

```bash
gt spawn --issue gt-xyz --molecule mol-shiny
```

1. Molecule is validated (steps, dependencies)
2. Child beads are created for each step:
   - `gt-xyz.design`, `gt-xyz.implement`, etc.
3. Inter-step dependencies are wired
4. First ready step(s) become available via `bd ready`
5. Polecat starts on first ready step

**Provenance tracking:**
- Each instance has an `instantiated_from` edge to the source molecule
- Enables querying: "show all instances of mol-shiny"

### Standard Molecules

Gas Town includes formula files (`.beads/formulas/`) for standard molecules:

**mol-shiny** (5 steps):
```
design → implement → review → test → submit
```
Full quality workflow with design phase and self-review.

**mol-quick-fix** (3 steps):
```
implement → test → submit
```
Fast path for small, well-understood changes.

**mol-research** (2 steps):
```
investigate → document
```
Exploration workflow for understanding problems.

Cook formula files into proto beads:
```bash
bd cook <formula-id>
```

### Usage

```bash
# List available molecules
gt molecule list

# Show molecule details
gt molecule show mol-shiny

# Instantiate on an issue
gt molecule instantiate mol-shiny --parent=gt-xyz

# Spawn polecat with molecule
gt spawn --issue gt-xyz --molecule mol-shiny
```

### Why Molecules?

**Goal: Nondeterministic Idempotence**

The idea is that workflows can survive crashes, context compaction, and restarts because state is stored externally in beads, not in agent memory.

1. **Crash recovery**: Agent dies mid-workflow? Restart and continue from last completed step. No work is lost.
2. **Context survival**: Claude's context compacts? Agent re-reads molecule state from beads and knows exactly where it was.
3. **Quality gates**: Every polecat follows the same review/test workflow, enforced by molecule structure.
4. **Error isolation**: Each step is a checkpoint; failures are contained, not cascading.
5. **Parallelism**: Independent steps can run in parallel across workers.
6. **Auditability**: Full history of who did what step, when - queryable in beads.
7. **Composability**: Build complex workflows from simple building blocks.
8. **Resumability**: Any worker can continue where another left off.

**Without molecules**: Agents are prompted with instructions, work from memory, and lose state on restart. Autonomous operation is impossible.

**With molecules**: Agents follow persistent TODO lists that survive anything. Work completion is guaranteed.

### Molecule vs Template

Beads has two related concepts:
- **bd template**: User-facing workflow templates with variable substitution
- **gt molecule**: Agent-focused execution templates with step dependencies

Both use similar structures but different semantics:
- Templates focus on parameterization (`{{variable}}` substitution)
- Molecules focus on execution (step states, nondeterministic dispatch)

### Config vs Molecule: When to Use Which

**The Key Principle: If it requires cognition, it's a molecule.**

| Use Case | Config | Molecule |
|----------|--------|----------|
| Static policy (max workers, timeouts) | ✅ | ❌ |
| Agent workflow (design → implement → test) | ❌ | ✅ |
| Outpost routing preferences | ✅ | ❌ |
| Error recovery with decisions | ❌ | ✅ |
| Environment variables | ✅ | ❌ |
| Multi-step processes that can fail | ❌ | ✅ |

**Config** (`config.json`, `outposts.yaml`):
- Declarative settings
- No decision-making required
- Read once at startup
- Changes require restart

**Molecules**:
- Procedural workflows
- Require agent cognition at each step
- Survive agent restarts
- Track progress through step states

**Example: Outpost Assignment**

Static policy in `outposts.yaml`:
```yaml
policy:
  default_preference: [local, gce-burst, cloudrun-burst]
```

But if assignment requires cognition (analyzing work characteristics, checking outpost health, making tradeoffs), escalate to a molecule like `mol-outpost-assign`.

**The insight**: Gas Town doesn't spawn workers on issues. It spawns workers on molecules. The issue is just the seed data for the molecule execution.

### Operational Molecules

Molecules aren't just for implementing features. Any multi-step process that requires cognition, can fail partway through, or needs to survive agent restarts should be a molecule.

**The key insight**: By encoding operational workflows as molecules, Gas Town gains **nondeterministic idempotence** for system operations, not just work. An agent can crash mid-startup, restart, read its molecule state, and continue from the last completed step.

#### mol-polecat-work

The full polecat lifecycle:

```
mol-polecat-work
├── .1 load-context     "gt prime, bd prime, check inbox"
├── .2 implement        "Write code, file discovered work"    ← .1
├── .3 self-review      "Check for bugs, security issues"     ← .2
├── .4 verify-tests     "Run tests, add coverage"             ← .2
├── .5 rebase-main      "Rebase, resolve conflicts"           ← .3, .4
├── .6 submit-merge     "Submit to queue, verify CI"          ← .5
├── .7 generate-summary "Squash summary, file remaining work" ← .6
└── .8 request-shutdown "Signal Witness, wait for kill"       ← .7
```

**Why this matters**: A polecat that crashes after step 4 doesn't lose work. On restart, it reads molecule state, sees ".4: completed, .5: pending", and continues rebasing.

#### mol-rig-activate

Activating a rig for work:

```
mol-rig-activate
├── .1 verify-rig     "Check config, git remote"
├── .2 start-witness  "Start if needed, verify health"  ← .1
├── .3 start-refinery "Start if needed, verify health"  ← .1
├── .4 sync-beads     "Sync from remote"                ← .2
├── .5 identify-ready "Query bd ready, prioritize"      ← .4
└── .6 spawn-workers  "Spawn polecats for work"         ← .5, .3
```

#### mol-graceful-shutdown

Shutting down Gas Town properly:

```
mol-graceful-shutdown
├── .1 notify-agents "Send shutdown to all agents"
├── .2 wait-squash   "Wait for molecule completion"   ← .1
├── .3 verify-clean  "Check git states clean"         ← .2
├── .4 kill-workers  "Terminate polecats, worktrees"  ← .3
├── .5 kill-core     "Terminate Witness, Refinery"    ← .4
└── .6 final-sync    "Final beads sync"               ← .5
```

**Key principle**: If a multi-step process requires cognition and can fail partway through, it should be a molecule. This applies to:
- Agent lifecycle (startup, work, shutdown)
- System operations (activation, deactivation)
- Batch processing (parallel worker coordination)
- Recovery procedures (doctor --fix)

### Pluggable Molecules

Some workflows benefit from **pluggable steps** - dimensions that can be added or removed dynamically. The canonical example is **code review**, where each review dimension (security, performance, test coverage) is a plugin molecule.

#### Philosophy: Plugins ARE Molecules

In Gas Town, plugins are molecules with specific labels. No separate format, no YAML configs, no directory conventions. Just beads.

```json
{
  "id": "mol-security-scan",
  "title": "Security scan: {{scope}}",
  "description": "Check for OWASP Top 10 vulnerabilities in {{scope}}",
  "labels": ["template", "plugin", "code-review", "phase:tactical", "tier:sonnet"],
  "issue_type": "task"
}
```

**Add a dimension**: Install a plugin molecule (`bd mol install mol-security-scan`)
**Remove a dimension**: Uninstall or don't bond it
**Customize a dimension**: Fork and modify the molecule

#### Plugin Labels

Labels encode metadata that would otherwise go in config files:

| Label | Purpose |
|-------|---------|
| `plugin` | Marks as bondable at hook points |
| `code-review` | Which molecule can use this plugin |
| `phase:tactical` | Grouping for ordering |
| `tier:sonnet` | Model hint |
| `witness` / `deacon` | Which patrol can use it |

#### Dynamic Assembly

At runtime, the parent molecule bonds plugins based on labels:

```bash
# In mol-code-review, during plugin-run step:
for plugin in $(bd mol list --label plugin,code-review --json | jq -r '.[].id'); do
  bd mol bond $plugin $CURRENT_MOL --var scope="$SCOPE"
done
```

Creates a structure like:
```
gt-xyz (mol-code-review instance)
├── discovery plugins (parallel)
│   ├── .file-census
│   ├── .dep-graph
│   └── .coverage-map
├── structural plugins (sequential)  ← depends on discovery
│   ├── .architecture-review
│   └── .abstraction-analysis
├── tactical plugins (parallel)       ← depends on structural
│   ├── .security-scan
│   └── .performance-review
└── .synthesis                        ← depends on tactical
```

#### Code Review Molecule

The **mol-code-review** molecule is the reference implementation:

**Discovery Phase** (parallel scouts):
- `mol-file-census` - Inventory: sizes, ages, churn rates
- `mol-dep-graph` - Dependencies, cycles, inversions
- `mol-coverage-map` - Test coverage, dead code

**Structural Phase** (sequential):
- `mol-architecture-review` - Does structure match domain?
- `mol-abstraction-analysis` - Wrangling at wrong layers?

**Tactical Phase** (parallel per hotspot):
- `security-scan` - OWASP Top 10, injection, auth bypass
- `performance-review` - N+1 queries, missing caching
- `complexity-analysis` - Cyclomatic > 10, deep nesting
- `test-gaps` - Untested branches, missing edge cases
- `elegance-review` - Magic numbers, unclear names

**Synthesis Phase** (single coordinator):
- Deduplicate findings
- Establish dependencies between fix-beads
- Prioritize by impact
- Sequence recommendations

#### Findings Become Beads

Each review step generates findings as beads:

```
gt-sec-001  SQL injection in login()     discovered-from: gt-xyz.tactical-security
gt-sec-002  Missing CSRF token           discovered-from: gt-xyz.tactical-security
gt-perf-001 N+1 query in dashboard       discovered-from: gt-xyz.tactical-performance
```

These are the work that "feeds the beast" - the review molecule generates fix beads.

#### Iteration Without Built-In Loops

You don't need convergence built into the molecule. Just run it again:

1. Run `gt molecule instantiate code-review`
2. Workers close all review beads, generate fix beads
3. Fix beads get closed
4. Run `gt molecule instantiate code-review` again
5. Fewer findings this time
6. Repeat until noise floor

Each instantiation is independent. The ledger shows all runs, enabling comparison.

#### Static vs Pluggable

| Aspect | Static Molecule | Pluggable Molecule |
|--------|-----------------|-------------------|
| Definition | Steps defined at template time | Steps bonded at runtime |
| Add step | Edit molecule template | Install plugin molecule |
| Remove step | Edit molecule template | Don't bond it |
| Customization | Edit description | Fork plugin molecule |
| Use case | Fixed workflows | Extensible workflows |

Both patterns are valid. Use static molecules for well-defined workflows (mol-shiny, mol-polecat-work). Use pluggable molecules when dimensions should be customizable (mol-code-review, mol-witness-patrol).

## Directory Structure

### HQ Level

The HQ (town root) is created by `gt install`:

```
~/gt/                              # HQ ROOT (Gas Town installation)
├── CLAUDE.md                      # Mayor role context (runs from here)
├── .beads/                        # Town-level beads (prefix: hq-)
│   ├── beads.db                   # Mayor mail, coordination, handoffs
│   └── config.yaml
│
├── .runtime/                      # Town runtime state (gitignored)
│   ├── daemon.json                # Daemon PID, heartbeat
│   ├── deacon.json                # Deacon cycle state
│   └── agent-requests.json        # Lifecycle requests
│
├── mayor/                         # Mayor configuration and state
│   ├── town.json                  # {"type": "town", "name": "..."}
│   ├── rigs.json                  # Registry of managed rigs
│   ├── config.json                # Town-level config (theme defaults, etc.)
│   └── state.json                 # Mayor agent state
│
├── rigs/                          # Standard location for rigs
│   ├── gastown/                   # A rig (project container)
│   └── wyvern/                    # Another rig
│
└── <rig>/                         # OR rigs at HQ root (legacy)
```

**Notes**:
- Mayor's mail is in town beads (`hq-*` issues), not JSONL files
- Rigs can be in `rigs/` or at HQ root (both work)
- `.runtime/` is gitignored - contains ephemeral process state
- See [docs/hq.md](hq.md) for advanced HQ configurations

### Rig Level

Created by `gt rig add <name> <git-url>`:

```
gastown/                           # Rig = container (NOT a git clone)
├── config.json                    # Rig identity only (type, name, git_url, beads.prefix)
├── .beads/ → mayor/rig/.beads     # Symlink to canonical beads in Mayor
│
├── .repo.git/                     # BARE REPO: Single git source of truth
│                                  # All worktrees share this .git database
│                                  # No working directory (invisible base)
│
├── settings/                      # Rig behavioral config (git-tracked)
│   ├── config.json                # Theme, merge_queue, max_workers
│   └── namepool.json              # Pool settings (style, max)
│
├── .runtime/                      # Rig runtime state (gitignored)
│   ├── witness.json               # Witness process state
│   ├── refinery.json              # Refinery process state
│   └── namepool-state.json        # In-use names, overflow counter
│
├── mayor/                         # Mayor's per-rig presence
│   ├── rig/                       # Worktree (human workspace)
│   │   └── .beads/                # Canonical rig beads (prefix: gt-, etc.)
│   └── state.json
│
├── refinery/                      # Refinery agent (merge queue processor)
│   ├── rig/                       # Worktree ON MAIN (can see polecat branches)
│   └── state.json
│
├── witness/                       # Witness agent (per-rig pit boss)
│   └── state.json                 # No clone needed (monitors polecats)
│
├── crew/                          # Overseer's personal workspaces
│   └── <name>/                    # Worktree or full clone (TBD)
│
└── polecats/                      # Worker directories (git worktrees)
    ├── Nux/                       # Worktree on polecat/Nux branch
    └── Toast/                     # Worktree on polecat/Toast branch
```

**Configuration tiers:**
- **Identity** (`config.json`): Rig name, git_url, beads prefix - rarely changes
- **Settings** (`settings/`): Behavioral config - git-tracked, shareable
- **Runtime** (`.runtime/`): Process state - gitignored, transient

**Git architecture (bare repo pattern):**
- `.repo.git/` is a **bare clone** (no working directory) - shared git database
- Mayor is a **regular clone** (separate `.git/`) - doesn't need branch visibility
- Refinery and polecats are **worktrees** of `.repo.git` - share refs
- Polecat branches are visible to refinery because they share the same .git database
- This is the standard git server pattern - not a hack

**Why bare repo?**
- No working directory to accidentally work in (invisible base)
- Refinery can see polecat branches immediately (shared refs)
- Only `main` gets pushed to origin (clean remote)
- Faster polecat spawn (worktrees are instant)

**Beads architecture:**
- Mayor's clone holds the canonical `.beads/` for the rig
- Rig root symlinks `.beads/` → `mayor/rig/.beads`
- All agents (crew, polecats, refinery) inherit beads via parent lookup
- Polecats use redirect files pointing to mayor/rig/.beads

**Key points:**
- The rig root has no `.git/` - it's a container, not a repository
- `.repo.git/` holds git data for refinery and polecats (worktrees)
- Mayor is a separate clone (doesn't need to see polecat branches)
- Refinery worktree is on `main` for merge operations
- Witness doesn't need a worktree (just monitors polecat state)

```mermaid
graph TB
    subgraph Rig["Rig: gastown (container, NOT a git clone)"]
        Config["config.json"]
        Beads[".beads/"]

        subgraph Polecats["polecats/"]
            Nux["Nux/<br/>(worktree)"]
            Toast["Toast/<br/>(worktree)"]
        end

        subgraph Refinery["refinery/"]
            RefRig["rig/<br/>(canonical main)"]
            RefState["state.json"]
        end

        subgraph Witness["witness/"]
            WitState["state.json"]
        end

        subgraph MayorRig["mayor/"]
            MayRig["rig/<br/>(git clone)"]
            MayState["state.json"]
        end

        subgraph Crew["crew/"]
            CrewMain["main/<br/>(git clone)"]
        end
    end

    Beads -.->|BEADS_DIR| Nux
    Beads -.->|BEADS_DIR| Toast
    Beads -.->|BEADS_DIR| RefRig
    Beads -.->|BEADS_DIR| MayRig
    Beads -.->|BEADS_DIR| CrewMain
```

### ASCII Directory Layout

For reference without mermaid rendering (see [hq.md](hq.md) for creation/setup):

```
~/gt/                                    # HQ ROOT (Gas Town installation)
├── CLAUDE.md                            # Mayor role context
├── .beads/                              # Town-level beads (gm-* prefix)
│   ├── beads.db                         # Mayor mail, coordination
│   └── config.yaml
│
├── mayor/                               # Mayor configuration and state
│   ├── town.json                        # {"type": "town", "name": "..."}
│   ├── rigs.json                        # Registry of managed rigs
│   └── state.json                       # Mayor agent state
│
├── gastown/                             # RIG (container, NOT a git clone)
│   ├── config.json                      # Rig configuration
│   ├── .repo.git/                       # Bare repo (shared git database)
│   ├── .beads/ → mayor/rig/.beads       # Symlink to Mayor's canonical beads
│   │
│   ├── mayor/                           # Mayor's per-rig presence
│   │   ├── rig/                         # Separate clone (canonical beads)
│   │   │   ├── .git/
│   │   │   ├── .beads/                  # CANONICAL rig beads (gt-* prefix)
│   │   │   └── <project files>
│   │   └── state.json
│   │
│   ├── refinery/                        # Refinery agent (merge queue)
│   │   ├── rig/                         # Worktree of .repo.git (on main)
│   │   │   ├── .git                     # File pointing to ../.repo.git
│   │   │   └── <project files>
│   │   └── state.json
│   │
│   ├── witness/                         # Witness agent (pit boss)
│   │   └── state.json                   # No clone needed
│   │
│   ├── crew/                            # Overseer's personal workspaces
│   │   └── <name>/                      # Full clone (inherits beads from rig)
│   │       ├── .git/
│   │       └── <project files>
│   │
│   ├── polecats/                        # Worker directories (worktrees)
│   │   ├── Nux/                         # Git worktree from .repo.git
│   │   │   └── <project files>          # (inherits beads from rig)
│   │   └── Toast/                       # Git worktree from .repo.git
│   │
│   └── molecules.jsonl                  # Optional local molecule catalog
│
└── wyvern/                              # Another rig (same structure)
    ├── config.json
    ├── .beads/ → mayor/rig/.beads
    ├── mayor/
    ├── refinery/
    ├── witness/
    ├── crew/
    └── polecats/
```

**Key changes from earlier design:**
- Town beads (`gm-*`) hold Mayor mail instead of JSONL files
- Mayor has per-rig clone that's canonical for beads
- `.repo.git/` bare repo serves as source for refinery/polecat worktrees
- Rig `.beads/` symlinks to Mayor's canonical beads
- Polecats are git worktrees from `.repo.git` (fast, share refs)

### Why Decentralized?

Agents live IN rigs rather than in a central location:
- **Locality**: Each agent works in the context of its rig
- **Independence**: Rigs can be added/removed without restructuring
- **Parallelism**: Multiple rigs can have active workers simultaneously
- **Simplicity**: Agent finds its context by looking at its own directory

## Agent Responsibilities

### Mayor

The Mayor is the global coordinator:
- **Work dispatch**: Spawns workers for issues, coordinates batch work on epics
- **Cross-rig coordination**: Routes work between rigs when needed
- **Escalation handling**: Resolves issues Witnesses can't handle
- **Strategic decisions**: Architecture, priorities, integration planning

**NOT Mayor's job**: Per-worker cleanup, session killing, nudging workers

### Witness

The Witness is the per-rig "pit boss":
- **Worker monitoring**: Track polecat health and progress
- **Nudging**: Prompt workers toward completion
- **Pre-kill verification**: Ensure git state is clean before killing sessions
- **Session lifecycle**: Kill sessions, update worker state
- **Self-cycling**: Hand off to fresh session when context fills
- **Escalation**: Report stuck workers to Mayor

**Key principle**: Witness owns ALL per-worker cleanup. Mayor is never involved in routine worker management.

### Refinery

The Refinery manages the merge queue:
- **PR review**: Check polecat work before merging
- **Integration**: Merge completed work to main
- **Conflict resolution**: Handle merge conflicts
- **Quality gate**: Ensure tests pass, code quality maintained

```mermaid
flowchart LR
    subgraph "Merge Queue Flow"
        P1[Polecat 1<br/>branch] --> Q[Merge Queue]
        P2[Polecat 2<br/>branch] --> Q
        P3[Polecat 3<br/>branch] --> Q
        Q --> R{Refinery}
        R -->|merge| M[main]
        R -->|conflict| P1
    end
```

#### Direct Landing (Bypass Merge Queue)

Sometimes Mayor needs to land a polecat's work directly, skipping the Refinery:

| Scenario | Use Direct Landing? |
|----------|---------------------|
| Single polecat, simple change | Yes |
| Urgent hotfix | Yes |
| Refinery unavailable | Yes |
| Multiple polecats, potential conflicts | No - use Refinery |
| Complex changes needing review | No - use Refinery |

**Commands:**

```bash
# Normal flow (through Refinery)
gt merge-queue add <rig> <polecat>     # Polecat signals PR ready
gt refinery process <rig>               # Refinery processes queue

# Direct landing (Mayor bypasses Refinery)
gt land --direct <rig>/<polecat>        # Land directly to main
gt land --direct --force <rig>/<polecat> # Skip safety checks
gt land --direct --skip-tests <rig>/<polecat>  # Skip test run
gt land --direct --dry-run <rig>/<polecat>     # Preview only
```

**Direct landing workflow:**

```mermaid
sequenceDiagram
    participant M as 🎩 Mayor
    participant R as Refinery Clone
    participant P as Polecat Branch
    participant B as 📦 Beads

    M->>M: Verify polecat session terminated
    M->>P: Check git state clean
    M->>R: Fetch polecat branch
    M->>R: Merge to main (fast-forward or merge commit)
    M->>R: Run tests (optional)
    M->>R: Push to origin
    M->>B: Close associated issue
    M->>P: Delete polecat branch (cleanup)
```

**Safety checks (skippable with --force):**
1. Polecat session must be terminated
2. Git working tree must be clean
3. No merge conflicts with main
4. Tests pass (skippable with --skip-tests)

**When direct landing makes sense:**
- Mayor is doing sequential work without parallel polecats
- Single worker completed an isolated task
- Hotfix needs to land immediately
- Refinery agent is down or unavailable

### Polecat

Polecats are the workers that do actual implementation:
- **Molecule execution**: Execute wisp molecules (not just "work on issues")
- **Self-verification**: Run decommission checklist before signaling done
- **Beads access**: Create issues for discovered work, close completed work
- **Clean handoff**: Ensure git state is clean for Witness verification
- **Shutdown request**: Request own termination via `gt handoff` (bottom-up lifecycle)

**Polecats are like wisps**: They exist only while working. When done, they request shutdown and are deleted (worktree removed, branch deleted). There is no "idle pool" of polecats.

**Polecat workflow** (molecule-first):
1. Spawn receives issue + proto molecule template
2. Bond creates wisp molecule from proto
3. Polecat executes wisp steps (design → implement → test → submit)
4. On completion, polecat generates summary and squashes wisps to digest
5. Request shutdown, get deleted

The polecat itself is transient, and so is its execution trace (wisps). Only the digest survives.

## Key Workflows

### Work Dispatch

Work flows through the system as a stream. The Overseer spawns workers, they process issues, and completed work enters the merge queue.

```mermaid
sequenceDiagram
    participant O as 👤 Overseer
    participant M as 🎩 Mayor
    participant W as 👁 Witness
    participant P as 🐱 Polecats
    participant R as 🔧 Refinery

    O->>M: Spawn workers for epic
    M->>W: Assign issues to workers
    W->>P: Start work

    loop For each worker
        P->>P: Work on issue
        P->>R: Submit to merge queue
        R->>R: Review & merge
    end

    R->>M: All work merged
    M->>O: Report results
```

**Note**: There is no "swarm ID" or batch boundary. Workers process issues independently. The merge queue handles coordination. Tackling an epic with parallel polecats is just spawning multiple workers for the epic's child issues.

### Worker Cleanup (Witness-Owned)

```mermaid
sequenceDiagram
    participant P as 🐱 Polecat
    participant W as 👁 Witness
    participant M as 🎩 Mayor
    participant O as 👤 Overseer

    P->>P: Complete work
    P->>W: Done signal

    W->>W: Capture git state
    W->>W: Assess cleanliness

    alt Git state dirty
        W->>P: Nudge (fix issues)
        P->>P: Fix issues
        P->>W: Done signal (retry)
    end

    alt Clean after ≤3 tries
        W->>W: Verify clean
        W->>P: Kill session
    else Stuck after 3 tries
        W->>M: Escalate
        alt Mayor can fix
            M->>W: Resolution
        else Mayor can't fix
            M->>O: Escalate to human
            O->>M: Decision
        end
    end
```

### Polecat Shutdown Protocol (Bottom-Up)

Polecats initiate their own shutdown. This enables streaming - workers come and go continuously without artificial batch boundaries.

```mermaid
sequenceDiagram
    participant P as 🐱 Polecat
    participant R as 🔧 Refinery
    participant W as 👁 Witness
    participant B as 📦 Beads

    P->>P: Complete wisp steps
    P->>P: Generate summary
    P->>B: Squash wisps → digest
    P->>R: Submit to merge queue
    P->>P: Run gt handoff

    Note over P: Verify git clean,<br/>PR exists,<br/>wisps squashed

    P->>W: Mail: "Shutdown request"
    P->>P: Set state = pending_shutdown

    W->>W: Verify safe to kill
    W->>P: Kill session
    W->>W: git worktree remove
    W->>W: git branch -d
```

**Key change**: Polecats generate their own summaries and squash wisps before handoff. The digest is the permanent record of what the polecat accomplished. This keeps beads as a pure tool - agents provide the intelligence for summarization.

**gt handoff command** (run by polecat):
1. Verify git state clean (no uncommitted changes)
2. Verify work handed off (PR created or in queue)
3. Send mail to Witness requesting shutdown
4. Wait for Witness to kill session (don't self-exit)

**Witness shutdown handler**:
1. Receive shutdown request
2. Verify PR merged or queued, no data loss risk
3. Kill session: `gt session stop <rig>/<polecat>`
4. Remove worktree: `git worktree remove polecats/<name>`
5. Delete branch: `git branch -d polecat/<name>`

**Why bottom-up?** In streaming, there's no "swarm end" to trigger cleanup. Each worker manages its own lifecycle. The Witness is the lifecycle authority that executes the actual termination.

### Session Cycling (Mail-to-Self)

When an agent's context fills, it hands off to its next session:

1. **Recognize**: Notice context filling (slow responses, losing track of state)
2. **Capture**: Gather current state (active work, pending decisions, warnings)
3. **Compose**: Write structured handoff note
4. **Send**: Mail handoff to own inbox
5. **Exit**: End session cleanly
6. **Resume**: New session reads handoff, picks up where old session left off

```mermaid
sequenceDiagram
    participant S1 as Agent Session 1
    participant MB as 📬 Mailbox
    participant S2 as Agent Session 2

    S1->>S1: Context filling up
    S1->>S1: Capture current state
    S1->>MB: Send handoff note
    S1->>S1: Exit cleanly

    Note over S1,S2: Session boundary

    S2->>MB: Check inbox
    MB->>S2: Handoff note
    S2->>S2: Resume from handoff state
```

## Key Design Decisions

### 1. Witness Owns Worker Cleanup

**Decision**: Witness handles all per-worker cleanup. Mayor is never involved.

**Rationale**:
- Separation of concerns (Mayor strategic, Witness operational)
- Reduced coordination overhead
- Faster shutdown
- Cleaner escalation path

### 2. Polecats Have Direct Beads Access

**Decision**: Polecats can create, update, and close beads issues directly.

**Rationale**:
- Simplifies architecture (no proxy through Witness)
- Empowers workers to file discovered work
- Faster feedback loop
- Beads v0.30.0+ handles multi-agent conflicts

### 3. Session Cycling via Mail-to-Self

**Decision**: Agents mail handoff notes to themselves when cycling sessions.

**Rationale**:
- Consistent pattern across all agent types
- Timestamped and logged
- Works with existing inbox infrastructure
- Clean separation between sessions

### 4. Decentralized Agent Architecture

**Decision**: Agents live in rigs (`<rig>/witness/rig/`) not centralized (`mayor/rigs/<rig>/`).

**Rationale**:
- Agents work in context of their rig
- Rigs are independent units
- Simpler role detection
- Cleaner directory structure

### 5. Three-Tier Configuration Architecture

**Decision**: Separate identity, settings, and runtime into distinct locations:
- **Identity** (`config.json`): Rig name, git_url, beads prefix - rarely changes
- **Settings** (`settings/`): Behavioral config - git-tracked, shareable, visible
- **Runtime** (`.runtime/`): Process state - gitignored, transient

**Rationale**:
- AI models often miss hidden directories (so `.gastown/` was bad)
- Identity config rarely changes; behavioral config may change often
- Runtime state should never be committed
- `settings/` is visible to agents, unlike hidden `.gastown/`

### 6. Rig as Container, Not Clone

**Decision**: The rig directory is a pure container, not a git clone of the project.

**Rationale**:
- **Prevents confusion**: Agents historically get lost (polecats in refinery, mayor in polecat dirs). If the rig root were a clone, it's another tempting target for confused agents. Two confused agents at once = collision disaster.
- **Single work location**: Each agent has exactly one place to work (their own `/rig/` clone)
- **Clear role detection**: "Am I in a `/rig/` directory?" = I'm in an agent clone
- **Refinery is canonical main**: Refinery's clone serves as the authoritative "main branch" - it pulls, merges PRs, and pushes. No need for a separate rig-root clone.

### 7. Plugins as Agents

**Decision**: Plugins are just additional agents with identities, mailboxes, and access to beads. No special plugin infrastructure.

**Rationale**:
- Fits Gas Town's intentionally rough aesthetic
- Zero new infrastructure needed (uses existing mail, beads, identities)
- Composable - plugins can invoke other plugins via mail
- Debuggable - just look at mail logs and bead history
- Extensible - anyone can add a plugin molecule to their catalog

**Structure**: Plugin molecules in `molecules.jsonl` with labels for discovery.

### 8. Rig-Level Beads via BEADS_DIR

**Decision**: Each rig has its own `.beads/` directory. Agents use the `BEADS_DIR` environment variable to point to it.

**Rationale**:
- **Centralized issue tracking**: All polecats in a rig share the same beads database
- **Project separation**: Even if the project repo has its own `.beads/`, Gas Town agents use the rig's beads instead
- **OSS-friendly**: For contributing to projects you don't own, rig beads stay separate from upstream
- **Already supported**: Beads supports `BEADS_DIR` env var (see beads `internal/beads/beads.go`)

**Configuration**: Gas Town sets `BEADS_DIR` when spawning agents:
```bash
export BEADS_DIR=/path/to/rig/.beads
```

**See also**: beads issue `bd-411u` for documentation of this pattern.

### 9. Direct Landing Option

**Decision**: Mayor can land polecat work directly, bypassing the Refinery merge queue.

**Rationale**:
- **Flexibility**: Not all work needs merge queue overhead
- **Sequential work**: Mayor doing single-polecat work shouldn't need Refinery
- **Emergency path**: Hotfixes can land immediately
- **Resilience**: System works even if Refinery is down

**Constraints**:
- Direct landing still uses Refinery's clone as the canonical main
- Safety checks prevent landing dirty or conflicting work
- Mayor takes responsibility for quality (no Refinery review)

**Commands**:
```bash
gt land --direct <rig>/<polecat>        # Standard direct land
gt land --direct --force <rig>/<polecat> # Skip safety checks
```

### 10. Beads Daemon Awareness

**Decision**: Gas Town must disable the beads daemon for worktree-based polecats.

**Rationale**:
- The beads daemon doesn't track which branch each worktree has checked out
- Daemon can commit beads changes to the wrong branch
- This is a beads limitation, not a Gas Town bug
- Full clones don't have this problem

**Configuration**:
```bash
# For worktree polecats (REQUIRED)
export BEADS_NO_DAEMON=1

# For full-clone polecats (optional)
# Daemon is safe, no special config needed
```

**See also**: beads docs/WORKTREES.md and docs/DAEMON.md for details.

### 11. Work is a Stream (No Swarm IDs)

**Decision**: Work state is encoded in beads epics and issues. There are no "swarm IDs" or separate swarm infrastructure - the epic IS the grouping, the merge queue IS the coordination.

**Rationale**:
- **No new infrastructure**: Beads already provides hierarchy, dependencies, status, priority
- **Shared state**: All rig agents share the same `.beads/` via BEADS_DIR
- **Queryable**: `bd ready` finds work with no blockers, enabling multi-wave orchestration
- **Auditable**: Beads history shows work progression
- **Resilient**: Beads sync handles multi-agent conflicts
- **No boundary problem**: When does a swarm start/end? Who's in it? These questions dissolve - work is a stream

**How it works**:
- Create an epic with child issues for batch work
- Dependencies encode ordering (task B depends on task A)
- Status transitions track progress (open → in_progress → closed)
- Witness queries `bd ready` to find next available work
- Spawn workers as needed - add more anytime
- Batch complete = all child issues closed (or just keep going)

**Example**: Batch work on authentication bugs:
```
gt-auth-epic              # Epic: "Fix authentication bugs"
├── gt-auth-epic.1        # "Fix login timeout" (ready, no deps)
├── gt-auth-epic.2        # "Fix session expiry" (ready, no deps)
└── gt-auth-epic.3        # "Update auth tests" (blocked by .1 and .2)
```

Workers process issues independently. Work flows through the merge queue. No "swarm ID" needed - the epic provides grouping, labels provide ad-hoc queries, dependencies provide sequencing.

### 12. Agent Session Lifecycle (One Daemon)

**Decision**: ONE daemon (Go process) for all Gas Town manages agent lifecycles. Agents use a unified `gt handoff` command to request lifecycle actions.

**Architecture**:
```
Gas Town Daemon (gt daemon)
├── Pokes Mayor periodically
├── Pokes all Witnesses periodically
├── Processes lifecycle requests from deacon/ inbox
└── Restarts sessions when cycle requested

Lifecycle Hierarchy:
  Daemon → manages Mayor, all Witnesses
  Witness → manages Polecats, Refinery (per rig)
```

**Rationale**:
- Agents can't restart themselves after exiting
- ONE daemon is simpler than per-rig daemons
- Daemon is dumb scheduler; intelligence is in agents
- Unified protocol means all agents work the same way

**Unified lifecycle command** (`gt handoff`):
```bash
gt handoff              # Context-aware default
gt handoff --shutdown   # Terminate, don't restart (polecats)
gt handoff --cycle      # Restart with handoff (long-running agents)
gt handoff --restart    # Fresh restart, no handoff
```

| Agent | Default | Sends request to |
|-------|---------|------------------|
| Polecat | --shutdown | rig/witness |
| Refinery | --cycle | rig/witness |
| Witness | --cycle | deacon/ |
| Mayor | --cycle | deacon/ |

**Lifecycle request protocol**:
1. Agent runs `gt handoff` (verifies git clean, sends handoff mail)
2. Agent sends lifecycle request to its manager
3. Agent sets `requesting_<action>: true` in state.json
4. Agent waits (does NOT self-exit)
5. Manager receives request, verifies safe
6. Manager kills session
7. Manager starts new session (for cycle/restart)
8. New session reads handoff mail, resumes work

**Daemon heartbeat loop**:
- Poke Mayor: "HEARTBEAT: check your rigs"
- Poke each Witness: "HEARTBEAT: check your workers"
- Agents ignore poke if already working
- Process any lifecycle requests in deacon/ inbox
- Restart dead sessions if cycle was requested

```mermaid
sequenceDiagram
    participant A1 as Agent Session 1
    participant M as Lifecycle Manager
    participant A2 as Agent Session 2

    A1->>A1: gt handoff --cycle
    A1->>A1: Send handoff mail to self
    A1->>M: Lifecycle request: cycle
    A1->>A1: Set requesting_cycle, wait

    M->>M: Verify safe to act
    M->>A1: Kill session
    M->>A2: Start new session
    A2->>A2: Read handoff mail
    A2->>A2: Resume work
```

**Polecat shutdown** (--shutdown default):
After Witness kills session:
- Remove worktree: `git worktree remove polecats/<name>`
- Delete branch: `git branch -d polecat/<name>`
- Polecat ceases to exist (transient)

### 13. Resource-Constrained Worker Pool

**Decision**: Each rig has a configurable `max_workers` limit for concurrent polecats.

**Rationale**:
- Claude Code can use 500MB+ RAM per session
- Prevents resource exhaustion on smaller machines
- Enables autonomous operation without human oversight
- Witness respects limit when spawning new workers

**Configuration** (in rig config.json):
```json
{
  "type": "rig",
  "max_workers": 8,
  "worker_spawn_delay": "5s"
}
```

**Witness behavior**:
- Query active worker count before spawning
- If at limit, wait for workers to complete
- Prioritize higher-priority ready issues

### 14. Outpost Abstraction for Federation

**Decision**: Federation uses an "Outpost" abstraction to support multiple compute backends (local, SSH/VM, Cloud Run, etc.) through a unified interface.

**Rationale**:
- Different workloads need different compute: burst vs long-running, cheap vs fast
- Cloud Run's pay-per-use model is ideal for elastic burst capacity
- VMs are better for autonomous long-running work
- Local is always the default for development
- Platform flexibility lets users choose based on their needs and budget

**Key insight**: Cloud Run's persistent HTTP/2 connections solve the "zero to one" cold start problem, making container workers viable for interactive-ish work at ~$0.017 per 5-minute session.

**Design principles**:
1. **Local-first** - Remote outposts are overflow, not primary
2. **Git remains source of truth** - All outposts sync via git
3. **HTTP for Cloud Run** - Don't force filesystem mail onto containers
4. **Graceful degradation** - System works with any subset of outposts

**See**: `docs/federation-design.md` for full architectural analysis.

## Multi-Wave Work Processing

For large task trees (like implementing GGT itself), workers can process multiple "waves" of work automatically based on the dependency graph.

### Wave Orchestration

A wave is not explicitly managed - it emerges from dependencies:

1. **Wave 1**: All issues with no dependencies (`bd ready`)
2. **Wave 2**: Issues whose dependencies are now closed
3. **Wave N**: Continue until all work is done

```mermaid
graph TD
    subgraph "Wave 1 (no dependencies)"
        A[Task A]
        B[Task B]
        C[Task C]
    end

    subgraph "Wave 2 (depends on Wave 1)"
        D[Task D]
        E[Task E]
    end

    subgraph "Wave 3 (depends on Wave 2)"
        F[Task F]
    end

    A --> D
    B --> D
    C --> E
    D --> F
    E --> F
```

### Witness Work Loop

```
while epic has open issues:
    ready_issues = bd ready --parent <epic-id>

    if ready_issues is empty and workers_active:
        wait for worker completion
        continue

    for issue in ready_issues:
        if active_workers < max_workers:
            spawn worker for issue
        else:
            break  # wait for capacity

    monitor workers, handle completions

all work complete - report to Mayor
```

### Long-Running Autonomy

With daemon session cycling, the system can run autonomously for extended periods:

- **Witness cycles**: Every few hours as context fills
- **Refinery cycles**: As merge queue grows complex
- **Workers cycle**: If individual tasks are very large
- **Daemon persistence**: Survives all agent restarts

The daemon is the only truly persistent component. All agents are transient sessions that hand off state via mail.

Work is a continuous stream - you can add new issues, spawn new workers, reprioritize the queue, all without "starting a new swarm" or managing batch boundaries.

## Configuration

### town.json

```json
{
  "type": "town",
  "version": 1,
  "name": "stevey-gastown",
  "created_at": "2024-01-15T10:30:00Z"
}
```

### rigs.json

```json
{
  "version": 1,
  "rigs": {
    "wyvern": {
      "git_url": "https://github.com/steveyegge/wyvern",
      "added_at": "2024-01-15T10:30:00Z"
    }
  }
}
```

### rig.json (Per-Rig Identity)

Each rig has a `config.json` at its root containing **identity only** (rarely changes):

```json
{
  "type": "rig",
  "version": 1,
  "name": "wyvern",
  "git_url": "https://github.com/steveyegge/wyvern",
  "beads": {
    "prefix": "wyv",
    "sync_remote": "origin"    // Optional: git remote for bd sync
  }
}
```

Behavioral settings live in `settings/config.json` (git-tracked, shareable):

```json
{
  "theme": "desert",
  "merge_queue": {
    "enabled": true,
    "auto_merge": true
  },
  "max_workers": 5
}
```

Runtime state lives in `.runtime/` (gitignored, transient):

```json
// .runtime/witness.json
{
  "state": "running",
  "started_at": "2024-01-15T10:30:00Z",
  "stats": { "polecats_spawned": 42 }
}
```

The rig's `.beads/` directory is always at the rig root. Gas Town:
1. Creates `.beads/` when adding a rig (`gt rig add`)
2. Runs `bd init --prefix <prefix>` to initialize it
3. Sets `BEADS_DIR` environment variable when spawning agents

This ensures all agents in the rig share a single beads database, separate from any beads the project itself might use.

## CLI Commands

### HQ Management

```bash
gt install [path]      # Create Gas Town HQ (see hq.md)
gt install --git       # Also initialize git with .gitignore
gt install --github=u/r  # Also create GitHub repo
gt git-init            # Initialize git for existing HQ
gt doctor              # Check workspace health
gt doctor --fix        # Auto-fix issues
```

### Agent Operations

```bash
gt status              # Overall town status
gt rigs                # List all rigs
gt polecats <rig>      # List polecats in a rig
```

### Communication

```bash
gt inbox               # Check inbox
gt send <addr> -s "Subject" -m "Message"
gt inject <polecat> "Message"    # Direct injection to session
gt capture <polecat> "<cmd>"     # Run command in polecat session
```

### Session Management

```bash
gt spawn --issue <id> --molecule mol-shiny  # Spawn polecat with workflow
gt handoff             # Polecat requests shutdown (run when done)
gt session stop <p>    # Kill polecat session (Witness uses this)
```

**Note**: `gt wake` and `gt sleep` are deprecated - polecats are transient, not pooled.

### Landing & Merge Queue

```bash
gt merge-queue add <rig> <polecat>  # Add to merge queue (normal flow)
gt merge-queue list <rig>           # Show pending merges
gt refinery process <rig>           # Trigger Refinery to process queue

gt land --direct <rig>/<polecat>    # Direct landing (bypass Refinery)
gt land --direct --force ...        # Skip safety checks
gt land --direct --skip-tests ...   # Skip test verification
gt land --direct --dry-run ...      # Preview only
```

### Emergency Operations

```bash
gt stop --all              # Kill ALL sessions (emergency halt)
gt stop --rig <name>       # Kill all sessions in one rig
gt doctor --fix            # Auto-repair common issues
```

## Plugins

Gas Town supports **plugins** - but in the simplest possible way: plugins are just more agents.

### Philosophy

Gas Town is intentionally rough and lightweight. A "credible plugin system" with manifests, schemas, and invocation frameworks would be pretentious for a project named after a Mad Max wasteland. Instead, plugins follow the same patterns as all Gas Town agents:

- **Identity**: Plugins have persistent identities like polecats and witnesses
- **Communication**: Plugins use mail for input/output
- **Artifacts**: Plugins produce beads, files, or other handoff artifacts
- **Lifecycle**: Plugins can be invoked on-demand or at specific workflow points

### Plugin Structure

Plugins are molecules with specific labels, stored in `molecules.jsonl`:

```json
{
  "id": "mol-merge-oracle",
  "title": "Analyze merge queue",
  "description": "Analyze pending changesets for conflicts and ordering.",
  "labels": ["template", "plugin", "refinery", "tier:sonnet"],
  "issue_type": "task"
}
```

### Invoking Plugins

Plugins are bonded during patrol execution:

```bash
# In mol-refinery-patrol plugin-run step:
bd mol bond mol-merge-oracle $PATROL_WISP \
  --var queue_state="$QUEUE_JSON"
```

Plugins execute as molecule steps, creating any necessary artifacts (beads, files, branches).

### Hook Points

Patrol molecules bond plugins at specific points. Discovery uses labels:

| Workflow Point | Patrol | Label Filter |
|----------------|--------|--------------|
| Refinery patrol | mol-refinery-patrol | `plugin,refinery` |
| Witness patrol | mol-witness-patrol | `plugin,witness` |
| Deacon patrol | mol-deacon-patrol | `plugin,deacon` |

Configuration is via labels - agents bond plugins that match their role.

### Example: Merge Oracle

The **merge-oracle** plugin analyzes changesets before the Refinery processes them:

**Input** (via mail from Refinery):
- List of pending changesets
- Current merge queue state

**Processing**:
1. Build overlap graph (which changesets touch same files/regions)
2. Classify disjointness (fully disjoint → parallel safe, overlapping → needs sequencing)
3. Use LLM to assess semantic complexity of overlapping components
4. Identify high-risk patterns (deletions vs modifications, conflicting business logic)

**Output**:
- Bead with merge plan (parallel groups, sequential chains)
- Mail to Refinery with recommendation (proceed / escalate to Mayor)
- If escalation needed: mail to Mayor with explanation

The mol-merge-oracle's description contains the prompts and classification criteria. Gas Town doesn't need to know the internals.

### Example: Plan Oracle

The **plan-oracle** plugin helps decompose work:

**Input**: An issue/epic that needs breakdown

**Processing**:
1. Analyze the scope and requirements
2. Identify dependencies and blockers
3. Estimate complexity (for parallelization decisions)
4. Suggest task breakdown

**Output**:
- Beads for the sub-tasks (created via `bd create`)
- Dependency links (via `bd dep add`)
- Mail back with summary and recommendations

### Example: Beads Hygiene

The **beads-hygiene** plugin detects and fixes cross-pollution between nested beads databases.

**Background**: Gas Town has a two-level beads architecture:
- **Town-level** (`~/gt/.beads/`): Mayor mail, cross-rig coordination, HQ-level issues
- **Rig-level** (`~/gt/<rig>/.beads/`): Project-specific work (bugs, features, tasks)

Workers sometimes get confused about which database they're in, especially when:
- Their cwd is in a rig but they interact with town-level beads
- They reference issues from the wrong level as dependencies
- They create issues with mismatched prefixes for their context

**Input** (periodic scan or on-demand via mail):
- List of all beads databases in the town
- Recent issue creation/update activity
- Agent identity and expected context

**Processing**:
1. Scan each beads database for prefix mismatches
   - Town-level should have `hq-*` prefix (headquarters)
   - Rig-level should have rig prefix (e.g., `gt-*` for gastown)
2. Check for cross-level dependency references
   - Flag `gt-*` issues that depend on `hq-*` (usually wrong)
3. Analyze recent activity for context confusion
   - Agent in `gastown/` creating HQ-level issues
   - Agent at town level creating rig-specific issues
4. Identify misfiled issues that should be moved

**Output**:
- Report of detected issues (via mail to Mayor)
- For each misfiled issue:
  - Original location and ID
  - Suggested correct location
  - Confidence level (definite misfile vs. ambiguous)
- Optionally: auto-move with `--fix` flag

**Hook Points**:
- Witness can invoke before spawning polecats (sanity check)
- Mayor can invoke periodically (nightly hygiene scan)
- Any agent can invoke on-demand when confused

**mol-beads-hygiene description (prompt core)**:
```
You are reviewing beads databases for cross-pollution between Gas Town's
two-level architecture:

TOWN LEVEL (~/gt/.beads/): Coordination, mayor mail, HQ issues
  - Prefix: hq-* (headquarters)
  - Contains: cross-rig coordination, strategic planning, HQ bugs

RIG LEVEL (~/gt/<rig>/.beads/): Project-specific work
  - Prefix: <rig>-* (e.g., gt-* for gastown rig)
  - Contains: bugs, features, tasks for that project

COMMON MISTAKES TO DETECT:
1. Issue created at wrong level (check prefix vs location)
2. Cross-level dependencies (usually wrong unless intentional)
3. Agent identity mismatch (polecat creating town-level issues)
4. Duplicate issues across levels (same title/description)

For each issue found, report:
- Issue ID and title
- Current location
- Why it appears misfiled
- Recommended action (move, merge, or leave with note)
```

### Why This Design

1. **Fits Gas Town's aesthetic**: Rough, text-based, agent-shaped
2. **Zero new infrastructure**: Uses existing mail, beads, identities
3. **Composable**: Plugins can invoke other plugins
4. **Debuggable**: Just look at mail logs and bead history
5. **Extensible**: Anyone can add a plugin by creating a directory

### Plugin Discovery

```bash
gt plugins <rig>           # List plugins in a rig
gt plugin status <name>    # Check plugin state
```

Or just `ls <rig>/plugins/`.

## Failure Modes and Recovery

Gas Town is designed for resilience. Common failure modes and their recovery:

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Agent crash | Session gone, state shows 'working' | `gt doctor` detects, reset state to idle |
| Git dirty state | Witness pre-kill check fails | Nudge worker, or manual commit/discard |
| Beads sync conflict | `bd sync` fails | Beads tombstones handle most cases |
| Tmux crash | All sessions inaccessible | `gt doctor --fix` cleans up |
| Stuck work | No progress for 30+ minutes | Witness escalates, Overseer intervenes |
| Disk full | Write operations fail | Clean logs, remove old clones |

### Recovery Principles

1. **Fail safe**: Prefer stopping over corrupting data
2. **State is recoverable**: Git and beads have built-in recovery
3. **Doctor heals**: `gt doctor --fix` handles common issues
4. **Emergency stop**: `gt stop --all` as last resort
5. **Human escalation**: Some failures need Overseer intervention

### Doctor Checks

`gt doctor` performs health checks at both workspace and rig levels:

**Workspace checks**: Config validity, Mayor mailbox, rig registry
**Rig checks**: Git state, clone health, Witness/Refinery presence
**Work checks**: Stuck detection, zombie sessions, heartbeat health

Run `gt doctor` regularly. Run `gt doctor --fix` to auto-repair issues.

## Federation: Outposts

Federation enables Gas Town to scale across machines via **Outposts** - remote compute environments that can run workers.

**Full design**: See `docs/federation-design.md`

### Outpost Types

| Type | Description | Cost Model | Best For |
|------|-------------|------------|----------|
| Local | Current tmux model | Free | Development, primary work |
| SSH/VM | Full Gas Town clone on VM | Always-on | Long-running, autonomous |
| CloudRun | Container workers on GCP | Pay-per-use | Burst, elastic, background |

### Core Abstraction

```go
type Outpost interface {
    Name() string
    Type() OutpostType  // local, ssh, cloudrun
    MaxWorkers() int
    ActiveWorkers() int
    Spawn(issue string, config WorkerConfig) (Worker, error)
    Workers() []Worker
    Ping() error
}

type Worker interface {
    ID() string
    Outpost() string
    Status() WorkerStatus  // idle, working, done, failed
    Issue() string
    Attach() error         // for interactive outposts
    Logs() (io.Reader, error)
    Stop() error
}
```

### Configuration

```yaml
# ~/gt/config/outposts.yaml
outposts:
  - name: local
    type: local
    max_workers: 4

  - name: gce-burst
    type: ssh
    host: 10.0.0.5
    user: steve
    town_path: /home/steve/ai
    max_workers: 8

  - name: cloudrun-burst
    type: cloudrun
    project: my-gcp-project
    region: us-central1
    service: gastown-worker
    max_workers: 20
    cost_cap_hourly: 5.00

policy:
  default_preference: [local, gce-burst, cloudrun-burst]
```

### Cloud Run Workers

Cloud Run enables elastic, pay-per-use workers:
- **Persistent HTTP/2 connections** solve cold start (zero-to-one) problem
- **Cost**: ~$0.017 per 5-minute worker session
- **Scaling**: 0→N automatically based on demand
- **When idle**: Scales to zero, costs nothing

Workers receive work via HTTP, clone code from git, run Claude, push results. No filesystem mail needed - HTTP is the control plane.

### SSH/VM Outposts

Full Gas Town clone on remote machines:
- **Model**: Complete town installation via SSH
- **Workers**: Remote tmux sessions
- **Sync**: Git for code and beads
- **Good for**: Long-running work, full autonomy if disconnected

### Design Principles

1. **Outpost abstraction** - Support multiple backends via unified interface
2. **Local-first** - Remote outposts are for overflow/burst, not primary
3. **Git as source of truth** - Code and beads sync everywhere
4. **HTTP for Cloud Run** - Don't force mail onto stateless containers
5. **Graceful degradation** - System works with any subset of outposts

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         MAYOR                                │
│  ┌──────────────────────────────────────────────────────┐   │
│  │               Outpost Manager                         │   │
│  │  - Tracks all registered outposts                     │   │
│  │  - Routes work to appropriate outpost                 │   │
│  │  - Monitors worker status across outposts             │   │
│  └──────────────────────────────────────────────────────┘   │
│         │              │                │                    │
│         ▼              ▼                ▼                    │
│  ┌──────────┐   ┌──────────┐     ┌──────────────┐           │
│  │  Local   │   │   SSH    │     │   CloudRun   │           │
│  │ Outpost  │   │ Outpost  │     │   Outpost    │           │
│  └────┬─────┘   └────┬─────┘     └──────┬───────┘           │
└───────┼──────────────┼──────────────────┼───────────────────┘
        │              │                  │
        ▼              ▼                  ▼
   ┌─────────┐   ┌─────────┐        ┌─────────────┐
   │  tmux   │   │  SSH    │        │  HTTP/2     │
   │ panes   │   │sessions │        │ connections │
   └─────────┘   └─────────┘        └─────────────┘
        │              │                  │
        └──────────────┼──────────────────┘
                       ▼
              ┌─────────────────┐
              │   Git Repos     │
              │  (code + beads) │
              └─────────────────┘
```

### CLI Commands

```bash
gt outpost list              # List configured outposts
gt outpost status [name]     # Detailed status
gt outpost add <type> ...    # Add new outpost
gt outpost ping <name>       # Test connectivity
```

### Implementation Status

Federation is tracked in **gt-9a2** (P3 epic). Key tasks:
- `gt-9a2.1`: Outpost/Worker interfaces
- `gt-9a2.2`: LocalOutpost (refactor current spawning)
- `gt-9a2.5`: SSHOutpost
- `gt-9a2.8`: CloudRunOutpost

## Implementation Status

Gas Town is written in Go. Current development epics:

- **Management**: gt-f9x (Town & Rig Management: install, doctor, federation)

See beads issues with `bd list --status=open` for current work items.
