---
title: "DOCS/CLI/GT RIG ADD"
---

## gt rig add

Add a new rig to the workspace

### Synopsis

Add a new rig by cloning a repository.

This creates a rig container with:
  - config.json           Rig configuration
  - .beads/               Rig-level issue tracking (initialized)
  - plugins/              Rig-level plugin directory
  - refinery/rig/         Canonical main clone
  - mayor/rig/            Mayor's working clone
  - crew/                 Empty crew directory (add members with 'gt crew add')
  - witness/              Witness agent directory
  - polecats/             Worker directory (empty)

The command also:
  - Seeds patrol molecules (Deacon, Witness, Refinery)
  - Creates ~/gt/plugins/ (town-level) if it doesn't exist
  - Creates <rig>/plugins/ (rig-level)

Use --adopt to register an existing directory instead of creating new:
  - Reads existing config.json if present
  - Auto-detects git URL from origin remote (git-url argument not required)
  - Adds entry to mayor/rigs.json

Example:
  gt rig add gastown https://github.com/steveyegge/gastown
  gt rig add my-project git@github.com:user/repo.git --prefix mp
  gt rig add existing-rig --adopt

```
gt rig add <name> <git-url> [flags]
```

### Options

```
      --adopt               Adopt an existing directory instead of creating new
      --branch string       Default branch name (default: auto-detected from remote)
      --force               With --adopt, register even if git remote cannot be detected
  -h, --help                help for add
      --local-repo string   Local repo path to share git objects (optional)
      --prefix string       Beads issue prefix (default: derived from name)
      --push-url string     Push URL for read-only upstreams (push to fork)
      --url string          Git remote URL for --adopt (default: auto-detected from origin)
```

### SEE ALSO

* [gt rig](../cli/gt_rig/)	 - Manage rigs in the workspace

