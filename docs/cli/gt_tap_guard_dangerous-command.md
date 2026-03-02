---
title: "GT TAP GUARD DANGEROUS-COMMAND"
---

## gt tap guard dangerous-command

Block dangerous commands (rm -rf, force push, etc.)

### Synopsis

Block dangerous commands via Claude Code PreToolUse hooks.

This guard blocks operations that could cause irreversible damage:
  - rm -rf with absolute paths (e.g., rm -rf /path)
  - git push --force / git push -f
  - git reset --hard
  - git clean -f / git clean -fd

The guard reads the tool input from stdin (Claude Code hook protocol)
and exits with code 2 to block dangerous operations.

Exit codes:
  0 - Operation allowed
  2 - Operation BLOCKED

```
gt tap guard dangerous-command [flags]
```

### Options

```
  -h, --help   help for dangerous-command
```

### SEE ALSO

* [gt tap guard](../cli/gt_tap_guard/)	 - Block forbidden operations (PreToolUse hook)

