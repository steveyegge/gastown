---
title: "GT NAMEPOOL CREATE"
---

## gt namepool create

Create a custom theme

### Synopsis

Create a custom namepool theme stored as a text file.

The theme is saved to <town>/settings/themes/<name>.txt and can be
used with 'gt namepool set <name>'. Names can be provided as arguments
or read from a file with --from-file.

Examples:
  gt namepool create tolkien aragorn legolas gimli gandalf frodo samwise
  gt namepool create tolkien --from-file ~/tolkien-names.txt

```
gt namepool create <name> [names...] [flags]
```

### Options

```
      --from-file string   Read names from file instead of arguments
  -h, --help               help for create
```

### SEE ALSO

* [gt namepool](../cli/gt_namepool/)	 - Manage polecat name pools

