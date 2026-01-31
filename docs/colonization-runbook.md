# Gas Town Colonization Runbook

> Step-by-step guide for creating a new Gas Town instance (e.g., gt12) that shares beads with an existing town (e.g., gt11).

## Overview

**Colonization** creates a new Gas Town instance that can operate independently while sharing the central beads database with the parent town. This enables:

- Multiple development environments (e.g., production gt11, staging gt12)
- Parallel agent workloads across machines
- Independent tmux session namespaces
- Shared issue tracking and coordination

## Prerequisites

1. **Existing parent town** (e.g., `/home/ubuntu/gt11`) with working beads database
2. **Git access** to gastown repository (https://github.com/groblegark/gastown.git)
3. **Go toolchain** for building binaries
4. **Sufficient disk space** (~500MB for clone + binaries)

## Step 1: Create Town Directory Structure

```bash
# Create the new town root
mkdir -p /home/ubuntu/gt12

# Create town-level directories
mkdir -p /home/ubuntu/gt12/{mayor,.beads,.claude,.runtime,daemon,deacon,logs}
```

## Step 2: Configure Town Identity

Create `/home/ubuntu/gt12/mayor/town.json`:

```json
{
  "type": "town",
  "version": 2,
  "name": "gastown12",
  "owner": "your.email@example.com",
  "public_name": "Gas Town 12",
  "created_at": "2026-01-27T00:00:00Z"
}
```

**Key fields:**
- `name`: Internal identifier (used in tmux sessions, logs)
- `owner`: Email for overseer notifications
- `public_name`: Human-readable display name

## Step 3: Clone and Build Gastown

```bash
# Clone fresh from GitHub (NOT copy from parent town)
cd /home/ubuntu/gt12
git clone https://github.com/groblegark/gastown.git gastown

# Build the binary
cd /home/ubuntu/gt12/gastown
make build

# Verify build
./gt --version
# Expected: gt version v0.2.5-XXX-gXXXXXXX
```

## Step 4: Create Rig Directory Structure

The rig holds agents and workspaces:

```bash
# Create rig subdirectories
mkdir -p /home/ubuntu/gt12/gastown/gastown/{crew,polecats,refinery,witness}
```

**Directory purposes:**
- `crew/`: Human workspace clones
- `polecats/`: Transient worker directories
- `refinery/`: Merge queue processor workspace
- `witness/`: Agent lifecycle monitor

## Step 5: Register the Rig

Create `/home/ubuntu/gt12/mayor/rigs.json`:

```json
{
  "version": 1,
  "rigs": {
    "gastown": {
      "git_url": "https://github.com/groblegark/gastown.git",
      "added_at": "2026-01-27T00:00:00Z",
      "beads": {
        "repo": "",
        "prefix": "gtn"
      }
    }
  }
}
```

**Important:** Use a unique prefix (`gtn` for gt12) different from the parent town (`gt` for gt11) to avoid bead ID collisions.

## Step 6: Configure Shared Beads (Optional)

To share beads with the parent town, configure routing:

### Option A: Symlink to Parent Beads

```bash
# Remove empty .beads and link to parent
rm -rf /home/ubuntu/gt12/.beads
ln -s /home/ubuntu/gt11/.beads /home/ubuntu/gt12/.beads
```

### Option B: Configure Dolt Remote

Create `/home/ubuntu/gt12/.beads/config.yaml`:

```yaml
storage:
  backend: dolt
  dolt:
    remote: file:///home/ubuntu/gt11/.beads/dolt
```

### Option C: Independent Beads (No Sharing)

Leave `.beads/` empty; `bd init` will create a fresh database on first use.

## Step 7: Verify Installation

```bash
cd /home/ubuntu/gt12

# Check town status
./gastown/gt status

# Expected output:
# Town: gastown12
# /home/ubuntu/gt12
#
# 🎩 mayor        ●
# 🐺 deacon       ●
#
# ─── gastown/ ───────────────────────────────────────────
# 🦉 witness      ○

# Check rig status
./gastown/gt rig list
./gastown/gt rig status gastown

# Expected: gastown rig OPERATIONAL with prefix gtn-
```

## Step 8: Start Agents (Optional)

```bash
# Start the town (mayor, deacon, boot)
./gastown/gt up

# Start rig agents (witness, refinery)
./gastown/gt rig start gastown
```

## Verification Checklist

| Check | Command | Expected |
|-------|---------|----------|
| Binary builds | `./gt --version` | Version string |
| Town recognized | `./gt status` | Shows town name |
| Rig registered | `./gt rig list` | Shows gastown |
| Rig operational | `./gt rig status gastown` | OPERATIONAL |
| Beads prefix | `./gt rig status gastown` | gtn- prefix |

## Rollback Procedure

To remove a colonized town:

```bash
# Stop all agents
cd /home/ubuntu/gt12
./gastown/gt down

# Remove town directory
rm -rf /home/ubuntu/gt12
```

**Warning:** If using shared beads (symlink), beads created with the new town's prefix (gtn-) will remain in the parent's database.

## Tmux Session Namespace

Each town uses its own tmux session prefix to avoid conflicts:

| Town | Prefix | Example Sessions |
|------|--------|------------------|
| gt11 | `gt-` | `gt-mayor`, `gt-gastown-witness` |
| gt12 | `gt12-` | `gt12-mayor`, `gt12-gastown-witness` |

The session prefix is derived from the town directory name.

## Cross-Town Communication

For agents in different towns to communicate:

1. **Shared beads**: Both towns must access the same beads database
2. **Mail routing**: Configure routes.jsonl to include cross-town prefixes
3. **Nudge delivery**: tmux session names must be resolvable from either town

## Troubleshooting

### "No rigs registered"
- Check `/home/ubuntu/gt12/mayor/rigs.json` exists and is valid JSON
- Ensure the rig directory path matches the rigs.json entry

### "Binary was built with 'go build' directly"
- Rebuild using `make build` instead of `go build`
- The Makefile sets required build flags

### Beads not visible
- Verify beads symlink or Dolt remote configuration
- Check `bd list` output from the new town root
- Ensure routes.jsonl includes the new town's prefix

## Future: `gt colonize` Command

This runbook documents the manual process. A future `gt colonize` command will automate:

```bash
# Proposed syntax
gt colonize /home/ubuntu/gt12 --from /home/ubuntu/gt11 --name gastown12 --share-beads
```

---

*Part of the Colonization Epic (hq-arcu0q)*
*Last updated: 2026-01-27*
