# Swarm (Ephemeral Worker View)

> **Note**: "Swarm" is an ephemeral concept, not a persistent entity.
> For tracking work, see [Convoys](convoy.md).

## What is a Swarm?

A **swarm** is simply "the workers currently assigned to a convoy's issues."
It has no separate ID and no persistent state - it's just a view of active workers.

| Concept | Persistent? | ID | Description |
|---------|-------------|-----|-------------|
| **Convoy** | Yes | hq-* | The tracking unit. What you create and track. |
| **Swarm** | No | None | The workers. Ephemeral view of who's working. |

## The Relationship

```
Convoy hq-abc ─────────tracks───────────► Issues
                                            │
                                            │ assigned to
                                            ▼
                                         Polecats
                                            │
                                    ────────┴────────
                                    "the swarm"
                                    (ephemeral)
```

When you say "kick off a swarm," you're really:
1. Creating a convoy (persistent tracking)
2. Assigning polecats to the convoy's issues
3. The swarm = those polecats while they work

When the work completes, the convoy lands and the swarm dissolves.

## Viewing the Swarm

The swarm appears in convoy status:

```bash
gt convoy status hq-abc
```

```
Convoy: hq-abc (Deploy v2.0)
════════════════════════════

Progress: 2/3 complete

Issues
  ✓ gt-xyz: Update API              closed
  → bd-ghi: Update docs             in_progress  @beads/amber
  ○ gt-jkl: Final review            open

Workers (the swarm)          ← this is the swarm
  beads/amber     bd-ghi     running 12m
```

## Historical Note

Earlier Gas Town development used "swarm" as if it were a persistent entity
with its own lifecycle. The `gt swarm` commands were built on this model.

The correct model is:
- **Convoy** = the persistent tracking unit (what `gt swarm` was trying to be)
- **Swarm** = ephemeral workers (no separate tracking needed)

The `gt swarm` command is being deprecated in favor of `gt convoy`.

## See Also

- [Convoys](convoy.md) - The persistent tracking unit
- [Propulsion Principle](propulsion-principle.md) - Worker execution model
