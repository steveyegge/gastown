# Research: --no-daemon Deprecation Impact and Fix Options

**Research task**: gt-y6pyqp.1
**Researcher**: gastown/polecats/furiosa
**Date**: 2026-02-02
**Related bugs**: gu-cg7, gt-wuld20

## Executive Summary

The `--no-daemon` flag has been deprecated in bd v0.50.0 and now returns an error when used. However, **the gt production code has already been fixed** to not use `--no-daemon`. The bugs gu-cg7 and gt-wuld20 appear to be outdated - current gt (v0.2.5-1382) works without errors.

The remaining issues are:
1. **Stale comments** in source code that reference `--no-daemon`
2. **Test files** that use `--no-daemon` for isolation (will fail with bd v0.50.0)

## Findings

### 1. Where --no-daemon Was Used

The `--no-daemon` flag was previously used to bypass the bd daemon and connect directly to the database. This was originally needed to avoid contention issues with daemon startup.

**Historical fix reference** (from `internal/mail/bd.go`):
```go
// Use the daemon for connection pooling. Previous --no-daemon was causing
// massive connection churn (~17 connections/second with 32 agents).
// See: hq-i97ri for the fix, hq-vvbubs/hq-33lwcx for original daemon issues.
```

### 2. Current Production Code Status

**Production code has been fixed** - no `--no-daemon` in actual execution paths:

| File | Status |
|------|--------|
| `internal/beads/beads.go:run()` | Uses `--allow-stale` only, no `--no-daemon` |
| `internal/mail/bd.go:runBdCommand()` | Explicitly removed `--no-daemon` |
| `internal/cmd/convoy.go` | Code doesn't use it, only stale comments |
| `internal/cmd/prime_molecule.go` | Only a comment, no actual usage |

### 3. Remaining Issues

#### 3.1 Stale Comments

Several files have comments referencing `--no-daemon` that are now misleading:

- `internal/cmd/convoy.go:1682-1683` - Comment says "bd --no-daemon show" but code doesn't use it
- `internal/cmd/convoy.go:1730` - Comment says "Use --no-daemon" but code doesn't
- `internal/cmd/convoy.go:1738` - Comment about "--no-daemon exit 0 bug"
- `internal/cmd/prime_molecule.go:47` - Comment about "--no-daemon exit 0 bug"
- `internal/beads/beads.go:263-264` - Comment about "--no-daemon exit code 0 bug"

#### 3.2 Test Files Using --no-daemon

Per `docs/storage-backends.md`, tests legitimately use `--no-daemon` for isolation:
> Tests legitimately use `--no-daemon` for isolation

However, with bd v0.50.0 returning an error for `--no-daemon`, these tests will fail:

- `internal/beads/beads_test.go`
- `internal/cmd/beads_db_init_test.go`
- `internal/cmd/beads_routing_integration_test.go`
- `internal/cmd/decision_integration_test.go`
- `internal/cmd/done_test.go`
- `internal/cmd/hook_slot_integration_test.go`
- `internal/cmd/install_integration_test.go`
- `internal/cmd/routes_jsonl_corruption_test.go`

### 4. Why --no-daemon Was Deprecated

From the documentation and code comments:

1. **Connection churn**: Direct database access created ~17 connections/second with 32 agents
2. **Dolt incompatibility**: Direct mode sends SQLite-specific queries to Dolt (from `docs/storage-backends.md`)
3. **Daemon provides pooling**: The bd daemon provides connection pooling for efficient multi-agent operation

## Fix Options

### Option A: Remove Stale Comments (Low effort)

Update the misleading comments in production code:
- `convoy.go` lines 1682-1683, 1730, 1738
- `prime_molecule.go` line 47
- `beads.go` lines 263-264

**Scope**: ~15 lines of comment changes

### Option B: Update Test Isolation Strategy (Medium effort)

Tests need an alternative to `--no-daemon` for isolation. Options:
1. Use a test-specific daemon instance
2. Use in-memory SQLite database
3. Request bd to support `--no-daemon` for testing only (environment variable or test mode)
4. Use separate database files per test

**Scope**: 8+ test files need updates

### Option C: Update bd to Allow --no-daemon in Test Mode (Beads team)

Request the beads team to:
1. Allow `--no-daemon` when `BD_TEST_MODE=1` is set
2. Or provide an alternative flag for test isolation

**Scope**: Changes in beads codebase, not gastown

### Option D: Close Bugs as Resolved (Immediate)

The bugs gu-cg7 and gt-wuld20 may already be resolved by the production code fixes. Verify and close if:
1. `gt hook status` works without error (confirmed)
2. `gt mail inbox` works without error (confirmed)
3. Refinery operations work without error (needs verification)

## Recommendations

1. **Immediate**: Close gu-cg7 if production commands work (they do in current testing)
2. **Short-term**: Clean up stale comments (Option A)
3. **Medium-term**: Fix tests to not rely on --no-daemon (Option B) or coordinate with beads team (Option C)
4. **Verify**: Check if gt-wuld20 Refinery issue is still occurring with current binaries

## Verification Results

```bash
$ gt --version
gt version v0.2.5-1382-g8f428401

$ gt hook status  # Works without error
$ gt mail inbox   # Works without error
$ bd --no-daemon list  # Returns error: "The --no-daemon flag is deprecated"
```

The production gt binary no longer triggers the deprecation error. The bugs appear to be from an older version (v0.2.5-1311) that has since been fixed.
