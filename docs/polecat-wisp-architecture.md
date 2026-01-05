# Polecat Wisp Architecture

How polecats use molecules and wisps to execute work in Gas Town.

## Overview

Polecats receive work via their hook - a pinned molecule attached to an issue.
They execute molecule steps sequentially, closing each step as they complete it.

## Molecule Types for Polecats

| Type | Storage | Use Case |
|------|---------|----------|
| **Regular Molecule** | `.beads/` (synced) | Discrete deliverables, audit trail |
| **Wisp** | `.beads/` (ephemeral, type=wisp) | Patrol cycles, operational loops |

Polecats typically use **regular molecules** because each assignment has audit value.
Patrol agents (Witness, Refinery, Deacon) use **wisps** to prevent accumulation.

## Step Execution

### The Traditional Approach

```bash
# 1. Check current status
gt hook

# 2. Find next step
bd ready --parent=gt-abc

# 3. Claim the step
bd update gt-abc.4 --status=in_progress

# 4. Do the work...

# 5. Close the step
bd close gt-abc.4

# 6. Repeat from step 2
```

### The Propulsion Approach

```bash
# 1. Check where you are
bd mol current

# 2. Do the work on current step...

# 3. Close and advance in one command
bd close gt-abc.4 --continue

# 4. Repeat from step 1
```

The `--continue` flag:
- Closes the current step
- Finds the next ready step in the same molecule
- Auto-marks it `in_progress`
- Outputs the transition

### Example Session

```bash
$ bd mol current
You're working on molecule gt-abc (Implement user auth)

  ✓ gt-abc.1: Design schema
  ✓ gt-abc.2: Create models
  → gt-abc.3: Add endpoints [in_progress] <- YOU ARE HERE
  ○ gt-abc.4: Write tests
  ○ gt-abc.5: Update docs

Progress: 2/5 steps complete

$ # ... implement the endpoints ...

$ bd close gt-abc.3 --continue
✓ Closed gt-abc.3: Add endpoints

Next ready in molecule:
  gt-abc.4: Write tests

→ Marked in_progress (use --no-auto to skip)

$ bd mol current
You're working on molecule gt-abc (Implement user auth)

  ✓ gt-abc.1: Design schema
  ✓ gt-abc.2: Create models
  ✓ gt-abc.3: Add endpoints
  → gt-abc.4: Write tests [in_progress] <- YOU ARE HERE
  ○ gt-abc.5: Update docs

Progress: 3/5 steps complete
```

## Molecule Completion

When closing the last step:

```bash
$ bd close gt-abc.5 --continue
✓ Closed gt-abc.5: Update docs

Molecule gt-abc complete! All steps closed.
Consider: bd mol squash gt-abc --summary '...'
```

After all steps are closed:

```bash
# Squash to digest for audit trail
bd mol squash gt-abc --summary "Implemented user authentication with JWT"

# Or if it's routine work
bd mol burn gt-abc
```

## Hook Management

### Checking Your Hook

```bash
gt hook
```

Shows what molecule is pinned to your current agent and the associated bead.

### Attaching Work from Mail

```bash
gt mail inbox
gt mol attach-from-mail <mail-id>
```

### Completing Work

```bash
# After all molecule steps closed
gt done

# This:
# 1. Syncs beads
# 2. Submits to merge queue
# 3. Notifies Witness
```

## Polecat Workflow Summary

```
1. Spawn with work on hook
2. gt hook           # What's hooked?
3. bd mol current          # Where am I?
4. Execute current step
5. bd close <step> --continue
6. If more steps: GOTO 3
7. gt done                 # Signal completion
8. Wait for Witness cleanup
```

## Wisp vs Molecule Decision

| Question | Molecule | Wisp |
|----------|----------|------|
| Does it need audit trail? | Yes | No |
| Will it repeat continuously? | No | Yes |
| Is it discrete deliverable? | Yes | No |
| Is it operational routine? | No | Yes |

Polecats: **Use molecules** (deliverables have audit value)
Patrol agents: **Use wisps** (routine loops don't accumulate)
