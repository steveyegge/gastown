---
title: "GT DOLT ROLLBACK"
---

## gt dolt rollback

Restore .beads directories from a migration backup

### Synopsis

Roll back a migration by restoring .beads directories from a backup.

If no backup directory is specified, the most recent migration-backup-TIMESTAMP/
directory is used automatically.

This command will:
1. Stop the Dolt server if running
2. Find the specified (or most recent) backup
3. Restore all .beads directories from the backup
4. Reset metadata.json files to their pre-migration state
5. Validate the restored state with bd list

The backup directory is expected to be in the format created by the migration
formula's backup step (migration-backup-YYYYMMDD-HHMMSS/).

```
gt dolt rollback [backup-dir] [flags]
```

### Options

```
      --dry-run   Show what would be restored without making changes
  -h, --help      help for rollback
      --list      List available backups and exit
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

