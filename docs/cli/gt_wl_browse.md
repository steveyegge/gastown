---
title: "GT WL BROWSE"
---

## gt wl browse

Browse wanted items on the commons board

### Synopsis

Browse the Wasteland wanted board (hop/wl-commons).

Uses the clone-then-discard pattern: clones the commons database to a
temporary directory, queries it, then deletes the clone.

EXAMPLES:
  gt wl browse                          # All open wanted items
  gt wl browse --project gastown        # Filter by project
  gt wl browse --type bug               # Only bugs
  gt wl browse --status claimed         # Claimed items
  gt wl browse --priority 0             # Critical priority only
  gt wl browse --limit 5               # Show 5 items
  gt wl browse --json                   # JSON output

```
gt wl browse [flags]
```

### Options

```
  -h, --help             help for browse
      --json             Output as JSON
      --limit int        Maximum items to display (default 50)
      --priority int     Filter by priority (0=critical, 2=medium, 4=backlog) (default -1)
      --project string   Filter by project (e.g., gastown, beads, hop)
      --status string    Filter by status (open, claimed, in_review, completed, withdrawn) (default "open")
      --type string      Filter by type (feature, bug, design, rfc, docs)
```

### SEE ALSO

* [gt wl](../cli/gt_wl/)	 - Wasteland federation commands

