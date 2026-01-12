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

Both do project work, but with key differences:

| Aspect | Crew | Polecat |
|--------|------|---------|
| **Lifecycle** | Persistent (user controls) | Transient (Witness controls) |
| **Monitoring** | None | Witness watches, nudges, recycles |
| **Work assignment** | Human-directed or self-assigned | Slung via `gt sling` |
| **Git state** | Pushes to main directly | Works on branch, Refinery merges |
| **Cleanup** | Manual | Automatic on completion |
| **Identity** | `<rig>/crew/<name>` | `<rig>/polecats/<name>` |

**When to use Crew**:
- Exploratory work
- Long-running projects
- Work requiring human judgment
- Tasks where you want direct control

**When to use Polecats**:
- Discrete, well-defined tasks
- Batch work (tracked via convoys)
- Parallelizable work
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

## The Propulsion Principle

All Gas Town agents follow the same core principle:

> **If you find something on your hook, YOU RUN IT.**

This applies regardless of role. The hook is your assignment. Execute it immediately
without waiting for confirmation. Gas Town is a steam engine - agents are pistons.

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

## Common Mistakes

1. **Using dogs for user work**: Dogs are Deacon infrastructure. Use crew or polecats.
2. **Confusing crew with polecats**: Crew is persistent and human-managed. Polecats are transient and Witness-managed.
3. **Working in wrong directory**: Gas Town uses cwd for identity detection. Stay in your home directory.
4. **Waiting for confirmation when work is hooked**: The hook IS your assignment. Execute immediately.
5. **Creating worktrees when dispatch is better**: If work should be owned by the target rig, dispatch it instead.
