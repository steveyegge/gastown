---
title: "GT DOLT FLATTEN"
---

## gt dolt flatten

Flatten database history to a single commit (NUCLEAR OPTION)

### Synopsis

Flatten a Dolt database's commit history to a single commit.

This is the NUCLEAR OPTION for compaction. It destroys all history.
Use only when automated compaction is insufficient.

All operations run via SQL on the running server — no downtime needed.

Safety protocol:
  1. Pre-flight: verifies backup freshness and records row counts
  2. Soft-resets to root commit on main (keeps all data staged)
  3. Commits all data as single commit
  4. Verifies row counts match (integrity check)

Requires --yes-i-am-sure flag as safety interlock.

```
gt dolt flatten <database> [flags]
```

### Options

```
  -h, --help            help for flatten
      --yes-i-am-sure   Required safety flag to confirm you want to destroy history
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

