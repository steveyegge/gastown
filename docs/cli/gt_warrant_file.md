---
title: "GT WARRANT FILE"
---

## gt warrant file

File a death warrant for an agent

### Synopsis

File a death warrant for an agent that needs termination.

The target should be an agent path like:
  - gastown/polecats/alpha
  - deacon/dogs/bravo
  - beads/polecats/charlie

Examples:
  gt warrant file gastown/polecats/alpha --reason "Zombie: no session, idle >10m"
  gt warrant file deacon/dogs/bravo --reason "Stuck: working on task for >2h"

```
gt warrant file <target> [flags]
```

### Options

```
  -h, --help            help for file
  -r, --reason string   Reason for the warrant (required unless --stdin)
      --stdin           Read reason from stdin (avoids shell quoting issues)
```

### SEE ALSO

* [gt warrant](../cli/gt_warrant/)	 - Manage death warrants for stuck agents

