---
title: "DOCS/CLI/GT MOL ATTACH-FROM-MAIL"
---

## gt mol attach-from-mail

Attach a molecule from a mail message

### Synopsis

Attach a molecule to the current agent's hook from a mail message.

This command reads a mail message, extracts the molecule ID from the body,
and attaches it to the agent's pinned bead (hook).

The mail body should contain an "attached_molecule:" field with the molecule ID.

Usage: gt mol attach-from-mail <mail-id>

Behavior:
1. Read mail body for attached_molecule field
2. Attach molecule to agent's hook
3. Mark mail as read
4. Return control for execution

Example:
  gt mol attach-from-mail msg-abc123

```
gt mol attach-from-mail <mail-id> [flags]
```

### Options

```
  -h, --help   help for attach-from-mail
```

### SEE ALSO

* [gt mol](../cli/gt_mol/)	 - Agent molecule workflow commands

