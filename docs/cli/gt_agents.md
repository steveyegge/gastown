---
title: "DOCS/CLI/GT AGENTS"
---

## gt agents

List Gas Town agent sessions

### Synopsis

List Gas Town agent sessions to stdout.

Shows Mayor, Deacon, Witnesses, Refineries, and Crew workers.
Polecats are hidden (use 'gt polecat list' to see them).

Use 'gt agents menu' for an interactive tmux popup menu.

```
gt agents [flags]
```

### Options

```
  -a, --all    Include polecats in the menu
  -h, --help   help for agents
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt agents check](../cli/gt_agents_check/)	 - Check for identity collisions and stale locks
* [gt agents fix](../cli/gt_agents_fix/)	 - Fix identity collisions and clean up stale locks
* [gt agents list](../cli/gt_agents_list/)	 - List agent sessions (no popup)
* [gt agents menu](../cli/gt_agents_menu/)	 - Interactive popup menu for session switching
* [gt agents state](../cli/gt_agents_state/)	 - Get or set operational state on agent beads

