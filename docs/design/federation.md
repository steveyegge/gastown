# Federation Architecture

> **Status: Partially implemented** -- Infrastructure (Dolt remotes) exists. Core federation features (URI scheme, cross-workspace queries, delegation) are not yet implemented.

Multi-workspace coordination for Gas Town and Beads.

## Overview

Federation enables multiple Gas Town instances to reference each other's work,
coordinate across organizations, and track distributed projects.

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

## Relationship Types (not yet implemented)

Planned relationship primitives: **employment** (entity-to-org membership),
**cross-reference** (inter-workspace `depends_on` links), and **delegation**
(work distribution across workspaces with terms and deadlines).

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

## Discovery (not yet implemented)

Workspace metadata lives in `~/gt/.town.json` (owner, name, public_name).
Planned commands: `gt remote add/list` for remote registration,
`bd show hop://...` and `bd list --remote=...` for cross-workspace queries.

## Implementation Status

- [x] Agent identity in git commits
- [x] BD_ACTOR default in beads create
- [x] Workspace metadata file (.town.json)
- [x] Cross-workspace URI scheme (hop://, beads://, local forms)
- [x] Dolt remotes configured (DoltHub endpoints)
- [x] Local remotesapi enabled (port 8000)
- [ ] DoltHub authentication (`dolt login`)
- [ ] Remote registration (gt remote add)
- [ ] Cross-workspace queries
- [ ] Delegation primitives

## Dolt Federation Configuration

### Current Setup

Town-level Dolt databases have remotes configured pointing to DoltHub:

```bash
# Check configured remotes for town database
cd ~/gt/.dolt-data/town && dolt remote -v
# origin https://doltremoteapi.dolthub.com/steveyegge/gastown-town {}
# local  http://localhost:8000/town {}
```

### Configured Remotes

| Database | Remote Name | URL | Purpose |
|----------|-------------|-----|---------|
| town | origin | `steveyegge/gastown-town` | DoltHub public federation |
| town | local | `http://localhost:8000/town` | Local development/testing |
| gastown | origin | `steveyegge/gastown-rig` | DoltHub public federation |
| beads | origin | `steveyegge/gastown-beads` | DoltHub public federation |

### Federation Endpoint Options

**1. DoltHub (Recommended for Public Federation)**

Like GitHub for Dolt - public, hosted, zero infrastructure:

```bash
# Login to DoltHub (one-time setup)
dolt login

# Push to remote
cd ~/gt/.dolt-data/town
dolt push origin main
```

**2. Local Remotesapi (Development/Testing)**

Already enabled in `~/gt/.dolt-data/config.yaml`:
- Port: 8000
- Mode: read-only (set `read_only: false` for full federation)

```bash
# Test local remote
dolt push local main
```

**3. Self-Hosted DoltLab (Enterprise)**

For private federation within an organization:
- Deploy DoltLab instance
- Configure remote: `dolt remote add corp https://doltlab.corp.example.com/org/repo`

**4. Direct Town-to-Town (Advanced)**

Two Gas Town instances federating directly:
- Town A runs remotesapi on accessible endpoint
- Town B adds Town A as remote: `dolt remote add town-a http://town-a.example.com:8000/town`

### Enabling Full Federation

To push/pull from configured remotes:

1. **DoltHub Authentication:**
   ```bash
   dolt login
   # Opens browser for OAuth
   # Creates credentials in ~/.dolt/creds/
   ```

2. **Create DoltHub Repository:**
   - Visit https://www.dolthub.com
   - Create repository matching remote name (e.g., `steveyegge/gastown-town`)

3. **Initial Push:**
   ```bash
   cd ~/gt/.dolt-data/town
   dolt push -u origin main
   ```

4. **Enable Write for Local Remotesapi:**
   Edit `~/gt/.dolt-data/config.yaml`:
   ```yaml
   remotesapi:
     port: 8000
     read_only: false  # Enable writes
   ```
   Restart daemon: `gt down && gt up`

### Security Considerations

- **DoltHub**: Public by default; use private repos for sensitive data
- **Local remotesapi**: Bind to localhost only; use TLS for network access
- **Authentication**: DoltHub uses OAuth; self-hosted can use TLS client certs

## Future Use Cases

- **Multi-repo projects**: Track work spanning multiple repositories with cross-workspace references
- **Distributed teams**: Team members in different workspaces contributing to the same project, each with their own audit trail
- **Contractor coordination**: Delegation chains across organizations with cascading completion and preserved attribution
- **Cross-workspace queries**: Aggregate view of work across organizations (`bd list --org=...`)
