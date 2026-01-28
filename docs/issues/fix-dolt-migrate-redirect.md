# Bug: `gt dolt migrate` does not follow `.beads/redirect` for rig databases

## Summary

`gt dolt migrate` only migrates the HQ database. Rig databases are silently skipped because `FindMigratableDatabases` does not follow the `.beads/redirect` file present in each rig's `.beads/` directory.

## Steps to Reproduce

1. Have a town with rigs that use tracked beads (e.g., `nexus`, `nrpk`)
2. Each rig has `<rig>/.beads/redirect` containing `mayor/rig/.beads`
3. The actual Dolt database lives at `<rig>/mayor/rig/.beads/dolt/beads/`
4. Run `gt dolt migrate`

## Expected Behavior

All rig databases should be discovered and migrated to `~/gt/.dolt-data/<rigName>/`.

## Actual Behavior

Only the HQ database is migrated. Rig databases are not found because `FindMigratableDatabases` looks at `<rig>/.beads/dolt/beads/` directly, ignoring the redirect file.

## Root Cause

In `internal/doltserver/doltserver.go`, `FindMigratableDatabases` hardcodes the beads path for both the town-level HQ and per-rig databases:

```go
// Town-level (HQ)
townSource := filepath.Join(townRoot, ".beads", "dolt", "beads")

// Per-rig
rigSource := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads")
```

Neither path calls `beads.ResolveBeadsDir()` to follow the redirect chain. The rest of the codebase (catalog, routes, types, etc.) correctly uses `ResolveBeadsDir` for this purpose.

## Fix

Replace both hardcoded paths with calls to `beads.ResolveBeadsDir()`:

```go
// Town-level (HQ)
townBeadsDir := beads.ResolveBeadsDir(townRoot)
townSource := filepath.Join(townBeadsDir, "dolt", "beads")

// Per-rig
resolvedBeadsDir := beads.ResolveBeadsDir(filepath.Join(townRoot, rigName))
rigSource := filepath.Join(resolvedBeadsDir, "dolt", "beads")
```

## Impact

Any town with rigs using tracked beads (redirect-based) cannot migrate rig databases to the centralized `.dolt-data/` layout, leaving polecats stuck with embedded Dolt mode and its single-writer limitation (read-only errors under concurrent access).
