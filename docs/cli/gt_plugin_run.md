---
title: "GT PLUGIN RUN"
---

## gt plugin run

Manually trigger plugin execution

### Synopsis

Manually trigger a plugin to run.

By default, checks if the gate would allow execution and informs you
if it wouldn't. Use --force to bypass gate checks.

Examples:
  gt plugin run rebuild-gt              # Run if gate allows
  gt plugin run rebuild-gt --force      # Bypass gate check
  gt plugin run rebuild-gt --dry-run    # Show what would happen

```
gt plugin run <name> [flags]
```

### Options

```
      --dry-run   Show what would happen without executing
      --force     Bypass gate check
  -h, --help      help for run
```

### SEE ALSO

* [gt plugin](../cli/gt_plugin/)	 - Plugin management

