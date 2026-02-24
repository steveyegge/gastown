---
title: "GT POLECAT NUKE"
---

## gt polecat nuke

Completely destroy a polecat (session, worktree, branch, agent bead)

### Synopsis

Completely destroy a polecat and all its artifacts.

This is the nuclear option for post-merge cleanup. It:
  1. Kills the Claude session (if running)
  2. Deletes the git worktree (bypassing all safety checks)
  3. Deletes the polecat branch
  4. Closes the agent bead (if exists)

SAFETY CHECKS: The command refuses to nuke a polecat if:
  - Worktree has unpushed/uncommitted changes
  - Polecat has an open merge request (MR bead)
  - Polecat has work on its hook

Use --force to bypass safety checks (LOSES WORK).
Use --dry-run to see what would happen and safety check status.

Examples:
  gt polecat nuke greenplace/Toast
  gt polecat nuke greenplace/Toast greenplace/Furiosa
  gt polecat nuke greenplace --all
  gt polecat nuke greenplace --all --dry-run
  gt polecat nuke greenplace/Toast --force  # bypass safety checks

```
gt polecat nuke <rig>/<polecat>... | <rig> --all [flags]
```

### Options

```
      --all       Nuke all polecats in the rig
      --dry-run   Show what would be nuked without doing it
  -f, --force     Force nuke, bypassing all safety checks (LOSES WORK)
  -h, --help      help for nuke
```

### SEE ALSO

* [gt polecat](../cli/gt_polecat/)	 - Manage polecats (persistent identity, ephemeral sessions)

