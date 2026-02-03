# Rig Database Configuration

This document describes the proper database setup for Gas Town rigs.

## Overview

When you create a rig with `gt rig add`, Gas Town automatically configures the beads database based on whether the source repository has tracked beads configuration.

## Database Modes

Gas Town supports two database backends:

### 1. SQLite with JSONL Sync (Default)

The standard beads configuration uses SQLite for local storage with JSONL export for git tracking.

**Configuration:**
```yaml
# .beads/config.yaml
prefix: <your-prefix>
sync-branch: beads-sync
```

**Used by:**
- `sf_ideation` - SQLite database with JSONL sync
- `carbon_backend` - SQLite database with JSONL sync

**File structure:**
```
<rig>/
├── .beads/
│   ├── beads.db           # SQLite database (gitignored)
│   ├── beads.db-shm       # SQLite shared memory (gitignored)
│   ├── beads.db-wal       # SQLite write-ahead log (gitignored)
│   ├── config.yaml        # Beads configuration
│   ├── issues.jsonl       # Tracked issues (committed to git)
│   └── interactions.jsonl # Tracked interactions (committed to git)
```

### 2. Dolt-Native Mode (Centralized)

Gas Town's own rigs use Dolt as the database backend with centralized storage.

**Configuration:**
```yaml
# .beads/config.yaml
prefix: gt
sync:
  mode: dolt-native

# Custom issue types for Gas Town
types.custom: "agent,role,rig,convoy,slot,queue,event,message,molecule,gate,merge-request"
```

**Used by:**
- `gastown` - Dolt-native mode with centralized database

**File structure:**
```
<rig>/
├── .beads/
│   ├── redirect            # Points to mayor/rig/.beads
│   └── ...
├── mayor/
│   └── rig/
│       └── .beads/         # Canonical database location
│           ├── beads.db    # SQLite (for bd compatibility)
│           ├── config.yaml # Dolt-native configuration
│           └── dolt/       # Dolt database files
```

## How `gt rig add` Configures Databases

When you run `gt rig add <name> <url>`, the following happens:

### If the source repo has tracked `.beads/`:

1. The source `.beads/` directory is cloned into `mayor/rig/.beads/`
2. A **redirect file** is created at `<rig>/.beads/redirect` containing:
   ```
   mayor/rig/.beads
   ```
3. All other worktrees (refinery, polecats) use this redirect to access the canonical database
4. The database is initialized with `bd init --prefix <detected-prefix>`

### If the source repo has no tracked `.beads/`:

1. A new local database is created at `<rig>/.beads/`
2. The database is initialized with `bd init --prefix <derived-prefix>`
3. Custom Gas Town types are configured

## Redirect-Based Architecture

Gas Town uses a redirect pattern for rigs with tracked beads:

```
<rig>/.beads/redirect → mayor/rig/.beads
```

This ensures:
- **Single source of truth**: The mayor's clone holds the canonical database
- **Consistent access**: All worktrees (refinery, polecats, crew) access the same data
- **Git-friendly**: The redirect file is local-only (not committed)

## Configuration Reference

### Standard Beads Config (config.yaml)

```yaml
# Issue prefix for this repository
prefix: "gt"

# Git branch for beads commits
sync-branch: "beads-sync"

# Cross-project dependencies
external_projects:
  beads: ../../../beads/mayor/rig

# Multi-repo configuration (experimental)
repos:
  primary: "."
  additional:
    - ~/beads-planning
```

### Dolt-Native Config

```yaml
prefix: gt

# Use Dolt instead of SQLite+JSONL
sync:
  mode: dolt-native

# Custom issue types for Gas Town
types.custom: "agent,role,rig,convoy,slot,queue,event,message,molecule,gate,merge-request"

# Cross-project dependencies
external_projects:
  beads: ../../../beads/mayor/rig
```

## Best Practices

### For New Rigs

1. **Use the default SQLite+JSONL mode** unless you have specific requirements for Dolt
2. **Commit `issues.jsonl`** to git so issues are tracked and shared
3. **Don't commit `beads.db`** - it's in `.gitignore` by default
4. **Set a meaningful prefix** that identifies your project (e.g., `gt` for gastown, `si` for sf_ideation)

### For Gas Town System Rigs

1. **Use Dolt-native mode** for better multi-writer support
2. **Configure custom types** for Gas Town-specific bead types (agent, role, rig, etc.)
3. **Set up external project dependencies** for cross-rig issue references

### Migration

To migrate an existing rig from SQLite to Dolt-native:

```bash
# Edit .beads/config.yaml
sync:
  mode: dolt-native

# Re-initialize
cd mayor/rig
bd init
```

## Troubleshooting

### "database not found" errors

Check if the redirect file exists and points to the right location:
```bash
cat <rig>/.beads/redirect
ls -la <rig>/mayor/rig/.beads/
```

### "read-only database" errors (Dolt mode)

Dolt-native mode requires the Dolt server to be running:
```bash
gt up  # Starts the Dolt server
```

### Prefix conflicts

If the source repo has a different prefix than expected:
```bash
# Use --prefix to override
gt rig add myproject https://github.com/user/repo.git --prefix mp
```

## See Also

- [INSTALLING.md](INSTALLING.md) - Gas Town installation guide
- [reference.md](reference.md) - Technical reference
- `gt rig add --help` - Command reference
