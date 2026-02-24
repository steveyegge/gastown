---
title: "GT DOLT"
---

## gt dolt

Manage the Dolt SQL server

### Synopsis

Manage the Dolt SQL server for Gas Town beads.

The Dolt server provides multi-client access to all rig databases,
avoiding the single-writer limitation of embedded Dolt mode.

Server configuration:
  - Port: 3307 (avoids conflict with MySQL on 3306)
  - User: root (default Dolt user, no password for localhost)
  - Data directory: .dolt-data/ (contains all rig databases)

Each rig (hq, gastown, beads) has its own database subdirectory.

```
gt dolt [flags]
```

### Options

```
  -h, --help   help for dolt
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt dolt cleanup](../cli/gt_dolt_cleanup/)	 - Remove orphaned databases from .dolt-data/
* [gt dolt fix-metadata](../cli/gt_dolt_fix-metadata/)	 - Update metadata.json in all rig .beads directories
* [gt dolt init](../cli/gt_dolt_init/)	 - Initialize and repair Dolt workspace configuration
* [gt dolt init-rig](../cli/gt_dolt_init-rig/)	 - Initialize a new rig database
* [gt dolt list](../cli/gt_dolt_list/)	 - List available rig databases
* [gt dolt logs](../cli/gt_dolt_logs/)	 - View Dolt server logs
* [gt dolt migrate](../cli/gt_dolt_migrate/)	 - Migrate existing dolt databases to centralized data directory
* [gt dolt migrate-wisps](../cli/gt_dolt_migrate-wisps/)	 - Migrate agent beads from issues to wisps table
* [gt dolt recover](../cli/gt_dolt_recover/)	 - Detect and recover from Dolt read-only state
* [gt dolt restart](../cli/gt_dolt_restart/)	 - Restart the Dolt server (kills imposters)
* [gt dolt rollback](../cli/gt_dolt_rollback/)	 - Restore .beads directories from a migration backup
* [gt dolt sql](../cli/gt_dolt_sql/)	 - Open Dolt SQL shell
* [gt dolt start](../cli/gt_dolt_start/)	 - Start the Dolt server
* [gt dolt status](../cli/gt_dolt_status/)	 - Show Dolt server status
* [gt dolt stop](../cli/gt_dolt_stop/)	 - Stop the Dolt server
* [gt dolt sync](../cli/gt_dolt_sync/)	 - Push Dolt databases to DoltHub remotes

