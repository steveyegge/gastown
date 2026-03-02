---
title: "GT DOLT DUMP"
---

## gt dolt dump

Dump Dolt server goroutine stacks for debugging

### Synopsis

Send SIGQUIT to the Dolt server to dump goroutine stacks to its log file.

Per Tim Sehn (Dolt CEO): kill -QUIT prints all goroutine stacks to stderr,
which is redirected to the server log. Useful for diagnosing hung servers.

The dump is written to the server log file. Use 'gt dolt logs' to view it.

```
gt dolt dump [flags]
```

### Options

```
  -h, --help   help for dump
```

### SEE ALSO

* [gt dolt](../cli/gt_dolt/)	 - Manage the Dolt SQL server

