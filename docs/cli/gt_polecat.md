---
title: "GT POLECAT"
---

## gt polecat

Manage polecats (persistent identity, ephemeral sessions)

### Synopsis

Manage polecat lifecycle in rigs.

Polecats have PERSISTENT IDENTITY but EPHEMERAL SESSIONS. Each polecat has
a permanent agent bead and CV chain that accumulates work history across
assignments. Sessions and sandboxes are ephemeral — spawned for specific
tasks, cleaned up on completion — but the identity persists.

A polecat is either:
  - Working: Actively doing assigned work
  - Stalled: Session crashed mid-work (needs Witness intervention)
  - Zombie: Finished but gt done failed (needs cleanup)
  - Nuked: Session ended, identity persists (ready for next assignment)

Self-cleaning model: When work completes, the polecat runs 'gt done',
which pushes the branch, submits to the merge queue, and exits. The
Witness then nukes the sandbox. The polecat's identity (agent bead)
persists with agent_state=nuked, preserving work history.

Session vs sandbox: The Claude session cycles frequently (handoffs,
compaction). The git worktree (sandbox) persists until nuke. Work
survives session restarts.

Cats build features. Dogs clean up messes.

```
gt polecat [flags]
```

### Options

```
  -h, --help   help for polecat
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt polecat check-recovery](../cli/gt_polecat_check-recovery/)	 - Check if polecat needs recovery vs safe to nuke
* [gt polecat gc](../cli/gt_polecat_gc/)	 - Garbage collect stale polecat branches
* [gt polecat git-state](../cli/gt_polecat_git-state/)	 - Show git state for pre-kill verification
* [gt polecat identity](../cli/gt_polecat_identity/)	 - Manage polecat identities
* [gt polecat list](../cli/gt_polecat_list/)	 - List polecats in a rig
* [gt polecat nuke](../cli/gt_polecat_nuke/)	 - Completely destroy a polecat (session, worktree, branch, agent bead)
* [gt polecat pool-init](../cli/gt_polecat_pool-init/)	 - Initialize a persistent polecat pool for a rig
* [gt polecat prune](../cli/gt_polecat_prune/)	 - Prune stale polecat branches (local and remote)
* [gt polecat remove](../cli/gt_polecat_remove/)	 - Remove polecats from a rig
* [gt polecat stale](../cli/gt_polecat_stale/)	 - Detect stale polecats that may need cleanup
* [gt polecat status](../cli/gt_polecat_status/)	 - Show detailed status for a polecat

