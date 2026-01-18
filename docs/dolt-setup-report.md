# Dolt Backend Setup Validation Report

**Date:** 2026-01-18
**Issue:** hq-7wyafd
**Polecat:** furiosa

## Summary

Successfully set up Dolt as the database backend for gastown beads to enable version control features.

## Installation

1. **Dolt CLI installed:** v1.80.2
   - Installed via GitHub releases binary download
   - Location: `/usr/local/bin/dolt`

2. **Dolt configuration:**
   - Global user.name: "Gas Town"
   - Global user.email: "gastown@pihealth.local"

## Configuration Changes

### metadata.json
Updated `/home/ubuntu/pihealth/gastown/mayor/rig/.beads/metadata.json`:
```json
{
  "database": "beads.db",
  "jsonl_export": "issues.jsonl",
  "backend": "dolt"
}
```

### Database Location
- Dolt database: `/home/ubuntu/pihealth/gastown/mayor/rig/.beads/dolt/beads/`
- SQLite backup: `/home/ubuntu/pihealth/gastown/mayor/rig/.beads/beads.db` (preserved)

## Verification

### Working Features

1. **`bd history <issue>`** - Shows version history for issues
   ```
   $ bd history gt-0144ae
   📜 History for gt-0144ae (2 entries)

   mevm4322 2026-01-18 22:36:40
     Author: Gas Town
     ✓ gt-0144ae: Test Dolt versioning [P2 - closed]

   hmv125ng 2026-01-18 22:35:38
     Author: Gas Town
     ○ gt-0144ae: Test Dolt versioning [P2 - open]
   ```

2. **`bd branch`** - Lists Dolt branches
   ```
   $ bd branch
   🌿 Branches:
     * main
   ```

3. **`bd vc status`** - Shows version control status
   ```
   $ bd vc status
   📊 Version Control Status
     Branch: main
     Commit: mevm4322
   ```

4. **Bootstrap from JSONL** - Imported 594 issues on first access

### Known Issues

1. **Auto-import warning**: The message "Auto-import failed: import requires SQLite storage backend" appears on some commands. This is expected since auto-import from JSONL is a SQLite-specific feature.

2. **Manual commits needed**: The Dolt backend doesn't automatically commit changes. Use `dolt add . && dolt commit -m "message"` in the Dolt database directory to create version history entries.

3. **Daemon compatibility**: Some operations fail with "database is read only" when the daemon is involved. Use `--no-daemon` flag when encountering this issue.

4. **Permission issues**: The manifest file may need 644 permissions after initial creation to prevent read-only errors.

## Recommendations

1. **For full history tracking**: After making changes via `bd` commands, manually commit in the Dolt database:
   ```bash
   cd /home/ubuntu/pihealth/gastown/mayor/rig/.beads/dolt/beads
   dolt add .
   dolt commit -m "Description of changes"
   ```

2. **When encountering read-only errors**: Use `--no-daemon` flag or check file permissions in the Dolt directory.

3. **Backup**: The original SQLite database (beads.db) is preserved and can be restored by removing the "backend" field from metadata.json.

## Files Changed

- `/home/ubuntu/pihealth/gastown/mayor/rig/.beads/metadata.json` - Added backend: dolt
- `/home/ubuntu/pihealth/gastown/mayor/rig/.beads/dolt/` - New Dolt database directory (created by bd)
- `/home/ubuntu/pihealth/gastown/mayor/rig/.beads/full-backup.jsonl` - Backup export before migration
