---
title: "DOCS/CLI/GT CONVOY"
---

## gt convoy

Track batches of work across rigs

### Synopsis

Manage convoys - the primary unit for tracking batched work.

A convoy is a persistent tracking unit that monitors related issues across
rigs. When you kick off work (even a single issue), a convoy tracks it so
you can see when it lands and what was included.

WHAT IS A CONVOY:
  - Persistent tracking unit with an ID (hq-*)
  - Tracks issues across rigs (frontend+backend, beads+gastown, etc.)
  - Auto-closes when all tracked issues complete → notifies subscribers
  - Can be reopened by adding more issues

WHAT IS A SWARM:
  - Ephemeral: "the workers currently assigned to a convoy's issues"
  - No separate ID - uses the convoy ID
  - Dissolves when work completes

TRACKING SEMANTICS:
  - 'tracks' relation is non-blocking (tracked issues don't block convoy)
  - Cross-prefix capable (convoy in hq-* tracks issues in gt-*, bd-*)
  - Landed: all tracked issues closed → notification sent to subscribers

COMMANDS:
  create    Create a convoy tracking specified issues
  add       Add issues to an existing convoy (reopens if closed)
  close     Close a convoy (verifies all items done, or use --force)
  land      Land an owned convoy (cleanup worktrees, close convoy)
  status    Show convoy progress, tracked issues, and active workers
  list      List convoys (the dashboard view)

```
gt convoy [flags]
```

### Options

```
  -h, --help          help for convoy
  -i, --interactive   Interactive tree view
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt convoy add](../cli/gt_convoy_add/)	 - Add issues to an existing convoy
* [gt convoy check](../cli/gt_convoy_check/)	 - Check and auto-close completed convoys
* [gt convoy close](../cli/gt_convoy_close/)	 - Close a convoy
* [gt convoy create](../cli/gt_convoy_create/)	 - Create a new convoy
* [gt convoy land](../cli/gt_convoy_land/)	 - Land an owned convoy (cleanup worktrees, close convoy)
* [gt convoy launch](../cli/gt_convoy_launch/)	 - Launch a staged convoy: transition to open and dispatch Wave 1
* [gt convoy list](../cli/gt_convoy_list/)	 - List convoys
* [gt convoy stage](../cli/gt_convoy_stage/)	 - Stage a convoy: analyze dependencies, compute waves, create staged convoy
* [gt convoy status](../cli/gt_convoy_status/)	 - Show convoy status
* [gt convoy stranded](../cli/gt_convoy_stranded/)	 - Find stranded convoys (ready work or empty) needing attention

