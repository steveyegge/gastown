---
title: "DOCS/CLI/GT DOCTOR"
---

## gt doctor

Run health checks on the workspace

### Synopsis

Run diagnostic checks on the Gas Town workspace.

Doctor checks for common configuration issues, missing files,
and other problems that could affect workspace operation.

Workspace checks:
  - town-config-exists       Check mayor/town.json exists
  - town-config-valid        Check mayor/town.json is valid
  - rigs-registry-exists     Check mayor/rigs.json exists (fixable)
  - rigs-registry-valid      Check registered rigs exist (fixable)
  - mayor-exists             Check mayor/ directory structure

Town root protection:
  - town-git                 Verify town root is under version control
  - town-root-branch         Verify town root is on main branch (fixable)
  - pre-checkout-hook        Verify pre-checkout hook prevents branch switches (fixable)

Infrastructure checks:
  - stale-binary             Check if gt binary is up to date with repo
  - beads-binary             Check that beads (bd) is installed and meets minimum version
  - daemon                   Check if daemon is running (fixable)
  - boot-health              Check Boot watchdog health (vet mode)
  - town-beads-config        Verify town .beads/config.yaml exists (fixable)

Cleanup checks (fixable):
  - orphan-sessions          Detect orphaned tmux sessions
  - orphan-processes         Detect orphaned Claude processes
  - session-name-format      Detect sessions with outdated naming format (fixable)
  - wisp-gc                  Detect and clean abandoned wisps (>1h)
  - stale-beads-redirect     Detect stale files in .beads directories with redirects

Clone divergence checks:
  - persistent-role-branches Detect crew/witness/refinery not on main
  - clone-divergence         Detect clones significantly behind origin/main
  - default-branch-all-rigs  Verify default_branch exists on remote for all rigs
  - worktree-gitdir-valid    Verify worktree .git files reference existing paths (fixable)

Crew workspace checks:
  - crew-state               Validate crew worker state.json files (fixable)
  - crew-worktrees           Detect stale cross-rig worktrees (fixable)

Migration checks (fixable):
  - sparse-checkout          Detect legacy sparse checkout across all rigs

Rig checks (with --rig flag):
  - rig-is-git-repo          Verify rig is a valid git repository
  - git-exclude-configured   Check .git/info/exclude has Gas Town dirs (fixable)
  - bare-repo-exists         Verify .repo.git exists when worktrees depend on it (fixable)
  - witness-exists           Verify witness/ structure exists (fixable)
  - refinery-exists          Verify refinery/ structure exists (fixable)
  - mayor-clone-exists       Verify mayor/rig/ clone exists (fixable)
  - polecat-clones-valid     Verify polecat directories are valid clones
  - beads-config-valid       Verify beads configuration (fixable)

Routing checks (fixable):
  - routes-config            Check beads routing configuration
  - prefix-mismatch          Detect rigs.json vs routes.jsonl prefix mismatches (fixable)
  - database-prefix          Detect database vs routes.jsonl prefix mismatches (fixable)

Session hook checks:
  - session-hooks            Check settings.json use session-start.sh
  - claude-settings          Check Claude settings.json match templates (fixable)
  - deprecated-merge-queue-keys  Detect stale deprecated keys in merge_queue config (fixable)
  - stale-task-dispatch      Detect stale task-dispatch guard in settings.json (fixable)

Dolt checks:
  - dolt-binary              Check that dolt is installed and meets minimum version
  - dolt-metadata            Check dolt metadata tables exist
  - dolt-server-reachable    Check dolt sql-server is reachable
  - dolt-orphaned-databases  Detect orphaned dolt databases

Patrol checks:
  - patrol-molecules-exist   Verify patrol molecules exist
  - patrol-hooks-wired       Verify daemon triggers patrols
  - patrol-not-stuck         Detect stale wisps (>1h)
  - patrol-plugins-accessible Verify plugin directories

Use --fix to attempt automatic fixes for issues that support it.
Use --rig to check a specific rig instead of the entire workspace.
Use --slow to highlight slow checks (default threshold: 1s, e.g. --slow=500ms).

```
gt doctor [flags]
```

### Options

```
      --fix                  Attempt to automatically fix issues
  -h, --help                 help for doctor
      --restart-sessions     Restart patrol sessions when fixing stale settings (use with --fix)
      --rig string           Check specific rig only
      --slow string[="1s"]   Highlight slow checks (optional threshold, default 1s)
  -v, --verbose              Show detailed output
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

