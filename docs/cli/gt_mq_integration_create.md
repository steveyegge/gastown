---
title: "DOCS/CLI/GT MQ INTEGRATION CREATE"
---

## gt mq integration create

Create an integration branch for an epic

### Synopsis

Create an integration branch for batch work on an epic.

Creates a branch from main and pushes it to origin. Future MRs for this
epic's children can target this branch.

Branch naming:
  Default: integration/<sanitized-title> (e.g., integration/add-user-auth)
  Config:  Set merge_queue.integration_branch_template in rig settings
  Override: Use --branch flag for one-off customization

Template variables:
  {title}  - Sanitized epic title (e.g., "add-user-authentication")
  {epic}   - Full epic ID (e.g., "RA-123")
  {prefix} - Epic prefix before first hyphen (e.g., "RA")
  {user}   - Git user.name (e.g., "klauern")

If two epics produce the same branch name, a numeric suffix from the
epic ID is appended automatically (e.g., integration/add-auth-123).

Actions:
  1. Verify epic exists
  2. Create branch from main (using template or --branch)
  3. Push to origin
  4. Store actual branch name in epic metadata

Examples:
  gt mq integration create gt-auth-epic
  # Creates integration/add-user-authentication (from epic title)

  gt mq integration create RA-123 --branch "klauern/PROJ-1234/{epic}"
  # Creates klauern/PROJ-1234/RA-123

```
gt mq integration create <epic-id> [flags]
```

### Options

```
      --base-branch string   Create integration branch from this branch instead of main
      --branch string        Override branch name template (supports {title}, {epic}, {prefix}, {user})
      --force                Recreate integration branch even if one already exists
  -h, --help                 help for create
```

### SEE ALSO

* [gt mq integration](../cli/gt_mq_integration/)	 - Manage integration branches for epics

