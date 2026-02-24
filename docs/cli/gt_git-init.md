---
title: "GT GIT-INIT"
---

## gt git-init

Initialize git repository for a Gas Town HQ

### Synopsis

Initialize or configure git for an existing Gas Town HQ.

This command:
  1. Creates a comprehensive .gitignore for Gas Town
  2. Initializes a git repository if not already present
  3. Optionally creates a GitHub repository (private by default)

The .gitignore excludes:
  - Polecat worktrees and rig clones (recreated with 'gt sling' or 'gt rig add')
  - Runtime state files (state.json, *.lock)
  - OS and editor files

And tracks:
  - CLAUDE.md and role contexts
  - .beads/ configuration and issues
  - Rig configs and hop/ directory

Examples:
  gt git-init                             # Init git with .gitignore
  gt git-init --github=user/repo          # Create private GitHub repo (default)
  gt git-init --github=user/repo --public # Create public GitHub repo

```
gt git-init [flags]
```

### Options

```
      --github string   Create GitHub repo (format: owner/repo, private by default)
  -h, --help            help for git-init
      --public          Make GitHub repo public (repos are private by default)
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

