---
title: "GT CREW ADD"
---

## gt crew add

Create a new crew workspace

### Synopsis

Create new crew workspace(s) with a clone of the rig repository.

Each workspace is created at <rig>/crew/<name>/ with:
- A full git clone of the project repository
- Mail directory for message delivery
- CLAUDE.md with crew worker prompting
- Optional feature branch (crew/<name>)

Examples:
  gt crew add dave                       # Create single workspace
  gt crew add murgen croaker goblin      # Create multiple at once
  gt crew add emma --rig greenplace      # Create in specific rig
  gt crew add fred --branch              # Create with feature branch

```
gt crew add <name> [flags]
```

### Options

```
      --branch       Create a feature branch (crew/<name>)
  -h, --help         help for add
      --rig string   Rig to create crew workspace in
```

### SEE ALSO

* [gt crew](../cli/gt_crew/)	 - Manage crew workers (persistent workspaces for humans)

