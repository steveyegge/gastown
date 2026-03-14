# Convoys

Convoys are the primary unit for tracking batched work across rigs.

## Quick Start

```bash
# Create a convoy tracking some issues
gt convoy create "Feature X" gt-abc gt-def --notify overseer

# Check progress
gt convoy status hq-cv-abc

# List active convoys (the dashboard)
gt convoy list

# See all convoys including landed ones
gt convoy list --all
```

## Concept

A **convoy** is a persistent tracking unit that monitors related issues across
multiple rigs. When you kick off work - even a single issue - a convoy tracks it
so you can see when it lands and what was included.

```
                 🚚 Convoy (hq-cv-abc)
                         │
            ┌────────────┼────────────┐
            │            │            │
            ▼            ▼            ▼
       ┌─────────┐  ┌─────────┐  ┌─────────┐
       │ gt-xyz  │  │ gt-def  │  │ bd-abc  │
       │ gastown │  │ gastown │  │  beads  │
       └────┬────┘  └────┬────┘  └────┬────┘
            │            │            │
            ▼            ▼            ▼
       ┌─────────┐  ┌─────────┐  ┌─────────┐
       │  nux    │  │ furiosa │  │  amber  │
       │(polecat)│  │(polecat)│  │(polecat)│
       └─────────┘  └─────────┘  └─────────┘
                         │
                    "the swarm"
                    (ephemeral)
```

## Convoy vs Swarm

| Concept | Persistent? | ID | Description |
|---------|-------------|-----|-------------|
| **Convoy** | Yes | hq-cv-* | Tracking unit. What you create, track, get notified about. |
| **Swarm** | No | None | Ephemeral. "The workers currently on this convoy's issues." |
| **Stranded Convoy** | Yes | hq-cv-* | A convoy with ready work but no polecats assigned. Needs attention. |

When you "kick off a swarm", you're really:
1. Creating a convoy (the tracking unit)
2. Assigning polecats to the tracked issues
3. The "swarm" is just those polecats while they're working

When issues close, the convoy lands and notifies you. The swarm dissolves.

## Convoy Lifecycle

```
OPEN ──(all issues close)──► LANDED/CLOSED
  ↑                              │
  └──(add more issues)───────────┘
       (auto-reopens)
```

| State | Description |
|-------|-------------|
| `open` | Active tracking, work in progress |
| `closed` | All tracked issues closed, notification sent |

Adding issues to a closed convoy reopens it automatically.

## Commands

### Create a Convoy

```bash
# Track multiple issues across rigs
gt convoy create "Deploy v2.0" gt-abc bd-xyz --notify gastown/joe

# Track a single issue (still creates convoy for dashboard visibility)
gt convoy create "Fix auth bug" gt-auth-fix

# With default notification (from config)
gt convoy create "Feature X" gt-a gt-b gt-c
```

### Add Issues

```bash
# Add issues to existing convoy
gt convoy add hq-cv-abc gt-new-issue
gt convoy add hq-cv-abc gt-issue1 gt-issue2 gt-issue3

# Adding to closed convoy requires reopening first
bd update hq-cv-abc --status=open
gt convoy add hq-cv-abc gt-followup-fix
```

### Check Status

```bash
# Show issues and active workers (the swarm)
gt convoy status hq-abc

# All active convoys (the dashboard)
gt convoy status
```

Example output:
```
🚚 hq-cv-abc: Deploy v2.0

  Status:    ●
  Progress:  2/4 completed
  Created:   2025-12-30T10:15:00-08:00

  Tracked Issues:
    ✓ gt-xyz: Update API endpoint [task]
    ✓ bd-abc: Fix validation [bug]
    ○ bd-ghi: Update docs [task]
    ○ gt-jkl: Deploy to prod [task]
```

### List Convoys (Dashboard)

```bash
# Active convoys (default) - the primary attention view
gt convoy list

# All convoys including landed
gt convoy list --all

# Only landed convoys
gt convoy list --status=closed

# JSON output
gt convoy list --json
```

Example output:
```
Convoys

  🚚 hq-cv-w3nm6: Feature X ●
  🚚 hq-cv-abc12: Bug fixes ●

Use 'gt convoy status <id>' for detailed view.
```

## Notifications

When a convoy lands (all tracked issues closed), subscribers are notified:

```bash
# Explicit subscriber
gt convoy create "Feature X" gt-abc --notify gastown/joe

# Multiple subscribers
gt convoy create "Feature X" gt-abc --notify mayor/ --notify --human
```

Notification content:
```
🚚 Convoy Landed: Deploy v2.0 (hq-cv-abc)

Issues (3):
  ✓ gt-xyz: Update API endpoint
  ✓ gt-def: Add validation
  ✓ bd-abc: Update docs

Duration: 2h 15m
```

## Create from Epic

Auto-discover tracked issues from an existing epic's children. Useful when
a planning/decomposition tool has already structured work as an epic with
child implementation beads.

```bash
# Auto-discover children from epic
gt convoy create --from-epic gt-epic-abc

# Override the convoy name (defaults to epic title)
gt convoy create --from-epic gt-epic-abc "Custom convoy name"

# Combine with other flags
gt convoy create --from-epic gt-epic-abc --owned --merge=direct
```

**How it works:**
1. Verifies the given bead is an epic
2. BFS-walks the parent-child hierarchy to find slingable descendants
3. Creates a standard convoy (`hq-cv-*`) tracking all slingable children (task, bug, feature, chore)

Non-slingable types (sub-epics, decisions) are recursed into but never
tracked directly. Only leaf work items appear in the convoy.

## Auto-Convoy on Sling

When you sling a single issue without an existing convoy:

```bash
gt sling bd-xyz beads/amber
```

This auto-creates a convoy so all work appears in the dashboard:
1. Creates convoy: "Work: bd-xyz"
2. Tracks the issue
3. Assigns the polecat

Even "swarm of one" gets convoy visibility.

## Cross-Rig Tracking

Convoys live in town-level beads (`hq-cv-*` prefix) and can track issues from any rig:

```bash
# Track issues from multiple rigs
gt convoy create "Full-stack feature" \
  gt-frontend-abc \
  gt-backend-def \
  bd-docs-xyz
```

The `tracks` relation is:
- **Non-blocking**: doesn't affect issue workflow
- **Additive**: can add issues anytime
- **Cross-rig**: convoy in hq-*, issues in gt-*, bd-*, etc.

## Convoy vs Rig Status

| View | Scope | Shows |
|------|-------|-------|
| `gt convoy status [id]` | Cross-rig | Issues tracked by convoy + workers |
| `gt rig status <rig>` | Single rig | All workers in rig + their convoy membership |

Use convoys for "what's the status of this batch of work?"
Use rig status for "what's everyone in this rig working on?"

## See Also

- [Propulsion Principle](propulsion-principle.md) - Worker execution model
- [Mail Protocol](../design/mail-protocol.md) - Notification delivery
