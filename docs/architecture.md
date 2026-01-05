# Gas Town Architecture

Technical architecture for Gas Town multi-agent workspace management.

## Two-Level Beads Architecture

Gas Town uses a two-level beads architecture to separate organizational coordination
from project implementation work.

| Level | Location | Prefix | Purpose |
|-------|----------|--------|---------|
| **Town** | `~/gt/.beads/` | `hq-*` | Cross-rig coordination, Mayor mail, agent identity |
| **Rig** | `<rig>/mayor/rig/.beads/` | project prefix | Implementation work, MRs, project issues |

### Town-Level Beads (`~/gt/.beads/`)

Organizational chain for cross-rig coordination:
- Mayor mail and messages
- Convoy coordination (batch work across rigs)
- Strategic issues and decisions
- **Town-level agent beads** (Mayor, Deacon)
- **Role definition beads** (global templates)

### Rig-Level Beads (`<rig>/mayor/rig/.beads/`)

Project chain for implementation work:
- Bugs, features, tasks for the project
- Merge requests and code reviews
- Project-specific molecules
- **Rig-level agent beads** (Witness, Refinery, Polecats)

## Agent Bead Storage

Agent beads track lifecycle state for each agent. Storage location depends on
the agent's scope.

| Agent Type | Scope | Bead Location | Bead ID Format |
|------------|-------|---------------|----------------|
| Mayor | Town | `~/gt/.beads/` | `hq-mayor` |
| Deacon | Town | `~/gt/.beads/` | `hq-deacon` |
| Dogs | Town | `~/gt/.beads/` | `hq-dog-<name>` |
| Witness | Rig | `<rig>/.beads/` | `<prefix>-<rig>-witness` |
| Refinery | Rig | `<rig>/.beads/` | `<prefix>-<rig>-refinery` |
| Polecats | Rig | `<rig>/.beads/` | `<prefix>-<rig>-polecat-<name>` |

### Role Beads

Role beads are global templates stored in town beads with `hq-` prefix:
- `hq-mayor-role` - Mayor role definition
- `hq-deacon-role` - Deacon role definition
- `hq-witness-role` - Witness role definition
- `hq-refinery-role` - Refinery role definition
- `hq-polecat-role` - Polecat role definition

Each agent bead references its role bead via the `role_bead` field.

## Agent Taxonomy

### Town-Level Agents (Cross-Rig)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Mayor** | Global coordinator, handles cross-rig communication and escalations | Persistent |
| **Deacon** | Daemon beacon - receives heartbeats, runs plugins and monitoring | Persistent |
| **Dogs** | Long-running workers for cross-rig batch work | Variable |

### Rig-Level Agents (Per-Project)

| Agent | Role | Persistence |
|-------|------|-------------|
| **Witness** | Monitors polecat health, handles nudging and cleanup | Persistent |
| **Refinery** | Processes merge queue, runs verification | Persistent |
| **Polecats** | Ephemeral workers assigned to specific issues | Ephemeral |

## Directory Structure

```
~/gt/                           Town root
├── .beads/                     Town-level beads (hq-* prefix)
│   ├── config.yaml             Beads configuration
│   ├── issues.jsonl            Town issues (mail, agents, convoys)
│   └── routes.jsonl            Prefix → rig routing table
├── mayor/                      Mayor config
│   └── town.json               Town configuration
└── <rig>/                      Project container (NOT a git clone)
    ├── config.json             Rig identity and beads prefix
    ├── .beads/ → mayor/rig/.beads  Symlink to canonical beads
    ├── .repo.git/              Bare repo (shared by worktrees)
    ├── mayor/rig/              Mayor's clone (canonical beads)
    ├── refinery/rig/           Worktree on main
    ├── witness/                No clone (monitors only)
    ├── crew/<name>/            Human workspaces
    └── polecats/<name>/        Worker worktrees
```

## Beads Routing

The `routes.jsonl` file maps issue ID prefixes to their storage locations:

```jsonl
{"prefix":"hq","path":"/Users/stevey/gt/.beads"}
{"prefix":"gt","path":"/Users/stevey/gt/gastown/mayor/rig/.beads"}
```

This enables transparent cross-rig beads operations:

```bash
bd show hq-mayor    # Routes to town beads
bd show gt-xyz      # Routes to gastown rig beads
```

## See Also

- [reference.md](reference.md) - Command reference
- [molecules.md](molecules.md) - Workflow molecules
- [identity.md](identity.md) - Agent identity and BD_ACTOR
