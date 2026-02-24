---
title: "GT STALE"
---

## gt stale

Check if the gt binary is stale

### Synopsis

Check if the gt binary was built from an older commit than the current repo HEAD.

This command compares the commit hash embedded in the binary at build time
with the current HEAD of the gastown repository.

Examples:
  gt stale              # Human-readable output
  gt stale --json       # Machine-readable JSON output
  gt stale --quiet      # Exit code only (0=stale, 1=fresh)

Exit codes:
  0 - Binary is stale (needs rebuild)
  1 - Binary is fresh (up to date)
  2 - Error (could not determine staleness)

```
gt stale [flags]
```

### Options

```
  -h, --help    help for stale
      --json    Output as JSON
  -q, --quiet   Exit code only (0=stale, 1=fresh)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

