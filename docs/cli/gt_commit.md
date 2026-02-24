---
title: "GT COMMIT"
---

## gt commit

Git commit with automatic agent identity

### Synopsis

Git commit wrapper that automatically sets git author identity for agents.

When run by an agent (GT_ROLE set), this command:
1. Detects the agent identity from environment variables
2. Converts it to a git-friendly name and email
3. Runs 'git commit' with the correct identity

The email domain is configurable in town settings (agent_email_domain).
Default: gastown.local

Examples:
  gt commit -m "Fix bug"              # Commit as current agent
  gt commit -am "Quick fix"           # Stage all and commit
  gt commit -- --amend                # Amend last commit

Identity mapping:
  Agent: gastown/crew/jack  →  Name: gastown/crew/jack
                                Email: gastown.crew.jack@gastown.local

When run without GT_ROLE (human), passes through to git commit with no changes.

```
gt commit [flags] [-- git-commit-args...]
```

### Options

```
  -h, --help   help for commit
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

