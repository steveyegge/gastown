---
title: "GT CLEANUP"
---

## gt cleanup

Clean up orphaned Claude processes

### Synopsis

Clean up orphaned Claude processes that survived session termination.

This command finds and kills Claude processes that are not associated with
any active Gas Town tmux session. These orphans can accumulate when:
- Polecat sessions are killed without proper cleanup
- Claude spawns subagent processes that outlive their parent
- Network or system issues interrupt normal shutdown

Uses aggressive tmux session verification to detect ALL orphaned processes,
not just those with PPID=1.

Examples:
  gt cleanup              # Clean up orphans with confirmation
  gt cleanup --dry-run    # Show what would be killed
  gt cleanup --force      # Kill without confirmation

```
gt cleanup [flags]
```

### Options

```
      --dry-run   Show what would be killed without killing
  -f, --force     Kill without confirmation
  -h, --help      help for cleanup
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

