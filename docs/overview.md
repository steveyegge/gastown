# Understanding Gas Town

This document provides a conceptual overview of Gas Town's architecture, focusing on
the role taxonomy and how different agents interact.

## Why Gas Town Exists

As AI agents become central to engineering workflows, teams face new challenges:

- **Accountability:** Who did what? Which agent introduced this bug?
- **Quality:** Which agents are reliable? Which need tuning?
- **Efficiency:** How do you route work to the right agent?
- **Scale:** How do you coordinate agents across repos and teams?

Gas Town is an orchestration layer that treats AI agent work as structured data.
Every action is attributed. Every agent has a track record. Every piece of work
has provenance. See [Why These Features](why-these-features.md) for the full rationale.

## Role Taxonomy

Gas Town has several agent types, each with distinct responsibilities and lifecycles.

### Infrastructure Roles

These roles manage the Gas Town system itself:

| Role | Description | Lifecycle |
|------|-------------|-----------|
| **Mayor** | Global coordinator at mayor/ | Singleton, persistent |
| **Deacon** | Background supervisor daemon ([watchdog chain](design/watchdog-chain.md)) | Singleton, persistent |
| **Witness** | Per-rig polecat lifecycle manager | One per rig, persistent |
| **Refinery** | Per-rig merge queue processor | One per rig, persistent |

### Worker Roles

These roles do actual project work:

| Role | Description | Lifecycle |
|------|-------------|-----------|
| **Polecat** | Ephemeral worker with own worktree | Transient, Witness-managed ([details](concepts/polecat-lifecycle.md)) |
| **Crew** | Persistent worker with own clone | Long-lived, user-managed |
| **Dog** | Deacon helper for infrastructure tasks | Ephemeral, Deacon-managed |

## Convoys: Tracking Work

A **convoy** (🚚) is how you track batched work in Gas Town. When you kick off work -
even a single issue - create a convoy to track it.

```bash
# Create a convoy tracking some issues
gt convoy create "Feature X" gt-abc gt-def --notify overseer

# Check progress
gt convoy status hq-cv-abc

# Dashboard of active convoys
gt convoy list
```

**Why convoys matter:**
- Single view of "what's in flight"
- Cross-rig tracking (convoy in hq-*, issues in gt-*, bd-*)
- Auto-notification when work lands
- Historical record of completed work (`gt convoy list --all`)

The "swarm" is ephemeral - just the workers currently assigned to a convoy's issues.
When issues close, the convoy lands. See [Convoys](concepts/convoy.md) for details.

## Crew vs Polecats

Both do project work, but with fundamentally different purposes:

| Aspect | Crew | Polecat |
|--------|------|---------|
| **Primary role** | Formula owners | Formula executors |
| **Lifecycle** | Persistent (user controls) | Transient (Witness controls) |
| **Monitoring** | Self-directed | Witness watches, nudges, recycles |
| **Work assignment** | Subscribes to formula-matching work | Spawned by crew with molecule |
| **Git state** | Pushes to main directly | Pushes branch, submits MR, exits |
| **Exit point** | User-controlled | MR submission (`gt done`) |
| **Cleanup** | Manual | Automatic on `gt done` |
| **Identity** | `<rig>/crew/<name>` | `<rig>/polecats/<name>` |

### The Core Principle

**Crew build formulas, polecats run them.**

Each crew member owns exactly one formula:
- `crew/code_review` owns `code-review.formula.toml`
- `crew/conflict_resolve` owns `mol-polecat-conflict-resolve.formula.toml`

Crew responsibilities:
1. **Own the formula** - Canonical source of truth for one workflow
2. **Spawn polecats** - When work arrives, dispatch to workers
3. **Collect feedback** - Aggregate execution results
4. **Iterate** - Improve formula based on execution patterns
5. **Never execute directly** - Crew orchestrate, polecats execute

See [Crew Formula Ownership](design/crew-formula-ownership.md) for full details.

**When to use Crew**:
- Formula development and iteration
- Orchestrating parallel polecat workflows
- Long-running oversight of specific workflow types
- Work requiring human judgment on formula design

**When to use Polecats**:
- Executing molecules (formulas instantiated as work)
- Discrete, well-defined tasks
- Batch work (tracked via convoys)
- Work that benefits from supervision

## Dogs vs Crew

**Dogs are NOT workers**. This is a common misconception.

| Aspect | Dogs | Crew |
|--------|------|------|
| **Owner** | Deacon | Human |
| **Purpose** | Infrastructure tasks | Project work |
| **Scope** | Narrow, focused utilities | General purpose |
| **Lifecycle** | Very short (single task) | Long-lived |
| **Example** | Boot (triages Deacon health) | Joe (fixes bugs, adds features) |

Dogs are the Deacon's helpers for system-level tasks:
- **Boot**: Triages Deacon health on daemon tick
- Future dogs might handle: log rotation, health checks, etc.

If you need to do work in another rig, use **worktrees**, not dogs.

## Cross-Rig Work Patterns

When a crew member needs to work on another rig:

### Option 1: Worktrees (Preferred)

Create a worktree in the target rig:

```bash
# gastown/crew/joe needs to fix a beads bug
gt worktree beads
# Creates ~/gt/beads/crew/gastown-joe/
# Identity preserved: BD_ACTOR = gastown/crew/joe
```

Directory structure:
```
~/gt/beads/crew/gastown-joe/     # joe from gastown working on beads
~/gt/gastown/crew/beads-wolf/    # wolf from beads working on gastown
```

### Option 2: Dispatch to Local Workers

For work that should be owned by the target rig:

```bash
# Create issue in target rig
bd create --prefix beads "Fix authentication bug"

# Create convoy and sling to target rig
gt convoy create "Auth fix" bd-xyz
gt sling bd-xyz beads
```

### When to Use Which

| Scenario | Approach |
|----------|----------|
| You need to fix something quick | Worktree |
| Work should appear in your CV | Worktree |
| Work should be done by target rig team | Dispatch |
| Infrastructure/system task | Let Deacon handle it |

## Directory Structure

```
~/gt/                           Town root
├── .beads/                     Town-level beads (hq-* prefix, mail)
├── mayor/                      Mayor config
│   └── town.json
├── deacon/                     Deacon daemon
│   └── dogs/                   Deacon helpers (NOT workers)
│       └── boot/               Health triage dog
└── <rig>/                      Project container
    ├── config.json             Rig identity
    ├── .beads/ → mayor/rig/.beads  (symlink or redirect)
    ├── .repo.git/              Bare repo (shared by worktrees)
    ├── mayor/rig/              Mayor's clone (canonical beads)
    ├── refinery/rig/           Worktree on main
    ├── witness/                No clone (monitors only)
    ├── crew/                   Persistent human workspaces
    │   ├── joe/                Local crew member
    │   └── beads-wolf/         Cross-rig worktree (wolf from beads)
    └── polecats/               Ephemeral worker worktrees
        └── Toast/              Individual polecat
```

## Identity and Attribution

All work is attributed to the actor who performed it:

```
Git commits:      Author: gastown/crew/joe <owner@example.com>
Beads issues:     created_by: gastown/crew/joe
Events:           actor: gastown/crew/joe
```

Identity is preserved even when working cross-rig:
- `gastown/crew/joe` working in `~/gt/beads/crew/gastown-joe/`
- Commits still attributed to `gastown/crew/joe`
- Work appears on joe's CV, not beads rig's workers

## The Rebase-as-Work Architecture

Gas Town uses a fundamentally different approach to merge conflicts than traditional workflows:

**Traditional approach (doesn't scale):**
```
Polecat finishes → waits for merge → handles conflicts → re-submits → waits...
```

**Gas Town approach (rebase-as-work):**
```
Polecat finishes → submits to MQ → EXITS IMMEDIATELY
Refinery handles rebase → conflict? → spawn FRESH polecat
```

### Key Principles

1. **"Polecat done at MR submit"** - A polecat's job ends when it runs `gt done`. It doesn't wait for merge results, doesn't handle conflicts, doesn't get feedback. It submits and dies.

2. **"Conflicts are new work"** - Merge conflicts aren't "fixes to existing work" - they're new tasks that happen to have context from a previous attempt. The Refinery creates a conflict-resolution task and a fresh polecat handles it.

3. **"Convoy age creates pressure"** - Old convoys automatically get priority boosted. No manual escalation needed - the system applies backpressure naturally.

4. **"Priority function is deterministic"** - MR scoring is purely mechanical: base priority, retry penalty, convoy age bonus, age tiebreaker. No human judgment in the queue.

### Why This Works

- **No blocking** - Polecats never wait for merge outcomes
- **Clean context** - Fresh polecats don't carry accumulated confusion
- **Linear history** - Sequential rebasing prevents cascading conflicts
- **Resource efficiency** - Polecat resources freed immediately on completion

See [PRIMING.md](../PRIMING.md) for the quick-start guide and [Refinery Merge Workflow](concepts/refinery-merge-workflow.md) for implementation details.

## The Propulsion Principle

All Gas Town agents follow the same core principle:

> **If you find something on your hook, YOU RUN IT.**

This applies regardless of role. The hook is your assignment. Execute it immediately
without waiting for confirmation. Gas Town is a steam engine - agents are pistons.

## Agent Advice System

The [Agent Advice](concepts/agent-advice.md) system provides dynamic guidance to agents
based on learned patterns and operational experience. Unlike static role templates, advice
can be created, updated, and removed at runtime.

Advice is scoped hierarchically:
- **Global** - applies to all agents everywhere
- **Role** - applies to a role type (polecat, crew, witness)
- **Rig** - applies to all agents in a specific rig
- **Agent** - applies to a specific agent identity

Advice is delivered during `gt prime` and appears in the agent's context.

```bash
# Create global advice
bd advice add "Always verify git status before pushing"

# Create role-specific advice
bd advice add --role polecat "Check hook before checking mail"

# Create rig-specific advice
bd advice add --rig gastown "Use fimbaz account for spawning"

# List all advice
bd advice list
```

See [Agent Advice](concepts/agent-advice.md) for full documentation.

## Model Evaluation and A/B Testing

Gas Town's attribution and work history features enable objective model comparison:

```bash
# Deploy different models on similar tasks
gt sling gt-abc gastown --model=claude-sonnet
gt sling gt-def gastown --model=gpt-4

# Compare outcomes
bd stats --actor=gastown/polecats/* --group-by=model
```

Because every task has completion time, quality signals, and revision count,
you can make data-driven decisions about which models to deploy where.

This is particularly valuable for:
- **Model selection:** Which model handles your codebase best?
- **Capability mapping:** Claude for architecture, GPT for tests?
- **Cost optimization:** When is a smaller model sufficient?

## Crew-Polecat Coordination

Crew own formulas and dispatch polecats to execute them. This creates a clean
separation between workflow design (crew) and workflow execution (polecat).

**The Dispatch Flow:**
1. Work arrives matching crew's formula subscription (label, type, etc.)
2. Crew runs `gt crew dispatch <bead>` to spawn a polecat
3. Polecat receives molecule from crew's formula
4. Polecat executes work autonomously
5. Polecat calls `gt done` when complete
6. Execution report generated for crew feedback
7. Crew receives `WORK_DONE: <issue-id>` mail

**The Feedback Loop:**
1. Polecat execution reports accumulate in crew's feedback inbox
2. Crew periodically reviews execution patterns
3. Common failures or slow steps trigger formula improvements
4. Updated formula benefits all future executions

**Example:**
```bash
# Crew discovers work matching their formula
gt crew prime  # Shows pending work for code-review formula

# Crew dispatches to polecat
gt crew dispatch gt-review-456

# ... polecat works ...

# Crew reviews execution feedback
gt crew feedback
# Shows: 95% success rate, step 3 avg 45s, 2 timeouts this week

# Crew iterates on formula if needed
gt crew formula edit
```

See [Crew Formula Ownership](design/crew-formula-ownership.md) for the full model.

## Common Mistakes

1. **Using dogs for user work**: Dogs are Deacon infrastructure. Use crew or polecats.
2. **Confusing crew with polecats**: Crew is persistent and human-managed. Polecats are transient and Witness-managed.
3. **Working in wrong directory**: Gas Town uses cwd for identity detection. Stay in your home directory.
4. **Waiting for confirmation when work is hooked**: The hook IS your assignment. Execute immediately.
5. **Creating worktrees when dispatch is better**: If work should be owned by the target rig, dispatch it instead.
