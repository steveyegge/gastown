---
title: "GT WITNESS"
---

## gt witness

Manage the Witness (per-rig polecat health monitor)

### Synopsis

Manage the Witness - the per-rig polecat health monitor.

The Witness patrols a single rig, watching over its polecats:
  - Detects stalled polecats (crashed or stuck mid-work)
  - Nudges unresponsive sessions back to life
  - Cleans up zombie polecats (finished but failed to exit)
  - Nukes sandboxes when polecats complete via 'gt done'

The Witness does NOT force session cycles or interrupt working polecats.
Polecats manage their own sessions (via gt handoff). The Witness handles
failures and edge cases only.

One Witness per rig. The Deacon monitors all Witnesses.

Role shortcuts: "witness" in mail/nudge addresses resolves to this rig's Witness.

```
gt witness [flags]
```

### Options

```
  -h, --help   help for witness
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt witness attach](../cli/gt_witness_attach/)	 - Attach to witness session
* [gt witness restart](../cli/gt_witness_restart/)	 - Restart the witness
* [gt witness start](../cli/gt_witness_start/)	 - Start the witness
* [gt witness status](../cli/gt_witness_status/)	 - Show witness status
* [gt witness stop](../cli/gt_witness_stop/)	 - Stop the witness

