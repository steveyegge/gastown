# Federation Architecture

> **Status: Design spec - not yet implemented**

> Multi-workspace coordination for Gas Town and Beads

## Overview

Federation enables multiple Gas Town instances to reference each other's work,
coordinate across organizations, and track distributed projects.

## Why Federation?

Real enterprise projects don't live in a single repo:

- **Microservices:** 50 repos, tight dependencies, coordinated releases
- **Platform teams:** Shared libraries used by dozens of downstream projects
- **Contractors:** External teams working on components you need to track
- **Acquisitions:** New codebases that need to integrate with existing work

Traditional tools force you to choose: unified tracking (monorepo) or team
autonomy (multi-repo with fragmented visibility). Federation provides both:
each workspace is autonomous, but cross-workspace references are first-class.

## Entity Model

### Three Levels

```
Level 1: Entity    - Person or organization (flat namespace)
Level 2: Chain     - Workspace/town per entity
Level 3: Work Unit - Issues, tasks, molecules on chains
```

### URI Scheme

Full work unit reference (HOP protocol):

```
hop://entity/chain/rig/issue-id
hop://steve@example.com/main-town/greenplace/gp-xyz
```

Cross-repo reference (same platform):

```
beads://platform/org/repo/issue-id
beads://github/acme/backend/ac-123
```

Within a workspace, short forms are preferred:

```
gp-xyz             # Local (prefix routes via routes.jsonl)
greenplace/gp-xyz  # Different rig, same chain
./gp-xyz           # Explicit current-rig ref
```

See `~/gt/docs/hop/GRAPH-ARCHITECTURE.md` for full URI specification.

## Relationship Types

### Employment

Track which entities belong to organizations:

```json
{
  "type": "employment",
  "entity": "alice@example.com",
  "organization": "acme.com"
}
```

### Cross-Reference

Reference work in another workspace:

```json
{
  "references": [
    {
      "type": "depends_on",
      "target": "hop://other-entity/chain/rig/issue-id"
    }
  ]
}
```

### Delegation

Distribute work across workspaces:

```json
{
  "type": "delegation",
  "parent": "hop://acme.com/projects/proj-123",
  "child": "hop://alice@example.com/town/greenplace/gp-xyz",
  "terms": { "portion": "backend", "deadline": "2025-02-01" }
}
```

## Agent Provenance

Every agent operation is attributed. See [identity.md](../concepts/identity.md) for the
complete BD_ACTOR format convention.

### Git Commits

```bash
# Set per agent session
GIT_AUTHOR_NAME="greenplace/crew/joe"
GIT_AUTHOR_EMAIL="steve@example.com"  # Workspace owner
```

Result: `abc123 Fix bug (greenplace/crew/joe <steve@example.com>)`

### Beads Operations

```bash
BD_ACTOR="greenplace/crew/joe"  # Set in agent environment
bd create --title="Task"        # Actor auto-populated
```

### Event Logging

All events include actor:

```json
{
  "ts": "2025-01-15T10:30:00Z",
  "type": "sling",
  "actor": "greenplace/crew/joe",
  "payload": { "bead": "gp-xyz", "target": "greenplace/polecats/Toast" }
}
```

## Discovery

### Workspace Metadata

Each workspace has identity metadata:

```json
// ~/gt/.town.json
{
  "owner": "steve@example.com",
  "name": "main-town",
  "public_name": "steve-greenplace"
}
```

### Remote Registration

```bash
gt remote add acme hop://acme.com/engineering
gt remote list
```

### Cross-Workspace Queries

```bash
bd show hop://acme.com/eng/ac-123    # Fetch remote issue
bd list --remote=acme                # List remote issues
```

## Aggregation

Query across relationships without hierarchy:

```bash
# All work by org members
bd list --org=acme.com

# All work on a project (including delegated)
bd list --project=proj-123 --include-delegated

# Agent's full history
bd audit --actor=greenplace/crew/joe
```

## Implementation Status

- [x] Agent identity in git commits
- [x] BD_ACTOR default in beads create
- [x] Workspace metadata file (.town.json)
- [x] Cross-workspace URI scheme (hop://, beads://, local forms)
- [ ] Remote registration
- [ ] Cross-workspace queries
- [ ] Delegation primitives

## Use Cases

### Multi-Repo Projects

Track work spanning multiple repositories:

```
Project X
├── hop://team/frontend/fe-123
├── hop://team/backend/be-456
└── hop://team/infra/inf-789
```

### Distributed Teams

Team members in different workspaces:

```
Alice's Town → works on → Project X
Bob's Town   → works on → Project X
```

Each maintains their own CV/audit trail.

### Contractor Coordination

Prime contractor delegates to subcontractors:

```
Acme/Project
└── delegates to → Vendor/SubProject
                   └── delegates to → Contractor/Task
```

Completion cascades up. Attribution preserved.

## Design Principles

1. **Flat namespace** - Entities not nested, relationships connect them
2. **Relationships over hierarchy** - Graph structure, not tree
3. **Git-native** - Federation uses git mechanics (remotes, refs)
4. **Incremental** - Works standalone, gains power with federation
5. **Privacy-preserving** - Each entity controls their chain visibility

## Enterprise Benefits

| Challenge | Without Federation | With Federation |
|-----------|-------------------|-----------------|
| Cross-repo dependencies | "Check with backend team" | Explicit dependency tracking |
| Contractor visibility | Email updates, status calls | Live status, same tooling |
| Release coordination | Spreadsheets, Slack threads | Unified timeline view |
| Agent attribution | Per-repo, fragmented | Cross-workspace CV |
| Compliance audit | Stitch together logs | Query across workspaces |

Federation isn't just about connecting repos - it's about treating distributed
engineering as a first-class concern, with the same visibility and tooling
you'd expect from a monorepo, while preserving team autonomy.
