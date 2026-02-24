---
title: "DOCS/CLI/GT MQ INTEGRATION"
---

## gt mq integration

Manage integration branches for epics

### Synopsis

Manage integration branches for batch work on epics.

Integration branches allow multiple MRs for an epic to target a shared
branch instead of main. After all epic work is complete, the integration
branch is landed to main as a single atomic unit.

Commands:
  create  Create an integration branch for an epic
  land    Merge integration branch to main
  status  Show integration branch status

```
gt mq integration [flags]
```

### Options

```
  -h, --help   help for integration
```

### SEE ALSO

* [gt mq](../cli/gt_mq/)	 - Merge queue operations
* [gt mq integration create](../cli/gt_mq_integration_create/)	 - Create an integration branch for an epic
* [gt mq integration land](../cli/gt_mq_integration_land/)	 - Merge integration branch to main
* [gt mq integration status](../cli/gt_mq_integration_status/)	 - Show integration branch status for an epic

