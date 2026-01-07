# Gas Town AI Agent Guide

A practical reference for AI agents learning to use the `gt` (Gas Town) multi-agent orchestration system.

> **Source**: Based on Steve Yegge's [Welcome to Gas Town](https://steve-yegge.medium.com/welcome-to-gas-town-4f25ee16dd04) blog post and official Gas Town documentation.

---

## What is Gas Town?

Gas Town is an orchestration platform for managing multiple Claude Code instances simultaneously. It solves the coordination problem: while a single AI agent works well, coordinating 10-30 parallel instances requires infrastructure for accountability, routing, and tracking.

**Core metaphor**: Gas Town is a steam engine. Agents are pistons. Throughput depends entirely on pistons firing when they have work.

---

## The Propulsion Principle (CRITICAL)

> **If you find something on your hook, YOU RUN IT.**

This is the most important concept. When you're spawned with work on your hook:
- **DO NOT** wait for confirmation
- **DO NOT** announce yourself and ask "should I start?"
- **DO** execute immediately

**Why**: There's no supervisor polling you. The hook IS your assignment. Every moment you wait, the entire system stalls.

**Startup behavior**:
```bash
gt hook              # Check what's on your hook
# If work hooked → EXECUTE IMMEDIATELY
# If hook empty → Check mail, then wait for instructions
```

---

## Architecture Overview

```
Town (~/gt)
├── mayor/              ← Global coordinator
├── deacon/             ← Background supervisor daemon
│   └── dogs/           ← Infrastructure helpers (NOT workers)
└── <rig>/              ← Project container (one per repo)
    ├── .beads/         ← Issue tracking
    ├── mayor/rig/      ← Canonical clone (read-only for Mayor)
    ├── crew/           ← Persistent human workspaces
    │   └── <name>/     ← Individual crew member
    ├── polecats/       ← Ephemeral workers
    │   └── <name>/     ← Individual polecat (git worktree)
    ├── refinery/       ← Merge queue processor
    └── witness/        ← Polecat lifecycle manager
```

---

## Agent Roles

### Infrastructure Roles (system management)

| Role | Purpose | Lifecycle |
|------|---------|-----------|
| **Mayor** | Global coordinator, dispatches work across rigs | Singleton, persistent |
| **Deacon** | Background supervisor daemon, watchdog chain | Singleton, persistent |
| **Witness** | Per-rig polecat lifecycle manager | One per rig, persistent |
| **Refinery** | Per-rig merge queue processor | One per rig, persistent |

### Worker Roles (do actual project work)

| Role | Purpose | Lifecycle |
|------|---------|-----------|
| **Polecat** | Ephemeral worker with own git worktree | Transient, Witness-managed |
| **Crew** | Persistent worker with own clone | Long-lived, user-managed |
| **Dog** | Deacon helper for infrastructure tasks | Very short, Deacon-managed |

### When to Use Each

**Use Crew for**:
- Exploratory work
- Long-running projects
- Work requiring human judgment

**Use Polecats for**:
- Discrete, well-defined tasks
- Batch/parallel work
- Work benefiting from supervision

**Never use Dogs for user work** - they're infrastructure-only.

---

## Essential Commands

### Checking Your Assignment

```bash
gt hook                  # What's on my hook? (START HERE)
bd show <issue-id>       # Get issue details
bd ready                 # What's ready to work (no blockers)
```

### Work Management

```bash
# Claiming work
bd update <id> --status=in_progress  # Claim an issue

# Completing work
bd close <id>                        # Mark complete
bd close <id1> <id2> ...             # Close multiple at once
bd close <id> --continue             # Close and auto-advance to next step

# Finding work
bd list --status=open                # All open issues
bd list --status=in_progress         # Your active work
bd blocked                           # Issues waiting on dependencies
```

### Dispatching Work (Mayor/Coordinator)

```bash
gt sling <issue> <rig>               # Assign work to a polecat
gt convoy create "Name" <issues>     # Create convoy for batch work
gt convoy list                       # Dashboard of active work
gt convoy status <id>                # Detailed convoy progress
```

### Communication

```bash
gt mail inbox                        # Check messages
gt mail read <id>                    # Read specific message
gt mail send <addr> -s "Subject" -m "Message"
gt nudge <target> "message"          # Send to tmux session
gt escalate <issue> "reason"         # Escalate to human
```

### Status & Diagnostics

```bash
gt status                            # Overall town status
gt rigs                              # List all rigs
gt polecat list [rig]                # List polecats in a rig
gt doctor                            # Health checks
```

---

## Beads (Issue Tracking)

Beads is the git-based issue tracking system. All work is tracked as beads.

### Issue Prefixes

Each rig has a prefix. Commands auto-route based on prefix:
```bash
bd show gt-xyz      # Routes to gastown rig
bd show bd-abc      # Routes to beads rig
bd show hq-xyz      # Routes to town-level beads
```

### Creating Issues

```bash
bd create --title="Fix the bug" --type=task --priority=2
# Priority: 0=critical, 1=high, 2=medium, 3=low, 4=backlog
# Types: task, bug, feature, epic
```

### Dependencies

**Important gotcha**: Think "X needs Y", NOT "X comes before Y"

```bash
# WRONG: Phase 1 comes before Phase 2 (temporal thinking)
bd dep add phase1 phase2

# RIGHT: Phase 2 needs Phase 1 (requirement thinking)
bd dep add phase2 phase1
```

Verify with `bd blocked` to see if dependencies are correct.

---

## Molecules (Multi-Step Workflows)

Molecules are workflow templates for multi-step work.

### Key Terms

| Term | Description |
|------|-------------|
| **Formula** | Source TOML template |
| **Protomolecule** | Frozen template ready for instantiation |
| **Molecule** | Active workflow with trackable steps |
| **Wisp** | Ephemeral molecule (never synced) |

### Working Through a Molecule

```bash
bd mol current              # Where am I in the workflow?
bd ready                    # What step is next?
bd close <step> --continue  # Complete step and advance
```

**The propulsion loop**:
```
1. gt hook              # What's hooked?
2. bd mol current       # Where am I?
3. Execute step
4. bd close <step> --continue
5. GOTO 2
```

---

## Convoys (Batch Tracking)

Convoys track related issues across rigs.

```bash
# Create convoy
gt convoy create "Feature X" gt-abc gt-def --notify overseer

# Track progress
gt convoy status hq-cv-abc

# Dashboard view
gt convoy list
```

**Auto-convoy**: When you `gt sling` a single issue, it auto-creates a convoy for dashboard visibility.

---

## Session Lifecycle

### Starting a Session

```bash
# 1. Check your hook FIRST
gt hook

# 2. If work hooked → EXECUTE (no waiting!)

# 3. If hook empty → check mail
gt mail inbox

# 4. Still nothing → wait for user instructions
```

### During a Session

- Track progress with `bd update <id> --status=in_progress`
- Add comments with `bd comments add <id> "note"`
- Check next steps with `bd ready`

### Ending a Session (MANDATORY)

```bash
git status                           # Check changes
git add <files>                      # Stage code
bd sync                              # Sync beads
git commit -m "type(scope): desc"    # Commit code
git push -u origin HEAD              # Push branch (MANDATORY before gt done!)
bd close <issue> --reason "..."      # Close your issue
gt done --exit                       # Submit to merge queue & exit
```

**Work is NOT complete until `gt done --exit` succeeds.** Pushing is required, but it's not the final step—you must also submit to the merge queue.

### Handoff (For Incomplete Work)

```bash
gt handoff -m "Context for next session..."
# Or send explicit mail:
gt mail send mayor/ -s "🤝 HANDOFF: Brief" -m "Context here"
```

---

## Common Patterns

### Cross-Rig Work

**Option 1: Worktree** (your identity preserved)
```bash
# gastown/crew/joe working on beads
gt worktree beads
# Creates ~/gt/beads/crew/gastown-joe/
```

**Option 2: Dispatch** (target rig owns it)
```bash
bd create --prefix beads "Fix auth bug"
gt sling bd-xyz beads
```

### Parallel Execution (Mayor Pattern)

```bash
# Create convoy for tracking
gt convoy create "Wave 1" gt-abc gt-def gt-ghi

# Dispatch to polecats (parallel)
gt sling gt-abc gastown
gt sling gt-def gastown
gt sling gt-ghi gastown

# Monitor (~100 tokens vs 80K for Task agents)
gt convoy list
gt convoy status hq-cv-xyz
```

---

## The Capability Ledger

Every completion is recorded. Every bead you close becomes permanent record.

- **Your work is visible** - quality accumulates, sloppy work is also recorded
- **Redemption is real** - consistent good work builds over time
- **Your CV grows with every completion** - your history is your reputation

Execute with care. Build a track record.

---

## Common Mistakes

1. **Waiting for confirmation when work is hooked** - The hook IS your assignment
2. **Using dogs for user work** - Dogs are Deacon infrastructure only
3. **Confusing crew with polecats** - Crew is persistent/human-managed, polecats are transient/Witness-managed
4. **Working in wrong directory** - Gas Town uses cwd for identity detection
5. **Creating worktrees when dispatch is better** - If target rig should own it, dispatch instead
6. **Reading formula files directly** - Use `bd cook` + `bd mol pour`, not manual bead creation
7. **Inverting dependencies** - Think "X needs Y", not "X comes before Y"
8. **Stopping after push** - Work isn't complete until `gt done --exit` succeeds

---

## Quick Reference Card

```bash
# Startup
gt hook                    # What's my assignment?
bd show <id>               # Issue details
bd ready                   # What's unblocked?

# Working
bd update <id> --status=in_progress
bd close <id> --continue   # Complete and advance

# Dispatch (Mayor)
gt sling <issue> <rig>
gt convoy create "Name" <issues>
gt convoy list

# Communication
gt mail inbox
gt mail send <addr> -s "Subj" -m "Msg"

# Session End
git push -u origin HEAD    # Push branch first
bd close <id> --reason "." # Close your issue
gt done --exit             # Submit & exit (FINAL STEP)

# Emergency
gt escalate <issue> "reason"
```

---

## Philosophy

Gas Town embodies **Nondeterministic Idempotence (NDI)**: execution paths may vary, but outcomes remain reliable because persistent bead-backed workflows survive crashes and sessions resume automatically from checkpoints.

The system prioritizes **throughput over perfection**. It accepts occasional lost work in exchange for velocity. This is "vibe coding at scale" - productive chaos managed through persistent data structures and hierarchical supervision.

**The contract**: When work lands on your hook, you execute. No politics. No waiting. Just pistons firing.
