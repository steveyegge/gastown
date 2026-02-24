---
title: "GT MOL SQUASH"
---

## gt mol squash

Compress molecule into a digest

### Synopsis

Squash the current molecule into a permanent digest.

This condenses a completed molecule's execution into a compact record.
The digest preserves:
- What molecule was executed
- When it ran
- Summary of results

Use this for patrol cycles and other operational work that should have
a permanent (but compact) record.

```
gt mol squash [target] [flags]
```

### Options

```
  -h, --help             help for squash
      --jitter string    Sleep a random duration from 0 to this value before squashing (e.g. '10s') to reduce concurrent Dolt lock contention
      --json             Output as JSON
      --no-digest        Skip digest bead creation (for patrol molecules that run frequently)
      --summary string   Optional summary for the squash digest (e.g. patrol observations)
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

