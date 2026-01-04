# Multi-Rig Beads Routing

How cross-rig beads routing works in Gas Town.

## Overview

When running Gas Town with multiple rigs, each rig has its own beads database
in `<rig>/mayor/rig/.beads/`. Gas Town uses **prefix-based routing** to
automatically direct `bd` commands to the correct rig.

## How It Works

When you add a rig with `gt rig add`, it creates a route entry in
`~/gt/.beads/routes.jsonl`:

```jsonl
{"prefix": "ap", "path": "ai-platform/mayor/rig"}
{"prefix": "gt", "path": "gastown/mayor/rig"}
{"prefix": "ho", "path": "houston/mayor/rig"}
```

This enables automatic routing:

```bash
bd show ap-123    # Routes to ai-platform beads
bd show gt-456    # Routes to gastown beads
bd show hq-789    # Routes to town-level beads
```

You don't need to think about which database to use - just use the issue ID.

## Adding a Rig

```bash
gt rig add myproject https://github.com/user/repo.git
```

This automatically:
1. Clones the repository
2. Sets up the rig structure (mayor/rig, crew, polecats, etc.)
3. Initializes beads with a prefix derived from the name
4. Creates a route entry in `~/gt/.beads/routes.jsonl`

### Custom Prefix

Override the auto-derived prefix:

```bash
gt rig add my-long-project-name https://github.com/user/repo.git --prefix mlp
```

## Verifying Routes

```bash
# List all rigs and their status
gt rig list

# Debug routing for a specific issue
BD_DEBUG_ROUTING=1 bd show <id>
```

## Creating Beads

**Create beads directly in the rig directory** to ensure correct prefix:

```bash
# From rig directory
cd ~/gt/ai-platform/mayor/rig
bd create --title="Add auth endpoint" --type=task
# Creates: ap-xxxxx

# Or use subshell
bash -c 'cd ~/gt/gastown/mayor/rig && bd create --title="Fix bug" --type=bug'
# Creates: gt-xxxxx
```

## Slinging Work

Once routing is configured, `gt sling` finds beads by prefix:

```bash
# From town root
gt sling ap-3f4 ai-platform    # Sling ai-platform issue to ai-platform rig
gt sling gt-abc gastown        # Sling gastown issue to gastown rig

# Dry-run to verify routing
gt sling ap-3f4 ai-platform --dry-run
```

**Match beads to rigs**: Read the bead first (`bd show <id>`) to understand which
codebase it modifies, then sling to the correct rig.

## Troubleshooting

### "No issue found" for known bead

1. Verify the route exists:
   ```bash
   cat ~/gt/.beads/routes.jsonl | grep <prefix>
   ```

2. If missing, the rig may have been added before routing was implemented:
   ```bash
   gt doctor --fix
   ```

3. Verify bead exists in rig:
   ```bash
   bash -c 'cd ~/gt/<rig>/mayor/rig && bd show <id>'
   ```

### Prefix conflict

If two rigs share a prefix:

```bash
bd rename-prefix <new-prefix>
```

### Debug routing

```bash
BD_DEBUG_ROUTING=1 bd show <id>
```

## Architecture Notes

**routes.jsonl**: Created by `gt rig add`, maps prefix → rig path. Each entry
tells `bd` where to find the beads database for that prefix.

**Canonical beads location**: Each rig's beads live in `mayor/rig/.beads/`.
Other worktrees (crew, polecats) use beads redirects to share the same database.

**Town-level beads**: `~/gt/.beads/` uses the `hq-` prefix for mayor mail and
cross-rig coordination.
