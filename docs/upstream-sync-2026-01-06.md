# Upstream Sync Findings (2026-01-06)

## Summary
- Merged `upstream/main` into `sync/upstream-main-2026-01-06` and resolved two conflicts.
- Updated beads runtime/test sync behavior to avoid stale JSONL failures after pulls.
- Full Go + GUI test suites pass; Vitest emits a deprecation warning about `test.poolOptions`.

## Conflicts Resolved
- `internal/beads/beads.go`: kept local `beads.Command`/`ApplyEnv` helpers and upstream `NewWithBeadsDir`; ensured BEADS_DIR/BEADS_JSONL overrides are preserved when `beadsDir` is set.
- `internal/session/manager.go`: kept `beads.Command` for `bd update` to ensure BEADS env is injected in worktrees.

## Compatibility Changes
- `internal/beads/beads.go`: auto-sync on "Database out of sync" checks both stdout/stderr; preserves env overrides when retrying.
- `internal/beads/beads_test.go`: pre-test sync now runs `bd --no-daemon sync --import-only` to avoid daemon mismatch.

## Upstream Content Notes
- Upstream removed tracked `.beads` issue data files; formulas/config remain tracked. Local DB/JSONL may still exist but are now untracked.

## Tests
- `go test ./...`
- `npm test` (in `gui/`) — warning: `test.poolOptions` deprecated in Vitest 4.

## Follow-up Candidates
- Update Vitest config to remove `test.poolOptions` usage per the Vitest 4 migration guide. (Issue: gt-dn3ar)
