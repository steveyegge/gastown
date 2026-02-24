---
title: "GT DISABLE"
---

## gt disable

Disable Gas Town system-wide

### Synopsis

Disable Gas Town for all agentic coding tools.

When disabled:
  - Shell hooks become no-ops
  - Claude Code SessionStart hooks skip 'gt prime'
  - Tools work 100% vanilla (no Gas Town behavior)

The workspace (~/gt) is preserved. Use 'gt enable' to re-enable.

Flags:
  --clean  Also remove shell integration from ~/.zshrc/~/.bashrc

Environment overrides still work:
  GASTOWN_ENABLED=1   - Enable for current session only

```
gt disable [flags]
```

### Options

```
      --clean   Remove shell integration from RC files
  -h, --help    help for disable
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

