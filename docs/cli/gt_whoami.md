---
title: "GT WHOAMI"
---

## gt whoami

Show current identity for mail commands

### Synopsis

Show the identity that will be used for mail commands.

Identity is determined by:
1. GT_ROLE env var (if set) - indicates an agent session
2. No GT_ROLE - you are the overseer (human)

Use --identity flag with mail commands to override.

Examples:
  gt whoami                      # Show current identity
  gt mail inbox                  # Check inbox for current identity
  gt mail inbox --identity mayor/  # Check Mayor's inbox instead

```
gt whoami [flags]
```

### Options

```
  -h, --help   help for whoami
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

