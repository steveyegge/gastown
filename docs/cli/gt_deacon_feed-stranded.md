---
title: "DOCS/CLI/GT DEACON FEED-STRANDED"
---

## gt deacon feed-stranded

Detect and feed stranded convoys automatically

### Synopsis

Detect stranded convoys and dispatch dogs to feed them.

A convoy is "stranded" when it is open AND either:
- Has ready issues (open, unblocked, no assignee) but no workers
- Has 0 tracked issues (empty — needs auto-close)

This command:
1. Runs 'gt convoy stranded --json' to find stranded convoys
2. For feedable convoys (ready_count > 0): dispatches a dog via gt sling
3. For empty convoys (ready_count == 0): auto-closes via gt convoy check
4. Rate limits to avoid spawning too many dogs at once

Rate limiting:
- Per-cycle limit (default 3): max convoys fed per invocation
- Per-convoy cooldown (default 10m): prevents re-feeding before dog finishes

This is called by the Deacon during patrol. Run manually for debugging.

Examples:
  gt deacon feed-stranded                  # Feed stranded convoys
  gt deacon feed-stranded --max-feeds 5    # Allow up to 5 feeds per cycle
  gt deacon feed-stranded --cooldown 5m    # 5 minute per-convoy cooldown
  gt deacon feed-stranded --json           # Machine-readable output

```
gt deacon feed-stranded [flags]
```

### Options

```
      --cooldown duration   Minimum time between feeds of same convoy (default: 10m)
  -h, --help                help for feed-stranded
      --json                Output results as JSON
      --max-feeds int       Max convoys to feed per invocation (default: 3)
```

### SEE ALSO

* [gt deacon](../cli/gt_deacon/)	 - Manage the Deacon (town-level watchdog)

