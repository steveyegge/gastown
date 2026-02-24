---
title: "DOCS/CLI/GT POLECAT IDENTITY SHOW"
---

## gt polecat identity show

Show polecat identity with CV summary

### Synopsis

Show detailed identity information for a polecat including work history.

Displays:
  - Identity bead ID and creation date
  - Session count
  - Completion statistics (issues completed, failed, abandoned)
  - Language breakdown from file extensions
  - Work type breakdown (feat, fix, refactor, etc.)
  - Recent work list with relative timestamps

Examples:
  gt polecat identity show gastown Toast
  gt polecat identity show gastown Toast --json

```
gt polecat identity show <rig> <name> [flags]
```

### Options

```
  -h, --help   help for show
      --json   Output as JSON
```

### SEE ALSO

* [gt polecat identity](../cli/gt_polecat_identity/)	 - Manage polecat identities

