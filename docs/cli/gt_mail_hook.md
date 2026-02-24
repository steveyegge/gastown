---
title: "GT MAIL HOOK"
---

## gt mail hook

Attach mail to your hook (alias for 'gt hook attach')

### Synopsis

Attach a mail message to your hook.

This is an alias for 'gt hook attach <mail-id>'. It attaches the specified
mail message to your hook so you can work on it.

The hook is the "durability primitive" - work on your hook survives session
restarts, context compaction, and handoffs.

Examples:
  gt mail hook msg-abc123                    # Attach mail to your hook
  gt mail hook msg-abc123 -s "Fix the bug"   # With subject for handoff
  gt mail hook msg-abc123 --force            # Replace existing incomplete work

Related commands:
  gt hook <bead>     # Attach any bead to your hook
  gt hook status     # Show what's on your hook
  gt unsling         # Remove work from hook

```
gt mail hook <mail-id> [flags]
```

### Options

```
  -n, --dry-run          Show what would be done
  -f, --force            Replace existing incomplete hooked bead
  -h, --help             help for hook
  -m, --message string   Message for handoff mail (optional)
  -s, --subject string   Subject for handoff mail (optional)
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system

