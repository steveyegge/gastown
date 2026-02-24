---
title: "DOCS/CLI/GT TAP"
---

## gt tap

Claude Code hook handlers

### Synopsis

Hook handlers for Claude Code PreToolUse and PostToolUse events.

These commands are called by Claude Code hooks to implement policies,
auditing, and input transformation. They tap into the tool execution
flow to guard, audit, inject, or check.

Subcommands:
  guard   - Block forbidden operations (PreToolUse, exit 2)
  audit   - Log/record tool executions (PostToolUse) [planned]
  inject  - Modify tool inputs (PreToolUse, updatedInput) [planned]
  check   - Validate after execution (PostToolUse) [planned]

Hook configuration in .claude/settings.json:
  {
    "PreToolUse": [{
      "matcher": "Bash(gh pr create*)",
      "hooks": [{"command": "gt tap guard pr-workflow"}]
    }]
  }

See ~/gt/docs/HOOKS.md for full documentation.

### Options

```
  -h, --help   help for tap
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt tap guard](../cli/gt_tap_guard/)	 - Block forbidden operations (PreToolUse hook)

