---
title: "GT UNINSTALL"
---

## gt uninstall

Remove Gas Town from the system

### Synopsis

Completely remove Gas Town from the system.

By default, removes:
  - Shell integration (~/.zshrc or ~/.bashrc)
  - Wrapper scripts (~/bin/gt-codex, ~/bin/gt-gemini, ~/bin/gt-opencode)
  - State directory (~/.local/state/gastown/)
  - Config directory (~/.config/gastown/)
  - Cache directory (~/.cache/gastown/)

The workspace (e.g., ~/gt) is NOT removed unless --workspace is specified.

Use --force to skip confirmation prompts.

Examples:
  gt uninstall                    # Remove Gas Town, keep workspace
  gt uninstall --workspace        # Also remove workspace directory
  gt uninstall --force            # Skip confirmation

```
gt uninstall [flags]
```

### Options

```
  -f, --force       Skip confirmation prompts
  -h, --help        help for uninstall
      --workspace   Also remove the workspace directory (DESTRUCTIVE)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

