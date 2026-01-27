# Storage Backends: SQLite vs Dolt

This document describes the two storage backends available for Gas Town beads, their differences, and how to switch between them.

## Overview

Gas Town beads supports two storage backends:

1. **SQLite** (default) - Traditional relational database with file-based storage
2. **Dolt** - Git-for-data database with version control and branching

Both backends are accessed through the `bd` CLI abstraction layer, which provides a consistent interface regardless of the underlying storage.

## Architecture

### Abstraction Model

```
┌─────────────────────────────────────┐
│  Gas Town Go Code                   │
│  (gt commands, daemons, agents)     │
└────────────┬────────────────────────┘
             │
             ▼
      ┌──────────────┐
      │  bd CLI      │ ◄─── Abstraction boundary
      └──────┬───────┘
             │
       ┌─────┴──────┐
       │            │
       ▼            ▼
   ┌────────┐  ┌───────┐
   │ SQLite │  │ Dolt  │
   └────────┘  └───────┘
```

**Key Principle:** Gas Town code should use `bd` CLI commands, not direct database access. Direct SQLite or Dolt queries bypass the abstraction and break compatibility.

## SQLite Backend

### Characteristics

- **File-based storage**: `beads.db` and `issues.db` files
- **Immediate persistence**: Writes are committed immediately
- **File-based locking**: Multiple readers, single writer with automatic retry
- **JSONL sync**: Bi-directional sync with `issues.jsonl` for git tracking
- **Mature and stable**: Default backend with extensive testing

### Files

```
.beads/
├── metadata.json      # Backend configuration
├── beads.db          # Main database (SQLite)
├── issues.jsonl      # JSONL export for git tracking
└── config.yaml       # Beads configuration
```

### Configuration

`metadata.json`:
```json
{
  "database": "beads.db",
  "jsonl_export": "issues.jsonl",
  "backend": "sqlite"
}
```

## Dolt Backend

### Characteristics

- **Git-style version control**: Full history, branching, and diffing
- **Directory-based storage**: `.dolt/` directory structure
- **Manual commits required**: Changes must be explicitly committed to history
- **Git-style locking**: Lock files prevent concurrent writes (`.dolt/noms/LOCK`)
- **Readonly fallback**: When lock can't be acquired, opens in readonly mode
- **JSONL export-only**: JSONL is generated from Dolt, not used for import

### Files

```
.beads/
├── metadata.json      # Backend configuration
├── config.yaml       # Beads configuration (no-auto-import: true)
├── issues.jsonl      # JSONL export (export-only)
└── dolt/
    └── beads/        # Dolt database directory
        ├── .dolt/    # Dolt metadata and history
        └── ...       # Database tables
```

### Configuration

`metadata.json`:
```json
{
  "jsonl_export": "issues.jsonl",
  "backend": "dolt",
  "prefix": "gt"
}
```

`config.yaml`:
```yaml
# For Dolt-native setup: Dolt is source of truth, JSONL is export-only
no-auto-import: true
```

### Version Control Features

Dolt provides version control features through the `bd` CLI:

```bash
# View issue history
bd history <issue-id>

# List branches
bd branch

# Show version control status
bd vc status

# Create a branch
bd branch create feature-branch

# Show diff between commits
bd diff <commit1> <commit2>
```

## Key Differences

| Feature | SQLite | Dolt |
|---------|--------|------|
| **Persistence** | Immediate | Manual commits required |
| **Version History** | No | Full history with `bd history` |
| **Branching** | No | Yes, via `bd branch` |
| **Lock Behavior** | Wait/retry | Fail to readonly mode |
| **JSONL Sync** | Bi-directional | Export-only |
| **Lock File** | Database file | `.dolt/noms/LOCK` |
| **CLI Required** | sqlite3 | dolt |

## Lock Handling

### SQLite Locking

- **Mechanism**: File-based locks on database file
- **Behavior**: Multiple readers allowed; single writer blocks readers
- **On Contention**: Waits and retries automatically
- **Duration**: Transaction-scoped (released when transaction completes)

### Dolt Locking

- **Mechanism**: Git-style lock files (`.dolt/noms/LOCK`)
- **Behavior**: Lock required for write access
- **On Contention**: Opens in readonly mode, write attempts fail with "database is read only"
- **Duration**: Process-scoped (released when process exits)

**Critical Difference:**
- SQLite: Lock failure → wait/retry → success
- Dolt: Lock failure → readonly mode → error on write attempt

**Retry Logic (rig-358fc7):** Dolt lock contention now has automatic retry with exponential backoff (30 attempts, ~6s total). Retryable errors:
- "database is read only" - transient lock contention
- "cannot update manifest" - manifest locked by another process

**Important:** Retry logic does NOT fix bd daemon incompatibility (rig-0eec57) which causes permanent read-only state.

## Migration Guide

### Prerequisites

1. **Install Dolt** (if migrating to Dolt):
   ```bash
   # See https://docs.dolthub.com/introduction/installation
   curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash
   ```

2. **Backup your data**:
   ```bash
   cd /path/to/gastown
   bd export --output backup.jsonl
   cp .beads/beads.db .beads/beads.db.backup  # SQLite only
   ```

### SQLite → Dolt Migration

1. **Export current data**:
   ```bash
   cd /path/to/beads
   bd sync  # Ensure JSONL is up to date
   ```

2. **Initialize Dolt backend**:
   ```bash
   bd init --backend dolt
   # This creates .beads/dolt/ and imports from issues.jsonl
   ```

3. **Update configuration**:

   Edit `.beads/metadata.json`:
   ```json
   {
     "jsonl_export": "issues.jsonl",
     "backend": "dolt",
     "prefix": "gt"
   }
   ```

   Edit `.beads/config.yaml`:
   ```yaml
   no-auto-import: true  # Dolt is source of truth
   ```

4. **Verify migration**:
   ```bash
   bd list --limit 10
   bd vc status
   bd info
   ```

5. **Preserve SQLite backup** (optional):
   ```bash
   # Keep the old SQLite database as backup
   # Remove "backend": "dolt" from metadata.json to revert if needed
   ```

### Dolt → SQLite Migration

1. **Export current data**:
   ```bash
   cd /path/to/beads
   bd sync  # Export Dolt to JSONL
   ```

2. **Update configuration**:

   Edit `.beads/metadata.json`:
   ```json
   {
     "database": "beads.db",
     "jsonl_export": "issues.jsonl",
     "backend": "sqlite"
   }
   ```

   Edit `.beads/config.yaml`:
   ```yaml
   # Remove or set to false:
   # no-auto-import: false
   ```

3. **Initialize SQLite**:
   ```bash
   # Remove old SQLite database if present
   rm -f .beads/beads.db .beads/issues.db

   # Import from JSONL
   bd sync --from-main
   ```

4. **Verify migration**:
   ```bash
   bd list --limit 10
   bd info
   ```

## Troubleshooting

### "database is read only" (Dolt)

**Cause:** Another process holds the Dolt lock file OR you're using direct mode (--no-daemon).

**Solutions:**
1. **Ensure daemon is running**: `bd daemon start` - Direct mode uses SQLite queries that fail on Dolt
2. Wait for other process to complete
3. Check for stale lock: `ls -la .beads/dolt/beads/.dolt/noms/LOCK`
4. Remove stale lock (if process is dead): `rm .beads/dolt/beads/.dolt/noms/LOCK`

**WARNING:** Do NOT use `--no-daemon` with Dolt backend! Direct mode sends SQLite-specific
PRAGMA commands that Dolt (MySQL-compatible) cannot process. Always use daemon mode with Dolt.

### "Auto-import failed" (Dolt)

**Cause:** Auto-import expects SQLite but backend is Dolt.

**Solution:** Set `no-auto-import: true` in `.beads/config.yaml`

### Convoy features not working (Dolt)

**Cause:** Old code using direct `sqlite3` CLI instead of `bd` CLI.

**Status:** Fixed in rig-384557 and rig-9bb08e. Update to latest version.

### "table issues has no column named pinned"

**Cause:** SQLite database is empty but JSONL has content.

**Solution:**
```bash
rm .beads/issues.db
bd sync --from-main
```

Or run: `gt doctor --fix`

### Missing dolt command

**Cause:** Dolt CLI not installed.

**Solution:**
```bash
curl -L https://github.com/dolthub/dolt/releases/latest/download/install.sh | bash
```

## Best Practices

### When to Use SQLite

- **Default choice**: Simple, mature, well-tested
- **No version control needed**: You just need issue tracking
- **Existing installations**: Already using SQLite
- **Simpler operations**: No manual commits required

### When to Use Dolt

- **Version control required**: Need history, branching, or diffing
- **Audit trail needed**: Track who changed what and when
- **Experimentation**: Try changes on a branch without affecting main
- **Data exploration**: Use SQL queries on historical data
- **Compliance**: Need complete change history for regulations

### Configuration Guidelines

1. **Set `no-auto-import: true`** for Dolt backend
2. **NEVER use `--no-daemon`** with Dolt - daemon mode is required (see [Daemon Policy](#daemon-policy-dolt))
3. **Commit regularly** with Dolt to build history
4. **Run `gt doctor`** to verify backend health
5. **Backup before migration** - always export to JSONL first

## Daemon Policy (Dolt)

**When using Dolt backend, daemon mode is REQUIRED for bd commands.**

### Why Daemon Mode is Required

Direct mode (`--no-daemon`) uses an internal SQLite driver that sends SQLite-specific
commands (like `PRAGMA`) to the database. Dolt is MySQL-compatible, not SQLite-compatible.
When these PRAGMA commands hit Dolt, you get errors like:

```
syntax error at position 7 near 'PRAGMA'
```

The daemon mode properly routes queries through Dolt's MySQL-compatible interface.

### Symptoms of Incorrect Direct Mode Usage

- `bd list` returns 0 issues even though data exists
- `bd status` shows empty counts
- Dolt server logs show PRAGMA syntax errors
- `bd show <id>` may work (uses JSONL fallback) while `bd list` fails

### For Tests: --no-daemon is Acceptable

Tests legitimately use `--no-daemon` for isolation:
- Each test gets its own clean database state
- Avoids interference with shared daemon
- Tests typically use SQLite backend anyway

When writing tests that use `--no-daemon`:
1. Ensure test uses SQLite backend, not Dolt
2. Add a comment explaining why `--no-daemon` is used
3. Don't use `--no-daemon` in production code paths

### Development Guidelines

1. **Use bd CLI exclusively** - never bypass with direct queries
2. **Test on both backends** - use table-driven tests (see internal/beads/backend_test.go)
3. **Handle readonly errors** - implement retry logic for Dolt lock contention
4. **Document backend assumptions** - clearly mark SQLite or Dolt-specific code
5. **Run `gt doctor`** regularly to catch backend issues early

## Related Issues

- **rig-384557** - Fixed convoy watcher to use bd CLI instead of sqlite3
- **rig-9bb08e** - Fixed convoy fetcher to use bd CLI instead of sqlite3
- **rig-508d0d** - Added backend detection to gt doctor
- **rig-358fc7** - Add Dolt lock retry logic (planned)
- **hq-5e4a58** - Emergency fix: removed --no-daemon from production code
- **hq-f33de4** - Document daemon-only policy for Dolt backend

## References

- [Dolt Documentation](https://docs.dolthub.com/)
- [Dolt GitHub](https://github.com/dolthub/dolt)
- [SQLite Documentation](https://www.sqlite.org/docs.html)
- Gas Town audit report: `docs/reports/rig-eb4531-storage-audit.md`

---

**Last Updated:** 2026-01-25
**Author:** furiosa (via dolt_doctor)
**Related Epic:** gt-0686b8 (Dolt-native gastown fork)
