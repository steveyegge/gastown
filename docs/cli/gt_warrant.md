---
title: "DOCS/CLI/GT WARRANT"
---

## gt warrant

Manage death warrants for stuck agents

### Synopsis

Manage death warrants for agents that need termination.

Death warrants are filed when an agent is stuck, unresponsive, or needs
forced termination. Boot handles warrant execution during triage cycles.

The warrant system provides a controlled way to terminate agents:
1. Deacon/Witness files a warrant with a reason
2. Boot picks up the warrant during triage
3. Boot executes the warrant (terminates session, updates state)
4. Warrant is marked as executed

Warrants are stored in ~/gt/warrants/ as JSON files.

### Options

```
  -h, --help   help for warrant
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt warrant execute](../cli/gt_warrant_execute/)	 - Execute a warrant (terminate agent)
* [gt warrant file](../cli/gt_warrant_file/)	 - File a death warrant for an agent
* [gt warrant list](../cli/gt_warrant_list/)	 - List pending warrants

