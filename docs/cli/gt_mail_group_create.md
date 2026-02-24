---
title: "DOCS/CLI/GT MAIL GROUP CREATE"
---

## gt mail group create

Create a new group

### Synopsis

Create a new mail distribution group.

Members can be specified as positional arguments or with --member flags.

Examples:
  gt mail group create ops-team gastown/witness gastown/crew/max
  gt mail group create ops-team --member gastown/witness --member gastown/crew/max

```
gt mail group create <name> [members...] [flags]
```

### Options

```
  -h, --help                 help for create
      --member stringArray   Member to add (repeatable)
```

### SEE ALSO

* [gt mail group](../cli/gt_mail_group/)	 - Manage mail groups

