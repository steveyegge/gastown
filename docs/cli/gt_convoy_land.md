---
title: "GT CONVOY LAND"
---

## gt convoy land

Land an owned convoy (cleanup worktrees, close convoy)

### Synopsis

Land an owned convoy, performing caller-side cleanup.

This is the caller-managed equivalent of the witness/refinery merge pipeline.
Use this to explicitly land a convoy when you're satisfied with the results.

The command:
  1. Verifies the convoy has the gt:owned label (refuses non-owned convoys)
  2. Checks all tracked issues are done/closed (use --force to override)
  3. Cleans up polecat worktrees associated with the convoy's tracked issues
  4. Closes the convoy bead with reason "Landed by owner"
  5. Sends completion notifications to owner/notify addresses

Use 'gt convoy close' instead for non-owned convoys.

Examples:
  gt convoy land hq-cv-abc                  # Land owned convoy
  gt convoy land hq-cv-abc --force          # Land even with open issues
  gt convoy land hq-cv-abc --keep-worktrees # Skip worktree cleanup
  gt convoy land hq-cv-abc --dry-run        # Preview what would happen

```
gt convoy land <convoy-id> [flags]
```

### Options

```
      --dry-run          Show what would happen without acting
  -f, --force            Land even if tracked issues are not all closed
  -h, --help             help for land
      --keep-worktrees   Skip worktree cleanup
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

