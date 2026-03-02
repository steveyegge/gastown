---
title: "GT TAP GUARD"
---

## gt tap guard

Block forbidden operations (PreToolUse hook)

### Synopsis

Block forbidden operations via Claude Code PreToolUse hooks.

Guard commands exit with code 2 to BLOCK tool execution when a policy
is violated. They're called before the tool runs, preventing the
forbidden operation entirely.

Available guards:
  pr-workflow        - Block PR creation and feature branches
  dangerous-command  - Block rm -rf, force push, hard reset, git clean

Example hook configuration:
  {
    "PreToolUse": [{
      "matcher": "Bash(gh pr create*)",
      "hooks": [{"command": "gt tap guard pr-workflow"}]
    }]
  }

### Options

```
  -h, --help   help for guard
```

### SEE ALSO

* [gt tap](../cli/gt_tap/)	 - Claude Code hook handlers
* [gt tap guard dangerous-command](../cli/gt_tap_guard_dangerous-command/)	 - Block dangerous commands (rm -rf, force push, etc.)
* [gt tap guard pr-workflow](../cli/gt_tap_guard_pr-workflow/)	 - Block PR creation and feature branches

