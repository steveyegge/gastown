---
title: "DOCS/CLI/GT DONE"
---

## gt done

Signal work ready for merge queue

### Synopsis

Signal that your work is complete and ready for the merge queue.

This is a convenience command for polecats that:
1. Submits the current branch to the merge queue
2. Auto-detects issue ID from branch name
3. Notifies the Witness with the exit outcome
4. Exits the Claude session (polecats don't stay alive after completion)

Exit statuses:
  COMPLETED      - Work done, MR submitted (default)
  ESCALATED      - Hit blocker, needs human intervention
  DEFERRED       - Work paused, issue still open

Examples:
  gt done                              # Submit branch, notify COMPLETED, exit session
  gt done --issue gt-abc               # Explicit issue ID
  gt done --status ESCALATED           # Signal blocker, skip MR
  gt done --status DEFERRED            # Pause work, skip MR

```
gt done [flags]
```

### Options

```
      --cleanup-status string   Git cleanup status: clean, uncommitted, unpushed, stash, unknown (ZFC: agent-observed)
  -h, --help                    help for done
      --issue string            Source issue ID (default: parse from branch name)
  -p, --priority int            Override priority (0-4, default: inherit from issue) (default -1)
      --resume                  Resume from last checkpoint (auto-detected, for Witness recovery)
      --status string           Exit status: COMPLETED, ESCALATED, or DEFERRED (default "COMPLETED")
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

