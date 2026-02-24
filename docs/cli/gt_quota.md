---
title: "GT QUOTA"
---

## gt quota

Manage account quota rotation

### Synopsis

Manage Claude Code account quota rotation for Gas Town.

When sessions hit rate limits, quota commands help detect blocked sessions
and rotate them to available accounts from the pool.

Commands:
  gt quota status            Show account quota status
  gt quota scan              Detect rate-limited sessions
  gt quota rotate            Swap blocked sessions to available accounts
  gt quota clear             Mark account(s) as available again

```
gt quota [flags]
```

### Options

```
  -h, --help   help for quota
```

### SEE ALSO

* [gt](../cli/gt/)	 - Gas Town - Multi-agent workspace manager
* [gt quota clear](../cli/gt_quota_clear/)	 - Mark account(s) as available again
* [gt quota rotate](../cli/gt_quota_rotate/)	 - Swap blocked sessions to available accounts
* [gt quota scan](../cli/gt_quota_scan/)	 - Detect rate-limited sessions
* [gt quota status](../cli/gt_quota_status/)	 - Show account quota status

