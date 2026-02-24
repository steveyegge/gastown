---
title: "GT CONVOY LIST"
---

## gt convoy list

List convoys

### Synopsis

List convoys, showing open convoys by default.

Examples:
  gt convoy list              # Open convoys only (default)
  gt convoy list --all        # All convoys (open + closed)
  gt convoy list --status=closed  # Recently landed
  gt convoy list --tree       # Show convoy + child status tree
  gt convoy list --json

```
gt convoy list [flags]
```

### Options

```
      --all             Show all convoys (open and closed)
  -h, --help            help for list
      --json            Output as JSON
      --status string   Filter by status (open, closed)
      --tree            Show convoy + child status tree
```

### SEE ALSO

* [gt convoy](../cli/gt_convoy/)	 - Track batches of work across rigs

