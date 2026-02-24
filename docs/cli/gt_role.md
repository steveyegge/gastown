---
title: "DOCS/CLI/GT ROLE"
---

## gt role

Show or manage agent role

### Synopsis

Display the current agent role and its detection source.

Role is determined by:
1. GT_ROLE environment variable (authoritative if set)
2. Current working directory (fallback)

If both are available and disagree, a warning is shown.

```
gt role [flags]
```

### Options

```
  -h, --help   help for role
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt role def](../cli/gt_role_def/)	 - Display role definition (session, health, env config)
* [gt role detect](../cli/gt_role_detect/)	 - Force cwd-based role detection (debugging)
* [gt role env](../cli/gt_role_env/)	 - Print export statements for current role
* [gt role home](../cli/gt_role_home/)	 - Show home directory for a role
* [gt role list](../cli/gt_role_list/)	 - List all known roles
* [gt role show](../cli/gt_role_show/)	 - Show current role

