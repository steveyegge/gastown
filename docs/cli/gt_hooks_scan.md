---
title: "DOCS/CLI/GT HOOKS SCAN"
---

## gt hooks scan

Scan workspace for existing Claude Code hooks

### Synopsis

Scan for .claude/settings.json files and display hooks by type.

Hook types:
  SessionStart     - Runs when Claude session starts
  PreCompact       - Runs before context compaction
  UserPromptSubmit - Runs before user prompt is submitted
  PreToolUse       - Runs before tool execution
  PostToolUse      - Runs after tool execution
  Stop             - Runs when Claude session stops
  WorktreeCreate   - Runs when agent worktree isolation creates a worktree
  WorktreeRemove   - Runs when agent worktree isolation removes a worktree

Examples:
  gt hooks scan              # List all hooks in workspace
  gt hooks scan --verbose    # Show hook commands
  gt hooks scan --json       # Output as JSON

```
gt hooks scan [flags]
```

### Options

```
  -h, --help      help for scan
      --json      Output as JSON
  -v, --verbose   Show hook commands
```

### SEE ALSO

* [gt hooks](../cli/gt_hooks/)	 - Centralized hook management for Gas Town

