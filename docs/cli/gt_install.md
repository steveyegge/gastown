---
title: "GT INSTALL"
---

## gt install

Create a new Gas Town HQ (workspace)

### Synopsis

Create a new Gas Town HQ at the specified path.

The HQ (headquarters) is the top-level directory where Gas Town is installed -
the root of your workspace where all rigs and agents live. It contains:
  - CLAUDE.md            Mayor role context (Mayor runs from HQ root)
  - mayor/               Mayor config, state, and rig registry
  - .beads/              Town-level beads DB (hq-* prefix for mayor mail)

If path is omitted, uses the current directory.

See docs/hq.md for advanced HQ configurations including beads
redirects, multi-system setups, and HQ templates.

Examples:
  gt install ~/gt                              # Create HQ at ~/gt
  gt install . --name my-workspace             # Initialize current dir
  gt install ~/gt --no-beads                   # Skip .beads/ initialization
  gt install ~/gt --git                        # Also init git with .gitignore
  gt install ~/gt --github=user/repo           # Create private GitHub repo (default)
  gt install ~/gt --github=user/repo --public  # Create public GitHub repo
  gt install ~/gt --shell                      # Install shell integration (sets GT_TOWN_ROOT/GT_RIG)
  gt install ~/gt --supervisor                 # Configure launchd/systemd for daemon auto-restart

```
gt install [path] [flags]
```

### Options

```
      --dolt-port int        Dolt SQL server port (default 3307; set when another instance owns the default port)
  -f, --force                Re-run install in existing HQ (preserves town.json and rigs.json)
      --git                  Initialize git with .gitignore
      --github string        Create GitHub repo (format: owner/repo, private by default)
  -h, --help                 help for install
  -n, --name string          Town name (defaults to directory name)
      --no-beads             Skip town beads initialization
      --owner string         Owner email for entity identity (defaults to git config user.email)
      --public               Make GitHub repo public (use with --github)
      --public-name string   Public display name (defaults to town name)
      --shell                Install shell integration (sets GT_TOWN_ROOT/GT_RIG env vars)
      --supervisor           Configure launchd/systemd for daemon auto-restart
      --wrappers             Install gt-codex/gt-gemini/gt-opencode wrapper scripts to ~/bin/
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

