---
title: "DOCS/CLI/GT STATUS"
---

## gt status

Show overall town status

### Synopsis

Display the current status of the Gas Town workspace.

Shows town name, registered rigs, polecats, and witness status.

Use --fast to skip mail lookups for faster execution.
Use --watch to continuously refresh status at regular intervals.

```
gt status [flags]
```

### Options

```
      --fast           Skip mail lookups for faster execution
  -h, --help           help for status
  -n, --interval int   Refresh interval in seconds (default 2)
      --json           Output as JSON
  -v, --verbose        Show detailed multi-line output per agent
  -w, --watch          Watch mode: refresh status continuously
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager

