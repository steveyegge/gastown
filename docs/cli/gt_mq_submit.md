---
title: "GT MQ SUBMIT"
---

## gt mq submit

Submit current branch to the merge queue

### Synopsis

Submit the current branch to the merge queue.

Creates a merge-request bead that will be processed by the Refinery.

Auto-detection:
  - Branch: current git branch
  - Issue: parsed from branch name (e.g., polecat/Nux/gp-xyz → gt-xyz)
  - Worker: parsed from branch name
  - Rig: detected from current directory
  - Target: automatically determined (see below)
  - Priority: inherited from source issue

Target branch auto-detection:
  1. If --epic is specified: target the integration branch for <epic> (using configured template)
  2. If source issue has a parent epic with an integration branch: target it
  3. Otherwise: target main

This ensures batch work on epics automatically flows to integration branches.

Polecat auto-cleanup:
  When run from a polecat work branch (polecat/<worker>/<issue>), this command
  automatically triggers polecat shutdown after submitting the MR. The polecat
  sends a lifecycle request to its Witness and waits for termination.

  Use --no-cleanup to disable this behavior (e.g., if you want to submit
  multiple MRs or continue working).

Examples:
  gt mq submit                           # Auto-detect everything + auto-cleanup
  gt mq submit --issue gp-abc            # Explicit issue
  gt mq submit --epic gt-xyz             # Target integration branch explicitly
  gt mq submit --priority 0              # Override priority (P0)
  gt mq submit --no-cleanup              # Submit without auto-cleanup

```
gt mq submit [flags]
```

### Options

```
      --branch string   Source branch (default: current branch)
      --epic string     Target epic's integration branch instead of main
  -h, --help            help for submit
      --issue string    Source issue ID (default: parse from branch name)
      --no-cleanup      Don't auto-cleanup after submit (for polecats)
  -p, --priority int    Override priority (0-4, default: inherit from issue) (default -1)
```

### SEE ALSO

* [gt mq](../cli/gt_mq/)	 - Merge queue operations

