# Molecules

Molecules are workflow templates that coordinate multi-step work in Gas Town.

## Molecule Lifecycle

```
Formula (source TOML) ─── "Ice-9"
    │
    ▼ bd cook
Protomolecule (frozen template) ─── Solid
    │
    ├─▶ bd mol pour ──▶ Mol (persistent) ─── Liquid ──▶ bd squash ──▶ Digest
    │
    └─▶ bd mol wisp ──▶ Wisp (ephemeral) ─── Vapor ──┬▶ bd squash ──▶ Digest
                                                     └▶ bd burn ──▶ (gone)
```

## Core Concepts

| Term | Description |
|------|-------------|
| **Formula** | Source TOML template defining workflow steps |
| **Protomolecule** | Frozen template ready for instantiation |
| **Molecule** | Active workflow instance with trackable steps |
| **Wisp** | Ephemeral molecule for patrol cycles (never synced) |
| **Digest** | Squashed summary of completed molecule |

## Common Mistake: Reading Formulas Directly

**WRONG:**
```bash
# Reading a formula file and manually creating beads for each step
cat .beads/formulas/mol-polecat-work.formula.toml
bd create --title "Step 1: Load context" --type task
bd create --title "Step 2: Branch setup" --type task
# ... creating beads from formula prose
```

**RIGHT:**
```bash
# Cook the formula into a proto, pour into a molecule
bd cook mol-polecat-work
bd mol pour mol-polecat-work --var issue=gt-xyz
# Now work through the step beads that were created
bd ready                    # Find next step
bd close <step-id>          # Complete it
```

**Key insight:** Formulas are source templates (like source code). You never read
them directly during work. The `cook` → `pour` pipeline creates step beads for you.
Your molecule already has steps - use `bd ready` to find them.

## Navigating Molecules

Molecules help you track where you are in multi-step workflows.

### Finding Your Place

```bash
bd mol current              # Where am I?
bd mol current gt-abc       # Status of specific molecule
```

Output:
```
You're working on molecule gt-abc (Feature X)

  ✓ gt-abc.1: Design
  ✓ gt-abc.2: Scaffold
  ✓ gt-abc.3: Implement
  → gt-abc.4: Write tests [in_progress] <- YOU ARE HERE
  ○ gt-abc.5: Documentation
  ○ gt-abc.6: Exit decision

Progress: 3/6 steps complete
```

### Seamless Transitions

Close a step and advance in one command:

```bash
bd close gt-abc.3 --continue   # Close and advance to next step
bd close gt-abc.3 --no-auto    # Close but don't auto-claim next
```

**The old way (3 commands):**
```bash
bd close gt-abc.3
bd ready --parent=gt-abc
bd update gt-abc.4 --status=in_progress
```

**The new way (1 command):**
```bash
bd close gt-abc.3 --continue
```

### Transition Output

```
✓ Closed gt-abc.3: Implement feature

Next ready in molecule:
  gt-abc.4: Write tests

→ Marked in_progress (use --no-auto to skip)
```

### When Molecule Completes

```
✓ Closed gt-abc.6: Exit decision

Molecule gt-abc complete! All steps closed.
Consider: bd mol squash gt-abc --summary '...'
```

## Molecule Commands

### Beads Operations (bd)

```bash
# Formulas
bd formula list              # Available formulas
bd formula show <name>       # Formula details
bd cook <formula>            # Formula → Proto

# Molecules (data operations)
bd mol list                  # Available protos
bd mol show <id>             # Proto details
bd mol pour <proto>          # Create mol
bd mol wisp <proto>          # Create wisp
bd mol bond <proto> <parent> # Attach to existing mol
bd mol squash <id>           # Condense to digest
bd mol burn <id>             # Discard wisp
bd mol current               # Where am I in the current molecule?
```

### Agent Operations (gt)

```bash
# Hook management
gt hook                    # What's on MY hook
gt mol current               # What should I work on next
gt mol progress <id>         # Execution progress of molecule
gt mol attach <bead> <mol>   # Pin molecule to bead
gt mol detach <bead>         # Unpin molecule from bead

# Agent lifecycle
gt mol burn                  # Burn attached molecule
gt mol squash                # Squash attached molecule
gt mol step done <step>      # Complete a molecule step
```

## Polecat Workflow

Polecats receive work via their hook - a pinned molecule attached to an issue.
They execute molecule steps sequentially, closing each step as they complete it.

### Molecule Types for Polecats

| Type | Storage | Use Case |
|------|---------|----------|
| **Regular Molecule** | `.beads/` (synced) | Discrete deliverables, audit trail |
| **Wisp** | `.beads/` (ephemeral) | Patrol cycles, operational loops |

Polecats typically use **regular molecules** because each assignment has audit value.
Patrol agents (Witness, Refinery, Deacon) use **wisps** to prevent accumulation.

### Hook Management

```bash
gt hook                        # What's on MY hook?
gt mol attach-from-mail <id>   # Attach work from mail message
gt done                        # Signal completion (syncs, submits to MQ, notifies Witness)
```

### Polecat Workflow Summary

```
1. Spawn with work on hook
2. gt hook                 # What's hooked?
3. bd mol current          # Where am I?
4. Execute current step
5. gt mol step done <step> # Close and auto-advance to next
6. If more steps: GOTO 3
7. gt done                 # Signal completion
```

### Wisp vs Molecule Decision

| Question | Molecule | Wisp |
|----------|----------|------|
| Does it need audit trail? | Yes | No |
| Will it repeat continuously? | No | Yes |
| Is it discrete deliverable? | Yes | No |
| Is it operational routine? | No | Yes |

## Best Practices

1. **Use `gt mol step done` for propulsion** - Auto-advances with dependency resolution
2. **Check progress with `bd mol current`** - Know where you are before resuming
3. **Squash completed molecules** - Create digests for audit trail
4. **Burn routine wisps** - Don't accumulate ephemeral patrol data
