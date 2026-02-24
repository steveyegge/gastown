---
title: "DOCS/CLI/GT TAP GUARD PR-WORKFLOW"
---

## gt tap guard pr-workflow

Block PR creation and feature branches

### Synopsis

Block PR workflow operations in Gas Town.

Gas Town workers push directly to main. PRs add friction that breaks
the autonomous execution model (GUPP principle).

This guard blocks:
  - gh pr create
  - git checkout -b (feature branches)
  - git switch -c (feature branches)

Exit codes:
  0 - Operation allowed (not in Gas Town agent context, not maintainer origin)
  2 - Operation BLOCKED (in agent context OR maintainer origin)

The guard blocks in two scenarios:
  1. Running as a Gas Town agent (crew, polecat, witness, etc.)
  2. Origin remote is steveyegge/gastown (maintainer should push directly)

Humans running outside Gas Town with a fork origin can still use PRs.

```
gt tap guard pr-workflow [flags]
```

### Options

```
  -h, --help   help for pr-workflow
```

### SEE ALSO

* [gt tap guard](../cli/gt_tap_guard/)	 - Block forbidden operations (PreToolUse hook)

