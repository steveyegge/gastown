---
title: "GT CONVOY STRANDED"
---

## gt convoy stranded

Find stranded convoys (ready work or empty) needing attention

### Synopsis

Find convoys that have ready issues but no workers processing them,
or empty convoys (0 tracked issues) that need cleanup.

A convoy is "stranded" when:
- Convoy is open AND either:
  - Has tracked issues that are ready but unassigned, OR
  - Has 0 tracked issues (empty — needs auto-close via convoy check)

Use this to detect convoys that need feeding or cleanup. The Deacon patrol
runs this periodically and dispatches dogs to feed stranded convoys.

Examples:
  gt convoy stranded              # Show stranded convoys
  gt convoy stranded --json       # Machine-readable output for automation

```
gt convoy stranded [flags]
```

### Options

```
  -h, --help   help for stranded
      --json   Output as JSON
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

