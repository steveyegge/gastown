---
title: "DOCS/CLI/GT CONVOY CREATE"
---

## gt convoy create

Create a new convoy

### Synopsis

Create a new convoy that tracks the specified issues.

The convoy is created in town-level beads (hq-* prefix) and can track
issues across any rig.

The --owner flag specifies who requested the convoy (receives completion
notification by default). If not specified, defaults to created_by.
The --notify flag adds additional subscribers beyond the owner.

The --merge flag sets the merge strategy for all work in the convoy:
  direct  Push branch directly to main (no MR, no refinery)
  mr      Create merge-request bead, refinery processes (default)
  local   Keep on feature branch (for upstream PRs, human review)

Examples:
  gt convoy create "Deploy v2.0" gt-abc bd-xyz
  gt convoy create "Release prep" gt-abc --notify           # defaults to mayor/
  gt convoy create "Release prep" gt-abc --notify ops/      # notify ops/
  gt convoy create "Feature rollout" gt-a gt-b --owner mayor/ --notify ops/
  gt convoy create "Feature rollout" gt-a gt-b gt-c --molecule mol-release
  gt convoy create --owned "Manual deploy" gt-abc           # caller-managed lifecycle
  gt convoy create "Quick fix" gt-abc --merge=direct        # bypass refinery

```
gt convoy create <name> [issues...] [flags]
```

### Options

```
  -h, --help                       help for create
      --merge string               Merge strategy: direct (push to main), mr (merge queue, default), local (keep on branch)
      --molecule string            Associated molecule ID
      --notify string[="mayor/"]   Additional address to notify on completion (default: mayor/ if flag used without value)
      --owned                      Mark convoy as caller-managed lifecycle (no automatic witness/refinery registration)
      --owner string               Owner who requested convoy (gets completion notification)
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

