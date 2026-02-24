---
title: "GT WL POST"
---

## gt wl post

Post a new wanted item to the commons

### Synopsis

Post a new wanted item to the Wasteland commons (shared wanted board).

Creates a wanted item with a unique w-<hash> ID and inserts it into the
wl-commons database. Phase 1 (wild-west): direct write to main branch.

The posted_by field is set to the rig's DoltHub org (DOLTHUB_ORG) or
falls back to the directory name.

Examples:
  gt wl post --title "Fix auth bug" --project gastown --type bug
  gt wl post --title "Add federation sync" --type feature --priority 1 --effort large
  gt wl post --title "Update docs" --tags "docs,federation" --effort small

```
gt wl post [flags]
```

### Options

```
  -d, --description string   Detailed description
      --effort string        Effort level: trivial, small, medium, large, epic (default "medium")
  -h, --help                 help for post
      --priority int         Priority: 0=critical, 1=high, 2=medium, 3=low, 4=backlog (default 2)
      --project string       Project name (e.g., gastown, beads)
      --tags string          Comma-separated tags (e.g., 'go,auth,federation')
      --title string         Title of the wanted item (required)
      --type string          Item type: feature, bug, design, rfc, docs
```

### SEE ALSO

* [gt wl](../cli/gt_wl/)	 - Wasteland federation commands

