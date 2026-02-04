# Bug: Formula Path Resolution in gt sling

## Summary
When running `gt sling` without `--hook-raw-bead`, the command fails to find the `mol-polecat-work` formula even though it exists.

## Error Message
```
Auto-applying mol-polecat-work for polecat work...
Instantiating formula mol-polecat-work...
Error: parsing formula: read mol-polecat-work: open /Users/unaziogah/gt/myproject/mol-polecat-work: no such file or directory
```

## Expected Behavior
The formula should be found at `~/gt/.beads/formulas/mol-polecat-work.formula.toml`

## Actual Behavior
The `bd cook` command is looking for the formula in the rig directory (`/Users/unaziogah/gt/myproject/`) instead of the town-level formulas directory.

## Root Cause (Investigation Notes)

The issue is in `internal/cmd/sling_helpers.go` around line 534:

```go
cookCmd := exec.Command("bd", "--no-daemon", "cook", formulaName)
cookCmd.Dir = formulaWorkDir  // This is set to the rig path
```

The `formulaWorkDir` is resolved by `beads.ResolveHookDir()` which returns the rig path based on the bead's prefix. The `bd cook` command then looks for the formula in that directory instead of the town-level `.beads/formulas/` directory.

## Files to Investigate
- `internal/cmd/sling_helpers.go:528-540` - `InstantiateFormulaOnBead` function
- `internal/beads/routes.go:245` - `ResolveHookDir` function
- Possibly a `bd` (beads CLI) issue with formula resolution

## Workaround
Use `--hook-raw-bead` flag when slinging:
```bash
gt sling <bead-id> <rig> --hook-raw-bead
```

This bypasses formula instantiation but means polecats won't automatically close issues when done.

## Impact
- Polecats don't get the workflow instructions from the formula
- Issues must be manually closed after work is complete
- Reduces autonomous operation capability

## Discovered
2026-01-29 while learning Gas Town workflow
