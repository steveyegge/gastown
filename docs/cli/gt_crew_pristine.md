---
title: "GT CREW PRISTINE"
---

## gt crew pristine

Sync crew workspaces with remote

### Synopsis

Ensure crew workspace(s) are up-to-date.

Runs git pull for the specified crew, or all crew workers.
Reports any uncommitted changes that may need attention.

Examples:
  gt crew pristine                # Pristine all crew workers
  gt crew pristine dave           # Pristine specific worker
  gt crew pristine --json         # JSON output

```
gt crew pristine [<name>] [flags]
```

### Options

```
  -h, --help         help for pristine
      --json         Output as JSON
      --rig string   Filter by rig name
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

