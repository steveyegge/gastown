# mol-commit-audit Report: gt-mol-bjz

## Summary

Completed audit and commit of uncommitted changes across the pihealth repository.

## Findings

### Scanned Changes
- **Modified tracked files:** Config, state, formula, and route files
- **Deleted files (restored):** 3 crew settings.json files that were accidentally deleted
- **Deleted files (ephemeral):** SQLite WAL files (beads.db-shm, beads.db-wal) - not high-risk

### Files Committed
1. `.beads/config.yaml` - Beads configuration
2. `.beads/formulas/e2e-fix-1test.formula.toml` - Formula update
3. `.beads/routes.jsonl` - Routing configuration
4. `fhc/.beads/config.yaml` - FHC beads config
5. `gastown/witness/state.json` - Witness state
6. `mayor/rigs.json` - Rig registry

### Restored Files
Crew settings.json files were found deleted and restored from HEAD:
- `gastown/crew/commit_auditor/.claude/settings.json`
- `gastown/crew/upstream_sync/.claude/settings.json`
- `gastown/crew/validator/.claude/settings.json`

### Skipped (Not High-Risk)
- SQLite database files (`*.beads.db`) - regenerated automatically
- SQLite WAL files (`beads.db-shm`, `beads.db-wal`) - ephemeral
- Submodule changes - require separate handling

## Actions Taken
1. Scanned `/home/ubuntu/pihealth` with `git status --porcelain`
2. Restored deleted crew settings.json files
3. Staged and committed 6 tracked files
4. Pushed to origin/master (commit 5a3e93db)

## High-Risk Deletions
**None found.** Only ephemeral SQLite WAL files were deleted.

## Recommendations
- Investigate why crew settings.json files keep getting deleted (recurring issue per git log)
- Consider adding a cron job or hook to prevent accidental deletions
