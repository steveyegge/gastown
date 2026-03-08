# Cross-DB Routing Consistency Audit

**Issue:** gt-g10v7 / GH#2423
**Date:** 2026-03-07

## Background

Multiple recent PRs (#2402, #2409, #2412) fixed cross-DB routing issues where
operations work correctly for town-level beads but fail for rig-prefixed IDs.
This audit examined all `bd` subprocess calls to identify remaining instances.

## Methodology

Audited three call patterns:
1. **`BdCmd()` builder** (`internal/cmd/bd_helpers.go`) — 48 call sites
2. **`runBdCommand()`** (`internal/mail/bd.go`) — 18 call sites
3. **Direct `exec.Command("bd", ...)`** — ~100+ call sites

## Safe Patterns (use as templates)

### Pattern A: Rig-prefixed bead with dynamic routing
```go
BdCmd("show", beadID, "--json").
    Dir(resolveBeadDir(beadID)).
    StripBeadsDir().
    Output()
```
Used in: `sling_helpers.go`, `bead.go`

### Pattern B: Town-level beads only
```go
BdCmd("create", "--type=convoy", ...).
    Dir(townBeads).
    WithAutoCommit().
    Run()
```
Used in: `convoy.go`, `formula.go`

### Pattern C: Explicit resolved beads dir
```go
BdCmd("mol", "wisp", "create", ...).
    WithBeadsDir(beads.ResolveBeadsDir(cfg.BeadsDir)).
    Dir(cfg.BeadsDir).
    Run()
```
Used in: `patrol_helpers.go`

## Issues Filed

| Issue | Priority | Location | Problem |
|-------|----------|----------|---------|
| gt-1gunh | P2 | convoy, sling_convoy, convoy_stage | dep add/remove with rig-prefixed beads uses town Dir without StripBeadsDir |
| gt-3q7pj | P2 | mailbox.go, delivery.go | Operations on bead IDs without prefix validation against beadsDir |
| gt-mmr06 | P3 | sling_helpers.go, sling_formula.go | Formula/molecule ops missing StripBeadsDir |
| gt-xpclw | P3 | handoff.go:1339 | show command has no Dir set for rig-prefixed beadID |
| gt-b8wo7 | P3 | wisps_migrate.go | bd sql helpers inherit BEADS_DIR without stripping |
| gt-99jw0 | P3 | sling_formula.go:62,68 | verifyFormulaExists has no Dir for formula lookup |

## Recommendations

1. **Linter rule**: Add a check that all `BdCmd()` calls either use
   `StripBeadsDir()` or `WithBeadsDir()` when a bead ID is passed as argument.
2. **Migrate direct calls**: Convert remaining `exec.Command("bd", ...)`
   to use `BdCmd()` for consistent env management.
3. **Document routing contract**: Add godoc to `BdCmd` explaining when
   `StripBeadsDir()` vs `WithBeadsDir()` is required.
