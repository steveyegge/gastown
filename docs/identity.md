# Agent Identity and Attribution

> Canonical format for agent identity in Gas Town

## Why Identity Matters

When you deploy AI agents at scale, anonymous work creates real problems:

- **Debugging:** "The AI broke it" isn't actionable. *Which* AI?
- **Quality tracking:** You can't improve what you can't measure.
- **Compliance:** Auditors ask "who approved this code?" - you need an answer.
- **Performance management:** Some agents are better than others at certain tasks.

Gas Town solves this with **universal attribution**: every action, every commit,
every bead update is linked to a specific agent identity. This enables work
history tracking, capability-based routing, and objective quality measurement.

## BD_ACTOR Format Convention

The `BD_ACTOR` environment variable identifies agents in slash-separated path format.
This is set automatically when agents are spawned and used for all attribution.

### Format by Role Type

| Role Type | Format | Example |
|-----------|--------|---------|
| **Mayor** | `mayor` | `mayor` |
| **Deacon** | `deacon` | `deacon` |
| **Witness** | `{rig}/witness` | `gastown/witness` |
| **Refinery** | `{rig}/refinery` | `gastown/refinery` |
| **Crew** | `{rig}/crew/{name}` | `gastown/crew/joe` |
| **Polecat** | `{rig}/polecats/{name}` | `gastown/polecats/toast` |

### Why Slashes?

The slash format mirrors filesystem paths and enables:
- Hierarchical parsing (extract rig, role, name)
- Consistent mail addressing (`gt mail send gastown/witness`)
- Path-like routing in beads operations
- Visual clarity about agent location

## Attribution Model

Gas Town uses three fields for complete provenance:

### Git Commits

```bash
GIT_AUTHOR_NAME="gastown/crew/joe"      # Who did the work (agent)
GIT_AUTHOR_EMAIL="steve@example.com"    # Who owns the work (overseer)
```

Result in git log:
```
abc123 Fix bug (gastown/crew/joe <steve@example.com>)
```

**Interpretation**:
- The agent `gastown/crew/joe` authored the change
- The work belongs to the workspace owner (`steve@example.com`)
- Both are preserved in git history forever

### Beads Records

```json
{
  "id": "gt-xyz",
  "created_by": "gastown/crew/joe",
  "updated_by": "gastown/witness"
}
```

The `created_by` field is populated from `BD_ACTOR` when creating beads.
The `updated_by` field tracks who last modified the record.

### Event Logging

All events include actor attribution:

```json
{
  "ts": "2025-01-15T10:30:00Z",
  "type": "sling",
  "actor": "gastown/crew/joe",
  "payload": { "bead": "gt-xyz", "target": "gastown/polecats/toast" }
}
```

## Environment Setup

Gas Town uses a centralized `config.AgentEnv()` function to set environment
variables consistently across all agent spawn paths (managers, daemon, boot).

### Example: Polecat Environment

```bash
# Set automatically for polecat 'toast' in rig 'gastown'
export GT_ROLE="polecat"
export GT_RIG="gastown"
export GT_POLECAT="toast"
export BD_ACTOR="gastown/polecats/toast"
export GIT_AUTHOR_NAME="gastown/polecats/toast"
export GT_ROOT="/home/user/gt"
export BEADS_DIR="/home/user/gt/gastown/.beads"
export BEADS_AGENT_NAME="gastown/toast"
export BEADS_NO_DAEMON="1"  # Polecats use isolated beads context
```

### Example: Crew Environment

```bash
# Set automatically for crew member 'joe' in rig 'gastown'
export GT_ROLE="crew"
export GT_RIG="gastown"
export GT_CREW="joe"
export BD_ACTOR="gastown/crew/joe"
export GIT_AUTHOR_NAME="gastown/crew/joe"
export GT_ROOT="/home/user/gt"
export BEADS_DIR="/home/user/gt/gastown/.beads"
export BEADS_AGENT_NAME="gastown/joe"
export BEADS_NO_DAEMON="1"  # Crew uses isolated beads context
```

### Manual Override

For local testing or debugging:

```bash
export BD_ACTOR="gastown/crew/debug"
bd create --title="Test issue"  # Will show created_by: gastown/crew/debug
```

See [reference.md](reference.md#environment-variables) for the complete
environment variable reference.

## Identity Parsing

The format supports programmatic parsing:

```go
// identityToBDActor converts daemon identity to BD_ACTOR format
// Town level: mayor, deacon
// Rig level: {rig}/witness, {rig}/refinery
// Workers: {rig}/crew/{name}, {rig}/polecats/{name}
```

| Input | Parsed Components |
|-------|-------------------|
| `mayor` | role=mayor |
| `deacon` | role=deacon |
| `gastown/witness` | rig=gastown, role=witness |
| `gastown/refinery` | rig=gastown, role=refinery |
| `gastown/crew/joe` | rig=gastown, role=crew, name=joe |
| `gastown/polecats/toast` | rig=gastown, role=polecat, name=toast |

## Audit Queries

Attribution enables powerful audit queries:

```bash
# All work by an agent
bd audit --actor=gastown/crew/joe

# All work in a rig
bd audit --actor=gastown/*

# All polecat work
bd audit --actor=*/polecats/*

# Git history by agent
git log --author="gastown/crew/joe"
```

## Design Principles

1. **Agents are not anonymous** - Every action is attributed
2. **Work is owned, not authored** - Agent creates, overseer owns
3. **Attribution is permanent** - Git commits preserve history
4. **Format is parseable** - Enables programmatic analysis
5. **Consistent across systems** - Same format in git, beads, events

## CV and Skill Accumulation

### Human Identity is Global

The global identifier is your **email** - it's already in every git commit. No separate "entity bead" needed.

```
steve@example.com                ← global identity (from git author)
├── Town A (home)                ← workspace
│   ├── gastown/crew/joe         ← agent executor
│   └── gastown/polecats/toast   ← agent executor
└── Town B (work)                ← workspace
    └── acme/polecats/nux        ← agent executor
```

### Agent vs Owner

| Field | Scope | Purpose |
|-------|-------|---------|
| `BD_ACTOR` | Local (town) | Agent attribution for debugging |
| `GIT_AUTHOR_EMAIL` | Global | Human identity for CV |
| `created_by` | Local | Who created the bead |
| `owner` | Global | Who owns the work |

**Agents execute. Humans own.** The polecat name in `completed-by: gastown/polecats/toast` is executor attribution. The CV credits the human owner (`steve@example.com`).

### Polecats Are Ephemeral

Polecats are like K8s pods - ephemeral executors with no persistent identity:
- Named pool for human convenience (furiosa, nux, slit)
- Names are transient - reused after cleanup
- No persistent polecat CV
- Work credits the human owner

### Skills Are Derived

Your CV emerges from querying work evidence:

```bash
# All work by owner (across all agents)
git log --author="steve@example.com"
bd list --owner=steve@example.com

# Skills derived from evidence
# - .go files touched → Go skill
# - issue tags → domain skills
# - commit patterns → activity types
```

### Multi-Town Aggregation

A human with multiple towns has one CV:

```bash
# Future: federated CV query
bd cv steve@example.com
# Discovers all towns, aggregates work, derives skills
```

See `~/gt/docs/hop/decisions/008-identity-model.md` for architectural rationale.

## Enterprise Use Cases

### Compliance and Audit

```bash
# Who touched this file in the last 90 days?
git log --since="90 days ago" -- path/to/sensitive/file.go

# All changes by a specific agent
bd audit --actor=gastown/polecats/toast --since=2025-01-01
```

### Performance Tracking

```bash
# Completion rate by agent
bd stats --group-by=actor

# Average time to completion
bd stats --actor=gastown/polecats/* --metric=cycle-time
```

### Model Comparison

When agents use different underlying models, attribution enables A/B comparison:

```bash
# Tag agents by model
# gastown/polecats/claude-1 uses Claude
# gastown/polecats/gpt-1 uses GPT-4

# Compare quality signals
bd stats --actor=gastown/polecats/claude-* --metric=revision-count
bd stats --actor=gastown/polecats/gpt-* --metric=revision-count
```

Lower revision counts suggest higher first-pass quality.
