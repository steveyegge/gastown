---
title: "DOCS/CLI/GT DOG DISPATCH"
---

## gt dog dispatch

Dispatch plugin execution to a dog

### Synopsis

Dispatch a plugin for execution by a dog worker.

This is the formalized command for sending plugin work to dogs. The Deacon
uses this during patrol cycles to dispatch plugins with open gates.

The command:
1. Finds the plugin definition (plugin.md)
2. Assigns work to an idle dog (marks as working)
3. Sends mail with plugin instructions to the dog
4. Returns immediately (non-blocking)

The dog discovers the work via its mail inbox and executes the plugin
instructions. On completion, the dog sends DOG_DONE mail to deacon/.

Examples:
  gt dog dispatch --plugin rebuild-gt
  gt dog dispatch --plugin rebuild-gt --rig gastown
  gt dog dispatch --plugin rebuild-gt --dog alpha
  gt dog dispatch --plugin rebuild-gt --create
  gt dog dispatch --plugin rebuild-gt --dry-run
  gt dog dispatch --plugin rebuild-gt --json

```
gt dog dispatch --plugin <name> [flags]
```

### Options

```
      --create          Create a dog if none idle
      --dog string      Dispatch to specific dog (default: any idle)
  -n, --dry-run         Show what would be done without doing it
  -h, --help            help for dispatch
      --json            Output as JSON
      --plugin string   Plugin name to dispatch (required)
      --rig string      Limit plugin search to specific rig
```

### SEE ALSO

* [gt dog](../cli/gt_dog/)	 - Manage dogs (cross-rig infrastructure workers)

