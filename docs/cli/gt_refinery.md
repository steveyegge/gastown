---
title: "DOCS/CLI/GT REFINERY"
---

## gt refinery

Manage the Refinery (merge queue processor)

### Synopsis

Manage the Refinery - the per-rig merge queue processor.

The Refinery serializes all merges to main for a rig:
  - Receives MRs submitted by polecats (via gt done)
  - Rebases work branches onto latest main
  - Runs validation (tests, builds, checks)
  - Merges to main when clear
  - If conflict: spawns FRESH polecat to re-implement (original is gone)

Work flows: Polecat completes → gt done → MR in queue → Refinery merges.
The polecat is already nuked by the time the Refinery processes.

One Refinery per rig. Persistent agent that processes work as it arrives.

Role shortcuts: "refinery" in mail/nudge addresses resolves to this rig's Refinery.

```
gt refinery [flags]
```

### Options

```
  -h, --help   help for refinery
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt refinery attach](../cli/gt_refinery_attach/)	 - Attach to refinery session
* [gt refinery blocked](../cli/gt_refinery_blocked/)	 - List MRs blocked by open tasks
* [gt refinery claim](../cli/gt_refinery_claim/)	 - Claim an MR for processing
* [gt refinery queue](../cli/gt_refinery_queue/)	 - Show merge queue
* [gt refinery ready](../cli/gt_refinery_ready/)	 - List MRs ready for processing (unclaimed and unblocked)
* [gt refinery release](../cli/gt_refinery_release/)	 - Release a claimed MR back to the queue
* [gt refinery restart](../cli/gt_refinery_restart/)	 - Restart the refinery
* [gt refinery start](../cli/gt_refinery_start/)	 - Start the refinery
* [gt refinery status](../cli/gt_refinery_status/)	 - Show refinery status
* [gt refinery stop](../cli/gt_refinery_stop/)	 - Stop the refinery
* [gt refinery unclaimed](../cli/gt_refinery_unclaimed/)	 - List unclaimed MRs available for processing

