---
title: "DOCS/CLI/GT HOOK"
---

## gt hook

Show or attach work on a hook

### Synopsis

Show what's on your hook, or attach new work.

With no arguments, shows your current hook status (alias for 'gt mol status').
With a bead ID, attaches that work to your hook.
With a bead ID and target, attaches work to another agent's hook.

The hook is the "durability primitive" - work on your hook survives session
restarts, context compaction, and handoffs. When you restart (via gt handoff),
your SessionStart hook finds the attached work and you continue from where
you left off.

Examples:
  gt hook                                    # Show what's on my hook
  gt hook status                             # Same as above
  gt hook gt-abc                             # Attach issue gt-abc to your hook
  gt hook gt-abc -s "Fix the bug"            # With subject for handoff mail
  gt hook gt-abc gastown/crew/max            # Attach gt-abc to max's hook

Related commands:
  gt sling <bead>    # Hook + start now (keep context)
  gt handoff <bead>  # Hook + restart (fresh context)
  gt unsling         # Remove work from hook

```
gt hook [bead-id] [target] [flags]
```

### Options

```
      --clear            Clear your hook (alias for 'gt unhook')
  -n, --dry-run          Show what would be done
  -f, --force            Replace existing incomplete hooked bead
  -h, --help             help for hook
      --json             Output as JSON (for status)
  -m, --message string   Message for handoff mail (optional)
  -s, --subject string   Subject for handoff mail (optional)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt hook attach](../cli/gt_hook_attach/)	 - Attach work to a hook
* [gt hook clear](../cli/gt_hook_clear/)	 - Clear your hook (alias for 'gt unhook')
* [gt hook detach](../cli/gt_hook_detach/)	 - Detach work from a hook
* [gt hook show](../cli/gt_hook_show/)	 - Show what's on an agent's hook (compact)
* [gt hook status](../cli/gt_hook_status/)	 - Show what's on your hook

