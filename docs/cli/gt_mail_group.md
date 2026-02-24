---
title: "DOCS/CLI/GT MAIL GROUP"
---

## gt mail group

Manage mail groups

### Synopsis

Create and manage mail distribution groups.

Groups are named collections of addresses used for mail distribution.
Members can be:
  - Direct addresses (gastown/crew/max)
  - Patterns (*/witness, gastown/*)
  - Other group names (nested groups)

Examples:
  gt mail group list                              # List all groups
  gt mail group show ops-team                     # Show group members
  gt mail group create ops-team gastown/witness gastown/crew/max
  gt mail group add ops-team deacon/
  gt mail group remove ops-team gastown/witness
  gt mail group delete ops-team

```
gt mail group [flags]
```

### Options

```
  -h, --help   help for group
```

### SEE ALSO

* [gt mail](../cli/gt_mail/)	 - Agent messaging system
* [gt mail group add](../cli/gt_mail_group_add/)	 - Add member to group
* [gt mail group create](../cli/gt_mail_group_create/)	 - Create a new group
* [gt mail group delete](../cli/gt_mail_group_delete/)	 - Delete a group
* [gt mail group list](../cli/gt_mail_group_list/)	 - List all groups
* [gt mail group remove](../cli/gt_mail_group_remove/)	 - Remove member from group
* [gt mail group show](../cli/gt_mail_group_show/)	 - Show group details

