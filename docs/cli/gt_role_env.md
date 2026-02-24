---
title: "GT ROLE ENV"
---

## gt role env

Print export statements for current role

### Synopsis

Print shell export statements for the current role.

Role is determined from GT_ROLE environment variable or current working directory.
This is a read-only command that displays the current role's env vars.

Examples:
  eval $(gt role env)    # Export current role's env vars
  gt role env            # View what would be exported

```
gt role env [flags]
```

### Options

```
  -h, --help   help for env
```

### SEE ALSO

* [gt role](../cli/gt_role/)	 - Show or manage agent role

